package ui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/transcript"
)

// A lead that ran its subagents as teammates: their replies come back as
// relayed teammate messages, which the segmenter joins to the lane by the
// teammate's name (#395). The lane that heard back draws `✓ … ago` with
// its finding beneath; the one that never did draws `⌀ lost … ago`; the
// card counts one lost, not two; and the reply stands as no prompt row.
func teammateLeadTrail(base time.Time) journey.Trail {
	input := func(v map[string]string) json.RawMessage {
		b, err := json.Marshal(v)
		if err != nil {
			panic(err)
		}
		return b
	}
	user := func(at time.Time, text string) transcript.Event {
		return transcript.Event{Type: transcript.EventUser, UUID: "u", SessionID: "s", Timestamp: at, Text: text}
	}
	calls := func(at time.Time, tus ...transcript.ToolUse) transcript.Event {
		return transcript.Event{Type: transcript.EventAssistant, UUID: "a", SessionID: "s", Timestamp: at, ToolUses: tus}
	}
	results := func(at time.Time, ids ...string) transcript.Event {
		ev := transcript.Event{Type: transcript.EventUser, UUID: "u", SessionID: "s", Timestamp: at}
		for _, id := range ids {
			ev.ToolResults = append(ev.ToolResults, transcript.ToolResult{ToolUseID: id, Text: "Spawned successfully. (This tool result is internal metadata; the agent's real result arrives as a task notification.)"})
		}
		return ev
	}
	s := journey.NewSegmenter()
	for _, ev := range []transcript.Event{
		user(base, "convene the panel on the plan"),
		calls(base.Add(time.Minute), transcript.ToolUse{ID: "e1", Name: "Edit", Input: input(map[string]string{"file_path": "/home/user/plan.md"})}),
		results(base.Add(time.Minute+time.Second), "e1"),
		calls(base.Add(10*time.Minute),
			transcript.ToolUse{ID: "a1", Name: "Agent", Input: input(map[string]string{"name": "panel-theorist", "team_name": "panel", "description": "Theorist: review the plan", "prompt": "review the plan"})},
			transcript.ToolUse{ID: "a2", Name: "Agent", Input: input(map[string]string{"name": "panel-skeptic", "team_name": "panel", "description": "Skeptic: review the plan", "prompt": "review the plan"})}),
		results(base.Add(10*time.Minute+time.Second), "a1", "a2"),
		user(base.Add(20*time.Minute), "Another Claude session sent a message:\n\n<teammate-message teammate_id=\"panel-theorist\" color=\"purple\">\nThe plan holds; two gates share a root cause.\n</teammate-message>"),
		{Type: transcript.EventAssistant, UUID: "a", SessionID: "s", Timestamp: base.Add(20*time.Minute + time.Second), Text: "Noted; waiting on the skeptic."},
	} {
		s.Observe(ev)
	}
	return s.Trail()
}

func TestATeammatesReplyClosesItsLaneOnTheTrail(t *testing.T) {
	forceASCII(t)
	tr := teammateLeadTrail(fixtureBase)
	if len(tr.Branches) != 2 || !tr.Branches[0].Done || tr.Branches[1].Done {
		t.Fatalf("Branches = %+v; want the theorist's closed and the skeptic's open", tr.Branches)
	}
	if len(tr.Prompts) != 1 {
		t.Fatalf("Prompts = %+v; the reply that closed its lane is no prompt", tr.Prompts)
	}

	m := boardModel(152, 30)
	tf := sessionKey("s-tfstate") // idle: an open lane on it is lost
	m.trails[tf] = tr
	col := strings.Join(m.boardColumn(tf, rowFor(t, m, tf), 44, 20), "\n")
	if !strings.Contains(col, "✓ 20m ago") || !strings.Contains(col, "The plan holds; two gates share a root") {
		t.Errorf("the theorist's lane is back, with its finding beneath it:\n%s", col)
	}
	if !strings.Contains(col, "⌀ lost 30m ago") {
		t.Errorf("the skeptic's lane is lost:\n%s", col)
	}
	if !strings.Contains(col, "◈1 lost") || strings.Contains(col, "◈2 lost") {
		t.Errorf("the card counts one lane lost, not two:\n%s", col)
	}
	if strings.Contains(col, "relayed") {
		t.Errorf("the reply is said once, beneath its lane, never again as a prompt:\n%s", col)
	}

	// The legs, where every lane's finding hangs beneath it.
	lv2 := renderLv(tr, fixtureBase.Add(40*time.Minute), levelWaypoints, 60, 30)
	rows := strings.Split(lv2, "\n")
	found := false
	for i, r := range rows {
		if strings.Contains(r, "Theorist: review the plan") && strings.Contains(r, "✓ 20m ago") {
			found = i+1 < len(rows) && strings.Contains(rows[i+1], "The plan holds; two gates share a root cause.")
		}
	}
	if !found {
		t.Errorf("the finding hangs under the theorist's lane at the legs:\n%s", lv2)
	}
	if !strings.Contains(lv2, "Skeptic: review the plan") || strings.Contains(lv2, "relayed") {
		t.Errorf("the skeptic's lane is drawn, and the reply is no prompt row:\n%s", lv2)
	}
}
