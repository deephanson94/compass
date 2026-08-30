package todo_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/todo"
)

// sid is the session every test reads for; other is a session whose files must
// never be picked up.
const (
	sid   = "11111111-1111-4111-8111-111111111111"
	other = "22222222-2222-4222-8222-222222222222"
)

// mtimeBase anchors every file's mtime so "newest wins" never touches the wall
// clock; ages below are offsets back from it.
var mtimeBase = time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

// ---------------------------------------------------------------- helpers

// todosRoot returns a fresh root and its (already created) todos/ directory.
func todosRoot(t *testing.T) (root, dir string) {
	t.Helper()
	root = t.TempDir()
	dir = filepath.Join(root, "todos")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir todos: %v", err)
	}
	return root, dir
}

// writeTodoFile writes body to <dir>/<name> and stamps it `age` before
// mtimeBase, so the newest file is the one with the smallest age.
func writeTodoFile(t *testing.T, dir, name, body string, age time.Duration) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	when := mtimeBase.Add(-age)
	if err := os.Chtimes(p, when, when); err != nil {
		t.Fatalf("chtimes %s: %v", name, err)
	}
}

// mustRead calls Read and fails on any error: Read never errors by contract.
func mustRead(t *testing.T, root, sessionID string) []todo.Item {
	t.Helper()
	items, err := todo.Read(root, sessionID)
	if err != nil {
		t.Fatalf("Read(%q, %q) returned error %v, want nil (a malformed file is skipped, not an error)",
			root, sessionID, err)
	}
	return items
}

func texts(items []todo.Item) string {
	var out []string
	for _, it := range items {
		out = append(out, string(it.Status)+":"+it.Text)
	}
	return strings.Join(out, " | ")
}

// assertItems compares the whole list — text, status and order.
func assertItems(t *testing.T, got []todo.Item, want ...todo.Item) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d items, want %d:\n  got:  %s\n  want: %s",
			len(got), len(want), texts(got), texts(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Items[%d] = %+v, want %+v\n  got:  %s\n  want: %s",
				i, got[i], want[i], texts(got), texts(want))
		}
	}
}

// ---------------------------------------------------------------- T42

// T42 — the shape Claude actually writes: content when present, activeForm as
// the fallback, status verbatim, file order preserved.
func TestT42ReadItemsAndActiveFormFallback(t *testing.T) {
	root, dir := todosRoot(t)
	writeTodoFile(t, dir, sid+"-agent.json",
		`[{"content":"write parser","status":"pending"},`+
			`{"activeForm":"Writing tests","status":"in_progress"}]`, 0)

	assertItems(t, mustRead(t, root, sid),
		todo.Item{Text: "write parser", Status: todo.Pending},
		todo.Item{Text: "Writing tests", Status: todo.InProgress},
	)
}

// content wins whenever it is there; activeForm only fills a gap.
func TestT42ContentWinsOverActiveForm(t *testing.T) {
	root, dir := todosRoot(t)
	writeTodoFile(t, dir, sid+"-agent.json",
		`[{"content":"write parser","activeForm":"Writing parser","status":"pending"},`+
			`{"content":"","activeForm":"Reading the tailer","status":"in_progress"}]`, 0)

	assertItems(t, mustRead(t, root, sid),
		todo.Item{Text: "write parser", Status: todo.Pending},
		todo.Item{Text: "Reading the tailer", Status: todo.InProgress},
	)
}

// Order is the file's order, not sorted by status: the renderer's "first
// pending is the next action" rule depends on it.
func TestT42OrderIsPreserved(t *testing.T) {
	root, dir := todosRoot(t)
	writeTodoFile(t, dir, sid+"-agent.json", `[
		{"content":"one","status":"completed"},
		{"content":"two","status":"in_progress"},
		{"content":"three","status":"pending"},
		{"content":"four","status":"completed"},
		{"content":"five","status":"pending"}
	]`, 0)

	assertItems(t, mustRead(t, root, sid),
		todo.Item{Text: "one", Status: todo.Completed},
		todo.Item{Text: "two", Status: todo.InProgress},
		todo.Item{Text: "three", Status: todo.Pending},
		todo.Item{Text: "four", Status: todo.Completed},
		todo.Item{Text: "five", Status: todo.Pending},
	)
}

// "kept verbatim": an unknown status is neither normalised nor dropped.
func TestT42StatusIsVerbatim(t *testing.T) {
	root, dir := todosRoot(t)
	writeTodoFile(t, dir, sid+"-agent.json",
		`[{"content":"cancelled work","status":"cancelled"},`+
			`{"content":"shouty","status":"PENDING"},`+
			`{"content":"no status at all"}]`, 0)

	assertItems(t, mustRead(t, root, sid),
		todo.Item{Text: "cancelled work", Status: todo.Status("cancelled")},
		todo.Item{Text: "shouty", Status: todo.Status("PENDING")},
		todo.Item{Text: "no status at all", Status: todo.Status("")},
	)
}

// The three named statuses are the literals the file uses.
func TestT42StatusConstants(t *testing.T) {
	for got, want := range map[todo.Status]string{
		todo.Pending:    "pending",
		todo.InProgress: "in_progress",
		todo.Completed:  "completed",
	} {
		if string(got) != want {
			t.Errorf("Status %q, want %q", string(got), want)
		}
	}
}

// T42 — several files match the session: the newest mtime wins, whatever the
// names sort like.
func TestT42NewestMtimeWins(t *testing.T) {
	newer := `[{"content":"newer plan","status":"pending"}]`
	older := `[{"content":"older plan","status":"pending"}]`

	t.Run("the later-named file is newer", func(t *testing.T) {
		root, dir := todosRoot(t)
		writeTodoFile(t, dir, sid+".json", older, 10*time.Minute)
		writeTodoFile(t, dir, sid+"-agent.json", newer, 1*time.Minute)
		assertItems(t, mustRead(t, root, sid), todo.Item{Text: "newer plan", Status: todo.Pending})
	})

	t.Run("the earlier-named file is newer", func(t *testing.T) {
		root, dir := todosRoot(t)
		writeTodoFile(t, dir, sid+".json", newer, 1*time.Minute)
		writeTodoFile(t, dir, sid+"-agent.json", older, 10*time.Minute)
		assertItems(t, mustRead(t, root, sid), todo.Item{Text: "newer plan", Status: todo.Pending})
	})

	t.Run("three files, the middle one is newest", func(t *testing.T) {
		root, dir := todosRoot(t)
		writeTodoFile(t, dir, "a-"+sid+".json", older, 10*time.Minute)
		writeTodoFile(t, dir, "b-"+sid+"-agent.json", newer, 30*time.Second)
		writeTodoFile(t, dir, "c-"+sid+"-agent-2.json", older, 5*time.Minute)
		assertItems(t, mustRead(t, root, sid), todo.Item{Text: "newer plan", Status: todo.Pending})
	})
}

// T42 — a malformed file is skipped, not an error: the next-newest match is
// used instead.
func TestT42MalformedFileIsSkipped(t *testing.T) {
	good := `[{"content":"good plan","status":"pending"}]`
	broken := []struct {
		name string
		body string
	}{
		{"truncated json", `[{"content":"half a plan"`},
		{"not json at all", "this is not a todo file"},
		{"empty file", ""},
		{"an object, not an array", `{"todos":[{"content":"wrapped","status":"pending"}]}`},
		{"an array of strings", `["write parser","write tests"]`},
		{"wrong field types", `[{"content":42,"status":true}]`},
	}
	for _, tc := range broken {
		t.Run(tc.name, func(t *testing.T) {
			root, dir := todosRoot(t)
			// The broken file is the newest, so it is tried first.
			writeTodoFile(t, dir, sid+"-agent.json", tc.body, 1*time.Minute)
			writeTodoFile(t, dir, sid+".json", good, 10*time.Minute)

			assertItems(t, mustRead(t, root, sid), todo.Item{Text: "good plan", Status: todo.Pending})
		})
	}
}

// A malformed file with no fallback is still not an error.
func TestT42MalformedFileAloneIsNotAnError(t *testing.T) {
	root, dir := todosRoot(t)
	writeTodoFile(t, dir, sid+"-agent.json", `[{"content":"half a plan"`, 0)

	if items := mustRead(t, root, sid); len(items) != 0 {
		t.Errorf("got %d items, want none: %s", len(items), texts(items))
	}
}

// T42 — files belonging to another session are invisible, however new.
func TestT42WrongSessionFilesAreIgnored(t *testing.T) {
	root, dir := todosRoot(t)
	writeTodoFile(t, dir, other+"-agent.json", `[{"content":"someone else","status":"pending"}]`, 0)
	writeTodoFile(t, dir, sid+"-agent.json", `[{"content":"mine","status":"pending"}]`, 5*time.Minute)

	assertItems(t, mustRead(t, root, sid), todo.Item{Text: "mine", Status: todo.Pending})
}

// Only *.json files are scanned, even when the name carries the session id.
func TestT42NonJSONFilesAreIgnored(t *testing.T) {
	root, dir := todosRoot(t)
	writeTodoFile(t, dir, sid+"-agent.json.bak", `[{"content":"stale backup","status":"pending"}]`, 0)
	writeTodoFile(t, dir, sid+"-agent.jsonl", `[{"content":"not a list","status":"pending"}]`, 0)
	writeTodoFile(t, dir, sid+"-agent.json", `[{"content":"mine","status":"pending"}]`, 5*time.Minute)

	assertItems(t, mustRead(t, root, sid), todo.Item{Text: "mine", Status: todo.Pending})
}

// T42 — nothing to read is (nil, nil), never an error.
func TestT42NothingToRead(t *testing.T) {
	t.Run("missing todos dir", func(t *testing.T) {
		root := t.TempDir() // no todos/ inside
		items, err := todo.Read(root, sid)
		if err != nil {
			t.Fatalf("Read = error %v, want nil for a missing todos dir", err)
		}
		if items != nil {
			t.Errorf("Read = %v, want nil items", items)
		}
	})

	t.Run("missing root", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "nope", "not-here")
		items, err := todo.Read(root, sid)
		if err != nil {
			t.Fatalf("Read = error %v, want nil for a missing root", err)
		}
		if items != nil {
			t.Errorf("Read = %v, want nil items", items)
		}
	})

	t.Run("empty todos dir", func(t *testing.T) {
		root, _ := todosRoot(t)
		items, err := todo.Read(root, sid)
		if err != nil {
			t.Fatalf("Read = error %v, want nil", err)
		}
		if items != nil {
			t.Errorf("Read = %v, want nil items", items)
		}
	})

	t.Run("no file matches the session", func(t *testing.T) {
		root, dir := todosRoot(t)
		writeTodoFile(t, dir, other+"-agent.json", `[{"content":"someone else","status":"pending"}]`, 0)
		items, err := todo.Read(root, sid)
		if err != nil {
			t.Fatalf("Read = error %v, want nil", err)
		}
		if items != nil {
			t.Errorf("Read = %v, want nil items", items)
		}
	})

	t.Run("empty session id matches nothing in an empty dir", func(t *testing.T) {
		root, _ := todosRoot(t)
		if items, err := todo.Read(root, ""); err != nil || len(items) != 0 {
			t.Errorf("Read(root, \"\") = %v, %v; want no items and no error", items, err)
		}
	})
}

// An empty plan is a real, readable answer: no items, no error.
func TestT42EmptyArrayIsNotAnError(t *testing.T) {
	root, dir := todosRoot(t)
	writeTodoFile(t, dir, sid+"-agent.json", `[]`, 0)

	if items := mustRead(t, root, sid); len(items) != 0 {
		t.Errorf("got %d items, want none: %s", len(items), texts(items))
	}
}

// Read does not mind unknown fields: real todo files carry more than three.
func TestT42UnknownFieldsAreIgnored(t *testing.T) {
	root, dir := todosRoot(t)
	writeTodoFile(t, dir, sid+"-agent.json", `[
		{"content":"write parser","activeForm":"Writing parser","status":"pending",
		 "id":"t1","priority":"high","metadata":{"source":"plan"}}
	]`, 0)

	assertItems(t, mustRead(t, root, sid), todo.Item{Text: "write parser", Status: todo.Pending})
}

// Text is kept verbatim: no clipping, no case folding — the renderer decides.
func TestT42TextIsVerbatim(t *testing.T) {
	long := strings.Repeat("wire the ghost rail into the trail renderer ", 3)
	root, dir := todosRoot(t)
	writeTodoFile(t, dir, sid+"-agent.json",
		`[{"content":"`+long+`","status":"pending"},`+
			`{"content":"résumé the ✓ pass","status":"pending"}]`, 0)

	assertItems(t, mustRead(t, root, sid),
		todo.Item{Text: long, Status: todo.Pending},
		todo.Item{Text: "résumé the ✓ pass", Status: todo.Pending},
	)
}
