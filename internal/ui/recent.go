package ui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/deephanson94/compass/internal/fleet"
)

// The recent band: the rows a short fleet leaves blank belong to sessions,
// not to air (#43). Under the live rows, the newest-ended archived sessions
// are drawn with the digits the live fleet has not used — a fleet of one
// draws up to eight — so the session you walked away from an hour ago is
// one key away instead of behind `A` and a search (#47). It is the
// archive's first rows, flat and newest first, never a second archive:
// no cursor walks into it, nothing pages it, and `A` is still the door to
// the rest.

// recentRow is one row of the band: the session and the digit that opens it.
type recentRow struct {
	sess int
	num  int
}

// recentRows is the band as the deck would draw it with room for at most
// n rows: the archived sessions that ended most recently, newest first,
// numbered on from the live fleet's last digit. Nothing on the board, in
// the archive, or under a search — the band is the live list's, and a
// search names what it found.
func (m *Model) recentRows(n int) []recentRow {
	if m.archiveView || m.fleetQuery != "" || n <= 0 || (m.level == levelBoard && m.boardShown()) {
		return nil // the board's blank rows are more columns' (#43), and its strip is the door
	}
	used := 0
	for _, s := range m.sessions {
		if s.Live && m.digits[s.Info.Key()] > 0 {
			used = max(used, m.digits[s.Info.Key()])
		}
	}
	free := 9 - used
	if free <= 0 {
		return nil
	}
	var idx []int
	for i, s := range m.sessions {
		if !s.Live {
			idx = append(idx, i)
		}
	}
	sort.SliceStable(idx, func(a, b int) bool {
		return m.sessions[idx[a]].Info.LastEventAt.After(m.sessions[idx[b]].Info.LastEventAt)
	})
	if len(idx) > free {
		idx = idx[:free]
	}
	if len(idx) > n {
		idx = idx[:n]
	}
	rows := make([]recentRow, 0, len(idx))
	for i, s := range idx {
		rows = append(rows, recentRow{sess: s, num: used + i + 1})
	}
	return rows
}

// recentHeader is the band's first row: what it is, how much more the
// archive holds, and the key that browses it — the fleet's own last line
// folded into the band, so nothing is said twice.
func (m *Model) recentHeader() string {
	return fmt.Sprintf("recent · %d archived · A browses", m.archivedCount())
}

// recentLines is the band drawn into avail rows of a column w wide: the
// header, then a row per session, the oldest dropped first when the rows
// run out. Nil when fewer than two rows are free, or nothing has ended.
func (m *Model) recentLines(w, avail int) []string {
	rows := m.recentRows(avail - 1)
	if len(rows) == 0 {
		return nil
	}
	out := []string{dimStyle.Render(clip(m.recentHeader(), w))}
	for _, r := range rows {
		out = append(out, m.recentLine(r, w))
	}
	return out
}

// recentLine is one band row, in shed order: digit · ○ · name · "the
// opening prompt…" · the verdict · when it ended. The verdict — the board's
// own clause, "✓ shipped", "✗ red 18✓ 2✗" — is what makes the row more
// than a name: it answers whether to reopen this one. Narrow, it goes
// first; the identity and the clock stay.
func (m *Model) recentLine(r recentRow, w int) string {
	s := m.sessions[r.sess]
	age := m.age(s.Info.LastEventAt)
	lead := " " + strconv.Itoa(r.num) + " " + fleet.Glyph(s.Snap.State) + " "
	name := sessionName(s.Info)
	if t := strings.TrimSpace(s.Info.Title); t != "" {
		name += ` · "` + t + `"`
	}
	verdict := ""
	if tr, ok := m.trails[s.Info.Key()]; ok && len(tr.Legs) > 0 {
		// The first clause alone, and without its clock: "✓ shipped 2h
		// ago" beside "2h" said the hour twice.
		verdict = strings.SplitN(boardVerdict(s, tr, m.now), " · ", 2)[0]
		if f := strings.Fields(verdict); len(f) > 2 && f[len(f)-1] == "ago" {
			verdict = strings.Join(f[:len(f)-2], " ")
		}
	}
	room := w - lipgloss.Width(lead) - lipgloss.Width(age) - 1
	body := clip(name, room)
	// The verdict outranks the prompt's tail: "webapp · "the checkout
	// suite …  ✗ red 18✓ 2✗" answers whether to reopen, and the whole
	// prompt does not. It goes only when the name's own floor would —
	// and its counts go first, so a red row keeps "✗ red" where a green
	// one keeps its tick.
	for _, v := range []string{verdict, firstWords(verdict, 2)} {
		if keep := room - lipgloss.Width(v) - 2; v != "" && keep >= recentNameFloor {
			body = pad(clip(name, keep), keep) + "  " + v
			break
		}
	}
	body = pad(body, room)
	return dimStyle.Render(lead) + body + " " + dimStyle.Render(age)
}

// firstWords is the first n words of s.
func firstWords(s string, n int) string {
	f := strings.Fields(s)
	if len(f) <= n {
		return s
	}
	return strings.Join(f[:n], " ")
}

// recentNameFloor is the least of a band row's name and prompt kept
// beside a verdict: enough for the name and the prompt's first words.
const recentNameFloor = 18

// openRecent answers a digit the live fleet does not use: the band's row
// with that number opens the archive on that session, as `A` and a search
// would have. False when no band row wears the digit.
func (m *Model) openRecent(num int) bool {
	for _, r := range m.recentRows(9) {
		if r.num == num {
			key := m.sessions[r.sess].Info.Key()
			m.archiveView = true
			m.restSelKey, m.selectedKey = m.selectedKey, ""
			m.restLevel = m.level // the way back lands where the digit was pressed (#54)
			m.level = levelTrail  // the archive is a list; it opens as one
			m.cursor, m.anchor = -1, -1
			m.fleetScroll = 0
			m.pointQuiet(key)
			m.clampSelection()
			m.note = "in the archive · A returns"
			return true
		}
	}
	return false
}
