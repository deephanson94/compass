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
		"git -C " + cwd + " rev-parse --show-toplevel":   cwd + "\n",
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

var sessions = map[string][]string{"s1": {cwd}}

// one is a session's only pull request.
func one(sts []Status) (Status, bool) {
	if len(sts) == 0 {
		return Status{}, false
	}
	return sts[0], true
}

// only is one without the ok.
func only(sts []Status) Status { s, _ := one(sts); return s }

func TestPollLinksTheSessionToItsPR(t *testing.T) {
	f := gitea(t, fixture(t), "2026-10-22T10:36:56Z")
	tr := New(f, "work", time.Minute, "")
	if err := tr.Poll(sessions, now); err != nil {
		t.Fatal(err)
	}
	st, ok := one(tr.States(now)["s1"])
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
	if st, ok := one(tr.States(now)["s1"]); ok {
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
	// The searches — yours, and the two for reviews — and nothing per PR.
	if got := f.teaCalls() - before; got != 1+len(reviewPaths) {
		t.Fatalf("second poll made %d tea calls, want the searches alone: %v", got, f.calls[before:])
	}
	for _, c := range f.calls[before:] {
		if !strings.Contains(c, "/repos/issues/search?") {
			t.Fatalf("second poll made a per-PR call: %s", c)
		}
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
	st, ok := one(tr.States(now)["s1"])
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
	if only(tr.States(now)["s1"]).Stale {
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
	if got := only(tr.States(now)["s1"]).Line(now); got != "PR #1118 merged" {
		t.Fatalf("after the merge %q, want PR #1118 merged", got)
	}
	if err := tr.Poll(sessions, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if st, ok := one(tr.States(now)["s1"]); ok {
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
	if st, ok := one(next.States(now)["s1"]); !ok || st.Number != 1118 {
		t.Fatalf("the cache lost the PR: %+v", st)
	}
}

// twoRepos is a working dir that is no checkout, with repoA under it and
// repoB elsewhere on disk; each has a PR open for its branch. repoA's
// reviewer asked for changes; repoB waits on its reviewer.
func twoRepos(t *testing.T) (f *fakeRunner, work, repoA, repoB string) {
	t.Helper()
	work = t.TempDir()
	repoA = filepath.Join(work, "repoA")
	repoB = filepath.Join(t.TempDir(), "repoB")
	for _, d := range []string{filepath.Join(repoA, ".git"), filepath.Join(work, "notes"), filepath.Join(repoB, ".git")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	a := fixture(t)
	a.IssueComments = a.IssueComments[:1] // the reviewer spoke last: your move
	b := fixture(t)
	b.Pull.Number, b.Pull.Head.Ref = 77, "dev/feature-y"
	enc := func(v any) string {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	f = &fakeRunner{answers: map[string]string{
		"git -C " + repoA + " rev-parse --show-toplevel":   repoA + "\n",
		"git -C " + repoA + " rev-parse --abbrev-ref HEAD": "dev/feature-x\n",
		"git -C " + repoA + " remote get-url origin":       "git@gitea.example.com:org/repoA.git\n",
		"git -C " + repoB + " rev-parse --show-toplevel":   repoB + "\n",
		"git -C " + repoB + " rev-parse --abbrev-ref HEAD": "dev/feature-y\n",
		"git -C " + repoB + " remote get-url origin":       "https://gitea.example.com/org/repoB\n",
		"tea api /user": `{"login":"me"}`,
		"tea api " + searchPath: `[{"number":1118,"updated_at":"a","repository":{"full_name":"org/repoA"}},` +
			`{"number":77,"updated_at":"b","repository":{"full_name":"org/repoB"}}]`,
		"tea api /repos/org/repoa/pulls/1118":           enc(a.Pull),
		"tea api /repos/org/repoa/pulls/1118/reviews":   enc(a.Reviews),
		"tea api /repos/org/repoa/issues/1118/comments": enc(a.IssueComments),
		"tea api /repos/org/repob/pulls/77":             enc(b.Pull),
		"tea api /repos/org/repob/pulls/77/reviews":     enc(b.Reviews),
		"tea api /repos/org/repob/issues/77/comments":   enc(b.IssueComments),
	}}
	return f, work, repoA, repoB
}

// A session opened in a working dir above its repositories, which pushed
// to one under it and one elsewhere, has both pull requests: the one under
// its cwd found by looking, the other by where its tool calls worked.
func TestASessionAboveTwoReposHasBoth(t *testing.T) {
	f, work, _, repoB := twoRepos(t)
	tr := New(f, "", time.Minute, "")
	if err := tr.Poll(map[string][]string{"s1": {work, repoB}}, now); err != nil {
		t.Fatal(err)
	}
	sts := tr.States(now)["s1"]
	if len(sts) != 2 {
		t.Fatalf("got %d PRs, want 2: %+v", len(sts), sts)
	}
	if sts[0].Number != 1118 || sts[0].Move != Yours || sts[1].Number != 77 || sts[1].Move != Theirs {
		t.Fatalf("not most urgent first: %+v", sts)
	}
	if got, want := Summary(sts, now), "PR repoa#1118 · changes requested 2h ago · +1 PR"; got != want {
		t.Fatalf("summary %q, want %q", got, want)
	}
	if got := Segment(tr.States(now)); got != "PR▲1" {
		t.Fatalf("segment %q, want PR▲1", got)
	}
}

// Without the directories its tool calls worked in, only the repository
// under the cwd is found; without looking under the cwd, only the other.
func TestEachWayOfFindingARepoIsNeeded(t *testing.T) {
	f, work, _, repoB := twoRepos(t)
	tr := New(f, "", time.Minute, "")
	if err := tr.Poll(map[string][]string{"s1": {work}}, now); err != nil {
		t.Fatal(err)
	}
	if sts := tr.States(now)["s1"]; len(sts) != 1 || sts[0].Number != 1118 {
		t.Fatalf("the cwd alone: %+v, want repoA's #1118 found under it", sts)
	}
	if err := tr.Poll(map[string][]string{"s1": {"/elsewhere", repoB}}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if sts := tr.States(now)["s1"]; len(sts) != 1 || sts[0].Number != 77 {
		t.Fatalf("a worked-in dir: %+v, want repoB's #77", sts)
	}
}

// A pull request two sessions share counts once on the status line.
func TestSegmentCountsASharedPROnce(t *testing.T) {
	st := Status{Repo: "org/repo", Number: 1, Move: Yours}
	if got := Segment(map[string][]Status{"a": {st}, "b": {st}}); got != "PR▲1" {
		t.Fatalf("got %q, want PR▲1", got)
	}
}

// asReviewer is the fixture as its reviewer sees it: alice opened it,
// reviewer1 asked for changes and commented, alice pushed and replied.
func asReviewer(t *testing.T) Detail {
	t.Helper()
	d := fixture(t)
	d.Pull.User.Login = "alice"
	d.Pull.CreatedAt = at("2026-10-21T09:00:00Z")
	for i := range d.IssueComments {
		if d.IssueComments[i].User.Login == "me" {
			d.IssueComments[i].User.Login = "alice"
		}
	}
	return d
}

func TestReviewAuthorRepliedIsYourMove(t *testing.T) {
	s, ok := JudgeReview(asReviewer(t), "reviewer1")
	if !ok || s.Move != Yours || s.Reason != "alice replied" {
		t.Fatalf("got %+v %v, want your move, alice replied", s, ok)
	}
	if got, want := s.Line(now), "review PR #1118 · alice replied 20m ago"; got != want {
		t.Fatalf("line %q, want %q", got, want)
	}
}

func TestReviewMyWordLastIsTheirMove(t *testing.T) {
	d := asReviewer(t)
	d.IssueComments = d.IssueComments[:1] // alice has not replied
	s, ok := JudgeReview(d, "reviewer1")
	if !ok || s.Move != Theirs || s.Who != "alice" {
		t.Fatalf("got %+v, want their move, waiting on alice", s)
	}
	if got, want := s.Line(now), "review PR #1118 · waiting on alice · 2h"; got != want {
		t.Fatalf("line %q, want %q", got, want)
	}
}

func TestReviewAuthorPushedIsYourMove(t *testing.T) {
	d := asReviewer(t)
	d.IssueComments = d.IssueComments[:1]
	d.HeadAt = at("2026-10-22T09:30:00Z")
	if s, _ := JudgeReview(d, "reviewer1"); s.Move != Yours || s.Reason != "alice pushed" {
		t.Fatalf("got %+v, want your move, alice pushed", s)
	}
}

func TestReviewRequestedAndNotGivenIsYourMove(t *testing.T) {
	d := asReviewer(t)
	d.Reviews, d.IssueComments = nil, nil
	s, ok := JudgeReview(d, "reviewer1")
	if !ok || s.Move != Yours || s.Reason != "review requested" || !s.At.Equal(d.Pull.CreatedAt) {
		t.Fatalf("got %+v, want your move, review requested since it opened", s)
	}
	d.Pull.RequestedReviewers = nil // nobody asked and I said nothing: not mine
	if _, ok := JudgeReview(d, "reviewer1"); ok {
		t.Fatal("a PR I was never asked to review and never touched is mine to review")
	}
}

func TestReviewMyApprovalIsTheirMove(t *testing.T) {
	d := asReviewer(t)
	d.Reviews = append(d.Reviews, Review{User: user{"reviewer1"}, State: "APPROVED", SubmittedAt: at("2026-10-22T10:45:00Z")})
	s, _ := JudgeReview(d, "reviewer1")
	if s.Move != Theirs || s.Reason != "you approved" {
		t.Fatalf("got %+v, want their move, you approved", s)
	}
	if got, want := s.Line(now), "review PR #1118 · you approved · waiting on alice · 11m"; got != want {
		t.Fatalf("line %q, want %q", got, want)
	}
}

func TestReviewAnotherReviewerDoesNotMoveMyTurn(t *testing.T) {
	d := asReviewer(t)
	d.IssueComments = append(d.IssueComments[:1], Comment{User: user{"reviewer2"}, CreatedAt: at("2026-10-22T10:50:00Z")})
	if s, _ := JudgeReview(d, "reviewer1"); s.Move != Theirs {
		t.Fatalf("reviewer2's comment moved my turn: %+v", s)
	}
}

// reviewGitea lists, besides my own #1118, alice's #2040 in another
// repository, which waits on my review and which no session works on.
func reviewGitea(t *testing.T) *fakeRunner {
	t.Helper()
	f := gitea(t, fixture(t), "x")
	r := asReviewer(t)
	r.Pull.Number, r.Pull.Head.Ref = 2040, "alice/fix"
	// From me's side: reviewer1 is someone else; make me the reviewer.
	for i := range r.Reviews {
		r.Reviews[i].User.Login = "me"
	}
	for i := range r.IssueComments {
		if r.IssueComments[i].User.Login == "reviewer1" {
			r.IssueComments[i].User.Login = "me"
		}
	}
	enc := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	f.answers["tea api "+reviewPaths[0]] = `[]`
	f.answers["tea api "+reviewPaths[1]] = `[{"number":2040,"updated_at":"r","repository":{"full_name":"org/other"}},` +
		`{"number":1118,"updated_at":"x","repository":{"full_name":"org/repo"}}]`
	f.answers["tea api /repos/org/other/pulls/2040"] = enc(r.Pull)
	f.answers["tea api /repos/org/other/pulls/2040/reviews"] = enc(r.Reviews)
	f.answers["tea api /repos/org/other/issues/2040/comments"] = enc(r.IssueComments)
	return f
}

// A pull request you review is tracked whether or not a session works on
// it: the status line counts it, and a session on its branch shows it.
func TestReviewsAreTrackedWithoutASession(t *testing.T) {
	f := reviewGitea(t)
	tr := New(f, "", time.Minute, "")
	if err := tr.Poll(sessions, now); err != nil {
		t.Fatal(err)
	}
	rv := tr.Reviews(now)
	if len(rv) != 1 || rv[0].Number != 2040 || rv[0].Move != Yours || rv[0].Reason != "alice replied" {
		t.Fatalf("reviews %+v, want alice's #2040 waiting on me", rv)
	}
	if got := ReviewSegment(rv); got != "RV▲1" {
		t.Fatalf("segment %q, want RV▲1", got)
	}
	// My own #1118, which the reviewed search also lists, is judged as mine.
	if st, _ := one(tr.States(now)["s1"]); st.Number != 1118 || st.Review {
		t.Fatalf("session's PR %+v, want my own #1118", st)
	}
	// A session that checks alice's branch out shows the review.
	f.answers["git -C /work/other rev-parse --show-toplevel"] = "/work/other\n"
	f.answers["git -C /work/other rev-parse --abbrev-ref HEAD"] = "alice/fix\n"
	f.answers["git -C /work/other remote get-url origin"] = "git@gitea.example.com:org/other.git\n"
	if err := tr.Poll(map[string][]string{"s2": {"/work/other"}}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if st, _ := one(tr.States(now)["s2"]); st.Number != 2040 || !st.Review {
		t.Fatalf("the reviewing session shows %+v, want review #2040", st)
	}
}

// A Gitea that does not know the review searches costs nothing but them:
// your own pull requests are fresh, and the reviews already known stay.
func TestAFailedReviewSearchKeepsTheRest(t *testing.T) {
	f := reviewGitea(t)
	tr := New(f, "", time.Minute, "")
	if err := tr.Poll(sessions, now); err != nil {
		t.Fatal(err)
	}
	delete(f.answers, "tea api "+reviewPaths[1])
	if err := tr.Poll(sessions, now.Add(time.Minute)); err != nil {
		t.Fatalf("a failed review search failed the poll: %v", err)
	}
	if st, _ := one(tr.States(now)["s1"]); st.Stale {
		t.Fatal("a failed review search marked my own PR stale")
	}
	if rv := tr.Reviews(now); len(rv) != 1 {
		t.Fatalf("a failed review search dropped the reviews: %+v", rv)
	}
	if r := tr.Report(); len(r.Errors) == 0 {
		t.Fatal("the diagnostic does not say the review search failed")
	}
}

// Your own pull request, listed by the reviewed search where no session
// works in its repository, is not one you review.
func TestMyOwnPRIsNoReview(t *testing.T) {
	tr := New(reviewGitea(t), "", time.Minute, "")
	if err := tr.Poll(map[string][]string{}, now); err != nil {
		t.Fatal(err)
	}
	for _, st := range tr.Reviews(now) {
		if st.Number == 1118 {
			t.Fatalf("my own #1118 is counted as a review: %+v", st)
		}
	}
}
