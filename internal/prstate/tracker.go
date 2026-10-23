package prstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultInterval is how often the search runs: one call covers every pull
// request you opened, and the detail of one is fetched only when it moved.
const DefaultInterval = 5 * time.Minute

// slowPoll is how long a poll may run before the rows it would refresh say
// they are the last state known.
const slowPoll = 30 * time.Second

// searchPath is the one call a poll always makes: every open pull request
// the login opened, across every repository.
const searchPath = "/repos/issues/search?type=pulls&created=true&state=open&limit=50"

// reviewPaths are the searches for other people's pull requests you review:
// the ones a review is requested of you on, and the ones you reviewed —
// Gitea can drop you from the requested list once you have.
var reviewPaths = []string{
	"/repos/issues/search?type=pulls&review_requested=true&state=open&limit=50",
	"/repos/issues/search?type=pulls&reviewed=true&state=open&limit=50",
}

// searchItem is one hit of the search.
type searchItem struct {
	Number     int    `json:"number"`
	UpdatedAt  string `json:"updated_at"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

// pr is one pull request as last seen.
type pr struct {
	Repo      string `json:"repo"`
	Number    int    `json:"number"`
	UpdatedAt string `json:"updated_at"` // the search's own word, compared as given
	HeadRef   string `json:"head_ref"`
	Status    Status `json:"status"`
	// Done is a merge or close found on the last poll: shown on that poll's
	// rows, dropped by the next one.
	Done bool `json:"done"`
	// Ignore is a pull request a review search listed that is not yours to
	// review after all — yours, or one you never touched. Kept so it is not
	// fetched again until it moves; never shown.
	Ignore bool `json:"ignore,omitempty"`
	// MissingAt is when a pull request a session names could not be read —
	// no such pull request, or none this login may see. It is not asked
	// again for missingRetry.
	MissingAt time.Time `json:"missing_at,omitzero"`
}

// missingRetry is how long a named pull request that could not be read is
// left alone: a number that is an issue, a typo, a repository out of reach.
const missingRetry = time.Hour

func prKey(repo string, n int) string { return fmt.Sprintf("%s#%d", repo, n) }

// Tracker keeps every session's pull request. Poll asks Gitea; States
// answers from what the last poll learned and never waits on the network.
// It is safe for concurrent use.
type Tracker struct {
	run      Runner
	login    string
	interval time.Duration
	path     string // the cache; "" keeps nothing between processes

	mu        sync.Mutex
	me        string
	lastPoll  time.Time
	polling   bool
	pollStart time.Time
	linking   bool
	stale     bool
	off       string   // why there is nothing to show; "" while it works
	errs      []string // what went wrong on the last poll, for the diagnostic
	found     [3]int   // what the last poll's searches listed: authored, review requested, reviewed
	unlinked  []string // your own open PRs the last poll skipped: no session works in their repository
	prs       map[string]*pr
	links     map[string][]link   // session key → the checkouts it works in
	cwds      map[string]string   // session key → the dirs its links were read from, joined
	refs      map[string][]string // session key → the pull requests it was pointed at, "owner/repo#n"
	refPolled time.Time           // when a poll last ran for a ref no poll had looked up
}

// Work is where one session works: the directories (its cwd first, then
// wherever its tool calls worked — fleet.Session.Dirs) and the pull
// requests it was pointed at by address (fleet.Session.PRs, "owner/repo#n").
type Work struct {
	Dirs []string
	PRs  []string
}

func dirsOf(work map[string]Work) map[string][]string {
	out := make(map[string][]string, len(work))
	for key, w := range work {
		out[key] = w.Dirs
	}
	return out
}

func refsOf(work map[string]Work) map[string][]string {
	out := map[string][]string{}
	for key, w := range work {
		if len(w.PRs) > 0 {
			out[key] = append([]string(nil), w.PRs...)
		}
	}
	return out
}

// refPollEvery is how soon after the last poll a pull request named for the
// first time is looked up: pasting an address should not wait five minutes.
const refPollEvery = 30 * time.Second

// cacheFile is what a Tracker writes down for the next process.
type cacheFile struct {
	Me       string              `json:"me"`
	LastPoll time.Time           `json:"last_poll"`
	Stale    bool                `json:"stale"`
	Off      string              `json:"off,omitempty"`
	PRs      []*pr               `json:"prs"`
	Links    map[string][]link   `json:"links"`
	CWDs     map[string]string   `json:"cwds"`
	Refs     map[string][]string `json:"refs,omitempty"`
}

// New returns a Tracker that polls through run as login ("" is tea's own
// default login) every interval, and keeps its state at path. A missing or
// unreadable cache is a fresh start.
func New(run Runner, login string, interval time.Duration, path string) *Tracker {
	t := &Tracker{run: run, login: login, interval: interval, path: path,
		prs: map[string]*pr{}, links: map[string][]link{}, cwds: map[string]string{}, refs: map[string][]string{}}
	if path == "" {
		return t
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return t
	}
	var f cacheFile
	if json.Unmarshal(raw, &f) != nil {
		return t
	}
	t.me, t.lastPoll, t.stale, t.off = f.Me, f.LastPoll, f.Stale, f.Off
	for _, p := range f.PRs {
		if p != nil {
			t.prs[prKey(p.Repo, p.Number)] = p
		}
	}
	if f.Links != nil {
		t.links = f.Links
	}
	if f.CWDs != nil {
		t.cwds = f.CWDs
	}
	if f.Refs != nil {
		t.refs = f.Refs
	}
	return t
}

// Interval is how often the tracker polls; 0 is off.
func (t *Tracker) Interval() time.Duration { return t.interval }

// Due says a poll is owed at now.
func (t *Tracker) Due(now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.interval > 0 && !t.polling && now.Sub(t.lastPoll) >= t.interval
}

// Kick starts a poll in the background when one is due, and otherwise links
// any session it has not seen in the background — git only, no network — so
// a session opened between polls finds a pull request the last poll already
// knows. It returns at once.
//
// A pull request a session names for the first time is looked up within
// refPollEvery, not at the next interval.
func (t *Tracker) Kick(work map[string]Work, now time.Time) {
	dirs := dirsOf(work)
	t.mu.Lock()
	if t.interval <= 0 {
		t.mu.Unlock()
		return
	}
	t.refs = refsOf(work) // no network: the names are the transcript's
	newRef := false
	for _, rs := range t.refs {
		for _, r := range rs {
			if t.prs[r] == nil {
				newRef = true
			}
		}
	}
	early := newRef && now.Sub(t.lastPoll) >= refPollEvery && now.Sub(t.refPolled) >= refPollEvery
	if !t.polling && (now.Sub(t.lastPoll) >= t.interval || early) {
		if early {
			t.refPolled = now
		}
		t.polling, t.pollStart = true, now
		t.mu.Unlock()
		go func() {
			_ = t.Poll(work, now)
			t.mu.Lock()
			t.polling = false
			t.mu.Unlock()
		}()
		return
	}
	fresh := map[string][]string{}
	if !t.linking {
		for key, ds := range dirs {
			if prev, ok := t.cwds[key]; !ok || prev != joinDirs(ds) {
				fresh[key] = ds
			}
		}
	}
	if len(fresh) == 0 {
		t.mu.Unlock()
		return
	}
	t.linking = true
	t.mu.Unlock()
	go func() {
		links := t.linkAll(fresh)
		t.mu.Lock()
		for key, ds := range fresh {
			t.cwds[key] = joinDirs(ds)
			if l, ok := links[key]; ok {
				t.links[key] = l
			} else {
				delete(t.links, key)
			}
		}
		t.linking = false
		t.mu.Unlock()
	}()
}

// PollIfDue polls now if a poll is owed, and waits for it: `compass status`
// is a fresh process, and tmux keeps the last line on the bar while it runs.
func (t *Tracker) PollIfDue(work map[string]Work, now time.Time) {
	if t.Due(now) {
		_ = t.Poll(work, now)
	}
}

func joinDirs(ds []string) string { return strings.Join(ds, "\n") }

// childReposMax bounds how many folders under a cwd are looked at.
const childReposMax = 64

// childRepos is the folders directly under dir that are git checkouts: a
// session opened in a working dir above its repositories works in them
// without its cwd naming any.
func childRepos(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		child := filepath.Join(dir, e.Name())
		if _, err := os.Stat(filepath.Join(child, ".git")); err == nil {
			out = append(out, child)
			if len(out) == childReposMax {
				break
			}
		}
	}
	return out
}

// linkAll reads the checkouts each session works in: its cwd, the folders
// under its cwd that are checkouts, and every other directory its tool
// calls worked in. One git trio per checkout, however many sessions and
// directories lead to it; a repository and branch is listed once.
func (t *Tracker) linkAll(dirs map[string][]string) map[string][]link {
	byDir := map[string]*link{}
	resolve := func(d string) *link {
		l, seen := byDir[d]
		if !seen {
			if got, ok := linkOf(t.run, d); ok {
				l = &got
			}
			byDir[d] = l
		}
		return l
	}
	out := map[string][]link{}
	for key, ds := range dirs {
		var ls []link
		have := map[link]bool{}
		for i, d := range ds {
			cands := []string{d}
			if i == 0 {
				cands = append(cands, childRepos(d)...)
			}
			for _, c := range cands {
				if l := resolve(c); l != nil && !have[*l] {
					have[*l] = true
					ls = append(ls, *l)
				}
			}
		}
		if len(ls) > 0 {
			out[key] = ls
		}
	}
	return out
}

// Poll links every session to its checkout, runs the search, and fetches
// the detail of each pull request a session's repository has open whose
// updated_at moved since the last poll. A failed call keeps the state it
// would have replaced and marks it stale; the error is the first one met.
func (t *Tracker) Poll(work map[string]Work, now time.Time) error {
	dirs, refs := dirsOf(work), refsOf(work)
	links := t.linkAll(dirs)
	repos := map[string]bool{}
	for _, ls := range links {
		for _, l := range ls {
			repos[l.Repo] = true
		}
	}
	named := map[string]bool{} // the pull requests some session names
	for _, rs := range refs {
		for _, r := range rs {
			named[r] = true
		}
	}
	sigs := make(map[string]string, len(dirs))
	for key, ds := range dirs {
		sigs[key] = joinDirs(ds)
	}

	t.mu.Lock()
	me := t.me
	prev := make(map[string]*pr, len(t.prs))
	for k, p := range t.prs {
		prev[k] = p
	}
	t.links, t.cwds, t.refs = links, sigs, refs
	t.mu.Unlock()

	finish := func(next map[string]*pr, stale bool, off string, err error) error {
		t.mu.Lock()
		if next != nil {
			t.prs = next
		} else if err != nil {
			t.errs, t.found = []string{err.Error()}, [3]int{} // the poll stopped here
		}
		t.me, t.lastPoll, t.stale, t.off = me, now, stale, off
		t.mu.Unlock()
		t.Save()
		return err
	}

	if me == "" {
		var u user
		err := api(t.run, t.login, "/user", &u)
		if err == nil && u.Login == "" {
			err = errors.New("tea api /user named no login")
		}
		if err != nil {
			return finish(nil, false, offReason(err), err)
		}
		me = u.Login
	}

	var items []searchItem
	if err := api(t.run, t.login, searchPath, &items); err != nil {
		if errors.Is(err, errNoTea) {
			return finish(nil, false, offReason(err), err)
		}
		return finish(nil, true, "", err)
	}

	var firstErr error
	var errs []string
	fail := func(err error) {
		if firstErr == nil {
			firstErr = err
		}
		errs = append(errs, err.Error())
	}
	found := [3]int{len(items)}
	var unlinked []string
	next := map[string]*pr{}
	seen := map[string]bool{}
	for _, it := range items {
		repo := strings.ToLower(it.Repository.FullName)
		if it.Number == 0 {
			continue
		}
		if !repos[repo] {
			if !named[prKey(repo, it.Number)] {
				unlinked = append(unlinked, prKey(repo, it.Number)) // no session sits in that repository
			}
			continue // a session that names it has it looked up below
		}
		k := prKey(repo, it.Number)
		seen[k] = true
		old := prev[k]
		if old != nil && old.UpdatedAt == it.UpdatedAt && !old.Done {
			next[k] = old // nothing moved: no call
			continue
		}
		d, err := fetch(t.run, t.login, repo, it.Number)
		if err != nil {
			fail(err)
			if old != nil {
				next[k] = old
			}
			continue
		}
		st := Judge(d, me)
		st.Repo = repo
		next[k] = &pr{Repo: repo, Number: it.Number, UpdatedAt: it.UpdatedAt, HeadRef: d.Pull.Head.Ref,
			Status: st, Done: st.Move == Merged || st.Move == Closed}
	}
	// Other people's pull requests you review. A Gitea that does not know
	// these searches fails them: that is said in the diagnostic and costs
	// nothing else — your own pull requests are still judged.
	reviewErr := false
	for i, path := range reviewPaths {
		var hits []searchItem
		if err := api(t.run, t.login, path, &hits); err != nil {
			errs = append(errs, err.Error())
			reviewErr = true
			continue
		}
		found[i+1] = len(hits)
		for _, it := range hits {
			repo := strings.ToLower(it.Repository.FullName)
			k := prKey(repo, it.Number)
			if it.Number == 0 || seen[k] {
				continue // yours, judged above, or listed by the other search
			}
			seen[k] = true
			old := prev[k]
			if old != nil && old.UpdatedAt == it.UpdatedAt && !old.Done {
				next[k] = old
				continue
			}
			d, err := fetch(t.run, t.login, repo, it.Number)
			if err != nil {
				fail(err)
				if old != nil {
					next[k] = old
				}
				continue
			}
			e := &pr{Repo: repo, Number: it.Number, UpdatedAt: it.UpdatedAt, HeadRef: d.Pull.Head.Ref}
			st, ok := JudgeReview(d, me)
			switch {
			case strings.EqualFold(d.Pull.User.Login, me) && named[k]:
				st, ok = Judge(d, me), true // your own, named by a session in a repository none has checked out
			case strings.EqualFold(d.Pull.User.Login, me):
				ok = false // your own, which the authored search answers for
			case !ok && named[k]:
				// Named, never asked, nothing said: a review you have not given.
				st.Move, st.Reason, st.At, ok = Yours, "not reviewed yet", d.Pull.CreatedAt, true
			}
			st.Repo = repo
			e.Status, e.Ignore = st, !ok
			e.Done = ok && (st.Move == Merged || st.Move == Closed)
			next[k] = e
		}
	}
	// The pull requests a session names that no search listed: someone
	// else's you look at without being asked, or your own in a repository
	// no session has checked out. One call says whether one moved; the rest
	// only when it did. One merged or closed is shown once, then kept
	// hidden for as long as a session still names it.
	for k := range named {
		if seen[k] {
			continue
		}
		seen[k] = true
		repo, n, ok := splitKey(k)
		if !ok {
			continue
		}
		old := prev[k]
		if old != nil && old.Done {
			cp := *old
			cp.Ignore = true
			next[k] = &cp
			continue
		}
		if old != nil && !old.MissingAt.IsZero() && now.Sub(old.MissingAt) < missingRetry {
			next[k] = old // could not be read an hour ago; not asked again yet
			continue
		}
		var p Pull
		err := api(t.run, t.login, pullPath(repo, n), &p)
		if err == nil && p.Number == 0 {
			err = fmt.Errorf("tea api %s: not a pull request", pullPath(repo, n))
		}
		if err != nil {
			// A name in a transcript is a hint, not a promise: an issue's
			// number, a typo, a repository this login cannot see. It is
			// said in the diagnostic and stales nothing else.
			errs = append(errs, "named "+k+": "+err.Error())
			if old != nil && old.MissingAt.IsZero() {
				next[k] = old // it was readable: keep what was known
			} else {
				next[k] = &pr{Repo: repo, Number: n, Ignore: true, MissingAt: now}
			}
			continue
		}
		stamp := p.UpdatedAt.UTC().Format(time.RFC3339)
		if old != nil && old.UpdatedAt == stamp {
			next[k] = old
			continue
		}
		d, err := fetchRest(t.run, t.login, repo, p)
		if err != nil {
			fail(err)
			if old != nil {
				next[k] = old
			}
			continue
		}
		var st Status
		if strings.EqualFold(p.User.Login, me) {
			st = Judge(d, me)
		} else if st, ok = JudgeReview(d, me); !ok {
			// Named, never asked, nothing said: a review you have not given.
			st.Move, st.Reason, st.At = Yours, "not reviewed yet", p.CreatedAt
		}
		st.Repo = repo
		next[k] = &pr{Repo: repo, Number: n, UpdatedAt: stamp, HeadRef: p.Head.Ref,
			Status: st, Done: st.Move == Merged || st.Move == Closed}
	}
	// What the open search no longer lists was merged or closed — or fell
	// past its page. One look says which, once; the next poll drops it.
	for k, old := range prev {
		if seen[k] || old.Done || old.Ignore || (!old.Status.Review && !repos[old.Repo]) {
			continue
		}
		if old.Status.Review && reviewErr {
			next[k] = old // the search that lists it failed: it was not seen, not gone
			continue
		}
		var p Pull
		err := api(t.run, t.login, pullPath(old.Repo, old.Number), &p)
		if err == nil && p.Number == 0 {
			err = fmt.Errorf("tea api %s: no pull request", pullPath(old.Repo, old.Number))
		}
		if err != nil {
			fail(err)
			next[k] = old
			continue
		}
		cp := *old
		gone := Status{Repo: old.Repo, Number: old.Number, URL: old.Status.URL, Review: old.Status.Review, Branch: old.Status.Branch}
		switch {
		case p.Merged:
			cp.Status, cp.Done = gone, true
			cp.Status.Move = Merged
		case p.State == "closed":
			cp.Status, cp.Done = gone, true
			cp.Status.Move = Closed
		}
		next[k] = &cp
	}
	t.mu.Lock()
	t.errs, t.found, t.unlinked = errs, found, unlinked
	t.mu.Unlock()
	return finish(next, firstErr != nil, "", firstErr)
}

// offReason is the one line the diagnostic gives for a feature that cannot run.
func offReason(err error) string {
	if errors.Is(err, errNoTea) {
		return "tea is not on PATH"
	}
	return "tea has no usable login (" + err.Error() + ")"
}

// splitKey reads "owner/repo#n" back.
func splitKey(k string) (repo string, n int, ok bool) {
	repo, num, found := strings.Cut(k, "#")
	if !found || !strings.Contains(repo, "/") {
		return "", 0, false
	}
	n, err := strconv.Atoi(num)
	return repo, n, err == nil && n > 0
}

func pullPath(repo string, n int) string { return fmt.Sprintf("/repos/%s/pulls/%d", repo, n) }

// fetch reads one pull request whole: the pull, its reviews, its comments,
// and when its head commit was made. The commit's date is a nicety — a
// Gitea that will not say leaves the push to the stale reviews.
func fetch(r Runner, login, repo string, n int) (Detail, error) {
	var p Pull
	if err := api(r, login, pullPath(repo, n), &p); err != nil {
		return Detail{Pull: p}, err
	}
	if p.Number == 0 {
		return Detail{Pull: p}, fmt.Errorf("tea api %s: no pull request", pullPath(repo, n))
	}
	return fetchRest(r, login, repo, p)
}

// fetchRest is fetch past the pull itself, already read.
func fetchRest(r Runner, login, repo string, p Pull) (Detail, error) {
	d := Detail{Pull: p}
	n := p.Number
	if err := api(r, login, pullPath(repo, n)+"/reviews", &d.Reviews); err != nil {
		return d, err
	}
	if err := api(r, login, fmt.Sprintf("/repos/%s/issues/%d/comments", repo, n), &d.IssueComments); err != nil {
		return d, err
	}
	if sha := d.Pull.Head.Sha; sha != "" {
		var c struct {
			Created time.Time `json:"created"`
			Commit  struct {
				Committer struct {
					Date time.Time `json:"date"`
				} `json:"committer"`
			} `json:"commit"`
		}
		path := fmt.Sprintf("/repos/%s/git/commits/%s?stat=false&verification=false&files=false", repo, url.PathEscape(sha))
		if api(r, login, path, &c) == nil {
			d.HeadAt = c.Commit.Committer.Date
			if d.HeadAt.IsZero() {
				d.HeadAt = c.Created
			}
		}
	}
	return d, nil
}

// States is every linked session's pull request as of the last poll, keyed
// by session key. It never touches the network. A session whose branch has
// no pull request open has no entry.
//
// A session working in several checkouts has a pull request for each that
// has one open, most urgent first (Sort).
func (t *Tracker) States(now time.Time) map[string][]Status {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.interval <= 0 || t.off != "" {
		return nil
	}
	stale := t.stale || (t.polling && now.Sub(t.pollStart) > slowPoll)
	out := map[string][]Status{}
	keys := map[string]bool{}
	for key := range t.links {
		keys[key] = true
	}
	for key := range t.refs {
		keys[key] = true
	}
	for key := range keys {
		ls := t.links[key]
		var sts []Status
		have := map[*pr]bool{}
		// The pull requests the session names, then the ones on its branches.
		for _, r := range t.refs[key] {
			if p := t.prs[r]; p != nil && !p.Ignore && !have[p] {
				have[p] = true
				st := p.Status
				st.Stale = stale
				sts = append(sts, st)
			}
		}
		for _, l := range ls {
			var best *pr
			for _, p := range t.prs {
				if p.Ignore || p.Repo != l.Repo || p.HeadRef != l.Branch {
					continue
				}
				// An open one over one just merged, then the newest.
				if best == nil || (best.Done && !p.Done) || (best.Done == p.Done && p.Number > best.Number) {
					best = p
				}
			}
			if best == nil || have[best] {
				continue
			}
			have[best] = true
			st := best.Status
			st.Stale = stale
			sts = append(sts, st)
		}
		if len(sts) > 0 {
			Sort(sts)
			out[key] = sts
		}
	}
	return out
}

// Reviews is every pull request of someone else's you review, most urgent
// first, whether or not a session works on its branch: the status line and
// the deck's header count them. It never touches the network.
func (t *Tracker) Reviews(now time.Time) []Status {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.interval <= 0 || t.off != "" {
		return nil
	}
	stale := t.stale || (t.polling && now.Sub(t.pollStart) > slowPoll)
	var out []Status
	for _, p := range t.prs {
		if p.Ignore || !p.Status.Review {
			continue
		}
		st := p.Status
		st.Stale = stale
		out = append(out, st)
	}
	Sort(out)
	return out
}

// Report is what the tracker knows, whole, for `compass prs`.
type Report struct {
	Me       string
	Off      string
	LastPoll time.Time
	Stale    bool
	Errors   []string
	// Found is how many pull requests each search listed on the last poll:
	// the ones you opened, the ones a review is requested of you on, the
	// ones you reviewed.
	Found [3]int
	// Unlinked is your own open PRs no live session works in the
	// repository of, as "owner/repo#n": listed, not shown.
	Unlinked []string
	// Links is each session's checkouts, by session key.
	Links map[string][]Link
	// PRs is every pull request tracked, the ones not yours to review included.
	PRs []Tracked
}

// Link is a checkout a session works in.
type Link struct{ Repo, Branch string }

// Tracked is one pull request as the tracker holds it.
type Tracked struct {
	Status
	HeadRef string
	Ignored bool
}

// Report hands back what the tracker knows.
func (t *Tracker) Report() Report {
	t.mu.Lock()
	defer t.mu.Unlock()
	r := Report{Me: t.me, Off: t.off, LastPoll: t.lastPoll, Stale: t.stale,
		Errors: append([]string(nil), t.errs...), Found: t.found, Links: map[string][]Link{},
		Unlinked: append([]string(nil), t.unlinked...)}
	for key, ls := range t.links {
		for _, l := range ls {
			r.Links[key] = append(r.Links[key], Link{Repo: l.Repo, Branch: l.Branch})
		}
	}
	for _, p := range t.prs {
		r.PRs = append(r.PRs, Tracked{Status: p.Status, HeadRef: p.HeadRef, Ignored: p.Ignore})
	}
	sort.Slice(r.PRs, func(i, j int) bool {
		if r.PRs[i].Repo != r.PRs[j].Repo {
			return r.PRs[i].Repo < r.PRs[j].Repo
		}
		return r.PRs[i].Number < r.PRs[j].Number
	})
	return r
}

// Off says why the tracker has nothing to show, "" when it works.
func (t *Tracker) Off() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.interval <= 0 {
		return "-pr-interval is 0"
	}
	return t.off
}

// Save writes the cache, atomically; a failure is silent and costs the next
// process one poll.
func (t *Tracker) Save() {
	if t.path == "" {
		return
	}
	t.mu.Lock()
	f := cacheFile{Me: t.me, LastPoll: t.lastPoll, Stale: t.stale, Off: t.off,
		Links: copyMap(t.links), CWDs: copyMap(t.cwds), Refs: copyMap(t.refs)}
	for _, p := range t.prs {
		f.PRs = append(f.PRs, p)
	}
	t.mu.Unlock()
	raw, err := json.Marshal(f)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(t.path), 0o755); err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(t.path), ".prs-*")
	if err != nil {
		return
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}
	_ = os.Rename(tmp.Name(), t.path)
}

// Probe is the diagnostic's line: whether the feature can run here, and why
// not. It asks tea who the login is — one call, on request only.
func Probe(r Runner, login string, interval time.Duration) string {
	if interval <= 0 {
		return "PR state: off — -pr-interval is 0"
	}
	var u user
	err := api(r, login, "/user", &u)
	if err == nil && u.Login == "" {
		err = errors.New("tea api /user named no login")
	}
	if err != nil {
		return "PR state: off — " + offReason(err)
	}
	return fmt.Sprintf("PR state: on — tea answers as %s, polling every %s", u.Login, interval)
}

func copyMap[V any](m map[string]V) map[string]V {
	out := make(map[string]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
