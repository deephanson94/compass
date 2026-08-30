// Package todo reads a session's own task list — the plan Claude keeps for
// itself — which the trail renders as ghost waypoints ahead of HEAD: the part
// of the journey that hasn't happened yet.
package todo

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
	// Implemented in M2 (docs/dev/M2-CONTRACT.md).
	return nil, nil
}
