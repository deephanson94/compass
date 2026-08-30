// Package transcript reads Claude Code session transcripts (JSONL) and turns
// each line into a typed Event. Parsing is deliberately fail-soft: an unknown
// line type is data we do not understand yet, never an error.
package transcript

import (
	"encoding/json"
	"strings"
	"time"
)

// EventType is the top-level `type` field of a transcript line.
type EventType string

// Known transcript line types. Anything else becomes EventUnknown.
const (
	EventUser       EventType = "user"
	EventAssistant  EventType = "assistant"
	EventAttachment EventType = "attachment"
	EventQueueOp    EventType = "queue-operation"
	EventUnknown    EventType = "unknown" // any other type: fail-soft, never error
)

// ToolUse is a single `tool_use` block from an assistant message.
type ToolUse struct {
	ID    string          // tool_use block id
	Name  string          // e.g. "Bash", "Read", "AskUserQuestion"
	Input json.RawMessage // raw input object (may be nil)
}

// ToolResult is a single `tool_result` block from a user-type line.
type ToolResult struct {
	ToolUseID string
	IsError   bool
}

// Event is one parsed transcript line.
type Event struct {
	Type        EventType
	UUID        string
	ParentUUID  string
	Timestamp   time.Time // zero if absent/unparseable
	SessionID   string
	CWD         string
	GitBranch   string
	Version     string
	IsSidechain bool
	Text        string       // assistant: all text blocks joined "\n"; user: string content (empty if content is a block array)
	ToolUses    []ToolUse    // assistant tool_use blocks, in order
	ToolResults []ToolResult // tool_result blocks inside user-type lines
}

// rawLine stages the common top-level fields. `message` and `content` stay raw
// because their shapes vary by line type.
type rawLine struct {
	Type        string          `json:"type"`
	UUID        string          `json:"uuid"`
	ParentUUID  string          `json:"parentUuid"` // may be null; null decodes to ""
	Timestamp   string          `json:"timestamp"`
	SessionID   string          `json:"sessionId"`
	CWD         string          `json:"cwd"`
	GitBranch   string          `json:"gitBranch"`
	Version     string          `json:"version"`
	IsSidechain bool            `json:"isSidechain"`
	Message     json.RawMessage `json:"message"`
	Content     json.RawMessage `json:"content"` // queue-operation: a plain string
}

type rawMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // string OR array of blocks
}

type rawBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
}

// ParseLine parses one JSONL line. Unknown `type` values return an Event with
// Type EventUnknown, whatever common fields parsed, and a nil error. Only
// invalid JSON returns an error.
func ParseLine(line []byte) (Event, error) {
	var raw rawLine
	if err := json.Unmarshal(line, &raw); err != nil {
		return Event{}, err
	}

	ev := Event{
		Type:        eventType(raw.Type),
		UUID:        raw.UUID,
		ParentUUID:  raw.ParentUUID,
		SessionID:   raw.SessionID,
		CWD:         raw.CWD,
		GitBranch:   raw.GitBranch,
		Version:     raw.Version,
		IsSidechain: raw.IsSidechain,
	}
	if raw.Timestamp != "" {
		if ts, err := time.Parse(time.RFC3339, raw.Timestamp); err == nil {
			ev.Timestamp = ts.UTC()
		}
	}

	// queue-operation carries the enqueued prompt as a top-level string.
	if len(raw.Message) == 0 && len(raw.Content) > 0 {
		var s string
		if err := json.Unmarshal(raw.Content, &s); err == nil {
			ev.Text = s
		}
	}

	if len(raw.Message) > 0 {
		var msg rawMessage
		if err := json.Unmarshal(raw.Message, &msg); err == nil {
			parseContent(&ev, msg.Content)
		}
	}
	return ev, nil
}

func eventType(s string) EventType {
	switch EventType(s) {
	case EventUser, EventAssistant, EventAttachment, EventQueueOp:
		return EventType(s)
	default:
		return EventUnknown
	}
}

// parseContent handles both content shapes: a plain string (a human prompt) and
// an array of blocks (text / thinking / tool_use / tool_result).
func parseContent(ev *Event, content json.RawMessage) {
	trimmed := skipSpace(content)
	if len(trimmed) == 0 {
		return
	}
	switch trimmed[0] {
	case '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err == nil {
			ev.Text = s
		}
	case '[':
		var blocks []rawBlock
		if err := json.Unmarshal(trimmed, &blocks); err != nil {
			return
		}
		var texts []string
		for _, b := range blocks {
			switch b.Type {
			case "text":
				// Only assistant turns contribute Text; a user block array is
				// tool results, whose Text stays empty by contract.
				if ev.Type == EventAssistant && b.Text != "" {
					texts = append(texts, b.Text)
				}
			case "tool_use":
				ev.ToolUses = append(ev.ToolUses, ToolUse{ID: b.ID, Name: b.Name, Input: b.Input})
			case "tool_result":
				ev.ToolResults = append(ev.ToolResults, ToolResult{ToolUseID: b.ToolUseID, IsError: b.IsError})
			}
			// "thinking" blocks are intentionally ignored.
		}
		if len(texts) > 0 {
			ev.Text = strings.Join(texts, "\n")
		}
	}
}

func skipSpace(b []byte) []byte {
	i := 0
	for i < len(b) {
		switch b[i] {
		case ' ', '\t', '\r', '\n':
			i++
			continue
		}
		break
	}
	return b[i:]
}
