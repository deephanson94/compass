package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
)

// The card board (`board = "cards"`). The column board draws every trail
// that fits, oldest row at the top, and on a wide screen that came to seven
// columns forty cells wide, each two days tall: the class counts over a day
// of legs, with the one row that says what is happening now at the bottom
// of a column nobody reads to the bottom of.
//
// A card answers the glance instead. It is wider — fewer across, so its
// rows stop truncating — and it says, top down:
//
//	▸1 ▲ tinker-lfm-cmdline                        stuck  26m
//	port LFM2 runner to cmdline · hung 26m in Bash: grep -n …
//	today 6h · 12 prompts · 9 build · 4 fix · 3 red   opus · ⌁ w:5.0
//	● scout  grep -n "^#" /hdd4/deep/v…               silent 26m
//	◆ test   run pytest; 3 failures remain   19✓ 3✗          2m
//	◆ fix    resolve tasks merge conflict                    3m
//	◉ 25/28 "yeah you can merge, get the issue6…"        33m ago
//
// the fleet row; the goal and what it is doing now; today's tally beside
// the tag; then today's work newest first, so the breathing HEAD row sits
// right under the sentence it is the evidence for. The cards keep the
// board's own packing: as wide as they can be read, then as tall as the
// screen has rows for, so a short day is a short card and a tall screen
// shows the whole of today. Yesterday and before are the session view's.
//
// The goal is the one sentence a model writes (narrator/goal.go), asked
// once a turn is over; until it lands the card quotes the latest prompt.
// Everything else on the card is the trail's own, deterministic.

// cardBoard switches the board to cards; set once, before the first frame.
var cardBoard bool

// SetBoardCards turns the card board on or off.
func SetBoardCards(on bool) { cardBoard = on }

// cardColMin is a card's least width: from here a row carries its label,
// its badge and its figure whole. 84 puts four across a 32-inch screen,
// where 60 put six and every row still clipped.
var cardColMin = 84

// cardsPerRow is how many cards stand in a row when the person has said
// (`cards_per_row`, or + and - on the board); 0 leaves it to the width.
var cardsPerRow int

// cardColFloor is the narrowest a card is drawn however many are asked for.
const cardColFloor = 40

// SetCardsPerRow sets how many cards stand in a row; 0 or less is automatic.
func SetCardsPerRow(n int) { cardsPerRow = max(0, n) }

// stepCardsPerRow is + and - on the card board: one card more or fewer in
// a row, starting from what the board drew, and the note says how many.
func (m *Model) stepCardsPerRow(delta int) {
	inner := m.width - 2*edgePad
	n, _ := boardColumns(inner, m.drawnCount(m.viewOrder()))
	maxN := max(1, (inner+gutterWidth)/(cardColFloor+gutterWidth))
	cardsPerRow = min(max(1, n+delta), maxN)
	m.retireTrailMemo()
	m.note = fmt.Sprintf("%d cards a row · + and - change it · cards_per_row in the config keeps it", cardsPerRow)
}

// SetCardWidth sets a card's least width; below 60 it keeps the default.
func SetCardWidth(w int) {
	if w >= 60 {
		cardColMin = w
	}
}

const (
	cardColMax  = 140
	cardHead    = 4  // the fleet row, the goal row, the tally row, and a row of air before the work
	cardRowsMin = 5  // a card with a quiet today still shows its last five moments
	cardRowsMax = 60 // a card fills the rows the board shares it; past this a day is the trail's
)

// boardColBounds is the board's column width floor and cap, by board.
func boardColBounds() (lo, hi int) {
	if cardBoard {
		return cardColMin, cardColMax
	}
	return boardColMin, boardColMax
}

// cardItem is one row of a card's work: a prompt, a leg or an open lane.
type cardItem struct {
	at     time.Time // when it was last true: a prompt's time, a leg's end (HEAD: now), a lane's start
	seq    int       // the trail's own order, for ties
	prompt int       // index into Prompts, else -1
	leg    int       // index into Legs, else -1
	lane   int       // index into Branches, else -1
}

// cardItems is the trail's moments, newest first.
func cardItems(tr journey.Trail, now time.Time) []cardItem {
	var out []cardItem
	seq := 0
	add := func(it cardItem) {
		it.seq = seq
		seq++
		out = append(out, it)
	}
	for i, p := range tr.Prompts {
		add(cardItem{at: p.At, prompt: i, leg: -1, lane: -1})
	}
	for i, l := range tr.Legs {
		if tinyLeg(l) {
			continue
		}
		at := l.End
		if l.Current || at.IsZero() {
			at = now
		}
		add(cardItem{at: at, prompt: -1, leg: i, lane: -1})
	}
	for i, b := range tr.Branches {
		if !b.Done {
			add(cardItem{at: b.Start, prompt: -1, leg: -1, lane: i})
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		if !out[a].at.Equal(out[b].at) {
			return out[a].at.After(out[b].at)
		}
		// At one instant a prompt comes before the work it started, so
		// newest first puts the work above it.
		return out[a].seq > out[b].seq
	})
	return out
}

// tinyLeg is a leg too slight for a card: closed in under ten seconds,
// touching no file, with no result — "closing thanks, no edits", "pass,
// no files". The trail keeps it; a card has better rows to spend.
func tinyLeg(l journey.Leg) bool {
	return !l.Current && len(l.Files) == 0 && legBadge(l) == "" && l.Class != journey.Ship &&
		!l.End.IsZero() && l.End.Sub(l.Start) < tinyLegSpan
}

// tinyLegSpan is how short a leg with nothing to show for it must be to
// stay off a card.
const tinyLegSpan = 10 * time.Second

// dayStart is the local midnight of t.
func dayStart(t time.Time) time.Time {
	y, mo, d := t.Date()
	return time.Date(y, mo, d, 0, 0, 0, 0, t.Location())
}

// cardWork is a card's rows of work, newest first: today's, or the last
// few moments when today has fewer, with a rule where the day turns, and
// a closing row naming what the card leaves to the session view.
func (m *Model) cardWork(tr journey.Trail, o TrailOpts) []string {
	items := cardItems(tr, o.Now)
	if len(items) == 0 {
		if bare := bareHeadRow(o, o.Width); bare != "" {
			return []string{bare}
		}
		return trailEmptyRows(o.Width)
	}
	today := dayStart(o.Now)
	n := 0
	for n < len(items) && !items[n].at.Before(today) {
		n++
	}
	n = max(n, min(cardRowsMin, len(items)))
	earlier := 0
	if n > cardRowsMax {
		earlier, n = n-cardRowsMax, cardRowsMax
	}
	var rows []string
	var prevDay time.Time
	if bare := bareHeadRow(o, o.Width); bare != "" && !hasHead(tr) {
		// Working, and no leg yet: the present still has a row, as on
		// the trail ("● scout  thinking…  for 40s").
		rows = append(rows, bare)
	}
	for i, it := range items[:n] {
		day := dayStart(it.at)
		if i > 0 && day.Before(prevDay) || i == 0 && day.Before(today) {
			if len(rows) > 0 && rows[len(rows)-1] == "" {
				rows = rows[:len(rows)-1] // the rule is the gap
			}
			rows = append(rows, dayRule(day, o.Now, o.Width))
		}
		prevDay = day
		rows = append(rows, m.cardRow(tr, it, o)...)
		if it.prompt >= 0 && i < n-1 {
			// A turn is your prompt and the work it led to; newest first,
			// the work stands above its prompt, and a row of air closes
			// the turn so the card reads as turns, not as one list.
			rows = append(rows, "")
		}
	}
	if earlier > 0 {
		rows = append(rows, dimStyle.Render(clip(fmt.Sprintf("↓ %d earlier today · tab opens the trail", earlier), o.Width)))
	}
	return rows
}

// hasHead says whether the trail has an open leg.
func hasHead(tr journey.Trail) bool {
	for i := len(tr.Legs) - 1; i >= 0; i-- {
		if tr.Legs[i].Current {
			return true
		}
	}
	return false
}

// cardRow draws one moment in the trail's own row shape: one row, or HEAD
// and the rest of a question too long for it, which is the one line the
// deck exists to deliver and is never clipped to its first words.
func (m *Model) cardRow(tr journey.Trail, it cardItem, o TrailOpts) []string {
	switch {
	case it.prompt >= 0:
		nth, total := promptNumber(tr, it.prompt)
		row := promptRow(tr.Prompts[it.prompt], o.Now, o.Width, nth, total, promptWait(tr, it.prompt), o.PromptLinks[it.prompt])
		if nth != total {
			// Your latest prompt is what the work above it answers; the
			// ones before it are context, and in a card of a busy day they
			// were half its rows at full brightness.
			row = dimStyle.Render(ansi.Strip(row))
		}
		return []string{row}
	case it.leg >= 0:
		l := tr.Legs[it.leg]
		o.Ask = askBefore(tr, l.Start)
		if l.Current {
			o.HeadWaits = headWaits(tr)
			if o.HeadTail == "" {
				o.HeadTail = headTail(tr, o.Now, o.HeadState != state.Idle, o.Agents)
			}
		}
		label, narrated := legLabel(l, o)
		if t := strings.Trim(label, " ."); t == "" && len(l.Files) > 0 {
			// A leg whose heuristic found no name ("build ."): the files
			// it touched are the most it can say.
			label = strings.Join(l.Files[:min(2, len(l.Files))], ", ")
		}
		out := []string{legRow(l, label, narrated, o)}
		if l.Current && o.HeadState == state.NeedsYou && o.Head != "" && label != o.Head {
			rest := strings.TrimSpace(strings.TrimPrefix(o.Head, label))
			for _, line := range wrapQuestion(rest, o.Width-trailWayWidth, 4) {
				out = append(out, strings.Repeat(" ", trailWayWidth)+dimStyle.Render(line))
			}
		}
		return out
	default:
		b := tr.Branches[it.lane]
		figure := "out " + relAge(o.Now, b.Start)
		head := dimStyle.Render(glyphBranch + " " + pad("agent", trailVerbWidth))
		room := o.Width - trailPrefixWidth - 1 - len([]rune(figure))
		if room < trailMinLabel {
			return []string{head + " " + dimStyle.Render(clip(figure, o.Width-trailPrefixWidth-1))}
		}
		return []string{head + " " + textStyle.Render(pad(clip(b.Label, room), room)) + " " + dimStyle.Render(figure)}
	}
}

// dayRule marks where a card's rows cross into an earlier day.
func dayRule(day, now time.Time, w int) string {
	label := day.Format("Mon 2 Jan")
	if dayStart(now).Sub(day) <= 24*time.Hour {
		label = "yesterday"
	}
	return dimStyle.Render(clip("── "+label+" "+strings.Repeat("─", max(0, w)), w))
}

// cardStatus is the card's second row: what the session is for, and what
// it is doing — or stuck on, or asking — now. The goal is the narrator's;
// the rest is the state machine's and the trail's.
func (m *Model) cardStatus(key string, s fleet.Session, tr journey.Trail, w int) string {
	badge := m.cardBadge(s, tr)
	bw := lipgloss.Width(badge)
	if bw+1 >= w {
		return badge
	}
	return badge + " " + m.cardStatusText(key, s, tr, w-bw-1)
}

// cardStatusText is the status row after its badge.
func (m *Model) cardStatusText(key string, s fleet.Session, tr journey.Trail, w int) string {
	goal, narrated := m.cardGoal(key, tr)
	now := m.cardNow(s, tr)
	if goal == "" {
		return dimStyle.Render(clip(now, w))
	}
	if now == "" {
		if narrated {
			return titleStyle.Render(clip(goal, w))
		}
		return dimStyle.Render(clip(goal, w))
	}
	// The goal is short and changes least; what it is doing now is the
	// half the eye scans the board for, so it keeps the room.
	// The goal is the narrator's best line and at most 32 cells: it is
	// drawn whole, and the clauses after it take what is left. A quoted
	// prompt standing in for one is clipped to a third.
	gw := min(lipgloss.Width(goal), w-12)
	if !narrated {
		gw = min(lipgloss.Width(goal), max(18, w/3))
	}
	goal = clip(goal, gw)
	styled := dimStyle.Render(goal)
	if narrated {
		styled = titleStyle.Render(goal)
	}
	return styled + dimStyle.Render(" · ") + textStyle.Render(clip(now, max(0, w-gw-3)))
}

// cardBadge is the card's health at a glance, in two levels: the main
// status, one of six words on a block of colour the eye finds across a wall
// of cards before it reads a word — and after it, dim, the sub-status: the
// one short reason it is that colour ("approve", "API 403", "timeout"),
// when there is one to give.
func (m *Model) cardBadge(s fleet.Session, tr journey.Trail) string {
	word, sub, col := m.cardState(s, tr)
	style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#111111")).Background(col)
	if col == colDim {
		// Grey on grey read as no badge at all: the quiet ones get a
		// light grey block with dark text, quieter than a colour and
		// still a block the eye can count.
		style = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#ffffff", Dark: "#111111"}).
			Background(lipgloss.AdaptiveColor{Light: "#6b6b6b", Dark: "#a0a0a0"})
	}
	badge := style.Render(" " + word + " ")
	if sub != "" {
		badge += " " + dimStyle.Render(sub)
	}
	return badge
}

// The card's six main statuses.
const (
	cardWorking      = "working"
	cardStillWorking = "still working"
	cardNeedsYou     = "needs you"
	cardNeedsAttn    = "needs attn"
	cardExited       = "exited"
	cardIdle         = "idle"
)

// cardState is the card's main status, its sub-status and its colour. The
// first that applies wins: exited, needs you, needs attn, still working,
// working, idle. The silence that makes an open turn still working (from
// ten minutes) or needs attn (from an hour) is the state machine's, so the
// fleet, the trail and the card agree on it.
func (m *Model) cardState(s fleet.Session, tr journey.Trail) (word, sub string, col lipgloss.TerminalColor) {
	snap := s.Snap
	_, runs, loop := circling(tr)
	switch {
	case snap.Exited:
		return cardExited, "mid-turn", colDim
	case snap.APIError:
		if snap.APIStatus != 0 {
			return cardNeedsAttn, fmt.Sprintf("API %d", snap.APIStatus), colStuck
		}
		return cardNeedsAttn, "API", colStuck
	case s.Waiting, snap.State == state.NeedsYou:
		// A question is what needs you means; only another reason says itself.
		if snap.Pending == state.PendingApprove {
			return cardNeedsYou, "approve", colNeedsYou
		}
		return cardNeedsYou, "", colNeedsYou
	case snap.State == state.Stuck:
		if snap.Pending == state.PendingBash {
			return cardNeedsAttn, "timeout", colStuck
		}
		return cardNeedsAttn, pendingSub(snap.Pending), colStuck
	case loop:
		return cardNeedsAttn, fmt.Sprintf("loop ×%d", runs), colStuck
	case snap.State == state.Working && snap.Late:
		return cardStillWorking, pendingSub(snap.Pending), colStill
	case snap.State == state.Working:
		// Working is fine and says nothing more — but for a subagent, which
		// is why the session's own transcript is quiet.
		if snap.Pending == state.PendingSubagent {
			return cardWorking, "subagent", colWorking
		}
		return cardWorking, "", colWorking
	}
	switch {
	case snap.Interrupted:
		return cardIdle, "cancel", colDim
	case m.backgroundAgent(s, tr):
		return cardIdle, "bg job", colDim
	case shippedLast(tr):
		return cardIdle, "✓ shipped", colDim
	case snap.Reason == "no activity yet":
		return cardIdle, "new", colDim
	}
	return cardIdle, "", colDim
}

// pendingSub is what a slow open turn waits on, as its sub-status says it.
func pendingSub(pending string) string {
	if pending == state.PendingTool {
		return ""
	}
	return pending
}

// backgroundAgent says a finished turn left an agent working: a lane still
// out whose own file was written inside the first tier.
func (m *Model) backgroundAgent(s fleet.Session, tr journey.Trail) bool {
	agents := m.agentsFor(s.Info.Key())
	for _, b := range tr.Branches {
		if b.Done {
			continue
		}
		if a, ok := agents[b.ToolUseID]; ok && !a.Wrote.IsZero() && m.now.Sub(a.Wrote) < state.StillAfter {
			return true
		}
	}
	return false
}

// shippedLast says whether a finished trail's last leg shipped.
func shippedLast(tr journey.Trail) bool {
	for i := len(tr.Legs) - 1; i >= 0; i-- {
		if l := tr.Legs[i]; !l.Current {
			return l.Class == journey.Ship
		}
	}
	return false
}

// cardGoal is the session's goal and whether a model wrote it: the
// narrated goal where it has landed, else the session's first prompt,
// quoted — the ask it was opened for, which the latest prompt seldom is,
// and which on a long day is far below the card's last row.
func (m *Model) cardGoal(key string, tr journey.Trail) (string, bool) {
	if g, ok := m.narrator.(goalNarrator); ok {
		if text, ok := g.Goal(key, tr); ok {
			return text, true
		}
	}
	for _, p := range tr.Prompts {
		if p.Teammate == "" && strings.TrimSpace(p.Text) != "" {
			return askQuote(p.Text, p.Relayed), false
		}
	}
	return "", false
}

// cardNow is what the session is doing this minute, in the deck's words —
// only what the card's first row of work does not already say. That row is
// HEAD: the question, the hung call and its silence, the leg in flight.
// So a question or a call adds nothing here, and the goal takes the row;
// what HEAD cannot say — that a silence is a hang, that the red is a
// loop, how a finished session came out — is said here.
func (m *Model) cardNow(s fleet.Session, tr journey.Trail) string {
	switch {
	case s.Snap.APIError:
		if r := strings.TrimSpace(s.Snap.Reason); r != "" {
			return "dead on the API: " + r
		}
		return "dead on the API"
	case s.Snap.State == state.NeedsYou:
		return ""
	case s.Snap.State == state.Stuck, s.Snap.Exited:
		return ""
	}
	if test, _, ok := circling(tr); ok {
		return "on " + test // the badge's "loop ×4" says how many runs
	}
	if s.Snap.State == state.Working {
		return ""
	}
	return m.cardVerdict(s, tr)
}

// cardVerdict is how a finished session came out, as the card says it:
// the board's verdict without its test result (the card does not carry
// one: the trail's legs do, for whoever opens it), without the day's wait
// on you (the tally's to say), and with a last leg named once — "build
// build" was a class with no label, said twice.
func (m *Model) cardVerdict(s fleet.Session, tr journey.Trail) string {
	var parts []string
	for _, p := range verdictPartsWith(tr, m.now, s.Snap.State != state.Idle, m.agentsFor(s.Info.Key())) {
		if strings.HasPrefix(p, "✗ red") || strings.HasPrefix(p, "✓ green") || strings.HasPrefix(p, "✓ shipped") {
			continue // the shipped is the badge's to say
		}
		parts = append(parts, p)
	}
	if len(parts) > 0 {
		return strings.Join(parts, " · ")
	}
	for i := len(tr.Legs) - 1; i >= 0; i-- {
		l := tr.Legs[i]
		if l.Current {
			continue
		}
		if l.Class == journey.Ship {
			return "" // "✓ shipped" beside the badge says it
		}
		label := strings.Trim(l.Label, " .")
		if label == "" && len(l.Files) > 0 {
			label = strings.Join(l.Files[:min(2, len(l.Files))], ", ")
		}
		if label == "" || label == l.Class.String() {
			return "last: " + l.Class.String()
		}
		return "last: " + l.Class.String() + " " + label
	}
	return ""
}

// cardTally is the third row's left half: today in clauses, shed last
// first — how long, how many prompts, the work by class, the reds, the
// agents out.
func cardTally(tr journey.Trail, now time.Time, pinned bool) string {
	today := dayStart(now)
	var parts []string
	if pinned {
		parts = append(parts, "pinned")
	}
	first := time.Time{}
	prompts := 0
	for _, p := range tr.Prompts {
		if !p.At.Before(today) {
			if first.IsZero() || p.At.Before(first) {
				first = p.At
			}
			if p.Teammate == "" {
				prompts++
			}
		}
	}
	counts := map[journey.Class]int{}
	red := 0
	for _, l := range tr.Legs {
		end := l.End
		if l.Current {
			end = now
		}
		if end.Before(today) {
			continue
		}
		if first.IsZero() || l.Start.Before(first) {
			first = l.Start
		}
		counts[l.Class]++
		if strings.Contains(legBadge(l), "✗") {
			red++
		}
	}
	if first.IsZero() {
		// Nothing today: the card's rows are an earlier day's, and the
		// tally says which.
		var last time.Time
		for _, l := range tr.Legs {
			if l.End.After(last) {
				last = l.End
			}
		}
		if last.IsZero() {
			return strings.Join(parts, " · ")
		}
		return strings.Join(append(parts, "last worked "+relAge(now, last)+" ago"), " · ")
	}
	if first.Before(today) {
		first = today
	}
	parts = append(parts, "today "+relAge(now, first))
	if prompts > 0 {
		parts = append(parts, plural(prompts, "prompt"))
	}
	for _, c := range summaryOrder {
		if counts[c] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[c], c))
		}
	}
	if red > 0 {
		parts = append(parts, fmt.Sprintf("%d red", red))
	}
	if out := agentsOut(tr); out > 0 {
		parts = append(parts, fmt.Sprintf("%s%d out", glyphBranch, out))
	}
	return strings.Join(parts, " · ")
}

// cardOpts is how a card draws its rows: the column's own options, at the
// card's width.
func (m *Model) cardOpts(key string, s fleet.Session, tr journey.Trail, w int) TrailOpts {
	o := m.boardColumnOpts(key, s, tr, w, 1000)
	o.Dense, o.Pinned = false, false
	return o
}

// cardRows is how tall a card is drawn whole: its head and its rows.
func (m *Model) cardRows(key string, w int) int {
	tr, ok := m.trails[key]
	if !ok {
		return cardFrame + cardHead + 1
	}
	r, ok := m.boardRows()[key]
	if !ok {
		return cardFrame + cardHead + 1
	}
	return cardFrame + cardHead + len(m.cardWork(tr, m.cardOpts(key, m.sessions[r.sess], tr, max(1, w-2-2*cardPad))))
}

// cardMinRows is the least a band of cards is drawn at: the head and
// four rows of work, which is HEAD and the three moments before it.
const cardMinRows = cardFrame + cardHead + 4

// cardBands packs the cards into bands: as many bands as the body holds at
// the least height a card is worth, and the rows shared out between them —
// a band that needs less than its share takes what it needs and leaves the
// rest to the bands that need more. More cards at a glance beats a whole
// day on two of them; a tall screen still shows the day whole.
func (m *Model) cardBands(all []string, n, cw, body int) (bands [][]string, heights []int) {
	for pos := 0; pos < len(all); pos += n {
		bands = append(bands, all[pos:min(pos+n, len(all))])
	}
	fits := (body + 1) / (cardMinRows + 1)
	if fits < 1 {
		fits = 1
	}
	if len(bands) > fits {
		bands = bands[:fits]
	}
	need := make([]int, len(bands))
	for b, band := range bands {
		bw := bandWidth(m.width-2*edgePad, len(band), cw)
		for _, key := range band {
			if key != "" {
				need[b] = max(need[b], m.cardRows(key, bw))
			}
		}
	}
	heights = make([]int, len(bands))
	budget := body + 1 - len(bands) // each band stands off the next by a row of air
	order := make([]int, len(bands))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return need[order[a]] < need[order[b]] })
	for i, b := range order {
		share := budget / (len(order) - i)
		heights[b] = max(1, min(need[b], share))
		budget -= heights[b]
	}
	return bands, heights
}

// cardKeys is every session the card board can draw, for the poll: n to a
// band and as many bands as the body holds at the least card height —
// cardBands draws from these, and a card the poll never read is a card
// that says "reading its transcript…" for good. The selected session is
// always among them.
func (m *Model) cardKeys(n int) []string {
	body := m.height - 5 - 2
	fits := max(1, (body+1)/(cardMinRows+1))
	var order []string
	for _, i := range m.viewOrder() {
		order = append(order, m.sessions[i].Info.Key())
	}
	order = m.pinRow(order, n)
	var keys []string
	for _, k := range order[:min(n*fits, len(order))] {
		if k != "" {
			keys = append(keys, k)
		}
	}
	if m.selectedKey != "" && !contains(keys, m.selectedKey) {
		keys = append(keys, m.selectedKey)
	}
	return keys
}

// boardCard draws one card w×h.
func (m *Model) boardCard(key string, r fleetRow, w, h int) []string {
	selected := key == m.selectedKey
	edge := cardEdge
	if selected {
		edge = cardEdgeSelected
	}
	line := lipgloss.NewStyle().Foreground(edge)
	if w < 2+2*cardPad+trailMinLabel || h < 3 {
		return fit(m.cardContent(key, r, w, h), h)
	}
	body := w - 2 // inside the border
	inner := body - 2*cardPad
	rows := fit(m.cardContent(key, r, inner, h-2), h-2)
	pad := strings.Repeat(" ", cardPad)
	out := make([]string, 0, h)
	out = append(out, line.Render("╭"+strings.Repeat("─", body)+"╮"))
	for i, row := range rows {
		bg := cardPanel
		if i == 0 {
			bg = cardBarColor(selected)
		}
		out = append(out, line.Render("│")+cardFill(pad+row, body, bg)+line.Render("│"))
	}
	out = append(out, line.Render("╰"+strings.Repeat("─", body)+"╯"))
	return out
}

// cardFrame is the rows a card's border takes: its top and its bottom.
const cardFrame = 2

// cardEdge is the colour of a card's border, and cardEdgeSelected the
// selected card's: the outline the first sketch drew, a step off the
// card's ground, and the selection lit along it.
var (
	cardEdge         = lipgloss.AdaptiveColor{Light: "#b8c2cc", Dark: "#4a5d66"}
	cardEdgeSelected = lipgloss.AdaptiveColor{Light: "#5b7a90", Dark: "#9fbccc"}
)

// cardPad is the space inside a card's panel, left and right.
const cardPad = 1

// cardPanel is the card's own ground: a panel a step off the terminal's,
// so a card is a block with edges, as the first sketch drew it, rather
// than rows that happen to stand near each other.
// The default is the owner's pick, a hair off their slate terminal: the
// border is what sets a card off now, the ground only quiets it. `card_tint`
// sets it.
var cardPanel = lipgloss.AdaptiveColor{Light: "#e6eaee", Dark: "#1f2b30"}

// cardBarLit is the first row's ground, and cardBarSelected the selected
// card's: steps brighter than the panel, set with it by `card_tint`.
var (
	cardBarLit      = lipgloss.AdaptiveColor{Light: "#d2d9e0", Dark: "#313d42"}
	cardBarSelected = lipgloss.AdaptiveColor{Light: "#b4c0cc", Dark: "#475359"}
)

// SetCardTint sets a card's ground from one "#rrggbb", and lights its bar
// two steps from it; anything else keeps the default.
func SetCardTint(hex string) {
	r, g, b, ok := parseHex(hex)
	if !ok {
		return
	}
	step := func(d int) string {
		c := func(v int) int { return min(255, max(0, v+d)) }
		return fmt.Sprintf("#%02x%02x%02x", c(r), c(g), c(b))
	}
	cardPanel = lipgloss.AdaptiveColor{Light: hex, Dark: hex}
	cardBarLit = lipgloss.AdaptiveColor{Light: step(-18), Dark: step(18)}
	cardBarSelected = lipgloss.AdaptiveColor{Light: step(-40), Dark: step(40)}
}

// parseHex reads "#rrggbb".
func parseHex(hex string) (r, g, b int, ok bool) {
	if len(hex) != 7 || hex[0] != '#' {
		return 0, 0, 0, false
	}
	if _, err := fmt.Sscanf(hex[1:], "%02x%02x%02x", &r, &g, &b); err != nil {
		return 0, 0, 0, false
	}
	return r, g, b, true
}

// cardBarColor is the ground of a card's first row.
func cardBarColor(selected bool) lipgloss.AdaptiveColor {
	if selected {
		return cardBarSelected
	}
	return cardBarLit
}

// cardContent is a card's rows, w wide, before the panel is laid under them.
func (m *Model) cardContent(key string, r fleetRow, w, h int) []string {
	s := m.sessions[r.sess]
	tag := m.columnTag(key, s, w)
	m.inCard = true
	rows := []string{m.entryLinesUnder(r, w, tag)[0]}
	m.inCard = false
	tr, loaded := m.trails[key]
	if !loaded {
		return fit(append(rows, dimStyle.Render(clip(glyphGhost+" reading its transcript…", w))), h)
	}
	rows = append(rows, m.cardStatus(key, s, tr, w))
	tally := cardTally(tr, m.now, m.pinned(key))
	tag = tagBesideDigest(m.tagLadder(s), w, func(room int) string { return shedClauses(tally, room) })
	third := dimStyle.Render(shedClauses(tally, w))
	if tag != "" {
		room := w - lipgloss.Width(tag) - 2
		third = pad(dimStyle.Render(shedClauses(tally, max(0, room))), w-lipgloss.Width(tag)) + dimStyle.Render(tag)
	}
	rows = append(rows, third, "") // the head, then air before the work
	work := m.cardWork(tr, m.cardOpts(key, s, tr, w))
	if room := h - cardHead; len(work) > room && room > 0 {
		// The card is cut short: its last row says how much it left out
		// rather than ending mid-day as if that were all.
		left := len(work) - room + 1
		work = append(work[:room-1], dimStyle.Render(clip(fmt.Sprintf("↓ %d more · tab opens the trail", left), w)))
	}
	rows = append(rows, work...)
	if m.boardMuted(s) {
		for i, line := range rows {
			rows[i] = dimStyle.Render(ansi.Strip(line))
		}
		// History is dim, but its badge still says how it ended: the
		// block is what a wall of cards is counted by.
		badge := m.cardBadge(s, tr)
		if bw := lipgloss.Width(badge); bw+1 < w {
			rows[1] = badge + " " + dimStyle.Render(ansi.Strip(m.cardStatusText(key, s, tr, w-bw-1)))
		}
	}
	return rows
}

// cardFill lays a ground under a row across the card's width. Every reset
// inside the row would end the ground where a styled word ends, so it goes
// back on after each one. Without colour it is the row, padded.
func cardFill(row string, w int, bg lipgloss.AdaptiveColor) string {
	probe := lipgloss.NewStyle().Background(bg).Render("x")
	on := probe[:strings.Index(probe, "x")]
	if gap := w - lipgloss.Width(row); gap > 0 {
		row += strings.Repeat(" ", gap)
	}
	if on == "" {
		return row
	}
	return on + strings.ReplaceAll(row, "\x1b[0m", "\x1b[0m"+on) + "\x1b[0m"
}

// cardBar is a card's first row on its bar's ground.
func cardBar(row string, w int, selected bool) string {
	return cardFill(row, w, cardBarColor(selected))
}

// goalNarrator is the narrator's goal half (narrator/goal.go): a narrator
// that has none — a harness's fake — leaves the card on its prompt.
type goalNarrator interface {
	Goal(key string, tr journey.Trail) (string, bool)
	RequestGoal(key string, tr journey.Trail) bool
}

// requestGoals asks for the goal of every card whose turn is over — a
// working session's goal is still moving, and asking it on every turn
// would be the cost the goal is meant not to have — and for a working
// session's first goal, once.
func (m *Model) requestGoals(trails map[string]journey.Trail) {
	g, ok := m.narrator.(goalNarrator)
	if !ok || !cardBoard {
		return
	}
	for _, i := range m.viewOrder() {
		s := m.sessions[i]
		tr, ok := trails[s.Info.Key()]
		if !ok || len(tr.Prompts) == 0 {
			continue
		}
		if s.Snap.State == state.Working {
			// Mid-turn, a session is asked only for its first goal: one
			// that never stops working would otherwise never have one.
			if _, has := g.Goal(s.Info.Key(), tr); has {
				continue
			}
		}
		g.RequestGoal(s.Info.Key(), tr)
	}
}

// ---------------------------------------------------------------- pins

// A pin holds a card's place: pinned sessions stand first on the board, in
// the order they were pinned, whatever their state does — the board's own
// order moves a card every time a session changes state, and the one you
// are following is the one you lose. A pin lasts as long as the session is
// live; one that leaves the live fleet (its pane closed, its window ran out)
// is unpinned on the next poll, so nobody has to remember to.

// pinned says whether a session is pinned.
func (m *Model) pinned(key string) bool {
	return m.pinRank(key) >= 0
}

// pinRank is a session's place among the pins, or -1.
func (m *Model) pinRank(key string) int {
	for i, k := range m.pins {
		if k == key {
			return i
		}
	}
	return -1
}

// togglePin is `p`: pin the selected session, or unpin it.
func (m *Model) togglePin() {
	s, ok := m.selected()
	if !ok || m.archiveView || !s.Live {
		m.note = "only a live session can be pinned"
		return
	}
	key := s.Info.Key()
	if i := m.pinRank(key); i >= 0 {
		m.pins = append(m.pins[:i:i], m.pins[i+1:]...)
		m.note = "unpinned " + sessionName(s.Info)
	} else {
		m.pins = append(m.pins, key)
		m.note = "pinned " + sessionName(s.Info) + " · it leaves when the session does"
	}
	m.retireTrailMemo()
	m.savePins()
}

// prunePins drops the pins of sessions that are no longer live. A poll
// with no fleet in it says nothing about the pins and keeps them.
func (m *Model) prunePins() {
	if len(m.pins) == 0 || len(m.sessions) == 0 {
		return
	}
	live := map[string]bool{}
	for _, s := range m.sessions {
		if s.Live {
			live[s.Info.Key()] = true
		}
	}
	kept := m.pins[:0:0]
	for _, k := range m.pins {
		if live[k] {
			kept = append(kept, k)
		}
	}
	if len(kept) != len(m.pins) {
		m.pins = kept
		m.savePins()
	}
}

// LoadPins reads the pins from path and remembers the path for saves.
func (m *Model) LoadPins(path string) {
	m.pinFile = path
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var pins []string
	if json.Unmarshal(raw, &pins) == nil {
		m.pins = pins
	}
}

func (m *Model) savePins() {
	if m.pinFile == "" {
		return
	}
	raw, err := json.Marshal(m.pins)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(m.pinFile), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(m.pinFile, raw, 0o644)
}

// pinRow gives the pins a row of their own on the card board: the keys,
// pins first as boardRank puts them, with blank slots ("") after the last
// pin up to the row's end, so the first unpinned card opens the next row.
// No pins, or no unpinned card after them, and the keys are as they were.
func (m *Model) pinRow(keys []string, n int) []string {
	if n <= 1 || m.archiveView {
		return keys
	}
	pinned := 0
	for pinned < len(keys) && m.pinned(keys[pinned]) {
		pinned++
	}
	if pinned == 0 || pinned == len(keys) || pinned%n == 0 {
		return keys
	}
	out := append([]string(nil), keys[:pinned]...)
	for len(out)%n != 0 {
		out = append(out, "")
	}
	return append(out, keys[pinned:]...)
}

// boardRank is a session's place in the board's order: a pinned session
// before everything, in pin order; the rest by what they owe.
func (m *Model) boardRank(s fleet.Session) int {
	if i := m.pinRank(s.Info.Key()); i >= 0 && !m.archiveView {
		return -len(m.pins) + i - 1
	}
	return m.obligation(s)
}
