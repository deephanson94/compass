package ui

import (
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/state"
)

// Done is the seventh status: a turn that finished and that you have not
// looked at since. Idle said both "it answered you a minute ago" and "it
// has sat there three days", in one grey word; switching between two
// sessions, the one that had just answered was the one you could not find.
// Done has its own colour, and the card's age beside it says how fresh; it
// goes back to idle once you have looked, or once doneFor has passed with
// nobody looking.

var (
	// doneFor is how long an unread finish stays done before it is idle
	// anyway: a session left three days is not news. `done_for` sets it.
	doneFor = 4 * time.Hour
	// colDone is done's colour: a teal, apart from working's green and
	// needs you's amber. `done_color` sets it.
	colDone lipgloss.TerminalColor = lipgloss.AdaptiveColor{Light: "#0f766e", Dark: "#2dd4bf"}
)

// SetDone sets how long an unread finish stays done (0 or less keeps the
// default) and its colour ("" or an unreadable one keeps the default).
func SetDone(after time.Duration, color string) {
	if after > 0 {
		doneFor = after
	}
	if validColor(color) {
		colDone = lipgloss.Color(color)
	}
}

// done says a finished turn is still news: idle, unread, and finished
// within doneFor.
func (m *Model) done(s fleet.Session) bool {
	if m.archiveView || !s.Live || s.Snap.State != state.Idle || s.Snap.Interrupted {
		return false // an Esc is yours: you saw where it stopped
	}
	last := s.Info.LastEventAt
	if last.IsZero() || m.now.Sub(last) > doneFor {
		return false
	}
	seen, ok := m.seen[s.Info.Key()]
	return !ok || seen.Before(last)
}

// markSeenAt records a look at t rather than at the deck's clock. The deck
// is suspended while tmux has the terminal, and its clock stops with it: a
// session that answered while the person was in it is read as of their
// return, not as of their leaving.
func (m *Model) markSeenAt(key string, t time.Time) {
	if t.After(m.now) {
		was := m.now
		m.now = t
		defer func() { m.now = was }()
	}
	m.markSeen(key)
}
