package opencode

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/deephanson94/compass/internal/state"
)

// unmarshal is the store's own fail-soft decode: a row whose JSON has moved
// on is skipped, never an error the fleet has to carry.
func unmarshal(raw string, into any) error { return json.Unmarshal([]byte(raw), into) }

// The ask door, for the other store (#344, round 59).
//
// A session whose last word is the model's, asking you something, stays live
// however long ago it asked. For a Claude Code session the fleet reads that
// off the end of the transcript; an opencode session has no transcript to
// read, so the same question is asked of the rows the store keeps — one
// fleet, one rule, or the word "waiting" means different things in two rows
// of the same list.
//
// The rule is the tail walk's, in this store's own vocabulary: a person's
// words settle it (nothing waits), a call still out is work in flight unless
// it is the question tool, a turn whose calls all came back is a turn the
// model is still in the middle of, and otherwise the model's last words
// decide by the same test rule 4 uses.

// askRows is how many part rows the walk reads per session. A question is
// the last thing in a session or it is not the question this door is for,
// and the rows are read newest-first, so a handful covers the last turns.
const askRows = 80

// askCache remembers one session's verdict against the clock the store
// stamps it with: a session that has not been touched since it was last
// read is not read again, which is what keeps a refresh a second cheap on a
// store with hundreds of sessions.
type askCache struct {
	updated time.Time
	asked   bool
	at      time.Time
}

// asked answers the door's question for one session, from the cache where
// the session has not moved since it was last asked.
func (s *Store) asked(in Info) (bool, time.Time) {
	s.mu.Lock()
	if c, ok := s.asks[in.ID]; ok && c.updated.Equal(in.Updated) {
		s.mu.Unlock()
		return c.asked, c.at
	}
	s.mu.Unlock()

	asked, at := s.walkAsk(in.ID)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.asks == nil {
		s.asks = map[string]askCache{}
	}
	s.asks[in.ID] = askCache{updated: in.Updated, asked: asked, at: at}
	return asked, at
}

// keepAsks drops the cache entries of sessions the store no longer lists,
// so a long run does not remember what is gone.
func (s *Store) keepAsks(live map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range s.asks {
		if !live[id] {
			delete(s.asks, id)
		}
	}
}

// walkAsk reads the session's last rows, newest first, and stops on the
// first message that settles the question.
func (s *Store) walkAsk(sessionID string) (bool, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`select m.id, m.data, p.data, m.time_created
		from part p join message m on m.id = p.message_id
		where p.session_id = ? order by m.time_created desc, m.id desc, p.id desc limit ?`,
		sessionID, askRows)
	if err != nil {
		return false, time.Time{}
	}
	defer rows.Close()

	// The rows arrive newest-first and grouped by message, so a message is
	// complete once the next id shows up.
	var curID string
	var cur message
	var parts []part
	var created int64
	settled, asked, at := false, false, time.Time{}
	settle := func() {
		if settled {
			return
		}
		asked, at, settled = askedIn(cur, parts, created)
	}
	for rows.Next() {
		var id, mdata, pdata string
		var msgAt int64
		if rows.Scan(&id, &mdata, &pdata, &msgAt) != nil {
			continue
		}
		if id != curID {
			if curID != "" {
				if settle(); settled {
					return asked, at
				}
			}
			curID, parts, created, cur = id, nil, msgAt, message{}
			if unmarshal(mdata, &cur) != nil {
				continue
			}
		}
		var p part
		if unmarshal(pdata, &p) == nil {
			parts = append(parts, p)
		}
	}
	if curID != "" {
		if settle(); settled {
			return asked, at
		}
	}
	return false, time.Time{}
}

// askedIn is the rule applied to one message: whether it settles the walk,
// and which way. The bool it returns last is "settled".
func askedIn(msg message, parts []part, created int64) (bool, time.Time, bool) {
	if msg.Role == "user" {
		for _, p := range parts {
			if p.Type == "text" && strings.TrimSpace(p.Text) != "" {
				return false, time.Time{}, true // your words: nothing waits
			}
		}
		return false, time.Time{}, false
	}
	if msg.Role != "assistant" {
		return false, time.Time{}, false
	}

	calls, out := 0, 0
	for _, p := range parts {
		if p.Type != "tool" {
			continue
		}
		calls++
		if p.State.Status == "completed" || p.State.Status == "error" {
			continue
		}
		out++
		if toolName(p.Tool) == state.AskUserQuestion {
			at := ms(p.State.Time.Start)
			if at.IsZero() {
				at = ms(created)
			}
			return true, at, true // the model asking in as many words
		}
	}
	if out > 0 {
		return false, time.Time{}, true // a call still out is work in flight
	}
	if calls > 0 {
		// Every call came back, so the model's words here are not the last
		// word — the results are, and the turn is the model's to continue.
		return false, time.Time{}, true
	}
	if len(msg.Error) > 0 {
		return false, time.Time{}, true // a refused call is not a question
	}
	var texts []string
	for _, p := range parts {
		if p.Type == "text" {
			texts = append(texts, p.Text)
		}
	}
	text := strings.TrimSpace(strings.Join(texts, "\n"))
	if text == "" {
		return false, time.Time{}, false
	}
	if msg.Time.Completed == 0 {
		return false, time.Time{}, true // still being written, not asked
	}
	return state.EndsWithQuestion(text), ms(created), true
}
