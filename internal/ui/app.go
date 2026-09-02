// Package ui renders the compass deck: the fleet on the left, the selected
// session's live pane in the middle, its trail on the right, and nothing that
// does not answer a question.
package ui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
	"github.com/deephanson94/compass/internal/tmuxop"
	"github.com/deephanson94/compass/internal/todo"
	"github.com/deephanson94/compass/internal/transcript"
)

// The deck's three cadences. Nothing else moves: the panel draws on data, not
// on frames.
const (
	tickInterval    = time.Second            // transcripts, fleet order, the trail
	paneInterval    = 5 * time.Second        // tmux panes come and go slowly
	captureInterval = 200 * time.Millisecond // the mirror, and only the mirror
	breathInterval  = 500 * time.Millisecond // HEAD's breath — SPEC §4's one animation
)

// trailChrome is what the trail column spends above the graph itself: its
// title, and one line of air (trailColumn).
const trailChrome = 2

// The zoom levels Tab moves between (SPEC §2.3).
const (
	levelTrail     = 1 // the graph of legs
	levelWaypoints = 2 // every leg unfolded
	levelReader    = 3 // the conversation itself
)

type (
	tickMsg        time.Time
	paneTickMsg    time.Time
	captureTickMsg time.Time
	breathTickMsg  time.Time
)

// narratedMsg is the narrator saying new labels have landed: the deck redraws
// and picks them up. It carries nothing — the cache is the payload.
type narratedMsg struct{}

type fleetMsg struct {
	sessions []fleet.Session
	err      error
	at       time.Time
	trail    journey.Trail
	hasTrail bool // the transcript was polled; without it the trail stands
	events   []transcript.Event
	todos    []todo.Item
	trailFor string // the session the payload belongs to; "" if none was polled
}

// Narrator is the deck's view of the narration service (internal/narrator):
// ask for labels, read the ones that have landed. An interface, so the panel
// stays renderable — and testable — without the CLI behind it.
//
// The string it is handed is the session's Key(), not its id: LegKey's
// signature did not change, but what compass passes into it did (M6 contract).
type Narrator interface {
	Labels(key string, tr journey.Trail) map[string]string
	Request(key string, tr journey.Trail, prompt string)
}

// panesMsg carries both shapes of the same truth: the key → pane map the deck
// looks things up in — keyed by SessionInfo.Key(), never by the session id
// (M6 contract) — and tmux's own ordering of the panes, which is the order the
// live fleet groups itself in (M5 contract, package tmuxop).
type panesMsg struct {
	panes map[string]tmuxop.Pane
	list  []tmuxop.Pane
}

type captureMsg struct {
	key   string // the session the frame was captured for
	frame string
}

// attachDoneMsg comes back when compass has its terminal again — or, inside
// tmux, when the client has finished moving. It names the pane, so the deck
// can say where the user just went.
type attachDoneMsg struct {
	target string
	inside bool // the client switched; compass never gave up its terminal
	err    error
}

// Model is the deck. It holds no session state of its own beyond what is on
// screen: the fleet Manager owns the truth, the feeds own the trails, and tmux
// owns the panes.
type Model struct {
	mgr      *fleet.Manager
	feeds    *feedStore
	runner   tmuxop.Runner
	proc     tmuxop.Proc
	narrator Narrator

	sessions []fleet.Session
	panes    map[string]tmuxop.Pane // keyed by SessionInfo.Key(), like everything else
	paneList []tmuxop.Pane          // tmux's own order: the live view's group order
	trail    journey.Trail
	events   []transcript.Event
	todos    []todo.Item
	labels   map[string]string // narrated leg labels for the selected session
	mirror   string
	err      error
	now      time.Time
	loaded   bool
	level    int // zoom: 1 legs, 2 waypoints, 3 the conversation

	// cursor is the Lv2 selection: an index into TrailRows(trail, level), or -1
	// before the level is entered. narrated is the trail the last narration was
	// asked for, so a trail that has not moved is not asked about twice.
	cursor   int
	narrated string

	// The reader's own state, all of it Lv3: where the document is scrolled,
	// which results are unfolded, and the search.
	scroll   int
	unfolded map[int]bool
	query    string
	draft    string // the query being typed; searching is true while it is
	docVer   int    // bumped whenever a fold changes, to retire the cache
	docCache readerCache

	// The trail's own viewport, and only the trail's: how far the panel is
	// scrolled into the trail's document, and whether it is pinned to the
	// bottom. Pinned is the resting state (M7 contract): a growing journey
	// keeps its newest row on screen without anybody pressing a key.
	trailScroll int
	trailPinned bool

	// anchor is the reader line the Lv2 cursor's row lands on — marked, so the
	// two panels say they are showing the same moment. -1 when there is no
	// cursor to follow.
	anchor int

	// selectedKey is the session the deck is pointed at, held by its Key() —
	// its transcript path. The session id is a label two sessions can share
	// (M6 contract); the path is the one thing that never repeats.
	selectedKey string

	width  int
	height int

	// The fleet column's own state: which of the two fleets is on screen, the
	// selection the other one is holding for when you come back, and how far the
	// column is scrolled (in rendered lines).
	archiveView bool
	restSelKey  string // the other view's selection, also a Key()
	fleetScroll int

	showHelp  bool
	searching bool
	pulse     bool // HEAD's breath is on its off-beat
	readonly  bool

	// showMirror opens the live mirror of the selected pane in the middle of
	// the deck at Lv1. Off by default (decision #15): the CLI it mirrors is
	// one Enter away, and the columns are worth more to the trail. `m` flips
	// it; `-mirror` / `mirror = true` starts it on.
	showMirror bool
	inTmux     bool   // $TMUX was set: Enter switches the client instead of suspending
	note       string // one line of consequence, cleared by the next keypress

	// spawn is how a built command reaches the world. The deck leaves it nil
	// and runs the command itself; a harness installs one to read the command
	// Enter built, with no tmux server anywhere near the test.
	spawn func(cmd *exec.Cmd, inside bool, done func(error) tea.Msg) tea.Cmd

	lastNeedsYou int
}

// readerCache holds the flattened document between keypresses: scrolling,
// folding and searching all need it, and re-flattening a long transcript on
// every key would be work nobody asked for.
type readerCache struct {
	lines []readerLine
	valid bool
	n     int // events the document was built from
	w     int // and the width it was wrapped to
	ver   int // and the fold generation
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
		cursor:       -1,
		trailPinned:  true,
		anchor:       -1,
		unfolded:     map[int]bool{},
		lastNeedsYou: -1,
	}
}

// Run starts the full-screen deck. In readonly mode compass keeps its one write
// action — attach — to itself.
//
// $TMUX is read once, here: whether compass is already inside the user's tmux
// decides both what Enter does and what the footer promises. New() leaves it
// false, so a harness renders one deterministic deck.
//
// build, when it is not nil, is asked for the narrator once the program exists:
// the narrator needs a way to say "labels landed", and that way is a message
// into this program. A nil return simply leaves the trail on its heuristics.
func Run(mgr *fleet.Manager, readonly, mirror bool, build func(notify func()) Narrator) error {
	m := New(mgr)
	m.readonly = readonly
	m.showMirror = mirror
	m.inTmux = os.Getenv("TMUX") != ""
	p := tea.NewProgram(m, tea.WithAltScreen())
	if build != nil {
		m.narrator = build(func() { p.Send(narratedMsg{}) })
	}
	_, err := p.Run()
	return err
}

// planItems turns the transcript's tasks into the shape the trail draws:
// deleted ones drop out, the rest keep their status and both their tenses.
func planItems(tasks []journey.Task) []todo.Item {
	items := make([]todo.Item, 0, len(tasks))
	for _, t := range tasks {
		if t.Status == "deleted" {
			continue
		}
		items = append(items, todo.Item{Text: t.Subject, Status: todo.Status(t.Status), Active: t.Active})
	}
	return items
}

// SetNarrator hands the deck its narrator (a harness passes a fake one).
func (m *Model) SetNarrator(n Narrator) {
	m.narrator = n
}

// SetEvents installs the selected session's transcript — what the Lv3 reader
// renders (exported so a harness can render a fixed document).
func (m *Model) SetEvents(events []transcript.Event) {
	m.events = events
	m.docCache.valid = false
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

// SetPanes gives the model the key → pane mapping: the location line in the
// fleet, the source of the mirror, and the pane Enter attaches to.
func (m *Model) SetPanes(panes map[string]tmuxop.Pane) {
	m.panes = panes
}

// SetPaneOrder hands the model tmux's own pane ordering — the list ListPanes
// returns, session by session, in index order. The live view groups itself in
// the order this list first mentions each tmux session.
func (m *Model) SetPaneOrder(list []tmuxop.Pane) {
	m.paneList = list
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
	return tea.Batch(m.refresh(), tick(), paneTick(), captureTick(), breathTick(),
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

func breathTick() tea.Cmd {
	return tea.Tick(breathInterval, func(t time.Time) tea.Msg { return breathTickMsg(t) })
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
	selected, root := m.selectedKey, mgr.Root()
	// The todo file on disk is named after the session id, not the key: the id
	// is what claude itself writes under. Two sessions sharing an id share that
	// plan, which is the truth on disk — not something compass may invent.
	path, sessionID := "", ""
	if s, ok := m.selected(); ok {
		path, sessionID = s.Info.TranscriptPath, s.Info.ID
	}
	return func() tea.Msg {
		now := time.Now()
		sessions, err := mgr.Refresh(now)
		msg := fleetMsg{sessions: sessions, err: err, at: now}
		if selected != "" {
			// The plan the session keeps for itself. A missing or unreadable
			// todo file is not news: the trail simply has no future to draw.
			msg.todos, _ = todo.Read(root, sessionID)
			msg.trailFor = selected
		}
		if feeds != nil {
			feeds.retain(sessions)
			if selected != "" && path != "" {
				msg.trail, msg.events = feeds.poll(selected, path)
				msg.hasTrail = true
			}
		}
		return msg
	}
}

// relistPanes re-reads the tmux server and re-pairs it with the fleet. The
// ordered pane list travels with the map — the ui is the only thing that knows
// tmux's order, and the fleet's liveness is the only thing that knows what the
// pairing means, so each MapSessions is reported straight back to the Manager.
func (m *Model) relistPanes() tea.Cmd {
	runner, proc, mgr := m.runner, m.proc, m.mgr
	infos := make([]fleet.SessionInfo, 0, len(m.sessions))
	for _, s := range m.sessions {
		infos = append(infos, s.Info)
	}
	return func() tea.Msg {
		panes, err := tmuxop.ListPanes(runner)
		if err != nil || len(panes) == 0 {
			markMapped(mgr, nil)
			return panesMsg{panes: map[string]tmuxop.Pane{}}
		}
		mapped := tmuxop.MapSessions(infos, panes, proc)
		markMapped(mgr, mapped)
		return panesMsg{panes: mapped, list: panes}
	}
}

// markMapped tells the fleet which sessions currently sit in a pane — the other
// half of liveness (M5 contract, fleet rule 1). The set it passes is a set of
// keys, so a twin sharing an id does not inherit its sibling's pane (M6
// contract). A harness drives the deck with no Manager at all, so a nil one is
// simply nothing to tell.
func markMapped(mgr *fleet.Manager, mapped map[string]tmuxop.Pane) {
	if mgr == nil {
		return
	}
	keys := make(map[string]bool, len(mapped))
	for key := range mapped {
		keys[key] = true
	}
	mgr.MarkPaneMapped(keys)
}

// capture mirrors the selected pane, and only it: 200ms of one capture-pane is
// cheap, one per session would not be.
func (m *Model) capture() tea.Cmd {
	// No mirror on screen, no capture-pane: five calls a second into tmux
	// for a frame nobody is looking at is the one cost the mirror had.
	if !m.mirrorShown() {
		return nil
	}
	pane, ok := m.selectedPane()
	if !ok {
		return nil
	}
	runner, key := m.runner, m.selectedKey
	return func() tea.Msg {
		frame, err := tmuxop.Capture(runner, pane.ID)
		if err != nil {
			return captureMsg{key: key}
		}
		return captureMsg{key: key, frame: frame}
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

	case breathTickMsg:
		// Only a working HEAD breathes; everything else redraws on data.
		if s, ok := m.selected(); ok && s.Snap.State == state.Working {
			m.pulse = !m.pulse
		} else {
			m.pulse = false
		}
		return m, breathTick()

	case fleetMsg:
		// Init lists panes against a fleet that has not arrived yet, so the
		// first pairing has nothing to pair. Re-list the moment there is
		// something to pair with: otherwise every session reads "no pane" —
		// and the mirror falls back to the transcript — until the 5s pane tick.
		first := !m.loaded
		m.sessions, m.err, m.now, m.loaded = msg.sessions, msg.err, msg.at, true
		m.clampSelection()
		if first && len(m.sessions) > 0 {
			return m, tea.Batch(m.titleCmd(), m.relistPanes())
		}
		if msg.trailFor != "" && msg.trailFor == m.selectedKey {
			items := msg.todos
			if msg.hasTrail {
				m.trail = msg.trail
				m.SetEvents(msg.events)
				m.requestNarration()
				// The plan comes from the transcript when the session kept one
				// there; the todo file is the fallback for a Claude Code that
				// still writes one.
				if len(msg.trail.Tasks) > 0 {
					items = planItems(msg.trail.Tasks)
				}
			}
			m.SetTodos(items)
		}
		return m, m.titleCmd()

	case narratedMsg:
		m.refreshLabels()
		return m, nil

	case askDoneMsg:
		if msg.err != nil {
			m.note = "ask ended: " + msg.err.Error()
		}
		return m, nil

	case panesMsg:
		m.panes, m.paneList = msg.panes, msg.list
		m.clampSelection()
		return m, nil

	case captureMsg:
		if msg.key == m.selectedKey {
			m.mirror = msg.frame
		}
		return m, nil

	case attachDoneMsg:
		switch {
		case msg.err != nil && msg.inside:
			// Inside tmux the handover is one sequence — select-window,
			// select-pane, switch-client — and tmux runs the rest even when a
			// step fails, so the client may well have moved anyway (a detached
			// server, for instance, has no client to switch and says so while
			// the selects still land). Report what tmux said rather than
			// claiming an outcome the deck cannot see.
			m.note = "tmux: " + firstLine(msg.err.Error())
		case msg.err != nil:
			m.note = "attach failed: " + firstLine(msg.err.Error())
		case msg.inside:
			// Nothing was suspended, so nothing announced its own return: the
			// deck says where the client went instead.
			m.note = "switched to " + msg.target
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

	// While a search query is being typed, every key belongs to it.
	if m.searching {
		m.note = ""
		m.searchKey(msg)
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
	case "shift+tab":
		m.zoomOut()
		return m, nil
	case "esc":
		// At Lv3 a standing search clears first; the second Esc zooms out.
		if m.level >= levelReader && m.query != "" {
			m.query = ""
			return m, nil
		}
		m.zoomOut()
		return m, nil
	case "a":
		return m, m.ask()
	case "A":
		// The archive is a view of the same fleet, at any depth: what is selected
		// stays selected, per view, so coming back lands where you left.
		m.toggleArchive()
		return m, m.refresh()
	case "m":
		m.showMirror = !m.showMirror
		switch {
		case !m.showMirror:
		case m.width < deckWideCols:
			m.note = fmt.Sprintf("the mirror needs %d columns; this terminal has %d", deckWideCols, m.width)
		case m.level != levelTrail:
			m.note = "the mirror shows at Lv1 (esc to zoom out)"
		}
		return m, m.capture()
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		m.selectIndex(int(key[0] - '1'))
		return m, m.refresh()
	}

	// The rest of the keys mean different things at different depths.
	switch m.level {
	case levelReader:
		return m.readerKey(key)
	case levelWaypoints:
		switch key {
		case "j", "down":
			m.cursorMove(1)
			return m, nil
		case "k", "up":
			m.cursorMove(-1)
			return m, nil
		case "G":
			// G means the same thing at every depth: back to the present. At
			// Lv2 the cursor is what the viewport follows, so it is the cursor
			// that travels — and landing on the newest row re-pins the panel.
			m.cursorToPresent()
			return m, nil
		case "enter":
			// The reader is already open on this row (the middle panel follows
			// the cursor), so Enter has only ever meant one thing: go there.
			return m, m.attach()
		case "g":
			if !m.selectOldestNeedsYou() {
				m.note = "nothing is waiting on you"
				return m, nil
			}
			return m, tea.Batch(m.refresh(), m.attach())
		}
	default: // levelTrail
		switch key {
		case "j", "down":
			m.move(1)
			return m, m.refresh()
		case "k", "up":
			m.move(-1)
			return m, m.refresh()
		case "ctrl+d":
			m.trailScrollBy(m.trailHalfPage())
			return m, nil
		case "ctrl+u":
			m.trailScrollBy(-m.trailHalfPage())
			return m, nil
		case "G":
			// Back to the present, whatever the offset was.
			m.trailPinned = true
			return m, nil
		case "enter":
			return m, m.attach()
		case "g":
			if !m.selectOldestNeedsYou() {
				m.note = "nothing is waiting on you"
				return m, nil
			}
			return m, tea.Batch(m.refresh(), m.attach())
		}
	}
	return m, nil
}

// readerKey is the Lv3 keymap: the document is the object, so the keys move
// through it rather than through the fleet.
func (m *Model) readerKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "enter":
		// Enter means one thing at every depth (M7 contract).
		return m, m.attach()
	case "j", "down":
		m.scrollBy(1)
	case "k", "up":
		m.scrollBy(-1)
	case "ctrl+d":
		m.scrollBy(m.readerHeight() / 2)
	case "ctrl+u":
		m.scrollBy(-m.readerHeight() / 2)
	case "g":
		m.scroll = 0
	case "G":
		m.scrollBy(1 << 30) // clamped to the last screenful
	case " ", "space":
		m.toggleFold()
	case "/":
		m.searching = true
		m.draft = ""
	case "n":
		m.jumpMatch(1)
	case "N":
		m.jumpMatch(-1)
	}
	return m, nil
}

// cursorMove walks the Lv2 selection over the trail's selectable rows.
func (m *Model) cursorMove(delta int) {
	rows := TrailRows(m.trail, m.level)
	if len(rows) == 0 {
		m.cursor = -1
		return
	}
	c := m.cursor + delta
	if m.cursor < 0 {
		// A cursor that has not been placed yet starts at the present: Tab into
		// Lv2 — and switching sessions inside it — opens on the newest row, the
		// same place the pinned trail is already showing.
		c = len(rows) - 1
	}
	if c < 0 {
		c = 0
	}
	if c >= len(rows) {
		c = len(rows) - 1
	}
	m.cursor = c
	if c == len(rows)-1 {
		// The newest row is the present: standing on it puts the panel back to
		// following the journey.
		m.trailPinned = true
	}
	m.keepCursorVisible()
	m.anchorReader()
}

// cursorToPresent puts the Lv2 cursor on the newest row, wherever it stood.
// cursorMove clamps, so the whole journey in one delta is simply the end of it.
func (m *Model) cursorToPresent() {
	m.cursorMove(len(TrailRows(m.trail, m.level)))
}

// trailBox is the block the trail column is currently drawn into: the same
// arithmetic deckLines and trailColumn do, so a scroll key moves the viewport
// that is actually on screen. Width first, then the rows the graph itself gets
// — the column spends two on its title and its line of air.
func (m *Model) trailBox() (int, int) {
	w := m.width
	if w <= 0 {
		w = 80
	}
	inner := w - 2*edgePad
	if inner < 10 {
		inner = w
	}
	_, _, width := m.layout(inner)

	h := m.height
	if h <= 0 {
		h = 24
	}
	height := h - 5 - trailChrome
	if height < 1 {
		height = 1
	}
	return width, height
}

// trailHalfPage is what ctrl+d and ctrl+u move: half the trail's screenful.
func (m *Model) trailHalfPage() int {
	_, h := m.trailBox()
	if h < 2 {
		return 1
	}
	return h / 2
}

// trailView measures the trail against its viewport: the whole document, one
// screenful, and the offset the panel is showing right now — a pinned panel is
// showing the last screenful, whatever Scroll says.
func (m *Model) trailView() (total, height, top int) {
	w, h := m.trailBox()
	total = len(TrailLines(m.trail, m.trailOpts(w, h)))
	top = m.trailScroll
	if m.trailPinned {
		top = lastScreenful(total, h)
	}
	return total, h, clampScroll(top, total, h)
}

// lastScreenful is the offset a pinned panel is showing: the bottom of the
// document, which is where the journey's newest row lives.
func lastScreenful(total, height int) int {
	return clampScroll(total, total, height)
}

// trailScrollBy moves the trail's viewport, clamped to the document. Scrolling
// up unpins; landing back on the last screenful re-pins, so the common case
// needs no key at all (M7 contract).
func (m *Model) trailScrollBy(delta int) {
	total, h, top := m.trailView()
	m.trailScroll = clampScroll(top+delta, total, h)
	m.trailPinned = m.trailScroll >= lastScreenful(total, h)
}

// keepCursorVisible scrolls the trail only as far as it must to keep the Lv2
// cursor's row on screen — a cursor already inside the viewport moves nothing,
// not even the pin.
func (m *Model) keepCursorVisible() {
	if m.level < levelWaypoints || m.cursor < 0 {
		return
	}
	w, h := m.trailBox()
	row := TrailCursorRow(m.trail, m.trailOpts(w, h))
	if row < 0 {
		return
	}
	total, height, top := m.trailView()
	switch {
	case row < top:
		top = row
	case row >= top+height:
		top = row - height + 1
	default:
		return // already on screen: the offset and the pin both stand
	}
	m.trailScroll = clampScroll(top, total, height)
	m.trailPinned = m.trailScroll >= lastScreenful(total, height)
}

// zoomIn is Tab: Lv1's legs unfold their waypoints, Lv2 opens the conversation
// itself. At the bottom the key says so rather than doing nothing.
func (m *Model) zoomIn() {
	switch {
	case m.level < levelWaypoints:
		m.level = levelWaypoints
		// Lv2 is the trail with a cursor on it, and the middle panel reading
		// whatever that cursor stands on: cursorMove(0) settles both.
		m.cursorMove(0)
	case m.level < levelReader:
		m.enterReader()
	default:
		m.note = "this is the deepest level"
	}
}

// zoomOut is Shift+Tab: one level back up. At Lv1 it is a no-op — zooming out
// of the trail would be zooming out of compass.
func (m *Model) zoomOut() {
	switch {
	case m.level > levelWaypoints:
		m.level = levelWaypoints
		// The reader goes back to following the cursor it left behind.
		m.anchorReader()
	case m.level > levelTrail:
		m.level = levelTrail
		m.cursor, m.anchor = -1, -1
	}
}

// attach hands the terminal to the selected session — Enter's whole job (M6
// contract). Outside tmux compass suspends itself the way `ask` does: the pane
// owns the terminal until the user detaches with their own prefix `d`, and the
// deck comes back exactly as it was. Inside tmux there is nothing to suspend —
// the command moves the client and returns at once.
//
// It is compass's only write, and only ever from a keypress.
func (m *Model) attach() tea.Cmd {
	if m.readonly {
		m.note = "read-only · attach is off"
		return nil
	}
	pane, ok := m.selectedPane()
	if !ok {
		m.note = "no tmux pane for this session"
		return nil
	}
	cmd := tmuxop.Attach(pane.Target, pane.ID, m.inTmux)
	done := func(err error) tea.Msg {
		return attachDoneMsg{target: pane.Target, inside: m.inTmux, err: err}
	}
	if m.spawn != nil {
		return m.spawn(cmd, m.inTmux, done)
	}
	if m.inTmux {
		// Nothing is suspended, so the command runs off the render loop like any
		// other — and tmux's own words are worth more than "exit status 1".
		return func() tea.Msg {
			out, err := cmd.CombinedOutput()
			if err != nil {
				if said := strings.TrimSpace(string(out)); said != "" {
					err = errors.New(said)
				}
			}
			return done(err)
		}
	}
	return tea.ExecProcess(cmd, done)
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
	pane, ok := m.panes[s.Info.Key()]
	return pane, ok
}

// selectedIndex resolves the sticky selection (by key) to a row. The fleet
// re-sorts every second; the cursor must stay on the session, not on the line
// number — and never on its twin.
func (m *Model) selectedIndex() int {
	for i, s := range m.sessions {
		if s.Info.Key() == m.selectedKey {
			return i
		}
	}
	return 0
}

func (m *Model) clampSelection() {
	if len(m.sessions) == 0 {
		m.selectedKey, m.restSelKey = "", ""
		return
	}
	order := m.fleetOrder()
	if len(order) == 0 {
		return // an empty view keeps whatever it had; the column says it is empty
	}
	for _, i := range order {
		if m.sessions[i].Info.Key() == m.selectedKey {
			return
		}
	}
	m.point(m.sessions[order[0]].Info.Key())
}

// toggleArchive swaps the two fleets, and their selections with them: the live
// view remembers where you were standing while you read an old journey.
func (m *Model) toggleArchive() {
	if !m.archiveView && m.archivedCount() == 0 {
		m.note = "nothing archived yet"
		return
	}
	m.archiveView = !m.archiveView
	m.selectedKey, m.restSelKey = m.restSelKey, m.selectedKey
	m.fleetScroll = 0
	if m.selectedKey != "" {
		// A remembered key is a fresh selection for everything downstream.
		key := m.selectedKey
		m.selectedKey = ""
		m.point(key)
	}
	m.clampSelection()
}

// point moves the selection. Trail and mirror belong to the session that was
// selected, so they leave with it rather than lingering as somebody else's.
func (m *Model) point(key string) {
	if key == m.selectedKey {
		return
	}
	m.selectedKey = key
	m.trail = journey.Trail{}
	m.todos = nil
	m.mirror = ""
	m.labels = nil
	m.events = nil
	m.docCache.valid = false
	m.unfolded = map[int]bool{}
	m.scroll = 0
	m.cursor, m.anchor = -1, -1
	m.trailScroll, m.trailPinned = 0, true
	m.query, m.draft, m.searching = "", "", false
}

// selectIndex is the `1`–`9` keys: an index into the rendered order, groups
// and their headers ignored.
func (m *Model) selectIndex(i int) {
	order := m.fleetOrder()
	if i < 0 || i >= len(order) {
		return
	}
	m.point(m.sessions[order[i]].Info.Key())
}

// move is j/k: one session down or up the rendered order, skipping headers —
// they name a group, they are not a place to stand.
func (m *Model) move(delta int) {
	order := m.fleetOrder()
	if len(order) == 0 {
		return
	}
	pos := 0
	for p, i := range order {
		if m.sessions[i].Info.Key() == m.selectedKey {
			pos = p
			break
		}
	}
	pos += delta
	if pos < 0 {
		pos = 0
	}
	if pos >= len(order) {
		pos = len(order) - 1
	}
	m.point(m.sessions[order[pos]].Info.Key())
}

// selectOldestNeedsYou grabs the session that has been waiting longest — the
// fleet is already sorted that way. Only a live session can be waiting on you
// (an archived one is idle by construction), so the archive is never searched;
// pressing `g` while browsing it comes back to the live fleet first.
func (m *Model) selectOldestNeedsYou() bool {
	for _, s := range m.sessions {
		if s.Live && s.Snap.State == state.NeedsYou {
			if m.archiveView {
				m.toggleArchive()
			}
			m.point(s.Info.Key())
			return true
		}
	}
	return false
}

func (m *Model) needsYouCount() int {
	n := 0
	for _, s := range m.sessions {
		if s.Live && s.Snap.State == state.NeedsYou {
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

// statusChips renders the same counts `compass status` prints — the live ones
// only (M5 contract, fleet rule 5). The archive is history: it cannot be
// working, and counting its idle hundreds would drown the pulse.
func (m *Model) statusChips() string {
	if !m.loaded {
		return dimStyle.Render("scanning…")
	}
	counts := map[state.State]int{}
	for _, s := range m.sessions {
		if !s.Live {
			continue
		}
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
	keys := "j/k move · " + m.enterKeymap() + " · g grab · ? help · q quit"
	if m.archiveView {
		// In the archive `g` has nothing to grab and `A` is the way home, so the
		// keymap says that instead. In the live view the archive announces itself
		// on the fleet's own last row: "N archived · A browses".
		keys = "j/k move · " + m.enterKeymap() + " · A live fleet · ? help · q quit"
	}
	switch {
	case m.showHelp:
		keys = "? or esc closes help"
	case m.searching:
		keys = "type to search · enter finds · esc cancels"
	case m.level >= levelReader:
		keys = "j/k scroll · space fold · / search · n/N · a ask · enter attach · esc back"
	case m.level >= levelWaypoints:
		keys = "j/k rows · enter attach · tab deeper · a ask · esc back"
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

// enterKeymap is what Enter promises, and — outside tmux, where compass hands
// its whole terminal over — how to come back (M6 contract). `prefix d` is the
// user's own detach key, because the terminal is genuinely theirs by then.
//
// The number keys are not in this line: the fleet column prints its own 1–9
// beside each session, and the parenthetical is what the footer owes an
// 80-column deck instead.
func (m *Model) enterKeymap() string {
	if m.inTmux {
		return "enter attach"
	}
	return "enter attach (prefix d returns)"
}

// column is one vertical panel of the deck.
type column struct {
	width int
	rows  []string
}

// mirrorShown says whether the live mirror is on screen: switched on, at
// Lv1, on a terminal wide enough for three columns. It is what capture()
// checks before spending a tmux call.
func (m *Model) mirrorShown() bool {
	return m.showMirror && m.level == levelTrail && m.width >= deckWideCols
}

// middleShown says whether the deck draws a middle panel at all: the mirror
// when it is on at Lv1, the reader at Lv3. Lv2 is the trail unfolded, with the
// fleet beside it and nothing else — the reader is a level, not a panel
// (decision #15).
func (m *Model) middleShown() bool {
	if m.width < deckWideCols {
		return false
	}
	return m.level >= levelReader || m.mirrorShown()
}

// layout is the deck's column widths for an inner width: fleet, middle, trail.
// middle is 0 when nothing is drawn there; fleet is 0 when the terminal holds
// one column, which is the trail's (the reader's at Lv3). Every panel that
// needs to know how wide it is — the trail's viewport, the reader's folds,
// the deck itself — asks here, so none of them can disagree.
func (m *Model) layout(inner int) (fleet, middle, trail int) {
	if inner < minDeckCols {
		return 0, 0, inner
	}
	if m.middleShown() {
		fleet, trail = sidePanelWidths(inner)
		return fleet, inner - fleet - trail - 2*gutterWidth, trail
	}
	fleet = twoColumnFleet(inner)
	return fleet, 0, inner - fleet - gutterWidth
}

// twoColumnFleet is the fleet's width when the trail has the rest of the deck:
// it grows from its floor toward its cap on a third of whatever is spare past
// both floors, and the trail takes the other two thirds and everything after.
// Session names are the thing the fleet truncates; the trail's labels and
// reports are the thing the deck exists to show.
func twoColumnFleet(inner int) int {
	spare := inner - fleetWidth - trailWidth - gutterWidth
	if spare < 0 {
		spare = 0
	}
	fleet := fleetWidth + spare/3
	if fleet > fleetWidthMax {
		fleet = fleetWidthMax
	}
	return fleet
}

// deckLines lays the deck out: the fleet, then either the trail alone beside
// it or a middle panel between them — the live mirror (Lv1, when it is on) or
// the reader (Lv3). Below minDeckCols there is one column: the trail, because
// the reason to run compass that narrow is to sit it beside a CLI in your own
// tmux, and beside a CLI the trail is the half that is not already on screen;
// the header carries the fleet's alarm and the trail's title names the
// selected session.
func (m *Model) deckLines(w, h int) []string {
	fw, mw, tw := m.layout(w)
	if fw == 0 {
		one := m.trailColumn
		if m.level >= levelReader {
			one = m.readerColumn
		}
		return fit(one(w, h), h)
	}
	if mw > 0 {
		middle := m.mirrorColumn
		if m.level >= levelReader {
			middle = m.readerColumn
		}
		return joinColumns(h, []column{
			{fw, m.fleetColumn(fw, h)},
			{mw, middle(mw, h)},
			{tw, m.trailColumn(tw, h)},
		})
	}
	// Two columns. At Lv3 on a terminal too narrow for three, the conversation
	// takes the trail's place rather than going unread.
	second := m.trailColumn
	if m.level >= levelReader {
		second = m.readerColumn
	}
	return joinColumns(h, []column{
		{fw, m.fleetColumn(fw, h)},
		{tw, second(tw, h)},
	})
}

// sidePanelWidths shares out a three-column deck. The fleet and the trail are
// the two panels only compass draws; the middle is a rendering of something
// else — a pane, a conversation. So once the middle has enough width to be
// readable, every further column goes to the sides — evenly, and no further
// than their caps, past which a row is padding rather than information.
func sidePanelWidths(w int) (fleet, trail int) {
	fleet, trail = fleetWidth, trailWidth
	spare := w - fleet - trail - 2*gutterWidth - mirrorEnough
	for spare > 0 && (fleet < fleetWidthMax || trail < trailWidthMax) {
		grew := false
		if trail < trailWidthMax && spare > 0 {
			trail, spare, grew = trail+1, spare-1, true
		}
		if fleet < fleetWidthMax && spare > 0 {
			fleet, spare, grew = fleet+1, spare-1, true
		}
		if !grew {
			break
		}
	}
	return fleet, trail
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
