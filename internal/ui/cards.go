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

// hungAfter is how long a stuck session's silence lasts before its card
// calls it hung rather than silent: stuck is ninety seconds into a quiet
// call, which a long build is; half an hour is a call that is not coming
// back.
var hungAfter = 30 * time.Minute

// SetHungAfter sets the silence a card calls hung; zero or less keeps the default.
func SetHungAfter(d time.Duration) {
	if d > 0 {
		hungAfter = d
	}
}

// cardColMin is a card's least width: from here a row carries its label,
// its badge and its figure whole. 84 puts four across a 32-inch screen,
// where 60 put six and every row still clipped.
var cardColMin = 84

// SetCardWidth sets a card's least width; below 60 it keeps the default.
func SetCardWidth(w int) {
	if w >= 60 {
		cardColMin = w
	}
}

const (
	cardColMax  = 140
	cardHead    = 3  // the fleet row, the goal row, the tally row
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
			rows = append(rows, dayRule(day, o.Now, o.Width))
		}
		prevDay = day
		rows = append(rows, m.cardRow(tr, it, o)...)
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

// cardBadge is the card's health at a glance: a block of colour the eye
// finds across a wall of cards before it reads a word — amber for a
// question, red for a hang, a loop or a dead API, green for work and for
// a finished session that shipped green, grey for the rest. The glyph and
// the word say it again for monochrome.
func (m *Model) cardBadge(s fleet.Session, tr journey.Trail) string {
	glyph, word, col := "○", "idle", colDim
	_, _, loop := circling(tr)
	switch {
	case s.Snap.APIError:
		glyph, word, col = glyphAPIError, "dead", colStuck
	case s.Waiting:
		glyph, word, col = fleet.Glyph(state.NeedsYou), "unanswered", colNeedsYou
	case s.Snap.State == state.NeedsYou:
		glyph, word, col = fleet.Glyph(state.NeedsYou), "needs you", colNeedsYou
	case s.Snap.State == state.Stuck && m.now.Sub(headSince(s)) >= hungAfter:
		glyph, word, col = fleet.Glyph(state.Stuck), "hung", colStuck
	case s.Snap.State == state.Stuck:
		glyph, word, col = fleet.Glyph(state.Stuck), "stuck", colStuck
	case loop:
		glyph, word, col = glyphCircling, "looping", colStuck
	case s.Snap.State == state.Working:
		glyph, word, col = fleet.Glyph(state.Working), "working", colWorking
	case redNow(tr):
		glyph, word = "✗", "red"
	case shippedLast(tr):
		glyph, word, col = "✓", "done", colWorking
	}
	style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#111111")).Background(col)
	if col == colDim {
		// Grey on grey read as no badge at all: the quiet ones get a
		// light grey block with dark text, quieter than a colour and
		// still a block the eye can count.
		style = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#ffffff", Dark: "#111111"}).
			Background(lipgloss.AdaptiveColor{Light: "#6b6b6b", Dark: "#a0a0a0"})
	}
	return style.Render(" " + glyph + " " + word + " ")
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
	case s.Snap.State == state.Stuck:
		if quiet := m.now.Sub(headSince(s)); quiet >= hungAfter {
			return "no output past " + relDuration(hungAfter)
		}
		return ""
	}
	if test, runs, ok := circling(tr); ok {
		return fmt.Sprintf("on %s · red %d runs running", test, runs)
	}
	if s.Snap.State == state.Working {
		return ""
	}
	return m.cardVerdict(s, tr)
}

// cardVerdict is how a finished session came out, as the card says it:
// the board's verdict without what the badge beside it already says (the
// red), without the day's wait on you (the tally's to say), and with a
// last leg named once — "build build" was a class with no label, said twice.
func (m *Model) cardVerdict(s fleet.Session, tr journey.Trail) string {
	var parts []string
	for _, p := range verdictPartsWith(tr, m.now, s.Snap.State != state.Idle, m.agentsFor(s.Info.Key())) {
		if strings.HasPrefix(p, "✗ red") && redNow(tr) {
			continue
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
		return cardHead + 1
	}
	r, ok := m.boardRows()[key]
	if !ok {
		return cardHead + 1
	}
	return cardHead + len(m.cardWork(tr, m.cardOpts(key, m.sessions[r.sess], tr, w)))
}

// cardMinRows is the least a band of cards is drawn at: the head and
// four rows of work, which is HEAD and the three moments before it.
const cardMinRows = cardHead + 4

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
			need[b] = max(need[b], m.cardRows(key, bw))
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
	var keys []string
	for _, i := range m.viewOrder() {
		if len(keys) == n*fits {
			break
		}
		keys = append(keys, m.sessions[i].Info.Key())
	}
	if m.selectedKey != "" && !contains(keys, m.selectedKey) {
		keys = append(keys, m.selectedKey)
	}
	return keys
}

// boardCard draws one card w×h.
func (m *Model) boardCard(key string, r fleetRow, w, h int) []string {
	s := m.sessions[r.sess]
	tag := m.columnTag(key, s, w)
	rows := []string{m.entryLinesUnder(r, w, tag)[0]}
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
	rows = append(rows, third)
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
	rows[0] = cardBar(rows[0], w, key == m.selectedKey)
	return fit(rows, h)
}

// cardBar tints a card's first row across the card's width: a title bar,
// so where one card begins is seen before anything on it is read. The
// selected card's bar is the brighter one.
func cardBar(row string, w int, selected bool) string {
	bg := lipgloss.AdaptiveColor{Light: "#dfe4ea", Dark: "#2b323c"}
	if selected {
		bg = lipgloss.AdaptiveColor{Light: "#c3ccd6", Dark: "#3f5163"}
	}
	probe := lipgloss.NewStyle().Background(bg).Render("x")
	on := probe[:strings.Index(probe, "x")]
	if on == "" {
		return row // no colour: the bar has nothing to draw with
	}
	// Every reset inside the row would end the tint where a styled word
	// ends; the tint goes back on after each one.
	row = strings.ReplaceAll(row, "\x1b[0m", "\x1b[0m"+on)
	if gap := w - lipgloss.Width(row); gap > 0 {
		row += strings.Repeat(" ", gap)
	}
	return on + row + "\x1b[0m"
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

// boardRank is a session's place in the board's order: a pinned session
// before everything, in pin order; the rest by what they owe.
func (m *Model) boardRank(s fleet.Session) int {
	if i := m.pinRank(s.Info.Key()); i >= 0 && !m.archiveView {
		return -len(m.pins) + i - 1
	}
	return m.obligation(s)
}
