package prstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
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
}

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
	off       string // why there is nothing to show; "" while it works
	prs       map[string]*pr
	links     map[string][]link // session key → the checkouts it works in
	cwds      map[string]string // session key → the dirs its links were read from, joined
}

// cacheFile is what a Tracker writes down for the next process.
type cacheFile struct {
	Me       string            `json:"me"`
	LastPoll time.Time         `json:"last_poll"`
	Stale    bool              `json:"stale"`
	Off      string            `json:"off,omitempty"`
	PRs      []*pr             `json:"prs"`
	Links    map[string][]link `json:"links"`
	CWDs     map[string]string `json:"cwds"`
}

// New returns a Tracker that polls through run as login ("" is tea's own
// default login) every interval, and keeps its state at path. A missing or
// unreadable cache is a fresh start.
func New(run Runner, login string, interval time.Duration, path string) *Tracker {
	t := &Tracker{run: run, login: login, interval: interval, path: path,
		prs: map[string]*pr{}, links: map[string][]link{}, cwds: map[string]string{}}
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
// dirs is each session's directories by session key: its cwd first, then
// wherever else its tool calls worked (fleet.Session.Dirs).
func (t *Tracker) Kick(dirs map[string][]string, now time.Time) {
	t.mu.Lock()
	if t.interval <= 0 {
		t.mu.Unlock()
		return
	}
	if !t.polling && now.Sub(t.lastPoll) >= t.interval {
		t.polling, t.pollStart = true, now
		t.mu.Unlock()
		go func() {
			_ = t.Poll(dirs, now)
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
func (t *Tracker) PollIfDue(dirs map[string][]string, now time.Time) {
	if t.Due(now) {
		_ = t.Poll(dirs, now)
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
func (t *Tracker) Poll(dirs map[string][]string, now time.Time) error {
	links := t.linkAll(dirs)
	repos := map[string]bool{}
	for _, ls := range links {
		for _, l := range ls {
			repos[l.Repo] = true
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
	t.links, t.cwds = links, sigs
	t.mu.Unlock()

	finish := func(next map[string]*pr, stale bool, off string, err error) error {
		t.mu.Lock()
		if next != nil {
			t.prs = next
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
	fail := func(err error) {
		if firstErr == nil {
			firstErr = err
		}
	}
	next := map[string]*pr{}
	seen := map[string]bool{}
	for _, it := range items {
		repo := strings.ToLower(it.Repository.FullName)
		if !repos[repo] || it.Number == 0 {
			continue // no session sits in that repository
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
	// What the open search no longer lists was merged or closed — or fell
	// past its page. One look says which, once; the next poll drops it.
	for k, old := range prev {
		if seen[k] || old.Done || !repos[old.Repo] {
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
		switch {
		case p.Merged:
			cp.Status = Status{Repo: old.Repo, Number: old.Number, URL: old.Status.URL, Move: Merged}
			cp.Done = true
		case p.State == "closed":
			cp.Status = Status{Repo: old.Repo, Number: old.Number, URL: old.Status.URL, Move: Closed}
			cp.Done = true
		}
		next[k] = &cp
	}
	return finish(next, firstErr != nil, "", firstErr)
}

// offReason is the one line the diagnostic gives for a feature that cannot run.
func offReason(err error) string {
	if errors.Is(err, errNoTea) {
		return "tea is not on PATH"
	}
	return "tea has no usable login (" + err.Error() + ")"
}

func pullPath(repo string, n int) string { return fmt.Sprintf("/repos/%s/pulls/%d", repo, n) }

// fetch reads one pull request whole: the pull, its reviews, its comments,
// and when its head commit was made. The commit's date is a nicety — a
// Gitea that will not say leaves the push to the stale reviews.
func fetch(r Runner, login, repo string, n int) (Detail, error) {
	var d Detail
	if err := api(r, login, pullPath(repo, n), &d.Pull); err != nil {
		return d, err
	}
	if d.Pull.Number == 0 {
		return d, fmt.Errorf("tea api %s: no pull request", pullPath(repo, n))
	}
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
	for key, ls := range t.links {
		var sts []Status
		for _, l := range ls {
			var best *pr
			for _, p := range t.prs {
				if p.Repo != l.Repo || p.HeadRef != l.Branch {
					continue
				}
				// An open one over one just merged, then the newest.
				if best == nil || (best.Done && !p.Done) || (best.Done == p.Done && p.Number > best.Number) {
					best = p
				}
			}
			if best == nil {
				continue
			}
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
		Links: copyMap(t.links), CWDs: copyMap(t.cwds)}
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
