// Package ui renders the compass deck: the fleet on the left, the selected
// session's card on the right, and nothing that does not answer a question.
package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/state"
)

// tickInterval is how often the deck re-reads the transcripts. Nothing else
// moves: the panel draws on data, not on frames.
const tickInterval = time.Second

type tickMsg time.Time

type fleetMsg struct {
	sessions []fleet.Session
	err      error
	at       time.Time
}

// Model is the deck. It holds no session state of its own beyond what is on
// screen: the fleet Manager owns the truth.
type Model struct {
	mgr *fleet.Manager

	sessions []fleet.Session
	err      error
	now      time.Time
	loaded   bool

	selectedID string
	width      int
	height     int

	showHelp  bool
	showTrail bool

	lastNeedsYou int
}

// New returns a deck bound to a fleet Manager.
func New(mgr *fleet.Manager) *Model {
	return &Model{mgr: mgr, now: time.Now(), lastNeedsYou: -1}
}

// Run starts the full-screen deck.
func Run(mgr *fleet.Manager) error {
	p := tea.NewProgram(New(mgr), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// SetSize sets the render dimensions (bubbletea does this via WindowSizeMsg;
// exported so a harness can render a fixed-size view).
func (m *Model) SetSize(width, height int) {
	m.width, m.height = width, height
}

// SetSessions installs a fleet snapshot as of now, without polling.
func (m *Model) SetSessions(sessions []fleet.Session, now time.Time) {
	m.sessions = sessions
	m.now = now
	m.loaded = true
	m.clampSelection()
}

// Init kicks off the first refresh and the once-a-second tick.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.refresh(), tick(), tea.SetWindowTitle(tabTitle(0)))
}

func tick() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// refresh polls every transcript off the render loop.
func (m *Model) refresh() tea.Cmd {
	mgr := m.mgr
	return func() tea.Msg {
		now := time.Now()
		if mgr == nil {
			return fleetMsg{at: now}
		}
		sessions, err := mgr.Refresh(now)
		return fleetMsg{sessions: sessions, err: err, at: now}
	}
}

// Update handles ticks, fleet snapshots, resizes and keys.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tickMsg:
		m.now = time.Time(msg)
		return m, tea.Batch(tick(), m.refresh())

	case fleetMsg:
		m.sessions, m.err, m.now, m.loaded = msg.sessions, msg.err, msg.at, true
		m.clampSelection()
		return m, m.titleCmd()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.showHelp {
		switch key {
		case "ctrl+c":
			return m, tea.Quit
		case "?", "esc", "q", "enter", " ":
			m.showHelp = false
			return m, nil
		}
		return m, nil
	}

	switch key {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	case "tab":
		m.showTrail = true
		return m, nil
	case "shift+tab", "esc":
		m.showTrail = false
		return m, nil
	case "j", "down":
		m.move(1)
		return m, nil
	case "k", "up":
		m.move(-1)
		return m, nil
	case "g":
		m.selectOldestNeedsYou()
		return m, nil
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		m.selectIndex(int(key[0] - '1'))
		return m, nil
	}
	return m, nil
}

// selectedIndex resolves the sticky selection (by session id) to a row. The
// fleet re-sorts every second; the cursor must stay on the session, not on the
// line number.
func (m *Model) selectedIndex() int {
	for i, s := range m.sessions {
		if s.Info.ID == m.selectedID {
			return i
		}
	}
	return 0
}

func (m *Model) clampSelection() {
	if len(m.sessions) == 0 {
		m.selectedID = ""
		return
	}
	for _, s := range m.sessions {
		if s.Info.ID == m.selectedID {
			return
		}
	}
	m.selectedID = m.sessions[0].Info.ID
}

func (m *Model) selectIndex(i int) {
	if i < 0 || i >= len(m.sessions) {
		return
	}
	m.selectedID = m.sessions[i].Info.ID
}

func (m *Model) move(delta int) {
	if len(m.sessions) == 0 {
		return
	}
	i := m.selectedIndex() + delta
	if i < 0 {
		i = 0
	}
	if i >= len(m.sessions) {
		i = len(m.sessions) - 1
	}
	m.selectedID = m.sessions[i].Info.ID
}

// selectOldestNeedsYou grabs the session that has been waiting longest — the
// fleet is already sorted that way.
func (m *Model) selectOldestNeedsYou() {
	for _, s := range m.sessions {
		if s.Snap.State == state.NeedsYou {
			m.selectedID = s.Info.ID
			return
		}
	}
}

func (m *Model) needsYouCount() int {
	n := 0
	for _, s := range m.sessions {
		if s.Snap.State == state.NeedsYou {
			n++
		}
	}
	return n
}

// titleCmd emits an OSC 2 tab title only when the attention count changes, so
// an unfocused terminal tab carries the fleet's health (SPEC §2.4).
func (m *Model) titleCmd() tea.Cmd {
	n := m.needsYouCount()
	if n == m.lastNeedsYou {
		return nil
	}
	m.lastNeedsYou = n
	return tea.SetWindowTitle(tabTitle(n))
}

func tabTitle(needsYou int) string {
	if needsYou > 0 {
		return fmt.Sprintf("⌂ compass ▲%d", needsYou)
	}
	return "⌂ compass"
}

// View renders the whole deck.
func (m *Model) View() string {
	w, h := m.width, m.height
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}
	inner := w - 2*edgePad
	if inner < 10 {
		inner = w
	}

	bodyHeight := h - 5 // header, hairline, blank, hairline, footer
	if bodyHeight < 1 {
		bodyHeight = 1
	}

	var body []string
	switch {
	case m.showHelp:
		body = helpLines(inner, bodyHeight)
	case m.err != nil:
		body = fit([]string{dimStyle.Render(clip("could not read "+m.root()+": "+m.err.Error(), inner))}, bodyHeight)
	case len(m.sessions) == 0:
		body = m.emptyLines(inner, bodyHeight)
	default:
		body = m.deckLines(inner, bodyHeight)
	}

	out := make([]string, 0, h)
	out = append(out, m.headerLine(inner))
	out = append(out, rule(inner))
	out = append(out, "")
	out = append(out, body...)
	out = append(out, rule(inner))
	out = append(out, m.footerLine(inner))

	for i, line := range out {
		if line == "" {
			continue
		}
		out[i] = strings.Repeat(" ", edgePad) + line
	}
	return strings.Join(out, "\n")
}

// headerLine: the product mark on the left, the fleet's pulse on the right.
func (m *Model) headerLine(w int) string {
	left := titleStyle.Render("⌂ compass")
	right := m.statusChips()
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// statusChips renders the same counts `compass status` prints.
func (m *Model) statusChips() string {
	if !m.loaded {
		return dimStyle.Render("scanning…")
	}
	counts := map[state.State]int{}
	for _, s := range m.sessions {
		counts[s.Snap.State]++
	}
	var parts []string
	for _, st := range []state.State{state.NeedsYou, state.Stuck, state.Working, state.Idle} {
		if n := counts[st]; n > 0 {
			parts = append(parts, stateStyle(st).Render(fmt.Sprintf("%s%d", fleet.Glyph(st), n)))
		}
	}
	if len(parts) == 0 {
		return dimStyle.Render("○ all quiet")
	}
	return strings.Join(parts, "  ")
}

func (m *Model) footerLine(w int) string {
	keys := "1-9 select · j/k move · g needs-you · ? help · q quit"
	if m.showHelp {
		keys = "? or esc closes help"
	}
	return dimStyle.Render(clip(keys, w))
}

// deckLines lays the fleet beside the detail card.
func (m *Model) deckLines(w, h int) []string {
	if w < minDeckCols {
		return fit(m.fleetLines(w, h), h)
	}
	fw := fleetWidth
	dw := w - fw - gutterWidth

	leftRaw := m.fleetLines(fw, h)
	rightRaw := m.detailLines(dw, h)
	left := fit(leftRaw, h)
	right := fit(rightRaw, h)

	// One hairline holds the two columns apart — the only vertical stroke on
	// the deck. It stops where the content stops; empty rows stay empty.
	stop := len(leftRaw)
	if len(rightRaw) > stop {
		stop = len(rightRaw)
	}
	if stop > h {
		stop = h
	}

	sep := ruleStyle.Render("│")
	lines := make([]string, h)
	for i := 0; i < h; i++ {
		if i >= stop {
			lines[i] = strings.TrimRight(left[i], " ")
			continue
		}
		lines[i] = strings.TrimRight(pad(left[i], fw)+" "+sep+" "+right[i], " ")
	}
	return lines
}

// root is the watched directory, safe to call without a Manager.
func (m *Model) root() string {
	if m.mgr == nil {
		return ""
	}
	return m.mgr.Root()
}

// emptyLines is a designed empty state, never a blank panel (SPEC §4).
func (m *Model) emptyLines(w, h int) []string {
	root := m.root()
	body := []string{
		textStyle.Render(clip("no sessions found under "+root, w)),
		"",
		dimStyle.Render(clip("compass watches "+root+"/projects for live", w)),
		dimStyle.Render(clip("Claude Code sessions. Start one in any terminal", w)),
		dimStyle.Render(clip("and it appears here within a second.", w)),
	}
	top := (h - len(body)) / 3
	if top < 0 {
		top = 0
	}
	out := make([]string, 0, h)
	for i := 0; i < top; i++ {
		out = append(out, "")
	}
	return fit(append(out, body...), h)
}

// fit pads or truncates a block to exactly h lines.
func fit(lines []string, h int) []string {
	if len(lines) > h {
		return lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return lines
}

// age renders the time since t, relative to the model's clock.
func (m *Model) age(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return state.ShortDuration(m.now.Sub(t))
}
