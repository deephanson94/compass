// Package prstate says whose move it is on the pull request a session's
// branch has open: the reviewer's, because the last word on it is yours, or
// yours, because a reviewer answered. It reads a Gitea through `tea api` and a
// session's checkout through `git`, both behind one Runner, and keeps what it
// learned in a small cache so a fresh `compass status` does not ask again.
package prstate

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Move is whose turn a pull request is.
type Move int

const (
	// Theirs: the newest word is yours — a reply or a push. Nothing to do.
	Theirs Move = iota
	// Yours: a reviewer answered, or the PR is approved and only merging is left.
	Yours
	// Merged and Closed are shown once, on the poll that found them, and dropped on the next.
	Merged
	Closed
)

// Status is one pull request's state as a session's row says it.
type Status struct {
	Repo   string // owner/repo, lower case
	Number int
	URL    string
	Move   Move

	// Reason is why it is your move: "changes requested", "approved —
	// ready to merge", "reviewed", "2 new comments". Empty for the rest.
	Reason string
	// Who is the reviewer it waits on, for Theirs.
	Who string
	// At is the event the line's age counts from: the reviewer's answer for
	// Yours, your last word for Theirs.
	At time.Time

	// Stale says the last poll failed, or is taking too long: this is the
	// last state known, not the present one.
	Stale bool

	// Review says this is someone else's pull request you are reviewing,
	// judged from your side of it (JudgeReview); the line says "review PR".
	Review bool
	// Branch is the pull request's head branch, for the diagnostic.
	Branch string `json:",omitempty"`
}

// Ready is the reason an approved pull request carries.
const Ready = "approved — ready to merge"

// Line is the status in words, the way the board, the fleet row and the
// diagnostic print it. The words carry the meaning on their own; colour
// only reinforces them (README, principle 5).
func (s Status) Line(now time.Time) string { return s.line(now, true, "") }

// line is Line with or without its clock, the number named as repo+"#n"
// when repo is given.
func (s Status) line(now time.Time, clock bool, repo string) string {
	if !clock {
		now = time.Time{}
	}
	pr := fmt.Sprintf("PR %s#%d", repo, s.Number)
	if s.Review {
		pr = "review " + pr
	}
	var line string
	switch s.Move {
	case Merged:
		line = pr + " merged"
	case Closed:
		line = pr + " closed"
	case Yours:
		switch {
		case s.Reason == Ready || s.At.IsZero() || now.IsZero():
			line = pr + " · " + s.Reason
		default:
			line = pr + " · " + s.Reason + " " + Age(now, s.At) + " ago"
		}
	default:
		who := s.Who
		if who == "" {
			who = "review"
		}
		line = pr + " · waiting on " + who
		if s.Reason != "" {
			line = pr + " · " + s.Reason + " · waiting on " + who // "you approved · waiting on alice"
		}
		if !s.At.IsZero() && !now.IsZero() {
			line += " · " + Age(now, s.At)
		}
	}
	if s.Stale {
		line += staleMark
	}
	return line
}

// Age is a duration the way a row says it: 40s, 12m, 3h, 2d.
func Age(now, at time.Time) string {
	d := now.Sub(at)
	switch {
	case d < time.Minute:
		if d < 0 {
			d = 0
		}
		return fmt.Sprintf("%ds", int(d/time.Second))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	default:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	}
}

// The Gitea shapes, as much of them as the rules read.
type user struct {
	Login string `json:"login"`
}

// Pull is a pull request, from /repos/<owner>/<repo>/pulls/<n>.
type Pull struct {
	Number    int       `json:"number"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	Merged    bool      `json:"merged"`
	UpdatedAt time.Time `json:"updated_at"`
	HTMLURL   string    `json:"html_url"`
	User      user      `json:"user"`
	Head      struct {
		Ref string `json:"ref"`
		Sha string `json:"sha"`
	} `json:"head"`
	RequestedReviewers []user `json:"requested_reviewers"`
}

// Review is one entry of /pulls/<n>/reviews. Stale means commits landed on
// the branch after it was submitted.
type Review struct {
	ID          int64     `json:"id"`
	User        user      `json:"user"`
	State       string    `json:"state"`
	SubmittedAt time.Time `json:"submitted_at"`
	Stale       bool      `json:"stale"`
	Dismissed   bool      `json:"dismissed"`
}

// Comment is one entry of /issues/<n>/comments.
type Comment struct {
	ID        int64     `json:"id"`
	User      user      `json:"user"`
	CreatedAt time.Time `json:"created_at"`
}

// Detail is everything the rules read about one pull request.
type Detail struct {
	Pull          Pull      `json:"pull"`
	Reviews       []Review  `json:"reviews"`
	IssueComments []Comment `json:"issue_comments"`
	// HeadAt is when the head commit was committed: the push, when it is
	// known. Zero when it could not be read; a stale review still says a
	// push came after it.
	HeadAt time.Time `json:"head_at"`
}

// event is one dated word on a pull request.
type event struct {
	at     time.Time
	who    string
	mine   bool
	review string // the review's state; "" for a comment or a push
}

// Judge decides whose move a pull request is. me is the login the PR's
// author goes by; it is compared without case.
//
// The newest event among the submitted reviews, the issue comments and the
// pushes decides it: yours if someone else said it, theirs if you did. An
// approval nobody has requested changes since is your move whatever came
// after it — what is left is merging.
func Judge(d Detail, me string) Status {
	p := d.Pull
	s := Status{Number: p.Number, URL: p.HTMLURL}
	switch {
	case p.Merged:
		s.Move = Merged
		return s
	case p.State == "closed":
		s.Move = Closed
		return s
	}
	isMe := func(login string) bool { return strings.EqualFold(login, me) }

	var evs []event
	// latest is each reviewer's latest decision: an approval or a request
	// for changes, not dismissed. A review that only comments decides nothing.
	latest := map[string]Review{}
	for _, r := range d.Reviews {
		if r.State == "PENDING" || r.State == "" || r.SubmittedAt.IsZero() {
			continue // not submitted: nobody has seen it
		}
		evs = append(evs, event{at: r.SubmittedAt, who: r.User.Login, mine: isMe(r.User.Login), review: r.State})
		if r.Stale {
			// Commits landed after this review. When is not said, only
			// after: the push stands just behind it.
			evs = append(evs, event{at: r.SubmittedAt.Add(time.Nanosecond), who: me, mine: true})
		}
		if isMe(r.User.Login) || r.Dismissed {
			continue
		}
		if r.State != "APPROVED" && r.State != "REQUEST_CHANGES" {
			continue
		}
		if prev, ok := latest[r.User.Login]; !ok || !r.SubmittedAt.Before(prev.SubmittedAt) {
			latest[r.User.Login] = r
		}
	}
	for _, c := range d.IssueComments {
		evs = append(evs, event{at: c.CreatedAt, who: c.User.Login, mine: isMe(c.User.Login)})
	}
	if !d.HeadAt.IsZero() {
		evs = append(evs, event{at: d.HeadAt, who: me, mine: true})
	}
	// Oldest first; on a tie your own word goes first, so the reviewer's
	// answer at the same second is the newer.
	sort.SliceStable(evs, func(i, j int) bool {
		if !evs[i].at.Equal(evs[j].at) {
			return evs[i].at.Before(evs[j].at)
		}
		return evs[i].mine && !evs[j].mine
	})

	approved, changes := time.Time{}, false
	for _, r := range latest {
		switch r.State {
		case "REQUEST_CHANGES":
			changes = true
		case "APPROVED":
			if r.SubmittedAt.After(approved) {
				approved = r.SubmittedAt
			}
		}
	}
	if !approved.IsZero() && !changes {
		s.Move, s.Reason, s.At = Yours, Ready, approved
		return s
	}

	if len(evs) == 0 || evs[len(evs)-1].mine {
		s.Move = Theirs
		s.Who = waitingOn(p, evs)
		if len(evs) > 0 {
			s.At = evs[len(evs)-1].at
		}
		return s
	}

	newest := evs[len(evs)-1]
	s.Move, s.At = Yours, newest.at
	switch {
	case newest.review == "REQUEST_CHANGES":
		s.Reason = "changes requested"
	case newest.review == "APPROVED":
		s.Reason = "approved" // and someone else still wants changes
	case newest.review != "":
		s.Reason = "reviewed"
	case changes:
		s.Reason = "changes requested"
	default:
		n := 0
		for i := len(evs) - 1; i >= 0 && !evs[i].mine; i-- {
			if evs[i].review == "" {
				n++
			}
		}
		s.Reason = fmt.Sprintf("%d new comment", n)
		if n != 1 {
			s.Reason += "s"
		}
	}
	return s
}

// JudgeReview decides whose move someone else's pull request is, as its
// reviewer me sees it. Your words are your submitted reviews and your
// comments; the author's are their comments and their pushes — a push is
// the head commit's date, or a review of yours marked stale. Another
// reviewer's words are neither: they do not move your turn.
//
// The newest of those decides it: the author's, or a review requested of
// you that you have not given, is your move; yours is theirs. An approval
// of yours that nothing of the author's came after is theirs too — what is
// left is merging. ok is false where you are neither requested nor have
// said anything: the pull request is not yours to review.
func JudgeReview(d Detail, me string) (s Status, ok bool) {
	p := d.Pull
	s = Status{Number: p.Number, URL: p.HTMLURL, Review: true, Branch: p.Head.Ref}
	isMe := func(login string) bool { return strings.EqualFold(login, me) }
	author := p.User.Login
	isAuthor := func(login string) bool { return strings.EqualFold(login, author) }

	requested := false
	for _, u := range p.RequestedReviewers {
		if isMe(u.Login) {
			requested = true
		}
	}
	var mine time.Time            // my newest word
	var pushAt, replyAt time.Time // the author's newest push and comment
	mineApproved := false         // my newest review approves
	var mineReviewAt time.Time
	later := func(t *time.Time, at time.Time) {
		if at.After(*t) {
			*t = at
		}
	}
	for _, r := range d.Reviews {
		if r.State == "PENDING" || r.State == "" || r.SubmittedAt.IsZero() || !isMe(r.User.Login) {
			continue
		}
		later(&mine, r.SubmittedAt)
		if !r.SubmittedAt.Before(mineReviewAt) {
			mineReviewAt, mineApproved = r.SubmittedAt, r.State == "APPROVED"
		}
		if r.Stale {
			// Commits landed after this review: the push stands just behind it.
			later(&pushAt, r.SubmittedAt.Add(time.Nanosecond))
		}
	}
	later(&pushAt, d.HeadAt)
	for _, c := range d.IssueComments {
		switch {
		case isMe(c.User.Login):
			later(&mine, c.CreatedAt)
		case isAuthor(c.User.Login):
			later(&replyAt, c.CreatedAt)
		}
	}
	theirs := pushAt
	later(&theirs, replyAt)

	switch p := d.Pull; {
	case p.Merged:
		s.Move = Merged
		return s, true
	case p.State == "closed":
		s.Move = Closed
		return s, true
	}
	if mine.IsZero() {
		if !requested {
			return s, false
		}
		s.Move, s.Reason, s.At = Yours, "review requested", p.CreatedAt
		return s, true
	}
	if !theirs.After(mine) {
		s.Move, s.Who, s.At = Theirs, author, mine
		if mineApproved {
			s.Reason = "you approved"
		}
		return s, true
	}
	s.Move, s.At = Yours, theirs
	switch pushed, replied := pushAt.After(mine), replyAt.After(mine); {
	case pushed && replied:
		s.Reason = author + " pushed and replied"
	case pushed:
		s.Reason = author + " pushed"
	default:
		s.Reason = author + " replied"
	}
	return s, true
}

// waitingOn names who a pull request waits on: the reviewers requested on
// it, else whoever last spoke on it who is not you.
func waitingOn(p Pull, evs []event) string {
	var names []string
	for _, u := range p.RequestedReviewers {
		if u.Login != "" {
			names = append(names, u.Login)
		}
	}
	if len(names) > 0 {
		return strings.Join(names, ", ")
	}
	for i := len(evs) - 1; i >= 0; i-- {
		if !evs[i].mine && evs[i].who != "" {
			return evs[i].who
		}
	}
	return ""
}

// Sort orders one session's pull requests most urgent first: yours, then
// theirs, then the ones just merged or closed; the longest-waiting first
// among yours, then by number.
func Sort(sts []Status) {
	rank := func(m Move) int {
		switch m {
		case Yours:
			return 0
		case Theirs:
			return 1
		}
		return 2
	}
	sort.SliceStable(sts, func(i, j int) bool {
		a, b := sts[i], sts[j]
		if ra, rb := rank(a.Move), rank(b.Move); ra != rb {
			return ra < rb
		}
		if a.Move == Yours && !a.At.Equal(b.At) {
			return a.At.Before(b.At)
		}
		if a.Repo != b.Repo {
			return a.Repo < b.Repo
		}
		return a.Number < b.Number
	})
}

// Summary is a session's pull requests in one line, sts sorted (Sort): the
// most urgent in full, then how many more and how many of those wait on
// you. Where there is more than one, each is named with its repository —
// "repoA#1118" — since two repositories can both have a #77.
func Summary(sts []Status, now time.Time) string {
	if forms := Summaries(sts, now); len(forms) > 0 {
		return forms[0]
	}
	return ""
}

// Summaries is Summary in every form a narrower row can take, longest
// first: without "ago", without the clock, without the repository's name.
// The count of the others is the last thing shed — it is the only word
// saying there are others.
func Summaries(sts []Status, now time.Time) []string {
	if len(sts) == 0 {
		return nil
	}
	first, repo := sts[0], ""
	tail := ""
	if len(sts) > 1 {
		repo = repoName(first.Repo)
		yours := 0
		for _, s := range sts[1:] {
			if s.Move == Yours {
				yours++
			}
		}
		tail = fmt.Sprintf(" · +%d PR", len(sts)-1)
		if yours > 0 {
			tail += fmt.Sprintf(" (%d yours)", yours)
		}
	}
	with := func(line string) string {
		// The stale mark is the line's last word, and the whole line's.
		if strings.HasSuffix(line, staleMark) {
			return strings.TrimSuffix(line, staleMark) + tail + staleMark
		}
		return line + tail
	}
	full := first.line(now, true, repo)
	forms := []string{with(full)}
	if short := strings.Replace(full, " ago", "", 1); short != full {
		forms = append(forms, with(short))
	}
	forms = append(forms, with(first.line(now, false, repo)))
	if repo != "" {
		bare := first.line(now, false, "")
		forms = append(forms, with(bare), with(strings.TrimPrefix(bare, "PR "))) // "#1118 · changes requested · +1 PR"
	}
	return forms
}

// staleMark ends a line whose poll failed.
const staleMark = " · PR state stale"

// repoName is a repository's own name, without its owner.
func repoName(repo string) string {
	if i := strings.LastIndexByte(repo, '/'); i >= 0 {
		return repo[i+1:]
	}
	return repo
}

// ReviewSegment is the status line's word for the pull requests you review:
// "RV▲2" when two wait on you, "RV▲0" when every one waits on its author,
// "" when there are none open.
func ReviewSegment(sts []Status) string {
	open, yours := 0, 0
	for _, s := range sts {
		switch s.Move {
		case Yours:
			open++
			yours++
		case Theirs:
			open++
		}
	}
	if open == 0 {
		return ""
	}
	return fmt.Sprintf("RV▲%d", yours)
}

// Segment is the status line's word for the pull requests: "PR▲1" when one
// waits on you, "PR▲0" when every one waits on its reviewers, "" when no
// session has one open. A pull request two sessions share counts once.
func Segment(states map[string][]Status) string {
	open, yours := map[string]bool{}, map[string]bool{}
	for _, sts := range states {
		for _, s := range sts {
			k := fmt.Sprintf("%s#%d", s.Repo, s.Number)
			switch s.Move {
			case Yours:
				open[k], yours[k] = true, true
			case Theirs:
				open[k] = true
			}
		}
	}
	if len(open) == 0 {
		return ""
	}
	return fmt.Sprintf("PR▲%d", len(yours))
}
