package fleet

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/deephanson94/compass/internal/transcript"
)

func toolEvent(t *testing.T, name string, input any) transcript.Event {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return transcript.Event{Type: transcript.EventAssistant, CWD: "/work",
		ToolUses: []transcript.ToolUse{{Name: name, Input: raw}}}
}

// Where a session's tool calls worked: the directory of a file it edited,
// and every directory its shell moved into or pointed git at.
func TestWorkDirs(t *testing.T) {
	home, _ := os.UserHomeDir()
	for _, c := range []struct {
		name  string
		input any
		want  []string
	}{
		{"Edit", map[string]string{"file_path": "/src/repoA/main.go"}, []string{"/src/repoA"}},
		{"Write", map[string]string{"file_path": "/src/repoB/pkg/x.go"}, []string{"/src/repoB/pkg"}},
		{"Bash", map[string]string{"command": "cd repoA && git push"}, []string{"/work/repoA"}},
		{"Bash", map[string]string{"command": `git -C "/src/repo B" push origin HEAD; cd ../other`}, []string{"/src/repo B", "/other"}},
		{"Bash", map[string]string{"command": "cd ~/code/repoC"}, []string{filepath.Join(home, "code/repoC")}},
		{"Bash", map[string]string{"command": "cd $REPO && make"}, nil},
		{"Bash", map[string]string{"command": "echo cdrom"}, nil},
		{"Read", map[string]string{"file_path": "/src/repoD/README"}, nil}, // reading is not working there
	} {
		got := workDirs(toolEvent(t, c.name, c.input), "")
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s %v: got %q, want %q", c.name, c.input, got, c.want)
		}
	}
	ev := toolEvent(t, "Edit", map[string]string{"file_path": "/src/repoA/main.go"})
	ev.IsSidechain = true
	if got := workDirs(ev, ""); got != nil {
		t.Errorf("a subagent's edit counted: %q", got)
	}
}

// The list keeps the most recent last, once each, and no more than its cap.
func TestNoteWorkDirs(t *testing.T) {
	got := noteWorkDirs([]string{"/a", "/b"}, "/a", "/c")
	if want := []string{"/b", "/a", "/c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	var many []string
	for i := 0; i < workDirsMax+5; i++ {
		many = noteWorkDirs(many, filepath.Join("/d", string(rune('a'+i%26)), string(rune('a'+i/26))))
	}
	if len(many) != workDirsMax {
		t.Fatalf("kept %d, want %d", len(many), workDirsMax)
	}
}

// What the session worked in survives the resume cache: a fresh `compass
// status` picks the session up past those events and still knows them.
func TestWorkDirsRideTheFold(t *testing.T) {
	e := &entry{dirs: []string{"/src/repoA"}}
	var back entry
	back.restore(e.fold())
	if !reflect.DeepEqual(back.dirs, e.dirs) {
		t.Fatalf("restored %q, want %q", back.dirs, e.dirs)
	}
}

// A pull request named by address — in a prompt, a command, a fetch — is
// one the session was pointed at.
func TestPRRefs(t *testing.T) {
	prompt := transcript.Event{Type: transcript.EventUser,
		Text: "review Tim's PR https://gitea-oa.example.inc/cai2_spt3_alg3/alg3_tools/pulls/4 please"}
	if got, want := prRefs(prompt), []string{"cai2_spt3_alg3/alg3_tools#4"}; !reflect.DeepEqual(got, want) {
		t.Errorf("prompt: got %q, want %q", got, want)
	}
	for _, c := range []struct {
		name  string
		input any
		want  []string
	}{
		{"Bash", map[string]string{"command": "tea api --login work /repos/Org/Repo/pulls/7/reviews"}, []string{"org/repo#7"}},
		{"WebFetch", map[string]string{"url": "https://github.com/o/r/pull/3"}, []string{"o/r#3"}},
		{"Bash", map[string]string{"command": "curl https://host/git/o/r/pulls/12/files"}, []string{"o/r#12"}},
		{"Bash", map[string]string{"command": "git log --oneline -5"}, nil},
	} {
		if got := prRefs(toolEvent(t, c.name, c.input)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s %v: got %q, want %q", c.name, c.input, got, c.want)
		}
	}
	meta := prompt
	meta.IsMeta = true
	if got := prRefs(meta); got != nil {
		t.Errorf("a harness line named %q", got)
	}
	side := toolEvent(t, "Bash", map[string]string{"command": "tea api /repos/o/r/pulls/1"})
	side.IsSidechain = true
	if got := prRefs(side); got != nil {
		t.Errorf("a subagent's call named %q", got)
	}
	e := &entry{prs: []string{"o/r#4"}}
	var back entry
	back.restore(e.fold())
	if !reflect.DeepEqual(back.prs, e.prs) {
		t.Errorf("the fold lost the named PRs: %q", back.prs)
	}
}
