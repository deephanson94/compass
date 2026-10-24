package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
)

// The card board's pins. Each test turns the card board on for itself and
// off again; the column board is the default and the rest of the suite is
// its own.

func withCards(t *testing.T) {
	t.Helper()
	cardBoard = true
	t.Cleanup(func() { cardBoard = false })
}

// sceneCards is the fleet from the person's own screen, a day into it:
// one asking, one hung for hours, one stuck for minutes, one looping, one
// working with agents out, one finished today and unread, one whose work
// was all yesterday, and one that only just started.
func sceneCards() scene {
	n := sceneNow // 18:00, so a leg at 08:00 is today and one at 18:00 the day before is yesterday
	tr := map[string]journey.Trail{}
	var ss []fleet.Session
	add := func(s fleet.Session, t journey.Trail) {
		ss = append(ss, s)
		tr[s.Info.Key()] = t
	}

	// Asking: a question under its HEAD.
	ask := trailOf(n.Add(-9*time.Hour), "i have many claude session ongoing at once", false,
		legSpec{journey.Scout, "session registry", 20 * time.Minute, []string{"registry.go"}, "", nil},
		legSpec{journey.Build, "pane pairing", 40 * time.Minute, []string{"panes.go"}, "", nil},
		legSpec{journey.Test, "go test ./...", 3 * time.Minute, nil, "212✓", nil},
		legSpec{journey.Ship, "commit", 1 * time.Minute, nil, "", nil})
	ask = withPrompt(ask, n.Add(-30*time.Minute), "does prefix+j always re-read the transcript?")
	ask.Legs = append(ask.Legs, journey.Leg{Class: journey.Scout, Label: "prefix+j handler", Start: n.Add(-28 * time.Minute), End: n.Add(-8 * time.Minute), Current: true, Votes: 3})
	add(sess("harness", "HARNESS_RELATED", "/home/user/harness", "main", "i have many claude session ongoing at once", state.NeedsYou, n.Add(-8*time.Minute), journey.Scout, "", "question", "Re-read on every prefix+j, or only when the file grew? [always / when grown]"), ask)

	// Hung: silent for seventeen hours inside one shell call.
	hung := trailOf(n.Add(-20*time.Hour), "flow view rebuild", true,
		legSpec{journey.Build, "flow view rebuild", 50 * time.Minute, []string{"flow.go"}, "", nil},
		legSpec{journey.Test, "pytest: sheet and MTP sample", 5 * time.Minute, nil, "103✓ 2✗", []string{"test_sheet"}},
		legSpec{journey.Scout, "skill script", 2 * time.Minute, nil, "", nil})
	s := sess("flow", "feat-flow-view", "/home/user/flow", "feat/flow", "flow view rebuild", state.Stuck, n.Add(-17*time.Hour), journey.Scout, "", "no output for 17h mid-turn", "Bash: python3 ~/.claude/skills/render.py")
	s.Info.LastEventAt = n.Add(-17 * time.Hour)
	add(s, hung)

	// Stuck for minutes: silent, not hung.
	short := trailOf(n.Add(-40*time.Minute), "backfill last week's shards", true,
		legSpec{journey.Scout, "the shard layout", 8 * time.Minute, []string{"shards.py"}, "", nil},
		legSpec{journey.Build, "backfill.py", 20 * time.Minute, []string{"backfill.py"}, "", nil})
	s = sess("etl", "etl", "/home/user/etl", "main", "backfill last week's shards", state.Stuck, n.Add(-6*time.Minute), journey.Build, "", "no output for 6m mid-turn", "Bash: python backfill.py --all")
	s.Info.LastEventAt = n.Add(-6 * time.Minute)
	add(s, short)

	// Looping: the same test red four runs running.
	loop := []legSpec{{journey.Scout, "middleware.py", 5 * time.Minute, []string{"middleware.py"}, "", nil}}
	for i := 0; i < 4; i++ {
		loop = append(loop,
			legSpec{journey.Fix, []string{"the expiry check", "the clock skew", "the refresh window", "the revoked path"}[i], 6 * time.Minute, []string{"tokens.py"}, "", nil},
			legSpec{journey.Test, "pytest", 3 * time.Minute, nil, "18✓ 2✗", []string{"test_refresh_expired"}})
	}
	loop = append(loop, legSpec{journey.Fix, "tokens.py", 2 * time.Minute, []string{"tokens.py"}, "", nil})
	add(sess("api", "tinker-lfm-cmdline", "/home/user/api", "port", "port LFM2 to the cmdline runner", state.Working, n.Add(-30*time.Second), journey.Fix, "18✓ 2✗", "tool call in flight", "Edit: tokens.py"),
		trailOf(n.Add(-80*time.Minute), "port LFM2 to the cmdline runner", true, loop...))

	// Working, agents out.
	work := trailOf(n.Add(-3*time.Hour), "tinker architecture diagram and slides", true,
		legSpec{journey.Scout, "the stage list", 10 * time.Minute, []string{"stages.md"}, "", nil},
		legSpec{journey.Build, "render loops diagram", 30 * time.Minute, []string{"loops.svg"}, "", nil},
		legSpec{journey.Docs, "slide notes", 10 * time.Minute, []string{"slides.md"}, "", nil},
		legSpec{journey.Build, "render bot image", 4 * time.Minute, []string{"bot.svg"}, "", nil})
	work = withBranch(work, 2, "review the slide copy", n.Add(-6*time.Minute), false, "")
	add(sess("stages", "tinker-stages-architecture", "/home/user/stages", "main", "tinker architecture diagram and slides", state.Working, n.Add(-20*time.Second), journey.Build, "", "tool call in flight", "Write: bot.svg"), work)

	// Finished today, unread.
	done := trailOf(n.Add(-5*time.Hour), "add retries to the s3 client", false,
		legSpec{journey.Scout, "the client", 6 * time.Minute, []string{"client.go"}, "", nil},
		legSpec{journey.Build, "retry with backoff", 25 * time.Minute, []string{"client.go"}, "", nil},
		legSpec{journey.Test, "go test ./s3", 2 * time.Minute, nil, "40✓", nil},
		legSpec{journey.Ship, "commit", 1 * time.Minute, nil, "", nil})
	add(sess("s3", "alg3_tools", "/home/user/s3", "main", "add retries to the s3 client", state.Idle, n.Add(-4*time.Hour), journey.Ship, "add retries to the s3 client", "", ""), done)

	// All of it yesterday: still live, nothing today.
	old := trailOf(n.Add(-30*time.Hour), "how do i run the demo with avatar?", false,
		legSpec{journey.Scout, "board with avatar idea", 10 * time.Minute, nil, "", nil},
		legSpec{journey.Scout, "why 29m port fails", 10 * time.Minute, nil, "", nil},
		legSpec{journey.Build, "investigate why S9 failed", 5 * time.Minute, nil, "", nil})
	old = withPrompt(old, n.Add(-29*time.Hour), "there are 4 dots under port/quantize, what are they?")
	add(sess("demo", "tinker_demo_lfm2", "/home/user/demo", "main", "how do i run the demo with avatar?", state.Idle, n.Add(-29*time.Hour), journey.Scout, "", "", ""), old)

	// Just started: one prompt, thinking.
	add(sess("gemma", "pr-gemmav4-qat-ses1", "/home/user/gemma", "pr", "/kickoff tinker-gemmav4-release", state.Working, n.Add(-34*time.Second), journey.Build, "", "thinking", "thinking…"),
		journey.Trail{Prompts: []journey.Prompt{{Text: "/kickoff tinker-gemmav4-release", At: n.Add(-40 * time.Second)}}})

	ids := []string{"harness", "flow", "etl", "api", "stages", "s3", "demo", "gemma"}
	panes, order := paneMap(ids, []string{"w:1.0", "w:3.0", "w:4.0", "w:5.0", "w:6.0", "w:7.0", "w:8.0", "w:9.0"})
	return scene{name: "cards", story: "A day into eight sessions on a 32-inch screen.", sessions: ss, trails: tr, panes: panes, order: order}
}

// card returns the rows of one session's card, found by its name, drawn
// at the width the board gives it and tall enough to stand whole, with
// the styling taken off.
func card(t *testing.T, m *Model, name string) []string {
	t.Helper()
	m.View() // the frame's own bookkeeping: digits, the poll's trails
	_, cw := boardColumns(m.width-2*edgePad, m.drawnCount(m.viewOrder()))
	rowOf := m.boardRows()
	for key, r := range rowOf {
		if sessionName(m.sessions[r.sess].Info) != name {
			continue
		}
		var out []string
		for _, l := range m.boardCard(key, r, cw, 200) {
			out = append(out, strings.TrimRight(ansi.Strip(l), " "))
		}
		return out
	}
	t.Fatalf("no card named %s", name)
	return nil
}

// cardUntilBlank cuts a card's rows at its first blank row: the band's end.
func cardUntilBlank(rows []string) []string {
	for i, r := range rows {
		if strings.TrimSpace(r) == "" {
			return rows[:i]
		}
	}
	return rows
}

func TestCardsWidthDisciplineAndDump(t *testing.T) {
	withCards(t)
	forceASCII(t)
	out := os.Getenv("COMPASS_SCENARIO_OUT")
	scenes := append([]scene{sceneCards()}, allScenes()...)
	sizes := [][2]int{{80, 24}, {120, 34}, {152, 40}, {220, 48}, {290, 80}}
	for _, sc := range scenes {
		for _, size := range sizes {
			w, h := size[0], size[1]
			m := sceneModel(sc, w, h)
			keys := append([]string{"j", "p", "j", "j", "p", "tab", "shift+tab"}, canonicalKeys...)
			var dump strings.Builder
			for _, k := range append(keys, "") {
				frame := m.View()
				lines := strings.Split(frame, "\n")
				if len(lines) != h {
					t.Errorf("%s %dx%d before %q: frame is %d rows, want %d", sc.name, w, h, k, len(lines), h)
				}
				for _, line := range lines {
					if x := lipgloss.Width(line); x > w {
						t.Errorf("%s %dx%d before %q: line runs past the terminal (%d): %q", sc.name, w, h, k, x, line)
					}
				}
				if out != "" {
					fmt.Fprintf(&dump, "=== before %q ===\n%s\n\n", k, frame)
				}
				if k == "" {
					break
				}
				pressKey(m, k)
				poll(m, sc)
			}
			if out != "" {
				_ = os.MkdirAll(out, 0o755)
				_ = os.WriteFile(filepath.Join(out, fmt.Sprintf("cards-%s-%dx%d.txt", sc.name, w, h)), []byte(dump.String()), 0o644)
			}
		}
	}
}

func TestCardShowsTheLatestDozenAndDimsOlderPrompts(t *testing.T) {
	withCards(t)
	forceASCII(t)
	m := sceneModel(sceneVeryLong(), 220, 80)
	rows := cardUntilBlank(card(t, m, "auth"))
	if len(rows) > cardHead+cardRowsMax+1 {
		t.Fatalf("a busy day's card has %d rows, want at most %d:\n%s", len(rows), cardHead+cardRowsMax+1, strings.Join(rows, "\n"))
	}
	if !strings.Contains(rows[len(rows)-1], "earlier today") {
		t.Fatalf("a capped card does not say how much it left out: %q", rows[len(rows)-1])
	}
}

func TestCardOlderPromptsAreDim(t *testing.T) {
	withCards(t)
	m := sceneModel(sceneCards(), 290, 80)
	tr := m.trails[sessionKey("harness")]
	o := m.cardOpts(sessionKey("harness"), m.sessions[0], tr, 80)
	items := cardItems(tr, m.now)
	var older, latest string
	for _, it := range items {
		if it.prompt == 0 {
			older = m.cardRow(tr, it, o)[0]
		}
		if it.prompt == 1 {
			latest = m.cardRow(tr, it, o)[0]
		}
	}
	if older != dimStyle.Render(ansi.Strip(older)) {
		t.Errorf("an older prompt is drawn at full brightness: %q", older)
	}
	if latest == dimStyle.Render(ansi.Strip(latest)) && dimStyle.Render("x") != "x" {
		t.Errorf("the latest prompt is dimmed: %q", latest)
	}
}

func TestCardsAreWideAndFewAcross(t *testing.T) {
	withCards(t)
	// A 32-inch screen: six cards across at 60 still clipped every row.
	n, w := boardColumns(390-2*edgePad, 9)
	if n != 4 || w < cardColMin {
		t.Fatalf("a 390-column board holds %d cards %d wide; want 4 of at least %d", n, w, cardColMin)
	}
	cardBoard = false
	if n, _ := boardColumns(390-2*edgePad, 9); n <= 4 {
		t.Fatalf("the column board should still fit more than four columns, got %d", n)
	}
}

func TestCardNewestFirstHeadOnTop(t *testing.T) {
	withCards(t)
	forceASCII(t)
	m := sceneModel(sceneCards(), 290, 80)
	rows := cardUntilBlank(card(t, m, "api"))
	if len(rows) < cardHead+3 {
		t.Fatalf("card too short: %q", rows)
	}
	work := rows[cardHead:]
	// HEAD is the fix on tokens.py, the newest leg: the first row of work.
	if !strings.Contains(work[0], "fix") || !strings.Contains(work[0], "tokens.py") {
		t.Fatalf("the card's first row of work is not HEAD: %q\n%s", work[0], strings.Join(rows, "\n"))
	}
	// And the trail's oldest leg, the scout of middleware.py, is below
	// every fix and test: time runs up the card.
	at := func(sub string) int {
		for i, r := range work {
			if strings.Contains(r, sub) {
				return i
			}
		}
		return -1
	}
	if s, f := at("middleware.py"), at("the expiry check"); s < 0 || f < 0 || s < f {
		t.Fatalf("oldest leg is not under the newer ones (scout %d, first fix %d):\n%s", s, f, strings.Join(work, "\n"))
	}
	// The prompt that started it is the last row: it is the oldest moment.
	if !strings.Contains(work[len(work)-1], "port LFM2") {
		t.Fatalf("the opening prompt is not the card's last row: %q", work[len(work)-1])
	}
}

func TestCardTodayOnlyAndYesterdayRule(t *testing.T) {
	withCards(t)
	forceASCII(t)
	m := sceneModel(sceneCards(), 290, 80)
	m.View()
	// A day-old trail: nothing today, so its last moments under a rule
	// that says which day they are, and a tally that says when it worked.
	rows := cardUntilBlank(card(t, m, "demo"))
	joined := strings.Join(rows, "\n")
	if !strings.Contains(joined, "── yesterday") {
		t.Fatalf("a card of yesterday's work has no day rule:\n%s", joined)
	}
	if !strings.Contains(rows[2], "last worked") {
		t.Fatalf("a card with nothing today says no when in its tally: %q", rows[2])
	}
	// The hung session's day began yesterday: today's tally counts nothing
	// from before midnight.
	rows = cardUntilBlank(card(t, m, "flow"))
	if strings.Contains(rows[2], "today 20h") {
		t.Fatalf("the tally reaches back before midnight: %q", rows[2])
	}
}

func TestCardStatusSaysWhatNow(t *testing.T) {
	withCards(t)
	forceASCII(t)
	m := sceneModel(sceneCards(), 290, 80)
	m.View()
	cases := []struct{ name, want, not string }{
		{"flow", "◍ hung  ", ""},   // past hung_after
		{"etl", "", "hung"},        // stuck, but not for long: HEAD's "silent 6m" says it
		{"api", "↻ looping  ", ""}, // the loop, which HEAD's "fix tokens.py" cannot say
		{"s3", "✓ done  ", ""},     // how a finished session came out
		{"harness", "", "Re-read"}, // the question is HEAD's row, not said twice
	}
	for _, c := range cases {
		rows := card(t, m, c.name)
		if c.want != "" && !strings.Contains(rows[1], c.want) {
			t.Errorf("%s: status row %q does not say %q", c.name, rows[1], c.want)
		}
		if c.not != "" && strings.Contains(rows[1], c.not) {
			t.Errorf("%s: status row %q says %q, which another row of the card says", c.name, rows[1], c.not)
		}
	}
	// No goal narrated: the first prompt stands in, quoted.
	rows := card(t, m, "harness")
	if !strings.Contains(rows[1], `"i have many claude session ongoing at once"`) {
		t.Errorf("without a goal the status row should quote the first prompt whole: %q", rows[1])
	}
	// The question HEAD asks is whole on the card, under HEAD's row.
	if got := strings.Join(cardUntilBlank(rows), " "); !strings.Contains(got, "[always / when grown]") {
		t.Errorf("the question is clipped on the card:\n%s", strings.Join(rows, "\n"))
	}
	// The hung threshold is the config's.
	SetHungAfter(24 * time.Hour)
	t.Cleanup(func() { hungAfter = 30 * time.Minute })
	m.retireTrailMemo()
	if rows := card(t, m, "flow"); strings.Contains(rows[1], "hung") || !strings.Contains(rows[1], "stuck") {
		t.Errorf("hung_after of a day still calls 17h hung: %q", rows[1])
	}
}

// goalFake is a narrator with goals: it says one for every session it is
// asked about, and counts the asks.
type goalFake struct {
	fakeNarrator
	mu    sync.Mutex
	goals map[string]string
	asks  map[string]int
}

func (g *goalFake) Goal(key string, _ journey.Trail) (string, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	text, ok := g.goals[key]
	return text, ok
}

func (g *goalFake) RequestGoal(key string, _ journey.Trail) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.asks == nil {
		g.asks = map[string]int{}
	}
	g.asks[key]++
	return true
}

func TestCardGoalIsNarratedAndAskedOnlyAtTurnEnd(t *testing.T) {
	withCards(t)
	forceASCII(t)
	sc := sceneCards()
	g := &goalFake{goals: map[string]string{sessionKey("harness"): "compass multi-session triage"}}
	m := sceneModel(sc, 290, 80)
	m.SetNarrator(g)
	poll(m, sc)
	rows := card(t, m, "harness")
	if !strings.HasSuffix(strings.TrimSpace(rows[1]), "▲ needs you  compass multi-session triage") {
		t.Fatalf("the narrated goal does not lead the status row: %q", rows[1])
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, id := range []string{"api", "stages", "gemma"} {
		if g.asks[sessionKey(id)] > 0 {
			t.Errorf("%s is working: its goal was asked mid-turn", id)
		}
	}
	for _, id := range []string{"harness", "flow", "s3"} {
		if g.asks[sessionKey(id)] == 0 {
			t.Errorf("%s's turn is over: its goal was never asked", id)
		}
	}
}

func TestCardPinHoldsThePlaceAndLeavesWithTheSession(t *testing.T) {
	withCards(t)
	forceASCII(t)
	sc := sceneCards()
	m := sceneModel(sc, 290, 80)
	first := func() string {
		order := m.viewOrder()
		return sessionName(m.sessions[order[0]].Info)
	}
	if first() == "s3" {
		t.Fatal("fixture: the calm session is already first")
	}
	m.point(sessionKey("s3"))
	pressKey(m, "p")
	poll(m, sc)
	if first() != "s3" {
		t.Fatalf("a pinned card is not first: %s is", first())
	}
	if rows := card(t, m, "s3"); !strings.HasPrefix(strings.TrimSpace(rows[2]), "pinned") {
		t.Fatalf("a pinned card does not say so: %q", rows[2])
	}
	// The pin outlives a state change: the session that asks still stands behind it.
	if pos := m.boardRank(m.sessions[m.viewOrder()[1]]); pos < 0 {
		t.Fatalf("only one session is pinned, yet the second card ranks as pinned")
	}
	// The session leaves the live fleet: the pin goes on the next poll.
	sessions := append([]fleet.Session(nil), m.sessions...)
	for i := range sessions {
		if sessions[i].Info.Key() == sessionKey("s3") {
			sessions[i].Live = false
		}
	}
	m.SetSessions(sessions, m.now)
	if m.pinned(sessionKey("s3")) {
		t.Fatal("a session that left the live fleet is still pinned")
	}
	// p twice is no pin.
	m.point(sessionKey("etl"))
	pressKey(m, "p")
	pressKey(m, "p")
	if m.pinned(sessionKey("etl")) {
		t.Fatal("p twice left the session pinned")
	}
}

func TestCardPinsPersist(t *testing.T) {
	withCards(t)
	path := filepath.Join(t.TempDir(), "pins.json")
	m := sceneModel(sceneCards(), 290, 80)
	m.LoadPins(path)
	m.point(sessionKey("etl"))
	pressKey(m, "p")
	again := sceneModel(sceneCards(), 290, 80)
	again.LoadPins(path)
	if !again.pinned(sessionKey("etl")) {
		t.Fatal("a pin did not survive a restart")
	}
	// An empty poll says nothing about pins.
	again.SetSessions(nil, again.now)
	if !again.pinned(sessionKey("etl")) {
		t.Fatal("a poll with no fleet unpinned a session")
	}
}

func TestPinKeyIsTheCardBoardsOnly(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneCards(), 290, 80)
	m.point(sessionKey("etl"))
	pressKey(m, "p")
	if m.pinned(sessionKey("etl")) {
		t.Fatal("p pinned a session on the column board")
	}
}

func TestCardCutShortSaysHowMuchMore(t *testing.T) {
	withCards(t)
	forceASCII(t)
	// A day of two hundred legs: the band cannot hold every row of today.
	m := sceneModel(sceneVeryLong(), 152, 40)
	frame := ansi.Strip(m.View())
	if !strings.Contains(frame, "more · tab opens the trail") {
		t.Fatalf("a card cut short does not say so:\n%s", frame)
	}
}

func TestCardLeavesYesterdayToTheTrailWhenTodayIsFull(t *testing.T) {
	withCards(t)
	forceASCII(t)
	sc := sceneCards()
	// The finished session's day was full, and it also worked the
	// evening before: yesterday is the session view's, not the card's.
	key := sessionKey("s3")
	yesterday := trailOf(sceneNow.Add(-26*time.Hour), "the night before", false,
		legSpec{journey.Scout, "last night's look", 10 * time.Minute, nil, "", nil},
		legSpec{journey.Build, "last night's build", 10 * time.Minute, nil, "", nil})
	tr := sc.trails[key]
	tr.Prompts = append(yesterday.Prompts, tr.Prompts...)
	tr.Legs = append(yesterday.Legs, tr.Legs...)
	sc.trails[key] = tr
	m := sceneModel(sc, 290, 80)
	rows := strings.Join(cardUntilBlank(card(t, m, "s3")), "\n")
	if strings.Contains(rows, "last night") || strings.Contains(rows, "yesterday") {
		t.Fatalf("a card with a full today draws yesterday too:\n%s", rows)
	}
	if !strings.Contains(rows, "retry with backoff") {
		t.Fatalf("today's work is missing:\n%s", rows)
	}
}

func TestCardPollReadsEveryBandNotJustTheFirst(t *testing.T) {
	withCards(t)
	m := sceneModel(sceneCards(), 290, 80) // three across, three bands
	polled := map[string]bool{}
	for _, tg := range m.boardTargets() {
		polled[tg.key] = true
	}
	for _, id := range []string{"harness", "flow", "etl", "api", "stages", "gemma", "s3", "demo"} {
		if !polled[sessionKey(id)] {
			t.Errorf("%s has a card on the board but its transcript is never polled", id)
		}
	}
}

// TestCardsShot writes the card board in colour, for a person to look at:
// COMPASS_CARDS_SHOT=/tmp/shot.ans go test ./internal/ui -run CardsShot
func TestCardsShot(t *testing.T) {
	out := os.Getenv("COMPASS_CARDS_SHOT")
	if out == "" {
		t.Skip("COMPASS_CARDS_SHOT names the file")
	}
	withCards(t)
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	w, h := 390, 64
	if v := os.Getenv("COMPASS_CARDS_SIZE"); v != "" {
		fmt.Sscanf(v, "%dx%d", &w, &h)
	}
	m := sceneModel(sceneCards(), w, h)
	if err := os.WriteFile(out, []byte(m.View()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCardsStandApartOnWhitespaceAndABar(t *testing.T) {
	withCards(t)
	forceASCII(t)
	m := sceneModel(sceneCards(), 290, 80)
	for _, l := range strings.Split(ansi.Strip(m.View()), "\n")[3:] {
		if strings.Contains(l, "│") {
			t.Fatalf("cards are drawn apart by a hairline, not by space: %q", l)
		}
	}
	lipgloss.SetColorProfile(termenv.TrueColor)
	bar := cardBar("x", 10, false)
	if !strings.Contains(bar, "48;") || lipgloss.Width(bar) != 10 {
		t.Fatalf("a card's first row has no tinted bar across its width: %q", bar)
	}
}
