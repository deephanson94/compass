package transcript

import (
	"encoding/json"
	"strconv"
	"strings"
)

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
	relayPrefix,
	`Background agent "`,
}

// relayPrefix opens the turn that carries another session's message: the
// one relay that is an ask — a lead telling a worker what to do next — and
// so a prompt of a kind, worn with the word "relayed" (#97).
const relayPrefix = "Another Claude session sent a message"

// Relayed reports whether a user turn is another session's message, relayed
// by the harness: not a person's words, but the ask the session is working
// on, and the only ask a worker session driven by a lead ever gets.
func (e Event) Relayed() bool {
	return e.Type == EventUser && strings.HasPrefix(strings.TrimSpace(e.Text), relayPrefix)
}

// RelayBody is the message inside a relayed turn's envelope: what the other
// session said, without the harness's opening words.
func (e Event) RelayBody() string {
	t := strings.TrimSpace(e.Text)
	if !strings.HasPrefix(t, relayPrefix) {
		return t
	}
	return strings.TrimSpace(strings.TrimLeft(strings.TrimPrefix(t, relayPrefix), ":"))
}

// agentOpen opens the envelope another session's message is relayed in
// when it was sent by name — `SendMessage` from a session, a subagent's
// hand-back: `<agent-message from="planner">…</agent-message>`. The `from`
// is the sender as the harness names it: a session's name, or its id.
const (
	agentOpen  = "<agent-message"
	agentClose = "</agent-message>"
)

// AgentMessage reads a relayed turn's `<agent-message from="…">` envelope:
// who sent it and what it said. ok is false for a turn that is not such a
// relay — a teammate's envelope, a bare relay, a person's words.
func (e Event) AgentMessage() (from, body string, ok bool) {
	if !e.Relayed() {
		return "", "", false
	}
	t := e.RelayBody()
	if !strings.HasPrefix(t, agentOpen) {
		return "", "", false
	}
	t = t[len(agentOpen):]
	end := strings.Index(t, ">")
	if end < 0 {
		return "", "", false
	}
	from = attr(t[:end], "from")
	body = t[end+1:]
	if j := strings.Index(body, agentClose); j >= 0 {
		body = body[:j]
	}
	return from, strings.TrimSpace(body), true
}

// RelayFrom is who a relayed turn came from, as the envelope names them:
// the agent-message's `from`, else the teammate's id, else "".
func (e Event) RelayFrom() string {
	if from, _, ok := e.AgentMessage(); ok {
		return from
	}
	if id, _, ok := e.Teammate(); ok {
		return id
	}
	return ""
}

// teammateOpen opens the envelope a teammate's message is relayed in:
// `<teammate-message teammate_id="panel-theorist" color="purple">…`.
const (
	teammateOpen  = "<teammate-message"
	teammateClose = "</teammate-message>"
)

// TeammateMessage is one teammate's message out of a relayed turn: who it
// came from and what it said.
type TeammateMessage struct {
	ID string // the envelope's teammate_id; "" when the envelope had none

	// Body is the message: the teammate's own words, or, where the
	// envelope carried the harness's idle notification for it, the
	// notification's `result` — what the teammate reported when it went
	// idle (#396).
	Body string
}

// Teammates reads every teammate envelope out of a relayed turn. A lead's
// teammates come back in one user turn: the preamble once, then an
// envelope per message, blank-line separated — five on one recorded line,
// two of them from the same teammate. Nil for a relay that is not a
// teammate's, and for an envelope the harness did not relay (the tag rule
// above: that is machinery). A teammate's message is a report to the
// session that sent it out, not an ask: it is no chapter of the trail
// (#394), and the lane it was sent out on closes on it (#395).
func (e Event) Teammates() []TeammateMessage {
	if !e.Relayed() {
		return nil
	}
	t := e.RelayBody()
	if !strings.HasPrefix(t, teammateOpen) {
		return nil
	}
	var out []TeammateMessage
	for {
		i := strings.Index(t, teammateOpen)
		if i < 0 {
			break
		}
		t = t[i+len(teammateOpen):]
		end := strings.Index(t, ">")
		if end < 0 {
			break
		}
		m := TeammateMessage{ID: attr(t[:end], "teammate_id")}
		t = t[end+1:]
		body := t
		if j := strings.Index(t, teammateClose); j >= 0 {
			body = t[:j]
			t = t[j+len(teammateClose):]
		} else {
			t = ""
		}
		m.Body = teammateWords(body)
		out = append(out, m)
	}
	return out
}

// Teammate is the first teammate message of a relayed turn: the teammate
// it came from and what it said; ok is false for a relay that is not a
// teammate's. Teammates reads them all.
func (e Event) Teammate() (id, body string, ok bool) {
	ms := e.Teammates()
	if len(ms) == 0 {
		return "", "", false
	}
	return ms[0].ID, ms[0].Body, true
}

// attr reads one quoted attribute out of a tag's head, "" when absent.
func attr(head, name string) string {
	i := strings.Index(head, name+`="`)
	if i < 0 {
		return ""
	}
	rest := head[i+len(name)+2:]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

// teammateWords is what a teammate said, out of the shape it was said in.
// A teammate that finishes its turn does not write to the lead; the
// harness relays its idle notification, a JSON object whose `result` is
// the teammate's last words:
//
//	{"type":"idle_notification","from":"scenario-batch-00","timestamp":"…",
//	 "idleReason":"available","result":"Done. Processed all 21 entries…"}
//
// The recorded line carried every result's closing quote escaped —
// `…\"}` — so the object does not parse; the field is read by hand when
// it does not. Anything that is not such an object is the message as
// written.
func teammateWords(body string) string {
	body = strings.TrimSpace(body)
	if !strings.HasPrefix(body, "{") {
		return body
	}
	var n struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal([]byte(body), &n); err == nil {
		if n.Result != "" {
			return strings.TrimSpace(n.Result)
		}
		return body
	}
	const key = `"result":"`
	i := strings.Index(body, key)
	if i < 0 {
		return body
	}
	r := strings.TrimSpace(body[i+len(key):])
	r = strings.TrimSuffix(r, "}")
	r = strings.TrimSuffix(strings.TrimSpace(r), `\"`)
	r = strings.TrimSuffix(r, `"`)
	if s, err := strconv.Unquote(`"` + r + `"`); err == nil {
		r = s
	}
	return strings.TrimSpace(r)
}

// compactionPreamble opens the turn that carries a summary of a conversation
// that ran out of context. It is machinery wearing a prompt's clothes: no tag,
// no flag, and eight thousand words of it.
const compactionPreamble = "This session is being continued from a previous conversation"

// Compaction reports whether a user turn is the summary the harness writes
// when a conversation ran out of context: the moment the session's memory
// was folded, which is worth a mark on the trail — a session that has been
// compacted twice is working from a summary of a summary.
func (e Event) Compaction() bool {
	return e.Type == EventUser && strings.HasPrefix(strings.TrimSpace(e.Text), compactionPreamble)
}

// Machinery reports whether a user turn is the harness talking to Claude
// rather than a person talking to either. A relayed message is neither: it
// is another session talking, and the ask it carries opens a chapter and
// titles the session like a person's would (#97).
func (e Event) Machinery() bool {
	if e.Type != EventUser {
		return false
	}
	if e.Relayed() {
		// The harness writes a relayed message as a meta turn
		// (`isMeta: true, userType: external`), and it is still another
		// session talking: read before the flag, or every message one
		// session sends another vanished from the trail (#399).
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
