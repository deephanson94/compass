package transcript

import "strings"

// A transcript's user lines are not all people talking. The harness writes its
// own turns through the same channel — notifications, reminders, hook feedback,
// the caveat before a local command's output, the output itself — and a reader
// that mistakes one for a prompt reports "<local-command-caveat>Caveat…" as
// what you asked for.
//
// Two signals separate them. isMeta, which Claude Code sets on some of its own
// bookkeeping turns; and the opening character. In a 5,400-line transcript,
// every user turn that opened with a tag was the harness's — task
// notifications, reminders, command wrappers, captured output — and not one
// human prompt opened with one. So a turn that opens with a tag is machinery,
// with one exception: a slash command expands to tags too, and that is the
// person talking. This replaced a list of tag names that grew by one every
// time a dogfood found the next wrapper.
//
// There is a third field that looks like it belongs here and does not.
// `origin` reads {"kind":"human"} in one Claude Code version and the bare
// string "cli" in another, so a reader that trusts its shape either rejects
// every prompt from the older one or fails to parse the line at all. Both
// versions are in this repo's own fixtures. Everything it would have caught,
// isMeta and the tag rule already do.

// relayPrefixes open the harness's turns that carry neither a flag nor a tag:
// a teammate's message relayed, a background agent's instructions echoed back.
// Matched at the start of the text only: a prompt that mentions one of these
// is still a prompt.
var relayPrefixes = []string{
	"Another Claude session sent a message",
	`Background agent "`,
}

// compactionPreamble opens the turn that carries a summary of a conversation
// that ran out of context. It is machinery wearing a prompt's clothes: no tag,
// no flag, and eight thousand words of it.
const compactionPreamble = "This session is being continued from a previous conversation"

// Machinery reports whether a user turn is the harness talking to Claude
// rather than a person talking to either.
func (e Event) Machinery() bool {
	if e.Type != EventUser {
		return false
	}
	if e.IsMeta {
		return true
	}
	return EnvelopeText(e.Text)
}

// EnvelopeText recognises an automated turn by what it opens with, for callers
// holding text rather than an event.
func EnvelopeText(text string) bool {
	t := strings.TrimSpace(text)
	if _, ok := SlashCommand(t); ok {
		return false // the one tagged turn a person wrote
	}
	if opensWithTag(t) {
		return true
	}
	if strings.HasPrefix(t, compactionPreamble) {
		return true
	}
	for _, p := range relayPrefixes {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}

// opensWithTag reports whether text begins "<name" for some letter-led name —
// the shape of every envelope the harness writes, and of no prompt a person
// has been seen to type.
func opensWithTag(t string) bool {
	if len(t) < 2 || t[0] != '<' {
		return false
	}
	c := t[1]
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// SlashCommand renders a slash-command turn the way the person typed it.
// Claude Code expands one into three tags, and the middle one — the bare word
// the fleet used to show — is the least useful of them:
//
//	<command-name>/model</command-name>
//	<command-message>model</command-message>
//	<command-args>claude-opus-5</command-args>
//
// It reports false for anything that is not one, including a prompt that
// happens to mention the tags.
func SlashCommand(text string) (string, bool) {
	name, ok := tagBody(text, "command-name")
	if !ok || name == "" {
		return "", false
	}
	if args, ok := tagBody(text, "command-args"); ok && args != "" {
		return name + " " + args, true
	}
	return name, true
}

// tagBody returns the contents of the first <tag>…</tag> in text.
func tagBody(text, tag string) (string, bool) {
	open, close := "<"+tag+">", "</"+tag+">"
	i := strings.Index(text, open)
	if i < 0 {
		return "", false
	}
	rest := text[i+len(open):]
	j := strings.Index(rest, close)
	if j < 0 {
		return "", false
	}
	return strings.TrimSpace(rest[:j]), true
}

// TaskNotification is what a background agent leaves behind when it stops.
// It does not come back as a tool_result — the tool_result was the launch
// acknowledgement, minutes earlier — but as a user turn wrapped in tags, and
// the tool-use-id inside is what ties it to the Agent call that started it.
type TaskNotification struct {
	TaskID    string
	ToolUseID string
	Status    string // "completed", "failed", …; "" when the envelope had none
	Summary   string // one line, e.g. `Agent "Implement tmuxop pane layer" finished`
	Result    string // the agent's own final words; may be long, may be empty
}

// ParseTaskNotification reads a task-notification turn. It reports false for
// anything that is not one, so a caller can hand it every user turn.
func ParseTaskNotification(text string) (TaskNotification, bool) {
	t := strings.TrimSpace(text)
	if !strings.HasPrefix(t, "<task-notification") {
		return TaskNotification{}, false
	}
	var n TaskNotification
	n.TaskID, _ = tagBody(t, "task-id")
	n.ToolUseID, _ = tagBody(t, "tool-use-id")
	n.Status, _ = tagBody(t, "status")
	n.Summary, _ = tagBody(t, "summary")
	n.Result, _ = tagBody(t, "result")
	return n, true
}
