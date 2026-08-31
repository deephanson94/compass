package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/deephanson94/compass/internal/fleet"
)

// The mirror is glass, not a terminal (decision log #9): compass paints the
// pane's own screen and never takes a key for it.
const (
	// readerEnd is any offset past the end of a document: RenderReader clamps
	// a scroll to the last screenful, so this asks for the newest lines
	// without having to measure the document first.
	readerEnd = 1 << 30

	mirrorMark   = "⌁"
	ansiReset    = "\x1b[0m"
	mirrorChrome = 2 // header, then one line of air
)

// mirrorColumn is the deck's middle panel: the selected session's pane as it
// renders right now, or — when no pane is mapped — what the transcript last
// said. The body is bottom-aligned: a terminal's action is at the bottom, and
// that is where the eye should already be.
func (m *Model) mirrorColumn(w, h int) []string {
	// The header names the source, so the panel never lies about what it shows —
	// and a frame is only ever drawn while the pane it came from is still there.
	pane, live := m.selectedPane()
	header := mirrorMark + " no pane · from transcript"
	if live {
		header = mirrorMark + " " + pane.Target + " · live"
	}

	rows := []string{dimStyle.Render(clip(header, w)), ""}
	body := h - mirrorChrome
	if body < 1 {
		return rows
	}
	if !live || strings.TrimSpace(m.mirror) == "" {
		return append(rows, bottom(m.transcriptBody(w, body), body)...)
	}
	return append(rows, bottom(frameLines(m.mirror, w, body), body)...)
}

// frameLines crops a captured screen to the column. capture-pane returns the
// pane's colours as raw escapes, so every line is cut with an ANSI-aware
// truncate and closed with a reset: a colour that escaped the column edge would
// repaint the trail beside it.
func frameLines(frame string, w, h int) []string {
	raw := strings.Split(strings.ReplaceAll(frame, "\r\n", "\n"), "\n")
	for len(raw) > 0 && strings.TrimSpace(raw[len(raw)-1]) == "" {
		raw = raw[:len(raw)-1] // a captured pane is mostly blank below the cursor
	}
	if len(raw) > h {
		raw = raw[len(raw)-h:] // the newest output wins the space
	}
	out := make([]string, len(raw))
	for i, line := range raw {
		line = strings.TrimRight(ansi.Truncate(line, w, ""), " ")
		if strings.Contains(line, "\x1b") && !strings.HasSuffix(line, ansiReset) {
			line += ansiReset // a cut can swallow the pane's own reset
		}
		out[i] = line
	}
	return out
}

// transcriptBody is what a session with no tmux pane shows in the mirror's
// place: its own conversation, newest last.
//
// It used to be three dim facts — title, state, activity — which the fleet
// column already carries. On a tall terminal that is three lines resting on
// the floor of a forty-row panel, and it reads as an empty screen, because
// next to nothing is what it was. A paneless session is not a session with
// nothing to show: compass is reading its transcript either way, and the
// transcript is the same thing the pane would have been rendering.
func (m *Model) transcriptBody(w, h int) []string {
	if _, ok := m.selected(); !ok {
		return []string{dimStyle.Render(clip("no session selected", w))}
	}
	if len(m.events) == 0 {
		return m.transcriptFacts(w)
	}
	// Anchored to the end: the newest turn is the one a mirror would be
	// showing. A scroll past the document's end clamps to the last screenful,
	// which is exactly what is wanted here.
	frame := RenderReader(m.events, ReaderOpts{
		Width: w, Height: h, Scroll: readerEnd,
		Unfolded: m.unfolded,
	})
	if strings.TrimSpace(frame) == "" {
		return m.transcriptFacts(w)
	}
	return strings.Split(frame, "\n")
}

// transcriptFacts is the last resort: a session whose transcript has not been
// read yet still says who it is.
func (m *Model) transcriptFacts(w int) []string {
	s, _ := m.selected()
	var rows []string
	if s.Info.Title != "" {
		rows = append(rows, dimStyle.Render(clip(`"`+s.Info.Title+`"`, w)), "")
	}
	rows = append(rows, dimStyle.Render(clip(verdict(s), w)))
	if s.Snap.Activity != "" {
		rows = append(rows, dimStyle.Render(clip(s.Snap.Activity, w)))
	}
	return rows
}

// verdict is the state and why, in one line.
func verdict(s fleet.Session) string {
	line := fleet.Glyph(s.Snap.State) + " " + stateLabel(s.Snap.State)
	if s.Snap.Reason != "" {
		line += " · " + s.Snap.Reason
	}
	return line
}

// bottom pads a block to exactly h lines with the content resting on the floor.
func bottom(lines []string, h int) []string {
	if len(lines) > h {
		return lines[len(lines)-h:]
	}
	out := make([]string, 0, h)
	for i := len(lines); i < h; i++ {
		out = append(out, "")
	}
	return append(out, lines...)
}
