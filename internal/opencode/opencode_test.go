package opencode

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
	"github.com/deephanson94/compass/internal/transcript"
)

// schema is opencode 1.18's own, as `opencode` created it: the four tables
// compass reads, verbatim.
const schema = `
CREATE TABLE project (id text PRIMARY KEY, worktree text NOT NULL, vcs text, name text, time_created integer NOT NULL, time_updated integer NOT NULL, sandboxes text NOT NULL DEFAULT '[]');
CREATE TABLE session (id text PRIMARY KEY, project_id text NOT NULL, workspace_id text, parent_id text, slug text NOT NULL, directory text NOT NULL, path text, title text NOT NULL, version text NOT NULL, share_url text, summary_additions integer, summary_deletions integer, summary_files integer, summary_diffs text, metadata text, cost real DEFAULT 0 NOT NULL, tokens_input integer DEFAULT 0 NOT NULL, tokens_output integer DEFAULT 0 NOT NULL, tokens_reasoning integer DEFAULT 0 NOT NULL, tokens_cache_read integer DEFAULT 0 NOT NULL, tokens_cache_write integer DEFAULT 0 NOT NULL, revert text, permission text, agent text, model text, time_created integer NOT NULL, time_updated integer NOT NULL, time_compacting integer, time_archived integer);
CREATE TABLE message (id text PRIMARY KEY, session_id text NOT NULL, time_created integer NOT NULL, time_updated integer NOT NULL, data text NOT NULL);
CREATE TABLE part (id text PRIMARY KEY, message_id text NOT NULL, session_id text NOT NULL, time_created integer NOT NULL, time_updated integer NOT NULL, data text NOT NULL);
`

// A session as opencode 1.18.29 wrote it against a mock model: one prompt,
// a bash call that slept three seconds, and a closing sentence. The rows
// are the real ones, ids and clocks included.
const (
	sess = "ses_f89d59660ffevSq3V17eRfwN6q"
	dir  = "/home/user/ocproj"
)

func writeStore(t *testing.T, done bool) string {
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
		values (?, 'global', null, 'eager-sailor', ?, '', 'New session - 2026-09-06T10:01:26.175Z', '1.18.29', 0, 'build', '{"id":"mock-1","providerID":"mock","variant":"default"}', 1788688886175, 1788688892888)`, sess, dir)
	must(`insert into session (id, project_id, parent_id, slug, directory, path, title, version, cost, agent, model, time_created, time_updated)
		values ('ses_child', 'global', ?, 'quiet-otter', ?, '', 'subagent', '1.18.29', 0, 'general', null, 1788688886175, 1788688892888)`, sess, dir)
	must(`insert into message values ('msg_0762a6a1f001GvplY4iLaxf7fq', ?, 1788688886303, 1788688892761,
		'{"role":"user","time":{"created":1788688886303},"agent":"build","model":{"providerID":"mock","modelID":"mock-1"},"summary":{"diffs":[]}}')`, sess)
	must(`insert into part values ('prt_0762a6a2c001eP5946bZFo3H9w', 'msg_0762a6a1f001GvplY4iLaxf7fq', ?, 1788688887312, 1788688887316, '{"type":"text","text":"\"run echo hi\""}')`, sess)
	must(`insert into message values ('msg_0762a6e2b0010YUBdVM8wZzlCL', ?, 1788688887339, 1788688892636,
		'{"parentID":"msg_0762a6a1f001GvplY4iLaxf7fq","role":"assistant","mode":"build","agent":"build","path":{"cwd":"/home/user/ocproj","root":"/home/user/ocproj"},"cost":0,"tokens":{"total":15,"input":10,"output":5,"reasoning":0,"cache":{"write":0,"read":0}},"modelID":"mock-1","providerID":"mock","time":{"created":1788688887339,"completed":1788688892634},"finish":"tool-calls"}')`, sess)
	must(`insert into part values ('prt_0762a7551001qZCmxiILp2ytZI', 'msg_0762a6e2b0010YUBdVM8wZzlCL', ?, 1788688889169, 1788688889173, '{"snapshot":"4a6c48bb","type":"step-start"}')`, sess)
	tool := `{"type":"tool","tool":"bash","callID":"call_1","state":{"status":"running","input":{"command":"sleep 3; echo hi from mock","description":"say hi"},"title":"sleep 3; echo hi from mock","time":{"start":1788688889199}}}`
	if done {
		tool = `{"type":"tool","tool":"bash","callID":"call_1","state":{"status":"completed","input":{"command":"sleep 3; echo hi from mock","description":"say hi"},"output":"hi from mock\n","metadata":{"output":"hi from mock\n","exit":0,"truncated":false},"title":"sleep 3; echo hi from mock","time":{"start":1788688889199,"end":1788688892503}}}`
	}
	must(`insert into part values ('prt_0762a755a001u7vwLfVuE4453J', 'msg_0762a6e2b0010YUBdVM8wZzlCL', ?, 1788688889179, 1788688892504, ?)`, sess, tool)
	if done {
		must(`insert into part values ('prt_0762a829b001NeKjCLB3CdlBMz', 'msg_0762a6e2b0010YUBdVM8wZzlCL', ?, 1788688892571, 1788688892574, '{"reason":"tool-calls","snapshot":"4a6c48bb","type":"step-finish","tokens":{"total":15,"input":10,"output":5,"reasoning":0,"cache":{"write":0,"read":0}},"cost":0}')`, sess)
		must(`insert into message values ('msg_0762a82e2001n4LRJ7Bxm3MRF5', ?, 1788688892642, 1788688892878,
			'{"parentID":"msg_0762a6a1f001GvplY4iLaxf7fq","role":"assistant","mode":"build","agent":"build","path":{"cwd":"/home/user/ocproj","root":"/home/user/ocproj"},"cost":0,"tokens":{"total":15,"input":10,"output":5,"reasoning":0,"cache":{"write":0,"read":0}},"modelID":"mock-1","providerID":"mock","time":{"created":1788688892642,"completed":1788688892876},"finish":"stop"}')`, sess)
		must(`insert into part values ('prt_0762a8360001WbDwKRjS3VP5b2', 'msg_0762a82e2001n4LRJ7Bxm3MRF5', ?, 1788688892768, 1788688892773, '{"type":"text","text":"Done — the shell printed hi.","time":{"start":1788688892768,"end":1788688892772}}')`, sess)
	}
	return path
}

// The store lists the root sessions with the tool's own model, and never
// the subagents opencode spawns as child sessions.
func TestSessionsAreTheRootsWithTheirModel(t *testing.T) {
	st, err := Open(writeStore(t, true))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	infos, err := st.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 {
		t.Fatalf("Sessions = %d, want the one root (the child is its lead's)", len(infos))
	}
	in := infos[0]
	if in.ID != sess || in.Directory != dir || in.Model != "mock/mock-1" || in.Title != "New session - 2026-09-06T10:01:26.175Z" {
		t.Errorf("Sessions()[0] = %+v", in)
	}
	if in.Key() != "opencode://"+sess || SessionID(in.Key()) != sess {
		t.Errorf("Key = %q", in.Key())
	}
	if !in.Updated.Equal(time.UnixMilli(1788688892888).UTC()) {
		t.Errorf("Updated = %v", in.Updated)
	}
}

// A session reads as the transcript's own events — the prompt, the call
// with the classifier's field names, its result, the closing words — in
// time order and each exactly once, so the state machine and the trail
// see an opencode session as they see a claude one.
func TestASessionReadsAsTranscriptEvents(t *testing.T) {
	st, err := Open(writeStore(t, true))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	src := st.NewSource(sess)
	evs, err := src.Poll()
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 4 {
		t.Fatalf("Poll = %d events, want 4:\n%+v", len(evs), evs)
	}
	if evs[0].Type != transcript.EventUser || evs[0].Text != `"run echo hi"` {
		t.Errorf("first event is not the prompt: %+v", evs[0])
	}
	if evs[1].Type != transcript.EventAssistant || len(evs[1].ToolUses) != 1 || evs[1].ToolUses[0].Name != "Bash" || evs[1].ToolUses[0].ID != "call_1" || evs[1].Model != "mock-1" {
		t.Errorf("second event is not the bash call: %+v", evs[1])
	}
	if c, ok := journey.Classify(evs[1]); !ok || c != journey.Build && c != journey.Scout && c != journey.Test {
		t.Errorf("the call does not classify: %v %v", c, ok)
	}
	if evs[2].Type != transcript.EventUser || len(evs[2].ToolResults) != 1 || evs[2].ToolResults[0].ToolUseID != "call_1" || evs[2].ToolResults[0].Text != "hi from mock\n" {
		t.Errorf("third event is not the result: %+v", evs[2])
	}
	if evs[3].Type != transcript.EventAssistant || evs[3].Text != "Done — the shell printed hi." {
		t.Errorf("fourth event is not the closing words: %+v", evs[3])
	}
	for i := 1; i < len(evs); i++ {
		if evs[i].Timestamp.Before(evs[i-1].Timestamp) {
			t.Errorf("events out of order at %d: %v before %v", i, evs[i].Timestamp, evs[i-1].Timestamp)
		}
	}
	again, err := src.Poll()
	if err != nil || len(again) != 0 {
		t.Errorf("a second poll of an unchanged session = %d events, %v", len(again), err)
	}
	m := state.NewMachine()
	for _, ev := range evs {
		m.Observe(ev)
	}
	if snap := m.Evaluate(evs[3].Timestamp.Add(time.Minute)); snap.State != state.Idle {
		t.Errorf("a finished session evaluates as %v, want idle", snap.State)
	}
}

// A call still running is a tool in flight: the machine says working, and
// the result arrives on a later poll without the call being said again.
func TestARunningCallIsInFlight(t *testing.T) {
	path := writeStore(t, false)
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	src := st.NewSource(sess)
	evs, err := src.Poll()
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 || len(evs[1].ToolUses) != 1 {
		t.Fatalf("Poll = %+v, want the prompt and the call", evs)
	}
	m := state.NewMachine()
	for _, ev := range evs {
		m.Observe(ev)
	}
	if snap := m.Evaluate(evs[1].Timestamp.Add(10 * time.Second)); snap.State != state.Working || snap.Activity != "Bash: sleep 3; echo hi from mock" {
		t.Errorf("a running call evaluates as %v %q", snap.State, snap.Activity)
	}
	// The call completes: the store is rewritten as opencode would update it.
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`update part set time_updated = 1788688892600, data = ? where id = 'prt_0762a755a001u7vwLfVuE4453J'`,
		`{"type":"tool","tool":"bash","callID":"call_1","state":{"status":"completed","input":{"command":"sleep 3; echo hi from mock"},"output":"hi from mock\n","title":"sleep 3; echo hi from mock","time":{"start":1788688889199,"end":1788688892503}}}`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	more, err := src.Poll()
	if err != nil {
		t.Fatal(err)
	}
	if len(more) != 1 || len(more[0].ToolResults) != 1 || more[0].ToolResults[0].ToolUseID != "call_1" {
		t.Fatalf("the next poll = %+v, want the result alone", more)
	}
}

// The scheme registers the store as a transcript source: a tailer on an
// opencode:// key polls the session, and a file path is untouched.
func TestTheSchemeTailsTheStore(t *testing.T) {
	st, err := Open(writeStore(t, true))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	transcript.RegisterScheme(Scheme, st.Opener())
	tl := transcript.NewTailer("opencode://" + sess)
	evs, err := tl.Poll()
	if err != nil || len(evs) != 4 {
		t.Fatalf("the tailer polled %d events, %v", len(evs), err)
	}
	if transcript.SchemeOf("/home/x/a.jsonl") != "" || transcript.SchemeOf("opencode://x") != Scheme {
		t.Errorf("SchemeOf misreads a path")
	}
}
