// Package ui renders the compass deck: the fleet on the left, the selected
// session's live pane in the middle, its trail on the right, and nothing that
// does not answer a question.
package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
	"github.com/deephanson94/compass/internal/tmuxop"
	"github.com/deephanson94/compass/internal/todo"
)

// The deck's three cadences. Nothing else moves: the panel draws on data, not
// on frames.
const (
	tickInterval    = time.Second            // transcripts, fleet order, the trail
	paneInterval    = 5 * time.Second        // tmux panes come and go slowly
	captureInterval = 200 * time.Millisecond // the mirror, and only the mirror
)

// The zoom levels Tab moves between (SPEC §2.3). Lv3 arrives in M3.
const (
	levelTrail     = 1 // the graph of legs
	levelWaypoints = 2 // every leg unfolded
)

type (
	tickMsg        time.Time
	paneTickMsg    time.Time
	captureTickMsg time.Time
)

type fleetMsg struct {
	sessions []fleet.Session
	err      error
	at       time.Time
	trail    journey.Trail
	hasTrail bool // the transcript was polled; without it the trail stands
	todos    []todo.Item
	trailFor string // the session the payload belongs to; "" if none was polled
}

type panesMsg struct {
	panes map[string]tmuxop.Pane
}

type captureMsg struct {
	id    string // the session the frame was captured for
	frame string
}

type revealMsg struct {
	target string
	err    error
}

// Model is the deck. It holds no session state of its own beyond what is on
// screen: the fleet Manager owns the truth, the feeds own the trails, and tmux
// owns the panes.
type Model struct {
	mgr    *fleet.Manager
	feeds  *feedStore
	runner tmuxop.Runner
	proc   tmuxop.Proc

	sessions []fleet.Session
	panes    map[string]tmuxop.Pane
	trail    journey.Trail
	todos    []todo.Item
	mirror   string
	err      error
	now      time.Time
	loaded   bool
	level    int // trail zoom: 1 legs, 2 waypoints

	selectedID string
	width      int
	height     int

	showHelp bool
	readonly bool
	note     string // one line of consequence, cleared by the next keypress

	lastNeedsYou int
}

// New returns a deck bound to a fleet Manager.
func New(mgr *fleet.Manager) *Model {
	return &Model{
		mgr:          mgr,
		feeds:        newFeedStore(),
		runner:       tmuxop.RealRunner{},
		proc:         tmuxop.RealProc{},
		now:          time.Now(),
		level:        levelTrail,
		lastNeedsYou: -1,
	}
}

// Run starts the full-screen deck. In readonly mode compass keeps its one write
// action — reveal — to itself.
func Run(mgr *fleet.Manager, readonly bool) error {
	m := New(mgr)
	m.readonly = readonly
	p := tea.NewProgram(m, tea.WithAltScreen())
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

// SetPanes gives the model the sessionID → pane mapping: the location line in
// the fleet, the source of the mirror, and the target of a reveal.
func (m *Model) SetPanes(panes map[string]tmuxop.Pane) {
	m.panes = panes
}

// SetTrail hands the model the selected session's trail for the right panel.
func (m *Model) SetTrail(tr journey.Trail) {
	m.trail = tr
}

// SetTodos hands the model the selected session's own task list — the plan the
// trail draws ahead of HEAD as ghosts.
func (m *Model) SetTodos(items []todo.Item) {
	m.todos = items
}

// SetMirror hands the model the latest captured frame for the selected session
// ("" = nothing to mirror; the panel then falls back to the transcript).
func (m *Model) SetMirror(frame string) {
	m.mirror = frame
}

// Init kicks off the first refresh and the three cadences.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.refresh(), tick(), paneTick(), captureTick(),
		m.relistPanes(), tea.SetWindowTitle(tabTitle(0)))
}

func tick() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func paneTick() tea.Cmd {
	return tea.Tick(paneInterval, func(t time.Time) tea.Msg { return paneTickMsg(t) })
}

func captureTick() tea.Cmd {
	return tea.Tick(captureInterval, func(t time.Time) tea.Msg { return captureTickMsg(t) })
}

// refresh polls every transcript off the render loop, and — for the selected
// session only — folds its events into a trail. One session's journey is the
// only one on screen; segmenting the rest would be work nobody looks at.
func (m *Model) refresh() tea.Cmd {
	mgr, feeds := m.mgr, m.feeds
	if mgr == nil {
		// A harness drives the model by hand (SetSessions); there is nothing to
		// poll and nothing it would want overwritten.
		return nil
	}
	selected, root := m.selectedID, mgr.Root()
	path := ""
	if s, ok := m.selected(); ok {
		path = s.Info.TranscriptPath
	}
	return func() tea.Msg {
		now := time.Now()
		sessions, err := mgr.Refresh(now)
		msg := fleetMsg{sessions: sessions, err: err, at: now}
		if selected != "" {
			// The plan the session keeps for itself. A missing or unreadable
			// todo file is not news: the trail simply has no future to draw.
			msg.todos, _ = todo.Read(root, selected)
			msg.trailFor = selected
		}
		if feeds != nil {
			feeds.retain(sessions)
			if selected != "" && path != "" {
				msg.trail, msg.hasTrail = feeds.poll(selected, path), true
			}
		}
		return msg
	}
}

// relistPanes re-reads the tmux server and re-pairs it with the fleet.
func (m *Model) relistPanes() tea.Cmd {
	runner, proc := m.runner, m.proc
	infos := make([]fleet.SessionInfo, 0, len(m.sessions))
	for _, s := range m.sessions {
		infos = append(infos, s.Info)
	}
	return func() tea.Msg {
		panes, err := tmuxop.ListPanes(runner)
		if err != nil || len(panes) == 0 {
			return panesMsg{panes: map[string]tmuxop.Pane{}}
		}
		return panesMsg{panes: tmuxop.MapSessions(infos, panes, proc)}
	}
}

// capture mirrors the selected pane, and only it: 200ms of one capture-pane is
// cheap, one per session would not be.
func (m *Model) capture() tea.Cmd {
	pane, ok := m.selectedPane()
	if !ok {
		return nil
	}
	runner, id := m.runner, m.selectedID
	return func() tea.Msg {
		frame, err := tmuxop.Capture(runner, pane.ID)
		if err != nil {
			return captureMsg{id: id}
		}
		return captureMsg{id: id, frame: frame}
	}
}

// Update handles the cadences, snapshots, resizes and keys.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tickMsg:
		m.now = time.Time(msg)
		return m, tea.Batch(tick(), m.refresh())

	case paneTickMsg:
		return m, tea.Batch(paneTick(), m.relistPanes())

	case captureTickMsg:
		return m, tea.Batch(captureTick(), m.capture())

	case fleetMsg:
		m.sessions, m.err, m.now, m.loaded = msg.sessions, msg.err, msg.at, true
		m.clampSelection()
		if msg.trailFor != "" && msg.trailFor == m.selectedID {
			if msg.hasTrail {
				m.trail = msg.trail
			}
			m.SetTodos(msg.todos)
		}
		return m, m.titleCmd()

	case panesMsg:
		m.panes = msg.panes
		return m, nil

	case captureMsg:
		if msg.id == m.selectedID {
			m.mirror = msg.frame
		}
		return m, nil

	case revealMsg:
		if msg.err != nil {
			m.note = "reveal failed"
		} else {
			m.note = "revealed " + msg.target
		}
		return m, nil

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

	m.note = "" // a keypress answers the last note

	switch key {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	case "tab":
		m.zoomIn()
		return m, nil
	case "shift+tab", "esc":
		m.zoomOut()
		return m, nil
	case "enter":
		return m, m.reveal()
	case "j", "down":
		m.move(1)
		return m, m.refresh()
	case "k", "up":
		m.move(-1)
		return m, m.refresh()
	case "g":
		if !m.selectOldestNeedsYou() {
			m.note = "nothing is waiting on you"
			return m, nil
		}
		return m, tea.Batch(m.refresh(), m.reveal())
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		m.selectIndex(int(key[0] - '1'))
		return m, m.refresh()
	}
	return m, nil
}

// zoomIn is Tab: the trail unfolds its waypoints. There is nothing deeper yet,
// so at Lv2 the key says where the next level went rather than doing nothing.
func (m *Model) zoomIn() {
	if m.level >= levelWaypoints {
		m.note = "deep dive (Lv3) arrives in M3"
		return
	}
	m.level = levelWaypoints
}

// zoomOut is Shift+Tab (and Esc): back to the graph. At Lv1 it is a no-op —
// zooming out of the trail would be zooming out of compass.
func (m *Model) zoomOut() {
	if m.level > levelTrail {
		m.level = levelTrail
	}
}

// reveal moves the user's tmux focus onto the selected pane — compass's only
// write, and only ever from a keypress.
func (m *Model) reveal() tea.Cmd {
	if m.readonly {
		m.note = "read-only · reveal is off"
		return nil
	}
	pane, ok := m.selectedPane()
	if !ok {
		m.note = "no tmux pane for this session"
		return nil
	}
	runner := m.runner
	return func() tea.Msg {
		return revealMsg{target: pane.Target, err: tmuxop.Reveal(runner, pane.Target, pane.ID)}
	}
}

// selected is the session the deck is pointed at.
func (m *Model) selected() (fleet.Session, bool) {
	if len(m.sessions) == 0 {
		return fleet.Session{}, false
	}
	return m.sessions[m.selectedIndex()], true
}

// selectedPane is the tmux pane the selected session lives in, if any.
func (m *Model) selectedPane() (tmuxop.Pane, bool) {
	s, ok := m.selected()
	if !ok {
		return tmuxop.Pane{}, false
	}
	pane, ok := m.panes[s.Info.ID]
	return pane, ok
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
	m.point(m.sessions[0].Info.ID)
}

// point moves the selection. Trail and mirror belong to the session that was
// selected, so they leave with it rather than lingering as somebody else's.
func (m *Model) point(id string) {
	if id == m.selectedID {
		return
	}
	m.selectedID = id
	m.trail = journey.Trail{}
	m.todos = nil
	m.mirror = ""
}

func (m *Model) selectIndex(i int) {
	if i < 0 || i >= len(m.sessions) {
		return
	}
	m.point(m.sessions[i].Info.ID)
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
	m.point(m.sessions[i].Info.ID)
}

// selectOldestNeedsYou grabs the session that has been waiting longest — the
// fleet is already sorted that way.
func (m *Model) selectOldestNeedsYou() bool {
	for _, s := range m.sessions {
		if s.Snap.State == state.NeedsYou {
			m.point(s.Info.ID)
			return true
		}
	}
	return false
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

// footerLine carries the keymap, and — briefly, on the right — whatever the
// last keypress did.
func (m *Model) footerLine(w int) string {
	keys := "1-9 select · j/k move · enter reveal · g needs-you · ? help · q quit"
	if m.showHelp {
		keys = "? or esc closes help"
	}
	left := dimStyle.Render(clip(keys, w))
	if m.note == "" {
		return left
	}
	note := dimStyle.Render(clip(m.note, w))
	gap := w - lipgloss.Width(left) - lipgloss.Width(note)
	if gap < 2 {
		return note // the note is the news; the keymap is always there
	}
	return left + strings.Repeat(" ", gap) + note
}

// column is one vertical panel of the deck.
type column struct {
	width int
	rows  []string
}

// deckLines lays the fleet beside the mirror and the trail. Wide terminals get
// all three; narrow ones drop the mirror first (it needs the most room to say
// anything), then the trail.
func (m *Model) deckLines(w, h int) []string {
	if w < minDeckCols {
		return fit(m.fleetColumn(w, h), h)
	}

	fw := fleetWidth
	if m.width >= deckWideCols {
		// The mirror takes everything the fixed columns do not need.
		mw := w - fw - trailWidth - 2*gutterWidth
		return joinColumns(h, []column{
			{fw, m.fleetColumn(fw, h)},
			{mw, m.mirrorColumn(mw, h)},
			{trailWidth, m.trailColumn(trailWidth, h)},
		})
	}

	tw := w - fw - gutterWidth
	return joinColumns(h, []column{
		{fw, m.fleetColumn(fw, h)},
		{tw, m.trailColumn(tw, h)},
	})
}

// joinColumns sets the columns side by side, held apart by hairlines — the only
// vertical strokes on the deck. A hairline stops where the content stops;
// empty rows stay empty.
func joinColumns(h int, cols []column) []string {
	stop := 0
	rows := make([][]string, len(cols))
	for i, c := range cols {
		if len(c.rows) > stop {
			stop = len(c.rows)
		}
		rows[i] = fit(c.rows, h)
	}
	if stop > h {
		stop = h
	}

	sep := " " + ruleStyle.Render("│") + " "
	lines := make([]string, h)
	for i := 0; i < h; i++ {
		if i >= stop {
			lines[i] = strings.TrimRight(rows[0][i], " ")
			continue
		}
		var b strings.Builder
		for j := range cols {
			if j > 0 {
				b.WriteString(sep)
			}
			if j == len(cols)-1 {
				b.WriteString(rows[j][i])
			} else {
				b.WriteString(pad(rows[j][i], cols[j].width))
			}
		}
		lines[i] = strings.TrimRight(b.String(), " ")
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
	return relAge(m.now, t)
}
