package narrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Defaults for the CLI call. "haiku" is the alias, not a pinned id: the CLI
// resolves it to whatever the current Haiku is, which is exactly the model we
// want and never a version we have to chase.
const (
	defaultBin   = "claude"
	defaultModel = "haiku"
)

// narrateTimeout bounds one batch. A narration that has not answered in a
// minute has lost its race with the panel it was meant to decorate.
const narrateTimeout = 60 * time.Second

// promptInstruction is the whole of what the model is told, verbatim. It lives
// in one const so the wording is a single thing to read, to change and to
// assert on.
const promptInstruction = `You name units of coding work.

Below is a JSON array of legs from one Claude Code session. Each leg has a key,
a class of work, the heuristic label it currently shows, the files it touched,
its waypoints (test runs, bugs, commits) and the user prompt it came from.

Return ONLY a JSON array of {"key","label"} objects, one per input leg, in the
same order. No prose, no explanation, no code fences.

Rules for each label:
- at most 5 words
- lowercase, unless a word is a proper noun
- name the work that was done, not the class it belongs to

LEGS:
`

// CLIRunner shells: <Bin> -p --model <Model> --output-format json <prompt>
// with its working directory set to Dir — a compass-private dir, so the
// narration session's own transcript never shows up in the fleet it describes.
type CLIRunner struct {
	Bin   string
	Model string
	Dir   string
}

// bin and model apply the defaults at the point of use: a zero CLIRunner works,
// and a caller that reads the struct back sees what it actually set.
func (r *CLIRunner) bin() string {
	if r.Bin != "" {
		return r.Bin
	}
	return defaultBin
}

func (r *CLIRunner) model() string {
	if r.Model != "" {
		return r.Model
	}
	return defaultModel
}

// Args returns the exact argv (excluding Bin) for a batch — pure, testable.
func (r *CLIRunner) Args(digests []Digest) []string {
	return []string{"-p", "--model", r.model(), "--output-format", "json", r.prompt(digests)}
}

// prompt is the single string the CLI is handed: the instruction block, then
// the digests as JSON. One argument, so nothing depends on a shell.
func (r *CLIRunner) prompt(digests []Digest) string {
	body, err := json.Marshal(digests)
	if err != nil {
		// Digest is plain data; this cannot fail in practice, and an empty
		// array is a batch the model will correctly answer nothing about.
		body = []byte("[]")
	}
	return promptInstruction + string(body)
}

// Narrate runs one batch through the CLI and parses what comes back. An empty
// batch never starts a process.
func (r *CLIRunner) Narrate(digests []Digest) ([]Label, error) {
	if len(digests) == 0 {
		return nil, nil
	}
	if r.Dir != "" {
		// First use creates the private dir; later batches find it there.
		if err := os.MkdirAll(r.Dir, 0o755); err != nil {
			return nil, fmt.Errorf("narrator: dir %s: %w", r.Dir, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), narrateTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.bin(), r.Args(digests)...)
	cmd.Dir = r.Dir
	var stderr strings.Builder
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("narrator: %s timed out after %s", r.bin(), narrateTimeout)
		}
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("narrator: %s: %w: %s", r.bin(), err, firstLine(msg))
		}
		return nil, fmt.Errorf("narrator: %s: %w", r.bin(), err)
	}
	return ParseResponse(out)
}

// envelope is the shape `--output-format json` prints: bookkeeping around the
// model's text, which is the only field worth reading here.
type envelope struct {
	Result string `json:"result"`
}

// rawLabel is one element of the array the model was asked for.
type rawLabel struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// ParseResponse extracts labels from the CLI's JSON envelope. Anything it
// cannot believe — a broken envelope, a result with no array in it, an array
// that will not parse — is an error that applies nothing at all: half-narrated
// is worse than un-narrated.
func ParseResponse(out []byte) ([]Label, error) {
	var env envelope
	if err := json.Unmarshal(out, &env); err != nil {
		return nil, fmt.Errorf("narrator: response envelope: %w", err)
	}
	if strings.TrimSpace(env.Result) == "" {
		return nil, errors.New("narrator: response has no result")
	}

	body, err := jsonArray(env.Result)
	if err != nil {
		return nil, err
	}
	var raws []rawLabel
	if err := json.Unmarshal([]byte(body), &raws); err != nil {
		return nil, fmt.Errorf("narrator: label array: %w", err)
	}

	labels := make([]Label, 0, len(raws))
	for _, raw := range raws {
		key := strings.TrimSpace(raw.Key)
		text := clip(strings.TrimSpace(raw.Label), labelText)
		if key == "" || text == "" {
			continue // an unnamed leg is simply one the model declined
		}
		labels = append(labels, Label{Key: key, Text: text})
	}
	return labels, nil
}

// jsonArray digs the array out of the model's text: fences come off, and then
// the first '[' through the last ']' is the widest thing that can be one.
func jsonArray(text string) (string, error) {
	text = strings.TrimSpace(stripFences(text))
	start := strings.IndexByte(text, '[')
	end := strings.LastIndexByte(text, ']')
	if start < 0 || end < start {
		return "", errors.New("narrator: result contains no JSON array")
	}
	return text[start : end+1], nil
}

// stripFences removes a surrounding ``` or ```json block, which the model adds
// however firmly it is told not to.
func stripFences(text string) string {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "```") {
		return text
	}
	trimmed = strings.TrimPrefix(trimmed, "```")
	if i := strings.IndexByte(trimmed, '\n'); i >= 0 {
		// Drop the rest of the opening line — the language tag, if any.
		trimmed = trimmed[i+1:]
	}
	if i := strings.LastIndex(trimmed, "```"); i >= 0 {
		trimmed = trimmed[:i]
	}
	return trimmed
}

// firstLine is the one line of a tool's complaint worth repeating.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
