// Package todo reads a session's own task list — the plan Claude keeps for
// itself — which the trail renders as ghost waypoints ahead of HEAD: the part
// of the journey that hasn't happened yet.
package todo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Status is a todo item's lifecycle state, kept verbatim from the file.
type Status string

const (
	Pending    Status = "pending"
	InProgress Status = "in_progress"
	Completed  Status = "completed"
)

// Item is one entry of the session's plan.
type Item struct {
	Text   string
	Status Status
}

// Read loads the session's todo list: it scans <root>/todos/ for *.json files
// whose name contains sessionID; when several match, the newest mtime wins.
// Items parse from a JSON array of objects — Text from "content" (falling back
// to "activeForm"), Status from "status" — order preserved. A missing dir or
// no matching file returns (nil, nil); a malformed file is skipped, never an
// error.
func Read(root, sessionID string) ([]Item, error) {
	if root == "" || sessionID == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(filepath.Join(root, "todos"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // a machine with no todo lists is not an error
		}
		return nil, err
	}

	for _, name := range matches(entries, sessionID) {
		items, ok := parseFile(filepath.Join(root, "todos", name))
		if ok {
			return items, nil
		}
		// Unreadable or malformed: fall through to the next-newest match
		// rather than claim the session has no plan.
	}
	return nil, nil
}

// rawItem is the on-disk shape. "activeForm" is the present-tense phrasing the
// writer keeps beside the plain one; either will do to name the step.
type rawItem struct {
	Content    string `json:"content"`
	ActiveForm string `json:"activeForm"`
	Status     string `json:"status"`
}

// candidate is one matching file and the mtime that ranks it.
type candidate struct {
	name string
	mod  time.Time
}

// matches returns the session's todo files, newest first — a session can leave
// several behind (one per agent), and the freshest is the live one.
func matches(entries []os.DirEntry, sessionID string) []string {
	var found []candidate
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") || !strings.Contains(name, sessionID) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		found = append(found, candidate{name: name, mod: info.ModTime()})
	}
	sort.SliceStable(found, func(i, j int) bool {
		if !found[i].mod.Equal(found[j].mod) {
			return found[i].mod.After(found[j].mod)
		}
		return found[i].name < found[j].name
	})

	names := make([]string, len(found))
	for i := range found {
		names[i] = found[i].name
	}
	return names
}

// parseFile reads one todo file; ok is false when it cannot be believed.
func parseFile(path string) (items []Item, ok bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var raws []rawItem
	if err := json.Unmarshal(data, &raws); err != nil {
		return nil, false
	}
	for _, raw := range raws {
		text := raw.Content
		if text == "" {
			text = raw.ActiveForm
		}
		if text == "" {
			continue // an entry with nothing to say is nothing to draw
		}
		items = append(items, Item{Text: text, Status: Status(raw.Status)})
	}
	return items, true
}
