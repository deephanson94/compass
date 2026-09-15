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

// sidechainText is a subagent's own words — its answer, or its own
// question, which is the lead's to answer and never yours.
func (b *transcriptBuilder) sidechainText(ts time.Time, text string) *transcriptBuilder {
	o := b.common(ts)
	o["type"] = "assistant"
	o["isSidechain"] = true
	o["message"] = map[string]any{
		"role": "assistant", "model": "claude-fable-5", "type": "message",
		"content":     []any{map[string]any{"type": "text", "text": text}},
		"stop_reason": "end_turn",
	}
	return b.add(o)
}

// calls is one assistant message carrying several tool_use blocks, the way
// the harness batches them: each pair is {id, name}, in the order written.
func (b *transcriptBuilder) calls(ts time.Time, uses ...[2]string) *transcriptBuilder {
	blocks := make([]any, 0, len(uses))
	for _, u := range uses {
		blocks = append(blocks, map[string]any{
			"type": "tool_use", "id": u[0], "name": u[1], "input": map[string]any{},
		})
	}
	o := b.common(ts)
	o["type"] = "assistant"
	o["message"] = map[string]any{
		"role": "assistant", "model": "claude-fable-5", "type": "message",
		"content": blocks, "stop_reason": "tool_use",
	}
	return b.add(o)
}

// textCalling is one assistant message that says something and calls a tool
// in the same turn — the narration a model writes before it goes to work.
func (b *transcriptBuilder) textCalling(ts time.Time, text, toolID, name string, input map[string]any) *transcriptBuilder {
	o := b.common(ts)
	o["type"] = "assistant"
	o["message"] = map[string]any{
		"role": "assistant", "model": "claude-fable-5", "type": "message",
		"content": []any{
			map[string]any{"type": "text", "text": text},
			map[string]any{"type": "tool_use", "id": toolID, "name": name, "input": input},
		},
		"stop_reason": "tool_use",
	}
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

// Rule 2, the order: a question you left behind sorts under everything
// happening today — the alarms of the moment and the work in flight — and
// over what is merely idle. `g` and the board both read this order, and the
// panel measured what happens when it sits higher: an archive where one
// session in four ends on a question takes every column the board has
// (round 59).
func TestTheWaitingSortUnderTheLiveAlarms(t *testing.T) {
	root := t.TempDir()
	needsYouAt(t, root, slugAlpha, idFreshUnmapped, time.Minute)
	stuckAt(t, root, slugAlpha, idStalePane, 2*time.Minute)
	askedQuestionAt(t, root, slugAlpha, idWaitingDay, 26*time.Hour)
	workingAt(t, root, slugBeta, idLiveWorker, 10*time.Second)
	idleAt(t, root, slugBeta, idLiveIdler, 30*time.Second)

	sessions := mustRefresh(t, fleet.NewManager(root), fleetNow)
	assertOrder(t, sessions, idFreshUnmapped, idStalePane, idLiveWorker, idWaitingDay, idLiveIdler)
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

// A message decides as a whole. Claude Code batches calls, so the model
// asking you something is written beside whatever else it dispatched, in
// whichever order it wrote them — and the machine reads the turn, not the
// block. A walk that settled on the first call disagreed with the machine
// whenever the question came second (round 59).
func TestAQuestionBesideAnotherCallOpensTheDoorInEitherOrder(t *testing.T) {
	ask := func(t *testing.T, id string, first, second [2]string) fleet.Session {
		t.Helper()
		root := t.TempDir()
		at := ago(26 * time.Hour)
		b := newTranscript(t, id, "/home/user/alpha", "main").
			prompt(ago(27*time.Hour), "map the payments module")
		b.calls(at, first, second)
		b.write(root, slugAlpha)
		return pick(t, mustRefresh(t, fleet.NewManager(root), fleetNow), id)
	}
	task := [2]string{"toolu_task9", "Task"}
	question := [2]string{"toolu_ask9", state.AskUserQuestion}

	assertWaiting(t, ask(t, idAskedTool, question, task), time.Time{})
	assertWaiting(t, ask(t, idAskedTool, task, question), time.Time{})
}

// The model's words are the last word only when nothing it dispatched came
// back after them. A turn that said something and then called a tool whose
// result is in the file is a turn the model is still in the middle of —
// the machine calls that working, or hung, and the door must not call it a
// question you owe (round 59).
func TestAMessageWhoseCallsCameBackIsNotTheLastWord(t *testing.T) {
	root := t.TempDir()
	at := ago(26 * time.Hour)
	newTranscript(t, idResumed, "/home/user/alpha", "main").
		prompt(ago(27*time.Hour), "find the gate").
		textCalling(at, "Which of the two files defines the gate?", "toolu_grep1", "Grep", map[string]any{"pattern": "gate"}).
		result(at.Add(time.Second), "toolu_grep1", "gate.go:12").
		write(root, slugAlpha)

	s := pick(t, mustRefresh(t, fleet.NewManager(root), fleetNow), idResumed)
	if s.Info.Asked {
		t.Errorf("Info.Asked = true on narration the turn moved past: the result came after it")
	}
	assertArchivedSnap(t, s)
}

// A subagent writing after the question is this session being busy — the
// machine counts those lines, and a door that read past them would call a
// session with an agent in flight a question you owe. The subagent's own
// question never opens the door either: that one is the lead's to answer.
func TestASubagentWritingAfterTheQuestionClosesTheDoor(t *testing.T) {
	root := t.TempDir()
	newTranscript(t, idWaitingWeek, "/home/user/alpha", "main").
		prompt(ago(27*time.Hour), "map the payments module").
		text(ago(26*time.Hour), "Two designs fit. Which should I build?").
		sidechainPrompt(ago(25*time.Hour), "scout the payments module").
		sidechainText(ago(24*time.Hour), "Should an unsigned manifest be fatal, or a warning?").
		write(root, slugAlpha)

	s := pick(t, mustRefresh(t, fleet.NewManager(root), fleetNow), idWaitingWeek)
	if s.Info.Asked {
		t.Errorf("Info.Asked = true with a subagent still writing under the question")
	}
	if s.Waiting {
		t.Errorf("Waiting = true on a session whose agent is in flight")
	}
}

// How far back the walk reads is decided by the clock, not by a fixed
// window: a file that has gone quiet is one whose question is the only
// thing that can keep it live, and the scan will not open it again until
// it moves, so it is read whole. A file still being written gets one
// window — the recency door already has it, so nothing is lost (round 59).
func TestTheWalkWidensForAFileThatHasGoneQuiet(t *testing.T) {
	// The bulk is the harness's own: a line nobody typed, which settles
	// nothing and pushes the question out of the last window.
	bulk := strings.Repeat("resumed. ", 12000) // ~108KB, past peekTail

	t.Run("quiet: the question is found however far back it is", func(t *testing.T) {
		root := t.TempDir()
		askedAt := ago(26 * time.Hour)
		newTranscript(t, idWaitingDay, "/home/user/alpha", "main").
			prompt(ago(27*time.Hour), "review the migration plan").
			text(askedAt, "The plan is drafted. Shall I proceed?").
			meta(ago(25*time.Hour), bulk).
			write(root, slugAlpha)

		assertWaiting(t, pick(t, mustRefresh(t, fleet.NewManager(root), fleetNow), idWaitingDay), askedAt)
	})

	t.Run("still moving: one window, and the recency door has it anyway", func(t *testing.T) {
		root := t.TempDir()
		newTranscript(t, idWaitingDay, "/home/user/alpha", "main").
			prompt(ago(4*time.Minute), "review the migration plan").
			text(ago(3*time.Minute), "The plan is drafted. Shall I proceed?").
			meta(ago(time.Minute), bulk).
			write(root, slugAlpha)

		s := pick(t, mustRefresh(t, fleet.NewManager(root), fleetNow), idWaitingDay)
		if s.Info.Asked {
			t.Errorf("Info.Asked = true: a file still being written read past its window")
		}
		if !s.Live || s.Waiting {
			t.Errorf("Live/Waiting = %v/%v, want live on the recency door and not waiting", s.Live, s.Waiting)
		}
	})
}

// The invariant the board, the ranks and the SPEC row all rest on: a
// session the ask door keeps live is a session the machine calls needs-you.
// Where the two differ the machine wins — it folds the whole file where the
// walk reads its last lines — and the door closes again.
//
// The round-59 pin for this built five sessions by hand and asserted over
// them; a hand-made session cannot disagree with a machine that never ran.
// This one is transcripts, and its cases are the shapes the panel broke the
// invariant with (round 60).
func TestWaitingIsAlwaysNeedsYou(t *testing.T) {
	cases := []struct {
		name  string
		write func(t *testing.T, root, id string)
	}{
		{"a call that never came back, then a resume and a question", func(t *testing.T, root, id string) {
			newTranscript(t, id, "/home/user/alpha", "main").
				prompt(ago(30*time.Hour), "build the release binary").
				tool(ago(29*time.Hour), "toolu_b1", "Bash", map[string]any{"command": "go build ./..."}).
				meta(ago(28*time.Hour), "Continue from where you left off.").
				text(ago(27*time.Hour), "The build never finished. Shall I retry it?").
				write(root, slugAlpha)
		}},
		{"a question, then a wordless assistant line", func(t *testing.T, root, id string) {
			newTranscript(t, id, "/home/user/alpha", "main").
				prompt(ago(30*time.Hour), "review the migration plan").
				text(ago(29*time.Hour), "The plan is drafted. Shall I proceed?").
				text(ago(28*time.Hour), "").
				write(root, slugAlpha)
		}},
		{"a call out, a question, then the result", func(t *testing.T, root, id string) {
			newTranscript(t, id, "/home/user/alpha", "main").
				prompt(ago(30*time.Hour), "find the gate").
				tool(ago(29*time.Hour), "toolu_g1", "Grep", map[string]any{"pattern": "gate"}).
				text(ago(28*time.Hour), "Which of the two files defines the gate?").
				result(ago(27*time.Hour), "toolu_g1", "gate.go:12").
				write(root, slugAlpha)
		}},
		{"a question, then a subagent's words", func(t *testing.T, root, id string) {
			newTranscript(t, id, "/home/user/alpha", "main").
				prompt(ago(30*time.Hour), "map the payments module").
				text(ago(29*time.Hour), "Two designs fit. Which should I build?").
				sidechainText(ago(28*time.Hour), "The manifest is unsigned.").
				write(root, slugAlpha)
		}},
		{"a question, then a refused call with no words of its own", func(t *testing.T, root, id string) {
			newTranscript(t, id, "/home/user/alpha", "main").
				prompt(ago(30*time.Hour), "run the backfill").
				text(ago(29*time.Hour), "The shards are ready. Shall I start?").
				apiError(ago(28*time.Hour), 403, "authentication_failed", "").
				write(root, slugAlpha)
		}},
		{"the plain case, which must still be waiting", func(t *testing.T, root, id string) {
			askedQuestionAt(t, root, slugAlpha, id, 26*time.Hour)
		}},
	}

	waited := 0
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			c.write(t, root, idWaitingDay)
			s := pick(t, mustRefresh(t, fleet.NewManager(root), fleetNow), idWaitingDay)
			if s.Waiting {
				waited++
				if s.Snap.State != state.NeedsYou || s.Snap.APIError {
					t.Errorf("Waiting with state %s (apiError %v) — the door opened on a verdict the machine does not share",
						s.Snap.State, s.Snap.APIError)
				}
			}
		})
	}
	if waited == 0 {
		t.Errorf("no case came out waiting: the invariant held vacuously")
	}
}

// A door the fold refused stays shut while the file sits still, and opens
// again the moment the file grows: the alternative is replaying a whole
// transcript every second to reach the same answer (round 60).
func TestARefusedDoorDoesNotKnockTwiceUntilTheFileMoves(t *testing.T) {
	root := t.TempDir()
	// A call that never came back under a question: the walk says asked,
	// the machine says hung.
	newTranscript(t, idWaitingWeek, "/home/user/alpha", "main").
		prompt(ago(30*time.Hour), "build the release binary").
		tool(ago(29*time.Hour), "toolu_b2", "Bash", map[string]any{"command": "go build ./..."}).
		meta(ago(28*time.Hour), "Continue from where you left off.").
		text(ago(27*time.Hour), "The build never finished. Shall I retry it?").
		write(root, slugAlpha)

	m := fleet.NewManager(root)
	first := pick(t, mustRefresh(t, m, fleetNow), idWaitingWeek)
	if first.Live || first.Waiting {
		t.Fatalf("Live/Waiting = %v/%v, want the archive: the machine calls this hung", first.Live, first.Waiting)
	}
	assertArchivedSnap(t, first)
	assertArchivedSnap(t, pick(t, mustRefresh(t, m, fleetNow), idWaitingWeek))

	// The file grows into a shape the machine agrees with: the result
	// lands, and a question is the last word.
	at := ago(20 * time.Hour)
	appendLines(t, root, slugAlpha, idWaitingWeek,
		continueTranscript(t, idWaitingWeek, "/home/user/alpha", "main", 4).
			result(at, "toolu_b2", "ok").
			text(at.Add(time.Second), "The build is green. Shall I tag it?"),
		at.Add(time.Second))

	assertWaiting(t, pick(t, mustRefresh(t, m, fleetNow), idWaitingWeek), time.Time{})
}

// The widening is reached by the file falling quiet, not only by a restart.
// A session you walk away from is peeked while it is still moving — narrow,
// because the recency door has it — and the (size, mtime) cache would then
// serve that narrow verdict for the rest of the run, so the question behind
// a long tail was found only by a compass started after the fact (round 60).
func TestAFileThatFallsQuietIsReadAgain(t *testing.T) {
	root := t.TempDir()
	bulk := strings.Repeat("resumed. ", 12000) // ~108KB, past one window
	askedAt := ago(3 * time.Minute)
	newTranscript(t, idWaitingDay, "/home/user/alpha", "main").
		prompt(ago(4*time.Minute), "review the migration plan").
		text(askedAt, "The plan is drafted. Shall I proceed?").
		meta(ago(2*time.Minute), bulk).
		write(root, slugAlpha)

	m := fleet.NewManager(root)
	// While it is moving: one window, no question found, and live anyway.
	moving := pick(t, mustRefresh(t, m, fleetNow), idWaitingDay)
	if moving.Info.Asked {
		t.Fatalf("a file still being written read past its window")
	}
	if !moving.Live || moving.Waiting {
		t.Fatalf("Live/Waiting = %v/%v, want live on the recency door", moving.Live, moving.Waiting)
	}

	// Ten minutes later nothing has been written and nobody has restarted
	// compass. The file has not moved, so the cache holds its narrow
	// answer — and the question is exactly what decides whether this
	// session is still on the board.
	later := fleetNow.Add(10 * time.Minute)
	quiet := pick(t, mustRefresh(t, m, later), idWaitingDay)
	if !quiet.Info.Asked {
		t.Errorf("Info.Asked = false once the file fell quiet: the widening waited for a restart")
	}
	assertWaiting(t, quiet, askedAt)
}
