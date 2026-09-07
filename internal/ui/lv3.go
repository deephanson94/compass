package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
	"github.com/deephanson94/compass/internal/transcript"
)

// doc is the flattened reader document, cached between keypresses — scrolling,
// folding and searching all walk it, and a long transcript should be flattened
// once per change, not once per key.
func (m *Model) doc(width int) []readerLine {
	c := &m.docCache
	cwd := m.readerCWD()
	events := m.readerEvents()
	lanes := fmt.Sprint(m.laneClauses()) + "|" + m.laneSilenceWord()
	if c.valid && c.n == len(events) && c.w == width && c.ver == m.docVer && c.cwd == cwd && c.lane == m.readerLane && c.lanes == lanes {
		return c.lines
	}
	lines := readerDoc(events, ReaderOpts{Width: width, Unfolded: m.unfolded, CWD: cwd, Now: m.now, Lanes: m.laneClauses(), Lane: m.readerLane != "", LaneSilence: m.laneSilenceWord()})
	m.docCache = readerCache{lines: lines, valid: true, n: len(events), w: width, ver: m.docVer, cwd: cwd, lane: m.readerLane, lanes: lanes}
	return lines
}

// readerEvents is the conversation the reader renders: the lead's, or the
// agent's own when the reader was opened on a lane (#49).
func (m *Model) readerEvents() []transcript.Event {
	if m.readerLane != "" {
		if a, ok := m.agentsFor(m.selectedKey)[m.readerLane]; ok {
			return a.Events
		}
		return nil
	}
	return m.events
}

// laneSilenceWord is the open lane's silence as its row says it — "silent
// 12m" — or "" for a lane that is writing, done, or not the reader's.
func (m *Model) laneSilenceWord() string {
	if m.readerLane == "" {
		return ""
	}
	b, ok := m.laneOpen()
	if !ok || b.Done {
		return ""
	}
	a, has := m.agentsFor(m.selectedKey)[m.readerLane]
	if !has {
		return ""
	}
	if d, silent := laneSilence(a, b, m.now); silent {
		word := "silent " + state.ShortDuration(d)
		// Wherever no row beside the reader carries the lane's "→N", the
		// stub under the hung call takes the fresher reading: below the
		// deck's width there is no trail panel at all, and at 120 the
		// trail's sub-row sheds the clause for want of cells, so the
		// silence stood alone on the whole frame (#66, #68, #69).
		if !m.trailRowSaysWrote(b) {
			tr := m.trails[m.selectedKey]
			agents := m.agentsFor(m.selectedKey)
			if n, ok := m.laneLinks(tr, agents)[b.Label]; ok && n > 0 {
				if at, ok := m.laneLinkWrote(tr, agents)[b.Label]; ok && !at.IsZero() {
					word += fmt.Sprintf(" · →%d wrote %s ago", n, relAge(m.now, at))
				}
			}
		}
		return word
	}
	return ""
}

// trailRowSaysWrote says whether the trail panel beside the reader already
// carries the lane's "→N wrote …" on its sub-row — the same arithmetic
// trailBody uses (#68), so the stub adds the clause only where that row
// cannot: at 120 the trail is drawn and the clause does not fit it.
func (m *Model) trailRowSaysWrote(b journey.Branch) bool {
	if m.width < deckWideCols {
		return false // the reader owns the screen: no trail panel beside it
	}
	inner := m.width - 2*edgePad
	if inner < 10 {
		inner = m.width
	}
	_, _, trailW := m.layout(inner)
	if trailW <= 0 {
		return false
	}
	tr := m.trails[m.selectedKey]
	agents := m.agentsFor(m.selectedKey)
	n, ok := m.laneLinks(tr, agents)[b.Label]
	if !ok || n <= 0 {
		return false
	}
	at, had := m.laneLinkWrote(tr, agents)[b.Label]
	if !had || at.IsZero() {
		return false
	}
	live, known := agents[b.ToolUseID]
	g, text, clock := laneHead(live, b, known, m.now)
	if text == "" || clock == "" {
		return false
	}
	body := trailW - trailWayWidth
	if body < trailMinLabel {
		return false
	}
	long := clock + fmt.Sprintf(" · →%d wrote %s ago", n, relAge(m.now, at))
	return body-len([]rune(long))-2 >= len([]rune(g+" "+text))
}

// nameAndBracket says whether a clause is the name with only a bracket
// after it — "fix the 401 on token refresh (commit)": the leg's bracket is
// not worth a second copy of the name on the row (#64, #66).
func nameAndBracket(name, clause string) bool {
	return strings.HasPrefix(clause, name+" (") && strings.HasSuffix(clause, ")")
}

// laneOpen is the lane the reader is on, if it is one the trail still has.
func (m *Model) laneOpen() (journey.Branch, bool) {
	if m.readerLane == "" {
		return journey.Branch{}, false
	}
	for _, b := range m.trail.Branches {
		if b.ToolUseID == m.readerLane {
			return b, true
		}
	}
	return journey.Branch{}, false
}

// readerCWD is the directory the reader shortens paths against: where the
// selected session was opened, which is where its paths are rooted however
// far it has since wandered.
func (m *Model) readerCWD() string {
	s, ok := m.selected()
	if !ok {
		return ""
	}
	if s.Info.OriginCWD != "" {
		return s.Info.OriginCWD
	}
	return s.Info.CWD
}

// readerWidth is the column the reader currently owns — the same arithmetic
// deckLines does, so Space and the anchor act on the document being drawn.
func (m *Model) readerWidth() int {
	w := m.width
	if w <= 0 {
		w = 80
	}
	inner := w - 2*edgePad
	if inner < 10 {
		inner = w
	}
	// The width the reader has when it is drawn — at Lv3 — whatever level is
	// asking. Lv2 computes the anchor for a panel it does not draw, and if it
	// used its own layout the anchor would be found at the trail's width and
	// then shown at the middle's, and the reader would jump as it took focus.
	switch {
	case inner < minDeckCols:
		return inner // one column, and at Lv3 it is the reader's
	case m.boardFits() && !m.archiveView:
		companion, _ := sessionSplit(inner) // the session view, at Lv2 or Lv3
		return companion
	case m.width < deckWideCols:
		return inner // the reader alone at Lv3 (layout): the deck is too narrow for a middle panel
	case m.width >= deckWideCols && m.width < readerRoomCols:
		return inner - trailWidth - gutterWidth // the fleet's width is the reader's (layout)
	case m.width >= deckWideCols:
		fleet, trail := sidePanelWidths(inner)
		return inner - fleet - trail - 2*gutterWidth
	default:
		return inner - twoColumnFleet(inner) - gutterWidth // it stands where the trail did
	}
}

// readerColumn is the deck's Lv3 middle panel: whose conversation this is, the
// search when one is live, and the document.
func (m *Model) readerColumn(w, h int) []string {
	rows := []string{m.readerTitle(w), m.readerAbove(w)}
	events := m.readerEvents()
	if br, ok := m.laneOpen(); ok && h > 2 && len(events) == 0 {
		// An agent whose file was read and holds no turn: the reader
		// says so, with the lead's assignment as the one thing known,
		// and how long the file has been empty (#53).
		// One clock, the title's: "· 18m out" is the silence of a file
		// that has nothing in it. The assignment wears the lane's own
		// glyph, not ❯ — the person did not type it.
		empty := "◍ the agent has written nothing since it was sent"
		if a, has := m.agentsFor(m.selectedKey)[br.ToolUseID]; has {
			_, hung := laneSilence(a, br, m.now)
			switch {
			case !a.Wrote.IsZero():
				// The file has been written and holds no turn to draw:
				// the page says that, with the clock the lane's own head
				// carries — "written nothing" contradicted the head two
				// panels left, which said it wrote forty seconds ago.
				if hung {
					empty = "◍ nothing to read yet · silent " + relAge(m.now, a.Wrote)
				} else {
					empty = "⋯ nothing to read yet · wrote " + relAge(m.now, a.Wrote) + " ago"
				}
			case !hung:
				empty = "⋯ the agent has written nothing yet"
			}
		}
		rows = append(rows, dimStyle.Render(clip(empty, w)))
		// The page owns the screen: no trail panel is on the frame to say
		// the call the lane is inside, and the row the person pressed Tab
		// on said it. The clock stays above, said once (#60, #66).
		if fw, mw, _ := m.layout(m.width); fw == 0 && mw == 0 {
			if a, has := m.agentsFor(m.selectedKey)[br.ToolUseID]; has {
				if g, text, _ := laneHead(a, br, true, m.now); text != "" && !a.Wrote.IsZero() {
					rows = append(rows, dimStyle.Render(clip(g+" "+text, w)))
				}
			}
		}
		rows = append(rows, "")
		rows = append(rows, textStyle.Render(clip(glyphBranch+" "+branchName(br.Label), w)), dimStyle.Render(clip("  the assignment, from "+m.readerName(), w)))
		for len(rows) < h {
			rows = append(rows, "") // the gutter runs the panel's height, as every other page
		}
		return rows
	}
	if h > 2 && len(events) == 0 && len(m.trail.Legs) > 0 {
		// The trail is in hand and the conversation is not yet: it is being
		// read, not absent. "nothing to read yet … as it happens" claimed a
		// session with a day of legs had not started.
		rows = append(rows, dimStyle.Render(clip(glyphSaid+" reading the transcript…", w)))
		for len(rows) < h {
			rows = append(rows, "")
		}
		return rows
	}
	if h > 2 {
		frame := RenderReader(events, ReaderOpts{
			Width:       w,
			Height:      h - 2,
			Scroll:      readerTopIn(m.doc(w), m.scroll, h-2), // never a result row without its owner
			Unfolded:    m.unfolded,
			Query:       m.query,
			Anchor:      m.anchor,
			CWD:         m.readerCWD(),
			Now:         m.now,
			Lanes:       m.laneClauses(),
			Lane:        m.readerLane != "",
			LaneSilence: m.laneSilenceWord(),
		})
		page := strings.Split(frame, "\n")
		rows = append(rows, page...)
		if len(rows) > 0 && m.anchorText != "" {
			// The title's copy of the anchored row goes where the page
			// draws that row and holds no other turn to tell it from:
			// "READER · hello    add a --version flag · 17:59" stood two
			// rows over "❯ add a --version flag    17:59" (#65).
			turns, said := 0, ""
			for _, l := range page {
				if r := ansi.Strip(l); strings.HasPrefix(r, glyphSaid+" ") {
					turns++
					said = oneSpace(strings.TrimRight(r, " "))
				}
			}
			if turns == 1 && saysSame(oneSpace(m.anchorText), said) {
				rows[0] = m.readerTitleWith(w, false)
			}
			if s, ok := m.selected(); ok && m.archiveView && !s.Live {
				// The archive's title names the session alone where the
				// turn row draws the ask, as the trail's does (#105) — at
				// eighty and a hundred, where the reader has the screen
				// and no trail title stands to be repeated (#120).
				drawn := false
				for _, l := range page {
					if r := ansi.Strip(l); strings.HasPrefix(r, glyphSaid+" ") && saysSame(oneSpace(archiveHeadline(s)), oneSpace(strings.TrimRight(r, " "))) {
						drawn = true
						break
					}
				}
				if fw, mw, _ := m.layout(m.width); drawn && fw == 0 && mw == 0 {
					rows[0] = m.readerTitleBare(w)
				} else if drawn {
					// Wider the title keeps the ask, but its clock is the
					// cursor's moment, not the ask's: "15:31" over a turn
					// row that times those same words "15:00". The clock
					// goes with the clause it does not time (#122, #115).
					rows[0] = m.readerTitleWith(w, false)
				}
			}
		}
	}
	if fw, mw, _ := m.layout(m.width); fw == 0 && mw == 0 && len(m.trail.Legs) == 0 && m.readerLane == "" {
		// The reader owns the screen: no trail panel and no fleet row is
		// on it to say the session is alive, and "⎿ ⋯ no reply yet" over
		// twenty blank rows was an abandoned session's screen too. The
		// tail carries the present the trail draws (§4; the shape of
		// #62, #66).
		if row := bareHeadRow(m.trailOpts(w, h), w); row != "" {
			last := -1
			for i, r := range rows {
				if lipgloss.Width(strings.TrimSpace(r)) > 0 {
					last = i
				}
			}
			if last >= 0 && last+2 < len(rows) {
				rows[last+2] = row
			}
		}
	}
	return rows
}

// readerTop is the first row of the page: the scroll, clamped — and never a
// result row whose owner is the row above. A tail page that opened on a
// bare "⎿ 20 passed" had its "↩ result of Bash(…)" as the last line above,
// and the first thing on the page was a test result with no owner.
func (m *Model) readerTop(doc []readerLine) int {
	return readerTopIn(doc, m.scroll, m.readerHeight())
}

// readerTopIn is the first row of a page h rows tall at this scroll: never
// a transcript blank, and never a result whose owner is the row above —
// except on the tail page, whose last row is the present and outranks its
// first. There the blank is stepped over forwards and the owner is named
// in the row above the page (readerAbove).
func readerTopIn(doc []readerLine, scroll, h int) int {
	top := clampScroll(scroll, len(doc), h)
	if top >= len(doc)-h {
		for top < len(doc)-1 && doc[top].kind == readerBlank {
			top++
		}
		return top
	}
	for top > 0 && isResultRow(doc[top]) && doc[top-1].kind == readerCall {
		top--
	}
	for top > 0 && doc[top].kind == readerBlank {
		top-- // a landing, like a step, opens on a line and not on air
	}
	return top
}

// isResultRow says whether a row is a call's "⎿" result.
func isResultRow(l readerLine) bool {
	return strings.HasPrefix(strings.TrimLeft(l.text, " "), glyphResult)
}

// readerAbove is the row under the reader's title: what the page is not
// showing above its first line, so a conversation opened on its tail says
// it is a tail — "↑ 212 lines above · 3 turns" — and air when nothing is.
func (m *Model) readerAbove(w int) string {
	if len(m.readerEvents()) == 0 {
		if m.readerLane != "" {
			return dimStyle.Render(clip(" the agent's own conversation", w))
		}
		return ""
	}
	doc := m.doc(w)
	top := readerTopIn(doc, m.scroll, m.readerHeight())
	if top == 0 {
		// The row that says where you are says it here too: the answer is
		// "the beginning", and a blank said nothing — at the widths where
		// a conversation fits whole, least of all.
		if m.readerLane != "" {
			return dimStyle.Render(clip(" the start of the agent's own conversation", w))
		}
		return dimStyle.Render(clip(" the start of the conversation", w))
	}
	turns := 0
	for i := 0; i < top; i++ {
		if doc[i].kind == readerSaid && (i == 0 || doc[i-1].kind != readerSaid) {
			turns++
		}
	}
	text := "↑ " + plural(top, "line") + " above"
	if m.readerLane != "" {
		// Not turns of yours: the person has no turns in an agent's
		// conversation. The first turn is the lead's assignment.
		text += " · the agent's own conversation"
	} else if turns > 0 {
		text += " · " + plural(turns, "turn") + " of yours"
	}
	if isResultRow(doc[top]) && doc[top-1].kind == readerCall {
		// The page opens on a result whose owner is the last line above:
		// the owner is named here, so "⎿ 20 passed" has one. It is the
		// one clause the row cannot shed — a result with no owner is what
		// the row is for — so a tight row clips it and drops the count.
		owner := " · " + strings.TrimSpace(doc[top-1].text)
		if lipgloss.Width(" "+text+owner) > w {
			text = "↑ " + plural(top, "line") + " above"
		}
		return dimStyle.Render(clip(" "+text+owner, w))
	}
	return dimStyle.Render(shedClauses(" "+text, w))
}

// readerTitle mirrors the trail's: READER · <name>, with the search state —
// the query being typed, or the one in force — on the right.
func (m *Model) readerTitle(w int) string { return m.readerTitleWith(w, true) }

// readerTitleWith draws the title with or without the anchored row's own
// words: where the page below draws that row and no other turn to tell it
// from, the clause is a second copy of a line two rows under it and the
// title keeps its clock alone (#65's record, the shape of #100 and #105).
func (m *Model) readerTitleWith(w int, anchorClause bool) string {
	return m.readerTitleAs(w, anchorClause, false)
}

// readerTitleBare names an archived session as the trail's title does —
// the session and the day it added up — where the page below draws the ask
// on its turn row (#120, #59 narrowed as #105 narrowed it).
func (m *Model) readerTitleBare(w int) string {
	return m.readerTitleAs(w, false, true)
}

func (m *Model) readerTitleAs(w int, anchorClause, bare bool) string {
	name := "—"
	if s, ok := m.selected(); ok {
		name = sessionName(s.Info)
		if m.archiveView && !s.Live && !bare {
			name = archiveHeadline(s) // as the trail beside it and the header above name it (#59)
		} else if m.archiveView && !s.Live {
			day := trailDay(m.trail, m.now, false)
			reserve := 0
			if m.level >= levelReader && (m.sessionView() || m.boardFits()) {
				reserve = lipgloss.Width("[reader]") + 2
			}
			if lipgloss.Width("READER · "+name+day) > w-1-reserve {
				day = trailDay(m.trail, m.now, true)
			}
			name += day
		}
	}
	right := ""
	if br, ok := m.laneOpen(); ok && !m.searching && m.query == "" {
		// The lane as drawn, led by its glyph so the title cannot be read
		// as the lead's own conversation, clocked by the lane (#49).
		glyph, clock := glyphBranch, relAge(m.now, br.Start)+" out"
		switch {
		case br.Done:
			// Back: the lane's own mark and when, as the trail draws it.
			back := br.End
			if back.IsZero() {
				back = br.Start
			}
			glyph, clock = branchDone, relAge(m.now, back)+" ago"
			if strings.TrimSpace(br.Report) == "" {
				glyph = branchEmpty
			}
			right = glyph + " " + branchName(br.Label) + " · " + clock
		default:
			if a, ok := m.agentsFor(m.selectedKey)[br.ToolUseID]; ok {
				if _, hung := laneSilence(a, br, m.now); hung {
					glyph = fleet.Glyph(state.Stuck)
				}
			}
			right = glyph + " " + branchName(br.Label) + " · " + clock
		}
	}
	switch {
	case right != "":
	case m.searching && !m.searchFleet:
		right = "/" + m.draft + "▏" // the fleet's query is the header's to echo, not the reader's
	case m.query != "":
		right = "/" + m.query
	case m.anchor >= 0 && !m.anchorAt.IsZero():
		// Where the reader is: the row it was anchored to and its moment,
		// so a reader scrolled to an hour can tell it is the hour. The row
		// gets whatever the name leaves, not half the panel. Where the
		// page draws the anchored turn beneath (#110) the clock goes with
		// the clause: the turn's row carries its own, and the note a
		// third — the title is the panel and the name, as the trail's is
		// over its subject (#115).
		if !anchorClause {
			break
		}
		right = m.anchorAt.Local().Format("15:04")
		if m.anchorText != "" {
			room := w - 1 - len([]rune("READER · "+name)) - 3 - len([]rune(right)) - 3
			if note := clipQuestion(m.anchorText, room); room >= 8 && !strings.HasPrefix(name, strings.TrimSuffix(note, "…")) && !nameAndBracket(name, note) {
				// The bracket clause whole or gone; clip marks the cut
				// with …. And the clause goes when what survives the cut
				// is the name already on the row: "fix the 401 on token
				// refresh (commit)" cut before its bracket said the
				// title's name twice and the leg never (#64).
				right = note + " · " + right
			}
		}
	}
	tag := ""
	if m.level >= levelReader && (m.sessionView() || m.boardFits()) {
		// The keys are here, and the card across the gutter has stopped
		// saying so: the word goes where the bar is — in the archive as
		// in the live view, since the trail's title gave it up on every
		// deck with a board (#20, #63).
		tag = "[reader]"
	}
	// The name is never clipped for the row: "▌READE… the question…" lost
	// which of two sessions was open. The anchored row gets what the name
	// and the tag leave; the tag goes before the row does.
	body := w - 1
	title := "READER · " + name
	room := body - lipgloss.Width(title) - 3 // three of air: "porter Another Claude…" read as one name
	if tag != "" {
		room -= lipgloss.Width(tag) + 2
	}
	if br, ok := m.laneOpen(); ok && lipgloss.Width(right) > room && !m.searching && m.query == "" {
		// The lane's own title, clipped at the label: the glyph and the
		// clock are what tell it from the lead's.
		glyph, clock := right[:strings.Index(right, " ")], right[strings.LastIndex(right, " · "):]
		if keep := room - lipgloss.Width(glyph) - 1 - lipgloss.Width(clock); keep >= 8 {
			right = glyph + " " + clip(branchName(br.Label), keep) + clock
		} else {
			right = clip(right, max(room, 0))
		}
	} else if lipgloss.Width(right) > room {
		if m.anchor >= 0 && !m.searching && m.query == "" {
			// The row's text, clipped to fit; the clock stays.
			clock := m.anchorAt.Local().Format("15:04")
			if room >= len(clock)+3+8 {
				// The card's rule: a question's bracket clause goes whole
				// or not at all, then the question is truncated — "[office
				// CIDR / keep bas…" read as options the menu does not have.
				right = clipQuestion(m.anchorText, room-len(clock)-3) + " · " + clock
			} else if room >= len(clock) {
				right = clock
			} else {
				right = ""
			}
		} else {
			right = clip(right, max(room, 0))
		}
	}
	if tag != "" {
		if right != "" {
			right += "  "
		}
		right += tag
	}
	mark := m.titleMark(panelReader)
	left := m.titleStyleFor(panelReader).Render(clip(title, body-lipgloss.Width(right)-1))
	gap := body - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return mark + left + strings.Repeat(" ", gap) + dimStyle.Render(right)
}

// clipQuestion truncates a question to w cells, its bracket clause of
// options whole or gone: cut inside, the brackets named options that were
// not there.
func clipQuestion(text string, w int) string {
	if i := strings.Index(text, " ["); i > 0 && lipgloss.Width(text) > w {
		text = strings.TrimSpace(text[:i])
	}
	return clip(text, w)
}

// enterReader is Tab on a Lv2 row: the reader is already open on that moment —
// the middle panel has been following the cursor since Lv2 — so all this does
// is hand it the keys.
func (m *Model) enterReader() {
	lane := m.laneWanted() // read where the cursor is before the level moves
	m.level = levelReader
	m.readerLane = ""
	if lane != "" {
		if _, ok := m.agentsFor(m.selectedKey)[lane]; ok {
			// The cursor is on a lane whose own file was read: the reader
			// shows the agent's conversation in place of the lead's, and
			// opens on its newest line — what it is doing now (#49).
			m.readerLane = lane
			m.anchor, m.anchorAt, m.anchorText = -1, time.Time{}, ""
			m.scroll = 0
			m.scrollBy(1 << 30)
			return
		}
	}
	m.anchorReader()
}

// anchorReader points the reader at the row the Lv2 cursor stands on: the
// middle panel is that moment of the conversation, and it re-anchors on every
// cursor move (M7 contract). A row the document does not reach yet leaves the
// reader where it is rather than jumping it somewhere arbitrary.
func (m *Model) anchorReader() {
	m.anchor = -1
	if m.cursor < 0 {
		return
	}
	rows := TrailRows(m.trail, m.level)
	if m.cursor >= len(rows) {
		return
	}
	opts := ReaderOpts{Width: m.readerWidth(), Unfolded: m.unfolded, CWD: m.readerCWD(), Now: m.now, Lanes: m.laneClauses()}
	if line := ReaderAnchor(m.events, opts, rows[m.cursor].Time); line >= 0 {
		// The scroll is clamped to the last screenful at once: an offset
		// past it drew the same frame, and the first j after it moved the
		// number and nothing else.
		doc := m.doc(opts.Width)
		m.scroll, m.anchor, m.anchorAt = clampScroll(line, len(doc), m.readerHeight()), line, rows[m.cursor].Time
		m.anchorText = rows[m.cursor].Text
		if row := rows[m.cursor]; row.Kind == "leg" && row.Leg >= 0 && row.Leg < len(m.trail.Legs) {
			// The row as drawn — the commit a ship leg is named by, the
			// plan's name for HEAD — not the heuristic label beneath it.
			w, h := m.trailBox()
			m.anchorText, _ = legLabel(m.trail.Legs[row.Leg], m.trailOpts(w, h))
			if o := m.trailOpts(w, h); m.trail.Legs[row.Leg].Current && o.HeadState == state.NeedsYou && o.Head != "" {
				// HEAD's row carries the first clause of the question and
				// spells the rest beneath it; the title gets the whole
				// question and clips it with a mark, not the row's cut.
				m.anchorText = o.Head
			}
		}
	}
}

// scrollBy moves the reader, clamped to the document. It reports whether
// the viewport moved at all, so a key at either end can say so.
func (m *Model) scrollBy(delta int) bool {
	doc := m.doc(m.readerWidth())
	if len(doc) <= m.readerHeight() {
		m.note = "all of it is on screen"
		return true // said; nothing more to say
	}
	was := m.readerTop(doc)
	m.scroll = clampScroll(was+delta, len(doc), m.readerHeight())
	// A single step never lands the top of the page on a line of air: the
	// press that only pushed a blank in and a line out revealed nothing.
	if (delta == 1 || delta == -1) && m.scroll < len(doc) && doc[m.scroll].kind == readerBlank {
		m.scroll = clampScroll(m.scroll+delta, len(doc), m.readerHeight())
	}
	// Nor does a step down stall on a call whose result is the next row:
	// the page kept its top on the call and `j` moved nothing.
	for delta > 0 && m.readerTop(doc) == was && m.scroll < len(doc)-m.readerHeight() {
		m.scroll++
	}
	// And what the step past that block lands on is a line, not the air
	// after it: a page whose first row is blank opened on nothing.
	for (delta == 1 || delta == -1) && m.scroll > 0 && m.scroll < len(doc)-m.readerHeight() && doc[m.readerTop(doc)].kind == readerBlank {
		m.scroll += delta
	}
	return m.readerTop(doc) != was
}

// readerChapter is `[` / `]` in the reader: the previous or next turn of
// yours — the ❯ rows — which are the conversation's chapters as the
// prompts are the trail's.
func (m *Model) readerChapter(key string) {
	doc := m.doc(m.readerWidth())
	var turns []int
	for i, l := range doc {
		if l.kind == readerSaid && (i == 0 || doc[i-1].kind != readerSaid) {
			turns = append(turns, i)
		}
	}
	if len(turns) == 0 {
		m.note = "no turns of yours in this conversation"
		return
	}
	// Where the reader is: the turn it is standing on, if `[ ]` put it
	// there; otherwise the line it is anchored to — HEAD's moment, or the
	// row the trail cursor chose — or the top of the page. `[` from a
	// fresh page lands on the turn governing that line: on a short
	// conversation that fits the panel, "no earlier turn" with a turn in
	// plain sight was false on its face.
	cur := -1
	for i, t := range turns {
		if t == m.anchor {
			cur = i
		}
	}
	at := m.readerTop(doc)
	if m.anchor > at {
		at = m.anchor
	}
	if key == "]" {
		for i, t := range turns {
			if (cur >= 0 && i > cur) || (cur < 0 && t > at) {
				m.landOnTurn(doc, turns, i)
				return
			}
		}
		m.note = "no later turn"
		return
	}
	for i := len(turns) - 1; i >= 0; i-- {
		if (cur >= 0 && i < cur) || (cur < 0 && turns[i] <= at) {
			m.landOnTurn(doc, turns, i)
			return
		}
	}
	m.note = "no earlier turn"
}

// landOnTurn scrolls the reader to the i-th of your turns and marks it: the
// inverse bar the trail's cursor uses, the title naming it, and a note
// saying which turn of how many — `[` and `]` moved the page before, and
// nothing on the page said what they had moved to.
func (m *Model) landOnTurn(doc []readerLine, turns []int, i int) {
	t := turns[i]
	m.scroll = clampScroll(t, len(doc), m.readerHeight())
	m.anchor, m.anchorAt = t, doc[t].at
	text := doc[t].text
	if doc[t].dim > 0 {
		text = string([]rune(text)[:doc[t].dim]) // without the clock
	}
	m.anchorText = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(text), glyphSaid), glyphBranch))
	m.anchorText = strings.TrimSpace(strings.TrimPrefix(m.anchorText, strings.TrimSpace(relayMark))) // the mark is the row's, not the turn's (#109)
	if t+1 < len(doc) && doc[t+1].kind == readerSaid && doc[t+1].event == doc[t].event {
		m.anchorText += "…" // the first row of a wrapped turn: the cut is marked (#57)
	}
	glyph := glyphSaid
	if m.readerLane != "" && i == 0 {
		glyph = glyphBranch // the assignment, not a turn of yours (#56)
	}
	// The count and the quote (#20), without the clock: the note stands
	// only while the turn it landed on is drawn, and that row carries its
	// clock at every width — the note's copy was never the only one, and
	// at 220 it was the clipped one over the whole (#128).
	m.note = fmt.Sprintf("%s %d/%d · %s", glyph, i+1, len(turns), `"`+m.anchorText+`"`) // the footer clips the quote to its room
}

// readerHeight is the rows the document gets: the deck body minus the
// column's title and its line of air.
func (m *Model) readerHeight() int {
	h := m.height
	if h <= 0 {
		h = 24
	}
	body := h - 5 - 2
	if body < 1 {
		body = 1
	}
	return body
}

// toggleFold is Space: the first folded result on screen opens (or the first
// open one closes) — the document's own order decides, top of the screen down.
func (m *Model) toggleFold() {
	width := m.readerWidth()
	doc := m.doc(width)
	top := m.readerTop(doc)
	for i := top; i < len(doc) && i < top+m.readerHeight(); i++ {
		if !doc[i].foldable() {
			continue
		}
		m.unfolded[doc[i].event] = !m.unfolded[doc[i].event]
		m.docVer++
		m.docCache.valid = false
		// Which one: the reader has no cursor, so Space takes the first
		// folded result on screen, and the note names the call it opened
		// — a person pressing it did not know which row it had acted on (#79).
		verb := "unfolded"
		if !m.unfolded[doc[i].event] {
			verb = "folded"
		}
		call := ""
		for j := i - 1; j >= 0 && j > i-4; j-- {
			if doc[j].kind == readerCall {
				call = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(doc[j].text), glyphCall)) // the call, not its glyph (#80)
				break
			}
		}
		if call != "" {
			m.note = verb + " " + call
		} else {
			m.note = verb + " the first result on screen"
		}
		return
	}
	m.note = "nothing to unfold on screen"
}

// jumpMatch is n/N: the next (or previous) document row the query appears in.
func (m *Model) jumpMatch(dir int) {
	if m.query == "" {
		m.note = "no search — / starts one"
		return
	}
	doc := m.doc(m.readerWidth())
	matches := readerMatches(doc, m.query)
	if len(matches) == 0 {
		m.note = "no matches"
		return
	}
	if dir > 0 {
		for _, line := range matches {
			if line > m.scroll {
				m.scroll = clampScroll(line, len(doc), m.readerHeight())
				return
			}
		}
		m.scroll = clampScroll(matches[0], len(doc), m.readerHeight()) // wrap
		return
	}
	for i := len(matches) - 1; i >= 0; i-- {
		if matches[i] < m.scroll {
			m.scroll = clampScroll(matches[i], len(doc), m.readerHeight())
			return
		}
	}
	m.scroll = clampScroll(matches[len(matches)-1], len(doc), m.readerHeight())
}

// searchKey handles a keypress while the query is being typed.
func (m *Model) searchKey(msg tea.KeyMsg) {
	switch msg.String() {
	case "enter":
		if m.searchFleet {
			// The query narrowed the fleet as it was typed; enter keeps it.
			m.fleetQuery = strings.TrimSpace(m.draft)
			m.draft, m.searching, m.searchFleet = "", false, false
			m.fleetScroll = 0
			m.clampSelection()
			if m.fleetQuery == "" {
				m.clearQuery()
			}
			return
		}
		m.query = strings.TrimSpace(m.draft)
		m.draft = ""
		m.searching = false
		if m.query != "" {
			m.scroll = 0
			m.jumpMatch(1)
		}
	case "esc":
		m.draft = ""
		if m.searchFleet {
			m.searching, m.searchFleet = false, false
			m.clearQuery()
			return
		}
		m.searching = false
	case "backspace":
		if r := []rune(m.draft); len(r) > 0 {
			m.draft = string(r[:len(r)-1])
		}
		m.narrowLive()
	default:
		if msg.Type == tea.KeyRunes {
			m.draft += string(msg.Runes)
		}
		m.narrowLive()
	}
}

// narrowLive applies the fleet search as it is typed, so the header's
// count is the feedback: six keystrokes with nothing on screen was a typo
// found only by its wrong result.
func (m *Model) narrowLive() {
	if !m.searchFleet {
		return
	}
	m.fleetQuery = strings.TrimSpace(m.draft)
	m.fleetScroll = 0
	m.clampSelection()
}

// requestNarration asks the narrator to name the trail's closed legs, at most
// once per shape of the trail — the narrator dedupes harder still. Everything
// it hands over is keyed by the session's Key(): two sessions sharing an id
// must never read each other's labels (M6 contract).
func (m *Model) requestNarration() {
	if m.narrator == nil || m.selectedKey == "" {
		return
	}
	shape := trailShape(m.selectedKey, m.trail)
	if shape == m.narrated {
		m.refreshLabels()
		return
	}
	prompt := ""
	if n := len(m.trail.Prompts); n > 0 {
		prompt = m.trail.Prompts[n-1].Text
	}
	// Remember the shape only once it is spoken for. A refusal — the one
	// batch in flight was a board column's — used to be remembered as if it
	// were an answer, and the trail being read was never asked for again.
	if m.narrator.Request(m.selectedKey, m.trail, prompt) {
		m.narrated = shape
	}
	m.refreshLabels()
}

// refreshLabels pulls whatever narration has landed for the selected trail.
func (m *Model) refreshLabels() {
	if m.narrator == nil || m.selectedKey == "" {
		return
	}
	m.labels = m.narrator.Labels(m.selectedKey, m.trail)
}

// trailShape fingerprints a trail cheaply: a new closed leg, or a session
// switch, is a new shape worth narrating.
func trailShape(key string, tr journey.Trail) string {
	closed := len(tr.Legs)
	if closed > 0 && tr.Legs[closed-1].Current {
		closed--
	}
	last := ""
	if closed > 0 {
		last = tr.Legs[closed-1].Start.String()
	}
	return key + "|" + strconv.Itoa(closed) + "|" + last
}

// readerName is the selected session's name, for the reader's sentences.
func (m *Model) readerName() string {
	if s, ok := m.selected(); ok {
		return sessionName(s.Info)
	}
	return "the lead"
}
