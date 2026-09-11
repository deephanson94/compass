package ui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/deephanson94/compass/internal/fleet"
)

// The recent band: the rows a short fleet leaves blank belong to sessions,
// not to air (#43). Under the live rows, the newest-ended archived sessions
// are drawn with the digits the live fleet has not used — a fleet of one
// draws up to eight — so the session you walked away from an hour ago is
// one key away instead of behind `A` and a search (#47). It is the
// archive's first rows, flat and newest first, never a second archive:
// no cursor walks into it, nothing pages it, and `A` is still the door to
// the rest.

// recentRow is one row of the band: the session and the digit that opens it.
type recentRow struct {
	sess int
	num  int
}

// sessionKeepsThePast says whether the session view draws the band under
// its trail. Only a fleet of one does: there the deck opens straight into
// the session, there is no level above to hold the past, and the band is
// the only place it shows — which is the whole of #47. With a fleet the
// deck draws a level up, that level already carries the band (the board's
// stranded rows, #147; the narrow list's group, #47) and the session view
// leaves it there, the footer naming the archive's door instead (#203,
// #327). The fleet is the deck's own and not the search's — `liveCount`,
// the same count that tells the help a fleet of one has no board (#53):
// a query that leaves one live row has not made a deck of one, and the
// fleet of one under a search keeps the band that answers it (#98, #102).
func (m *Model) sessionKeepsThePast() bool {
	return m.sessionView() && m.liveCount() == 1
}

// recentRows is the band as the deck would draw it with room for at most
// n rows: the archived sessions that ended most recently, newest first,
// numbered on from the live fleet's last digit. Nothing on the board or in
// the archive — the band is the live list's. Under a search it holds the
// rows that match: a search names what it found, and the band going dark
// on the one it had drawn a keystroke earlier was the opposite (#98).
func (m *Model) recentRows(n int) []recentRow {
	if m.archiveView || n <= 0 || (m.level == levelBoard && m.boardShown() && !m.onBoardBand) {
		return nil // the board's blank rows are more columns' (#43), and its strip is the door
	}
	used := 0
	for _, s := range m.sessions {
		if s.Live && m.digits[s.Info.Key()] > 0 {
			used = max(used, m.digits[s.Info.Key()])
		}
	}
	free := 9 - used
	if free <= 0 {
		return nil
	}
	var idx []int
	for i, s := range m.sessions {
		if !s.Live && archiveHeadline(s) != "" && m.matchesQuery(s) {
			// A session with no title and no prompt has nothing to go
			// back to: a bare name on the band was a slot spent (#78).
			idx = append(idx, i)
		}
	}
	// A session with legs before one without: the eight slots are for
	// sessions worth going back to, and four "Test message" sessions took
	// them from yesterday's work (#78). Within each, newest first.
	worked := func(i int) bool {
		tr, ok := m.trails[m.sessions[i].Info.Key()]
		return ok && len(tr.Legs) > 0
	}
	sort.SliceStable(idx, func(a, b int) bool {
		if wa, wb := worked(idx[a]), worked(idx[b]); wa != wb {
			return wa
		}
		return m.sessions[idx[a]].Info.LastEventAt.After(m.sessions[idx[b]].Info.LastEventAt)
	})
	if len(idx) > free {
		idx = idx[:free]
	}
	if len(idx) > n {
		idx = idx[:n]
	}
	rows := make([]recentRow, 0, len(idx))
	for i, s := range idx {
		rows = append(rows, recentRow{sess: s, num: used + i + 1})
	}
	return rows
}

// recentHeader is the band's first row: what it is, how much more the
// archive holds, and the key that browses it — the fleet's own last line
// folded into the band, so nothing is said twice.
func (m *Model) recentHeader() string {
	// Under a search the band holds the rows that match (#98): the count
	// is of what matched, in the header's own form (`2 of 4`), or the
	// line said forty-one over the one row the query left — the door's
	// own count (#169), and the hidden count's beside it (#176).
	archived := m.archiveDoorCount(m.archivedCount())
	if n := m.hiddenCount(); n > 0 {
		// The line the band folds in carried the hidden count: at a
		// hundred columns a hide left no trace on the screen (#86).
		return fmt.Sprintf("recent · %s archived · %s hidden%s", archived, m.hiddenDoorCount(n), m.archiveDoorKey())
	}
	return fmt.Sprintf("recent · %s archived%s", archived, m.archiveDoorKey())
}

// recentLines is the band drawn into avail rows of a column w wide: the
// header, then a row per session, the oldest dropped first when the rows
// run out. Nil when fewer than two rows are free, or nothing has ended.
func (m *Model) recentLines(w, avail int) []string {
	rows := m.recentRows(avail - 1)
	if len(rows) == 0 {
		return nil
	}
	// What the frame draws, as drawn: the digit pressed on this frame
	// opens one of these rows and no other (#47, #255). Every caller
	// that gets rows draws them.
	m.drawnBand = rows
	head := m.recentHeader()
	if lipgloss.Width(head) > w {
		head = strings.Replace(head, " · A browses", " · A", 1) // the key survives whole
	}
	if lipgloss.Width(head) > w {
		// The band's name goes before its counts: at eighty the header is
		// the archive's own line, "41 archived · 1 hidden · A", the shed
		// that line makes for itself two rows up (#92).
		head = strings.TrimPrefix(head, "recent · ")
	}
	if lipgloss.Width(head) > w {
		head = shedHiddenSearch(head) // the hidden count's search clause goes before the key is clipped (#176)
	}
	return append([]string{dimStyle.Render(clip(head, w))}, m.bandRows(rows, w)...)
}

// bandRows draws the band's rows with its forms decided once for all of
// them — the verdict's (#57) and the tool word's (#79) — whichever column
// the band stands in.
func (m *Model) bandRows(rows []recentRow, w int) []string {
	// One verdict form for the band: the counts go for every row when
	// any row would have to buy them out of its prompt, so two rows do
	// not keep "212✓" while two beside them drop it (#57).
	short := false
	for _, r := range rows {
		if _, full := m.recentVerdict(r, w); !full {
			short = true
		}
	}
	worded := false
	for _, r := range rows {
		if _, said, _ := m.recentKeep(r, w, short); len(strings.Fields(said)) > 1 {
			worded = true
		}
	}
	tool := m.bandSaysTool(rows, w, short)
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, m.recentLineWith(r, w, short, tool, worded))
	}
	return out
}

// bandTool is the word a band row wears for its tool: "opencode" on an
// OpenCode row always, and on every row where the fleet — live and
// archived alike — runs two tools, the row rule (#50) applied to the
// band. A fleet of claudes needs no word.
func (m *Model) bandTool(s fleet.Session) string {
	if s.Info.ToolName() != "claude" || m.toolsAnywhere() > 1 {
		return s.Info.ToolName()
	}
	return ""
}

// toolsAnywhere counts the CLIs every session, live or archived, runs under.
func (m *Model) toolsAnywhere() int {
	seen := map[string]bool{}
	for _, s := range m.sessions {
		seen[s.Info.ToolName()] = true
	}
	return len(seen)
}

// bandSaysTool says whether the band has a tool word to draw at all: a
// row draws its own where it keeps its prompt's first words beside it
// (recentRowKeepsPrompt), and sheds it where it would not — a tool word
// over a two-letter prompt named the tool and not the session. The word
// is the row's identity, like the pane tag, so a long name on one row
// does not strip the others (#79).
func (m *Model) bandSaysTool(rows []recentRow, w int, short bool) bool {
	any := false
	for _, r := range rows {
		s := m.sessions[r.sess]
		word := m.bandTool(s)
		if word == "" {
			continue
		}
		any = true
		// A row of the default tool sheds its word before a row of the
		// other tool: where an OpenCode row cannot keep "opencode", no
		// row draws a word — a band whose only word was "claude", on the
		// one row that did not need it, named the wrong thing (#80).
		if s.Info.ToolName() != "claude" && !m.recentRowKeepsTool(r, w, short, word) {
			return false
		}
	}
	return any
}

// recentRowKeepsTool says whether the row, wearing its tool word beside
// the band's verdict form, still shows its prompt's first cells.
func (m *Model) recentRowKeepsTool(r recentRow, w int, short bool, word string) bool {
	s := m.sessions[r.sess]
	prompt := strings.TrimSpace(archiveHeadline(s))
	if prompt == "" {
		return true
	}
	keep, _, _ := m.recentKeep(r, w, short)
	return keep-lipgloss.Width(sessionName(s.Info)+" · "+word+` · "`) >= recentPromptWords
}

// recentKeep is the cells a band row's name and prompt keep beside the
// verdict form the row takes — the whole verdict, its first two words, or
// its mark — and that form (#57, #67).
func (m *Model) recentKeep(r recentRow, w int, short bool) (keep int, said string, room int) {
	return m.recentKeepWorded(r, w, short, false)
}

// recentKeepWorded is recentKeep with the band's own answer to whether any
// row of it keeps a verdict word. Where one does, the row whose word the
// mark cannot say — "shipped", not "red" or "green" — keeps it rather than
// standing as the band's one wordless tick beside two reading "✓ green"
// (#161, on the band; #57's device, band-wide).
func (m *Model) recentKeepWorded(r recentRow, w int, short, worded bool) (keep int, said string, room int) {
	s := m.sessions[r.sess]
	lead := " " + strconv.Itoa(r.num) + " " + fleet.Glyph(s.Snap.State) + " "
	room = w - lipgloss.Width(lead) - lipgloss.Width(m.age(s.Info.LastEventAt)) - 1
	verdict, _ := m.recentVerdict(r, w)
	if short {
		verdict = firstWords(verdict, 2)
	}
	mark := ""
	if rs := []rune(verdict); len(rs) > 0 && strings.ContainsRune("✓✗⚑", rs[0]) {
		mark = string(rs[0])
	}
	keep = room
	rungs, floors := []string{verdict, firstWords(verdict, 2), mark}, []int{recentNameFloor, recentNameFloor, recentNameFloor}
	if two := firstWords(verdict, 2); worded && mark != "" && strings.HasSuffix(two, " shipped") {
		rungs[2] = two
		floors[2] = recentNameFloor - lipgloss.Width(two) + lipgloss.Width(mark)
	}
	for i, v := range rungs {
		if k := room - lipgloss.Width(v) - 2; v != "" && k >= floors[i] {
			return k, v, room
		}
	}
	return keep, "", room
}

// recentVerdict is the row's verdict clause and whether its full form
// fits beside the prompt's floor.
func (m *Model) recentVerdict(r recentRow, w int) (verdict string, full bool) {
	s := m.sessions[r.sess]
	verdict = m.verdictClause(s)
	if verdict == "" {
		return "", true
	}
	lead := " " + strconv.Itoa(r.num) + " " + fleet.Glyph(s.Snap.State) + " "
	room := w - lipgloss.Width(lead) - lipgloss.Width(m.age(s.Info.LastEventAt)) - 1
	return verdict, room-lipgloss.Width(verdict)-2 >= recentPromptFloor
}

// verdictClause is the first clause of a session's verdict without its
// clock — "✓ shipped", "✗ red 18✓ 2✗" — the clause that answers whether
// to reopen it (#47), for the band and the archive row (#103). Empty for a
// session with no legs.
func (m *Model) verdictClause(s fleet.Session) string {
	tr, ok := m.trails[s.Info.Key()]
	if !ok || len(tr.Legs) == 0 {
		return ""
	}
	verdict := strings.SplitN(boardVerdict(s, tr, m.now), " · ", 2)[0]
	if f := strings.Fields(verdict); len(f) > 2 && f[len(f)-1] == "ago" {
		verdict = strings.Join(f[:len(f)-2], " ")
	}
	return verdict
}

// recentLine is one band row, in shed order: digit · ○ · name · "the
// opening prompt…" · the verdict · when it ended. The verdict — the board's
// own clause, "✓ shipped", "✗ red 18✓ 2✗" — is what makes the row more
// than a name: it answers whether to reopen this one. Narrow, it goes
// first; the identity and the clock stay.
func (m *Model) recentLine(r recentRow, w int) string {
	return m.recentLineWith(r, w, false, false, false)
}

// recentLineWith is recentLine with the band's verdict form decided:
// short keeps the verdict's first two words for every row.
func (m *Model) recentLineWith(r recentRow, w int, short, tool, worded bool) string {
	s := m.sessions[r.sess]
	age := m.age(s.Info.LastEventAt)
	lead := " " + strconv.Itoa(r.num) + " " + fleet.Glyph(s.Snap.State) + " "
	name := sessionName(s.Info)
	prompt := strings.TrimSpace(archiveHeadline(s))
	// The verdict outranks the prompt's tail: "webapp · "the checkout
	// suite …  ✗ red 18✓ 2✗" answers whether to reopen, and the whole
	// prompt does not. It goes only when the prompt's own floor would —
	// and its counts go first, so a red row keeps "✗ red" where a green
	// one keeps its tick; last, the mark alone (#57, #67). The first
	// clause alone, and without its clock: "✓ shipped 2h ago" beside
	// "2h" said the hour twice.
	keep, said, room := m.recentKeepWorded(r, w, short, worded)
	// Which tool ran it: the row rule (#50), on the band — where the row
	// keeps its prompt's first cells beside the word, and shed where it
	// would not: a tool word over a three-letter prompt named the tool
	// and not the session (#79). The word is the row's own, like the
	// pane tag, so a long name on one row does not strip the others.
	if word := m.bandTool(s); tool && word != "" && m.recentRowKeepsTool(r, w, short, word) {
		name += " · " + word
	}
	if prompt != "" {
		name += " · " + askQuote(prompt, askRelayed(s))
	}
	body := clip(name, room)
	if said != "" {
		body = pad(clip(name, keep), keep) + "  " + said
	}
	body = pad(body, room)
	return dimStyle.Render(lead) + body + " " + dimStyle.Render(age)
}

// firstWords is the first n words of s.
func firstWords(s string, n int) string {
	f := strings.Fields(s)
	if len(f) <= n {
		return s
	}
	return strings.Join(f[:n], " ")
}

// recentNameFloor is the least of a band row's name and prompt kept
// beside a verdict: enough for the name and the prompt's first words.
// recentPromptFloor is what the full verdict, counts and all, must leave:
// a band at a hundred columns showed less prompt than one at eighty
// because the counts bought their cells out of it (#57).
const (
	recentNameFloor   = 18
	recentPromptFloor = 26
	recentPromptWords = 8 // the prompt's first words a tool word must leave (#79)
)

// openRecent answers a digit the live fleet does not use: the band's row
// with that number opens the archive on that session, as `A` and a search
// would have. False when no band row wears the digit.
func (m *Model) openRecent(num int) bool {
	// The band the frame drew, and no other row (#255). `recentRows` is
	// what the band *could* hold; the column draws what fits, oldest
	// dropped first, and on the board it draws none at all (#43). Asking
	// the could-hold list let a digit no row on the frame wears open the
	// archive: at eighty the fleet-hygiene list drew four live rows and
	// no band, and `5` to `9` each opened an archived session the frame
	// never named, in an order it never showed. The digit is its row's:
	// where the board drew none there is no session 5, and where the
	// list drew three there is no session 8.
	rows := m.drawnBand
	if rows == nil {
		rows = m.recentRows(9) // no frame drawn yet: what the band could hold
	}
	for _, r := range rows {
		if r.num == num {
			key := m.sessions[r.sess].Info.Key()
			m.archiveView = true
			m.restSelKey, m.selectedKey = m.selectedKey, ""
			m.restLevel = m.level // the way back lands where the digit was pressed (#54)
			m.level = levelTrail  // the archive is a list; it opens as one
			m.cursor, m.anchor = -1, -1
			m.fleetScroll = 0
			m.pointQuiet(key)
			m.clampSelection()
			m.note = "" // the footer's `A fleet` says the way home; a note cost `tab deeper` at eighty (#57)
			return true
		}
	}
	return false
}

// strandedBand is the band drawn under the board's strip, where the board's
// own rows have run out and no column can take them.
func (m *Model) strandedBand(w, avail int) []string {
	m.onBoardBand = true
	defer func() { m.onBoardBand = false }()
	return m.recentLines(w, avail)
}
