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
	answer int // the menu digit pressed, when the reply was an answer
	err    error
}

// hookState is what the hook compares a session against between refreshes.
type hookState struct {
	state        state.State
	apiError     bool
	circling     bool
	shippedOnRed bool
	back         int // lanes returned
}

// sentReply is the last line compass typed into a session, so the board
// can say so until the transcript shows the session took it.
type sentReply struct {
	text   string
	at     time.Time
	answer int // the menu digit pressed, 0 for a typed line
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
	replyRemedy                  // the remedy a refusal names, typed and entered
	replyAnswer                  // the menu's own digit, then enter
	replyStop                    // escape: the CLI interrupts its turn
)

// DefaultReplies are the quick replies `r` offers when the config names
// none: the three lines a person keeps typing into a fleet of sessions.
var DefaultReplies = []string{
	"please continue",
	"report status",
	quotaReply,
}

// quotaReply is the stock line for a session that died on its quota; it is
// offered only to one that did.
const quotaReply = "you were stuck on the quota limit; it's back now — please resume where you left off"

// hadAPIError says whether the selected session's conversation ends on a
// refusal, for a session whose state has since moved on.
func (m *Model) hadAPIError(s fleet.Session) bool {
	if s.Info.Key() != m.selectedKey {
		return false
	}
	for i := len(m.events) - 1; i >= 0 && i >= len(m.events)-3; i-- {
		if m.events[i].APIError {
			return true
		}
	}
	return false
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

	// The event hook: a command the config names, run on the moments that
	// matter while nobody is looking at the deck — a question, a hang, a
	// refusal, a loop, agents returning. hookRun is the seam a harness
	// replaces; before is what each session was at the last refresh.
	hook     string
	hookRun  func(event, session, tmux, detail string)
	before   map[string]hookState
	pulse    bool // HEAD's breath is on its off-beat
	readonly bool

	// The board's data: one trail per column, and each column's narrated
	// labels. The selected session's trail is here as well as in trail.
	trails      map[string]journey.Trail
	boardLabels map[string]map[string]string
	fleetQuery  string               // the fleet search in force; "" = none
	searchFleet bool                 // the search being typed is the fleet's, not the reader's
	querySel    string               // the selection before the search, restored when it is cancelled
	lastLook    map[string]time.Time // the look before the current one, per session: the read-line while it is open
	hookFired   map[string]time.Time // when each session+event last ran the hook, for the cool-off
	hidden      map[string]bool      // sessions taken off the board with `x`, by Key()
	digits      map[string]int       // each live session's number, kept from first sight
	opened      map[string]bool      // sessions the person opened this run, by Key(): a look to commit on leaving
	hiddenFile  string               // where they persist; "" = memory only
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

	lastTitle string // the terminal title last set
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
		mgr:         mgr,
		feeds:       newFeedStore(),
		runner:      tmuxop.RealRunner{},
		replies:     DefaultReplies,
		sent:        map[string]sentReply{},
		proc:        tmuxop.RealProc{},
		now:         time.Now(),
		level:       levelBoard,
		cursor:      -1,
		trailPinned: true,
		anchor:      -1,
		unfolded:    map[int]bool{},
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
func Run(mgr *fleet.Manager, readonly, mirror bool, replies []string, hook string, build func(notify func()) Narrator) error {
	m := New(mgr)
	m.readonly = readonly
	m.showMirror = mirror
	m.hook = hook
	if len(replies) > 0 {
		m.replies = replies
	}
	if base, err := os.UserCacheDir(); err == nil {
		// Beside the resume cache. What was read is a fact about the person,
		// not the session, and it has to outlive the process for "bright
		// means unread" to mean anything across restarts.
		m.LoadSeen(filepath.Join(base, "compass", "seen.json"))
		m.LoadHidden(filepath.Join(base, "compass", "hidden.json"))
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
	m.assignDigits()
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
		m.relistPanes(), tea.SetWindowTitle(tabTitle(0, 0, 0, 0)))
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
			if m.level == levelBoard && m.boardFits() && len(m.viewOrder()) == 1 {
				// One session: a board of one column filled a corner of a
				// wide screen and read as half-drawn. Open the session.
				m.zoomIn()
			}
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
		m.fireHooks()
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
			verb := "sent"
			if msg.answer > 0 {
				verb = fmt.Sprintf("answered %d ·", msg.answer)
			}
			m.note = fmt.Sprintf("↪ %s %s · to %s %s", verb, `"`+msg.text+`"`, mirrorMark, msg.target) // the footer clips the quote to its room
			m.sent[msg.key] = sentReply{text: msg.text, at: m.now, answer: msg.answer}
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
		if m.level <= levelTrail && m.fleetQuery != "" {
			// On the board or a list a standing search clears first; in a
			// session esc is the way back to the board, search or not.
			m.clearQuery()
			return m, nil
		}
		m.zoomOut()
		return m, nil
	case "/":
		if m.level < levelReader {
			// The fleet's search: the board, the list and the archive
			// narrow to what matches; the reader keeps its own `/`.
			m.searching, m.searchFleet, m.draft = true, true, ""
			m.querySel = m.selectedKey
			return m, nil
		}
	case "a":
		return m, m.ask()
	case "r":
		m.offerReplies()
		return m, nil
	case "x":
		m.toggleHidden()
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
		if m.width < deckWideCols {
			// Not flipped: a mirror switched on out of sight appeared
			// unbidden when the terminal widened, and the second press
			// was a silent key.
			m.note = fmt.Sprintf("mirror needs %d columns", deckWideCols)
			return m, nil
		}
		if m.archiveView {
			m.note = "no mirror in the archive · A returns to the fleet"
			return m, nil
		}
		m.showMirror = !m.showMirror
		switch {
		case !m.showMirror && m.sessionView() && m.level == levelWaypoints:
			m.note = "the conversation" // short: the panel's own title says the rest, and the keys stay
		case !m.showMirror:
		case m.sessionView() && m.level == levelWaypoints:
			m.note = "the live pane"
		case m.level == levelBoard:
			m.note = "mirror on · beside a session (tab)" // the flag flipped: said as a state, not a refusal
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
				if n <= 1 {
					m.note = "the trail is one row" // no key goes anywhere
				}
				return m, nil
			}
			m.cursorMove(1)
			return m, nil
		case "k", "up":
			if m.cursor == 0 {
				m.note = "at the start of the trail"
				if len(TrailRows(m.trail, m.level)) <= 1 {
					m.note = "the trail is one row"
				}
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
				if len(TrailRows(m.trail, m.level)) <= 1 {
					m.note = "the trail is one row"
				}
			}
			return m, nil
		case "G":
			// G means the same thing at every depth: back to the present. At
			// Lv2 the cursor is what the viewport follows, so it is the cursor
			// that travels — and landing on the newest row re-pins the panel.
			if n := len(TrailRows(m.trail, m.level)); m.cursor >= n-1 && m.trailPinned {
				m.note = "at the present"
				if n <= 1 {
					m.note = "the trail is one row"
				}
				return m, nil
			}
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
			was := m.selectedKey
			if m.level == levelBoard && m.boardShown() {
				m.boardMove(1)
			} else if key == "j" || key == "down" {
				m.move(1)
			} else {
				return m, nil
			}
			if m.selectedKey == was && m.note == "" {
				m.note = m.onlyOrLast(1)
			}
			return m, m.refresh()
		case "k", "up", "h", "left":
			was := m.selectedKey
			if m.level == levelBoard && m.boardShown() {
				m.boardMove(-1)
			} else if key == "k" || key == "up" {
				m.move(-1)
			} else {
				return m, nil
			}
			if m.selectedKey == was && m.note == "" {
				m.note = m.onlyOrLast(-1)
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
			if len(TrailRows(m.trail, m.level)) <= 1 {
				m.note = "the trail is one row"
			}
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
			if len(TrailRows(m.trail, m.level)) <= 1 {
				m.note = "the trail is one row"
			}
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
	m.note = fmt.Sprintf("◉ %d/%d · %s · %s", nth, len(prompts), `"`+rows[target].Text+`"`, rows[target].Time.Local().Format("15:04")) // the footer clips the quote to its room
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
		m.note = "the deepest level"
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
		if m.boardFits() && !m.archiveView && m.liveCount() == 1 {
			m.note = "the only session · nothing to zoom out to"
			if s, ok := m.selected(); ok && m.fleetQuery != "" && !m.matchesQuery(s) {
				m.clearQuery() // no board to go out to: the query the session fails goes here instead
			}
			return
		}
		m.level = levelTrail
		m.cursor, m.anchor = -1, -1
		if m.boardFits() && !m.archiveView {
			m.level = levelBoard        // the session view came from the board; back to it
			m.commitLook(m.selectedKey) // closing the session is the look: the digest stops billing
			if s, ok := m.selected(); ok && m.fleetQuery != "" && !m.matchesQuery(s) {
				// A board the query would hide this session from: the
				// query goes, since a board without the session that was
				// just open answers "no session matches" beside its keys.
				m.clearQuery()
			}
		}
	case m.level > levelBoard && m.boardFits():
		m.level = levelBoard
		m.commitLook(m.selectedKey)
	case m.level == levelTrail:
		m.note = fmt.Sprintf("no board under %d columns", deckWideCols)
	case m.level == levelBoard:
		m.note = "the board is the top"
	}
}

// clearQuery drops the fleet search and puts the selection back where it
// was before the search began, if that session is still on the board.
func (m *Model) clearQuery() {
	m.fleetQuery = ""
	if m.querySel != "" {
		m.point(m.querySel)
		m.querySel = ""
	}
	m.clampSelection()
	m.note = "search cleared"
}

// liveCount is how many sessions are on the board, query or no query.
func (m *Model) liveCount() int {
	n := 0
	for _, s := range m.sessions {
		if m.onBoard(s) {
			n++
		}
	}
	return n
}

// commitLook records a look that is over: the session was read and closed,
// so the read-line goes to the present and the digest has nothing to add.
func (m *Model) commitLook(key string) {
	if key == "" {
		return
	}
	if m.seen == nil {
		m.seen = make(map[string]time.Time)
	}
	m.seen[key] = m.now
	delete(m.lastLook, key)
	delete(m.opened, key)
	m.saveSeen()
}

// looked is the moment the trail's read-line stands for: the look before
// the current one while the session is open, the last look otherwise.
func (m *Model) looked(key string) time.Time {
	if at, ok := m.lastLook[key]; ok && key == m.selectedKey {
		return at
	}
	return m.seen[key]
}

// matchesQuery says whether a session answers the fleet search: its name,
// its opening prompt, its branch, any prompt of its trail, a leg's label,
// or a file a leg touched. Three hundred archived sessions are a corpus,
// and a scroll was the only way through it.
func (m *Model) matchesQuery(s fleet.Session) bool {
	q := strings.ToLower(strings.TrimSpace(m.fleetQuery))
	if q == "" {
		return true
	}
	has := func(text string) bool { return strings.Contains(strings.ToLower(text), q) }
	if has(sessionName(s.Info)) || has(s.Info.Title) || has(s.Info.GitBranch) {
		return true
	}
	tr := m.trails[s.Info.Key()]
	for _, p := range tr.Prompts {
		if has(p.Text) {
			return true
		}
	}
	for _, l := range tr.Legs {
		if has(l.Label) {
			return true
		}
		for _, f := range l.Files {
			if has(f) {
				return true
			}
		}
	}
	return false
}

// onBoard says whether a live session is shown in the live view: hidden
// ones are not, unless they need you or are stuck — a session taken off
// the board comes back the moment it has something to say.
func (m *Model) onBoard(s fleet.Session) bool {
	if !s.Live {
		return false
	}
	if !m.hidden[s.Info.Key()] {
		return true
	}
	if _, _, loop := circling(m.trails[s.Info.Key()]); loop {
		return true
	}
	return s.Snap.State == state.NeedsYou || s.Snap.State == state.Stuck
}

// hiddenCount is how many live sessions are off the board right now.
func (m *Model) hiddenCount() int {
	n := 0
	for _, s := range m.sessions {
		if s.Live && !m.onBoard(s) {
			n++
		}
	}
	return n
}

// toggleHidden is `x`: the selected session leaves the board — a test
// session, a /resume you are done with — and stays off it until `x` again
// in the archive, where hidden sessions are listed, or until it needs you.
func (m *Model) toggleHidden() {
	s, ok := m.selected()
	if !ok {
		return
	}
	key := s.Info.Key()
	if m.hidden == nil {
		m.hidden = map[string]bool{}
	}
	if m.hidden[key] {
		delete(m.hidden, key)
		m.saveHidden()
		m.note = sessionName(s.Info) + " is back on the board"
		return
	}
	if !s.Live {
		m.note = "the archive is already off the board"
		return
	}
	if m.liveCount() <= 1 && !m.archiveView {
		m.note = "the only session stays"
		return
	}
	// What owes you an alarm stays, and says so: a note that reported a
	// hide while the column stood was the screen lying.
	name := sessionName(s.Info)
	switch {
	case s.Snap.APIError:
		m.note = name + " stays · dead on the API" // short enough to stand whole beside the help at 80
		return
	case s.Snap.State == state.NeedsYou:
		m.note = name + " stays · it is asking"
		return
	case s.Snap.State == state.Stuck:
		m.note = name + " stays · it hangs"
		return
	case m.isCircling(s):
		m.note = name + " stays · it is circling"
		return
	}
	// Where the selection goes: the neighbour as drawn — the next column,
	// or the next row of the list — not the first column.
	drawn := func() []int {
		if m.boardShown() {
			return m.viewOrder()
		}
		return m.fleetOrder()
	}
	pos := 0
	for i, idx := range drawn() {
		if m.sessions[idx].Info.Key() == key {
			pos = i
		}
	}
	m.hidden[key] = true
	m.saveHidden()
	// The note names the session the way its row does — digit, and the
	// pane when a namesake shares its tmux session.
	if d := m.digits[key]; d > 0 {
		name = strconv.Itoa(d) + " " + name
	}
	m.note = name + " hidden · A, then x" // the strip's own form: it fits eighty columns beside the keys
	if m.sharesTmux(s) {
		if pane, ok := m.panes[key]; ok {
			m.note += " · " + mirrorMark + " " + pane.Target // the last clause, the first shed
		}
	}
	if !m.archiveView {
		if order := drawn(); len(order) > 0 {
			m.pointQuiet(m.sessions[order[min(pos, len(order)-1)]].Info.Key())
		}
		m.clampSelection()
	}
}

// fireHooks runs the event hook for every session whose state crossed a
// line since the last refresh: into needs-you (an API error named as such),
// into stuck, into circling, or lanes coming back. The first refresh sets
// the baseline and fires nothing — a launch is not an event.
func (m *Model) fireHooks() {
	run := m.hookRun
	if run == nil && m.hook != "" {
		hook := m.hook
		run = func(event, session, tmux, detail string) {
			cmd := exec.Command("sh", "-c", hook)
			cmd.Env = append(os.Environ(),
				"COMPASS_EVENT="+event, "COMPASS_SESSION="+session, "COMPASS_TMUX="+tmux, "COMPASS_DETAIL="+detail)
			go func() { _ = cmd.Run() }()
		}
	}
	first := m.before == nil
	if first {
		m.before = map[string]hookState{}
	}
	for _, s := range m.sessions {
		if !s.Live {
			continue
		}
		key := s.Info.Key()
		tr := m.trails[key]
		now := hookState{state: s.Snap.State, apiError: s.Snap.APIError}
		_, _, now.circling = circling(tr)
		now.shippedOnRed = strings.Contains(boardVerdict(s, tr, m.now), "shipped on red")
		for _, b := range tr.Branches {
			if b.Done {
				now.back++
			}
		}
		was, known := m.before[key]
		m.before[key] = now
		if first || !known || run == nil {
			continue
		}
		tmux := ""
		if pane, ok := m.panes[key]; ok {
			tmux = pane.Target
		}
		name := sessionName(s.Info)
		// One firing per session and event every ten minutes: a session
		// flapping across a line is one call, not a fork storm.
		fire := func(event, detail string) {
			if m.hookFired == nil {
				m.hookFired = map[string]time.Time{}
			}
			if at, ok := m.hookFired[key+"|"+event]; ok && m.now.Sub(at) < hookCoolOff {
				return
			}
			m.hookFired[key+"|"+event] = m.now
			run(event, name, tmux, detail)
		}
		switch {
		case now.state == state.NeedsYou && (was.state != state.NeedsYou || now.apiError && !was.apiError):
			event := "needs_you"
			if now.apiError {
				event = "api_error"
			}
			fire(event, strings.TrimSpace(m.headFor(s)))
		case now.state == state.Stuck && was.state != state.Stuck:
			fire("stuck", strings.TrimSpace(m.headFor(s)))
		}
		if now.circling && !was.circling {
			test, runs, _ := circling(tr)
			fire("circling", fmt.Sprintf("%s · %s failure", test, ordinal(runs)))
		}
		if now.back > was.back {
			// One call for the set that came back this refresh, saying how
			// many were empty: "returned" and "returned empty" are
			// different phone calls.
			empty := 0
			for _, b := range tr.Branches {
				if b.Done && strings.TrimSpace(b.Report) == "" {
					empty++
				}
			}
			detail := plural(now.back-was.back, "lane") + " returned"
			if empty > 0 {
				detail += fmt.Sprintf(" · %d empty", empty)
			}
			fire("agents_back", detail)
		}
		if now.shippedOnRed && !was.shippedOnRed {
			fire("shipped_on_red", "a commit on top of a red run")
		}
	}
}

// hookCoolOff is the least time between two firings of one event for one
// session.
const hookCoolOff = 10 * time.Minute

// onlyOrLast is what a move that moved nothing says: the fleet has one
// session, or the selection is at its end.
func (m *Model) onlyOrLast(delta int) string {
	if len(m.viewOrder()) <= 1 {
		return "the only session"
	}
	if delta > 0 {
		return "the last session"
	}
	return "the first session"
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
		m.note = "no pane · nothing to type into"
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
	if s, ok := m.selected(); ok && s.Snap.APIError {
		// Dead on the API: the remedy the refusal names, then the quota
		// line. "please continue" into a 403 is a turn that 403s again.
		if strings.Contains(s.Snap.Activity, "/login") {
			out = append(out, replyChoice{label: "/login", text: "/login", kind: replyRemedy})
		}
		for _, r := range m.replies {
			if r == quotaReply {
				out = append(out, replyChoice{label: r, text: r, kind: replyLine})
			}
		}
		return out
	}
	for _, r := range m.replies {
		if r == quotaReply {
			// The stock quota line only where a quota was hit: it was
			// offered to a session fifty seconds old.
			if s, ok := m.selected(); !ok || !m.hadAPIError(s) {
				continue
			}
		}
		out = append(out, replyChoice{label: r, text: r, kind: replyLine})
	}
	if s, ok := m.selected(); ok && (s.Snap.State == state.Working || s.Snap.State == state.Stuck) {
		// Only where there is a turn to interrupt: an idle session
		// offered "stop" under "waiting for a prompt".
		out = append(out, replyChoice{label: "stop — interrupt the turn", kind: replyStop})
	}
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
		m.note = "no pane · nothing to type into"
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
		text := c.text
		if text == "" {
			text = c.label // an answer or stop: the label is the act
		}
		return replyDoneMsg{key: key, target: target, text: text, answer: c.n, err: err}
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
		m.note = "no pane · nothing to attach to"
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
	if !m.archiveView {
		// Nothing selected yet: the board's first column — what owes
		// you most — not the first row of a list that opened on a corpse.
		order = m.viewOrder()
	}
	m.point(m.sessions[order[0]].Info.Key())
}

// toggleArchive swaps the two fleets, and their selections with them: the live
// view remembers where you were standing while you read an old journey.
func (m *Model) toggleArchive() {
	if !m.archiveView && m.archivedCount() == 0 && m.hiddenCount() == 0 {
		m.note = "nothing archived yet"
		return
	}
	m.archiveView = !m.archiveView
	m.selectedKey, m.restSelKey = m.restSelKey, m.selectedKey
	m.fleetScroll = 0
	if m.archiveView && m.hiddenCount() > 0 {
		// Something is hidden: the archive opens on it — the note said
		// "A, then x", and a remembered cursor on an old row beat the row
		// the person came for.
		if s, ok := m.selected(); !ok || !s.Live {
			m.selectedKey = ""
			for _, s := range m.sessions {
				if s.Live && !m.onBoard(s) {
					m.selectedKey = s.Info.Key()
					break
				}
			}
		}
	}
	if m.selectedKey != "" {
		// A remembered key is a fresh selection for everything downstream
		// — and not a look: compass chose where to land.
		key := m.selectedKey
		m.selectedKey = ""
		m.pointQuiet(key)
	}
	m.clampSelection()
}

// point moves the selection. Trail and mirror belong to the session that was
// selected, so they leave with it rather than lingering as somebody else's.
func (m *Model) point(key string) {
	m.pointAs(key, false)
}

// pointQuiet moves the selection without reading the session it lands on:
// a hide or a search moved it, not the person.
func (m *Model) pointQuiet(key string) {
	m.pointAs(key, true)
}

func (m *Model) pointAs(key string, quiet bool) {
	if key == m.selectedKey {
		return
	}
	if old := m.selectedKey; old != "" && m.level >= levelTrail && !m.boardShown() && m.opened[old] {
		// No board: the trail on screen was the old session's, and
		// leaving it closes it — the look is over, and its digest was
		// billing for legs the person had just read. Only a session the
		// person opened: a search's landing is not a look on the way out
		// either.
		m.commitLook(old)
	}
	m.selectedKey = key
	if m.level >= levelTrail && !m.boardShown() && !(m.searching && m.searchFleet) && !quiet {
		// No board: selecting a session puts its trail on screen, which
		// is opening it. "unread" never cleared at 100 columns. A search
		// or a hide moving the selection is not a look.
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
	if !m.searchFleet {
		// The reader's search belongs to the session that left. The
		// fleet's does not: narrowing it moved the selection, and the
		// move ended the typing — the next key went to the deck, and
		// enter attached instead of keeping the query.
		m.query, m.draft, m.searching = "", "", false
	}
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
		if s.Live && s.Snap.State == state.NeedsYou && !s.Snap.APIError {
			// A session dead on the API is not one a keypress helps.
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
		if m.onBoard(s) && s.Snap.State == state.NeedsYou && !s.Snap.APIError {
			n++
		}
	}
	return n
}

// apiErrorCount is how many sessions on the board are dead on the API.
func (m *Model) apiErrorCount() int {
	n := 0
	for _, s := range m.sessions {
		if m.onBoard(s) && s.Snap.APIError {
			n++
		}
	}
	return n
}

// titleCmd emits an OSC 2 tab title only when the attention count changes, so
// an unfocused terminal tab carries the fleet's health (SPEC §2.4).
func (m *Model) titleCmd() tea.Cmd {
	title := tabTitle(m.needsYouCount(), m.stuckCount(), m.circlingCount(), m.apiErrorCount())
	if title == m.lastTitle {
		return nil
	}
	m.lastTitle = title
	return tea.SetWindowTitle(title)
}

// tabTitle is the terminal's title: the alarms, so a tab bar says what
// the deck says without the deck being looked at.
func tabTitle(needsYou, stuck, loops, dead int) string {
	title := "⌂ compass"
	if needsYou > 0 {
		title += fmt.Sprintf(" ▲%d", needsYou)
	}
	if stuck > 0 {
		title += fmt.Sprintf(" ◍%d", stuck)
	}
	if loops > 0 {
		title += fmt.Sprintf(" ↻%d", loops)
	}
	if dead > 0 {
		title += fmt.Sprintf(" %s%d", glyphAPIError, dead)
	}
	return title
}

func (m *Model) stuckCount() int {
	n := 0
	for _, s := range m.sessions {
		if m.onBoard(s) && s.Snap.State == state.Stuck {
			n++
		}
	}
	return n
}

func (m *Model) circlingCount() int {
	n := 0
	for _, s := range m.sessions {
		if m.isCircling(s) {
			n++
		}
	}
	return n
}

// isCircling says whether a session on the board is going round the same
// failure: the loop wears its own glyph and word on the row, so the
// header's count has a referent. A session asking or hung keeps its own
// alarm; a loop is the one the state machine called healthy.
func (m *Model) isCircling(s fleet.Session) bool {
	if !m.onBoard(s) || s.Snap.State == state.NeedsYou || s.Snap.State == state.Stuck {
		return false
	}
	_, _, ok := circling(m.trails[s.Info.Key()])
	return ok
}

// rowGlyph is the glyph a session's row wears: the fleet's own for its
// state, ↻ for a session going round the same failure.
func (m *Model) rowGlyph(s fleet.Session) string {
	if m.isCircling(s) {
		return glyphCircling
	}
	if s.Snap.APIError {
		return glyphAPIError
	}
	return fleet.Glyph(s.Snap.State)
}

// glyphCircling marks a session failing the same test leg after leg.
const glyphCircling = "↻"

// glyphAPIError marks a session dead on the API — a refusal nothing you
// type clears, which the needs-you glyph read as a question.
const glyphAPIError = "⊘"

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
		body = helpLinesWith(inner, bodyHeight, helpOpts{board: m.boardFits(), refused: m.refusedKeys(), keymap: m.keymapAt(inner)})
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
		left, top, cap := m.panelPlace(inner, panelWidth(panel), len(panel), false)
		if len(panel) > cap {
			// The box would run into the head rows of the band below:
			// its rows of air go first, and the tighter box is placed
			// again — on its own band's trail rows when it now fits
			// there, else wherever the rows are free.
			panel = m.replyPanelN(inner, cap)
			left, top, _ = m.panelPlace(inner, panelWidth(panel), len(panel), true)
		}
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
		// And what is right of it stays too: a seventy-column box was
		// blanking five columns of board to draw itself.
		after := ""
		if lipgloss.Width(line) > left+pw {
			rest := ansi.TruncateLeft(line, left+pw+1, "")
			// A cut inside a token read as a wrong number ("…4m ago" for
			// 14m): the peek begins at the next space.
			if plain := ansi.Strip(rest); len(plain) > 0 && plain[0] != ' ' {
				if i := strings.Index(plain, " "); i >= 0 {
					cut := ansi.StringWidth(plain[:i]) // cells, not bytes
					rest = strings.Repeat(" ", cut) + ansi.TruncateLeft(rest, cut, "")
				} else {
					rest = strings.Repeat(" ", lipgloss.Width(rest))
				}
			}
			after = "…" + rest
		}
		rows[top+i] = before + pad(p, pw) + after
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
	return m.replyPanelN(inner, m.height-5)
}

// replyPanelAir says whether a panel ph rows tall still carries its rows of
// air — that is, whether shedding them would make it shorter.
func (m *Model) replyPanelAir(ph int) bool {
	return len(m.replyPanelN(m.width-2*edgePad, 0)) < ph
}

// replyPanelN is the panel within avail rows: past that, its rows of air go
// before the box loses its bottom or covers a head row.
func (m *Model) replyPanelN(inner, avail int) []string {
	name, target, who := "—", "", ""
	s, ok := m.selected()
	if ok {
		name = sessionName(s.Info)
		if d := m.digits[s.Info.Key()]; d > 0 && !m.archiveView {
			// The row's own digit, which is the session's for life — a
			// position named another session on the same screen.
			who = strconv.Itoa(d) + " · "
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
	if fw, _, _ := m.layout(inner); fw > 0 && !m.boardShown() && m.level < levelReader {
		// Beside a fleet list the panel stands over the trail and leaves
		// the list legible; at eighty columns it was standing on both.
		if max := inner - fw - gutterWidth - 6; body > max {
			body = max
		}
	}
	if body < 20 {
		body = 20
	}
	var rows []readerLine
	if ok {
		if s.Snap.APIError {
			// A dead session's state is one row, clipped: the row
			// beneath carries the refusal already and the reader has
			// the rest, and a four-row state stood the box on two other
			// alarms' verdict rows.
			rows = append(rows, readerLine{text: clip(m.replyState(s), body), kind: readerBody})
		} else {
			for _, line := range wrapPrefix(m.replyState(s), "", "", body) {
				rows = append(rows, readerLine{text: line, kind: readerBody})
			}
		}
		rows = append(rows, readerLine{kind: readerBlank})
	}
	// Each group says what its keys press: an answer is the CLI menu's
	// own digit, a line is typed and entered, stop is escape. One sentence
	// over the box described the dangerous one and not the other.
	choices := m.replyChoices()
	heads := map[replyKind]string{
		replyAnswer: "answers · sent as the menu's own digit",
		replyRemedy: "remedy · typed into the pane and entered · log in again",
		replyLine:   "lines · typed into the prompt and entered",
		replyStop:   "stop · escape, which interrupts the turn",
	}
	if ok && s.Snap.APIError {
		// A dead session: a stock line starts a turn into the refusal,
		// and the panel says so rather than offering it as a reply.
		heads[replyLine] = "lines · start a turn — only once the quota is back"
	}
	if body < 44 {
		// The narrow panel: the mechanism, in fewer words — and on a
		// dead session the warning, which is the head's whole point.
		heads[replyAnswer] = "answers · the menu's digit"
		heads[replyRemedy] = "remedy · log in again"
		heads[replyLine] = "lines · typed and entered"
		heads[replyStop] = "stop · escape"
		if ok && s.Snap.APIError {
			heads[replyLine] = "lines · only once the quota is back"
		}
	}
	if len(choices) > 0 && choices[0].kind == replyAnswer {
		// Under a menu a typed line lands in the menu: said, so the
		// answers above read as the safer keys they are.
		heads[replyLine] = "lines · typed into the menu — the answers above are safer"
	}
	lastKind := replyKind(-1)
	for i, c := range choices {
		if c.kind != lastKind {
			if i > 0 {
				rows = append(rows, readerLine{kind: readerBlank})
			}
			for _, line := range wrapPrefix(heads[c.kind], "", "", body) {
				rows = append(rows, readerLine{text: line, kind: readerBody})
			}
		}
		lastKind = c.kind
		kind := readerText
		if c.kind == replyStop {
			kind = readerFoldErr
		}
		for _, line := range wrapPrefix(c.label, fmt.Sprintf("%d  ", i+1), "   ", body) {
			rows = append(rows, readerLine{text: line, kind: kind})
		}
	}
	rows = append(rows, readerLine{kind: readerBlank})
	if m.replyTyping {
		rows = append(rows, readerLine{text: "› " + m.replyDraft + "▏", kind: readerSaid},
			readerLine{text: "enter sends · esc back to the menu", kind: readerBody})
	} else {
		typed := "t  type a line — entered the same way"
		if ok && s.Snap.APIError {
			typed = "t  type a line — a turn, too"
		}
		rows = append(rows, readerLine{text: typed, kind: readerText},
			readerLine{text: "a digit acts · t types · esc closes", kind: readerBody})
	}
	if len(rows)+2 > avail {
		// Too tall for the body: the rows of air go before the box loses
		// its bottom.
		kept := rows[:0]
		for _, r := range rows {
			if r.kind != readerBlank {
				kept = append(kept, r)
			}
		}
		rows = kept
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
	if s.Snap.APIError {
		text := strings.TrimSpace(s.Snap.Activity)
		if text == "" {
			text = s.Snap.Reason
		}
		return glyphAPIError + " stopped on an API error " + since + " ago · " + text + " · no turn takes a line; the remedy is typed into the pane"
	}
	switch s.Snap.State {
	case state.NeedsYou:
		q := strings.TrimSpace(m.headFor(s))
		if q == "" {
			q = "a question"
		}
		return "▲ on a question · " + q + " — pick an answer below, or type a line into that prompt"
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
// The third value is the rows the panel may take from top without covering
// a head row: on the board, the band below begins where the selected band's
// trail rows end, and a box over its name rows left a band with a verdict
// and a trail and no session named. A panel taller than that is rebuilt
// without its air and placed again.
func (m *Model) panelPlace(inner, pw, ph int, tight bool) (left, top, cap int) {
	body := m.height - 5
	if m.level == levelBoard && m.boardShown() {
		if x, y, bh, last, ok := m.boardBandAt(inner); ok {
			top := y + 3
			left := min(x, max(inner-pw, 0))
			if last {
				if bh < ph && y+bh+1+ph <= body {
					// A band shorter than the panel, with free rows under it:
					// the panel stands under the band rather than on the strip.
					top = y + bh + 1
				} else if top+ph > body && y >= ph {
					// Lifting the box to fit would cover the row it is about:
					// it stands above the row instead, where the rows are free.
					top = y - ph
				}
				return left, top, max(body-top, 0)
			}
			next := y + bh + 1 // the head row of the band below
			switch {
			case top+ph <= next:
				// On its own band's trail rows, the band below untouched.
				return left, top, next - top
			case m.replyPanelAir(ph) && !tight:
				// Its rows of air would run into the band below: the
				// caller sheds them and asks again, so the box stays by
				// the row it is about.
				return left, top, next - top
			case next+3+ph <= body:
				// Too tall for those even tight: under the head rows of
				// the band below, over that band's trail — every session
				// on the board keeps its name.
				return left, next + 3, body - (next + 3)
			}
			return left, top, next - top
		}
		return 0, 3, max(body-3, 0)
	}
	fw, mw, _ := m.layout(inner)
	switch {
	case fw == 0 && mw > 0:
		_, tw := sessionSplit(inner)
		return min(tw+gutterWidth, max(inner-pw, 0)), 2, max(body-2, 0)
	case fw > 0:
		return min(fw+gutterWidth, max(inner-pw, 0)), 2, max(body-2, 0)
	}
	return 0, 2, max(body-2, 0)
}

// headerLine: the product mark on the left, the fleet's pulse on the right.
func (m *Model) headerLine(w int) string {
	left := titleStyle.Render("⌂ compass")
	if m.level == levelBoard && m.boardShown() {
		left += dimStyle.Render(" · board")
	}
	if m.fleetQuery != "" {
		// The search in force, and how much of the fleet answers it.
		total := 0
		for _, s := range m.sessions {
			if s.Live != m.archiveView {
				total++
			}
		}
		left += dimStyle.Render(fmt.Sprintf(" · /%s · %d of %d", m.fleetQuery, len(m.viewOrder()), total))
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
	// One chip per session: a circling session is counted under ↻ and a
	// dead one under ⊘, and nowhere else — the tally summed to one more
	// than the board drew, and the count is the first thing read.
	counts := map[state.State]int{}
	oldest := map[state.State]time.Time{}
	loops, dead, loopSince, deadSince, deadWord := 0, 0, time.Time{}, time.Time{}, ""
	for _, s := range m.sessions {
		if !m.onBoard(s) {
			continue
		}
		switch {
		case s.Snap.APIError:
			dead++
			if deadSince.IsZero() || headSince(s).Before(deadSince) {
				deadSince = headSince(s)
			}
			if w := apiWord(s); deadWord == "" || w == deadWord {
				deadWord = w
			} else {
				deadWord = "api error" // more than one kind: the general word
			}
			continue
		case m.isCircling(s):
			loops++
			if at := circlingSince(m.trails[s.Info.Key()]); !at.IsZero() && (loopSince.IsZero() || at.Before(loopSince)) {
				loopSince = at
			}
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
	for _, st := range []state.State{state.NeedsYou, state.Stuck} {
		if n := counts[st]; n > 0 {
			// The states you must not miss carry their wait: "▲1 4m" is
			// a change you can read from across the room, where a census
			// is not.
			parts = append(parts, stateStyle(st).Render(fmt.Sprintf("%s%d %s", fleet.Glyph(st), n, m.age(oldest[st]))))
		}
	}
	if loops > 0 {
		chip := fmt.Sprintf("↻%d", loops)
		if !loopSince.IsZero() {
			chip += " " + m.age(loopSince) // the loop's age, as its row counts it
		}
		parts = append(parts, stuckStyle.Render(chip))
	}
	if dead > 0 {
		parts = append(parts, needsYouStyle.Render(fmt.Sprintf("%s%d %s %s", glyphAPIError, dead, deadWord, m.age(deadSince))))
	}
	for _, st := range []state.State{state.Working, state.Idle} {
		if n := counts[st]; n > 0 {
			parts = append(parts, stateStyle(st).Render(fmt.Sprintf("%s%d", fleet.Glyph(st), n)))
		}
	}
	// Agents out are work in flight nobody's glyph shows: "●2 ○2 all calm"
	// over four lanes still out twenty minutes in was a claim, and wrong.
	out, oldestOut := 0, time.Time{}
	for _, s := range m.sessions {
		if !m.onBoard(s) || s.Snap.State == state.Idle {
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
		chip := fmt.Sprintf("archive %d", m.archivedCount())
		if n := m.hiddenCount(); n > 0 {
			chip += fmt.Sprintf(" · %d hidden", n) // the list holds both; the chip says so
		}
		parts = append(parts, dimStyle.Render(chip))
	}
	if len(parts) == 0 {
		return dimStyle.Render("○ all quiet")
	}
	// What owes you: every session with a column — the alarms, the work
	// in flight, what stopped red or with steps left, what is not read
	// yet — counted as one number, with the unread named beside it.
	// "all calm" beside "2 unread" was the header contradicting itself.
	owed, unread := 0, 0
	for _, s := range m.sessions {
		if !m.onBoard(s) {
			continue
		}
		switch m.obligation(s) {
		case rankUnread:
			unread++
		case rankOwed:
			owed++
		}
	}
	if owed > 0 {
		parts = append(parts, dimStyle.Render(fmt.Sprintf("%d owe you", owed)))
	}
	if unread > 0 {
		parts = append(parts, dimStyle.Render(fmt.Sprintf("%d unread", unread)))
	}
	if counts[state.NeedsYou] == 0 && counts[state.Stuck] == 0 && loops == 0 && dead == 0 && out == 0 && owed == 0 && unread == 0 && !m.archiveView {
		// Calm, said aloud: the absence of a warm glyph is the design, and
		// in monochrome an absence is also what a clipped header looks like.
		parts = append(parts, dimStyle.Render("all calm"))
	}
	return strings.Join(parts, "  ")
}

// footerLine carries the keymap, and — briefly, on the right — whatever the
// last keypress did.
func (m *Model) footerLine(w int) string {
	keys := m.keymap()
	return m.footerWith(keys, w)
}

// keymap is the whole keymap for where the keys are now, before any of it
// is shed for width: the row's promise, and what the help asks when it has
// to choose which key rows a short body keeps.
func (m *Model) keymap() string {
	keys := "j/k move · " + m.enterKeymap() + " · tab deeper · [ ] chapters · r reply · a ask · / search · x hide · g grab · ? help · q quit"
	if m.archiveView {
		// In the archive `g` has nothing to grab and `A` is the way home, so the
		// keymap says that instead. In the live view the archive announces itself
		// on the fleet's own last row: "N archived · A browses".
		keys = "j/k move · " + m.enterKeymap() + " · tab deeper · a ask · / search · x unhide · A fleet · ? help · q quit"
	}
	switch {
	case m.showHelp:
		keys = "? or esc closes help"
	case m.searching && m.searchFleet:
		keys = "/" + m.draft + "▏ · enter keeps it · esc cancels"
	case m.searching:
		keys = "type to search · enter finds · esc cancels"
	case m.replying && m.replyTyping:
		keys = "type the line · enter sends · esc back"
	case m.replying:
		keys = fmt.Sprintf("reply: 1–%d · t types a line · esc closes", len(m.replyChoices()))
	case m.level == levelBoard && m.boardShown():
		keys = "h/l columns · " + m.enterKeymap() + " · tab session · r reply · a ask · / search · x hide · g grab · ? help · q quit"
		if m.archiveView {
			keys = "h/l columns · " + m.enterKeymap() + " · tab session · / search · x unhide · A fleet · ? help · q quit"
		}
	case m.level == levelTrail && m.boardShown():
		keys = "j/k move · " + m.enterKeymap() + " · [ ] chapters · r reply · a ask · / search · ⇧tab board · g grab · ? help · q quit"
		if m.archiveView {
			keys = "j/k move · " + m.enterKeymap() + " · tab deeper · a ask · / search · x unhide · ⇧tab board · A fleet · ? help · q quit"
		}
	case m.level >= levelReader && m.sessionView():
		keys = "j/k scroll · space unfold · / search · n/N · [ ] turns · h/l session · r reply · a ask · " + m.enterKeymap() + " · esc back · ? help · q quit"
	case m.level >= levelReader:
		keys = "j/k scroll · space unfold · / search · n/N · [ ] turns · r reply · a ask · " + m.enterKeymap() + " · esc back · ? help · q quit"
	case m.level >= levelWaypoints && m.sessionView():
		keys = "j/k legs · h/l session · [ ] chapters · m live pane · r reply · a ask · tab reader · " + m.enterKeymap() + " · esc board · ? help · q quit"
	case m.level >= levelWaypoints:
		keys = "j/k rows · [ ] chapters · r reply · " + m.enterKeymap() + " · tab deeper · a ask · esc back · ? help · q quit"
	}
	if m.archiveView {
		if s, ok := m.selected(); !ok || !s.Live || m.onBoard(s) {
			keys = strings.Replace(keys, " · x unhide", "", 1) // the cursor is not on a hidden row: the key answers no question
		}
	}
	// The keymap sheds its optional fragments before it clips: a footer
	// that ends in "· ? he" says less than one without the chapters.
	// The view's own keys go last: `x`, `/` and `r` were shed at the
	// widths the walkthrough pressed them.
	if m.liveCount() == 1 && !m.archiveView {
		// One session: the keys that move between sessions answer no
		// question, and "esc board" beside "nothing to zoom out to" was
		// two answers.
		for _, drop := range []string{"h/l columns · ", " · h/l session", " · esc board", " · g grab", " · x hide"} {
			keys = strings.Replace(keys, drop, "", 1)
		}
	}
	if m.showMirror {
		keys = strings.Replace(keys, "m live pane", "m conversation", 1) // the toggle's other side
	}
	if s, ok := m.selected(); ok && !m.archiveView {
		if pane, has := m.panes[s.Info.Key()]; !has || pane.Target == "" {
			// `r` types into a pane, like `enter` attaches to one: a row
			// that says "no pane" does not offer the other write either.
			keys = strings.Replace(keys, " · r reply", "", 1)
		}
	}
	return keys
}

// keymapAt is the keymap as the footer draws it at this width, its
// optional keys shed: what the person can actually see being offered,
// which is what the help asks before it cuts a key row.
func (m *Model) keymapAt(w int) string {
	keys := shedKeys(m.keymap(), m.shedOrder(false), func(k string) bool { return lipgloss.Width(k) <= w })
	// The deck names keys outside its footer too: the fleet's last row
	// offers the archive, and a hide note names the way back. A key the
	// person can see being offered anywhere is one the help owes a row.
	if m.archivedCount() > 0 {
		keys += " · A browses"
	}
	if m.hiddenCount() > 0 {
		keys += " · A, then x"
	}
	return keys
}

// footerWith renders the keymap and the note into one row w wide.
func (m *Model) footerWith(keys string, w int) string {
	if m.note == "" {
		keys = shedKeys(keys, m.shedOrder(false), func(k string) bool { return lipgloss.Width(k) <= w })
		return dimStyle.Render(clip(keys, w))
	}
	var left string
	// The note is the news, but the keymap is the only place the reader's
	// keys are named: shed the keymap's fragments for the note first, and
	// clip the note before the keymap goes.
	// The keys a note is about — the chapters it counts, the reply it
	// reports — go last: a footer that dropped `[ ] turns` on the frame
	// that said "❯ 3/12" read as the key having gone.
	drops := m.shedOrder(m.chapterNote())
	note := m.note
	fitsWith := func(k, n string) bool { return lipgloss.Width(k)+2+max(12, lipgloss.Width(n)) <= w }
	fits := func(n string) bool { return fitsWith(keys, n) }
	// shed is the keys with their optional fragments gone, in order, until
	// the note fits — stopping short of `upto` when one is named. It
	// always starts from the whole keymap: a key shed for a clause the
	// note then gave up stayed shed, and 13 blank columns stood where
	// `r reply` had been.
	whole := keys
	shed := func(n, upto string) string {
		order := drops
		if upto != "" {
			for i, drop := range drops {
				if drop == upto {
					order = drops[:i]
					break
				}
			}
		}
		return shedKeys(whole, order, func(k string) bool { return fitsWith(k, n) })
	}
	pane := "" // the clause the note gave up, if the keys leave room for it after all
	if i := strings.LastIndex(note, " · "); i > 0 && !fits(note) {
		// The note's pane clause — a fact the card's third row carries
		// too — goes before any key does.
		if tail := note[i+len(" · "):]; strings.HasPrefix(tail, mirrorMark) {
			note, pane = note[:i], note[i:]
		}
	}
	if strings.HasPrefix(note, "↪ answered") && !fits(note) {
		// An answer's digit says which line went: its quote is the
		// label, and goes before the destination does.
		if i, j := strings.Index(note, " · "), strings.LastIndex(note, " · "); i > 0 && i != j && strings.Contains(note[i:j], `"`) {
			note = note[:i] + note[j:]
		}
	}
	if i := strings.LastIndex(note, " · "); i > 0 && !fits(note) && strings.HasPrefix(note[i+len(" · "):], "to "+mirrorMark) {
		// A trace's destination is the one clause that proves where a
		// line landed (§5): it outranks the optional keys and yields only
		// to the way out and the help.
		if k := shed(note, " · ? help"); fitsWith(k, note) {
			keys = k
		} else {
			note = note[:i]
		}
	}
	// The note's forms, fullest first: a chapter note gives up its quote
	// before its clock, any other its trailing clauses. The keys are shed
	// only as far as the note's shortest form needs, and the note then
	// takes the room the keys leave — "◉ 11/12" beside 37 blank columns
	// and three shed keys said less than either half could.
	forms := noteForms(note)
	// A chapter note's keys are shed against its first clause; any other
	// note keeps its clauses (the way back, "A, then x") over the optional
	// keys. `? help` goes last of all (#32): only when the note's first
	// clause alone cannot stand beside it.
	minimal := note
	if m.chapterNote() && (strings.HasPrefix(note, glyphSaid) || strings.HasPrefix(note, glyphPrompt)) {
		minimal = forms[len(forms)-1]
	}
	if !fits(minimal) {
		keys = shed(minimal, " · ? help")
		if !fitsWith(keys, minimal) && !fitsWith(keys, forms[len(forms)-1]) {
			keys = shed(minimal, "")
		}
	}
	if pane != "" && strings.Contains(keys, attachHint) {
		// The pane clause goes before a key — not before the attach
		// hint, which #31 ranks beneath a key or a note: the parenthetical
		// stood where the hidden session's pane should have been.
		if bare := strings.Replace(keys, attachHint, "", 1); fitsWith(bare, note+pane) {
			keys, note = bare, note+pane
			forms = noteForms(note)
		}
	}
	left = dimStyle.Render(clip(keys, w))
	room := w - lipgloss.Width(left) - 2
	if room < 12 {
		return dimStyle.Render(shedClauses(note, w)) // no keymap fits beside it
	}
	shown := ""
	for _, f := range forms {
		if lipgloss.Width(f) <= room {
			shown = dimStyle.Render(f)
			break
		}
		if q := fitQuote(f, room); q != "" {
			// The quote clipped to the room rather than dropped whole:
			// a 220-column footer cut a prompt at 38 with 23 to spare.
			shown = dimStyle.Render(q)
			break
		}
	}
	if shown == "" {
		shown = dimStyle.Render(shedClauses(forms[len(forms)-1], room))
	}
	gap := w - lipgloss.Width(left) - lipgloss.Width(shown)
	return left + strings.Repeat(" ", gap) + shown
}

// noteForms is a footer note at every length it can be shown, fullest
// first: whole; a chapter note without its quote (the clock stays); then
// each trailing clause shed in turn, down to the first.
// refusedKeys are the keys this deck would refuse right now: the help does
// not name them, and on a short body their rows go to the keys the footer
// is offering instead.
func (m *Model) refusedKeys() []string {
	var refused []string
	if m.liveCount() == 1 && !m.archiveView {
		refused = append(refused, "g", "x") // nothing to grab, and hiding the only session is refused
	}
	return refused
}

// attachHint is the parenthetical the footer carries outside tmux: the
// lowest-ranked fragment on the row (#31), beneath a key or a note.
const attachHint = " (prefix d returns)"

// shedKeys gives up the keymap's optional fragments in order until fits
// holds, then puts back, most recently shed first, each one that fits
// after all: a greedy shed left a list with 14 free cells and none of
// `/ search`, `x hide`, `g grab` on it because `[ ] chapters` went last
// for one cell.
func shedKeys(whole string, order []string, fits func(string) bool) string {
	gone := map[string]bool{}
	build := func() string {
		k := whole
		for _, frag := range order {
			if gone[frag] {
				k = strings.Replace(k, frag, "", 1)
			}
		}
		return k
	}
	var shed []string
	for _, frag := range order {
		if fits(build()) {
			break
		}
		if strings.Contains(whole, frag) {
			gone[frag] = true
			shed = append(shed, frag)
		}
	}
	for i := len(shed) - 1; i >= 0; i-- {
		if shed[i] == " · n/N" && gone[" · / search"] {
			continue // the walk keys ride only with the search they walk
		}
		gone[shed[i]] = false
		if !fits(build()) {
			// A key comes back only if every key ranked above it came
			// back too: `a ask` returned to a reader row that could not
			// fit `[ ] turns`, and the row offered the lesser key.
			gone[shed[i]] = true
			break
		}
	}
	return build()
}

// chapterNote says whether the note is a chapter key's: a chapter counted,
// or a chapter key's refusal — either keeps the chapter keys (#24).
func (m *Model) chapterNote() bool {
	for _, p := range []string{glyphSaid, glyphPrompt, "no later prompt", "no earlier prompt", "no later turn", "no earlier turn"} {
		if strings.HasPrefix(m.note, p) {
			return true
		}
	}
	return false
}

// shedOrder is the order the footer gives up its optional keys in, first
// to go first. The way in (`tab deeper`) outlasts `x hide` and `g grab`,
// which refuse on a fleet of one, on the list — deeper in, the person has
// pressed it, and the chapter keys outlast it. A chapter note keeps the
// chapter keys over everything; `? help` goes last of all.
func (m *Model) shedOrder(chapter bool) []string {
	// First to go first. What every level shares — the attach hint, `a
	// ask`, the between-sessions keys — goes before anything a level owns,
	// and every level keeps its own keys longest: the chapter keys are the
	// trail's and the reader's, not the list's, and `space unfold` is the
	// reader's alone.
	// `m` ranks the same whichever side the toggle is on: its label
	// changes with the mirror's state, and a rank that changed with it
	// made the row's order flip under one keypress.
	mirror := " · m live pane"
	if m.showMirror {
		mirror = " · m conversation" // the toggle's other label, the same rank
	}
	order := []string{attachHint, mirror, " · h/l session"}
	// The way in and the way out are not shared in the same sense as
	// `h/l session` or the attach hint — they are how you enter and leave
	// this level — so they stand with the level's own keys below, ranked
	// under the keys the level is for.
	var own []string
	switch {
	case m.level >= levelReader:
		own = []string{" · g grab", " · x hide", " · x unhide", " · tab deeper", " · tab reader", " · [ ] chapters", " · r reply", " · n/N", " · / search", " · a ask", " · enter attach", " · enter · no pane", " · esc back", " · esc board", " · [ ] turns", " · space unfold"}
	case m.level >= levelWaypoints:
		own = []string{" · g grab", " · n/N", " · / search", " · x hide", " · x unhide", " · space unfold", " · [ ] turns", " · a ask", " · enter attach", " · enter · no pane", " · esc back", " · esc board", " · r reply", " · tab deeper", " · tab reader", " · [ ] chapters"}
	default:
		// The board and the list: the chapters belong to a trail that is
		// not open, and the way in outlasts the keys that act on a row.
		// In the archive `a` is the reason to be there — a claude on a
		// session you can no longer attach to — so it stands with the
		// archive's own keys; on the live list it is the trail's.
		own = []string{" · [ ] chapters", " · [ ] turns", " · space unfold", " · a ask", " · n/N", " · / search", " · g grab", " · x hide", " · r reply", " · tab deeper", " · enter attach", " · enter · no pane", " · x unhide"}
		if m.archiveView {
			own = []string{" · [ ] chapters", " · [ ] turns", " · space unfold", " · n/N", " · / search", " · g grab", " · x hide", " · r reply", " · tab deeper", " · enter attach", " · enter · no pane", " · a ask", " · x unhide"}
		}
	}
	order = append(order, own...)
	// The way out is the last key to go before the help — but a chapter
	// note's own keys go after it (#24): the row refusing `]` must carry
	// `[ ] chapters`, and every other row must carry the way out.
	var out []string
	for i := 0; i < len(order); i++ {
		if order[i] == " · esc back" || order[i] == " · esc board" {
			out = append(out, order[i])
			order = append(order[:i], order[i+1:]...)
			i--
		}
	}
	order = append(order, out...)
	if chapter {
		// A chapter key's note keeps the chapter keys over everything but
		// the way out and the help (#24).
		var keep []string
		for i := 0; i < len(order); i++ {
			if order[i] == " · [ ] chapters" || order[i] == " · [ ] turns" {
				keep = append(keep, order[i])
				order = append(order[:i], order[i+1:]...)
				i--
			}
		}
		// Past the level's own last key: under a note a chapter key put
		// there, the row keeps the key the note is about (#24). Elsewhere
		// the level's own rank stands.
		order = append(order, keep...)
	}
	return append(order, " · ? help")
}

// fitQuote is form with its quoted clause clipped so the whole fits room,
// or "" when that would leave too little of the quote to read.
func fitQuote(form string, room int) string {
	return fitQuoteMin(form, room, 12)
}

// fitQuoteMin is fitQuote with the floor named: a footer wants a dozen
// characters before it bothers, a board column three — there the choice is
// a stub of the bytes or no bytes at all.
func fitQuoteMin(form string, room, min int) string {
	i, j := strings.Index(form, `"`), strings.LastIndex(form, `"`)
	if i < 0 || j <= i {
		return ""
	}
	over := lipgloss.Width(form) - room
	q := form[i+1 : j]
	keep := lipgloss.Width(q) - over - 1 // the cells the quote may keep before its mark
	if keep < min {
		return "" // too little of the quote to read: it goes whole
	}
	cut := string([]rune(q)[:keep])
	if sp := strings.LastIndex(cut, " "); sp >= 8 {
		cut = cut[:sp] // at a word boundary: "please c…" answered nothing
	}
	return form[:i+1] + strings.TrimRight(cut, " ") + "…" + form[j:]
}

func noteForms(note string) []string {
	forms := []string{note}
	if strings.HasPrefix(note, glyphSaid) || strings.HasPrefix(note, glyphPrompt) {
		if i, j := strings.Index(note, " · "), strings.LastIndex(note, " · "); i > 0 && i != j && strings.Contains(note[i:j], `"`) {
			forms = append(forms, note[:i]+note[j:])
		}
	}
	rest := forms[len(forms)-1]
	for strings.Contains(rest, " · ") {
		rest = rest[:strings.LastIndex(rest, " · ")]
		forms = append(forms, rest)
	}
	return forms
}

// enterKeymap is what Enter promises, and — outside tmux, where compass hands
// its whole terminal over — how to come back (M6 contract). `prefix d` is the
// user's own detach key, because the terminal is genuinely theirs by then.
//
// The number keys are not in this line: the fleet column prints its own 1–9
// beside each session, and the parenthetical is what the footer owes an
// 80-column deck instead.
func (m *Model) enterKeymap() string {
	if s, ok := m.selected(); ok {
		if pane, has := m.panes[s.Info.Key()]; !has || pane.Target == "" {
			return "enter · no pane" // an attach that cannot work is not promised
		}
	}
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
	// The archive is a list at every width (decision #18): the flag
	// stays with the session view, so a board press that said "the
	// mirror shows beside a session" cannot surface two screens later
	// squeezing the archive's list.
	return m.showMirror && !m.archiveView && m.width >= deckWideCols && (m.level == levelTrail || (m.sessionView() && m.level == levelWaypoints))
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
