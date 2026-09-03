package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// The seen-times are what "bright means unread" rests on, and they must
// survive a restart: without this file every launch called every session from
// the last day unread again, and the delta line below could never say how far
// back "since you looked" reaches.
//
// The file lives beside the resume cache (~/.cache/compass/seen.json), is a
// flat key → time map, and is best-effort everywhere: a deck that cannot read
// or write it simply starts with nothing seen, which is the truth of a fresh
// machine.

// seenKeep bounds the file: an entry older than this names a session the
// board would show dim regardless, so remembering it buys nothing.
const seenKeep = 30 * 24 * time.Hour

// LoadSeen reads the seen-times from path and remembers the path for saves.
func (m *Model) LoadSeen(path string) {
	m.seenFile = path
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	seen := map[string]time.Time{}
	if json.Unmarshal(raw, &seen) != nil {
		return
	}
	m.seen = seen
}

// saveSeen writes the map back, pruning what no longer earns its line. It
// runs on markSeen only — an explicit keypress — never on a tick.
// LoadHidden reads the sessions the person took off the board (`x`), kept
// beside the seen-times so a restart forgets nothing.
func (m *Model) LoadHidden(path string) {
	m.hiddenFile = path
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var hidden map[string]bool
	if json.Unmarshal(raw, &hidden) == nil {
		m.hidden = hidden
	}
}

func (m *Model) saveHidden() {
	if m.hiddenFile == "" {
		return
	}
	raw, err := json.Marshal(m.hidden)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(m.hiddenFile), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(m.hiddenFile, raw, 0o644)
}

func (m *Model) saveSeen() {
	if m.seenFile == "" {
		return
	}
	cutoff := time.Now().Add(-seenKeep)
	for key, at := range m.seen {
		if at.Before(cutoff) {
			delete(m.seen, key)
		}
	}
	raw, err := json.Marshal(m.seen)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(m.seenFile), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(m.seenFile, raw, 0o644)
}
