// Package opencode reads OpenCode's sessions out of its SQLite store and
// hands them to compass in the transcript's own vocabulary, so a session
// run by opencode sits in the fleet beside one run by claude — same states,
// same trail, same reader — and the row says which tool and which model.
//
// OpenCode keeps everything in one file, ~/.local/share/opencode/opencode.db:
// a `session` row per conversation (title, directory, the model it was
// opened with, a parent for the subagents its task tool spawns), a
// `message` row per turn whose JSON `data` carries the role, the provider
// and model, and the clocks, and a `part` row per block of a message —
// text, a tool call with its state (pending → running → completed | error),
// reasoning, step markers. compass opens the file read-only, never writes,
// and reads a session incrementally: what it has already reported is not
// reported again.
package opencode

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/transcript"
	_ "modernc.org/sqlite" // the pure-Go driver: no cgo, one binary
)

// Scheme is the transcript scheme opencode sessions are keyed under:
// "opencode://<session id>". The key stays unique across tools, and the
// tailer opens the store instead of a file.
const Scheme = "opencode"

// Tool is the name the fleet shows for a session run by opencode.
const Tool = "opencode"

// DefaultDB is where opencode keeps its store: $XDG_DATA_HOME/opencode, or
// ~/.local/share/opencode.
func DefaultDB() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "opencode", "opencode.db")
}

// Info is one opencode session as the fleet needs it.
type Info struct {
	ID        string
	ParentID  string // "" for a root session; a subagent names the session that spawned it
	Directory string
	Title     string
	Model     string // "<provider>/<model>" as the session was opened with, "" when unknown
	Created   time.Time
	Updated   time.Time
}

// Key is the transcript path the fleet keys this session by.
func (i Info) Key() string { return Scheme + "://" + i.ID }

// SessionID reads the id back out of a key; "" when the key is not one.
func SessionID(key string) string {
	if !strings.HasPrefix(key, Scheme+"://") {
		return ""
	}
	return strings.TrimPrefix(key, Scheme+"://")
}

// Store is the open database.
type Store struct {
	path string
	db   *sql.DB
	mu   sync.Mutex
}

// Open opens the store read-only. A missing file is an error: the caller
// decides whether opencode is on this machine at all.
func Open(path string) (*Store, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(2000)&_pragma=query_only(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{path: path, db: db}, nil
}

// Close closes the store.
func (s *Store) Close() error { return s.db.Close() }

// Path is the file the store reads.
func (s *Store) Path() string { return s.path }

// Sessions lists every root session, newest first. Subagents — sessions
// with a parent — are not fleet rows: they belong to their lead's trail.
func (s *Store) Sessions() ([]Info, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`select id, coalesce(parent_id, ''), directory, title, coalesce(model, ''), time_created, time_updated
		from session where parent_id is null order by time_updated desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Info
	for rows.Next() {
		var in Info
		var model string
		var created, updated int64
		if err := rows.Scan(&in.ID, &in.ParentID, &in.Directory, &in.Title, &model, &created, &updated); err != nil {
			return nil, err
		}
		in.Model = modelName(model)
		in.Created, in.Updated = ms(created), ms(updated)
		out = append(out, in)
	}
	return out, rows.Err()
}

// Infos is Sessions as the fleet takes them: keyed by the scheme path,
// named by the store, at the directory the session runs in.
func (s *Store) Infos() ([]fleet.SessionInfo, error) {
	sessions, err := s.Sessions()
	if err != nil {
		return nil, err
	}
	out := make([]fleet.SessionInfo, 0, len(sessions))
	for _, in := range sessions {
		out = append(out, fleet.SessionInfo{
			ID: in.ID, TranscriptPath: in.Key(), ProjectSlug: Scheme,
			CWD: in.Directory, OriginCWD: in.Directory, Title: titleOf(in.Title),
			StartedAt: in.Created, LastEventAt: in.Updated,
			Tool: Tool, Model: in.Model,
		})
	}
	return out, nil
}

// Attach registers the store with the fleet: its scheme with the
// transcript package, its sessions with the manager.
func (s *Store) Attach(mgr *fleet.Manager) {
	transcript.RegisterScheme(Scheme, s.Opener())
	mgr.AddSource(s.Infos)
}

// modelName turns the session's model JSON — {"id":"mock-1","providerID":
// "mock","variant":"default"} — into "mock/mock-1".
func modelName(raw string) string {
	if raw == "" {
		return ""
	}
	var m struct {
		ID         string `json:"id"`
		ProviderID string `json:"providerID"`
	}
	if json.Unmarshal([]byte(raw), &m) != nil || m.ID == "" {
		return ""
	}
	if m.ProviderID == "" {
		return m.ID
	}
	return m.ProviderID + "/" + m.ID
}

func ms(t int64) time.Time {
	if t == 0 {
		return time.Time{}
	}
	return time.UnixMilli(t).UTC()
}

// message is a message row's JSON data, the fields compass reads.
type message struct {
	Role       string `json:"role"`
	ProviderID string `json:"providerID"`
	ModelID    string `json:"modelID"`
	Path       struct {
		CWD string `json:"cwd"`
	} `json:"path"`
	Time struct {
		Created   int64 `json:"created"`
		Completed int64 `json:"completed"`
	} `json:"time"`
	Error json.RawMessage `json:"error"`
}

// part is a part row's JSON data.
type part struct {
	Type   string `json:"type"`
	Text   string `json:"text"`
	Tool   string `json:"tool"`
	CallID string `json:"callID"`
	State  struct {
		Status string          `json:"status"`
		Input  json.RawMessage `json:"input"`
		Output string          `json:"output"`
		Title  string          `json:"title"`
		Error  string          `json:"error"`
		Time   struct {
			Start int64 `json:"start"`
			End   int64 `json:"end"`
		} `json:"time"`
	} `json:"state"`
}

// row is one part with its message, as the reader walks them.
type row struct {
	msgID   string
	partID  string
	msg     message
	part    part
	msgAt   int64
	updated int64
}

// Source tails one session out of the store, reporting each turn once:
// a user message when it is complete, an assistant's words when its turn
// completes, a tool call when its part appears, its result when the part
// completes or errors. It is the transcript.Source the fleet's tailer
// wraps for an opencode:// key.
type Source struct {
	store     *Store
	sessionID string

	said   map[string]bool // message ids whose words were reported
	called map[string]bool // part ids whose tool call was reported
	done   map[string]bool // part ids whose result was reported
	last   int64           // newest time_updated seen, so an unchanged session costs one query
}

// NewSource returns a Source at the start of the session.
func (s *Store) NewSource(sessionID string) *Source {
	return &Source{store: s, sessionID: sessionID, said: map[string]bool{}, called: map[string]bool{}, done: map[string]bool{}}
}

// Opener is the transcript scheme opener for this store.
func (s *Store) Opener() func(path string) transcript.Source {
	return func(path string) transcript.Source { return s.NewSource(SessionID(path)) }
}

// Poll returns the events written since the last call.
func (src *Source) Poll() ([]transcript.Event, error) {
	src.store.mu.Lock()
	defer src.store.mu.Unlock()
	db := src.store.db
	var newest sql.NullInt64
	if err := db.QueryRow(`select max(time_updated) from part where session_id = ?`, src.sessionID).Scan(&newest); err != nil {
		return nil, err
	}
	if newest.Valid && newest.Int64 <= src.last && src.last > 0 {
		return nil, nil
	}
	rows, err := db.Query(`select m.id, p.id, m.data, p.data, m.time_created, p.time_updated
		from part p join message m on m.id = p.message_id
		where p.session_id = ? order by m.time_created, m.id, p.id`, src.sessionID)
	if err != nil {
		return nil, err
	}
	var all []row
	for rows.Next() {
		var r row
		var mdata, pdata string
		if err := rows.Scan(&r.msgID, &r.partID, &mdata, &pdata, &r.msgAt, &r.updated); err != nil {
			rows.Close()
			return nil, err
		}
		if json.Unmarshal([]byte(mdata), &r.msg) != nil || json.Unmarshal([]byte(pdata), &r.part) != nil {
			continue
		}
		all = append(all, r)
		if r.updated > src.last {
			src.last = r.updated
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return src.events(all), nil
}

// pending is an event waiting to be sorted into time order.
type pending struct {
	at time.Time
	ev transcript.Event
}

// events turns the rows into the events not yet reported, in time order.
func (src *Source) events(all []row) []transcript.Event {
	var out []pending
	byMsg := map[string][]row{}
	var order []string
	for _, r := range all {
		if _, ok := byMsg[r.msgID]; !ok {
			order = append(order, r.msgID)
		}
		byMsg[r.msgID] = append(byMsg[r.msgID], r)
	}
	for _, id := range order {
		parts := byMsg[id]
		msg := parts[0].msg
		base := transcript.Event{UUID: id, SessionID: src.sessionID, CWD: msg.Path.CWD}
		var texts []string
		for _, r := range parts {
			switch r.part.Type {
			case "text":
				texts = append(texts, r.part.Text)
			case "tool":
				if r.part.State.Status == "pending" || src.called[r.partID] && src.done[r.partID] {
					continue
				}
				if !src.called[r.partID] {
					src.called[r.partID] = true
					at := ms(r.part.State.Time.Start)
					if at.IsZero() {
						at = ms(msg.Time.Created)
					}
					ev := base
					ev.Type, ev.Timestamp, ev.Model = transcript.EventAssistant, at, msg.ModelID
					ev.UUID = r.partID
					ev.ToolUses = []transcript.ToolUse{{ID: r.part.CallID, Name: toolName(r.part.Tool), Input: toolInput(r.part.State.Input)}}
					out = append(out, pending{at, ev})
				}
				if st := r.part.State.Status; (st == "completed" || st == "error") && !src.done[r.partID] {
					src.done[r.partID] = true
					at := ms(r.part.State.Time.End)
					if at.IsZero() {
						at = ms(r.updated)
					}
					text := r.part.State.Output
					if st == "error" && r.part.State.Error != "" {
						text = r.part.State.Error
					}
					ev := base
					ev.Type, ev.Timestamp = transcript.EventUser, at
					ev.UUID = r.partID + "/result"
					ev.ToolResults = []transcript.ToolResult{{ToolUseID: r.part.CallID, IsError: st == "error", Text: clamp(text)}}
					out = append(out, pending{at, ev})
				}
			}
		}
		// The words of a turn, once: a user's as soon as they are there,
		// an assistant's when the turn is complete — its text parts are
		// still being streamed until then.
		if src.said[id] {
			continue
		}
		text := strings.TrimSpace(strings.Join(texts, "\n"))
		switch msg.Role {
		case "user":
			if text == "" {
				continue
			}
			src.said[id] = true
			ev := base
			ev.Type, ev.Timestamp, ev.Text = transcript.EventUser, ms(msg.Time.Created), text
			out = append(out, pending{ev.Timestamp, ev})
		case "assistant":
			if msg.Time.Completed == 0 {
				continue
			}
			src.said[id] = true
			if text == "" && len(msg.Error) == 0 {
				continue
			}
			ev := base
			ev.Type, ev.Timestamp, ev.Text, ev.Model = transcript.EventAssistant, ms(msg.Time.Created), text, msg.ModelID
			if len(msg.Error) > 0 && text == "" {
				ev.Text, ev.APIError = errorText(msg.Error), true
			}
			out = append(out, pending{ev.Timestamp, ev})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].at.Before(out[j].at) })
	evs := make([]transcript.Event, 0, len(out))
	for _, p := range out {
		evs = append(evs, p.ev)
	}
	return evs
}

// errorText is the message an assistant turn failed with, as opencode
// records it — {"name":"APIError","data":{"message":"…"}}.
func errorText(raw json.RawMessage) string {
	var e struct {
		Name string `json:"name"`
		Data struct {
			Message string `json:"message"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &e) != nil {
		return "API Error"
	}
	if e.Data.Message != "" {
		return "API Error: " + e.Data.Message
	}
	return "API Error: " + e.Name
}

// toolName maps opencode's tool ids onto the names the trail classifies
// by: the same verbs, capitalised the way Claude Code writes them.
func toolName(tool string) string {
	switch tool {
	case "bash":
		return "Bash"
	case "read":
		return "Read"
	case "edit", "patch", "multiedit":
		return "Edit"
	case "write":
		return "Write"
	case "glob":
		return "Glob"
	case "grep":
		return "Grep"
	case "task":
		return "Agent"
	case "todowrite", "todoread":
		return "TodoWrite"
	case "webfetch":
		return "WebFetch"
	case "websearch":
		return "WebSearch"
	case "question":
		return "AskUserQuestion"
	}
	return tool
}

// toolInput renames the fields the classifier reads: opencode says
// filePath where Claude Code says file_path; the command is the command.
func toolInput(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return raw
	}
	if fp, ok := m["filePath"]; ok {
		if _, has := m["file_path"]; !has {
			m["file_path"] = fp
		}
	}
	out, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return out
}

// resultCap bounds a tool's output the way the transcript package does.
const resultCap = 4000

func clamp(s string) string {
	if len(s) <= resultCap {
		return s
	}
	return s[:resultCap/2] + "\n…\n" + s[len(s)-resultCap/2:]
}

// titleOf is a session's title as the fleet may headline it: OpenCode names
// a fresh session "New session - <timestamp>" until it titles it, and that
// default is no title — the band showed four of them where the first
// prompt was the one thing worth reading (#78).
func titleOf(title string) string {
	t := strings.TrimSpace(title)
	if strings.HasPrefix(t, "New session - ") || strings.HasPrefix(t, "New session – ") {
		return ""
	}
	return t
}
