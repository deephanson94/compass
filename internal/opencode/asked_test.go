package opencode

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// The ask door, for this store (#344, round 59). The panel's two-tools
// reviewer proved the door was Claude-only: an opencode session holding the
// same question, at the same age, came back `archived` where a claude one
// came back waiting on you. One fleet, one rule — a row that says "waiting"
// must mean the same thing whichever tool wrote it.

// askStore writes a store with one root session whose last message is
// `last`, and returns the path. `last` is the message and part rows, built
// by the helpers below.
func askStore(t *testing.T, last func(must func(string, ...any), sessionID string)) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "opencode.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	must := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	must(`insert into project values ('global', ?, 'git', null, 1788688753946, 1788688885654, '[]')`, dir)
	must(`insert into session (id, project_id, parent_id, slug, directory, path, title, version, cost, agent, model, time_created, time_updated)
		values (?, 'global', null, 'eager-sailor', ?, '', 'rate-limit the public api', '1.18.29', 0, 'build', '{"id":"mock-1","providerID":"mock","variant":"default"}', 1788688886175, 1788688892888)`, sess, dir)
	// The prompt that opened it, always.
	must(`insert into message values ('msg_ask_user', ?, 1788688886303, 1788688886303,
		'{"role":"user","time":{"created":1788688886303}}')`, sess)
	must(`insert into part values ('prt_ask_user', 'msg_ask_user', ?, 1788688886303, 1788688886303, '{"type":"text","text":"rate-limit the public api"}')`, sess)
	last(must, sess)
	return path
}

// saidMsg is an assistant turn that finished, carrying `text`.
func saidMsg(id, text string) func(func(string, ...any), string) {
	return func(must func(string, ...any), sessionID string) {
		must(fmt.Sprintf(`insert into message values ('%s', ?, 1788688887339, 1788688892636,
			'{"role":"assistant","modelID":"mock-1","providerID":"mock","time":{"created":1788688887339,"completed":1788688892634},"finish":"stop"}')`, id), sessionID)
		must(fmt.Sprintf(`insert into part values ('%s_p', '%s', ?, 1788688892768, 1788688892773, ?)`, id, id),
			sessionID, fmt.Sprintf(`{"type":"text","text":%q}`, text))
	}
}

// callMsg is an assistant turn that called one tool, left in `status`.
func callMsg(id, tool, status string) func(func(string, ...any), string) {
	return func(must func(string, ...any), sessionID string) {
		must(fmt.Sprintf(`insert into message values ('%s', ?, 1788688887339, 1788688892636,
			'{"role":"assistant","modelID":"mock-1","providerID":"mock","time":{"created":1788688887339,"completed":1788688892634},"finish":"tool-calls"}')`, id), sessionID)
		part := fmt.Sprintf(`{"type":"tool","tool":%q,"callID":"call_1","state":{"status":%q,"input":{"question":"Per-key buckets or per-IP?"},"output":"ok","time":{"start":1788688889199,"end":1788688892503}}}`, tool, status)
		must(fmt.Sprintf(`insert into part values ('%s_p', '%s', ?, 1788688889179, 1788688892504, ?)`, id, id), sessionID, part)
	}
}

func askedInfo(t *testing.T, path string) (bool, time.Time) {
	t.Helper()
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	infos, err := st.Infos()
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 {
		t.Fatalf("Infos = %d sessions, want the one root", len(infos))
	}
	return infos[0].Asked, infos[0].AskedAt
}

// The door reads this store the way it reads a transcript.
func TestTheStoreAnswersTheAskDoor(t *testing.T) {
	cases := []struct {
		name  string
		last  func(func(string, ...any), string)
		asked bool
	}{
		{"the model's last words are a question", saidMsg("msg_z", "Two designs fit. Which should I build?"), true},
		{"the model's last words are not", saidMsg("msg_z", "The plan is applied and the gates pass."), false},
		{"the question tool is still out", callMsg("msg_z", "question", "running"), true},
		{"another call is still out", callMsg("msg_z", "bash", "running"), false},
		{"the call came back", callMsg("msg_z", "bash", "completed"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			asked, at := askedInfo(t, askStore(t, c.last))
			if asked != c.asked {
				t.Errorf("Asked = %v, want %v", asked, c.asked)
			}
			if asked && at.IsZero() {
				t.Errorf("AskedAt is zero on a session that is asking")
			}
		})
	}
}

// And it clears itself here too: the person replies, and the last word in
// the store is theirs.
func TestAReplyClosesTheStoresDoor(t *testing.T) {
	path := askStore(t, func(must func(string, ...any), sessionID string) {
		saidMsg("msg_z", "Two designs fit. Which should I build?")(must, sessionID)
		must(`insert into message values ('msg_reply', ?, 1788688893000, 1788688893000,
			'{"role":"user","time":{"created":1788688893000}}')`, sessionID)
		must(`insert into part values ('prt_reply', 'msg_reply', ?, 1788688893000, 1788688893000, '{"type":"text","text":"the first one"}')`, sessionID)
	})
	if asked, _ := askedInfo(t, path); asked {
		t.Errorf("Asked = true after the person replied")
	}
}

// The question tool's input reaches the row as a question, not as the
// tool's own name: opencode asks one question where Claude Code's tool
// takes a list, and the row prints what is being decided (#85's rule, in
// the one place this round turns on).
func TestTheQuestionToolsInputReadsAsAQuestion(t *testing.T) {
	out := string(toolInput([]byte(`{"question":"Per-key buckets or per-IP?","options":[{"label":"per key"}]}`)))
	for _, want := range []string{`"questions"`, "Per-key buckets or per-IP?", "per key"} {
		if !contains(out, want) {
			t.Errorf("toolInput = %s, want it to carry %s", out, want)
		}
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (len(sub) == 0 || indexOf(s, sub) >= 0) }

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// The statuses the store's own reader acts on, and the one it skips. A
// `question` part left pending is a call the harness never dispatched:
// `events` skips it, so the machine never sees it, and a door that opened
// on it would put `waiting 9d` on a row the machine calls hung — the
// invariant the fleet pins, broken through the one package its transcripts
// cannot reach (round 60).
func TestAPendingQuestionIsNotAnOpenDoor(t *testing.T) {
	for _, status := range []string{"pending", "running", "completed"} {
		t.Run(status, func(t *testing.T) {
			asked, _ := askedInfo(t, askStore(t, callMsg("msg_z", "question", status)))
			if want := status == "running"; asked != want {
				t.Errorf("a %s question: Asked = %v, want %v", status, asked, want)
			}
		})
	}
}

// A turn the gateway refused is recorded on the message, and opencode may
// write no part for it at all: an inner join never reached such a turn, so
// the same question with an error turn over it answered two ways depending
// on whether a step marker happened to be written (round 60).
func TestARefusedTurnWithNoPartsClosesTheDoor(t *testing.T) {
	path := askStore(t, func(must func(string, ...any), sessionID string) {
		saidMsg("msg_z", "Two designs fit. Which should I build?")(must, sessionID)
		must(`insert into message values ('msg_err', ?, 1788688893000, 1788688893000,
			'{"role":"assistant","modelID":"mock-1","providerID":"mock","time":{"created":1788688893000,"completed":1788688893100},"error":{"name":"APIError","data":{"message":"429 rate limited"}}}')`, sessionID)
	})
	if asked, _ := askedInfo(t, path); asked {
		t.Errorf("Asked = true under a refused turn: nothing you type clears a 429")
	}
}

// A turn's words are read in the order it wrote them. The walk took the
// parts back newest-first inside the message too, so a turn that ends on a
// question read as one that does not, and one that merely contains a
// question read as one that ends on it (round 60).
func TestATurnsWordsAreReadInTheOrderItWroteThem(t *testing.T) {
	two := func(first, second string) func(func(string, ...any), string) {
		return func(must func(string, ...any), sessionID string) {
			must(`insert into message values ('msg_z', ?, 1788688887339, 1788688892636,
				'{"role":"assistant","modelID":"mock-1","providerID":"mock","time":{"created":1788688887339,"completed":1788688892634},"finish":"stop"}')`, sessionID)
			must(`insert into part values ('prt_a', 'msg_z', ?, 1788688892768, 1788688892773, ?)`,
				sessionID, fmt.Sprintf(`{"type":"text","text":%q}`, first))
			must(`insert into part values ('prt_b', 'msg_z', ?, 1788688892774, 1788688892775, ?)`,
				sessionID, fmt.Sprintf(`{"type":"text","text":%q}`, second))
		}
	}
	if asked, _ := askedInfo(t, askStore(t, two("Which should I build?", "I will start on the first."))); asked {
		t.Errorf("Asked = true on a turn that asks and then answers itself")
	}
	if asked, _ := askedInfo(t, askStore(t, two("I looked at both designs.", "Which should I build?"))); !asked {
		t.Errorf("Asked = false on a turn that ends on its question")
	}
}
