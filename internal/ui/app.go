// Package ui renders the compass deck: the fleet on the left, the selected
// session's live pane in the middle, its trail on the right, and nothing that
// does not answer a question.
package ui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
	levelBoard     = 0 // every trail that fits, side by side (decision #16)
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

	// trails is every board column's journey, keyed by session; nil when the
	// board was not polled (a narrow terminal). The selected session's is in
	// here and in trail both.
	trails map[string]journey.Trail
}

// Narrator is the deck's view of the narration service (internal/narrator):
// ask for labels, read the ones that have landed. An interface, so the panel
// stays renderable — and testable — without the CLI behind it.
//
// The string it is handed is the session's Key(), not its id: LegKey's
// signature did not change, but what compass passes into it did (M6 contract).
type Narrator interface {
	Labels(key string, tr journey.Trail) map[string]string
	// Request reports false when the trail must be asked for again next
	// tick (see narrator.Request).
	Request(key string, tr journey.Trail, prompt string) bool
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

// replyDoneMsg says a quick reply was typed into a pane, or was not.
type replyDoneMsg struct {
	key    string // the session it went to
	target string
	text   string
	err    error
}

// sentReply is the last line compass typed into a session, so the board
// can say so until the transcript shows the session took it.
type sentReply struct {
	text string
	at   time.Time
}

// replyChoice is one line the panel offers: an answer to the question the
// session is sitting on, a stock line, or stop.
type replyChoice struct {
	label string // what the panel shows
	text  string // what is typed, for a line
	kind  replyKind
	n     int // an answer's number in the CLI's own menu
}

type replyKind int

const (
	replyLine   replyKind = iota // typed and entered
	replyAnswer                  // the menu's own digit, then enter
	replyStop                    // escape: the CLI interrupts its turn
)

// DefaultReplies are the quick replies `r` offers when the config names
// none: the three lines a person keeps typing into a fleet of sessions.
var DefaultReplies = []string{
	"please continue",
	"report status",
	"you were stuck on the quota limit; it's back now — please resume where you left off",
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
	anchor     int
	anchorAt   time.Time // the moment the anchor stands for; zero when none
	anchorText string    // what that row said

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

	showHelp    bool
	searching   bool
	replying    bool                 // the reply panel is up; a digit picks a line
	replyTyping bool                 // …and a line is being typed into it
	replyDraft  string               // the line so far
	replies     []string             // the stock lines `r` offers, in order
	sent        map[string]sentReply // the last line sent to each session, by Key()
	pulse       bool                 // HEAD's breath is on its off-beat
	readonly    bool

	// The board's data: one trail per column, and each column's narrated
	// labels. The selected session's trail is here as well as in trail.
	trails      map[string]journey.Trail
	boardLabels map[string]map[string]string
	seen        map[string]time.Time // when each session's trail or pane was last opened
	seenFile    string               // where the seen-times persist; "" = memory only (harness)
	boardForced bool                 // the deck left the board only because the terminal narrowed
	boardShapes map[string]string    // each column's trail shape its labels were read for
	refreshing  bool                 // a refresh is in flight; the tick does not launch another

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
	n     int    // events the document was built from
	w     int    // and the width it was wrapped to
	ver   int    // and the fold generation
	cwd   string // and the directory its paths were shortened against
}

// New returns a deck bound to a fleet Manager.
func New(mgr *fleet.Manager) *Model {
	return &Model{
		mgr:          mgr,
		feeds:        newFeedStore(),
		runner:       tmuxop.RealRunner{},
		replies:      DefaultReplies,
		sent:         map[string]sentReply{},
		proc:         tmuxop.RealProc{},
		now:          time.Now(),
		level:        levelBoard,
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
func Run(mgr *fleet.Manager, readonly, mirror bool, replies []string, build func(notify func()) Narrator) error {
	m := New(mgr)
	m.readonly = readonly
	m.showMirror = mirror
	if len(replies) > 0 {
		m.replies = replies
	}
	if base, err := os.UserCacheDir(); err == nil {
		// Beside the resume cache. What was read is a fact about the person,
		// not the session, and it has to outlive the process for "bright
		// means unread" to mean anything across restarts.
		m.LoadSeen(filepath.Join(base, "compass", "seen.json"))
	}
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
	// The board needs width. A deck that opened on it and then found itself
	// in a narrow terminal is a single trail from here on, so the first Tab
	// does something visible.
	switch {
	case m.level == levelBoard && !m.boardFits():
		m.level, m.boardForced = levelTrail, true
	case m.boardForced && m.level == levelTrail && m.boardFits():
		// The width came back — a tmux zoom, a window snap — and so does
		// the view it took away.
		m.level, m.boardForced = levelBoard, false
	}
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
	if m.refreshing {
		// A board of large transcripts can take longer to replay than the
		// tick between refreshes. One at a time: the next tick tries again.
		return nil
	}
	m.refreshing = true
	selected, root := m.selectedKey, mgr.Root()
	// The todo file on disk is named after the session id, not the key: the id
	// is what claude itself writes under. Two sessions sharing an id share that
	// plan, which is the truth on disk — not something compass may invent.
	path, sessionID := "", ""
	if s, ok := m.selected(); ok {
		path, sessionID = s.Info.TranscriptPath, s.Info.ID
	}
	targets := m.boardTargets()
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
				msg.trail, msg.events = feeds.poll(selected, path, true)
				msg.hasTrail = true
			}
			// The board's columns. Each feed reads only what its transcript
			// has grown since the last poll; the first poll of a new column
			// replays its whole journey once, the same as selecting it does.
			if len(targets) > 0 {
				msg.trails = make(map[string]journey.Trail, len(targets))
				for _, t := range targets {
					if t.key == selected && msg.hasTrail {
						msg.trails[t.key] = msg.trail
						continue
					}
					// A column draws only the trail; the reader's events are
					// kept for the selected session alone.
					tr, _ := feeds.poll(t.key, t.path, false)
					msg.trails[t.key] = tr
				}
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
		m.SetSize(msg.Width, msg.Height)
		return m, nil

	case tickMsg:
		m.now = time.Time(msg)
		return m, tea.Batch(tick(), m.refresh())

	case paneTickMsg:
		return m, tea.Batch(paneTick(), m.relistPanes())

	case captureTickMsg:
		return m, tea.Batch(captureTick(), m.capture())

	case breathTickMsg:
		// Only a working HEAD breathes; everything else redraws on data. On
		// the board every working column has one.
		if m.anyWorking() {
			m.pulse = !m.pulse
		} else {
			m.pulse = false
		}
		return m, breathTick()

	case fleetMsg:
		m.refreshing = false
		if m.loaded && msg.at.Before(m.now) {
			// A slower refresh landing after a faster one: its fleet, its
			// trails and its clock are all older than what is on screen.
			return m, nil
		}
		// Init lists panes against a fleet that has not arrived yet, so the
		// first pairing has nothing to pair. Re-list the moment there is
		// something to pair with: otherwise every session reads "no pane" —
		// and the mirror falls back to the transcript — until the 5s pane tick.
		first := !m.loaded
		m.sessions, m.err, m.now, m.loaded = msg.sessions, msg.err, msg.at, true
		m.clampSelection()
		if first && len(m.sessions) > 0 {
			m.refreshBoard(msg.trails)
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
		// After the selected session's own narration request, so the column
		// being read is never starved of the one batch in flight.
		m.refreshBoard(msg.trails)
		return m, m.titleCmd()

	case narratedMsg:
		m.refreshLabels()
		m.boardShapes = nil // labels landed: every column reads them again
		m.refreshBoard(m.trails)
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

	case replyDoneMsg:
		if msg.err != nil {
			m.note = "could not send: " + firstLine(msg.err.Error())
		} else {
			m.note = fmt.Sprintf("sent to %s %s · %s", mirrorMark, msg.target, clip(`"`+msg.text+`"`, 40))
			m.sent[msg.key] = sentReply{text: msg.text, at: m.now}
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

	// While the quick replies are up, a digit picks one and anything else
	// puts them away: a reply is typed into someone's session, and it is
	// never sent by a key that meant something else.
	if m.replying {
		m.note = ""
		return m, m.replyKey(msg)
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
	case "r":
		m.offerReplies()
		return m, nil
	case "A":
		// The archive is a view of the same fleet, at any depth: what is selected
		// stays selected, per view, so coming back lands where you left. It is
		// a list, not a board: three hundred columns of "reading its
		// transcript…" answered nothing, and the list has the prompts.
		was := m.archiveView
		m.toggleArchive()
		switch {
		case m.archiveView == was:
			// Nothing archived: the note says so, and the deck stays.
		case m.archiveView && m.level != levelTrail:
			// The archive is a list; it opens as one, whatever the depth.
			m.level = levelTrail
			m.cursor, m.anchor = -1, -1
		case !m.archiveView && m.boardFits():
			// And leaving it goes back to the board, which is where `A`
			// was pressed: the fleet list beside one trail is not a
			// level a terminal with a board has.
			m.level = levelBoard
			m.cursor, m.anchor = -1, -1
		}
		return m, m.refresh()
	case "m":
		m.showMirror = !m.showMirror
		switch {
		case !m.showMirror:
		case m.width < deckWideCols:
			m.note = fmt.Sprintf("the mirror needs %d columns", deckWideCols)
		case m.level == levelBoard:
			m.note = "the mirror shows beside a session (tab)"
		case m.sessionView() && m.level >= levelReader:
			// The live pane has no keys: they go back to the trail.
			m.level = levelWaypoints
			m.anchorReader()
			m.note = "the live pane · m again for the conversation"
		case !m.sessionView() && m.level != levelTrail:
			m.note = "the mirror shows beside the trail (esc to zoom out)"
		}
		return m, m.capture()
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		i := int(key[0] - '1')
		if !m.selectIndex(i) {
			m.note = fmt.Sprintf("no session %d", i+1)
		}
		return m, m.refresh()
	}

	// The rest of the keys mean different things at different depths.
	switch m.level {
	case levelReader:
		return m.readerKey(key)
	case levelWaypoints:
		switch key {
		case "h", "left", "l", "right":
			if m.sessionView() {
				if key == "h" || key == "left" {
					m.sessionMove(-1)
				} else {
					m.sessionMove(1)
				}
				return m, m.refresh()
			}
			return m, nil
		case "j", "down":
			// A key that moves nothing says why: two identical frames
			// after `j` read as a dead key, and the cursor opens on the
			// newest row, so the first `j` of every visit was that.
			if n := len(TrailRows(m.trail, m.level)); m.cursor >= n-1 {
				m.note = "at the present · k goes back"
				return m, nil
			}
			m.cursorMove(1)
			return m, nil
		case "k", "up":
			if m.cursor == 0 {
				m.note = "at the start of the trail"
				return m, nil
			}
			m.cursorMove(-1)
			return m, nil
		case "ctrl+d":
			// Half a page of rows: the cursor is what the viewport follows
			// here, so the cursor is what moves (SPEC §3).
			was := m.cursor
			m.cursorMove(m.trailHalfPage())
			if m.cursor == was {
				m.note = "at the present · k goes back"
			}
			return m, nil
		case "ctrl+u":
			was := m.cursor
			m.cursorMove(-m.trailHalfPage())
			if m.cursor == was {
				m.note = "at the start of the trail"
			}
			return m, nil
		case "G":
			// G means the same thing at every depth: back to the present. At
			// Lv2 the cursor is what the viewport follows, so it is the cursor
			// that travels — and landing on the newest row re-pins the panel.
			m.cursorToPresent()
			return m, nil
		case "[", "]":
			m.chapter(key)
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
			m.note = "→ " + m.selectedLocation()
			return m, tea.Batch(m.refresh(), m.attach())
		}
	default: // levelTrail, and the board
		switch key {
		case "j", "down", "l", "right":
			// The board runs sideways: h/l and the arrows say so, and j/k
			// keep working for hands that reach for them.
			if m.level == levelBoard && m.boardShown() {
				m.boardMove(1)
			} else if key == "j" || key == "down" {
				m.move(1)
			} else {
				return m, nil
			}
			return m, m.refresh()
		case "k", "up", "h", "left":
			if m.level == levelBoard && m.boardShown() {
				m.boardMove(-1)
			} else if key == "k" || key == "up" {
				m.move(-1)
			} else {
				return m, nil
			}
			return m, m.refresh()
		case "ctrl+d", "ctrl+u":
			if m.level == levelBoard && m.boardShown() {
				m.note = "the board shows the present · tab into a trail to scroll back"
				return m, nil
			}
			if total, h, _ := m.trailView(); total <= h {
				m.note = "the whole trail is on screen"
				return m, nil
			}
			if key == "ctrl+u" {
				m.trailScrollBy(-m.trailHalfPage())
			} else {
				m.trailScrollBy(m.trailHalfPage())
			}
			return m, nil
		case "G":
			if m.level == levelBoard && m.boardShown() {
				m.note = "the board is already at the present"
				return m, nil
			}
			if m.trailPinned {
				m.note = "already at the present"
				return m, nil
			}
			// Back to the present, whatever the offset was.
			m.trailPinned = true
			return m, nil
		case "[", "]":
			if m.level == levelBoard && m.boardShown() {
				m.note = "prompts are chapters of one trail · tab into it"
				return m, nil
			}
			m.chapter(key)
			return m, nil
		case "enter":
			return m, m.attach()
		case "g":
			if !m.selectOldestNeedsYou() {
				m.note = "nothing is waiting on you"
				return m, nil
			}
			m.note = "→ " + m.selectedLocation()
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
	case "h", "left", "l", "right":
		if m.sessionView() {
			if key == "h" || key == "left" {
				m.sessionMove(-1)
			} else {
				m.sessionMove(1)
			}
			return m, m.refresh()
		}
	case "j", "down":
		if !m.scrollBy(1) {
			m.note = "end of the conversation"
		}
	case "k", "up":
		if !m.scrollBy(-1) {
			m.note = "start of the conversation"
		}
	case "ctrl+d":
		if !m.scrollBy(m.readerHeight() / 2) {
			m.note = "end of the conversation"
		}
	case "ctrl+u":
		if !m.scrollBy(-m.readerHeight() / 2) {
			m.note = "start of the conversation"
		}
	case "[", "]":
		m.readerChapter(key)
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

// chapter is `[` / `]`: the previous or next prompt, treated as a chapter
// of the trail. A day-long journey is a dozen of your own prompts with the
// work between them, and "take me to where I said 'now the audit log'" is
// what getting back to an hour actually means. At Lv1 the viewport opens on
// the prompt; at Lv2 the cursor lands on it. The note says which chapter
// this is and when it began.
func (m *Model) chapter(key string) {
	rows := TrailRows(m.trail, m.level)
	var prompts []int // indices into rows
	for i, r := range rows {
		if r.Kind == "prompt" {
			prompts = append(prompts, i)
		}
	}
	if len(prompts) == 0 {
		m.note = "no prompts in this trail"
		return
	}
	w, h := m.trailBox()
	o := m.trailOpts(w, h)
	doc, sel := trailDoc(m.trail, o)
	docRow := map[int]int{} // row index → document line
	for line, r := range sel {
		if r >= 0 {
			docRow[r] = line
		}
	}
	// Where we are: the cursor at Lv2, the top of the viewport at Lv1.
	at := -1
	if m.level >= levelWaypoints && m.cursor >= 0 {
		at = m.cursor
	} else {
		top := trailTop(len(doc), o)
		for i := range rows {
			if docRow[i] >= top {
				at = i
				break
			}
		}
		if at < 0 {
			at = len(rows)
		}
	}
	target := -1
	if key == "]" {
		for _, p := range prompts {
			if p > at {
				target = p
				break
			}
		}
		if target < 0 {
			m.note = "no later prompt · G is the present"
			return
		}
	} else {
		for i := len(prompts) - 1; i >= 0; i-- {
			if prompts[i] < at {
				target = prompts[i]
				break
			}
		}
		if target < 0 {
			m.note = "no earlier prompt"
			return
		}
	}
	nth := 0
	for i, p := range prompts {
		if p == target {
			nth = i + 1
		}
	}
	if m.level >= levelWaypoints {
		m.cursor = target
		m.cursorMove(0)
	} else {
		line := docRow[target]
		m.trailScroll = clampScroll(line, len(doc), h)
		m.trailPinned = m.trailScroll >= lastScreenful(len(doc), h)
	}
	m.note = fmt.Sprintf("◉ %d/%d · %s · %s", nth, len(prompts), clip(`"`+rows[target].Text+`"`, 40), rows[target].Time.Local().Format("15:04"))
}

// firstRowInView is the first selectable row at or below the trail
// viewport's top, or -1 when none is.
func (m *Model) firstRowInView() int {
	w, h := m.trailBox()
	o := m.trailOpts(w, h)
	doc, sel := trailDoc(m.trail, o)
	top := trailTop(len(doc), o)
	for i := top; i < len(sel); i++ {
		if sel[i] >= 0 {
			return sel[i]
		}
	}
	return -1
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
	// A row the panel does not draw — a waypoint the leg's own row already
	// carries — is not a place to stand: step over it, the way the key was
	// going, and back the other way at the ends.
	if !m.cursorDrawn() {
		step := 1
		if delta < 0 {
			step = -1
		}
		for i := c + step; i >= 0 && i < len(rows); i += step {
			m.cursor = i
			if m.cursorDrawn() {
				break
			}
		}
		if !m.cursorDrawn() {
			for i := c - step; i >= 0 && i < len(rows); i -= step {
				m.cursor = i
				if m.cursorDrawn() {
					break
				}
			}
		}
		c = m.cursor
	}
	if c == len(rows)-1 {
		// The newest row is the present: standing on it puts the panel back to
		// following the journey.
		m.trailPinned = true
	}
	m.keepCursorVisible()
	m.anchorReader()
}

// cursorDrawn reports whether the trail draws a row for the cursor.
func (m *Model) cursorDrawn() bool {
	w, h := m.trailBox()
	return TrailCursorRow(m.trail, m.trailOpts(w, h)) >= 0
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
	case m.level < levelTrail:
		m.level, m.boardForced = levelTrail, false
		if m.boardFits() && !m.archiveView {
			// Three levels on a terminal with a board: the board chooses,
			// the session reads, the reader digs. The single trail with a
			// fleet list beside it was the board's column drawn wider
			// next to a list the board already is (decision #18).
			m.level = levelWaypoints
		}
		m.markSeen(m.selectedKey)
		// The column's trail, plan and labels are already in hand: the single
		// trail opens on them rather than bare until the next poll. The
		// reader's events are not — those are kept for the selected session
		// only, and arrive with its next poll.
		if tr, ok := m.trails[m.selectedKey]; ok {
			m.trail = tr
			m.todos = planItems(tr.Tasks)
			if l := m.boardLabels[m.selectedKey]; l != nil {
				m.labels = l
			}
		}
		if m.level == levelWaypoints {
			m.cursorMove(0) // the cursor opens on the present; the reader follows
		}
	case m.level < levelWaypoints:
		m.level = levelWaypoints
		// Lv2 is the trail with a cursor on it. A trail scrolled back to
		// some earlier hour puts the cursor there — on the first row in
		// view — rather than at the present: ten presses of ctrl+u are a
		// place, and Tab used to throw it away.
		if !m.trailPinned {
			m.cursor = m.firstRowInView()
		}
		m.cursorMove(0)
	case m.level < levelReader:
		m.enterReader()
	default:
		m.note = "this is the deepest level"
	}
}

// zoomOut is Shift+Tab: one level back up. From the single trail it is the
// board, on a terminal wide enough for one; on a narrow terminal Lv1 is the
// top, and zooming out of the trail would be zooming out of compass.
func (m *Model) zoomOut() {
	switch {
	case m.level > levelWaypoints:
		m.level = levelWaypoints
		// The reader goes back to following the cursor it left behind.
		m.anchorReader()
	case m.level > levelTrail:
		m.level = levelTrail
		m.cursor, m.anchor = -1, -1
		if m.boardFits() && !m.archiveView {
			m.level = levelBoard // the session view came from the board; back to it
		}
	case m.level > levelBoard && m.boardFits():
		m.level = levelBoard
	case m.level == levelTrail:
		m.note = fmt.Sprintf("no board under %d columns", deckWideCols)
	}
}

// offerReplies is `r`: the quick replies go on the footer, numbered, for
// the selected session's pane. Nothing is sent until a digit is pressed.
// It is the second of compass's two writes, and like attach it is gated on
// a keypress and switched off by read-only.
func (m *Model) offerReplies() {
	if m.readonly {
		m.note = "read-only · replies are off"
		return
	}
	if _, ok := m.selectedPane(); !ok {
		m.note = "no tmux pane for this session · nothing to reply to"
		return
	}
	m.replying, m.replyTyping, m.replyDraft = true, false, ""
}

// replyChoices is what the panel offers for the selected session: the
// options of the question it is sitting on, as the CLI's own digits; the
// stock lines; and stop. Nine at most — the digits are the keys.
func (m *Model) replyChoices() []replyChoice {
	var out []replyChoice
	if s, ok := m.selected(); ok && s.Snap.State == state.NeedsYou {
		if use, ok := pendingQuestion(m.events); ok {
			for i, label := range askedOptions(use.Input) {
				out = append(out, replyChoice{label: label, kind: replyAnswer, n: i + 1})
			}
		}
	}
	for _, r := range m.replies {
		out = append(out, replyChoice{label: r, text: r, kind: replyLine})
	}
	out = append(out, replyChoice{label: "stop — interrupt the turn (escape)", kind: replyStop})
	if len(out) > 9 {
		out = out[:9]
	}
	return out
}

// replyKey handles a key while the replies are up: a digit sends its line,
// anything else closes the menu and sends nothing.
func (m *Model) replyKey(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()
	if m.replyTyping {
		// A line being typed: enter sends it, esc goes back to the menu,
		// everything else is the line.
		switch key {
		case "enter":
			text := strings.TrimSpace(m.replyDraft)
			m.replying, m.replyTyping, m.replyDraft = false, false, ""
			if text == "" {
				return nil
			}
			return m.send(replyChoice{label: text, text: text, kind: replyLine})
		case "esc":
			m.replyTyping, m.replyDraft = false, ""
		case "backspace":
			if r := []rune(m.replyDraft); len(r) > 0 {
				m.replyDraft = string(r[:len(r)-1])
			}
		case "ctrl+c":
			return tea.Quit
		default:
			if msg.Type == tea.KeyRunes || key == " " {
				m.replyDraft += string(msg.Runes)
			}
		}
		return nil
	}
	if key == "t" {
		m.replyTyping = true
		return nil
	}
	m.replying = false
	if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
		i := int(key[0] - '1')
		choices := m.replyChoices()
		if i < len(choices) {
			return m.send(choices[i])
		}
		m.note = fmt.Sprintf("no reply %d", i+1)
		return nil
	}
	if key == "ctrl+c" {
		return tea.Quit
	}
	return nil // q included: a menu is closed, not quit from
}

// send carries one choice to the selected session's pane, off the render
// loop: a line is typed and entered; an answer is the menu's own digit,
// then enter; stop is escape.
func (m *Model) send(c replyChoice) tea.Cmd {
	pane, ok := m.selectedPane()
	if !ok {
		m.note = "no tmux pane for this session · nothing to reply to"
		return nil
	}
	runner, target, key := m.runner, pane.Target, m.selectedKey
	return func() tea.Msg {
		var err error
		switch c.kind {
		case replyAnswer:
			err = tmuxop.SendKeys(runner, pane.ID, strconv.Itoa(c.n))
		case replyStop:
			err = tmuxop.SendKey(runner, pane.ID, "Escape")
		default:
			err = tmuxop.SendKeys(runner, pane.ID, c.text)
		}
		return replyDoneMsg{key: key, target: target, text: c.label, err: err}
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
	m.markSeen(m.selectedKey)
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
// anyWorking says whether a HEAD on screen is moving: the selected session's
// on a single trail, any column's on the board.
func (m *Model) anyWorking() bool {
	if m.level == levelBoard && m.boardShown() {
		n, _ := boardColumns(m.width-2*edgePad, len(m.viewOrder()))
		for _, key := range m.boardKeys(n) {
			if s, ok := m.session(key); ok && s.Snap.State == state.Working {
				return true
			}
		}
		return false
	}
	s, ok := m.selected()
	return ok && s.Snap.State == state.Working
}

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
	if m.level >= levelTrail && !m.boardShown() {
		// No board: selecting a session puts its trail on screen, which
		// is opening it. "unread" never cleared at 100 columns.
		m.markSeen(key)
	}
	// The board already holds this session's trail, plan and labels: use
	// them, rather than blanking the panel until the next poll — which, on a
	// terminal too narrow for the board, made every j/k show "nothing yet"
	// over a session with a hundred legs, for a second or for good.
	m.trail, m.todos, m.labels = journey.Trail{}, nil, nil
	if tr, ok := m.trails[key]; ok {
		m.trail = tr
		m.todos = planItems(tr.Tasks)
		if l := m.boardLabels[key]; l != nil {
			m.labels = l
		}
	}
	m.mirror = ""
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
//
// The numbers are the view's order — urgent first — at every level: the
// board prints them on its columns and the fleet list prints the same ones
// beside its rows, however it groups them. A number that meant one session
// on the board and another one Tab later was how you attach to the wrong
// pane.
// selectedLocation names the selected session and, when it has one, its pane:
// "infra · ops:0.0". It is what `g` says as it goes.
func (m *Model) selectedLocation() string {
	s, ok := m.selected()
	if !ok {
		return "—"
	}
	if pane, ok := m.panes[s.Info.Key()]; ok && pane.Target != "" {
		return sessionName(s.Info) + " · " + pane.Target
	}
	return sessionName(s.Info)
}

func (m *Model) selectIndex(i int) bool {
	return m.boardSelect(i)
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
		body = helpLinesFor(inner, bodyHeight, m.boardFits())
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

	if m.replying {
		// The quick replies float over the deck as a small panel: a line
		// of footer was too easy to miss, and the person pressing `r`
		// expected something to pop up.
		panel := m.replyPanel(inner)
		left, top := m.panelPlace(inner, panelWidth(panel))
		overlay(out[3:3+bodyHeight], panel, left, top)
	}

	for i, line := range out {
		if line == "" {
			continue
		}
		out[i] = strings.Repeat(" ", edgePad) + line
	}
	return strings.Join(out, "\n")
}

// panelWidth is the widest row of a panel.
func panelWidth(panel []string) int {
	pw := 0
	for _, p := range panel {
		if w := lipgloss.Width(p); w > pw {
			pw = w
		}
	}
	return pw
}

// overlay draws panel over rows at left, top, leaving what is around it:
// the deck stays where it was, with the panel on top. A panel that would
// run off the bottom is lifted until it fits.
func overlay(rows, panel []string, left, top int) {
	if len(panel) == 0 || len(rows) == 0 {
		return
	}
	pw := panelWidth(panel)
	if top+len(panel) > len(rows) {
		top = len(rows) - len(panel)
	}
	if top < 0 {
		top = 0
	}
	if left < 0 {
		left = 0
	}
	for i, p := range panel {
		if top+i >= len(rows) {
			break
		}
		// What is left of the panel stays; what is right of it goes: a
		// column's tail sliced at the border read as a leg with no label.
		line := rows[top+i]
		before := ansi.Truncate(line, left, "")
		if lipgloss.Width(line) > left && left > 1 {
			// A row cut by the panel's edge says it was cut: "✗ red
			// 310✓ 2✗ · shipped" alone inverted "shipped on red".
			before = ansi.Truncate(line, left-1, "") + "…"
		}
		if w := lipgloss.Width(before); w < left {
			before += strings.Repeat(" ", left-w)
		}
		rows[top+i] = before + pad(p, pw)
	}
}

// replyPanelMax is the widest the reply panel gets: a long stock line
// wraps rather than stretching the box across the deck.
const replyPanelMax = 64

// replyPanel is the quick replies as a boxed list: who it goes to — the
// board's number, the name, the pane, since two sessions can share a name
// and a tmux session — what that session is doing right now, the numbered
// lines, and the two keys that matter. A column of air on each side keeps
// the deck's text from running into the border.
func (m *Model) replyPanel(inner int) []string {
	name, target, who := "—", "", ""
	s, ok := m.selected()
	if ok {
		name = sessionName(s.Info)
		for i, key := range m.viewOrder() {
			if m.sessions[key].Info.Key() == m.selectedKey {
				who = strconv.Itoa(i+1) + " · "
			}
		}
	}
	if pane, ok := m.selectedPane(); ok {
		target = pane.Target
	}
	title := " reply to " + who + name
	if target != "" {
		title += " · " + mirrorMark + " " + target
	}
	title += " "

	body := replyPanelMax
	if max := inner - 8; body > max {
		body = max
	}
	if body < 20 {
		body = 20
	}
	var rows []readerLine
	if ok {
		for _, line := range wrapPrefix(m.replyState(s), "", "", body) {
			rows = append(rows, readerLine{text: line, kind: readerBody})
		}
		rows = append(rows, readerLine{kind: readerBlank})
	}
	choices := m.replyChoices()
	lastKind := replyKind(-1)
	for i, c := range choices {
		if i > 0 && c.kind != lastKind {
			rows = append(rows, readerLine{kind: readerBlank}) // answers, lines, stop: three groups
		}
		lastKind = c.kind
		label := c.label
		if c.kind == replyAnswer {
			label += "   ← answers the question"
			if i > 0 && choices[i-1].kind == replyAnswer {
				label = c.label
			}
		}
		kind := readerText
		if c.kind == replyStop {
			kind = readerFoldErr
		}
		for _, line := range wrapPrefix(label, fmt.Sprintf("%d  ", i+1), "   ", body) {
			rows = append(rows, readerLine{text: line, kind: kind})
		}
	}
	rows = append(rows, readerLine{kind: readerBlank})
	if m.replyTyping {
		rows = append(rows, readerLine{text: "› " + m.replyDraft + "▏", kind: readerSaid},
			readerLine{text: "enter sends the line · esc back to the menu", kind: readerBody})
	} else {
		rows = append(rows, readerLine{text: "t  type a line", kind: readerText},
			readerLine{text: "a digit sends · esc closes", kind: readerBody})
	}

	box := func(s string, style lipgloss.Style) string {
		return " " + ruleStyle.Render("│") + " " + style.Render(pad(clip(s, body), body)) + " " + ruleStyle.Render("│") + " "
	}
	fill := body + 2 - lipgloss.Width(title)
	if fill < 0 {
		title, fill = clip(title, body+2), 0
	}
	out := []string{" " + ruleStyle.Render("┌"+title+strings.Repeat("─", fill)+"┐") + " "}
	for _, r := range rows {
		out = append(out, box(r.text, readerStyle(r.kind)))
	}
	out = append(out, " "+ruleStyle.Render("└"+strings.Repeat("─", body+2)+"┘")+" ")
	return out
}

// replyState is the one line the panel owes before a digit is pressed:
// what the session is doing, because the line lands in its input and
// what that means depends on it. A session on a question gets the
// question, since the digits of that menu are on the same keys.
func (m *Model) replyState(s fleet.Session) string {
	since := relAge(m.now, headSince(s))
	switch s.Snap.State {
	case state.NeedsYou:
		q := strings.TrimSpace(m.headFor(s))
		if q == "" {
			q = "a question"
		}
		return "▲ on a question · " + q + " — the line is typed into that prompt"
	case state.Stuck:
		return "◍ stuck · silent " + since + " — the line is typed under the hung call"
	case state.Working:
		// The turn's own clock — "for 1h", or "◈3 out 20m · quiet 15m"
		// — not the last write's: how long the line will queue is how
		// long the turn has been going.
		tail := ""
		if s.Info.Key() == m.selectedKey {
			tail = headTail(m.trail, m.now, true)
		} else if tr, ok := m.trails[s.Info.Key()]; ok {
			tail = headTail(tr, m.now, true)
		}
		if tail == "" {
			tail = "for " + since
		}
		return "● working " + tail + " — the line queues behind its turn"
	default:
		return "○ idle " + since + " — waiting for a prompt"
	}
}

// panelPlace is where a panel goes so the selection stays in view: on the
// board, under the selected column's header; in a session, over the
// companion; on a narrow deck, over the trail — never over the row or the
// card that names the session it is about.
func (m *Model) panelPlace(inner, pw int) (left, top int) {
	if m.level == levelBoard && m.boardShown() {
		if x, y, ok := m.boardPlace(inner); ok {
			return min(x, max(inner-pw, 0)), y + 3
		}
		return 0, 3
	}
	fw, mw, _ := m.layout(inner)
	switch {
	case fw == 0 && mw > 0:
		_, tw := sessionSplit(inner)
		return min(tw+gutterWidth, max(inner-pw, 0)), 2
	case fw > 0:
		return min(fw+gutterWidth, max(inner-pw, 0)), 2
	}
	return 0, 2
}

// headerLine: the product mark on the left, the fleet's pulse on the right.
func (m *Model) headerLine(w int) string {
	left := titleStyle.Render("⌂ compass")
	if m.level == levelBoard && m.boardShown() {
		left += dimStyle.Render(" · board")
	}
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
	oldest := map[state.State]time.Time{}
	for _, s := range m.sessions {
		if !s.Live {
			continue
		}
		counts[s.Snap.State]++
		if st := s.Snap.State; st == state.NeedsYou || st == state.Stuck {
			if at, ok := oldest[st]; !ok || headSince(s).Before(at) {
				oldest[st] = headSince(s) // a hung row's silence, as the row counts it
			}
		}
	}
	var parts []string
	for _, st := range []state.State{state.NeedsYou, state.Stuck, state.Working, state.Idle} {
		n := counts[st]
		if n == 0 {
			continue
		}
		chip := fmt.Sprintf("%s%d", fleet.Glyph(st), n)
		// The two states you must not miss carry their wait: "▲1 4m" is a
		// change you can read from across the room, where a census is not.
		if at, ok := oldest[st]; ok {
			chip += " " + m.age(at)
		}
		parts = append(parts, stateStyle(st).Render(chip))
	}
	// Agents out are work in flight nobody's glyph shows: "●2 ○2 all calm"
	// over four lanes still out twenty minutes in was a claim, and wrong.
	out, oldestOut := 0, time.Time{}
	for _, s := range m.sessions {
		if !s.Live || s.Snap.State == state.Idle {
			continue // an idle session's open lanes are lost, not out
		}
		for _, b := range m.trails[s.Info.Key()].Branches {
			if !b.Done {
				out++
				if oldestOut.IsZero() || b.Start.Before(oldestOut) {
					oldestOut = b.Start
				}
			}
		}
	}
	if out > 0 {
		parts = append(parts, dimStyle.Render(fmt.Sprintf("◈%d out · oldest %s", out, m.age(oldestOut))))
	}
	if m.archiveView {
		parts = append(parts, dimStyle.Render(fmt.Sprintf("archive %d", m.archivedCount())))
	}
	if len(parts) == 0 {
		return dimStyle.Render("○ all quiet")
	}
	if counts[state.NeedsYou] == 0 && counts[state.Stuck] == 0 && out == 0 && !m.archiveView {
		// Calm, said aloud: the absence of a warm glyph is the design, and
		// in monochrome an absence is also what a clipped header looks like.
		parts = append(parts, dimStyle.Render("all calm"))
	}
	return strings.Join(parts, "  ")
}

// footerLine carries the keymap, and — briefly, on the right — whatever the
// last keypress did.
func (m *Model) footerLine(w int) string {
	keys := "j/k move · " + m.enterKeymap() + " · [ ] chapters · r reply · g grab · ? help · q quit"
	if m.archiveView {
		// In the archive `g` has nothing to grab and `A` is the way home, so the
		// keymap says that instead. In the live view the archive announces itself
		// on the fleet's own last row: "N archived · A browses".
		keys = "j/k move · " + m.enterKeymap() + " · tab deeper · a ask · A live fleet · ? help · q quit"
	}
	switch {
	case m.showHelp:
		keys = "? or esc closes help"
	case m.searching:
		keys = "type to search · enter finds · esc cancels"
	case m.replying && m.replyTyping:
		keys = "type the line · enter sends · esc back"
	case m.replying:
		keys = fmt.Sprintf("reply: 1–%d sends · t types a line · esc closes", len(m.replyChoices()))
	case m.level == levelBoard && m.boardShown():
		keys = "h/l columns · " + m.enterKeymap() + " · tab one trail · r reply · g grab · ? help · q quit"
		if m.archiveView {
			keys = "h/l columns · " + m.enterKeymap() + " · tab one trail · A live fleet · ? help · q quit"
		}
	case m.level == levelTrail && m.boardShown():
		keys = "j/k move · " + m.enterKeymap() + " · [ ] chapters · r reply · ⇧tab board · g grab · ? help · q quit"
		if m.archiveView {
			keys = "j/k move · " + m.enterKeymap() + " · tab deeper · a ask · ⇧tab board · A live fleet · ? help · q quit"
		}
	case m.level >= levelReader && m.sessionView():
		keys = "j/k scroll · space unfold · / search · n/N · [ ] turns · h/l session · r reply · a ask · enter attach · esc back"
	case m.level >= levelReader:
		keys = "j/k scroll · space unfold · / search · n/N · [ ] turns · r reply · a ask · enter attach · esc back"
	case m.level >= levelWaypoints && m.sessionView():
		keys = "j/k legs · h/l session · [ ] chapters · m live pane · r reply · tab reader · enter attach · esc board"
	case m.level >= levelWaypoints:
		keys = "j/k rows · [ ] chapters · r reply · enter attach · tab deeper · a ask · esc back"
	}
	// The keymap sheds its optional fragments before it clips: a footer
	// that ends in "· ? he" says less than one without the chapters.
	for _, drop := range []string{" · r reply", " · [ ] chapters", " · [ ] turns", " · m live pane", " · h/l session", " · tab deeper", " · a ask", " · g grab"} {
		if lipgloss.Width(keys) <= w {
			break
		}
		keys = strings.Replace(keys, drop, "", 1)
	}
	left := dimStyle.Render(clip(keys, w))
	if m.note == "" {
		return left
	}
	// The note is the news, but the keymap is the only place the reader's
	// keys are named: shed the keymap's fragments for the note first, and
	// clip the note before the keymap goes.
	// The keys a note is about — the chapters it counts, the reply it
	// reports — go last: a footer that dropped `[ ] turns` on the frame
	// that said "❯ 3/12" read as the key having gone.
	drops := []string{" · a ask", " · tab deeper", " · h/l session", " · m live pane", " · g grab", " · / search", " · n/N", " · r reply", " · ? help", " · [ ] chapters", " · [ ] turns", " · space unfold"}
	if strings.HasPrefix(m.note, glyphSaid) || strings.HasPrefix(m.note, glyphPrompt) {
		// A chapter note keeps the chapter keys over everything.
		drops = []string{" · a ask", " · tab deeper", " · h/l session", " · m live pane", " · g grab", " · / search", " · n/N", " · r reply", " · ? help", " · space unfold", " · [ ] chapters", " · [ ] turns"}
	}
	for _, drop := range drops {
		if lipgloss.Width(keys)+2+lipgloss.Width(m.note) <= w {
			break
		}
		keys = strings.Replace(keys, drop, "", 1)
	}
	left = dimStyle.Render(clip(keys, w))
	room := w - lipgloss.Width(left) - 2
	if room < 12 {
		return dimStyle.Render(clip(m.note, w)) // no keymap fits beside it
	}
	note := dimStyle.Render(clip(m.note, room))
	gap := w - lipgloss.Width(left) - lipgloss.Width(note)
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
	// In the session view the live pane stands in for the conversation
	// while the keys are on the trail; at Lv3 the keys are the reader's,
	// so the reader is what is drawn.
	return m.showMirror && m.width >= deckWideCols && (m.level == levelTrail || (m.sessionView() && m.level == levelWaypoints))
}

// middleShown says whether the deck draws a middle panel at all: the mirror
// when it is on at Lv1, the reader at Lv3. Lv2 is the trail unfolded, with the
// fleet beside it and nothing else — the reader is a level, not a panel
// (decision #15).
func (m *Model) middleShown() bool {
	if m.width < deckWideCols {
		return false
	}
	// The reader is drawn from Lv2, following the cursor; at Lv3 the keys
	// move into it. Lv2 used to be the trail with a cursor and nothing
	// beside it, and on a wide terminal that was Lv1 with one row
	// inverted — a keypress that bought nothing to look at.
	return m.level >= levelWaypoints || m.mirrorShown()
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
	if m.level >= levelReader && m.width < deckWideCols {
		// A deck too narrow for a middle panel gives the reader the whole
		// width at Lv3: beside a 41-column fleet it wrapped every line,
		// and the fleet is two Shift+Tabs away.
		return 0, 0, inner
	}
	if m.sessionView() {
		// The session view: the companion — conversation, or the live
		// pane — and the trail, no fleet list. The board is the fleet.
		companion, trail := sessionSplit(inner)
		return 0, companion, trail
	}
	if m.middleShown() {
		if m.level >= levelReader && m.width < readerRoomCols {
			// The keys are the reader's and the fleet is two Shift+Tabs
			// away: on a deck too narrow for three panels the reader
			// takes the fleet's width rather than wrapping at 46 columns
			// beside eleven idle rows.
			return 0, inner - trailWidth - gutterWidth, trailWidth
		}
		fleet, trail = sidePanelWidths(inner)
		return fleet, inner - fleet - trail - 2*gutterWidth, trail
	}
	fleet = twoColumnFleet(inner)
	return fleet, 0, inner - fleet - gutterWidth
}

// sessionView says whether the deck is showing one session on a terminal
// that has a board: the trail with its cursor, a companion panel following
// it, and no fleet list, because the board is the fleet.
func (m *Model) sessionView() bool {
	return m.boardFits() && !m.archiveView && m.level >= levelWaypoints
}

// sessionSplit divides the session view: the trail takes a little under
// half, enough for its detail to ride on the row, and the companion the
// rest.
func sessionSplit(inner int) (companion, trail int) {
	trail = inner * 45 / 100
	if trail < trailWidth {
		trail = trailWidth
	}
	if trail > sessionTrailMax {
		trail = sessionTrailMax
	}
	return inner - trail - gutterWidth, trail
}

// sessionMove is h/l inside a session: the neighbouring session in the
// board's order, at the same depth, its cursor on the present.
func (m *Model) sessionMove(delta int) {
	was := m.selectedKey
	m.boardMove(delta)
	if m.selectedKey == was {
		if delta < 0 {
			m.note = "the first session"
		} else {
			m.note = "the last session"
		}
		return
	}
	m.cursorMove(0)
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
	if m.level == levelBoard && m.boardShown() {
		return m.boardLines(w, h)
	}
	fw, mw, tw := m.layout(w)
	if fw == 0 && mw > 0 {
		// Two columns, the trail and its companion: the conversation, or
		// the live pane while `m` has it and the keys are on the trail. The
		// trail leads — it is the column the board handed over, the keys
		// are on it, and its card names the session — and the companion
		// reads to its right, the way a file tree stands beside the file.
		companion := m.readerColumn
		if m.mirrorShown() {
			companion = m.mirrorColumn
		}
		return joinColumns(h, []column{
			{tw, m.trailColumn(tw, h)},
			{mw, companion(mw, h)},
		})
	}
	if fw == 0 {
		one := m.trailColumn
		if m.level >= levelReader {
			one = m.readerColumn
		}
		return fit(one(w, h), h)
	}
	if mw > 0 {
		middle := m.mirrorColumn
		if m.level >= levelWaypoints && !(m.sessionView() && m.showMirror) {
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
