package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
)

// The board is Lv0: every trail that fits, side by side, with the fleet at
// its floor on the left. It is the deck's default on any terminal wide enough
// for it, and Shift+Tab from a single trail returns to it (decision #16).
//
// It exists because a wide monitor showing one trail and a column of names
// left the person looking at it worried about everything it was not showing.
// The mirror was one answer to that worry and turned out to be the wrong one:
// a glimpse of one CLI's last screenful. The board is the other: the journeys
// themselves, all the ones that are moving, at a glance.
//
// Columns are in the fleet's own order — needs-you first, then stuck, then
// working, then idle by recency — so the sessions that can want you are on
// the left and the ones that have gone quiet fall off the right. The selected
// session always has a column. Idle sessions' trails are drawn dim: they are
// history until someone types into them, and the eye should land on the ones
// that are moving (SPEC §4).
const (
	boardColMin = 34 // narrower than this a trail row says nothing
	boardColMax = trailWidthMax
)

// boardShown says whether the board can be drawn at all: only on a terminal
// wide enough for the fleet and at least one trail column at its minimum.
func (m *Model) boardShown() bool {
	n, _ := boardColumns(m.width - 2*edgePad)
	return m.width >= deckWideCols && n > 0
}

// boardColumns says how many trail columns a board of the given inner width
// holds beside the fleet, and how wide each is. Every column that fits at the
// minimum is opened; the width left over goes to all of them evenly, up to
// the cap past which a row is padding rather than information.
func boardColumns(inner int) (n, w int) {
	avail := inner - fleetWidth - gutterWidth
	if avail < boardColMin {
		return 0, 0
	}
	n = (avail + gutterWidth) / (boardColMin + gutterWidth)
	w = (avail - (n-1)*gutterWidth) / n
	if w > boardColMax {
		w = boardColMax
	}
	return n, w
}

// boardKeys picks the sessions that get a column: the first n of the view in
// its own order, with the selected session always among them — it takes the
// last column when it would not otherwise have one, so `7` on a three-column
// board shows session 7's trail rather than nothing.
func (m *Model) boardKeys(n int) []string {
	if n <= 0 {
		return nil
	}
	keys := make([]string, 0, n)
	for _, s := range m.sessions {
		if s.Live == m.archiveView {
			continue // the other view's sessions
		}
		keys = append(keys, s.Info.Key())
		if len(keys) == n {
			break
		}
	}
	if _, ok := m.selected(); ok && m.selectedKey != "" {
		present := false
		for _, k := range keys {
			if k == m.selectedKey {
				present = true
				break
			}
		}
		if !present {
			if len(keys) == n {
				keys[n-1] = m.selectedKey
			} else {
				keys = append(keys, m.selectedKey)
			}
		}
	}
	return keys
}

// boardTarget is one session the refresh polls for the board: its key and the
// transcript behind it.
type boardTarget struct{ key, path string }

// boardTargets is what refresh polls when the board can be drawn: every
// session with a column. The selected session is among them, so a single poll
// serves the board and the single trail alike.
func (m *Model) boardTargets() []boardTarget {
	if !m.boardShown() {
		return nil
	}
	n, _ := boardColumns(m.width - 2*edgePad)
	var out []boardTarget
	for _, key := range m.boardKeys(n) {
		if s, ok := m.session(key); ok && s.Info.TranscriptPath != "" {
			out = append(out, boardTarget{key: key, path: s.Info.TranscriptPath})
		}
	}
	return out
}

// session finds a session by key in the current fleet.
func (m *Model) session(key string) (fleet.Session, bool) {
	for _, s := range m.sessions {
		if s.Info.Key() == key {
			return s, true
		}
	}
	return fleet.Session{}, false
}

// boardLines lays the board: the fleet at its floor, then one trail column per
// session, each under the same row the fleet gives that session — number,
// glyph, name, age — so a column is recognisably the fleet row it belongs to,
// and `1`–`9` reads off the board as readily as off the list.
func (m *Model) boardLines(w, h int) []string {
	n, cw := boardColumns(w)
	cols := []column{{fleetWidth, m.fleetColumn(fleetWidth, h)}}
	rowOf := map[string]fleetRow{}
	for _, r := range m.fleetRows() {
		if !r.header {
			rowOf[m.sessions[r.sess].Info.Key()] = r
		}
	}
	for _, key := range m.boardKeys(n) {
		r, ok := rowOf[key]
		if !ok {
			continue
		}
		cols = append(cols, column{cw, m.boardColumn(key, r, cw, h)})
	}
	return joinColumns(h, cols)
}

// boardColumn is one session's column: its fleet row as the header, a line of
// air, and its trail pinned to the present. An idle session's trail is drawn
// dim, glyphs and all: the shapes still carry the classes (SPEC §4), and the
// eye goes to the columns that are moving.
func (m *Model) boardColumn(key string, r fleetRow, w, h int) []string {
	s := m.sessions[r.sess]
	rows := []string{m.entryLines(r, w)[0], ""}
	if h <= 2 {
		return fit(rows, h)
	}
	tr := m.trails[key]
	working := s.Snap.State == state.Working && !m.archiveView
	frame := RenderTrail(tr, TrailOpts{
		Todos:      planItems(tr.Tasks),
		Labels:     m.boardLabels[key],
		SessionKey: key,
		Now:        m.now,
		Width:      w,
		Height:     h - 2,
		Level:      levelTrail,
		Cursor:     -1,
		Pulse:      m.pulse && working,
		Pinned:     true,
	})
	lines := strings.Split(frame, "\n")
	if m.boardMuted(s) {
		for i, line := range lines {
			lines[i] = dimStyle.Render(ansi.Strip(line))
		}
	}
	return append(rows, lines...)
}

// boardMuted says whether a column is drawn dim: an idle session's, and every
// archived one's. A session that is working, or waiting on you, keeps its
// tints — those are the columns the eye should land on.
func (m *Model) boardMuted(s fleet.Session) bool {
	return m.archiveView || s.Snap.State == state.Idle
}

// refreshBoard folds a refresh's trails into the board: the trails themselves,
// and the narrated labels that have landed for each. Narration is requested
// for every column — one batch at a time, the narrator's own rule — so a
// column reads in prose rather than file names once it has been on screen a
// while, the same as the single trail does.
func (m *Model) refreshBoard(trails map[string]journey.Trail) {
	if trails == nil {
		return
	}
	m.trails = trails
	m.boardLabels = make(map[string]map[string]string, len(trails))
	if m.narrator == nil {
		return
	}
	for key, tr := range trails {
		m.boardLabels[key] = m.narrator.Labels(key, tr)
		if key != m.selectedKey {
			m.narrator.Request(key, tr, "")
		}
	}
}
