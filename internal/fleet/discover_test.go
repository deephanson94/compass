package fleet_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/deephanson94/compass/internal/fleet"
)

// The ids and timestamps baked into testdata/tree.
const (
	sessionA = "aaaaaaaa-1111-4111-8111-aaaaaaaaaaaa" // -home-user-alpha, oldest
	sessionB = "bbbbbbbb-2222-4222-8222-bbbbbbbbbbbb" // -home-user-alpha, middle
	sessionC = "cccccccc-3333-4333-8333-cccccccccccc" // -home-user-beta, newest
)

var treeBase = time.Date(2026, 8, 30, 11, 0, 0, 0, time.UTC)

// longPromptFirstLine is the first line of session B's opening prompt; it is
// deliberately longer than the 80-rune title budget.
const longPromptFirstLine = "refactor the cache layer so that warm entries survive a restart and cold reads are logged for the ops team"

// copyTestdataTree materialises testdata/tree as a throwaway Claude home and
// stamps deterministic mtimes so that mtime- and event-derived orderings agree.
func copyTestdataTree(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "claude")
	src := filepath.Join("..", "..", "testdata", "tree")
	if err := os.CopyFS(root, os.DirFS(src)); err != nil {
		t.Fatalf("copy %s -> %s: %v", src, root, err)
	}

	mtimes := map[string]time.Time{
		filepath.Join(root, "projects", "-home-user-alpha", sessionA+".jsonl"): treeBase.Add(60 * time.Second),
		filepath.Join(root, "projects", "-home-user-alpha", sessionB+".jsonl"): treeBase.Add(1830 * time.Second),
		filepath.Join(root, "projects", "-home-user-alpha", "empty.jsonl"):     treeBase.Add(2 * time.Hour),
		filepath.Join(root, "projects", "-home-user-beta", sessionC+".jsonl"):  treeBase.Add(3660 * time.Second),
	}
	for path, mt := range mtimes {
		if err := os.Chtimes(path, mt, mt); err != nil {
			t.Fatalf("chtimes %s: %v", path, err)
		}
	}
	return root
}

func findSession(t *testing.T, got []fleet.SessionInfo, id string) fleet.SessionInfo {
	t.Helper()
	for _, s := range got {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("session %s not found in %v", id, idsOf(got))
	return fleet.SessionInfo{}
}

func idsOf(got []fleet.SessionInfo) []string {
	out := make([]string, 0, len(got))
	for _, s := range got {
		out = append(out, s.ID)
	}
	return out
}

// T13 — two project slugs, three real transcripts, one empty file that must be
// skipped, and one session subdirectory holding a subagent transcript that must
// NOT be recursed into (subagents are M1).
func TestT13DiscoverSkipsEmptyFilesAndSubagentDirs(t *testing.T) {
	root := copyTestdataTree(t)

	got, err := fleet.Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("Discover returned %d sessions %v, want 3", len(got), idsOf(got))
	}

	// Sorted by LastEventAt descending.
	want := []string{sessionC, sessionB, sessionA}
	if gotIDs := idsOf(got); strings.Join(gotIDs, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v (LastEventAt descending)", gotIDs, want)
	}

	for _, s := range got {
		if s.ID == "empty" {
			t.Error("Discover returned the empty transcript; empty files must be skipped")
		}
		if s.ID == "agent-x" || strings.Contains(filepath.ToSlash(s.TranscriptPath), "/subagents/") {
			t.Errorf("Discover recursed into a session subdirectory: %s", s.TranscriptPath)
		}
		if strings.HasSuffix(s.TranscriptPath, ".txt") {
			t.Errorf("Discover picked up a non-jsonl file: %s", s.TranscriptPath)
		}
	}

	t.Run("fields", func(t *testing.T) {
		a := findSession(t, got, sessionA)
		if want := filepath.Join(root, "projects", "-home-user-alpha", sessionA+".jsonl"); a.TranscriptPath != want {
			t.Errorf("TranscriptPath = %q, want %q", a.TranscriptPath, want)
		}
		if a.ProjectSlug != "-home-user-alpha" {
			t.Errorf("ProjectSlug = %q, want %q", a.ProjectSlug, "-home-user-alpha")
		}
		if a.CWD != "/home/user/alpha" {
			t.Errorf("CWD = %q, want %q", a.CWD, "/home/user/alpha")
		}
		if a.GitBranch != "main" {
			t.Errorf("GitBranch = %q, want %q", a.GitBranch, "main")
		}
		if a.Title != "add retries to the http client" {
			t.Errorf("Title = %q, want %q", a.Title, "add retries to the http client")
		}
		if want := treeBase; !a.StartedAt.Equal(want) {
			t.Errorf("StartedAt = %v, want %v (first event timestamp)", a.StartedAt, want)
		}
		if want := treeBase.Add(60 * time.Second); !a.LastEventAt.Equal(want) {
			t.Errorf("LastEventAt = %v, want %v (last event timestamp)", a.LastEventAt, want)
		}

		c := findSession(t, got, sessionC)
		if c.ProjectSlug != "-home-user-beta" {
			t.Errorf("ProjectSlug = %q, want %q", c.ProjectSlug, "-home-user-beta")
		}
		if c.GitBranch != "main" || c.CWD != "/home/user/beta" {
			t.Errorf("cwd/branch = %q/%q, want /home/user/beta/main", c.CWD, c.GitBranch)
		}
		// The prompt has a second line; only the first becomes the title.
		if c.Title != "why is the cache cold on boot?" {
			t.Errorf("Title = %q, want only the first line of the prompt", c.Title)
		}
	})

	t.Run("long title is cut to 80 runes with an ellipsis", func(t *testing.T) {
		b := findSession(t, got, sessionB)
		if strings.ContainsAny(b.Title, "\n\r") {
			t.Fatalf("Title = %q, want a single line", b.Title)
		}
		if !strings.HasSuffix(b.Title, "…") {
			t.Fatalf("Title = %q, want a trailing … because the prompt was cut", b.Title)
		}
		// The contract says "first line, max 80 runes, … if cut"; accept the
		// ellipsis being inside or just past that budget, nothing looser.
		if n := utf8.RuneCountInString(b.Title); n < 79 || n > 81 {
			t.Errorf("Title is %d runes (%q), want ~80", n, b.Title)
		}
		head := strings.TrimSuffix(b.Title, "…")
		if !strings.HasPrefix(longPromptFirstLine, head) {
			t.Errorf("Title head %q is not a prefix of the prompt %q", head, longPromptFirstLine)
		}
	})
}

func TestDiscoverMissingRootOrProjectsDir(t *testing.T) {
	t.Run("root does not exist", func(t *testing.T) {
		got, err := fleet.Discover(filepath.Join(t.TempDir(), "no-such-home"))
		if err != nil {
			t.Fatalf("Discover on a missing root returned error %v, want nil", err)
		}
		if got != nil {
			t.Errorf("Discover on a missing root returned %v, want nil", idsOf(got))
		}
	})

	t.Run("root exists but has no projects dir", func(t *testing.T) {
		got, err := fleet.Discover(t.TempDir())
		if err != nil {
			t.Fatalf("Discover without projects/ returned error %v, want nil", err)
		}
		if got != nil {
			t.Errorf("Discover without projects/ returned %v, want nil", idsOf(got))
		}
	})

	t.Run("projects dir is empty", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "projects"), 0o755); err != nil {
			t.Fatal(err)
		}
		got, err := fleet.Discover(root)
		if err != nil {
			t.Fatalf("Discover on an empty projects/ returned error %v, want nil", err)
		}
		if len(got) != 0 {
			t.Errorf("Discover on an empty projects/ returned %v, want none", idsOf(got))
		}
	})
}

// The event timestamps win; the mtime is only a fallback. (A transcript can be
// touched — copied, rsynced, checked out — long after its last event.)
func TestDiscoverPrefersEventTimestampsOverMtime(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "projects", "-home-user-gamma")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const id = "eeeeeeee-5555-4555-8555-eeeeeeeeeeee"
	path := filepath.Join(dir, id+".jsonl")

	first := time.Date(2026, 8, 30, 8, 0, 0, 0, time.UTC)
	last := time.Date(2026, 8, 30, 8, 5, 0, 0, time.UTC)
	body := `{"parentUuid":null,"isSidechain":false,"type":"user","message":{"role":"user",` +
		`"content":"vendor the http client"},"uuid":"e1","timestamp":"2026-08-30T08:00:00.000Z",` +
		`"cwd":"/home/user/gamma","sessionId":"` + id + `","version":"2.1.251","gitBranch":"chore/vendor"}` + "\n" +
		`{"parentUuid":"e1","isSidechain":false,"type":"assistant","message":{"role":"assistant",` +
		`"content":[{"type":"text","text":"Vendored."}],"stop_reason":"end_turn"},"uuid":"e2",` +
		`"timestamp":"2026-08-30T08:05:00.000Z","cwd":"/home/user/gamma","sessionId":"` + id + `",` +
		`"version":"2.1.251","gitBranch":"chore/vendor"}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	// Touched hours after the session actually went quiet.
	touched := time.Date(2026, 8, 30, 20, 0, 0, 0, time.UTC)
	if err := os.Chtimes(path, touched, touched); err != nil {
		t.Fatal(err)
	}

	got, err := fleet.Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Discover returned %d sessions, want 1", len(got))
	}
	s := got[0]
	if !s.LastEventAt.Equal(last) {
		t.Errorf("LastEventAt = %v, want the last event timestamp %v (mtime %v is only a fallback)",
			s.LastEventAt, last, touched)
	}
	if !s.StartedAt.Equal(first) {
		t.Errorf("StartedAt = %v, want the first event timestamp %v", s.StartedAt, first)
	}
	if s.Title != "vendor the http client" || s.CWD != "/home/user/gamma" || s.GitBranch != "chore/vendor" {
		t.Errorf("title/cwd/branch = %q/%q/%q, want the values from the transcript",
			s.Title, s.CWD, s.GitBranch)
	}
}

// LastEventAt falls back to the file mtime when no line carries a timestamp.
func TestDiscoverLastEventAtFallsBackToMtime(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "projects", "-home-user-gamma")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "dddddddd-4444-4444-8444-dddddddddddd.jsonl")
	body := `{"type":"atis-latch","atis":"","sessionId":"dddddddd-4444-4444-8444-dddddddddddd"}` + "\n" +
		`{"type":"mode","mode":"normal","sessionId":"dddddddd-4444-4444-8444-dddddddddddd"}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	mtime := time.Date(2026, 8, 30, 9, 30, 0, 0, time.UTC)
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}

	got, err := fleet.Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Discover returned %d sessions, want 1", len(got))
	}
	if got[0].LastEventAt.Unix() != mtime.Unix() {
		t.Errorf("LastEventAt = %v, want the file mtime %v", got[0].LastEventAt, mtime)
	}
}

// The row's second line answers "what did I ask it". It used to answer with
// whatever user line came first, which in a session started by a slash command
// is the caveat the harness writes before the command's own output — so nine
// live sessions reported "<local-command-caveat>Caveat…" and "<command-message>
// kickoff</co…" instead of any work.
func TestDiscoverTitlesSkipMachineryAndReadSlashCommandsAsTyped(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "projects", "-home-user-porter")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const id = "77777777-7777-4777-8777-777777777777"

	// The opening of a real session: the caveat, then the expanded command,
	// then the model. Nothing a person typed appears in plain text at all.
	body := `{"type":"user","isMeta":true,"message":{"role":"user","content":` +
		`"<local-command-caveat>Caveat: The messages below were generated by the user while running ` +
		`local commands. DO NOT respond to these messages.</local-command-caveat>"},"uuid":"p1",` +
		`"timestamp":"2026-08-30T09:00:00.000Z","cwd":"/home/user/porter","sessionId":"` + id + `",` +
		`"gitBranch":"main"}` + "\n" +
		`{"type":"user","message":{"role":"user","content":"<command-name>/kickoff</command-name>\n` +
		`            <command-message>kickoff</command-message>\n` +
		`            <command-args>probe the prefix tree</command-args>"},"uuid":"p2",` +
		`"timestamp":"2026-08-30T09:00:01.000Z","cwd":"/home/user/porter","sessionId":"` + id + `",` +
		`"gitBranch":"main"}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"On it."}],` +
		`"stop_reason":"end_turn"},"uuid":"p3","timestamp":"2026-08-30T09:00:30.000Z",` +
		`"cwd":"/home/user/porter","sessionId":"` + id + `","gitBranch":"main"}` + "\n"

	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := fleet.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	s := findSession(t, got, id)
	if want := "/kickoff probe the prefix tree"; s.Title != want {
		t.Errorf("Title = %q, want %q", s.Title, want)
	}
	// The whole point: no wrapper survives into the row.
	for _, bad := range []string{"command-message", "local-command-caveat", "Caveat"} {
		if strings.Contains(s.Title, bad) {
			t.Errorf("Title = %q; it still carries the harness's own %q", s.Title, bad)
		}
	}
	// A session whose first line is machinery still started when it started.
	if want := time.Date(2026, 8, 30, 9, 0, 0, 0, time.UTC); !s.StartedAt.Equal(want) {
		t.Errorf("StartedAt = %v, want %v", s.StartedAt, want)
	}
}

// A session with nothing but machinery in it has no title rather than a wrong
// one: an empty second line falls through to the state, which is at least true.
func TestDiscoverLeavesTheTitleEmptyWhenNobodyHasSpoken(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "projects", "-home-user-quiet")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const id = "88888888-8888-4888-8888-888888888888"
	body := `{"type":"user","isMeta":true,"message":{"role":"user","content":` +
		`"Continue from where you left off."},"uuid":"q1",` +
		`"timestamp":"2026-08-30T09:00:00.000Z","cwd":"/home/user/quiet","sessionId":"` + id + `",` +
		`"gitBranch":"main"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := fleet.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if s := findSession(t, got, id); s.Title != "" {
		t.Errorf("Title = %q, want empty: the harness talking to itself is not a prompt", s.Title)
	}
}

// The title is the strongest thing you said, not the first. The first thing
// most people type is "/model", and four of nine rows said so.
func TestDiscoverPrefersProseOverASlashCommand(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "projects", "-home-user-ranked")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	line := func(uuid, ts, content string) string {
		return `{"type":"user","message":{"role":"user","content":"` + content + `"},"uuid":"` + uuid + `",` +
			`"timestamp":"` + ts + `","cwd":"/home/user/ranked","sessionId":"SID","gitBranch":"main"}` + "\n"
	}
	const bareModel = `<command-name>/model</command-name>\n<command-message>model</command-message>`
	const kickoff = `<command-name>/kickoff</command-name>\n<command-message>kickoff</command-message>\n<command-args>probe the tree</command-args>`

	cases := []struct {
		id, want string
		lines    []string
	}{
		{"99999999-0001-4999-8999-999999999999", "fix the 401 bug", []string{
			line("a", "2026-08-30T09:00:00.000Z", bareModel),
			line("b", "2026-08-30T09:00:01.000Z", kickoff),
			line("c", "2026-08-30T09:00:02.000Z", "fix the 401 bug"),
		}},
		{"99999999-0002-4999-8999-999999999999", "/kickoff probe the tree", []string{
			line("a", "2026-08-30T09:00:00.000Z", bareModel),
			line("b", "2026-08-30T09:00:01.000Z", kickoff),
		}},
		{"99999999-0003-4999-8999-999999999999", "/model", []string{
			line("a", "2026-08-30T09:00:00.000Z", bareModel),
		}},
	}
	for _, c := range cases {
		body := strings.ReplaceAll(strings.Join(c.lines, ""), "SID", c.id)
		if err := os.WriteFile(filepath.Join(dir, c.id+".jsonl"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := fleet.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if s := findSession(t, got, c.id); s.Title != c.want {
			t.Errorf("Title = %q, want %q", s.Title, c.want)
		}
	}
}

// The wrapper the last dogfood found: a local command's captured output,
// which carries no isMeta and which the title ranking promoted over "/model"
// as prose. The rule is now the shape — a turn that opens with a tag — so the
// next wrapper is covered before anyone sees it.
func TestDiscoverIgnoresCapturedCommandOutput(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "projects", "-home-user-stdout")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	line := func(uuid, ts, content string) string {
		return `{"type":"user","message":{"role":"user","content":"` + content + `"},"uuid":"` + uuid + `",` +
			`"timestamp":"` + ts + `","cwd":"/home/user/stdout","sessionId":"SID","gitBranch":"main"}` + "\n"
	}
	const stdout = `<local-command-stdout>Set model to claude-opus-5</local-command-stdout>`
	cases := []struct {
		id, want string
		lines    []string
	}{
		{"77777777-0001-4777-8777-777777777777", "fix the 401 bug", []string{
			line("a", "2026-08-30T09:00:00.000Z", stdout),
			line("b", "2026-08-30T09:00:01.000Z", "fix the 401 bug"),
		}},
		{"77777777-0002-4777-8777-777777777777", "", []string{
			line("a", "2026-08-30T09:00:00.000Z", stdout),
		}},
	}
	for _, c := range cases {
		body := strings.ReplaceAll(strings.Join(c.lines, ""), "SID", c.id)
		if err := os.WriteFile(filepath.Join(dir, c.id+".jsonl"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := fleet.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if s := findSession(t, got, c.id); s.Title != c.want {
			t.Errorf("Title = %q, want %q", s.Title, c.want)
		}
	}
}
