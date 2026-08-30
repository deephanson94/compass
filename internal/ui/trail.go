package ui

import (
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
)

// The trail's vocabulary (SPEC §2.1). Every mark is a distinct silhouette, so
// the graph reads with the colour switched off.
const (
	glyphPrompt = "◉" // the human turn the journey started from
	glyphLeg    = "◆" // a closed leg
	glyphHead   = "●" // HEAD — the leg in progress
	glyphBranch = "◈" // a subagent lane
	railStroke  = "│" // rail between two nodes
	railEnd     = "╵" // the rail's tail, above the oldest node
	railFork    = "├─"
	branchOpen  = "⋯" // the branch is still out there
	branchDone  = "✓" // it came back
)

// Column geometry: the class verb sits in its own column so the glyphs, the
// verbs and the labels each line up vertically, and the ages hold the right
// margin (SPEC §4).
const (
	trailVerbWidth   = 6                  // "design" is the longest verb
	trailPrefixWidth = 3 + trailVerbWidth // glyph, space, verb column, space
	trailForkWidth   = 4                  // "├─◈ "
	trailMinLabel    = 6                  // below this a label says nothing
)

// RenderTrail draws the Lv1 trail into a width×height block: newest at the top
// like `git log`, one line per node, the rail running down the left. The frame
// is exactly height lines; when the journey is longer than the panel the oldest
// rows fall off the bottom, so HEAD is never the thing that gets cropped.
func RenderTrail(tr journey.Trail, now time.Time, width, height int) string {
	return strings.Join(fit(trailRows(tr, now, width, height), height), "\n")
}

// trailRows is RenderTrail without the bottom padding, so a column that wants
// to know where its content stops can ask.
func trailRows(tr journey.Trail, now time.Time, width, height int) []string {
	if width < trailPrefixWidth || height < 1 {
		return nil
	}
	nodes := trailNodes(tr)
	if len(nodes) == 0 {
		return crop(trailEmptyRows(width), height)
	}

	rows := make([]string, 0, height+4)
	last := len(nodes) - 1
	for i, n := range nodes {
		rows = append(rows, n.render(tr, now, width))

		forks := branchRows(tr, n.leg, width)
		rows = append(rows, forks...)

		switch {
		case i == last:
			// The rail has nothing older to reach for.
		case i+1 == last:
			rows = append(rows, ruleStyle.Render(railEnd))
		case len(forks) == 0:
			rows = append(rows, ruleStyle.Render(railStroke))
		}
	}
	// Branches that forked before any leg opened hang off the bottom of the rail.
	rows = append(rows, branchRows(tr, -1, width)...)

	if len(tr.Legs) == 0 {
		// A journey that has only been asked for: say what comes next rather
		// than leaving the panel half empty (SPEC §4).
		rows = append(rows, "", dimStyle.Render(clip("scouting will appear here", width)))
	}
	return crop(rows, height)
}

// trailEmptyRows is the designed empty state: never a blank panel (SPEC §4).
func trailEmptyRows(width int) []string {
	return []string{
		dimStyle.Render(clip("◌ nothing yet", width)),
		"",
		dimStyle.Render(clip("scouting will appear here", width)),
	}
}

// trailNode is one row on the rail: a leg or a prompt, resolved to a time so
// the two can be interleaved.
type trailNode struct {
	at     time.Time
	leg    int // index into Trail.Legs, or -1
	prompt int // index into Trail.Prompts, or -1
}

// trailNodes interleaves legs and prompts, newest first. A leg that starts on
// the same tick as the prompt that provoked it sits above it: the prompt came
// first (decision log #1 — newest on top).
func trailNodes(tr journey.Trail) []trailNode {
	nodes := make([]trailNode, 0, len(tr.Prompts)+len(tr.Legs))
	for i, p := range tr.Prompts {
		nodes = append(nodes, trailNode{at: p.At, leg: -1, prompt: i})
	}
	for i, l := range tr.Legs {
		nodes = append(nodes, trailNode{at: l.Start, leg: i, prompt: -1})
	}
	// Ascending first — stable, so ties keep prompts before legs — then flipped.
	sort.SliceStable(nodes, func(i, j int) bool { return nodes[i].at.Before(nodes[j].at) })
	for i, j := 0, len(nodes)-1; i < j; i, j = i+1, j-1 {
		nodes[i], nodes[j] = nodes[j], nodes[i]
	}
	return nodes
}

func (n trailNode) render(tr journey.Trail, now time.Time, width int) string {
	if n.leg >= 0 {
		return legRow(tr.Legs[n.leg], now, width)
	}
	return promptRow(tr.Prompts[n.prompt], now, width)
}

// legRow: glyph, class verb, label, and the age held at the right margin. HEAD
// points at itself — `← 3m` — because it is the only line that is still moving.
func legRow(l journey.Leg, now time.Time, width int) string {
	glyph := glyphLeg
	age := relAge(now, l.Start)
	if l.Current {
		glyph = glyphHead
		age = "← " + age
	}
	head := classStyle(l.Class).Render(glyph + " " + pad(l.Class.String(), trailVerbWidth))

	labelWidth := width - trailPrefixWidth - 1 - len([]rune(age))
	if labelWidth < trailMinLabel {
		// Too narrow for a label: the verb and the age still answer "what, when".
		return head + padLeft(dimStyle.Render(age), width-(trailPrefixWidth-1))
	}
	label := textStyle.Render(pad(clip(l.Label, labelWidth), labelWidth))
	return head + " " + label + " " + dimStyle.Render(age)
}

// promptRow quotes the human turn — the only words on the trail that are not
// ours.
func promptRow(p journey.Prompt, now time.Time, width int) string {
	age := relAge(now, p.At)
	textWidth := width - 2 - 1 - len([]rune(age))
	if textWidth < trailMinLabel {
		return dimStyle.Render(glyphPrompt) + padLeft(dimStyle.Render(age), width-1)
	}
	text := textStyle.Render(pad(clip(`"`+p.Text+`"`, textWidth), textWidth))
	return dimStyle.Render(glyphPrompt) + " " + text + " " + dimStyle.Render(age)
}

// branchRows draws the subagent lanes that forked off leg index after (-1 for
// the ones that forked before any leg opened), newest first, each hanging off
// the rail at the node it left from.
func branchRows(tr journey.Trail, after, width int) []string {
	var rows []string
	for i := len(tr.Branches) - 1; i >= 0; i-- {
		b := tr.Branches[i]
		if b.AfterLeg != after {
			continue
		}
		mark := branchOpen
		if b.Done {
			mark = branchDone
		}
		labelWidth := width - trailForkWidth - 2
		if labelWidth < trailMinLabel {
			continue
		}
		label := dimStyle.Render(pad(clip(branchName(b.Label), labelWidth), labelWidth))
		rows = append(rows, ruleStyle.Render(railFork)+textStyle.Render(glyphBranch)+" "+
			label+" "+dimStyle.Render(mark))
	}
	return rows
}

// branchName never renders empty: an unnamed subagent is still "agent".
func branchName(label string) string {
	if strings.TrimSpace(label) == "" {
		return "agent"
	}
	return label
}

// relAge is the M0 age format, relative to the caller's clock.
func relAge(now, t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return state.ShortDuration(now.Sub(t))
}

// crop keeps the newest rows — the head of the list — and drops the tail.
func crop(rows []string, height int) []string {
	if len(rows) > height {
		return rows[:height]
	}
	return rows
}

// trailColumn is the deck's right-hand panel: the title, one line of air, and
// the graph.
func (m *Model) trailColumn(w, h int) []string {
	rows := []string{m.trailTitle(w), ""}
	if h > 2 {
		rows = append(rows, trailRows(m.trail, m.now, w, h-2)...)
	}
	return rows
}

// trailTitle: whose trail this is, and how deep we are in it.
func (m *Model) trailTitle(w int) string {
	name := "—"
	if s, ok := m.selected(); ok {
		name = sessionName(s.Info)
	}
	level := "[Lv1]"
	left := dimStyle.Render(clip("TRAIL · "+name, w-len(level)-1))
	gap := w - lipgloss.Width(left) - len(level)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + dimStyle.Render(level)
}
