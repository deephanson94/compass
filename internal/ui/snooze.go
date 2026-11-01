package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/deephanson94/compass/internal/state"
)

// `z` snoozes a session: off the board like `x`, until its next prompt.
// A session a weekly cron prompts sat idle on the board all week, or, hidden
// with `x`, stayed hidden through the run the cron started — `x` is for good,
// and working is not one of the alarms that bring a hidden session back.
// Snoozed, it is hidden the same way (the archive lists it, `x` there brings
// it back) and comes back by itself the moment it does something new.
//
// "Something new" is a beat of its own: a prompt or a reply, which moves the
// state machine's Since past the beat it stood on when it was snoozed, or a
// turn in flight. A line of bookkeeping (a resume, a hook, an agent back)
// moves neither and wakes nothing.

// snooze is `z` on the selected session.
func (m *Model) snooze() {
	s, ok := m.selected()
	if !ok {
		return
	}
	key, name := s.Info.Key(), sessionName(s.Info)
	switch {
	case m.archiveView:
		m.note = "z snoozes from the board · x brings one back"
		return
	case m.hidden[key]:
		m.note = name + " is already off the board"
		return
	case s.Snap.State == state.Working:
		m.note = name + " is working · z snoozes it once the turn is over"
		return
	}
	m.toggleHidden() // the hide itself, with x's refusals and its note
	if !m.hidden[key] {
		return // refused: x's note says why
	}
	if m.snoozed == nil {
		m.snoozed = map[string]time.Time{}
	}
	m.snoozed[key] = s.Snap.Since
	m.saveSnoozed()
	m.note = strings.Replace(m.note, " is hidden · A, then x", " is snoozed · back on its next prompt", 1)
}

// wakeSnoozed brings back every snoozed session that has done something
// since: run on every poll, so the card is on the board the poll after the
// cron's prompt lands.
func (m *Model) wakeSnoozed() {
	woke := false
	for key, beat := range m.snoozed {
		s, ok := m.session(key)
		if !ok {
			continue // not in this fleet: it keeps its snooze
		}
		if s.Snap.State == state.Working || s.Snap.Since.After(beat) {
			delete(m.snoozed, key)
			delete(m.hidden, key)
			woke = true
		}
	}
	if woke {
		m.saveSnoozed()
		m.saveHidden()
	}
}

// unsnooze forgets a snooze the person ended themselves (`x` in the
// archive): the session is back, and its next idle hour does not hide it.
func (m *Model) unsnooze(key string) {
	if _, ok := m.snoozed[key]; ok {
		delete(m.snoozed, key)
		m.saveSnoozed()
	}
}

// LoadSnoozed reads the snoozes kept beside the hidden set.
func (m *Model) LoadSnoozed(path string) {
	m.snoozedFile = path
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var snoozed map[string]time.Time
	if json.Unmarshal(raw, &snoozed) == nil {
		m.snoozed = snoozed
	}
}

func (m *Model) saveSnoozed() {
	if m.snoozedFile == "" {
		return
	}
	raw, err := json.Marshal(m.snoozed)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(m.snoozedFile), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(m.snoozedFile, raw, 0o644)
}
