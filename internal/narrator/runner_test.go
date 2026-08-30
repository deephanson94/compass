package narrator_test

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/deephanson94/compass/internal/narrator"
)

// ---------------------------------------------------------------- fixtures

// twoDigests is the T45 batch: one fully populated leg and one that has nothing
// but an identity, so every Args assertion also exercises the empty-field path.
func twoDigests() []narrator.Digest {
	return []narrator.Digest{
		{
			Key:       "sess-alpha/1788091200000000000/build",
			Class:     "build",
			Label:     "auth.go",
			Files:     []string{"auth.go", "token.go"},
			Waypoints: []string{"18 passed, 2 failed", "TestRefreshRotatesTheToken"},
			Prompt:    "make refresh rotate the token without logging anyone out",
		},
		{
			Key:   "sess-alpha/1788091440000000000/test",
			Class: "test",
			// Label, Files, Waypoints and Prompt deliberately zero.
		},
	}
}

// wantFlags is the exact flag prefix of the argv, in order. The prompt is the
// one element that follows.
var wantFlags = []string{"-p", "--model", "haiku", "--output-format", "json"}

// argvPrompt checks the flag prefix and returns the single trailing prompt
// argument. "exactly" in the contract means six elements: five flags and one
// prompt — the digests must not leak out as extra argv words.
func argvPrompt(t *testing.T, args []string, wantModel string) string {
	t.Helper()
	if len(args) != 6 {
		t.Fatalf("Args returned %d elements, want exactly 6 (5 flags + 1 prompt):\n%q", len(args), args)
	}
	want := append([]string(nil), wantFlags...)
	want[2] = wantModel
	for i, w := range want {
		if args[i] != w {
			t.Fatalf("Args[%d] = %q, want %q\nfull argv: %q", i, args[i], w, args)
		}
	}
	return args[5]
}

func mustContain(t *testing.T, prompt, needle, why string) {
	t.Helper()
	if !strings.Contains(prompt, needle) {
		t.Errorf("prompt is missing %s (%q)\n---prompt---\n%s\n------------", why, needle, prompt)
	}
}

// ---------------------------------------------------------------- T45

// T45 — the argv is exactly the headless invocation the contract names, with
// the whole batch folded into the single prompt argument.
func TestT45ArgsIsTheHeadlessInvocation(t *testing.T) {
	r := &narrator.CLIRunner{Dir: t.TempDir()}
	digests := twoDigests()
	prompt := argvPrompt(t, r.Args(digests), "haiku")

	// Every digest's identity must reach the model: the keys are what come
	// back, so a batch whose keys are not in the prompt can never be applied.
	for _, d := range digests {
		mustContain(t, prompt, d.Key, "a digest key")
	}

	// The rest of what the model sees about the first leg.
	mustContain(t, prompt, "build", "the first digest's class")
	mustContain(t, prompt, "test", "the second digest's class")
	mustContain(t, prompt, "auth.go", "the first digest's label/file")
	mustContain(t, prompt, "token.go", "the first digest's second file")
	mustContain(t, prompt, "18 passed, 2 failed", "the first digest's waypoint")
	mustContain(t, prompt, "TestRefreshRotatesTheToken", "the first digest's second waypoint")
	mustContain(t, prompt, "make refresh rotate the token", "the first digest's prompt context")

	// The JSON-only instruction (contract, narrator section): return ONLY a
	// JSON array of {"key","label"}, ≤5 words per label.
	low := strings.ToLower(prompt)
	for _, needle := range []string{"json", "only", "key", "label", "word"} {
		if !strings.Contains(low, needle) {
			t.Errorf("prompt does not instruct %q — the JSON-only instruction is missing\n---prompt---\n%s\n------------", needle, prompt)
		}
	}
}

// The CLI alias is the default; an explicit model replaces it and nothing else.
func TestT45ArgsHonoursACustomModel(t *testing.T) {
	r := &narrator.CLIRunner{Bin: "/opt/homebrew/bin/claude", Model: "claude-haiku-4-5", Dir: t.TempDir()}
	args := r.Args(twoDigests())
	argvPrompt(t, args, "claude-haiku-4-5")

	// Args excludes Bin (the contract is explicit): the binary is the exec
	// path, never an argument.
	for i, a := range args {
		if strings.Contains(a, "/opt/homebrew/bin/claude") {
			t.Errorf("Args[%d] = %q leaks Bin into the argv", i, a)
		}
	}
}

// A zero-value runner still produces the default invocation: Bin and Dir have
// nothing to do with the argv, and an empty Model means "haiku".
func TestT45ArgsZeroValueRunnerUsesTheDefaultModel(t *testing.T) {
	var r narrator.CLIRunner
	argvPrompt(t, r.Args(twoDigests()), "haiku")
}

// Digests with empty Label, Files, Waypoints and Prompt must marshal cleanly:
// no "<nil>", no "%!" verb wreckage, no broken UTF-8, and the keys still there.
func TestT45ArgsEmptyDigestFieldsMarshalCleanly(t *testing.T) {
	digests := []narrator.Digest{
		{Key: "sess-beta/1788091200000000000/scout", Class: "scout"},
		{Key: "sess-beta/1788091440000000000/docs", Class: "docs"},
	}
	r := &narrator.CLIRunner{Dir: t.TempDir()}
	prompt := argvPrompt(t, r.Args(digests), "haiku")

	for _, d := range digests {
		mustContain(t, prompt, d.Key, "a digest key")
	}
	for _, junk := range []string{"<nil>", "%!", "[]string", "map[", "ObjectsAreEqual"} {
		if strings.Contains(prompt, junk) {
			t.Errorf("prompt contains Go-formatting debris %q — digests are not being marshaled cleanly\n---prompt---\n%s\n------------", junk, prompt)
		}
	}
	if !utf8.ValidString(prompt) {
		t.Errorf("prompt is not valid UTF-8")
	}
}

// A batch of one is the common case; it must not be special-cased away.
func TestT45ArgsSingleDigest(t *testing.T) {
	d := narrator.Digest{Key: "sess-gamma/1788091200000000000/fix", Class: "fix", Label: "tailer.go"}
	prompt := argvPrompt(t, (&narrator.CLIRunner{}).Args([]narrator.Digest{d}), "haiku")
	mustContain(t, prompt, d.Key, "the only digest's key")
}

// ---------------------------------------------------------------- T46

// envelope wraps a model response in the CLI's --output-format json envelope,
// extra bookkeeping keys and all: ParseResponse must ignore everything but
// "result".
func envelope(t *testing.T, result string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"type":           "result",
		"subtype":        "success",
		"is_error":       false,
		"duration_ms":    1234,
		"num_turns":      1,
		"result":         result,
		"session_id":     "9f1c0000-0000-4000-8000-000000000001",
		"total_cost_usd": 0.00042,
		"usage":          map[string]any{"input_tokens": 812, "output_tokens": 44},
	})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return raw
}

func assertLabels(t *testing.T, got []narrator.Label, want ...narrator.Label) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d labels, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("labels[%d] = %+v, want %+v", i, got[i], w)
		}
	}
}

// T46 — the clean case, spelled out literally: the envelope's result field
// holds a bare JSON array.
func TestT46ParseResponseCleanEnvelope(t *testing.T) {
	out := []byte(`{"result":"[{\"key\":\"k1\",\"label\":\"maps the auth module\"}]"}`)
	got, err := narrator.ParseResponse(out)
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	assertLabels(t, got, narrator.Label{Key: "k1", Text: "maps the auth module"})
}

// The model fences its JSON more often than not.
func TestT46ParseResponseFencedArray(t *testing.T) {
	got, err := narrator.ParseResponse(envelope(t,
		"```json\n[{\"key\":\"k1\",\"label\":\"maps the auth module\"},{\"key\":\"k2\",\"label\":\"pins the token clock\"}]\n```"))
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	assertLabels(t, got,
		narrator.Label{Key: "k1", Text: "maps the auth module"},
		narrator.Label{Key: "k2", Text: "pins the token clock"},
	)
}

// A bare ``` fence with no language tag is just as common.
func TestT46ParseResponseUnlabelledFence(t *testing.T) {
	got, err := narrator.ParseResponse(envelope(t,
		"```\n[{\"key\":\"k1\",\"label\":\"maps the auth module\"}]\n```"))
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	assertLabels(t, got, narrator.Label{Key: "k1", Text: "maps the auth module"})
}

// Prose on both sides of the array — the instruction says "only JSON", the
// model does not always listen.
func TestT46ParseResponseSurroundingProse(t *testing.T) {
	got, err := narrator.ParseResponse(envelope(t,
		"Sure! Here are the labels you asked for:\n\n"+
			"[{\"key\":\"k1\",\"label\":\"maps the auth module\"}]\n\n"+
			"Let me know if you would like different wording."))
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	assertLabels(t, got, narrator.Label{Key: "k1", Text: "maps the auth module"})
}

// Extra keys on the array's objects are ignored, not fatal.
func TestT46ParseResponseIgnoresExtraObjectKeys(t *testing.T) {
	got, err := narrator.ParseResponse(envelope(t,
		`[{"key":"k1","label":"maps the auth module","confidence":0.91,"why":"it reads auth.go"}]`))
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	assertLabels(t, got, narrator.Label{Key: "k1", Text: "maps the auth module"})
}

// A 40-rune label is clipped to 32 runes. The exact clipping style is the
// implementation's business (a hard cut and an ellipsised cut are both ≤32 and
// both keep the first 31 runes), so this pins the width and the prefix, not the
// last rune.
func TestT46ParseResponseClipsLabelsTo32Runes(t *testing.T) {
	const long = "abcdefghijabcdefghijabcdefghijabcdefghij" // 40 runes, no spaces
	if got := utf8.RuneCountInString(long); got != 40 {
		t.Fatalf("fixture is %d runes, want 40", got)
	}
	got, err := narrator.ParseResponse(envelope(t,
		`[{"key":"k1","label":"`+long+`"}]`))
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d labels, want 1: %+v", len(got), got)
	}
	if n := utf8.RuneCountInString(got[0].Text); n != 32 {
		t.Errorf("clipped label is %d runes (%q), want 32", n, got[0].Text)
	}
	if want := string([]rune(long)[:31]); !strings.HasPrefix(got[0].Text, want) {
		t.Errorf("clipped label = %q, want it to start with the source's first 31 runes %q", got[0].Text, want)
	}
}

// Runes, not bytes: a 40-rune multi-byte label is 40 runes wide and must be cut
// on rune boundaries.
func TestT46ParseResponseClipsOnRunesNotBytes(t *testing.T) {
	long := strings.Repeat("é", 40) // 40 runes, 80 bytes
	got, err := narrator.ParseResponse(envelope(t, `[{"key":"k1","label":"`+long+`"}]`))
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d labels, want 1: %+v", len(got), got)
	}
	if n := utf8.RuneCountInString(got[0].Text); n != 32 {
		t.Errorf("clipped label is %d runes (%q), want 32 — clipping must count runes, not bytes", n, got[0].Text)
	}
	if !utf8.ValidString(got[0].Text) {
		t.Errorf("clipped label %q is not valid UTF-8 — clipping cut a rune in half", got[0].Text)
	}
}

// A label already inside the budget is returned untouched.
func TestT46ParseResponseLeavesShortLabelsAlone(t *testing.T) {
	got, err := narrator.ParseResponse(envelope(t, `[{"key":"k1","label":"tightens the retry loop"}]`))
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	assertLabels(t, got, narrator.Label{Key: "k1", Text: "tightens the retry loop"})
}

// ParseResponse does not know which keys were asked for — a key nobody
// requested still comes back. Filtering is the Narrator's job, and a parser
// that silently dropped rows would hide a model that is renaming things.
func TestT46ParseResponseReturnsUnknownKeys(t *testing.T) {
	got, err := narrator.ParseResponse(envelope(t,
		`[{"key":"k1","label":"maps the auth module"},{"key":"never-asked-for","label":"invented a leg"}]`))
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	assertLabels(t, got,
		narrator.Label{Key: "k1", Text: "maps the auth module"},
		narrator.Label{Key: "never-asked-for", Text: "invented a leg"},
	)
}

// An empty array is a valid, complete answer: no labels and no error.
func TestT46ParseResponseEmptyArray(t *testing.T) {
	got, err := narrator.ParseResponse(envelope(t, "[]"))
	if err != nil {
		t.Fatalf("ParseResponse on an empty array: %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d labels, want 0: %+v", len(got), got)
	}
}

// Everything that must fail. "applies nothing" is the point: a partial parse
// would poison the cache with half a batch.
func TestT46ParseResponseErrors(t *testing.T) {
	cases := []struct {
		name string
		out  []byte
	}{
		{"malformed envelope json", []byte(`{"result": "[{\"key\":\"k1\"`)},
		{"envelope is not an object", []byte(`["result"]`)},
		{"envelope is empty", []byte(``)},
		{"envelope is whitespace", []byte("  \n\t ")},
		{"result field missing", []byte(`{"type":"result","is_error":false}`)},
		{"result field is not a string", []byte(`{"result":{"labels":[]}}`)},
		{"result holds no array at all", envelope(t, "I could not name these legs, sorry.")},
		{"result holds an object, not an array", envelope(t, `{"key":"k1","label":"maps the auth module"}`)},
		{"result holds a truncated array", envelope(t, `[{"key":"k1","label":"maps the auth`)},
		{"result holds an unfenced fence and nothing else", envelope(t, "```json\n```")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := narrator.ParseResponse(tc.out)
			if err == nil {
				t.Fatalf("ParseResponse returned nil error, want one; labels = %+v", got)
			}
			if len(got) != 0 {
				t.Errorf("ParseResponse returned %d labels alongside its error, want none: %+v", len(got), got)
			}
		})
	}
}

// Rows that carry no key cannot be applied to anything. Dropping them keeps the
// rest of a good batch usable; the contract's "unknown keys are ignored" is the
// same instinct.
func TestT46ParseResponseSkipsKeylessRows(t *testing.T) {
	got, err := narrator.ParseResponse(envelope(t,
		`[{"label":"no key at all"},{"key":"k1","label":"maps the auth module"}]`))
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	for _, l := range got {
		if l.Key == "" {
			t.Fatalf("ParseResponse returned a keyless label %+v — it can never be applied", l)
		}
	}
	assertLabels(t, got, narrator.Label{Key: "k1", Text: "maps the auth module"})
}
