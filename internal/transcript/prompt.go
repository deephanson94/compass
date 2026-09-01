package transcript

import "strings"

// A transcript's user lines are not all people talking. The harness writes its
// own turns through the same channel — notifications, reminders, hook feedback,
// the caveat before a local command's output — and a reader that mistakes one
// for a prompt reports "<local-command-caveat>Caveat…" as what you asked for.
//
// Two signals separate them: isMeta, which Claude Code sets on its own
// bookkeeping turns, and the opening tag, for the envelopes that carry no flag.
// Only the second is a pattern, and it is the last resort.
//
// There is a third field that looks like it belongs here and does not.
// `origin` reads {"kind":"human"} in one Claude Code version and the bare
// string "cli" in another, so a reader that trusts its shape either rejects
// every prompt from the older one or fails to parse the line at all. Both
// versions are in this repo's own fixtures. Everything it would have caught,
// isMeta and the tags already do.

// envelopeTags open the automated turns the harness wraps around its own
// messages. Matched at the start of the text only: a prompt that *mentions*
// one of these is still a prompt.
var envelopeTags = []string{
	"<system-reminder",
	"<task-notification",
	"<wake",
	"<local-command-caveat",
	"<teammate-message",
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
	if strings.HasPrefix(t, compactionPreamble) {
		return true
	}
	for _, tag := range envelopeTags {
		if strings.HasPrefix(t, tag) {
			return true
		}
	}
	return false
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
