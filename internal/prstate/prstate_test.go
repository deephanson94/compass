package prstate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// now is twenty minutes after the fixture's last word.
var now = time.Date(2026, 10, 22, 10, 56, 56, 0, time.UTC)

// fixture is PR #1118 as Gitea returned it: the reviewer asked for changes
// and commented, then I pushed and replied.
func fixture(t *testing.T) Detail {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "pr1118.json"))
	if err != nil {
		t.Fatal(err)
	}
	var d Detail
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestJudgeFixtureIsTheirMove(t *testing.T) {
	d := fixture(t)
	if !d.Reviews[0].Stale {
		t.Fatal("the fixture's review is stale: commits landed after it")
	}
	s := Judge(d, "me")
	if s.Move != Theirs || s.Who != "reviewer1" {
		t.Fatalf("got %+v, want their move waiting on reviewer1", s)
	}
	if got, want := s.Line(now), "PR #1118 · waiting on reviewer1 · 20m"; got != want {
		t.Fatalf("line %q, want %q", got, want)
	}
}

func TestJudgeWithoutMyReplyIsYourMove(t *testing.T) {
	d := fixture(t)
	d.IssueComments = d.IssueComments[:1] // my last comment, gone
	s := Judge(d, "me")
	if s.Move != Yours || s.Reason != "changes requested" {
		t.Fatalf("got %+v, want your move, changes requested", s)
	}
	if got, want := s.Line(now), "PR #1118 · changes requested 2h ago"; got != want {
		t.Fatalf("line %q, want %q", got, want)
	}
}

func TestJudgeApprovalAfterMyReplyIsReadyToMerge(t *testing.T) {
	d := fixture(t)
	d.Reviews = append(d.Reviews, Review{ID: 73200, User: user{"reviewer1"}, State: "APPROVED",
		SubmittedAt: at("2026-10-22T10:50:00Z")})
	s := Judge(d, "me")
	if s.Move != Yours || s.Reason != Ready {
		t.Fatalf("got %+v, want your move, %s", s, Ready)
	}
	if got, want := s.Line(now), "PR #1118 · approved — ready to merge"; got != want {
		t.Fatalf("line %q, want %q", got, want)
	}
}

func TestJudgeApprovalThenMyCommentStaysReady(t *testing.T) {
	// What is left after an approval is merging, whoever spoke last.
	d := fixture(t)
	d.Reviews = append(d.Reviews, Review{User: user{"reviewer1"}, State: "APPROVED", SubmittedAt: at("2026-10-22T09:00:00Z")})
	if s := Judge(d, "me"); s.Reason != Ready {
		t.Fatalf("got %+v, want %s", s, Ready)
	}
}

func TestJudgeStaleReviewMeansIPushedAfterIt(t *testing.T) {
	// The reviewer asked for changes and said nothing since; commits landed
	// after the review. The push is the newest word, and it is mine.
	d := fixture(t)
	d.IssueComments = nil
	if s := Judge(d, "me"); s.Move != Theirs {
		t.Fatalf("a stale review left it %+v, want their move", s)
	}
	d.Reviews[0].Stale = false // nothing pushed since: the reviewer's word stands
	if s := Judge(d, "me"); s.Move != Yours || s.Reason != "changes requested" {
		t.Fatalf("got %+v, want your move, changes requested", s)
	}
}

func TestJudgePendingReviewIsIgnored(t *testing.T) {
	d := fixture(t)
	d.Reviews = append(d.Reviews, Review{User: user{"reviewer1"}, State: "PENDING", SubmittedAt: at("2026-10-22T10:50:00Z")})
	if s := Judge(d, "me"); s.Move != Theirs {
		t.Fatalf("an unsubmitted review moved the turn: %+v", s)
	}
}

func TestJudgeNewCommentsCount(t *testing.T) {
	d := fixture(t)
	d.Reviews = nil
	d.IssueComments = append(d.IssueComments,
		Comment{User: user{"reviewer1"}, CreatedAt: at("2026-10-22T10:40:00Z")},
		Comment{User: user{"reviewer2"}, CreatedAt: at("2026-10-22T10:41:00Z")})
	s := Judge(d, "me")
	if s.Move != Yours || s.Reason != "2 new comments" {
		t.Fatalf("got %+v, want your move, 2 new comments", s)
	}
}

func TestJudgePushAfterCommentIsTheirMove(t *testing.T) {
	d := fixture(t)
	d.IssueComments = d.IssueComments[:1]
	d.HeadAt = at("2026-10-22T09:00:00Z") // pushed after the reviewer's comment
	if s := Judge(d, "me"); s.Move != Theirs {
		t.Fatalf("a push after the reviewer's word left it %+v", s)
	}
}

func TestJudgeMergedAndClosed(t *testing.T) {
	d := fixture(t)
	d.Pull.Merged = true
	if s := Judge(d, "me"); s.Move != Merged || s.Line(now) != "PR #1118 merged" {
		t.Fatalf("got %+v %q", s, s.Line(now))
	}
	d = fixture(t)
	d.Pull.State = "closed"
	if s := Judge(d, "me"); s.Move != Closed || s.Line(now) != "PR #1118 closed" {
		t.Fatalf("got %+v %q", s, s.Line(now))
	}
}

func TestParseRemote(t *testing.T) {
	for _, c := range []struct{ in, host, repo string }{
		{"git@gitea.example.com:Org/Repo.git", "gitea.example.com", "org/repo"},
		{"ssh://git@gitea.example.com:2222/org/repo.git", "gitea.example.com", "org/repo"},
		{"https://gitea.example.com/org/repo", "gitea.example.com", "org/repo"},
		{"https://gitea.example.com/git/org/repo.git/", "gitea.example.com", "org/repo"},
	} {
		host, repo, ok := ParseRemote(c.in)
		if !ok || host != c.host || repo != c.repo {
			t.Errorf("%s → %s %s %v, want %s %s", c.in, host, repo, ok, c.host, c.repo)
		}
	}
	for _, bad := range []string{"", "/srv/git/repo", "https://host/onlyone"} {
		if _, _, ok := ParseRemote(bad); ok {
			t.Errorf("%q parsed", bad)
		}
	}
}

// fakeRunner answers git and tea from a table and counts the tea calls.
type fakeRunner struct {
	answers map[string]string // "tea api /path" (the --login pair dropped) or "git …" → stdout
	fail    error             // every tea call fails with this, when set
	calls   []string
}

func (f *fakeRunner) Output(name string, args ...string) ([]byte, error) {
	if name == "tea" {
		var kept []string
		for i := 0; i < len(args); i++ {
			if args[i] == "--login" {
				i++
				continue
			}
			kept = append(kept, args[i])
		}
		args = kept
	}
	key := name + " " + strings.Join(args, " ")
	if name == "tea" {
		f.calls = append(f.calls, key)
		if f.fail != nil {
			return nil, f.fail
		}
	}
	out, ok := f.answers[key]
	if !ok {
		return nil, errors.New("no answer for " + key)
	}
	return []byte(out), nil
}

func (f *fakeRunner) teaCalls() int { return len(f.calls) }

const cwd = "/work/repo"

// gitea builds a fake whose search lists the fixture's PR with updated.
func gitea(t *testing.T, d Detail, updated string) *fakeRunner {
	t.Helper()
	enc := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	f := &fakeRunner{answers: map[string]string{
		"git -C " + cwd + " rev-parse --abbrev-ref HEAD": "dev/feature-x\n",
		"git -C " + cwd + " remote get-url origin":       "git@gitea.example.com:org/repo.git\n",
		"tea api /user":                                `{"login":"me"}`,
		"tea api " + searchPath:                        `[{"number":1118,"updated_at":"` + updated + `","repository":{"full_name":"org/repo"}}]`,
		"tea api /repos/org/repo/pulls/1118":           enc(d.Pull),
		"tea api /repos/org/repo/pulls/1118/reviews":   enc(d.Reviews),
		"tea api /repos/org/repo/issues/1118/comments": enc(d.IssueComments),
	}}
	return f
}

var sessions = map[string]string{"s1": cwd}

func TestPollLinksTheSessionToItsPR(t *testing.T) {
	f := gitea(t, fixture(t), "2026-10-22T10:36:56Z")
	tr := New(f, "work", time.Minute, "")
	if err := tr.Poll(sessions, now); err != nil {
		t.Fatal(err)
	}
	st, ok := tr.States(now)["s1"]
	if !ok {
		t.Fatal("no PR for the session")
	}
	if got, want := st.Line(now), "PR #1118 · waiting on reviewer1 · 20m"; got != want {
		t.Fatalf("line %q, want %q", got, want)
	}
}

func TestPollPassesTheLogin(t *testing.T) {
	f := gitea(t, fixture(t), "x")
	var seen []string
	wrapped := runnerFunc(func(name string, args ...string) ([]byte, error) {
		if name == "tea" {
			seen = append(seen, strings.Join(args, " "))
		}
		return f.Output(name, args...)
	})
	if err := New(wrapped, "work", time.Minute, "").Poll(sessions, now); err != nil {
		t.Fatal(err)
	}
	for _, s := range seen {
		if !strings.HasPrefix(s, "api --login work ") {
			t.Fatalf("tea called without the login: %q", s)
		}
	}
}

type runnerFunc func(name string, args ...string) ([]byte, error)

func (r runnerFunc) Output(name string, args ...string) ([]byte, error) { return r(name, args...) }

func TestPollNoMatchingBranchShowsNothing(t *testing.T) {
	f := gitea(t, fixture(t), "x")
	f.answers["git -C "+cwd+" rev-parse --abbrev-ref HEAD"] = "dev/other\n"
	tr := New(f, "", time.Minute, "")
	if err := tr.Poll(sessions, now); err != nil {
		t.Fatal(err)
	}
	if st, ok := tr.States(now)["s1"]; ok {
		t.Fatalf("a branch with no PR shows %q", st.Line(now))
	}
}

func TestPollSkipsDetailWhenNothingMoved(t *testing.T) {
	f := gitea(t, fixture(t), "2026-10-22T10:36:56Z")
	tr := New(f, "", time.Minute, "")
	if err := tr.Poll(sessions, now); err != nil {
		t.Fatal(err)
	}
	before := f.teaCalls()
	if err := tr.Poll(sessions, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got := f.teaCalls() - before; got != 1 {
		t.Fatalf("second poll made %d tea calls, want the search alone: %v", got, f.calls[before:])
	}
	// And when it moves, the detail is read again.
	f.answers["tea api "+searchPath] = strings.Replace(f.answers["tea api "+searchPath], "10:36:56", "10:50:00", 1)
	before = f.teaCalls()
	if err := tr.Poll(sessions, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got := f.teaCalls() - before; got < 4 {
		t.Fatalf("a moved PR made %d calls, want its detail read", got)
	}
}

func TestPollErrorKeepsTheLastStateAndMarksItStale(t *testing.T) {
	f := gitea(t, fixture(t), "x")
	tr := New(f, "", time.Minute, "")
	if err := tr.Poll(sessions, now); err != nil {
		t.Fatal(err)
	}
	f.fail = errors.New("connection refused")
	if err := tr.Poll(sessions, now.Add(time.Minute)); err == nil {
		t.Fatal("a failed search reported no error")
	}
	st, ok := tr.States(now)["s1"]
	if !ok {
		t.Fatal("a failed poll erased the PR")
	}
	if !st.Stale || !strings.HasSuffix(st.Line(now), "· PR state stale") {
		t.Fatalf("got %q, want the last state marked stale", st.Line(now))
	}
	if st.Who != "reviewer1" {
		t.Fatalf("the state kept is not the last one: %+v", st)
	}
	// The next good poll clears it.
	f.fail = nil
	if err := tr.Poll(sessions, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if tr.States(now)["s1"].Stale {
		t.Fatal("a good poll left the state stale")
	}
}

func TestPollMergedShowsOnceThenGoes(t *testing.T) {
	d := fixture(t)
	f := gitea(t, d, "x")
	tr := New(f, "", time.Minute, "")
	if err := tr.Poll(sessions, now); err != nil {
		t.Fatal(err)
	}
	// Merged: the open search no longer lists it.
	f.answers["tea api "+searchPath] = `[]`
	d.Pull.Merged, d.Pull.State = true, "closed"
	b, _ := json.Marshal(d.Pull)
	f.answers["tea api /repos/org/repo/pulls/1118"] = string(b)
	if err := tr.Poll(sessions, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got := tr.States(now)["s1"].Line(now); got != "PR #1118 merged" {
		t.Fatalf("after the merge %q, want PR #1118 merged", got)
	}
	if err := tr.Poll(sessions, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if st, ok := tr.States(now)["s1"]; ok {
		t.Fatalf("the merge is still shown a poll later: %q", st.Line(now))
	}
}

func TestNoTeaIsOff(t *testing.T) {
	f := &fakeRunner{fail: errNoTea, answers: map[string]string{}}
	tr := New(f, "", time.Minute, "")
	_ = tr.Poll(sessions, now)
	if tr.Off() == "" || len(tr.States(now)) != 0 {
		t.Fatalf("no tea: off %q, states %v", tr.Off(), tr.States(now))
	}
	if got := Probe(f, "", time.Minute); !strings.Contains(got, "off — tea is not on PATH") {
		t.Fatalf("probe says %q", got)
	}
}

func TestIntervalZeroIsOff(t *testing.T) {
	f := gitea(t, fixture(t), "x")
	tr := New(f, "", 0, "")
	tr.Kick(sessions, now)
	if tr.Due(now) || f.teaCalls() != 0 || tr.States(now) != nil {
		t.Fatal("-pr-interval 0 polled")
	}
}

func TestCacheCarriesStateToTheNextProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prs.json")
	f := gitea(t, fixture(t), "x")
	if err := New(f, "", time.Minute, path).Poll(sessions, now); err != nil {
		t.Fatal(err)
	}
	next := New(f, "", time.Minute, path)
	if next.Due(now.Add(30 * time.Second)) {
		t.Fatal("a fresh process polls again inside the interval")
	}
	if st, ok := next.States(now)["s1"]; !ok || st.Number != 1118 {
		t.Fatalf("the cache lost the PR: %+v", st)
	}
}
