package fleet_test

// The ask door (#344, amending docs/dev/M5-CONTRACT.md rule 1):
//
//	1. a session whose transcript's last word is the model's, asking you
//	   something, is live however long ago it asked — no pane, no recency.
//	2. it is marked Waiting, which is what the board ranks under the alarms
//	   of the moment and what the status line leaves out.
//	3. it clears itself: the moment a person replies, the last word is
//	   theirs and the session falls back to the two doors that were there
//	   before.
//	4. a window of 0 — "tmux panes only" — shuts this door too.
//
// The field finding behind it: a session that asks "which of these should I
// do next?" while you are elsewhere drops off the board five minutes later,
// and the question is gone by the time you think to look for it.

import (
	"strings"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/state"
)

const (
	idWaitingDay  = "6f00000c-0000-4000-8000-00000000f60c" // asked a day ago, no pane
	idWaitingWeek = "70000000-0000-4000-8000-000000000d70" // asked a week ago, no pane
	idAskedTool   = "8100000e-0000-4000-8000-00000000e181" // AskUserQuestion still out
	idResumed     = "9200000f-0000-4000-8000-00000000f292" // asked, then the harness resumed it
	idRefused     = "a3000010-0000-4000-8000-000000001a03" // asked nothing: the API refused the call
)

// meta is a user line the harness wrote, not the person: the resume prompt,
// a hook's feedback. `isMeta` is Claude Code's own flag for it.
func (b *transcriptBuilder) meta(ts time.Time, text string) *transcriptBuilder {
	o := b.common(ts)
	o["type"] = "user"
	o["isMeta"] = true
	o["message"] = map[string]any{"role": "user", "content": text}
	return b.add(o)
}

// apiError is a call the gateway refused, written as Claude Code writes it: a
// synthetic assistant message carrying the error's own words and the flags
// that say it is not the model speaking.
func (b *transcriptBuilder) apiError(ts time.Time, status int, key, text string) *transcriptBuilder {
	o := b.common(ts)
	o["type"] = "assistant"
	o["isApiErrorMessage"] = true
	o["apiErrorStatus"] = status
	o["error"] = key
	o["message"] = map[string]any{
		"role": "assistant", "model": "<synthetic>", "type": "message",
		"content":           []any{map[string]any{"type": "text", "text": text}},
		"isApiErrorMessage": true, "apiErrorStatus": status, "error": key,
	}
	return b.add(o)
}

// sidechainPrompt is a subagent's own conversation — the newest lines in the
// file while a Task runs, and not this session speaking.
func (b *transcriptBuilder) sidechainPrompt(ts time.Time, text string) *transcriptBuilder {
	o := b.common(ts)
	o["type"] = "user"
	o["isSidechain"] = true
	o["message"] = map[string]any{"role": "user", "content": text}
	return b.add(o)
}

// assertWaiting pins what the door is for: the session is live, it is marked
// as waiting on you, and the verdict is the real one — read off the file, not
// the archive's stand-in.
func assertWaiting(t *testing.T, s fleet.Session, askedAt time.Time) {
	t.Helper()
	if !s.Live {
		t.Errorf("%s: Live = false — a question nobody has answered keeps a session live", s.Info.ID)
	}
	if !s.Waiting {
		t.Errorf("%s: Waiting = false, want true — nothing but the question is holding it open", s.Info.ID)
	}
	if s.Snap.State != state.NeedsYou {
		t.Errorf("%s: state = %s, want needs-you", s.Info.ID, s.Snap.State)
	}
	if !askedAt.IsZero() && !s.Snap.Since.Equal(askedAt) {
		t.Errorf("%s: Since = %v, want the question's instant %v", s.Info.ID, s.Snap.Since, askedAt)
	}
	if s.Snap.Reason == "archived" {
		t.Errorf("%s: Reason = %q — the question was answered by the archive's stand-in", s.Info.ID, s.Snap.Reason)
	}
}

// A day-old question, no pane, nothing written since: the case the door was
// built for. Without it this session is archived and its question is gone.
func TestAQuestionKeepsASessionLiveHoweverOldItIs(t *testing.T) {
	root := t.TempDir()
	askedAt, lastAt := askedQuestionAt(t, root, slugAlpha, idWaitingDay, 26*time.Hour)
	weekAt, _ := askedQuestionAt(t, root, slugAlpha, idWaitingWeek, 7*24*time.Hour)

	sessions := mustRefresh(t, fleet.NewManager(root), fleetNow)
	day := pick(t, sessions, idWaitingDay)
	assertWaiting(t, day, askedAt)
	assertWaiting(t, pick(t, sessions, idWaitingWeek), weekAt)

	if !day.Info.Asked {
		t.Errorf("%s: Info.Asked = false — the tail read no question", day.Info.ID)
	}
	if !day.Info.AskedAt.Equal(askedAt) {
		t.Errorf("%s: Info.AskedAt = %v, want %v", day.Info.ID, day.Info.AskedAt, askedAt)
	}
	if !day.Info.LastEventAt.Equal(lastAt) {
		t.Errorf("%s: LastEventAt = %v, want %v", day.Info.ID, day.Info.LastEventAt, lastAt)
	}
	if day.Info.Title != "review the migration plan" {
		t.Errorf("%s: Title = %q, want the opening prompt", day.Info.ID, day.Info.Title)
	}
}

// Rule 3: the door is held open by the question, so answering it is what
// closes it — no bookkeeping, no expiry, nothing to remember.
func TestAnAnsweredQuestionFallsBackToTheArchive(t *testing.T) {
	root := t.TempDir()
	askedQuestionAt(t, root, slugAlpha, idWaitingDay, 26*time.Hour)
	m := fleet.NewManager(root)
	assertWaiting(t, pick(t, mustRefresh(t, m, fleetNow), idWaitingDay), time.Time{})

	// You come back and reply. The reply is old too — this is a session you
	// answered and then walked away from again.
	replyAt := ago(25 * time.Hour)
	appendLines(t, root, slugAlpha, idWaitingDay,
		continueTranscript(t, idWaitingDay, "/home/user/alpha", "main", 3).
			prompt(replyAt, "yes, apply it"),
		replyAt)

	after := pick(t, mustRefresh(t, m, fleetNow), idWaitingDay)
	if after.Info.Asked {
		t.Errorf("Info.Asked = true after a person replied: the last word is theirs")
	}
	assertArchivedSnap(t, after)
	if after.Waiting {
		t.Errorf("Waiting = true on an answered session")
	}
}

// The harness's own turns are not an answer. "Continue from where you left
// off." is nobody speaking, and the machine does not count it either — so a
// resumed-and-abandoned session is still one you owe an answer.
func TestTheHarnessOwnTurnDoesNotAnswerTheQuestion(t *testing.T) {
	root := t.TempDir()
	askedAt, _ := askedQuestionAt(t, root, slugAlpha, idResumed, 26*time.Hour)
	metaAt := ago(25 * time.Hour)
	appendLines(t, root, slugAlpha, idResumed,
		continueTranscript(t, idResumed, "/home/user/alpha", "main", 3).
			meta(metaAt, "Continue from where you left off."),
		metaAt)

	assertWaiting(t, pick(t, mustRefresh(t, fleet.NewManager(root), fleetNow), idResumed), askedAt)
}

// The model asking in as many words: an AskUserQuestion call with no result
// under it. The machine calls that needs-you without any quiet threshold, and
// the door reads the file the same way.
func TestAnAskUserQuestionStillOutOpensTheDoor(t *testing.T) {
	root := t.TempDir()
	calledAt := ago(30 * time.Hour)
	newTranscript(t, idAskedTool, "/home/user/alpha", "main").
		prompt(ago(31*time.Hour), "which index should we build first?").
		tool(calledAt, "toolu_ask1", "AskUserQuestion", map[string]any{
			"questions": []any{map[string]any{"question": "which index first?"}},
		}).
		write(root, slugAlpha)

	assertWaiting(t, pick(t, mustRefresh(t, fleet.NewManager(root), fleetNow), idAskedTool), calledAt)
}

// What the door must not admit: work that stopped, and a call the API
// refused. Neither is a question, and a fleet that resurrected every session
// that ever stalled would be the archive again.
func TestTheDoorIsForQuestionsOnly(t *testing.T) {
	root := t.TempDir()
	stalledCallAt(t, root, slugAlpha, idArchQuestion, 26*time.Hour)
	refusedAt := ago(26 * time.Hour)
	newTranscript(t, idRefused, "/home/user/alpha", "main").
		prompt(ago(27*time.Hour), "run the backfill").
		apiError(refusedAt, 403, "authentication_failed", "API Error: 403 · Please run /login").
		write(root, slugAlpha)

	sessions := mustRefresh(t, fleet.NewManager(root), fleetNow)
	for _, id := range []string{idArchQuestion, idRefused} {
		s := pick(t, sessions, id)
		if s.Info.Asked {
			t.Errorf("%s: Info.Asked = true — only a question opens the door", id)
		}
		assertArchivedSnap(t, s)
	}
}

// Waiting is the third door's own word: a session tmux has, or one that spoke
// a moment ago, is live on its own terms and is today's fleet, question or
// not.
func TestAPaneOrTheWindowIsNotWaiting(t *testing.T) {
	root := t.TempDir()
	askedQuestionAt(t, root, slugAlpha, idWaitingDay, 26*time.Hour)
	askedQuestionAt(t, root, slugAlpha, idFreshUnmapped, time.Minute)

	m := fleet.NewManager(root)
	m.MarkPaneMapped(panesFor(t, root, idWaitingDay))
	sessions := mustRefresh(t, m, fleetNow)

	paned := pick(t, sessions, idWaitingDay)
	if !paned.Live || paned.Waiting {
		t.Errorf("%s: Live/Waiting = %v/%v, want live and not waiting — tmux has it", paned.Info.ID, paned.Live, paned.Waiting)
	}
	fresh := pick(t, sessions, idFreshUnmapped)
	if !fresh.Live || fresh.Waiting {
		t.Errorf("%s: Live/Waiting = %v/%v, want live and not waiting — it spoke a minute ago",
			fresh.Info.ID, fresh.Live, fresh.Waiting)
	}
}

// Rule 2, the order: a question you left behind sorts under the alarms of the
// moment and over the work in flight. `g` and the board both read this order.
func TestTheWaitingSortUnderTheLiveAlarms(t *testing.T) {
	root := t.TempDir()
	needsYouAt(t, root, slugAlpha, idFreshUnmapped, time.Minute)
	stuckAt(t, root, slugAlpha, idStalePane, 2*time.Minute)
	askedQuestionAt(t, root, slugAlpha, idWaitingDay, 26*time.Hour)
	workingAt(t, root, slugBeta, idLiveWorker, 10*time.Second)
	idleAt(t, root, slugBeta, idLiveIdler, 30*time.Second)

	sessions := mustRefresh(t, fleet.NewManager(root), fleetNow)
	assertOrder(t, sessions, idFreshUnmapped, idStalePane, idWaitingDay, idLiveWorker, idLiveIdler)
}

// Rule 4: the off switch. "tmux panes only" is an answer to the whole
// question of what is live.
func TestAZeroWindowShutsTheAskDoorToo(t *testing.T) {
	root := t.TempDir()
	askedQuestionAt(t, root, slugAlpha, idWaitingDay, 26*time.Hour)

	m := fleet.NewManager(root)
	m.SetLiveWindow(0)
	s := pick(t, mustRefresh(t, m, fleetNow), idWaitingDay)
	if s.Live || s.Waiting {
		t.Errorf("with window 0: Live/Waiting = %v/%v, want neither", s.Live, s.Waiting)
	}
	assertArchivedSnap(t, s)
}

// The status line is the glance at what is happening. A question from
// yesterday is not happening — the board is where it is kept.
func TestTheStatusLineLeavesTheWaitingOut(t *testing.T) {
	root := t.TempDir()
	askedQuestionAt(t, root, slugAlpha, idWaitingDay, 26*time.Hour)
	workingAt(t, root, slugBeta, idLiveWorker, 10*time.Second)

	if got, want := fleet.NewManager(root).StatusLine(fleetNow), "●1"; got != want {
		t.Errorf("StatusLine = %q, want %q — a waiting session is not an alarm the bar raises", got, want)
	}

	quiet := t.TempDir()
	askedQuestionAt(t, quiet, slugAlpha, idWaitingDay, 26*time.Hour)
	if got, want := fleet.NewManager(quiet).StatusLine(fleetNow), "○ all quiet"; got != want {
		t.Errorf("StatusLine = %q, want %q", got, want)
	}
}

// A subagent's own lines are the newest in the file while a Task runs, and
// they are not this session speaking: what decides is the lead's own last
// word, which here is a call still out. A session with agents in flight is
// working, not asking, however long the lines have been quiet.
func TestASubagentInFlightIsNotAQuestion(t *testing.T) {
	root := t.TempDir()
	calledAt := ago(26 * time.Hour)
	newTranscript(t, idWaitingWeek, "/home/user/alpha", "main").
		prompt(ago(27*time.Hour), "map the payments module").
		tool(calledAt, "toolu_task1", "Task", map[string]any{"prompt": "scout payments"}).
		sidechainPrompt(ago(25*time.Hour), "scout the payments module").
		write(root, slugAlpha)

	s := pick(t, mustRefresh(t, fleet.NewManager(root), fleetNow), idWaitingWeek)
	if s.Info.Asked {
		t.Errorf("Info.Asked = true: a Task still out is work in flight, not a question")
	}
	if s.Waiting {
		t.Errorf("Waiting = true on a session whose agent never came back")
	}
	assertArchivedSnap(t, s)
}

// The walk gets the last window of the file and no more. A question buried
// behind more than that much of a subagent's output is not one this door
// opens for — and the bound is what keeps a session mid-Task from re-reading
// a megabyte on every scan, which is a second-by-second cost where the door
// is a once-a-day gain.
func TestTheAskWalkReadsOneWindowOfTheTail(t *testing.T) {
	root := t.TempDir()
	askedAt := ago(30 * time.Hour)
	b := newTranscript(t, idWaitingWeek, "/home/user/alpha", "main").
		prompt(ago(31*time.Hour), "map the payments module").
		text(askedAt, "Two designs fit. Which should I build?")
	// A subagent's own conversation, wider than the window the walk reads.
	bulk := strings.Repeat("payments ", 4096) // ~40KB a line
	for i := 0; i < 4; i++ {
		b = b.sidechainPrompt(ago(29*time.Hour), bulk)
	}
	b.write(root, slugAlpha)

	s := pick(t, mustRefresh(t, fleet.NewManager(root), fleetNow), idWaitingWeek)
	if s.Info.Asked {
		t.Errorf("Info.Asked = true: the walk read past its window")
	}
	assertArchivedSnap(t, s)
}
