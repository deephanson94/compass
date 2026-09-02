package ui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/deephanson94/compass/internal/transcript"
)

// The reader's vocabulary (SPEC §2.3). It is the CLI's own: a human turn is a
// chevron, a tool call a filled dot, what came back hangs under it on an elbow.
const (
	glyphSaid   = "❯" // the human's turn
	glyphCall   = "⏺" // a tool call
	glyphResult = "⎿" // what it returned, folded
	glyphErrRes = "✗" // …and it failed
	glyphLate   = "↩" // a result that arrived after other calls
)

const (
	resultIndent  = "  "   // the gutter a result hangs in, under its call
	bodyIndent    = "    " // an unfolded result's own lines, under the elbow
	unfoldCap     = 20     // how many result lines an unfold is allowed to spend
	readerMinBody = 24     // below this the document says nothing worth reading

	// readerMeasure is the widest a line of prose is allowed to run. A panel
	// three-quarters of a wide monitor is 150 columns, and a paragraph
	// wrapped to 150 columns is a wall: the eye loses the line on the way
	// back. Calls and results still take the whole width — a path or a
	// command is read once, not along.
	readerMeasure = 100
)

// ReaderOpts is everything the reader needs beyond the events themselves.
// Scroll is a line index into the flattened document, not an event index: the
// document is what the eye moves through.
type ReaderOpts struct {
	Width, Height int
	Scroll        int          // top line index into the flattened document
	Unfolded      map[int]bool // event indices whose tool output is unfolded
	Query         string       // current search ("" = none); matches highlighted

	// Anchor is the document line the trail's cursor stands on, marked the same
	// way the trail marks that cursor: one inverse bar in each panel, saying
	// literally that this row and this line are the same moment. -1 for none.
	//
	// Scrolling the anchor to the top would say it on its own, except that near
	// the end of a document the scroll clamps and the anchor lands mid-panel
	// with nothing naming it — which is exactly where a reader is looking when
	// they walk the newest legs.
	Anchor int

	// Now is the moment the document is read at: an agent still out says
	// how long, the same clock its lane on the trail carries. Zero leaves
	// the stub bare.
	Now time.Time

	// CWD is the session's working directory: a tool call's path is shown
	// relative to it, the way the CLI shows it, so a column of
	// `Edit(/home/user/compass/internal/ui/reader.go)` reads as
	// `Edit(internal/ui/reader.go)`. An event carrying its own cwd wins.
	CWD string
}

// RenderReader renders the Lv3 conversation panel: the session's events as one
// document, oldest first — chronological, like the CLI itself — cropped to a
// width×height block. Pure: the same events and options always draw the same
// frame.
func RenderReader(events []transcript.Event, o ReaderOpts) string {
	h := o.Height
	if h < 1 {
		return ""
	}
	if o.Width < readerMinBody {
		return strings.Join(fit(nil, h), "\n")
	}

	doc := readerDoc(events, o)
	if len(doc) == 0 {
		doc = readerEmptyDoc(o.Width)
	}
	top := clampScroll(o.Scroll, len(doc), h)

	rows := make([]string, 0, h)
	for i := top; i < len(doc) && len(rows) < h; i++ {
		line := doc[i].render(o.Query)
		if i == o.Anchor {
			line = markAnchor(line, o.Width)
		}
		rows = append(rows, line)
	}
	return strings.Join(fit(rows, h), "\n")
}

// markAnchor inverts the anchored line across the panel, the way the trail
// inverts its cursor row. The line is stripped back to its plain text first: a
// reset left over from a tint would cancel the inversion halfway across.
func markAnchor(line string, w int) string {
	plain := strings.TrimRight(ansi.Strip(line), " ")
	if pad := w - lipgloss.Width(plain); pad > 0 {
		plain += strings.Repeat(" ", pad)
	}
	return cursorStyle.Render(plain)
}

// ReaderAnchor maps a moment on the trail to a line in the reader: the first
// line of the first event at or after at. It is the other half of Enter at Lv2
// (SPEC §3) — a waypoint names a time, and the reader opens there. -1 means the
// document has nothing at or after that moment.
func ReaderAnchor(events []transcript.Event, o ReaderOpts, at time.Time) int {
	doc := readerDoc(events, o)
	for i, l := range doc {
		if l.event < 0 || l.at.IsZero() {
			continue
		}
		if !l.at.Before(at) {
			// A moment that lands on a result lands on its call instead:
			// a page opened on "⎿ edited · +1 −1" named nothing.
			for i > 0 && (l.kind == readerFold || l.kind == readerFoldErr || l.kind == readerBody) {
				prev := doc[i-1]
				if prev.kind != readerCall && prev.kind != readerFold && prev.kind != readerFoldErr && prev.kind != readerBody {
					break
				}
				i, l = i-1, prev
			}
			return i
		}
	}
	return -1
}

// clampScroll holds a scroll offset inside the document: never past the last
// screenful, never above the first line.
func clampScroll(scroll, lines, height int) int {
	maxTop := lines - height
	if maxTop < 0 {
		maxTop = 0
	}
	if scroll > maxTop {
		scroll = maxTop
	}
	if scroll < 0 {
		scroll = 0
	}
	return scroll
}

// readerKind is what a document row is, which decides its one accent and
// whether Space can act on it.
type readerKind int

const (
	readerBlank   readerKind = iota
	readerSaid               // a human turn
	readerText               // the model's own words
	readerCall               // a tool call, one line
	readerFold               // a folded result — Space's target
	readerFoldErr            // …one that failed
	readerBody               // an unfolded result's lines
	readerHead               // a heading in the model's own words
	readerCode               // a fenced block in the model's own words
)

// readerLine is one row of the flattened document: plain text, the accent it
// wears, and the event it came from so folding and anchoring can find it.
type readerLine struct {
	text  string
	kind  readerKind
	event int       // index into the events slice; -1 for filler
	at    time.Time // the event's timestamp, for ReaderAnchor

	// dim is the rune offset where the row's dim tail begins — a call's
	// argument, a turn's clock — or 0 for a row of one accent. The text is
	// one string so a search still matches across the seam.
	dim int
}

// foldable reports whether Space can act on this row.
func (l readerLine) foldable() bool {
	return l.kind == readerFold || l.kind == readerFoldErr
}

// render applies the row's accent, inverting whatever the search matched.
func (l readerLine) render(query string) string {
	if r := []rune(l.text); l.dim > 0 && l.dim < len(r) {
		return highlight(string(r[:l.dim]), query, readerStyle(l.kind)) +
			highlight(string(r[l.dim:]), query, dimStyle)
	}
	return highlight(l.text, query, readerStyle(l.kind))
}

// readerStyle is the one accent a row is allowed to carry. Errors are the only
// warm thing the reader may draw — the same rule the fleet lives by (SPEC §4).
func readerStyle(k readerKind) lipgloss.Style {
	switch k {
	case readerSaid, readerHead:
		return promptStyle
	case readerText:
		return textStyle
	case readerCall:
		return textStyle
	case readerFoldErr:
		return stuckStyle
	default:
		return dimStyle
	}
}

// docBuilder assembles the document, keeping the air between blocks honest:
// one blank line between turns, none between a call and what it returned.
type docBuilder struct {
	lines []readerLine
	width int
	cwd   string
	now   time.Time
}

func (d *docBuilder) push(text string, kind readerKind, event int, at time.Time) {
	d.lines = append(d.lines, readerLine{text: text, kind: kind, event: event, at: at})
}

// pushDim is push with the row's tail dim from rune offset dim on.
func (d *docBuilder) pushDim(text string, dim int, kind readerKind, event int, at time.Time) {
	d.lines = append(d.lines, readerLine{text: text, kind: kind, event: event, at: at, dim: dim})
}

// measure is the width prose wraps to: the panel's, up to readerMeasure.
func (d *docBuilder) measure() int {
	if d.width > readerMeasure {
		return readerMeasure
	}
	return d.width
}

// gap opens one line of air, never two, never at the top.
func (d *docBuilder) gap() {
	if n := len(d.lines); n > 0 && d.lines[n-1].kind != readerBlank {
		d.lines = append(d.lines, readerLine{kind: readerBlank, event: -1})
	}
}

// last is the kind of the row the builder is standing on.
func (d *docBuilder) last() readerKind {
	if n := len(d.lines); n > 0 {
		return d.lines[n-1].kind
	}
	return readerBlank
}

// readerDoc flattens the events into rows. Sidechains are skipped: a subagent's
// own conversation is the trail's branch lane, not this document's business.
func readerDoc(events []transcript.Event, o ReaderOpts) []readerLine {
	d := &docBuilder{width: o.Width, cwd: o.CWD, now: o.Now}
	unfolded := o.Unfolded
	answered := map[string]bool{}
	for _, ev := range events {
		for _, res := range ev.ToolResults {
			answered[res.ToolUseID] = true
		}
	}
	// Where each result lands, and where each call is made, so a call whose
	// result comes after other calls can say so at the call site.
	resultAt := map[string]int{}
	var callAt []int
	for i, ev := range events {
		if ev.IsSidechain {
			continue
		}
		for _, res := range ev.ToolResults {
			if _, seen := resultAt[res.ToolUseID]; !seen {
				resultAt[res.ToolUseID] = i
			}
		}
		if len(ev.ToolUses) > 0 {
			callAt = append(callAt, i)
		}
	}
	nextCall := func(after int) int {
		for _, c := range callAt {
			if c > after {
				return c
			}
		}
		return -1
	}
	calls := map[string]transcript.ToolUse{}
	lastCall := ""
	for i, ev := range events {
		if ev.IsSidechain {
			continue
		}
		switch ev.Type {
		case transcript.EventUser:
			if text := strings.TrimSpace(ev.Text); text != "" {
				d.said(i, ev.Timestamp, text)
			}
			for _, res := range ev.ToolResults {
				if res.ToolUseID != lastCall {
					// A result that is not the last call's — a background
					// agent's, back long after other calls, or the lead's
					// own edit landing after an agent was dispatched — is
					// named at the call's own depth, in words: hung under
					// the call above it, it read as that call's second
					// answer, and under an Agent it said the agent did it.
					if use, ok := calls[res.ToolUseID]; ok {
						d.late(i, ev.Timestamp, use, ev.CWD)
					}
				}
				d.result(i, ev.Timestamp, calls[res.ToolUseID], res, unfolded[i], ev.CWD)
			}
		case transcript.EventAssistant:
			if text := strings.TrimSpace(ev.Text); text != "" {
				d.text(i, ev.Timestamp, text)
			}
			for _, use := range ev.ToolUses {
				calls[use.ID] = use
				lastCall = use.ID
				d.call(i, ev.Timestamp, use, ev.CWD)
				switch {
				case !answered[use.ID]:
					// Dispatched and not back, or hung: the reader said
					// nothing, and an agent still out looked exactly like
					// one returned and folded.
					d.pending(i, ev.Timestamp, use)
				case nextCall(i) >= 0 && resultAt[use.ID] > nextCall(i):
					// Answered, but only after other calls were made: the
					// result is drawn where it landed, under "↩ result of",
					// and the call site says so rather than looking hung.
					d.push(resultIndent+glyphResult+" "+clip(glyphLate+" result below", d.width-len(resultIndent)-2), readerBody, i, ev.Timestamp)
				}
			}
		}
	}
	return d.lines
}

// said draws a human turn: the chevron leads it, the rest hangs under it.
// The first row carries the turn's clock on the right, dim, so the turns
// read as the chapters they are and `[ ]` lands on a moment with a name.
func (d *docBuilder) said(event int, at time.Time, text string) {
	d.gap()
	rows := wrapPrefix(text, glyphSaid+" ", "  ", d.measure())
	for i, row := range rows {
		if i == 0 && !at.IsZero() {
			clock := at.Local().Format("15:04")
			if room := d.width - len([]rune(row)) - 2; room >= len(clock) {
				d.pushDim(row+strings.Repeat(" ", room-len(clock)+2)+clock, len([]rune(row))+1, readerSaid, event, at)
				continue
			}
		}
		d.push(row, readerSaid, event, at)
	}
}

// text draws the model's own words, wrapped — the one place in compass that
// wraps rather than truncates, because a document is meant to be read. The
// words are markdown, more often than not, and the reader owes them the
// little that a terminal can honour: a heading in bold, a fenced block set
// off and left alone, a bullet as a bullet, the ** around an emphasis
// dropped rather than printed.
func (d *docBuilder) text(event int, at time.Time, text string) {
	d.gap()
	for _, row := range proseRows(text, d.measure(), d.width) {
		d.push(row.text, row.kind, event, at)
	}
}

// proseRows lays the model's words out: prose wrapped to measure, fenced
// code clipped at width and indented, headings and bullets marked.
func proseRows(text string, measure, width int) []readerLine {
	var out []readerLine
	fenced := false
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			fenced = !fenced
			continue // the fence itself is punctuation nobody reads
		}
		if fenced {
			out = append(out, readerLine{text: clip(resultIndent+strings.TrimRight(strings.ReplaceAll(line, "\t", "    "), " "), width), kind: readerCode})
			continue
		}
		if trimmed == "" {
			out = append(out, readerLine{kind: readerBlank})
			continue
		}
		kind, first, cont := readerText, "", ""
		if head := strings.TrimLeft(trimmed, "#"); len(head) < len(trimmed) && strings.HasPrefix(head, " ") {
			kind, trimmed = readerHead, strings.TrimSpace(head)
		} else if indent := len(line) - len(strings.TrimLeft(line, " \t")); strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			first = strings.Repeat(" ", indent) + "• "
			cont = strings.Repeat(" ", indent+2)
			trimmed = strings.TrimSpace(trimmed[2:])
		}
		trimmed = strings.ReplaceAll(trimmed, "**", "")
		for _, row := range wrapPrefix(trimmed, first, cont, measure) {
			out = append(out, readerLine{text: row, kind: kind})
			first = cont
		}
	}
	// The air the text ends with is not the document's to keep.
	for len(out) > 0 && out[len(out)-1].kind == readerBlank {
		out = out[:len(out)-1]
	}
	return out
}

// call draws a tool call as one line — `⏺ Bash(pytest -x)` — the M0 Activity
// derivation's input summary in the parentheses.
//
// The name is the row's accent and the argument is dim: a stretch of calls
// reads as "Read, Read, Edit, Bash" at a glance, and the prose between them
// is the only thing on the page in the page's own colour. The two calls
// whose argument is the point — a question to you, an agent's assignment —
// keep it plain.
func (d *docBuilder) call(event int, at time.Time, use transcript.ToolUse, cwd string) {
	if k := d.last(); k == readerSaid || k == readerText || k == readerHead || k == readerCode {
		d.gap()
	}
	name := use.Name
	if name == "" {
		name = "tool"
	}
	head := glyphCall + " " + name
	line := head + "(" + d.argument(use, cwd) + ")"
	dim := len([]rune(head))
	switch use.Name {
	case "AskUserQuestion", "Agent", "Task":
		dim = 0
	}
	// A call line wraps rather than clips: the question on an
	// AskUserQuestion is the one line the session is waiting on, and the
	// trail beside the reader was showing more of it than the reader.
	for i, row := range wrapPrefix(line, "", "  ", d.width) {
		if i >= 3 {
			break
		}
		if i == 0 {
			d.pushDim(row, dim, readerCall, event, at)
			continue
		}
		if dim > 0 {
			d.push(row, readerBody, event, at) // the argument's overflow, dim like the argument
			continue
		}
		d.push(row, readerCall, event, at)
	}
}

// argument is what a call is about, with the session's directory taken off
// the front of a path: the CLI shows `Read(internal/ui/reader.go)`, and a
// column of absolute paths was the widest thing on the page.
func (d *docBuilder) argument(use transcript.ToolUse, cwd string) string {
	summary := toolSummary(use)
	if cwd == "" {
		cwd = d.cwd
	}
	switch use.Name {
	case "Read", "Edit", "Write", "NotebookEdit", "Glob", "Grep":
		return relPath(summary, cwd)
	}
	return summary
}

// shorten takes the session's directory out of a line of a tool's own
// wording — "The file /home/user/api/src/auth.py has been updated." — so a
// preview names the file the way the call above it did.
func (d *docBuilder) shorten(line, cwd string) string {
	if cwd == "" {
		cwd = d.cwd
	}
	if cwd = strings.TrimRight(cwd, "/"); cwd == "" {
		return line
	}
	return strings.ReplaceAll(line, cwd+"/", "")
}

// relPath strips cwd (and its trailing slash) off the front of path.
func relPath(path, cwd string) string {
	cwd = strings.TrimRight(cwd, "/")
	if cwd == "" || path == cwd {
		return path
	}
	if rest, ok := strings.CutPrefix(path, cwd+"/"); ok && rest != "" {
		return rest
	}
	return path
}

// pending draws the stub under a call nothing has answered yet.
func (d *docBuilder) pending(event int, at time.Time, use transcript.ToolUse) {
	word := "⋯ no result yet"
	switch use.Name {
	case "Agent":
		word = "⋯ still out"
		if !d.now.IsZero() && d.now.After(at) {
			// The lane on the trail says "⋯ 20m out"; three bare stubs
			// were three agents nobody could tell apart.
			word += " · " + relDuration(d.now.Sub(at))
		}
	case "AskUserQuestion":
		word = "⋯ no answer yet"
	}
	d.push(resultIndent+glyphResult+" "+clip(word, d.width-len(resultIndent)-2), readerBody, event, at)
}

// late names a result that arrived after other calls: "↩ result of
// Edit(tokens.py)" at a call's own depth, so what hangs beneath it is read
// as that call's, never as the call above's.
func (d *docBuilder) late(event int, at time.Time, use transcript.ToolUse, cwd string) {
	head := glyphLate + " result of " + use.Name
	line := head + "(" + d.argument(use, cwd) + ")"
	d.pushDim(clip(line, d.width), len([]rune(head)), readerCall, event, at)
}

// result draws what a call returned: one folded row leading with the first
// line of the output and how much follows it ("collected 20 items · +5
// lines"), or — when it failed — the first line of the failure, which is the
// only part anybody reads first. A file's contents (Read, and the listing
// tools) are counted, not quoted: their first line is the file's, not a
// message. Unfolding spends up to unfoldCap rows on the rest, and the folded
// row says only how many there are, so the first line is not read twice.
// "space unfolds" is the footer's to say, not every row's.
func (d *docBuilder) result(event int, at time.Time, use transcript.ToolUse, res transcript.ToolResult, open bool, cwd string) {
	lines := resultBody(res.Text)
	if len(lines) == 0 {
		d.push(resultIndent+glyphResult+" "+clip("no output", d.width-len(resultIndent)-2), readerBody, event, at)
		return
	}

	kind, head := readerFold, plural(len(lines), "line")
	switch {
	case !res.IsError && !open && editShape(use) != "":
		// "The file tokens.py has been updated." restated the call above
		// it, five times a screen. The shape of the change is the news.
		head = editShape(use)
	case res.IsError:
		// The first line that says something: a failed pytest opens with
		// its row of dots, and "✗ ......" said nothing about what failed.
		_, said := previewLine(lines)
		kind, head = readerFoldErr, glyphErrRes+" "+d.shorten(said, cwd)
	case open, countsOnly(use.Name):
	default:
		// The first line that says something, and how much more there is:
		// "+5 lines" when it was the first line, the whole count when it
		// was not — pytest's row of dots is not what the run said.
		_, preview := previewLine(lines)
		head = d.shorten(preview, cwd)
		if len(lines) > 1 {
			head += " · " + more(len(lines)-1)
		}
	}
	if res.IsError && len(lines) > 1 && !open {
		head += " · " + more(len(lines)-1)
	}
	d.push(resultIndent+glyphResult+" "+clip(head, d.width-len(resultIndent)-2), kind, event, at)
	if !open {
		return
	}
	for i, line := range lines {
		if i == unfoldCap {
			d.push(bodyIndent+clip("… "+plural(len(lines)-unfoldCap, "more line"), d.width-len(bodyIndent)), readerBody, event, at)
			break
		}
		d.push(bodyIndent+clip(line, d.width-len(bodyIndent)), readerBody, event, at)
	}
}

// resultBody is a tool result's text as rows: tabs opened out, trailing blank
// lines dropped — a captured stdout ends in newlines nobody needs to scroll.
func resultBody(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	for i, l := range lines {
		lines[i] = strings.TrimRight(strings.ReplaceAll(l, "\t", "    "), " ")
	}
	return lines
}

// editShape is what an Edit or Write did, counted from its own input —
// "+12 −3" for an edit, "12 lines" for a write — since the tool's own
// wording says only that the file was touched.
func editShape(use transcript.ToolUse) string {
	switch use.Name {
	case "Edit":
		old, new := inputField(use.Input, "old_string"), inputField(use.Input, "new_string")
		if old == "" && new == "" {
			return ""
		}
		return fmt.Sprintf("edited · +%d −%d", lineCount(new), lineCount(old))
	case "Write":
		if content := inputField(use.Input, "content"); content != "" {
			return "written · " + plural(lineCount(content), "line")
		}
	}
	return ""
}

// lineCount is the lines in s: zero for nothing, one for a line without
// its newline.
func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(strings.TrimSuffix(s, "\n"), "\n") + 1
}

// more is the one shape a fold's remainder wears — "5 more lines" — on a
// clean result and a failed one alike; "+1 line" beside "edited · +1 −1"
// read as a diff stat.
func more(n int) string {
	if n == 1 {
		return "1 more line"
	}
	return fmt.Sprintf("%d more lines", n)
}

// countsOnly names the tools whose result is a file or a listing: the first
// line of one says nothing about how the call went.
func countsOnly(tool string) bool {
	switch tool {
	case "Read", "Glob", "Grep", "NotebookEdit":
		return true
	}
	return false
}

// previewLine is the first line with words on it, and its index — a row of
// dots or dashes is not a preview of anything. Failing that, the first line
// with anything on it.
func previewLine(lines []string) (int, string) {
	first := -1
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		if first < 0 {
			first = i
		}
		if letters(t) >= 3 {
			return i, t
		}
	}
	if first < 0 {
		return 0, ""
	}
	return first, strings.TrimSpace(lines[first])
}

// letters counts the letters in s.
func letters(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			n++
		}
	}
	return n
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// readerEmptyDoc is the designed empty state: never a blank panel (SPEC §4).
func readerEmptyDoc(width int) []readerLine {
	return []readerLine{
		{text: clip(glyphSaid+" nothing to read yet", width), kind: readerBody, event: -1},
		{kind: readerBlank, event: -1},
		{text: clip("the conversation appears here as it happens", width), kind: readerBody, event: -1},
	}
}

// toolSummary is what a call is about, in one phrase: the same field the M0
// Activity derivation reads (state.activityFor), rendered for the reader's
// `Name(summary)` one-liner rather than for the fleet's activity column.
func toolSummary(use transcript.ToolUse) string {
	switch use.Name {
	case "Bash":
		return firstLine(inputField(use.Input, "command"))
	case "Read", "Edit", "Write", "NotebookEdit":
		return inputField(use.Input, "file_path")
	case "Grep", "Glob":
		if p := inputField(use.Input, "pattern"); p != "" {
			return p
		}
		return inputField(use.Input, "path")
	case "Task", "Agent":
		return inputField(use.Input, "description")
	case "AskUserQuestion":
		// The question is the call: "AskUserQuestion()" with the question
		// folded beneath it hid the one line the session was waiting on.
		return askedQuestion(use.Input)
	case "WebFetch", "WebSearch":
		if u := inputField(use.Input, "url"); u != "" {
			return u
		}
		return inputField(use.Input, "query")
	}
	// An unknown tool still has something to say: take the first field that
	// reads like a subject rather than printing raw JSON at the user.
	for _, key := range []string{"description", "command", "file_path", "path", "pattern", "query", "prompt", "name"} {
		if v := firstLine(inputField(use.Input, key)); v != "" {
			return v
		}
	}
	return ""
}

// askedQuestion is an AskUserQuestion call's first question and its options,
// on one line: "Open port 22 …? [office CIDR / keep bastion]".
func askedQuestion(input json.RawMessage) string {
	var in struct {
		Questions []struct {
			Question string `json:"question"`
			Options  []struct {
				Label string `json:"label"`
			} `json:"options"`
		} `json:"questions"`
	}
	if err := json.Unmarshal(input, &in); err != nil || len(in.Questions) == 0 {
		return ""
	}
	q := in.Questions[0]
	text := firstLine(q.Question)
	var labels []string
	for _, o := range q.Options {
		if l := strings.TrimSpace(o.Label); l != "" {
			labels = append(labels, l)
		}
	}
	if len(labels) > 0 {
		text += " [" + strings.Join(labels, " / ") + "]"
	}
	return text
}

// inputField pulls one string field out of a raw tool input object.
func inputField(input json.RawMessage, key string) string {
	if len(input) == 0 {
		return ""
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(input, &obj); err != nil {
		return ""
	}
	raw, ok := obj[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// wrapPrefix wraps text to width, leading the first row with first and every
// continuation with cont. Words longer than the column are cut rather than
// allowed to run past it.
func wrapPrefix(text, first, cont string, width int) []string {
	var out []string
	prefix := first
	room := width - len([]rune(prefix))
	if room < 4 {
		return []string{clip(prefix+text, width)}
	}

	for _, para := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		para = strings.TrimRight(para, " \t")
		if strings.TrimSpace(para) == "" {
			out = append(out, "")
			continue
		}
		for _, row := range wrapLine(para, width-len([]rune(cont)), width-len([]rune(first)), len(out) == 0) {
			out = append(out, prefix+row)
			prefix = cont
		}
	}
	if len(out) == 0 {
		return []string{clip(first+text, width)}
	}
	return out
}

// wrapLine breaks one paragraph on word boundaries. The first row of the first
// paragraph gets firstWidth (the lead glyph costs it a column or two); every
// row after that gets width.
func wrapLine(text string, width, firstWidth int, isFirst bool) []string {
	room := width
	if isFirst {
		room = firstWidth
	}
	if room < 1 {
		room = 1
	}

	var out []string
	line := ""
	flush := func() {
		out = append(out, line)
		line = ""
		room = width
	}
	for _, word := range strings.Fields(text) {
		for len([]rune(word)) > room && len([]rune(word)) > width {
			// A word nothing can hold: cut it at the column and carry on.
			if line != "" {
				flush()
				continue
			}
			r := []rune(word)
			out = append(out, string(r[:room]))
			word = string(r[room:])
			room = width
		}
		switch {
		case line == "":
			line = word
			room -= len([]rune(word))
		case len([]rune(word))+1 <= room:
			line += " " + word
			room -= len([]rune(word)) + 1
		default:
			flush()
			line = word
			room -= len([]rune(word))
		}
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}

// highlight renders text in base, with every occurrence of query inverted. The
// match is case-insensitive and rune-wise, so a query never lands mid-rune.
func highlight(text, query string, base lipgloss.Style) string {
	if text == "" {
		return ""
	}
	q := []rune(query)
	if len(q) == 0 {
		return base.Render(text)
	}
	src := []rune(text)
	lowerSrc, lowerQ := lower(src), lower(q)

	var b strings.Builder
	i, plain := 0, 0
	for i+len(q) <= len(src) {
		if !equalAt(lowerSrc, lowerQ, i) {
			i++
			continue
		}
		b.WriteString(base.Render(string(src[plain:i])))
		b.WriteString(matchStyle.Render(string(src[i : i+len(q)])))
		i += len(q)
		plain = i
	}
	b.WriteString(base.Render(string(src[plain:])))
	return b.String()
}

func lower(r []rune) []rune {
	out := make([]rune, len(r))
	for i, c := range r {
		out[i] = unicode.ToLower(c)
	}
	return out
}

func equalAt(hay, needle []rune, at int) bool {
	for i, c := range needle {
		if hay[at+i] != c {
			return false
		}
	}
	return true
}

// readerMatches is every document row the query appears in, top-down — what
// n and N step through.
func readerMatches(doc []readerLine, query string) []int {
	if strings.TrimSpace(query) == "" {
		return nil
	}
	q := lower([]rune(query))
	var out []int
	for i, l := range doc {
		src := lower([]rune(l.text))
		for j := 0; j+len(q) <= len(src); j++ {
			if equalAt(src, q, j) {
				out = append(out, i)
				break
			}
		}
	}
	return out
}
