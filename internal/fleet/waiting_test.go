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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/state"
)

const (
	idWaitingDay   = "6f00000c-0000-4000-8000-00000000f60c" // asked a day ago, no pane
	idWaitingWeek  = "70000000-0000-4000-8000-000000000d70" // asked a week ago, no pane
	idAskedTool    = "8100000e-0000-4000-8000-00000000e181" // AskUserQuestion still out
	idResumed      = "9200000f-0000-4000-8000-00000000f292" // asked, then the harness resumed it
	idRefused      = "a3000010-0000-4000-8000-000000001a03" // asked nothing: the API refused the call
	idStatedOnly   = "b4000011-0000-4000-8000-000000001b04" // ended on a report, not a question
	idInterleaved  = "c5000012-0000-4000-8000-000000001c05" // one turn, results written inside it
	idSettledBatch = "d6000013-0000-4000-8000-000000001d06" // a batch whose calls all came back
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

// turn is one assistant message as Claude Code actually writes it: one
// content block per line, every line carrying the same `message.id`. A
// block is `{kind, id, name}` — kind "text" uses `id` as its words, kind
// "tool" is a call. Measured on this machine's own transcripts: of 656
// assistant lines, none carried two tool_use blocks, and 188 message ids
// spanned more than one line (#348).
func (b *transcriptBuilder) turn(ts time.Time, msgID string, blocks ...[3]string) *transcriptBuilder {
	for _, bl := range blocks {
		o := b.common(ts)
		o["type"] = "assistant"
		var content any
		switch bl[0] {
		case "text":
			content = []any{map[string]any{"type": "text", "text": bl[1]}}
		default:
			content = []any{map[string]any{
				"type": "tool_use", "id": bl[1], "name": bl[2], "input": map[string]any{},
			}}
		}
		o["message"] = map[string]any{
			"role": "assistant", "id": msgID, "model": "claude-fable-5", "type": "message",
			"content": content, "stop_reason": "tool_use",
		}
		b = b.add(o)
	}
	return b
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

	// The third shape is the one the other two never reach: a turn that
	// called nothing and was refused by nobody, whose last words are a
	// report. Both cases above settle before rule 4 is ever asked — the
	// stalled call on `out > 0`, the refusal on APIError — so with
	// `EndsWithQuestion` forced true this test still passed, and the rule
	// the door is named for was observed by nothing (round 62).
	newTranscript(t, idStatedOnly, "/home/user/alpha", "main").
		prompt(ago(27*time.Hour), "summarise the backfill").
		text(ago(26*time.Hour), "The backfill is done. 212 rows moved and the index is rebuilt.").
		write(root, slugAlpha)

	sessions := mustRefresh(t, fleet.NewManager(root), fleetNow)
	for _, id := range []string{idArchQuestion, idRefused, idStatedOnly} {
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

// A question the harness is holding open outranks an agent in flight, as
// the machine's own rules do: rule 2 precedes rule 3. A walk that stopped
// at the subagent's first line archived a session that had asked you in as
// many words and dispatched an agent in the same breath — the batch the
// either-order test exists for (round 61).
func TestAHeldQuestionOutranksAnAgentInFlight(t *testing.T) {
	root := t.TempDir()
	at := ago(26 * time.Hour)
	b := newTranscript(t, idAskedTool, "/home/user/alpha", "main").
		prompt(ago(27*time.Hour), "map the payments module")
	b.calls(at, [2]string{"toolu_ask7", state.AskUserQuestion}, [2]string{"toolu_task7", "Task"})
	b.sidechainPrompt(ago(25*time.Hour), "scout the payments module").
		sidechainText(ago(24*time.Hour), "The manifest is unsigned.").
		write(root, slugAlpha)

	assertWaiting(t, pick(t, mustRefresh(t, fleet.NewManager(root), fleetNow), idAskedTool), at)
}

// And the other side of the same mark: an agent writing under the model's
// own words is a session that is working, whatever those words end on.
func TestAnAgentInFlightStillClosesTheDoorOnPlainWords(t *testing.T) {
	root := t.TempDir()
	newTranscript(t, idWaitingWeek, "/home/user/alpha", "main").
		prompt(ago(27*time.Hour), "map the payments module").
		text(ago(26*time.Hour), "Two designs fit. Which should I build?").
		sidechainPrompt(ago(25*time.Hour), "scout the payments module").
		write(root, slugAlpha)

	s := pick(t, mustRefresh(t, fleet.NewManager(root), fleetNow), idWaitingWeek)
	if s.Info.Asked || s.Waiting {
		t.Errorf("Asked/Waiting = %v/%v with an agent still writing", s.Info.Asked, s.Waiting)
	}
}

// The either-order rule, on the shape the harness writes. A turn is one
// line per content block with the id repeated, so the question and the
// agent it was dispatched with are two lines and the walk meets one of
// them first. Round 59's rule was pinned on a fixture that put both blocks
// on one line — a shape this machine's own transcripts never contain — so
// the guarantee held only in the test (#348).
func TestTheEitherOrderRuleHoldsOnTheShapeTheHarnessWrites(t *testing.T) {
	ask := [3]string{"tool", "toolu_ask8", state.AskUserQuestion}
	task := [3]string{"tool", "toolu_task8", "Task"}
	words := [3]string{"text", "Two designs fit.", ""}

	for _, c := range []struct {
		name   string
		blocks [][3]string
	}{
		{"the question written first", [][3]string{ask, task}},
		{"the agent written first", [][3]string{task, ask}},
		{"words, then the agent, then the question", [][3]string{words, task, ask}},
		{"a plain batch with no agent at all", [][3]string{{"tool", "toolu_b8", "Bash"}, ask}},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			at := ago(26 * time.Hour)
			b := newTranscript(t, idAskedTool, "/home/user/alpha", "main").
				prompt(ago(27*time.Hour), "map the payments module")
			b.turn(at, "msg_batch_1", c.blocks...)
			b.write(root, slugAlpha)

			assertWaiting(t, pick(t, mustRefresh(t, fleet.NewManager(root), fleetNow), idAskedTool), at)
		})
	}
}

// And the other side of the same grouping: a turn whose lines carry no
// question of its own is work in flight whatever else it said. (What
// bounds the hold on the older side has its own pin below — this fixture
// settles on the prompt either way, so it never measured that.)
func TestATurnWithNoQuestionIsStillWorkInFlight(t *testing.T) {
	root := t.TempDir()
	at := ago(26 * time.Hour)
	b := newTranscript(t, idWaitingWeek, "/home/user/alpha", "main").
		prompt(ago(27*time.Hour), "map the payments module").
		text(ago(26*time.Hour+time.Minute), "Shall I start with the ledger?")
	b.turn(at, "msg_batch_2",
		[3]string{"text", "Looking at both.", ""},
		[3]string{"tool", "toolu_task9", "Task"},
		[3]string{"tool", "toolu_b9", "Bash"})
	b.write(root, slugAlpha)

	s := pick(t, mustRefresh(t, fleet.NewManager(root), fleetNow), idWaitingWeek)
	if s.Info.Asked || s.Waiting {
		t.Errorf("Asked/Waiting = %v/%v: a turn with two calls out and no question of its own",
			s.Info.Asked, s.Waiting)
	}
}

// What `compass status` costs, pinned. That process is a fresh one every
// few seconds out of tmux, so every byte it re-reads it re-reads forever.
// Round 61 fixed two ways this feature had quietly taken its resume away
// and shipped all of it unpinned; the panel found that by reverting each
// line and watching the package stay green (#348).
func TestTheAskDoorKeepsTheStatusLinesResume(t *testing.T) {
	t.Run("a quiet file peeked wide is not peeked again by the next process", func(t *testing.T) {
		root := t.TempDir()
		askedQuestionAt(t, root, slugAlpha, idWaitingDay, 26*time.Hour)
		path := filepath.Join(root, "projects", slugAlpha, idWaitingDay+".jsonl")
		cache := filepath.Join(t.TempDir(), "resume.json")

		warm := fleet.NewManager(root)
		c := fleet.OpenResumeCache(cache)
		warm.UseResumeCache(c)
		mustRefresh(t, warm, fleetNow)
		c.Save()

		// The second process is handed the same bytes, with the question
		// taken out of them — same length, same mtime, so the entry's
		// `(size, mtime)` key is untouched and only the cache's own memory
		// of having read wide decides. A process that forgot it re-reads
		// the file, finds no question and archives the session; on a real
		// home directory that is every quiet transcript, every few seconds.
		//
		// The replacement has to be the same size: doctoring it longer
		// invalidated the entry either way, so both the fold and its
		// revert re-read and both still answered `Asked=true` — the pin
		// held nothing for a round (round 62).
		doctored := strings.Replace(string(readFile(t, path)), "Shall I proceed?", "Shall I proceed.", 1)
		if len(doctored) != len(string(readFile(t, path))) {
			t.Fatalf("the doctored file is %d bytes against %d: the size key decides it and the cache is never asked", len(doctored), len(string(readFile(t, path))))
		}
		writeFile(t, path, doctored, fileTime(t, path))

		next := fleet.NewManager(root)
		next.UseResumeCache(fleet.OpenResumeCache(cache))
		s := pick(t, mustRefresh(t, next, fleetNow), idWaitingDay)
		if !s.Info.Asked {
			t.Fatalf("Info.Asked = false: the scan re-read a file it had already read wide, and the question it had is not in these bytes")
		}
	})

	t.Run("a door the fold refused still records its mark", func(t *testing.T) {
		root := t.TempDir()
		// A question over a call that never came back: the walk says
		// asked, the machine says hung, so the door is refused.
		newTranscript(t, idWaitingWeek, "/home/user/alpha", "main").
			prompt(ago(30*time.Hour), "build the release binary").
			tool(ago(29*time.Hour), "toolu_r1", "Bash", map[string]any{"command": "go build ./..."}).
			meta(ago(28*time.Hour), "Continue from where you left off.").
			text(ago(27*time.Hour), "The build never finished. Shall I retry it?").
			write(root, slugAlpha)
		cache := filepath.Join(t.TempDir(), "resume.json")

		warm := fleet.NewManager(root)
		c := fleet.OpenResumeCache(cache)
		warm.UseResumeCache(c)
		mustRefresh(t, warm, fleetNow)
		c.Save()

		raw := string(readFile(t, cache))
		if !strings.Contains(raw, "points") {
			t.Errorf("the refused session left no mark in %s: every status run replays its whole transcript", raw)
		}
	})

}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return raw
}

func writeFile(t *testing.T, path, content string, at time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

func fileTime(t *testing.T, path string) time.Time {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return fi.ModTime()
}

// TestTheHeldTurnSurvivesTheResultsWrittenInsideIt is the shape the fold
// for #348 did not reach. Grouping a turn by `message.id` reassembles the
// lines that are *adjacent*, and this harness does not always write them
// that way: on a turn that calls several times it writes each call's
// result between the calls, so one assistant message is three or four
// lines with `user` lines in the gaps. Measured on this machine: of 527
// assistant message ids, 249 span several lines and the two that are not
// consecutive are exactly the multi-call turns — the only shape this rule
// is about.
//
// The walk reads backwards, so it met a result first and read it as the
// end of the held turn, one line short of the question. A bare result line
// says nothing about whose turn it is: it records the call it answers and
// the hold stands (round 62).
func TestTheHeldTurnSurvivesTheResultsWrittenInsideIt(t *testing.T) {
	root := t.TempDir()
	at := ago(26 * time.Hour)
	b := newTranscript(t, idInterleaved, "/home/user/alpha", "main").
		prompt(ago(27*time.Hour), "map the payments module")
	// The measured shape, with a batched question where one would sit.
	b.turn(at, "msg_gap",
		[3]string{"text", "Three of these are independent.", ""},
		[3]string{"tool", "toolu_askg", state.AskUserQuestion},
		[3]string{"tool", "toolu_g1", "Task"})
	b.result(at.Add(time.Second), "toolu_g1", "agent done")
	b.turn(at.Add(2*time.Second), "msg_gap", [3]string{"tool", "toolu_g2", "Task"})
	b.result(at.Add(3*time.Second), "toolu_g2", "agent done")
	b.turn(at.Add(4*time.Second), "msg_gap", [3]string{"tool", "toolu_g3", "Task"})
	b.write(root, slugAlpha)

	assertWaiting(t, pick(t, mustRefresh(t, fleet.NewManager(root), fleetNow), idInterleaved), at)
}

// And the other door into the same turn: a batch whose calls have all come
// back settled the walk on the spot, without reading the rest of the turn.
// A question in an earlier line of that same message was never seen, so a
// batch that asked you something and ran one Bash call archived itself the
// moment the Bash result landed. The turn is held to its start instead,
// and the hold settles false of its own accord if no question is in it.
func TestABatchThatCameBackIsStillReadToItsStart(t *testing.T) {
	root := t.TempDir()
	at := ago(26 * time.Hour)
	b := newTranscript(t, idSettledBatch, "/home/user/alpha", "main").
		prompt(ago(27*time.Hour), "map the payments module")
	b.turn(at,
		"msg_done",
		[3]string{"tool", "toolu_askd", state.AskUserQuestion},
		[3]string{"tool", "toolu_bd", "Bash"})
	b.result(at.Add(time.Second), "toolu_bd", "212 passed")
	b.write(root, slugAlpha)

	assertWaiting(t, pick(t, mustRefresh(t, fleet.NewManager(root), fleetNow), idSettledBatch), at)
}

// TestTheHeldTurnOutlivesTheWindowItStartedIn is the hold's other end. The
// walk reads the file backwards in 64KB windows and widens while it is
// still undecided, and it closed a held turn after every window rather
// than at the start of the file — after which the walk is settled and each
// later window is a no-op, so the widening the loop exists for never
// reached the question.
//
// One tool result written between a turn's lines was enough to cross the
// boundary, and 64KB is the ordinary size of that shape: a subagent's
// report is what sits between the calls of the turn the hold is for. The
// question does not move; only the bytes under it do (round 62).
func TestTheHeldTurnOutlivesTheWindowItStartedIn(t *testing.T) {
	for _, n := range []int{1024, 70 * 1024, 200 * 1024} {
		root := t.TempDir()
		at := ago(26 * time.Hour)
		b := newTranscript(t, idInterleaved, "/home/user/alpha", "main").
			prompt(ago(27*time.Hour), "map the payments module")
		b.turn(at,
			"msg_wide",
			[3]string{"tool", "toolu_askw", state.AskUserQuestion},
			[3]string{"tool", "toolu_w1", "Task"})
		b.result(at.Add(time.Second), "toolu_w1", strings.Repeat("x", n))
		b.turn(at.Add(2*time.Second), "msg_wide", [3]string{"tool", "toolu_w2", "Task"})
		b.write(root, slugAlpha)

		s := pick(t, mustRefresh(t, fleet.NewManager(root), fleetNow), idInterleaved)
		if !s.Info.Asked || !s.Waiting {
			t.Errorf("a %dKB result inside the turn: Asked/Waiting = %v/%v — the question is the same distance from the end of the file, and only the bytes between its lines changed",
				n/1024, s.Info.Asked, s.Waiting)
		}
	}
}

// And the settle that the hold is bounded by on the other side: a turn
// whose calls are still out is work in flight, and a question from an
// *earlier* turn is not its last word. Without the different-id settle the
// walk reads straight past the boundary into the older turn and answers
// with a question the newer turn already superseded.
func TestAQuestionFromAnEarlierTurnIsNotThisTurnsLastWord(t *testing.T) {
	root := t.TempDir()
	b := newTranscript(t, idSettledBatch, "/home/user/alpha", "main").
		prompt(ago(30*time.Hour), "map the payments module")
	b.turn(ago(29*time.Hour), "msg_old", [3]string{"tool", "toolu_asko", state.AskUserQuestion})
	b.turn(ago(26*time.Hour), "msg_new", [3]string{"tool", "toolu_bn", "Bash"})
	b.write(root, slugAlpha)

	s := pick(t, mustRefresh(t, fleet.NewManager(root), fleetNow), idSettledBatch)
	if s.Info.Asked || s.Waiting {
		t.Errorf("Asked/Waiting = %v/%v: the newest turn has a call still out, and the question above it belongs to a turn that ended",
			s.Info.Asked, s.Waiting)
	}
}

// TestTheVerdictDoesNotDependOnWhereTheWindowFalls is the hold's third
// size rule, and the one that outlived the first fix. The walk widens by
// re-reading from the new window's start to the end of the file, so every
// line of the previous window was fed to it a second time — with the state
// the first pass left behind. That is not the no-op it looks like: a line
// the first pass skipped because nothing was held is, on the second pass
// with a turn held, "written below a held turn and not one of its results",
// and it settles the walk false. A hook's own user line and a thinking-only
// line of another turn both do it.
//
// So the verdict depended on where the 64KB boundary happened to fall
// against the question — the same lines, the same question, 700 bytes of
// tool output apart, waiting at 64000 and archived at 64700. Each line is
// walked exactly once now; the sizes below sit either side of that band
// (round 62).
func TestTheVerdictDoesNotDependOnWhereTheWindowFalls(t *testing.T) {
	for _, tail := range []string{"a hook's line under the turn", "a thinking-only line of another turn"} {
		for _, n := range []int{64000, 64700} {
			root := t.TempDir()
			at := ago(26 * time.Hour)
			b := newTranscript(t, idInterleaved, "/home/user/alpha", "main").
				prompt(ago(27*time.Hour), "map the payments module")
			b.turn(at, "msg_edge",
				[3]string{"tool", "toolu_aske", state.AskUserQuestion},
				[3]string{"tool", "toolu_te", "Task"})
			b.result(at.Add(time.Second), "toolu_te", strings.Repeat("x", n))
			if strings.HasPrefix(tail, "a hook") {
				b.meta(at.Add(2*time.Second), "<hook>stop hook ran</hook>")
			} else {
				b.turn(at.Add(2*time.Second), "msg_other", [3]string{"text", "", ""})
			}
			b.write(root, slugAlpha)

			s := pick(t, mustRefresh(t, fleet.NewManager(root), fleetNow), idInterleaved)
			if !s.Info.Asked || !s.Waiting {
				t.Errorf("%s, %d-byte result: Asked/Waiting = %v/%v — the question did not move, only the bytes under it",
					tail, n, s.Info.Asked, s.Waiting)
			}
		}
	}
}

// TestTheHeldQuestionOutranksAnAgentOnEitherOrder is round 61's guarantee
// on the shape the harness writes. A question the harness is holding open
// outranks an agent in flight — the machine's rule 2 before its rule 3 —
// and a one-line fixture pinned that. Split over two lines, `busy` refused
// the hold, so the guarantee depended on which block the harness wrote
// first: `[Ask, Task]` with an agent running archived the session, where
// `[Task, Ask]` kept it waiting. Same turn, same agent, same question
// (round 62).
func TestTheHeldQuestionOutranksAnAgentOnEitherOrder(t *testing.T) {
	ask := [3]string{"tool", "toolu_aska", state.AskUserQuestion}
	task := [3]string{"tool", "toolu_taska", "Task"}

	for _, c := range []struct {
		name   string
		blocks [][3]string
	}{
		{"the question written first", [][3]string{ask, task}},
		{"the agent written first", [][3]string{task, ask}},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			at := ago(26 * time.Hour)
			b := newTranscript(t, idAskedTool, "/home/user/alpha", "main").
				prompt(ago(27*time.Hour), "map the payments module")
			b.turn(at, "msg_agent", c.blocks...)
			// The agent it dispatched, writing in the lead's own file.
			b.sidechainPrompt(at.Add(time.Second), "look at the ledger tables")
			b.write(root, slugAlpha)

			assertWaiting(t, pick(t, mustRefresh(t, fleet.NewManager(root), fleetNow), idAskedTool), at)
		})
	}
}
