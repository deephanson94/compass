package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/deephanson94/compass/internal/fleet"
)

// Quiet cards. A board that a PR watcher fills holds ten review sessions
// that finished hours or days ago beside the three you are working in, and
// at full size they push yours off the screen. A quiet session gives up its
// card for one row in the band under the cards — number, glyph, name, how
// it came out, its pane, its age — and takes its card back the moment it
// owes you anything, or the moment you select it.
//
// Quiet is the person's call, set in the config: the tmux sessions whose
// sessions are quiet when idle (`compact_sessions = "pr"`), and how long
// any idle session waits before it is (`idle_compact = "1d"`). Neither set,
// nothing is quiet and the board is as it was.

var (
	quietTmux  map[string]bool // tmux sessions whose idle sessions are quiet
	quietAfter time.Duration   // idle this long and any session is quiet; 0 = never
)

// SetQuiet sets which sessions give up their card when idle: those in the
// named tmux sessions, and any idle for after (0 turns that off).
func SetQuiet(tmuxSessions []string, after time.Duration) {
	quietTmux = map[string]bool{}
	for _, s := range tmuxSessions {
		if s = strings.TrimSpace(s); s != "" {
			quietTmux[s] = true
		}
	}
	quietAfter = max(0, after)
}

// quiet says whether a session draws as a row instead of a card: idle on
// the card's own terms (not asking, not hung, not looping, not exited),
// neither pinned nor selected, and in a quiet tmux session or idle past
// quietAfter.
func (m *Model) quiet(s fleet.Session) bool {
	if !cardBoard || m.archiveView || (len(quietTmux) == 0 && quietAfter == 0) {
		return false
	}
	key := s.Info.Key()
	if key == m.selectedKey || m.pinned(key) || !s.Live {
		return false
	}
	if word, _, _ := m.cardState(s, m.trails[key]); word != cardIdle {
		return false
	}
	if p, ok := m.panes[key]; ok && quietTmux[tmuxSessionName(p.Target)] {
		return true
	}
	return quietAfter > 0 && !s.Info.LastEventAt.IsZero() && m.now.Sub(s.Info.LastEventAt) >= quietAfter
}

// splitQuiet takes the quiet sessions out of keys, in order, and says how
// many it took.
func (m *Model) splitQuiet(keys []string) ([]string, int) {
	if !cardBoard || m.archiveView {
		return keys, 0
	}
	out := make([]string, 0, len(keys))
	n := 0
	for _, k := range keys {
		if s, ok := m.session(k); ok && m.quiet(s) {
			n++
			continue
		}
		out = append(out, k)
	}
	return out, n
}

// quietReserve is the rows the cards leave for the quiet band: a row per
// quiet session past the strip's own row, which the band's header takes.
// None where that would leave the cards less than one band's least height:
// the strip then names them, as it names any session without a card.
func quietReserve(quiet, body int) int {
	if quiet == 0 || body-quiet < cardMinRows {
		return 0
	}
	return quiet
}

// quietCount is how many sessions on the board are quiet now.
func (m *Model) quietCount() int {
	if !cardBoard || m.archiveView {
		return 0
	}
	n := 0
	for _, i := range m.viewOrder() {
		if m.quiet(m.sessions[i]) {
			n++
		}
	}
	return n
}

// quietBand is the strip drawn as rows: under a header that keeps the
// strip's own clauses (the hidden count, the archive's door), one row per
// session without a card, in the board's order. Nil when a session without
// a card is not quiet (the pack ran out of rows: the strip names it, as
// before) or the rows are not there.
func (m *Model) quietBand(keys []string, rowOf map[string]fleetRow, w, avail int) []string {
	if !cardBoard || m.archiveView {
		return nil
	}
	shown := map[string]bool{}
	for _, k := range keys {
		shown[k] = true
	}
	var rest []fleet.Session
	for _, i := range m.viewOrder() {
		s := m.sessions[i]
		if shown[s.Info.Key()] {
			continue
		}
		if !m.quiet(s) {
			return nil
		}
		rest = append(rest, s)
	}
	if len(rest) == 0 || len(rest)+1 > avail {
		return nil
	}
	head := fmt.Sprintf("quiet · %d", len(rest))
	if n := m.hiddenCount(); n > 0 {
		head += " · " + m.hiddenClause(n)
	}
	if n := m.archivedCount(); n > 0 {
		head += fmt.Sprintf(" · %s archived%s", m.archiveDoorCount(n), m.archiveDoorKey())
	}
	out := []string{dimStyle.Render(clip(head, w))}
	for _, s := range rest {
		out = append(out, m.quietRow(s, rowOf, w))
	}
	return out
}

// quietRow is one quiet session on one row: its digit, its glyph and name,
// how it came out, its pane, and its age on the right.
func (m *Model) quietRow(s fleet.Session, rowOf map[string]fleetRow, w int) string {
	key := s.Info.Key()
	num := "  "
	if r, ok := rowOf[key]; ok && r.num > 0 {
		num = fmt.Sprintf("%d ", r.num)
	}
	left := num + m.rowGlyph(s) + " " + sessionName(s.Info)
	var clauses []string
	if _, sub, _ := m.cardState(s, m.trails[key]); sub != "" {
		clauses = append(clauses, sub)
	} else {
		clauses = append(clauses, cardIdle)
	}
	if tag := m.boardTag(s); tag != "" {
		clauses = append(clauses, tag)
	}
	age := m.age(s.Info.LastEventAt)
	room := w - lipgloss.Width(age) - 1
	line := shedClauses(left+" · "+strings.Join(clauses, " · "), room)
	pad := max(1, w-lipgloss.Width(line)-lipgloss.Width(age))
	return dimStyle.Render(line + strings.Repeat(" ", pad) + age)
}
