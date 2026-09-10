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
	"regexp"
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
	trailFor string                          // the session the payload belongs to; "" if none was polled
	agents   map[string]map[string]agentLive // what the subagents' own files say, per session, per call

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
	mgr        *fleet.Manager
	feeds      *feedStore
	runner     tmuxop.Runner
	replyBox   box      // where the reply panel will land, set in View before the body (#108)
	panelSteps bool     // the box steps a column right: its first placement covered the strip whole
	replyRows  []string // what that panel says, so a covered row can leave its sentence to it (#133)
	bodyRows   []string // the body as drawn, settled before the footer
	trailRows  []string // the trail column's drawn rows, settled before the fleet column
	proc       tmuxop.Proc
	narrator   Narrator

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
	// walkRow is the document row `n` and `N` stand on, plus one — nought
	// while no walk has been taken. The walk keeps its own place instead
	// of reading it back off the page: `clampScroll` pins the page at the
	// last screenful, so a walk that asks the page where it stands cannot
	// step between the matches that share that screenful, and can never
	// reach the wrap `walkTo` says it has.
	walkRow  int
	draft    string // the query being typed; searching is true while it is
	docVer   int    // bumped whenever a fold changes, to retire the cache
	docCache readerCache

	// The trail's own viewport, and only the trail's: how far the panel is
	// scrolled into the trail's document, and whether it is pinned to the
	// bottom. Pinned is the resting state (M7 contract): a growing journey
	// keeps its newest row on screen without anybody pressing a key.
	trailScroll int
	trailPinned bool

	// anchor is the reader's own cursor: the document line marked, and the
	// row Space acts on. It opens on the Lv2 cursor's row — so the two
	// panels say they are showing the same moment — and from there `j`/`k`
	// and `ctrl+d`/`ctrl+u` step it a line or a half page at a time
	// (readerCursorMove). -1 when there is no cursor to draw.
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
	restLevel   int    // the level the archive was opened from, for the way back (#53)
	fleetScroll int
	onBoardBand bool
	// drawnBand is the stranded band the board's last frame drew under
	// its strip, as drawn: the rows a digit on that frame can open (#47).
	drawnBand []recentRow

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
	hook        string
	hookRun     func(event, session, tmux, detail string)
	before      map[string]hookState
	pulse       bool // HEAD's breath is on its off-beat
	noLaneHeads bool // the board packed tighter than its lanes' heads (round 47)
	askBelow    bool // the card below this row draws its ask on its own ◉ row (#107)
	readonly    bool

	// The board's data: one trail per column, and each column's narrated
	// labels. The selected session's trail is here as well as in trail.
	trails map[string]journey.Trail
	// agents is what the subagents' own transcripts say, per session, per
	// Agent call: read by refresh for every open lane on the board, kept
	// with events for the lane the reader is on (#49).
	agents map[string]map[string]agentLive
	// readerLane is the Agent call whose conversation the reader shows in
	// place of the lead's: Tab on a lane opens it, leaving the reader
	// clears it.
	readerLane  string
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
	// noteYields marks a note the mark's own move outranks: `g` walked the
	// cursor and the page's word stood over it (#304). The footer spends
	// it, because only there can the trade be measured (#281, #308).
	noteYields bool

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
	lane  string // and the lane, when the reader is on an agent's conversation
	lanes string // and what the lanes' own files said, which moves the stubs
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
	if m.level >= levelWaypoints && m.cursor >= 0 && m.anchor < 0 && m.readerLane == "" && len(events) > 0 {
		// The conversation landed after the cursor was placed — a way
		// back from the archive, a digit — so the reader anchors now,
		// and its title carries the row and its clock (#55).
		m.anchorReader()
	}
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
	// A window that changed size is still looking at the row it was
	// looking at: the reader's page comes back to its cursor (#319).
	m.keepReaderCursorOnPage()
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
	laneWanted := m.laneWanted()
	agentPaths := map[string]string{} // session key → transcript path, for the lanes' files
	if s, ok := m.selected(); ok {
		agentPaths[s.Info.Key()] = s.Info.TranscriptPath
	}
	for _, t := range targets {
		agentPaths[t.key] = t.path
	}
	return func() tea.Msg {
		now := time.Now()
		sessions, err := mgr.Refresh(now)
		msg := fleetMsg{sessions: sessions, err: err, at: now}
		// The agents' own files, for every open lane of every trail polled:
		// paired by the call's id, never by its name.
		pollAgents := func(key string, tr journey.Trail) {
			files := pairAgents(agentDir(agentPaths[key]))
			if len(files) == 0 {
				return
			}
			for _, b := range tr.Branches {
				path, ok := files[b.ToolUseID]
				if !ok {
					continue // a returned lane's file is read too: its conversation is the finding's (#54)
				}
				if msg.agents == nil {
					msg.agents = map[string]map[string]agentLive{}
				}
				if msg.agents[key] == nil {
					msg.agents[key] = map[string]agentLive{}
				}
				msg.agents[key][b.ToolUseID] = feeds.pollAgent(key, b.ToolUseID, path, now, key == selected && b.ToolUseID == laneWanted)
			}
		}
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
				pollAgents(selected, msg.trail)
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
					pollAgents(t.key, tr)
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
		if msg.agents != nil || msg.hasTrail {
			m.agents = msg.agents // a poll that read the lanes' files replaces what was known
		}
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
	// The page keys are the half-page keys: a person reaching for PgDn on
	// a long trail should get the move the deck offers, not a dead key.
	switch key {
	case "pgdown":
		key = "ctrl+d"
	case "pgup":
		key = "ctrl+u"
	}

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

	m.note, m.noteYields = "", false // a keypress answers the last note

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
			m.query, m.walkRow = "", 0
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
		from := m.level
		m.toggleArchive()
		switch {
		case m.archiveView == was:
			// Nothing archived: the note says so, and the deck stays.
		case m.archiveView:
			// The archive is a list; it opens as one, whatever the depth,
			// and remembers the depth for the way back.
			m.restLevel = from
			if m.level != levelTrail {
				m.level = levelTrail
				m.cursor, m.anchor = -1, -1
			}
		case !m.archiveView && (!m.boardFits() || len(m.viewOrder()) == 1) && m.restLevel >= levelWaypoints:
			// Back where `A` was pressed: the legs with their cursor, or
			// the reader — a fleet of one has no board to land on (#47),
			// and a narrow deck's list beside one trail had lost its
			// cursor on the way back (#53).
			m.level = levelWaypoints
			m.cursor, m.anchor = -1, -1
			m.cursorMove(0)
			if m.restLevel >= levelReader {
				m.enterReader()
			}
		case !m.archiveView && m.boardFits() && len(m.viewOrder()) == 1:
			m.level = levelWaypoints
			m.cursor, m.anchor = -1, -1
			m.cursorMove(0)
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
			// Said once. The second clause spelled out what `A fleet`
			// says on the very same row, twenty cells to its left, and
			// the row is where a key is named (#95, #96, #299, #303):
			// forty-nine cells of note took five of the eleven keys the
			// row drew a press earlier off it at 120 — `tab deeper`, the
			// frame's only naming of the way deeper (#264, #296, #297),
			// and `j/k move` among them — and four of thirteen at 152.
			// The refusal keeps its own fact, the row keeps the door.
			m.note = "no mirror in the archive"
			return m, nil
		}
		if m.sessionView() && m.level >= levelReader {
			// In the reader the key is not a toggle. The mirror is a
			// panel of the session view and the reader is a level, not
			// a panel (#15), so the frame `m` is pressed on here draws
			// the conversation whichever way the flag stands: switched
			// on it goes to the pane and says `the live pane` below,
			// switched off it left the reader byte for byte the same
			// with an empty note and moved a panel the person meets one
			// `esc` later — the silent second press the width guard
			// above refuses in its own words (#62, #221, #241). Here
			// `m` means the live pane, on either press; the way back is
			// `m conversation`, the key the session view names.
			m.showMirror = true
		} else {
			m.showMirror = !m.showMirror
		}
		switch {
		case !m.showMirror && m.sessionView() && m.level == levelWaypoints:
			m.note = "the conversation" // short: the panel's own title says the rest, and the keys stay
		case !m.showMirror && m.level == levelBoard:
			// The board draws no mirror either way (#15), so the flag is
			// the whole of what the key changed — and the frame said
			// nothing: `m` here turned the mirror off, came back byte for
			// byte with the note gone, and left no way to know which way
			// the flag stands. That is the silent second press #241 and
			// #290 refused one and two levels in, on the one level where
			// the panel never answers for the key. It is the state
			// `mirror on` already says, said the other way (#37, #177):
			// ten cells, inside the note's own reserve, so the row gives
			// up nothing for it.
			m.note = "mirror off"
		case !m.showMirror:
		case m.sessionView() && m.level == levelWaypoints:
			m.note = "the live pane"
		case m.level == levelBoard:
			// The flag flipped: said as a state, not a refusal (#37), and
			// nothing the frame already says — `beside a session` is the
			// help's own `m` row and `(tab)` this footer's `tab session`;
			// the 34-cell form cost the 120 footer three keys (#177).
			m.note = "mirror on"
		case m.sessionView() && m.level >= levelReader:
			// The live pane has no keys: they go back to the trail.
			m.level = levelWaypoints
			m.anchorReader()
			m.note = "the live pane" // and the footer keeps `m conversation`: the way back is a key, not a clause (#62)
		case !m.sessionView() && m.level != levelTrail:
			m.note = "the mirror shows beside the trail (esc to zoom out)"
		}
		return m, m.capture()
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		i := int(key[0] - '1')
		was, wasArchive := m.selectedKey, m.archiveView
		if found := m.selectIndex(i) || m.openRecent(i+1); found {
			if m.selectedKey == was && m.archiveView == wasArchive {
				// The digit names the row the deck is already on: it
				// moved nothing and it is not refused, so it says why —
				// the device `h/l` uses at either end of the board and
				// `j`, `G` and the search walk use one level down (#24,
				// #221, #228, #231, #235). The header names the row at
				// the same cells on every frame, so the note leaves the
				// digit and the name to it (#233), and the noun with
				// them: at eighty `the session you are on` (22 cells)
				// took the archive's own footer down to `j/k move · a
				// ask · A fleet · ? help · q quit`, costing the frame
				// `tab deeper`, its only naming of the way deeper, for
				// an answer to a key that had moved nothing — the harm
				// #156, #159, #175, #187, #190, #194, #198, #201 and
				// #264 each folded, twice on this scene at this width.
				// Eighteen cells leave the way deeper standing.
				m.note = "the one you are on"
			}
		} else {
			m.note = fmt.Sprintf("no session %d", i+1)
			shown, drawn := m.selected()
			for _, s := range m.sessions {
				if s.Live && m.hidden[s.Info.Key()] && m.digits[s.Info.Key()] == i+1 && !m.archiveView {
					// The digit is a hidden session's: the refusal names
					// it and the way to it, as the strip beside it does (#57).
					m.note = fmt.Sprintf("%d %s is hidden · A, then x", i+1, sessionName(s.Info))
					if pane, ok := m.panes[s.Info.Key()]; ok && m.sharesTmux(s) {
						m.note += " · " + mirrorMark + " " + pane.Target // the hide note's own form (#62)
					}
				}
				if s.Live && !m.archiveView && !m.hidden[s.Info.Key()] && drawn && s.Info.Key() == shown.Info.Key() && m.digits[s.Info.Key()] == i+1 {
					// Under a fleet query that matches nothing the list
					// draws no row, so `selectIndex` finds none — but the
					// digit is the selected live session's own, the header
					// draws it as `1 hello` at the same cells, and the
					// trail beside it is that session's. That is #238's
					// question, and its answer: the deck already gives
					// this sentence at this stand wherever the query
					// happens to match the row, so the note is the deck's
					// own and repeats nothing the header says (#233).
					m.note = "the one you are on"
				}
				if s.Live && m.archiveView && drawn && s.Info.Key() == shown.Info.Key() && m.digits[s.Info.Key()] == i+1 {
					if m.archiveDrawsRow(s.Info.Key()) {
						// The archive draws this row under the archive's
						// own digit (#32), and that digit is not this
						// one: the frame draws `▸1 ● api` and its header
						// says `1 api`, so `2` is a number no row of the
						// frame wears. #245 gave it `the session you are
						// on` to keep #242's `2 api is live` from naming
						// a digit the frame does not — but the sentence
						// names one too, and it is then said of two
						// digits at once: `1` answers it on the same
						// frame, and on a two-card archive `2` and `3`
						// both do. #245's reason was that refusing
						// denies a session the frame has selected; the
						// deck already refuses that very digit on that
						// very frame from any other caret, so the
						// refusal is the answer for a number no row
						// wears, not a denial of the session. The way to
						// the fleet's numbering is `A fleet`, on the
						// footer already (#232). The deck's own sentence
						// stands.
						continue
					}
					// In the archive the digits are the archive's own
					// (#32), so a live session's digit finds no row here
					// — but "no session 1" denies a session this very
					// frame has selected, is drawing the trail of, and
					// offers `enter attach` for, and that the board one
					// `esc` away calls `1 hello`. The refusal names it
					// and says where it is, as the hidden twin one
					// branch above does (#57); the footer's `A fleet` is
					// the way, so the note does not buy the key twice.
					m.note = fmt.Sprintf("%d %s is live", i+1, sessionName(s.Info))
				}
			}
			if m.note == fmt.Sprintf("no session %d", i+1) && (!m.archiveView || len(m.viewOrder()) == 0) {
				// The view draws no row for this digit, but the fleet
				// still numbers a live session by it: `no session 1` is
				// the sentence for a digit no session ever had, and the
				// board one `esc` away calls this one `1 infra` while
				// the chips beside the note count it (`▲1 4m`). Where
				// the digit is the selected session's the deck already
				// says so (#238, #243) and in an archive drawing no row
				// it names it (#242); this is the same frame's other
				// digits. The refusal names it and says what it is, as
				// the hidden twin above does (#57); the way back is
				// already on the frame — `esc clears it` under a query,
				// `A fleet` in the archive — so the note does not buy a
				// key twice (#232). Where the archive draws rows the
				// digits are its own (#32) and the refusal stands.
				for _, s := range m.sessions {
					if s.Live && !m.hidden[s.Info.Key()] && m.digits[s.Info.Key()] == i+1 {
						m.note = fmt.Sprintf("%d %s is live", i+1, sessionName(s.Info))
						break
					}
				}
			}
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
			// The question is the cursor's own, not an index compare.
			// `TrailRows` counts rows the panel does not draw — a
			// waypoint the leg's own row already carries — and
			// `cursorMove` steps over them; where the trail's last row is
			// one of those the cursor's last stand is a row short of the
			// count, the index test read "not at the end", the move
			// stepped onto the undrawn row and back, and the key drew
			// nothing and said nothing. Ask the move whether it moved, as
			// `ctrl+d` below already does (#24, #213, #221).
			was := m.cursor
			m.cursorMove(1)
			if m.cursor == was {
				m.note = "at the present · k goes back"
				if len(TrailRows(m.trail, m.level)) <= 1 {
					m.note = "no leg to move to" // no key goes anywhere
				}
			}
			return m, nil
		case "k", "up":
			if m.cursor == 0 {
				m.note = "at the start"
				if len(TrailRows(m.trail, m.level)) <= 1 {
					m.note = "no leg to move to"
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
				if len(TrailRows(m.trail, m.level)) <= 1 {
					// A trail of one row: `k` goes nowhere either, and
					// pressed on this very frame it answers `no leg to
					// move to`. `j`, `k` and `ctrl+u` each ask this
					// question at this end of this trail; the page key
					// down was the one branch that did not, so it sent
					// the person to a key that moves nothing and, at
					// eighty, its fourteen extra cells cost the frame
					// `tab deeper`, its only naming of the way deeper
					// (#175, #187, #190, #194, #198, #201, #264, #283).
					m.note = "no leg to move to" // no key goes anywhere
				}
			}
			return m, nil
		case "ctrl+u":
			was := m.cursor
			m.cursorMove(-m.trailHalfPage())
			if m.cursor == was {
				m.note = "at the start"
				if len(TrailRows(m.trail, m.level)) <= 1 {
					m.note = "no leg to move to"
				}
			}
			return m, nil
		case "G":
			// G means the same thing at every depth: back to the present. At
			// Lv2 the cursor is what the viewport follows, so it is the cursor
			// that travels — and landing on the newest row re-pins the panel.
			// The same question as `j` above: the journey's end is the
			// last row the panel draws, not the last row the list counts.
			was, pinned := m.cursor, m.trailPinned
			m.cursorToPresent()
			if m.cursor == was && pinned {
				m.note = "at the present"
				if len(TrailRows(m.trail, m.level)) <= 1 {
					m.note = "no leg to move to"
				}
			}
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
		if !m.readerCursorMove(1) {
			m.note = "end of the conversation"
		}
	case "k", "up":
		if !m.readerCursorMove(-1) {
			m.note = "start of the conversation"
		}
	case "ctrl+d":
		if !m.readerCursorMove(m.readerHeight() / 2) {
			m.note = "end of the conversation"
		}
	case "ctrl+u":
		if !m.readerCursorMove(-(m.readerHeight() / 2)) {
			m.note = "start of the conversation"
		}
	case "[", "]":
		m.readerChapter(key)
	case "g":
		// The start of the conversation, the other end of `G` below.
		// The question is the page's own — did the page move — and the
		// sentence is the one `k` and `ctrl+u` already give for this end
		// (#24, #228). Before that, `g` was the reader's last silent
		// key: on 522 of the corpus's 608 reader stands it moved no line
		// and said nothing, and on 87 of those the frame came back byte
		// for byte the same, the dead key SPEC's round-one rule bans,
		// while `k` and `ctrl+u` on that very frame both answered.
		//
		// The key named for an end takes the mark there too (#313): `g` is
		// the start, so the cursor goes to the document's first row —
		// markOldestLine, the mirror of `G`'s markNewestLine below — reusing
		// the same walk `j`/`k` already do rather than a fresh one. Before
		// this the viewport moved and the mark did not: on a page that
		// fits, `G` said "end of the conversation" while the only `▸` stood
		// under "the start of the conversation" a screen above it.
		//
		// What it says is the move's, not the page's (#304). On a page
		// that fits `scrollBy` answers the page's question itself, and
		// with a mark to move that word stood over a press that had just
		// walked the cursor: `k` landing the mark on the same first row
		// of the same page says nothing, `g` said "all of it is on
		// screen" — the very sentence #304 took off `j`, `k`, `ctrl+d`
		// and `ctrl+u` for standing over a press that acted, and #309
		// left at this end only where the press moved nothing. The mark
		// says it by moving; the note yields to it in footerLine, where
		// the row can be measured under both (#281, #308). A page that
		// scrolls is untouched: there `g` says the end it reached, as
		// `G` does at the other one (#310).
		before := m.anchor
		moved := m.scrollBy(-(1 << 30)) // clamped to the first screenful
		m.markOldestLine()
		switch {
		case m.readerPageFits() && m.anchor != before:
			m.noteYields = true
		case !moved:
			m.note = "start of the conversation"
		}
	case "G":
		// Back to the present, which in the reader is the end of the
		// conversation. The question is the page's own — did the page
		// move — the device `ctrl+d` two cases up already uses, and the
		// sentence is the reader's own for that end (#24, #228): on a
		// page already showing the last screenful `G` moved no line and
		// said nothing, the dead key SPEC's round-one rule bans while
		// `j` and `ctrl+d` on the same page both answered.
		// On a page that fits, `scrollBy` answers the page's question
		// itself and reports the move made (lv3.go), so this end's own
		// word was unreachable there: `g` and `G`, the two keys named
		// for the two opposite ends, came back as one frame saying `all
		// of it is on screen`, the sentence #309 has just taken off `j`
		// and `k` at those same two ends. One fact keeps one name (#24,
		// #228, #307): the end says the end here too. The top keeps
		// #83's word, as #309 left it — the frame draws " the start of
		// the conversation" over it — and the width trade is #309's own,
		// measured in footerLine.
		//
		// The mark goes with it: markNewestLine, the same landing a lane's
		// reader opens on (#313).
		moved := m.scrollBy(1 << 30) // clamped to the last screenful
		m.markNewestLine()
		if !moved || m.readerPageFits() {
			m.note = "end of the conversation"
		}
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

// chapterStand is where the chapter keys stand: the trail's rows, the
// indices of its prompts, the map from row to document line, and `at` —
// the cursor at Lv2, the top of the viewport at Lv1. `chapter` moves from
// it and `chapterKeysMove` asks whether there is anywhere to move to, so
// the key the footer offers and the answer the press gives are one thing.
func (m *Model) chapterStand() ([]TrailRow, []int, map[int]int, int) {
	rows := TrailRows(m.trail, m.level)
	var prompts []int // indices into rows
	for i, r := range rows {
		if r.Kind == "prompt" {
			prompts = append(prompts, i)
		}
	}
	if len(prompts) == 0 {
		return rows, nil, nil, -1
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
	return rows, prompts, docRow, at
}

// chapterKeysMove reports whether `[` or `]` moves anything from where the
// trail stands: a prompt before it, or a prompt after it.
func (m *Model) chapterKeysMove() bool {
	_, prompts, _, at := m.chapterStand()
	for _, p := range prompts {
		if p != at {
			return true
		}
	}
	return false
}

// chapter is `[` / `]`: the previous or next prompt, treated as a chapter
// of the trail. A day-long journey is a dozen of your own prompts with the
// work between them, and "take me to where I said 'now the audit log'" is
// what getting back to an hour actually means. At Lv1 the viewport opens on
// the prompt; at Lv2 the cursor lands on it. The note says which chapter
// this is and when it began.
func (m *Model) chapter(key string) {
	rows, prompts, docRow, at := m.chapterStand()
	if len(prompts) == 0 {
		m.note = "no prompts in this trail"
		return
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
			// The help names G at every width, and the clause cost the
			// footer a key the `[` refusal beside it kept (#166, #162).
			m.note = "no later prompt"
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
			m.note = "no earlier prompt" // the chapter key's own question is prompts, not legs (#161)
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
		w, h := m.trailBox()
		doc, _ := trailDoc(m.trail, m.trailOpts(w, h))
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
		m.readerLane = "" // the lead's conversation comes back with the cursor
		// The reader goes back to following the cursor it left behind.
		m.anchorReader()
	case m.level > levelTrail:
		if m.boardFits() && !m.archiveView && m.liveCount() == 1 {
			m.note = "nothing to zoom out to"
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
	case m.level > levelBoard && m.boardFits() && !m.boardShown():
		// The board fits but the view has no session to put in a column,
		// so one level down the deck draws this very list again — the
		// same rows, the board-less deck's own keymap, and the trail
		// beside it — while the look on that trail was committed and the
		// fleet's title lost the word that says where the keys are (#20,
		// #63). There is no board to go out to, and the note says so in
		// the shape the width's refusal already has (#31); the row that
		// says why is on the frame already (#64).
		m.note = "no board"
	case m.level > levelBoard && m.boardFits():
		m.level = levelBoard
		m.commitLook(m.selectedKey)
	case m.level == levelTrail && m.liveCount() == 1 && !m.archiveView:
		m.note = "nothing to zoom out to" // no board at any width (#31)
	case m.level == levelTrail:
		m.note = fmt.Sprintf("no board under %d columns", deckWideCols)
	case m.level == levelBoard && !m.boardShown():
		m.note = "no board" // a list is drawn: the note is the frame's, not the level's
	case m.level == levelBoard:
		m.note = "the board is the top"
	}
}

// escClearsQuery answers whether Esc, pressed on this very frame, drops the
// standing fleet search — the promise the miss's second row makes. On the
// board or a list the key clears first, which is the branch `esc` takes
// above; deeper, Esc is one level out, and the query goes only where that
// step lands on the board and the selected session fails the search
// (zoomOut). On a narrow deck, in the archive or from the reader it does
// not, and no row may say it does.
func (m *Model) escClearsQuery() bool {
	if m.fleetQuery == "" {
		return false
	}
	if m.level <= levelTrail {
		return true
	}
	if m.level > levelWaypoints || m.archiveView || !m.boardFits() {
		return false
	}
	s, ok := m.selected()
	return ok && !m.matchesQuery(s)
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
// anyNeedsYou says whether a live session on the board is waiting on you
// and not dead on the API — the one `g` would grab.
func (m *Model) anyNeedsYou() bool {
	for _, s := range m.sessions {
		if m.onBoard(s) && s.Snap.State == state.NeedsYou && !s.Snap.APIError {
			return true
		}
	}
	return false
}

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
		if m.archiveView && m.level == levelBoard && !m.boardShown() {
			// That row was the last one the archive's board had: the
			// board is gone from under the keys and the deck draws the
			// list in its place, so the level is the list's too. Left at
			// the board's, `tab deeper` landed on the same rows one level
			// down and `⇧tab` called that list a board.
			m.level = levelTrail
		}
		// The note wears the number of the view it names, as the hide
		// note does (#256): the row leaves the archive the moment the
		// key acts, so the frame that follows draws it nowhere, and on
		// a fleet with two sessions called `api` every `api` left on
		// the frame — the header's and the one row the archive still
		// lists — is the other one, the one still hidden. The board is
		// where this one went and the board digit is the key that
		// reaches it there (#16), so the note says which `api` came
		// back. `m.digits` is kept for a session's life and hiding
		// never moved it (`assignDigits`), so the digit is the one the
		// hide note spent and the one the board draws on the next `A`.
		back := sessionName(s.Info)
		d := m.digits[key]
		if m.archiveView {
			// In the archive the numbers are the archive's own and they
			// are positional (#32): the row this session just left hands
			// its number straight to the next one, so the board digit
			// the note spends can be the number the frame draws for
			// another session — `1 porter is back on the board` under a
			// header reading `1 harness`, over a row drawn `▸1 ● harness`
			// that `1` opens. That is the hide note's own rule, one
			// keypress later: a digit is a key, and one digit must not
			// name two sessions (#245, #256). The note wears the number
			// the frame draws for this session where the archive still
			// draws it, keeps the board digit where the frame draws no
			// row wearing it — the empty archive #257 holds — and where
			// another row wears it spends no digit at all.
			if row, ok := m.boardRows()[key]; ok {
				d = row.num
			} else if m.numberDrawn(d) {
				d = 0
			}
		}
		if d > 0 {
			back = strconv.Itoa(d) + " " + back
		}
		m.note = back + " is back on the board"
		if d == 0 && m.sharesTmux(s) {
			// No number to tell two namesakes apart: the pane does, the
			// clause the hide note spends one keypress earlier (#53,
			// #62), and it is the first thing shed for the keys.
			if pane, ok := m.panes[key]; ok {
				m.note += " · " + mirrorMark + " " + pane.Target
			}
		}
		return
	}
	if refusal := m.hideRefusal(s); refusal != "" {
		m.note = refusal
		return
	}
	name := sessionName(s.Info)
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
	// pane when a namesake shares its tmux session. In the archive the
	// numbers are the archive's own (#32) and the frame goes on drawing
	// this row under the cursor: `▸1 ● harness`, under a header reading
	// `1 harness`, while the note said `2 harness is hidden` — a number
	// neither the row nor the header wears, the digit the frame does not
	// draw that #245 took out of the refusal one branch away and #248
	// kept the header true to. `boardRows` already numbers the hidden row
	// as drawn, for the same reason: a digit is a key, and one digit must
	// not name two sessions. The note wears the number the frame draws.
	if d := m.digits[key]; d > 0 {
		if m.archiveView {
			d = m.boardRows()[key].num
		}
		if d > 0 {
			name = strconv.Itoa(d) + " " + name
		}
	}
	m.note = name + " is hidden · A, then x" // the strip's own form: it fits eighty columns beside the keys
	if m.archiveView {
		// In the archive the route is the view the person is standing
		// in: `A, then x` sends them out and back for a key this very
		// frame's footer names, `x unhide` — the repetition #233 folded,
		// and #232's rule that a note yields to the key it names. At
		// eighty the eleven cells the clause spends are what `x unhide`
		// costs, so the note buys back the key it was pointing at.
		m.note = name + " is hidden"
	}
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

// numberDrawn says whether the frame that follows draws a row of its own
// wearing this number — the archive's positional numbers included (#32).
// A note that spends a number the frame draws for another session gives
// one digit two sessions (#245, #256).
func (m *Model) numberDrawn(num int) bool {
	if num <= 0 {
		return false
	}
	for _, row := range m.boardRows() {
		if row.num == num {
			return true
		}
	}
	return false
}

// hideRefusal is what `x` answers about this session instead of taking it
// off the board, or "" when the key acts. What owes you an alarm stays,
// and says so: a note that reported a hide while the column stood was the
// screen lying. It is one sentence for the key and for the footer that
// offers it (#24, #210).
func (m *Model) hideRefusal(s fleet.Session) string {
	name := sessionName(s.Info)
	switch {
	case !s.Live:
		// The subject is the row the caret is on, not the view: it is the
		// session that is off the board, and the archive is where it is
		// read. The long form spent thirty-six cells and the footer paid
		// for them — at eighty `tab deeper`, `a ask` and `/ search`, three
		// keys that act on this very row, and `/ search` again at 100 and
		// 120 — for a sentence about a key the footer does not offer
		// (#24, #52, #210: the note is one sentence for the key and for
		// the footer that offers it, and a refusal never costs a key that
		// acts). Its routes never pressed `tab`, so the archive's own
		// session view went on paying: at eighty `A tab x` took ` j/k
		// rows · [ ] chapters · tab deeper · esc back · A fleet · ? help
		// · q quit` down to ` j/k rows · esc back · A fleet · ? help · q
		// quit`, losing `tab deeper`, that frame's only naming of the
		// way deeper, for an answer to a key that moved nothing — while
		// `G` on the same frame draws fourteen cells and the key stands.
		// So the sentence yields the last of what the frame supplies and
		// answers the key's own question instead. In the archive `x`
		// brings a hidden row back (§3), and the archive's own header
		// says `hidden · x brings one back` of the rows it does bring
		// back (#291): this row is not one of them. Where the row is is
		// what the frame says three ways already — `▌FLEET · archive`,
		// the header's `archive 12` chip and the row's own `○` (#175,
		// #187, #190, #194, #198, #201, #264, #283, #287).
		return "it is not hidden"
	case m.liveCount() <= 1:
		// The rule is the fleet's, not the view's: `liveCount` counts
		// what is `onBoard`, the same number in the archive as on the
		// board. The `&& !m.archiveView` this clause carried was written
		// when the count was `len(m.viewOrder())`, which in the archive
		// is the archive's own list (round 15) — so the scope came off
		// the number and stayed on the clause, and `x` pressed in the
		// archive on the one live session took it off a board the frame
		// does not draw, leaving `nothing live` and `○ all quiet` beside
		// a trail still drawing `● scout thinking… for 40s`. The same
		// key on the same session one `A` away already says this.
		return "the live one stays"
	case s.Snap.APIError:
		return name + " stays · dead on the API"
	case s.Snap.State == state.NeedsYou:
		return name + " stays · it is asking"
	case s.Snap.State == state.Stuck:
		return name + " stays · it hangs"
	case m.isCircling(s):
		return name + " stays · it is looping"
	}
	return ""
}

// archiveDrawsRow says whether the archive view draws a row for this
// session — a hidden live session in its `hidden` group, or an archived
// one — as against an archive a search has cut to no rows (#242, #245).
func (m *Model) archiveDrawsRow(key string) bool {
	for _, i := range m.viewOrder() {
		if m.sessions[i].Info.Key() == key {
			return true
		}
	}
	return false
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
	if len(m.viewOrder()) == 0 {
		// A standing query this view answers with nothing: the column
		// draws `no session matches /q` and the header counts `0 of 4`,
		// so every fleet-of-one sentence (#152) counts a row the frame
		// does not draw. The move says what it could not do, in the
		// object the frame's own footer names — the list's row, the
		// board's column (`h/l columns`), the trail's `no leg to move
		// to` one level down. The band under a board miss (#163) is not
		// a cursor's (#51), so it is no row to move to either.
		if m.level == levelBoard && m.boardShown() {
			return "no column to move to"
		}
		return "no row to move to"
	}
	if len(m.viewOrder()) <= 1 {
		if m.archiveView {
			return "the only session"
		}
		// Live and archive are two words (#10, #146): the band two rows
		// above numbers twelve more, so the scoped word is the true one.
		return "the only live one"
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
		m.note = "reply needs a pane"
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
		m.note = "reply needs a pane"
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
		m.note = "attach needs a pane"
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

// sessionByKey finds a session on the deck by its key.
func (m *Model) sessionByKey(key string) (fleet.Session, bool) {
	for _, s := range m.sessions {
		if s.Info.Key() == key {
			return s, true
		}
	}
	return fleet.Session{}, false
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
	if m.archiveView && m.selectedKey == "" && m.restSelKey != "" && len(m.fleetOrder()) == 0 {
		// The archive answers nothing here — a standing search cut it to
		// no rows — so the swap had no row to land on and clampSelection
		// keeps what an empty view had, which on the first `A` is
		// nothing. `selectedIndex` then falls to nought for a key no
		// session wears, so the header and the trail's title named the
		// fleet's first row while the panel went on drawing the session
		// the person left, and `enter attach` offered that first row's
		// pane. The frame is still the live session's: keep it selected,
		// as the hidden branch above already keeps the hidden one.
		m.selectedKey = m.restSelKey
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
	// The lane belonged to the session that left: a digit or h/l at Lv3
	// opened the new session's reader on an agent it does not have, and
	// drew "reading the transcript…" under "the agent's own conversation"
	// for good. The reader comes back to the session it is on (#69).
	m.readerLane = ""
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
	if m.level >= levelWaypoints {
		// Switching sessions inside the trail opens on the present, the
		// way tab and h/l do: a digit landed with the keys in the trail
		// and no row under the cursor, and the first j placed it (#70).
		m.cursorMove(0)
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

// View renders the whole deck. The reply box floats over the deck, and the
// board's leftmost column begins at cell zero, so a box placed by that
// column stands on the strip whole — the archive's own line, its count and
// the key that browses it. The panel's footer is its own and `A` does not
// act while the panel is up (#62, #64), so no key can say it either, and
// the frame named neither. Where the frame it drew names the archive
// nowhere and the box is what covered it, the box steps one board column
// right and the deck is drawn again: the strip keeps its cells and the
// band beside it is composed at the width the box leaves (#126) — the
// placement the very same panel already has one column over.
func (m *Model) View() string {
	m.panelSteps = false
	out := m.viewOnce()
	if !m.replying || m.archiveView || m.showHelp || m.archivedCount() == 0 || frameNamesTheArchive(out) {
		return out
	}
	m.panelSteps = true
	if stepped := m.viewOnce(); frameNamesTheArchive(stepped) {
		return stepped
	}
	// A step that buys nothing is not taken: the frame stands as it was,
	// and the model's box and body rows are that frame's.
	m.panelSteps = false
	return m.viewOnce()
}

// frameNamesTheArchive asks rowNamesTheArchive's question of a drawn
// frame — the reply box's overlay included, so the door is read off the
// row the person sees and not off the row under the box. The box's own
// edge may stand on the same row, so the door is matched where it ends
// rather than by what the row ends with.
// The key half is optional: while a line has the keyboard the archive's
// line wears no key (#282's rule on this row, archiveDoorKey), and it is
// the count the box must not cover.
var doorOnARow = regexp.MustCompile(`\d archived(?: · [^·]*hidden)?(?: · A(?: browses)?)?(?:\s|$)`)

func frameNamesTheArchive(frame string) bool {
	for _, row := range strings.Split(ansi.Strip(frame), "\n") {
		if doorOnARow.MatchString(row + " ") {
			return true
		}
	}
	return false
}

func (m *Model) viewOnce() string {
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
	m.replyBox = box{}
	m.replyRows = nil
	// The band this frame draws is the frame's, like the reply box:
	// each drawer records it where it draws it and a digit reads it
	// there (#47, #255). Empty, not nil: a frame that draws no band has
	// drawn one of no rows, and nil is the model no frame has been drawn
	// from yet, which the deck never reads a key on (#221).
	m.drawnBand = []recentRow{}
	if m.replying {
		panel := m.replyPanel(inner)
		left, top, cap := m.panelPlace(inner, panelWidth(panel), len(panel), false)
		if len(panel) > cap {
			panel = m.replyPanelN(inner, cap)
			left, top, _ = m.panelPlace(inner, panelWidth(panel), len(panel), true)
		}
		m.replyBox = box{on: true, left: left, top: top, w: panelWidth(panel), h: len(panel)}
		m.replyRows = panel
	}
	switch {
	case m.showHelp:
		// A fleet of one at any width has no board (#31): the help that
		// taught "board → trail" beside a ⇧tab that refuses it was
		// keyed on the terminal's width, not on what the deck draws (#53).
		body = helpLinesWith(inner, bodyHeight, helpOpts{board: m.boardFits() && m.liveCount() > 1, reader: m.level >= levelReader, refused: m.refusedKeys(), keymap: m.keymapAt(inner), recent: m.archivedCount() > 0, tools: m.toolsAnywhere() > 1})
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
	m.bodyRows = body // the footer is composed against the rows the frame drew
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

// steppedLeft is the box's left after View has found that the frame it
// drew names the archive nowhere: one board column right, where the strip
// keeps its cells and the band beside it draws at a column's width.
func (m *Model) steppedLeft(left, inner, pw int) int {
	if !m.panelSteps {
		return left
	}
	n, cw := boardColumns(inner, m.drawnCount(m.viewOrder()))
	if n == 0 {
		return left
	}
	if step := min(bandWidth(inner, n, cw)+gutterWidth, max(inner-pw, 0)); step > left && step-1 >= fleetWidth {
		return step
	}
	return left
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
			// The mark stands at the cut, not where the text ran out:
			// twelve cells left of the panel it read as a clipped prompt (#62).
			before = pad(truncateWhole(line, left-1), left-1) + "…"
		}
		// The sliver this side of the box is its own column too (#139).
		// Where the box begins inside a column's own prefix — glyph and
		// class, the width `panelHides` already calls too narrow for the
		// row to say anything — what stands left of it is not a row but
		// a fragment of one cut at the border: `◉ "` inside an opening
		// quote, `◆ t…` for a leg with no label, and blanks under a mark,
		// which is the very thing #126 composes a column at the box's
		// width to stop. The fragment goes blank and the mark goes with
		// it. Where the box begins at a column's own edge the gap is the
		// gutter alone and the mark still stands (#62, #64).
		// The leftmost rail whose tail is a sliver is the column's own:
		// a rail standing inside the fragment is a card's continuation
		// and goes with it.
		for plain, i := ansi.Strip(ansi.Truncate(line, left, "")), 0; i < len(plain); {
			j := strings.Index(plain[i:], "│")
			if j < 0 {
				break
			}
			i += j + len("│")
			if gap := ansi.StringWidth(plain[i:]); gap > 1 && gap <= trailPrefixWidth+1 {
				before = ansi.Truncate(line, left-gap, "")
				break
			}
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
			// A peek that runs on into the next column whole kept the
			// sliver of the column the box covered — a lone "6m" for a
			// leg whose class and label are under the box. The sliver is
			// its own column: it goes blank unless it holds a space (#139).
			if plain := ansi.Strip(rest); strings.ContainsRune(plain, '│') {
				head := plain[:strings.IndexRune(plain, '│')]
				if t := strings.TrimSpace(head); t != "" && strings.IndexByte(t, ' ') < 0 {
					cut := ansi.StringWidth(head)
					rest = strings.Repeat(" ", cut) + ansi.TruncateLeft(rest, cut, "")
				}
			}
			if peek := strings.TrimSpace(ansi.Strip(rest)); peek != "" && strings.IndexByte(peek, ' ') >= 0 {
				// A mark for a peek with something in it: when the rule
				// above blanked the whole peek, the left mark already says
				// the row was cut (#64). And a lone word with no digit —
				// "ago", "still", "report" — answers nothing: #55 kept the
				// peek for the "2✗ 3m" and "for 33m" that read (#73).
				after = markCut(line, left, pw) + rest
			}
		}
		// No paint past the panel: a row that ended in the box's own
		// padding stood a cell into the terminal's margin (#63).
		rows[top+i] = strings.TrimRight(before+pad(p, pw)+after, " ")
	}
}

// markCut is the mark the overlay draws at the box's right edge: "…" where
// the box covered something of the column the peek belongs to, and a bare
// cell where it covered nothing. #64's rule — no right-hand mark when the
// peek rule blanked the whole peek — was measured on the whole rest of the
// row, so a board column the box covered while it was blank still wore the
// mark because an untouched column further right had words in it: at 220
// six rows of a reply frame said "something is hidden here" over a column
// that was empty. The question is what this column lost, so the answer is
// read from this column's own covered cells; a rail alone still counts
// (#75).
func markCut(line string, left, pw int) string {
	cut := ansi.TruncateLeft(ansi.Truncate(line, left+pw+1, ""), left, "")
	if plain := ansi.Strip(cut); strings.Contains(plain, "│") {
		cut = plain[strings.LastIndex(plain, "│")+len("│"):]
	}
	if strings.TrimSpace(ansi.Strip(cut)) == "" {
		return " "
	}
	return "…"
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
		if r, ok := m.boardRows()[s.Info.Key()]; ok && r.num > 0 {
			// The number the frame draws, which is the header's own
			// device (`headerName`): in the archive the numbers are the
			// archive's own (#32), and the card sat over a row drawn
			// `▸1 ● api` under a header reading `1 api` saying only
			// `reply to api`, on a fleet drawing two rows by that name.
			// The card prints the row's digit (#31) — the drawn one,
			// as the hide note takes it (#256) and the refusal (#245).
			who = strconv.Itoa(r.num) + " · "
		} else if d := m.digits[s.Info.Key()]; d > 0 && (!m.archiveView || (s.Live && len(m.viewOrder()) == 0)) {
			// The row's own digit, which is the session's for life — a
			// position named another session on the same screen. In the
			// archive the numbers are the archive's own (#32), but an
			// archive drawing no row claims no number, and the live
			// session such a frame still selects wears the digit its
			// header draws three rows above (#248) and its own refusal
			// calls it by (#242).
			who = strconv.Itoa(d) + " · "
		}
	}
	if pane, ok := m.selectedPane(); ok {
		target = pane.Target
	}
	toolWord := ""
	if s, ok := m.selected(); ok {
		// The one panel that types into another CLI says which (#53):
		// the tool's word, in the header's own form (#46).
		if tool := m.toolTag(s); tool != "" && strings.SplitN(tool, " · ", 2)[0] != shortModel(s.Info.Model) {
			toolWord = " · " + strings.SplitN(tool, " · ", 2)[0]
		}
	}
	head := func(tool string) string {
		title := " reply to " + who + name + tool
		if target != "" {
			title += " · " + mirrorMark + " " + target
		}
		return title + " "
	}
	title := head(toolWord)

	body := replyPanelMax
	if max := inner - 8; body > max {
		body = max
	}
	if fw, mw, _ := m.layout(inner); fw > 0 && !m.boardShown() && m.level < levelReader {
		// Beside a fleet list the panel stands over the trail and leaves
		// the list legible; at eighty columns it was standing on both.
		if max := inner - fw - gutterWidth - 6; body > max {
			body = max
		}
	} else if fw == 0 && mw > 0 && m.sessionView() {
		// In the session view the panel stands over the companion and
		// never over the trail beside it: at 120 a 68-cell box over a
		// 64-cell column took the rail, HEAD's clock and the band (#60).
		if max := mw - 8; body > max { // the box is the body and its frame, three cells off the rail
			body = max
		}
	}
	if body < 20 {
		body = 20
	}
	if lipgloss.Width(title) > body-2 && toolWord != "" {
		// The head sheds the tool before the pane, §4's own order: the
		// pane is what the write goes to, and "⌁ dev:2…" is a legal
		// target that is not this one (#54). The header two rows up
		// still says the tool.
		title = head("")
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
			tail = headTail(m.trail, m.now, true, m.agentsFor(m.selectedKey))
		} else if tr, ok := m.trails[s.Info.Key()]; ok {
			tail = headTail(tr, m.now, true, m.agentsFor(s.Info.Key()))
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
			left := m.steppedLeft(min(x, max(inner-pw, 0)), inner, pw)
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
	right := m.statusChips()
	// The selected session's identity rides the header — the one row at
	// the same cells at every level and width — so a zoom never moves the
	// name and a digit press always shows its landing (#46). It sheds
	// before the chips, which never do: the board word first, then the
	// tag, then the search, and last the name clips around its digit.
	board := ""
	if m.level == levelBoard && m.boardShown() {
		board = " · board"
	}
	digit, name, tag := m.headerName()
	tool := ""
	if s, ok := m.selected(); ok {
		tool = m.toolTag(s) // "opencode · sonnet-4-5" where the fleet runs two tools (#50)
	}
	query := ""
	if m.fleetQuery != "" {
		if m.archiveView {
			// In the archive the chip beside it counts the search
			// already (`archive 1 of 41`): the clause says which search,
			// and a line answers a question once (#64).
			query = " · /" + m.fleetQuery
		} else {
			// The search in force, and how much of the fleet answers it —
			// the fleet this list can draw. A hidden session is not a row
			// the numerator can ever reach, and the door beside it counts
			// the hidden half itself (`1 of 1 hidden`, #176), so counting
			// it here said `3 of 4` where four of four answered (#178).
			total := 0
			for _, s := range m.sessions {
				if m.onBoard(s) {
					total++
				}
			}
			query = fmt.Sprintf(" · /%s · %d of %d", m.fleetQuery, len(m.viewOrder()), total)
		}
	}
	room := w - lipgloss.Width(right) - 2 // two cells of air before the chips: their own separator's width, so `· claude ●1` never reads as one clause (#121)
	compose := func(board, tag, query string, name string) string {
		// The identity first, in the same cells at every level: the board
		// word after it, so a Tab out of the board moves nothing.
		left := titleStyle.Render("⌂ compass")
		if name != "" {
			who := name
			if digit != "" {
				who = digit + " " + name
			}
			left += dimStyle.Render(" · ") + who
			if tool != "" {
				left += dimStyle.Render(" · " + tool)
			}
			if tag != "" {
				left += dimStyle.Render(" · " + tag)
			}
		}
		return left + dimStyle.Render(board) + dimStyle.Render(query)
	}
	left := compose(board, tag, query, name)
	// The model goes before the tool word (#52): "1 api · opencode ·
	// ⌁ dev:2.0" is the rung between the full tag and none, and the
	// header skipped it and threw the tool away with a cell to spare (#62).
	word := ""
	if s, ok := m.selected(); ok && tool != "" {
		if w := strings.SplitN(tool, " · ", 2)[0]; w != tool && w != shortModel(s.Info.Model) {
			word = w
		}
	}
	for _, try := range []func() string{
		func() string { board = ""; return compose(board, tag, query, name) },
		func() string {
			if word != "" {
				tool = word
			}
			return compose(board, tag, query, name)
		},
		func() string { tool = ""; return compose(board, tag, query, name) },
		func() string { tag = ""; return compose(board, tag, query, name) },
		func() string { query = ""; return compose(board, tag, query, name) },
	} {
		if lipgloss.Width(left) <= room {
			break
		}
		left = try()
	}
	if over := lipgloss.Width(left) - room; over > 0 && name != "" {
		// The name clips around its digit, and goes whole before it
		// would be a bare mark.
		keep := lipgloss.Width(name) - over
		if keep >= 3 {
			left = compose(board, tag, query, clip(name, keep))
		} else {
			left = compose(board, tag, query, "")
		}
	}
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// headerName is the selected session as the header names it: the digit its
// row wears for life (the archive's drawn number in the archive, #32), its
// name, and its ⌁ tag only when another session shares the name — the reply
// panel's own form (#31), so the two read as one fact. Nothing when nothing
// is selected.
func (m *Model) headerName() (digit, name, tag string) {
	s, ok := m.selected()
	if !ok {
		return "", "", ""
	}
	name = sessionName(s.Info)
	if m.archiveView && !s.Live && s.Info.Name == "" {
		// The archive's rows are titled by what they asked for (#56,
		// #59) — where the session has no name of its own. A session its
		// person named is told apart by that name (#79, #234), and the
		// ask the header used to hang beside it is the trail's own `◉`
		// row, three rows below it on every frame that draws one: the
		// second whole copy #265 took off the archive board's card head
		// and #269 off the ship row, on the one row that is drawn at
		// every level. At eighty it was worse than a copy — the header
		// clipped it, `"the checkout suite flake…`, and paid for the
		// fragment with `· claude`, the word an archive holding two
		// tools says (#79, #80), which every other archive header at
		// that width keeps. #86's reason for the ask on a head is "where
		// nothing else says it"; here the trail says it whole.
		name = archiveHeadline(s)
	}
	if r, ok := m.boardRows()[s.Info.Key()]; ok && r.num > 0 {
		digit = strconv.Itoa(r.num)
	} else if d := m.digits[s.Info.Key()]; d > 0 && (!m.archiveView || (s.Live && len(m.viewOrder()) == 0)) {
		// Off the view under a search: the digit is still its own. In the
		// archive the numbers are the archive's own (#32) — but an archive
		// drawing no row claims no number, and the live session the frame
		// is still selecting, drawing the trail of and offering `enter
		// attach` for (#244) wears the digit it took for life (#30); its
		// own refusal on that frame already calls it `1 hello is live`
		// (#242), a digit the header must not be the one to drop.
		digit = strconv.Itoa(d)
	}
	for _, o := range m.sessions {
		if o.Info.Key() != s.Info.Key() && o.Live == s.Live && sessionName(o.Info) == name {
			tag = m.boardTag(s)
			break
		}
	}
	return digit, name, tag
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
		chip := fmt.Sprintf("◈%d out · oldest %s", out, m.age(oldestOut))
		// A lane whose own file has gone quiet displaces "oldest", which
		// is the wrong alarm: a lane out 20m that wrote 5s ago is fine.
		silent, longest := 0, time.Duration(0)
		for _, s := range m.sessions {
			if !m.onBoard(s) || s.Snap.State == state.Idle {
				continue
			}
			n, d, _, _ := lanesLive(m.agentsFor(s.Info.Key()), openLanes(m.trails[s.Info.Key()]), m.now)
			silent += n
			longest = max(longest, d)
		}
		if silent > 0 {
			chip = fmt.Sprintf("◈%d out · %d silent %s", out, silent, state.ShortDuration(longest))
		}
		parts = append(parts, dimStyle.Render(chip))
	}
	if m.archiveView {
		// The list holds both, and the chip says so — so under a search
		// it says what the search left, as the door that opened this view
		// does (#169, #176): `archive 41 · 1 hidden` stood over one drawn
		// row, no hidden group and no `x unhide`.
		chip := fmt.Sprintf("archive %s", m.archiveDoorCount(m.archivedCount()))
		if n := m.hiddenCount(); n > 0 {
			chip += fmt.Sprintf(" · %s hidden", m.hiddenDoorCount(n))
		}
		parts = append(parts, dimStyle.Render(chip))
	} else if n := m.hiddenCount(); n > 0 && m.level >= levelTrail && m.sessionView() && !m.showHelp {
		if fw, _, _ := m.layout(m.width); fw == 0 {
			// The hidden count and its door are the fleet's last line and
			// the board's strip; in the session view above the board's
			// width neither is drawn, and a fleet of four counted three
			// on the chips with no clause saying where the fourth went,
			// while the same keypresses at eighty drew `1 hidden · A,
			// then x`. The question is what the frame drew (#199): where
			// no fleet body stands, the chip carries the clause (#202).
			parts = append(parts, dimStyle.Render(m.hiddenClause(n)))
		}
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
//
// The hide key is the last key the session view and the reader take onto
// the row and the first they give up: `x` acts there, so the footer names
// it, and it is not the reason a person is that deep, so it is taken only
// where the finished row still names every key the row without it named
// (#39's ranks, measured as #281 measures them — on the finished row,
// because the note's own reserve is what the keys are shed against).
func (m *Model) footerLine(w int) string {
	keys := m.keymap()
	// A note the mark's own move outranks yields to it (#304): `g` on a
	// page that fits walked the cursor and kept `scrollBy`'s word for the
	// page over the press that acted. It yields only where the row without
	// it still names every key it named with it — a word costs no key
	// (#281, #308) — and the row keeps the word rather than a key.
	if m.level >= levelReader && m.noteYields && m.note != "" {
		with := m.footerTraded(keys, w)
		said := m.note
		m.note = ""
		without := m.footerTraded(keys, w)
		m.note = said
		if footerNamesAll(with, without) {
			return without
		}
		return with
	}
	// The end's own word is one cell longer than #83's, which the same key
	// draws at the other end of the same page, and a word costs no key
	// (#308): where the finished row under it names fewer keys than under
	// #83's, the row keeps #83's word, as it already does under `g` and `G`
	// on the same page. The trade is measured before every other, because
	// the cell this one wants is the cell they are all spent against
	// (#281, #284, #306).
	if m.level >= levelReader && m.note == "end of the conversation" && m.readerPageFits() {
		with := m.footerTraded(keys, w)
		m.note = "all of it is on screen"
		without := m.footerTraded(keys, w)
		m.note = "end of the conversation"
		if !footerNamesAll(without, with) {
			return without
		}
		return with
	}
	// The clause the reply refusal already says goes before any trade is
	// measured. #165 gave both no-pane refusals their naming form
	// "precisely so that naming the key would buy a key back", and the
	// harm it folded was the refusal saying `no pane` a few cells from the
	// footer's own `enter · no pane` on the same row — which the reply
	// refusal still did, on the paneless session at eighty, where it cost
	// `/ search` and `x hide`, two keys that act on that row. Beside
	// either no-pane note the clause is the same fact twice — this session
	// has no pane, which is why neither key can work (#95, #96) — and the
	// attach refusal's own half of it is #232's and #299's. The clause is
	// a stuck key besides: the attach it names cannot work either, so it
	// yields where a key that acts comes back (#210, #216). Like every
	// clause this row trades, it goes only where the row without it still
	// names every key the row with it named (#281, #284, #289).
	if clause := m.replyRefusalSaid(keys); clause != "" {
		bare := strings.Replace(keys, clause, "", 1)
		with, without := m.footerTraded(keys, w), m.footerTraded(bare, w)
		if footerNamesAll(noPaneClauseGone(ansi.Strip(with)), without) {
			return without
		}
		return with
	}
	return m.footerTraded(keys, w)
}

// noPaneClauseGone is a drawn row with the keymap's own `enter · no pane`
// taken out, in whichever form the row drew it, so the row under the
// refusal can be read against the row beside it key for key.
func noPaneClauseGone(row string) string {
	for _, f := range []string{" · enter · no pane", "enter · no pane · ", "enter · no pane"} {
		if strings.Contains(row, f) {
			return strings.Replace(row, f, "", 1)
		}
	}
	return row
}

// replyRefusalSaid is the keymap's own `enter · no pane` under the reply
// refusal — the note that already says it — or "" anywhere else. The attach
// refusal's clause is taken one layer in (attachRefusalSaid, #299). Every
// other stuck key stays under its own note, because its note does not say
// what the clause says: `infra stays · it is asking` never mentions the
// pane, so the row refusing `x` must (#24, #57).
func (m *Model) replyRefusalSaid(whole string) string {
	if m.note != "reply needs a pane" {
		return ""
	}
	for _, k := range []string{" · enter · no pane", "enter · no pane · "} {
		if strings.Contains(whole, k) {
			return k
		}
	}
	return ""
}

// footerTraded draws the row for this keymap with the trades the cursor,
// mirror, grab and search clauses each pay for their place on it.
func (m *Model) footerTraded(keys string, w int) string {
	// The reader's fitting page names the key that walks its cursor
	// (#300) on the same terms as every clause new to a row since #281:
	// only where the finished row still names every key it named without
	// it. The trade is measured first, before the mirror's, because the
	// eleven cells this clause wants are the cells that trade is spent
	// against. Where the width is not there the row comes back exactly as
	// it stood, `space unfold` at its head.
	if clause := "j/k rows · "; m.level >= levelReader && m.readerPageFits() && strings.HasPrefix(keys, clause) {
		bare := strings.Replace(keys, clause, "", 1)
		if !footerNamesAll(m.footerMirrorTraded(bare, w), m.footerMirrorTraded(keys, w)) {
			keys = bare
		}
	}
	if m.level >= levelReader && !m.readerPageFits() && strings.HasPrefix(keys, "j/k rows · ") {
		// The word is two cells shorter than the one it replaced (#307),
		// and on four rows the two cells moved the note's reserve so the
		// shed took `x hide` or `m live pane` off a row that had named
		// them. A word costs no key: where the finished row under the
		// short word names fewer keys than under the long one, the row is
		// drawn to the long word's width and the note ends two cells in.
		was := m.footerMirrorTraded(strings.Replace(keys, "j/k rows · ", "j/k scroll · ", 1), w)
		now := m.footerMirrorTraded(keys, w)
		if !footerNamesAll(strings.Replace(was, "j/k scroll · ", "j/k rows · ", 1), now) {
			return m.footerMirrorTraded(keys, w-2)
		}
		return now
	}
	return m.footerMirrorTraded(keys, w)
}

// footerMirrorTraded draws the row with the mirror, grab and search
// clauses' own trades measured on it.
func (m *Model) footerMirrorTraded(keys string, w int) string {
	// The reader's mirror key is new to its row for the same reason as
	// the hide key and the search key one level out, and pays the same
	// price: it is taken only where the finished row still names every
	// key it named without it (#281, #284, #289). At 220 the reader's
	// row stands 145 cells of 220; at 152 and 120 the fourteen cells the
	// clause needs come out of `n/N`, `[ ] turns` and `x hide`, so the
	// row keeps its keys and the clause waits for the width.
	if clause := " · m live pane"; m.level >= levelReader && strings.Contains(keys, clause) {
		if bare := strings.Replace(keys, clause, "", 1); !footerNamesAll(m.footerGuarded(bare, w), m.footerGuarded(keys, w)) {
			keys = bare
		}
	}
	// The grab key is new to the session view's row for the same reason
	// as the hide key, the search key and the reader's mirror key, and
	// pays the same price: it is taken only where the finished row still
	// names every key it named without it (#281, #284, #289, #293).
	// Its trade is measured on the row the search trade below finishes,
	// because that trade is what the cells are spent against: measured on
	// the half-traded row the clause bought itself in at 152 by taking
	// `ctrl+d/u half page` and handing the search key back in its place.
	// The archive's three levels take the clause on the same terms
	// (#295's own measure, one view over): the trade is the row's, not
	// the level's, so it is measured wherever the clause is new to the
	// row — the archive's board and list included.
	if clause := " · g grab"; m.level < levelReader && (m.level >= levelWaypoints || m.archiveView) && strings.Contains(keys, clause) {
		bare := strings.Replace(keys, clause, "", 1)
		if !footerNamesAll(m.footerSearchTraded(bare, w), m.footerSearchTraded(keys, w)) {
			keys = bare
		}
	}
	return m.footerSearchTraded(keys, w)
}

// footerSearchTraded draws the row for this keymap with the search
// clause's own trade measured on it (#289).
func (m *Model) footerSearchTraded(keys string, w int) string {
	row := m.footerGuarded(keys, w)
	// The search key is new to the session view's row for the same reason
	// as the hide key and pays the same price: it is taken only where the
	// finished row still names every key it named without it (#281, #284).
	// The reader named it before either of them and is not measured here.
	if clause := " · / search"; m.level >= levelWaypoints && m.level < levelReader && strings.Contains(keys, clause) {
		if bare := m.footerGuarded(strings.Replace(keys, clause, "", 1), w); !footerNamesAll(bare, row) {
			return bare
		}
	}
	return row
}

// footerGuarded draws the row for this keymap with the hide clause's own
// trade measured on it (#284).
func (m *Model) footerGuarded(keys string, w int) string {
	row := m.footerWith(keys, w)
	guarded := []string{" · x hide", " · x unhide"}
	if m.level < levelWaypoints {
		// Below the session view the two views' own rows have named
		// their hide key since #24; the one clause measured here is the
		// one the archive's live row takes, which is new to the row for
		// the same reason and pays the same price (#284).
		if !m.archiveView {
			return row
		}
		guarded = []string{" · x hide"}
	}
	for _, clause := range guarded {
		if !strings.Contains(keys, clause) {
			continue
		}
		bare := m.footerWith(strings.Replace(keys, clause, "", 1), w)
		if !footerNamesAll(bare, row) {
			return bare // the key cost the row another key
		}
	}
	return row
}

// footerNamesAll says whether the finished row `now` names every key
// `was` named. The note is not a key and is read past: it stands after
// the gap the keymap never contains (#134's reserve), and the attach
// aside is not a key either (#55).
func footerNamesAll(was, now string) bool {
	read := func(s string) []string {
		s = strings.ReplaceAll(ansi.Strip(s), attachHint, "")
		if i := strings.Index(s, "  "); i >= 0 {
			s = s[:i]
		}
		var out []string
		for _, frag := range strings.Split(s, " · ") {
			if f := strings.TrimSpace(frag); f != "" {
				out = append(out, f)
			}
		}
		return out
	}
	has := map[string]bool{}
	for _, k := range read(now) {
		has[k] = true
	}
	for _, k := range read(was) {
		if !has[k] {
			return false
		}
	}
	return true
}

// keymap is the whole keymap for where the keys are now, before any of it
// is shed for width: the row's promise, and what the help asks when it has
// to choose which key rows a short body keeps.
func (m *Model) keymap() string {
	keys := "j/k move · " + m.enterKeymap() + " · tab deeper · [ ] chapters · r reply · a ask · / search · x hide · g grab · ? help · q quit"
	if m.archiveView {
		// In the archive `A` is the way home, so the keymap says that
		// where the live view says `⇧tab board`; `g` is named below,
		// where the frame after it is drawn (#295 and the block under
		// this switch — it has something to grab here). In the live view
		// the archive announces itself
		// on the fleet's own last row: "N archived · A browses". The chapter
		// keys act here as they do on the live list, and answered
		// `no earlier prompt` on a row that did not name them (#193).
		// `r reply` stands here too: the archive draws and selects live
		// rows — the hidden one under `hidden · x brings one back` (#29),
		// and the live session an archive with nothing in it keeps (#244,
		// #248) — and for those the pane is real. Which of the two writes
		// a row is offered is the pane's question, not the view's (#53):
		// the test below takes `r reply` off any row that says `no pane`,
		// so an archived row loses it there and no row is offered one
		// write and not the other.
		keys = "j/k move · " + m.enterKeymap() + " · tab deeper · [ ] chapters · r reply · a ask · / search · x unhide · A fleet · ? help · q quit"
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
			// The archive's board goes one level to the archive's list,
			// not to the session view: `zoomIn` stops at `levelTrail`
			// wherever the archive is open (#18's three levels are the
			// live board's), so the live board's `tab session` — true
			// there, where the key lands on the panel chipped
			// `[session]` — named a level this key does not reach, and
			// landed on the one chipped `[fleet]`, whose own footer then
			// names `tab deeper` for the step that is left. The word is
			// the archive's own at every other level (#40, #246).
			// `a` acts here on the very row the caret is on — the
			// historian for the selected session, `case "a"` at every
			// level — and the archive is where it is the reason to be
			// (#264, and the shed's own comment below). The archive's
			// list one `tab deeper` away names it, so does the live
			// board this branch was copied from before `a ask` existed,
			// and the board's own footer stood 82 cells wide in 120 with
			// the key nowhere on it: a key that acts and is never named
			// is the one thing a footer is for (#24, #175, #187).
			keys = "h/l columns · " + m.enterKeymap() + " · tab deeper · r reply · a ask · / search · x unhide · A fleet · ? help · q quit"
		}
	case m.level == levelTrail && m.boardShown():
		keys = "j/k move · ctrl+d/u half page · " + m.enterKeymap() + " · [ ] chapters · r reply · a ask · / search · ⇧tab board · g grab · ? help · q quit"
		if m.archiveView {
			keys = "j/k move · ctrl+d/u half page · " + m.enterKeymap() + " · tab deeper · [ ] chapters · r reply · a ask · / search · x unhide · ⇧tab board · A fleet · ? help · q quit" // the chapter keys act here too (#193)
		}
	case m.level >= levelReader && m.sessionView():
		// `m` is the deck's key, not a level's, and here it always acts:
		// pressed in the reader it takes the deck to the session view
		// with the live pane standing and says `the live pane` (#290),
		// whichever way the flag stands — and no key on the row said so,
		// on a row that stood 145 of 220 cells. A key that acts and is
		// never named is the one thing a footer is for (#24, #175, #187,
		// #277, #284, #289). The label is one-sided because the key is:
		// in the reader there is no `m conversation` to offer, the way
		// back being the key the session view names. `shedOrder` has
		// ranked the clause among the shared keys all along, so it sheds
		// at the rank it already has (#39, #281).
		keys = "j/k rows · ctrl+d/u half page · space unfold · / search · n/N · [ ] turns · h/l session · m live pane · r reply · a ask · " + m.hideKeymap() + " · " + m.enterKeymap() + " · esc back · ? help · q quit"
	case m.level >= levelReader:
		keys = "j/k rows · ctrl+d/u half page · space unfold · / search · n/N · [ ] turns · r reply · a ask · " + m.hideKeymap() + " · " + m.enterKeymap() + " · esc back · ? help · q quit"
	case m.level >= levelWaypoints && m.sessionView():
		// `/` opens the fleet search here as it does on the board, on a
		// list and in the reader: pressed at this level it takes the
		// header's `/query · n of m`, narrows the fleet beside the
		// trail and swaps the row for `/▏ · enter keeps it · esc
		// cancels` — and no key on the row said so, on the very frame a
		// fleet of one opens at. Every neighbouring level names it, and
		// `shedOrder` has ranked `· / search` among this level's own
		// keys all along with nothing on the row to match: a key that
		// acts and is never named is the one thing a footer is for
		// (#24, #175, #187, #277, #284). It stands where the reader
		// stands it, before the hide key, and sheds at the rank it
		// already has (#39, #281).
		// `g` is the fleet's key and it acts here: pressed in the session
		// view it takes the oldest session waiting on you, moves the
		// header, the trail and the reader onto it and attaches — the
		// deck comes back standing on another session's row, on a
		// two-tool fleet the other tool's — and no key on the row said
		// so, on a row that stood 172 of 220 cells with forty-eight
		// blank. A key that acts and is never named is the one thing a
		// footer is for (#24, #175, #187, #277, #284, #289), and the
		// help in the reader sends the person here for it ("the grab is
		// a level out", #246). `shedOrder` has ranked `· g grab` first
		// among this level's own keys all along with nothing on the row
		// to match, so it sheds at the rank it already has (#39, #281),
		// and #78's and #36's gates below still take it off a fleet with
		// nothing amber and off a fleet of one. It stands after the hide
		// key, where the board and the list already stand it (#52).
		// The pair is `j/k rows`, not `j/k legs`: the cursor steps
		// `TrailRows`, whose kinds are `prompt`, `leg`, `waypoint` and
		// `branch`, and whose prompt row carries `Leg: -1` — "a boundary
		// rather than a span of work" in the trail's own words, `◉` and
		// not a leg in SPEC §2.1's. The fleet-and-trail layout of this
		// very level has said `rows` all along, so one key, one act, one
		// level now wears one name (#220, #307). The two words are the
		// same width: no row's keys move.
		keys = "j/k rows · ctrl+d/u half page · h/l session · [ ] chapters · m live pane · r reply · a ask · / search · " + m.hideKeymap() + " · g grab · tab reader · " + m.enterKeymap() + " · esc board · ? help · q quit"
	case m.level >= levelWaypoints:
		keys = "j/k rows · ctrl+d/u half page · [ ] chapters · r reply · " + m.enterKeymap() + " · tab deeper · a ask · / search · " + m.hideKeymap() + " · esc back · ? help · q quit"
	}
	if m.archiveView && m.level < levelReader && !m.showHelp && !m.searching && !m.replying && !strings.Contains(keys, " · g grab") {
		// `g` is the fleet's key and it acts in the archive too: pressed
		// on any of the archive's three levels it finds the session
		// waiting on you, leaves the archive for the live fleet standing
		// on that session's row and attaches — on the two-tool scene from
		// the hidden `2 api · opencode` to `1 infra · claude · sonnet-4-5`,
		// the other tool on the other model — and no key on the row said
		// so, on a row that stood 152 of 220 cells with sixty-eight blank
		// and the attach aside still on it. The keymap above said `g` "has
		// nothing to grab" here, which the frame refutes; `A fleet`, the
		// other half of that reason, is a different key with a different
		// landing — it comes home on the row you left, where `g` comes
		// home on another session's and attaches. A key that acts and is
		// never named is the one thing a footer is for (#24, #175, #187,
		// #277, #284, #289, #295). It stands with the level's own keys,
		// before the way home (#56), and sheds at the rank `shedOrder`
		// already gives it (#39, #281); #78's gate below still takes it
		// off a fleet with nothing amber.
		if strings.Contains(keys, " · A fleet") {
			keys = strings.Replace(keys, " · A fleet", " · g grab · A fleet", 1)
		} else {
			keys = strings.Replace(keys, " · ? help", " · g grab · ? help", 1)
		}
	}
	if m.level == levelTrail && !m.showHelp && !m.searching && !m.replying {
		// At Lv1 the page keys drive the trail beside the list (§3); on a
		// frame whose trail fits, they do nothing, and a footer naming
		// them beside "j/k move" read as though they paged the list (#83).
		if total, h, _ := m.trailView(); total <= h {
			keys = strings.Replace(keys, "ctrl+d/u half page · ", "", 1)
		}
	}
	if m.level == levelWaypoints && !m.showHelp && !m.searching && !m.replying {
		// #220's rule at what the keys actually do here. At Lv1 and in the
		// reader `ctrl+d/u` move the viewport, so a page that fits leaves
		// them nothing (#83, #200); at Lv2 they move the *cursor* half a
		// screenful of rows — "the cursor is what the viewport follows
		// here, so the cursor is what moves" (§3) — and on a trail the
		// panel draws whole they still walk the `▸` down the rail. So the
		// question is the cursor's, not the viewport's: where the trail
		// has one row the cursor has nowhere to go and both keys refuse
		// (`no leg to move to`), which is the movement key's own test
		// (#213, #219); anywhere else the key acts and stays (#215, #218:
		// the frame is what a person sees).
		if len(TrailRows(m.trail, m.level)) <= 1 {
			keys = strings.Replace(keys, "ctrl+d/u half page · ", "", 1)
			keys = strings.Replace(keys, " · ctrl+d/u half page", "", 1)
		}
	}
	if m.level >= levelReader && !m.showHelp && !m.searching && !m.replying {
		// #83 one level down, at what the keys do here now. #83 shed both
		// keys from a page all on screen because "the keys that move the
		// viewport move nothing" and the app says so itself the moment
		// they are pressed — true while the reader had no cursor. #300
		// gave it one and drew it: on a page that fits `j` and `k` walk
		// the `▸` a row at a time and `space` unfolds the row it stands
		// on, so a second fold on that one screen is reachable by these
		// keys and by nothing else, and the row went on naming `space
		// unfold` and naming nothing that reaches it. A key that acts and
		// is never named is the one thing a footer is for (#24, #175,
		// #187). #220 settled the words one level out, where the same
		// keys move the cursor and not the viewport: the movement key is
		// `j/k rows`, and the reader's keys are the same keys: they walk
		// the mark a row at a time and move the viewport only when the
		// mark would leave it, so `rows` is what they move on every page
		// and `scroll` was the name of a job they do only sometimes and
		// often not at all (#308). The page key stays shed where the page
		// fits: it is a shortcut for a distance `j` covers, the first
		// thing this row gives up and a key the help teaches (#42, #51,
		// #200). Where the clause is new to the row — the page that fits,
		// which shed its movement key altogether under #83 — it is taken
		// only where the finished row still names every key it named
		// without it (footerTraded, #281, #284, #295); where the row
		// named `j/k scroll` already the new word is two cells shorter
		// and costs nothing.
		if m.readerPageFits() {
			keys = strings.Replace(keys, "ctrl+d/u half page · ", "", 1)
		}
	}
	if m.level >= levelWaypoints && !m.showHelp && !m.searching && !m.replying && m.archivedCount() > 0 && !m.archiveView && !m.rowNamesTheArchive() {
		// Where no band or fleet row names the archive, the footer does
		// (#62). The question is what the frame drew, not how wide it is:
		// below the board's width the reader takes the whole screen and
		// nothing names it, and above it the door is the band's — the
		// fleet's own last line is not drawn beside the reader — so a
		// fleet that leaves the band no digit (`free = 9 - used`) leaves
		// the frame nothing that names the archive. Twelve live sessions
		// and three hundred archived: at 120, 152 and 220 the reader's
		// frame named neither the count nor the key, while the same
		// keypresses at a hundred columns named both. The session view
		// one press shallower is the same frame: the trail and the
		// reader panel take the whole screen, the fleet list that names
		// the door at a hundred columns is gone, and the band is not
		// drawn — eleven more frames per width naming neither. The door
		// goes before the way out, whichever word this level's way out
		// wears (#56, #62).
		if strings.Contains(keys, " · esc back") {
			keys = strings.Replace(keys, " · esc back", " · esc back · A archive", 1)
		} else {
			keys = strings.Replace(keys, " · esc board", " · esc board · A archive", 1)
		}
	}
	if m.archiveView {
		// The clause is the row's, not the view's. On a hidden row `x`
		// brings it back and the archive says `x unhide`; on the live row
		// an archive with nothing in it keeps (#244, #248) the same key
		// takes it off the board, which is `x hide` and is what the live
		// list one `A` away already draws — the clause was dropped there
		// on the reasoning that "the key answers no question", and the
		// key answers it: pressed, it hid the session, moved the header,
		// the trail and the reader to another row and put a row back in
		// the archive, with nothing on the footer having offered it
		// (#24, #175, #187, #277, #284). On an archived row `x` refuses
		// in its own words and `hideKeyStuck` takes the clause under
		// #93's rule, which is where the drop belongs.
		s, ok := m.selected()
		switch {
		case !ok || !s.Live:
			keys = strings.Replace(keys, " · x unhide", "", 1)
		case m.onBoard(s):
			keys = strings.Replace(keys, " · x unhide", " · x hide", 1)
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
	if m.liveCount() == 1 && m.archiveView {
		keys = strings.Replace(keys, " · ⇧tab board", "", 1) // no board to go back to: A is the way (#53)
	}
	if !m.anyNeedsYou() {
		// Nothing amber: the grab answers no question, and its refusal
		// ("nothing is waiting on you") was the only thing it could say.
		// The help still teaches it (#78).
		keys = strings.Replace(keys, " · g grab", "", 1)
	}
	if m.archiveView && m.level >= levelWaypoints && !strings.Contains(keys, "A fleet") {
		// Below the list the archive's footer still names the way home (#55).
		keys = strings.Replace(keys, " · ? help", " · A fleet · ? help", 1)
	}
	if m.showMirror && m.level < levelReader {
		keys = strings.Replace(keys, "m live pane", "m conversation", 1) // the toggle's other side
	}
	if s, ok := m.selected(); ok {
		if pane, has := m.panes[s.Info.Key()]; !has || pane.Target == "" {
			// `r` types into a pane, like `enter` attaches to one: a row
			// that says "no pane" does not offer the other write either.
			keys = strings.Replace(keys, " · r reply", "", 1)
		}
	}
	if m.readerLane != "" && m.level >= levelReader {
		// The agent's own conversation: `r` and `a` are the lead's, and
		// a footer offering them here read as steering the agent (#49).
		keys = strings.Replace(strings.Replace(keys, " · r reply", "", 1), " · a ask", "", 1)
		if len(m.readerEvents()) == 0 {
			// Nothing to scroll, unfold, search or step: a page with no
			// turns offers only the way out (#56).
			for _, drop := range []string{"j/k scroll · ", "j/k rows · ", "ctrl+d/u half page · ", "space unfold · ", "/ search · ", "n/N · ", "[ ] turns · ", " · h/l session", "h/l session · "} {
				keys = strings.Replace(keys, drop, "", 1)
			}
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
//
// A width refused is drawn twice. The unit is the one word of it the
// person's own terminal supplies (#190), and it yields wherever the row
// without it names a key the row with it did not. The trade is measured on
// the finished row, because the note's own reserve is what the keys are
// shed against: #190's gate read the row half-shed and only for a key
// naming a level, so at a hundred columns `mirror needs 110 columns` stood
// without `r reply` — the key that types into the very pane it is refusing
// to draw — where the same refusal at eighty already stood bare. The cells
// go to a key that acts, and only where one comes back, which is how the
// deck's own chapter yield spends them (#24, #165, #175, #187, #194, #198,
// #201, #264).
func (m *Model) footerWith(keys string, w int) string {
	row, named := m.footerRow(keys, w)
	if unit := " columns"; strings.HasSuffix(m.note, unit) {
		was := m.note
		m.note = strings.TrimSuffix(was, unit)
		short, shortNamed := m.footerRow(keys, w)
		m.note = was
		if keysGained(named, shortNamed) {
			return short
		}
	}
	return row
}

// keysGained says whether the footer the shorter note leaves names every
// key the longer one named and one more besides. The attach aside is not a
// key (#55) and is read past on both sides, so a note yields its cells for
// a key and never for the parenthetical that finishes one.
func keysGained(was, now string) bool {
	read := func(s string) []string {
		var out []string
		for _, frag := range strings.Split(strings.ReplaceAll(s, attachHint, ""), " · ") {
			if f := strings.TrimSpace(frag); f != "" {
				out = append(out, f)
			}
		}
		return out
	}
	had, has := read(was), read(now)
	if len(has) <= len(had) {
		return false
	}
	for _, c := range had {
		found := false
		for _, d := range has {
			if d == c {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// footerRow draws one such row and says which keys it named.
func (m *Model) footerRow(keys string, w int) (string, string) {
	if m.note == "" {
		fits := func(k string) bool { return lipgloss.Width(k) <= w }
		keys = shedKeys(keys, m.chapterYield(keys, m.footerDrops(false), fits), fits)
		return dimStyle.Render(clip(keys, w)), keys
	}
	var left string
	// The clause the note already says goes before any key is shed.
	// `attach needs a pane` names the key and says what `enter · no pane`
	// says, so a row drawing both says one sentence twice — the harm #165
	// gave the note its form to end and the harm #95 and #96 folded on the
	// ship row and the header. It is not a stuck key, which yields only
	// where a key that acts comes back (#210, #216): nothing is given up,
	// the sentence is simply not said twice, and the cells it held go back
	// to the keymap, where they buy `/ search`, `[ ] chapters` and `n/N`
	// back on the rows that had shed them for it (#264's rule: a note
	// costs no key that acts on the row it refuses).
	if k := m.attachRefusalSaid(keys); k != "" {
		keys = strings.Replace(keys, k, "", 1)
	}
	// The note is the news, but the keymap is the only place the reader's
	// keys are named: shed the keymap's fragments for the note first, and
	// clip the note before the keymap goes.
	// The keys a note is about — the chapters it counts, the reply it
	// reports — go last: a footer that dropped `[ ] turns` on the frame
	// that said "❯ 3/12" read as the key having gone.
	drops := m.footerDrops(m.chapterNote())
	note := m.note
	// The trace note's quote is the row's: where a drawn row carries the
	// bytes that went, the footer says only where they went — the row
	// answers "what", the note answers "where" — and the keys the note
	// was shedding come back (#128's rule, the pane clause's own).
	if short, ok := m.noteLeavesTheQuoteToTheRow(note); ok {
		note = short
	}
	// The chapter note's quote is the row's too: `[` and `]` land the frame
	// on the very turn they count — the viewport opens on it at Lv1, the
	// cursor stands on it at Lv2, the reader is on its page at Lv3 — so the
	// row drawing that sentence is under the note, and the footer said it
	// again. #128's rule, and #134's reason for dropping a cut one ("the
	// turn the note landed on is drawn with it"), which holds whether the
	// copy would be cut or not. The count and the clock, which no row
	// draws, stay.
	if short, ok := m.noteLeavesTheChapterQuoteToTheRow(note); ok {
		note = short
	}
	note = m.noteLeavesTheWayBackToTheRow(note)
	// The note's reserve is twelve cells, or the note itself where it is
	// shorter: a note with no longer form to grow into buys nothing with
	// the cells it holds back from the keys.
	noteFloor := func() int { return min(12, lipgloss.Width(note)) }
	fitsWith := func(k, n string) bool { return lipgloss.Width(k)+2+max(noteFloor(), lipgloss.Width(n)) <= w }
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
	if strings.HasPrefix(note, "↪ answered") {
		// The digit is the answer's quote — which line went — and a
		// drawn row of this frame carries it: the note keeps the verb
		// and the destination, the shape "↪ sent to ⌁ dev:2.0" already
		// has (#128).
		if i := strings.Index(note, " · "); i > 0 && m.rowCarriesTrace(note[:i]) {
			note = "↪ answered" + note[i+len(" ·"):]
		}
	}
	if i := strings.LastIndex(note, " · "); i > 0 && !fits(note) && strings.HasPrefix(note[i+len(" · "):], "to "+mirrorMark) {
		// A trace's destination is the one clause that proves where a
		// line landed (§5): it outranks the optional keys and yields only
		// to the way out and the help.
		k := shed(note, " · ? help")
		// The stuck key yields here too. This row is shed before the
		// yield below is taken, so it was shed against the unyielded
		// rank: at eighty a line sent from a list a search had emptied
		// left ` j/k move · ? help · q quit` beside `↪ sent "please
		// continue" · to ⌁ main:0.0` — the movement key, which on that
		// frame answers `no row to move to`, held while `enter attach`
		// and `tab deeper` went, and one keypress later, under the
		// shorter note, both came back. A key that cannot move is not
		// the key a row keeps over one that acts (#210, #213, #216).
		// The trade is taken only where the row gives up nothing else it
		// names: the yield may buy a key that acts with a key that
		// cannot move, never with another key that acts (#39's ranks).
		rank := drops
		drops = m.chapterYield(whole, drops, func(k string) bool { return fitsWith(k, note) })
		if y := shed(note, " · ? help"); fitsWith(y, note) && yieldKeepsTheRow(k, y, m.stuckKeys(whole)) {
			k = y
		} else {
			drops = rank
		}
		if fitsWith(k, note) {
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
	drops = m.chapterYield(whole, drops, func(k string) bool { return fitsWith(k, note) })
	forms := noteForms(note)
	// A chapter note's keys are shed against its first clause; any other
	// note keeps its clauses (the way back, "A, then x") over the optional
	// keys. `? help` goes last of all (#32): only when the note's first
	// clause alone cannot stand beside it.
	minimal := note
	if m.chapterNote() && (strings.HasPrefix(note, glyphSaid) || strings.HasPrefix(note, glyphPrompt) || strings.HasPrefix(note, glyphBranch)) {
		minimal = forms[len(forms)-1] // the lane reader's note too (#60): its quote goes before the keys
	}
	if !fits(minimal) {
		keys = shed(minimal, " · ? help")
		if !fitsWith(keys, minimal) && !fitsWith(keys, forms[len(forms)-1]) {
			keys = shed(minimal, "")
		}
	}
	if wayBack := " · A, then x"; strings.Contains(minimal, wayBack) {
		// The way back is the one clause of a note that yields to a key
		// naming a level — `enter attach`, `tab deeper`, `tab session`,
		// `tab reader` — and only then: on the fleets whose strip draws
		// the hidden count beside the archive door the strip has no room
		// for the clause (#173), and `3 notebooks is hidden · A, then x`
		// cost the eighty-column footer `tab deeper`, the frame's only
		// naming of the way deeper; `A` stands on the strip and the help
		// says what `x` does there (#175).
		short := strings.Replace(minimal, wayBack, "", 1)
		if k, base := shed(short, " · ? help"), shed("", ""); levelKeyLost(base, keys) && !levelKeyLost(base, k) {
			keys, minimal = k, short
			note = strings.Replace(note, wayBack, "", 1)
			forms = noteForms(note)
		}
	}
	if i := strings.Index(minimal, " stays · "); i > 0 && strings.HasPrefix(m.note, minimal[:i+len(" stays")]) {
		// A hide refused: the clause after "stays" is the selected row's
		// own state, and the row two lines up draws it — `⊘ billing
		// quota 18m` over `Please run /login · API Err…`, `◍ etl  stuck
		// 6m`. So the reason yields to a key naming a level exactly as
		// the way back does (#175), and only where the key comes back:
		// `billing stays · dead on the API` cost the eighty-column
		// footer `tab deeper`, the frame's only naming of the way
		// deeper, while `etl stays · it hangs` on the same scene at the
		// same width kept it.
		short := minimal[:i+len(" stays")]
		if k, base := shed(short, " · ? help"), shed("", ""); levelKeyLost(base, keys) && !levelKeyLost(base, k) {
			keys, minimal = k, short
			note = short
			forms = noteForms(note)
		}
	}
	if i := strings.Index(minimal, " to "+mirrorMark); i > 0 && strings.HasPrefix(minimal, "↪ ") && strings.HasPrefix(m.note, "↪ ") {
		// A trace's destination proves where a line landed (#39) and
		// stays; the arrow is its preposition. `↪ sent to ⌁ harness:1.0`
		// is 23 cells against the 22 the eighty-column Lv1 footer leaves
		// beside `tab deeper`, so a reply to this fleet's own namesake
		// cost the frame its only naming of the way deeper while the same
		// keys on `⌁ tinker:0.0` (22) kept it — the harm #175, #187 and
		// #190 each folded. The word `to` answers no question the glyph
		// does not: the header, the strip and the tag row all draw the
		// pane as `⌁ harness:1.0` bare. So it yields to a key naming a
		// level, and only where the key comes back.
		short := minimal[:i] + " " + minimal[i+len(" to "):]
		if k, base := shed(short, " · ? help"), shed("", ""); levelKeyLost(base, keys) && !levelKeyLost(base, k) {
			keys, minimal = k, short
			note = strings.Replace(note, " to "+mirrorMark, " "+mirrorMark, 1)
			forms = noteForms(note)
		}
	}
	if strings.HasSuffix(minimal, " needs a pane") &&
		!strings.Contains(shedKeys(whole, drops, func(k string) bool { return lipgloss.Width(k) <= w }), "enter · no pane") {
		// #165 gave the no-pane refusal the naming form `attach needs a
		// pane` / `reply needs a pane` — nineteen and eighteen cells
		// against the seven of `no pane` — "precisely so that naming
		// the key would buy a key back": beside the row's own `enter ·
		// no pane` the clause says the same thing twice, and #232 takes
		// it there. Where the row draws no such clause the naming buys
		// nothing, and in the archive's own session view at eighty it
		// costs `tab deeper`, the frame's only naming of the way deeper
		// — the harm #156, #159, #165, #175, #187, #190, #194, #198,
		// #264, #283, #287 and #296 each folded — while the same
		// refusal on the archive's list one level out keeps the key.
		// So the naming yields to a key naming a level, and only where
		// the key comes back; the key the note answers is the key just
		// pressed, and `no pane` is the word the row's own clause and
		// the board's card already use. The candidate is measured with
		// the chapter key's yield below, since the two are taken
		// together, and against the footer this frame draws with no
		// news on it (#175), as that yield measures it.
		short := "no pane"
		cand := drops
		if order, moved := chapterKeyAboveTheWayIn(drops); moved && !m.chapterNote() && strings.Contains(whole, " · [ ] chapters") {
			cand = order
		}
		for i, d := range cand {
			if d == " · ? help" {
				cand = cand[:i]
				break
			}
		}
		// `fitsWith`'s own arithmetic for the short form's reserve.
		floor := max(min(12, lipgloss.Width(short)), lipgloss.Width(short))
		k := shedKeys(whole, cand, func(k string) bool { return lipgloss.Width(k)+2+floor <= w })
		plain := shedKeys(whole, drops, func(k string) bool { return lipgloss.Width(k) <= w })
		if levelKeyLost(plain, keys) && !levelKeyLost(plain, k) {
			note, minimal = short, short
			forms = noteForms(note)
			keys = shed(minimal, " · ? help")
		}
	}
	if goesBack := " · k goes back"; strings.HasSuffix(minimal, goesBack) {
		// `at the present · k goes back` names the key that retreats,
		// and the row it is drawn on names it already, first of all its
		// clauses, as `j/k rows` — one sentence said twice on one row,
		// the harm #95 and #96 folded on the ship row and the header
		// and #299 folded on this very row for `enter · no pane`. The
		// keymap is the only place the reader's keys are named, so it
		// is the note's clause that goes. Fourteen cells: at eighty in
		// the archive's own session view `j` and `ctrl+d` at the end of
		// a trail moved nothing and cost the footer `[ ] chapters` and
		// `tab deeper`, the frame's only naming of the way deeper — the
		// word `tab` stands nowhere else on the frame — while `G` on
		// the same stand answers `at the present` and keeps it. The
		// clause yields to a key naming a level and only where the key
		// comes back (#175, #297), and only where the finished row
		// still names the key the clause named. Measured with the
		// chapter key's yield below, since the two are taken on the
		// same row (#297).
		short := strings.TrimSuffix(minimal, goesBack)
		cand := drops
		if order, moved := chapterKeyAboveTheWayIn(drops); moved && !m.chapterNote() && strings.Contains(whole, " · [ ] chapters") {
			cand = order
		}
		for i, d := range cand {
			if d == " · ? help" {
				cand = cand[:i]
				break
			}
		}
		floor := max(min(12, lipgloss.Width(short)), lipgloss.Width(short))
		k := shedKeys(whole, cand, func(k string) bool { return lipgloss.Width(k)+2+floor <= w })
		plain := shedKeys(whole, drops, func(k string) bool { return lipgloss.Width(k) <= w })
		if strings.Contains(k, "j/k ") && levelKeyLost(plain, keys) && !levelKeyLost(plain, k) {
			keys, minimal = k, short
			note = strings.TrimSuffix(note, goesBack)
			forms = noteForms(note)
		}
	}
	if chapters := " · [ ] chapters"; !m.chapterNote() && strings.Contains(whole, chapters) {
		// The way in outlasts a key that moves inside a panel already
		// open (#39). At eighty the trail's own keys are 65 cells
		// against the 64 the twelve-cell note floor leaves, so every
		// note the trail draws — `at the start`, `no leg to move to`,
		// `mirror needs 110 columns` — cost the footer `tab deeper`,
		// the frame's only naming of the way deeper (the harm #175,
		// #187, #190, #194 and #198 each folded), while `[ ] chapters`,
		// fifteen cells and the widest optional key on the row, stood.
		// The chapter key yields to a key naming a level, and only
		// where the key comes back. Under a chapter key's own note the
		// key the note is about stays where it is (#24, #57).
		order, moved := chapterKeyAboveTheWayIn(drops)
		if moved {
			up := order
			for i, d := range order {
				if d == " · ? help" {
					up = order[:i]
					break
				}
			}
			k := shedKeys(whole, up, func(k string) bool { return fitsWith(k, minimal) })
			// The measure is the footer this frame draws with no news
			// on it — what the person saw one keypress ago — not the
			// note floor: at eighty the floor alone already costs the
			// key, so #175's own base would say nothing was lost.
			plain := shedKeys(whole, drops, func(k string) bool { return lipgloss.Width(k) <= w })
			if levelKeyLost(plain, keys) && !levelKeyLost(plain, k) {
				keys, drops = k, order
			}
		}
	}
	if pane != "" {
		// The pane clause goes before a key — not before the attach
		// hint, which #31 ranks beneath a key or a note: the parenthetical
		// stood where the hidden session's pane should have been. And it
		// comes back whenever the keys leave it room, whether or not the
		// hint is still there to give up: a refusal three cells longer
		// than the hide note lost the pane the hide note kept (#63).
		bare := strings.Replace(keys, attachHint, "", 1)
		switch {
		case fitsWith(keys, note+pane):
			note += pane
			forms = noteForms(note)
		case fitsWith(bare, note+pane):
			keys, note = bare, note+pane
			forms = noteForms(note)
		}
	}
	left = dimStyle.Render(clip(keys, w))
	room := w - lipgloss.Width(left) - 2
	if room < noteFloor() {
		return dimStyle.Render(shedClauses(note, w)), "" // no keymap fits beside it
	}
	// The reserve is room the note grows into (#134, #180). Where the note
	// has no form to grow into it the cells stand blank and a key is
	// missing — `❯ 1/1` beside 22 of them at eighty, with `space unfold`
	// shed — so a key comes back while the note is drawn at the same width
	// beside it.
	if back := keysHeld(whole, drops, keys, func(k string) int {
		return m.noteDrawnIn(note, w-lipgloss.Width(clip(k, w))-2)
	}); back != keys {
		keys = back
		left = dimStyle.Render(clip(keys, w))
		room = w - lipgloss.Width(left) - 2
	}
	shown := ""
	for _, f := range forms {
		if lipgloss.Width(f) <= room {
			shown = dimStyle.Render(f)
			break
		}
		if m.chapterNote() && (strings.HasPrefix(note, glyphSaid) || strings.HasPrefix(note, glyphBranch)) {
			// The turn the note landed on is drawn with it (#128), so a
			// quote cut to the room says less than the count alone (#134).
			continue
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
	return left + strings.Repeat(" ", gap) + shown, keys
}

// noteDrawnIn is the width the note is drawn at in room cells — the fullest
// form that fits there, the quote clipped where that is allowed, the last
// form shed to the room when none fits. It is what the footer's reserve is
// measuring: twelve cells the note cannot grow into are blank cells.
func (m *Model) noteDrawnIn(note string, room int) int {
	if room < 0 {
		return 0
	}
	forms := noteForms(note)
	for _, f := range forms {
		if w := lipgloss.Width(f); w <= room {
			return w
		}
		if m.chapterNote() && (strings.HasPrefix(note, glyphSaid) || strings.HasPrefix(note, glyphBranch)) {
			continue
		}
		if q := fitQuote(f, room); q != "" {
			return lipgloss.Width(q)
		}
	}
	return lipgloss.Width(shedClauses(forms[len(forms)-1], room))
}

// keysHeld puts the shed fragments back, lowest-ranked first, while held —
// the width the note is drawn at beside the keys — does not fall. It is the
// backfill of shedKeys measured against the note rather than against the
// reserve, and it keeps #39's discipline: a key comes back only if every
// key ranked below it came back too.
func keysHeld(whole string, order []string, keys string, held func(string) int) string {
	gone := map[string]bool{}
	for _, frag := range order {
		if strings.Contains(whole, frag) && !strings.Contains(keys, frag) {
			gone[frag] = true
		}
	}
	build := func() string {
		k := whole
		for _, frag := range order {
			if gone[frag] {
				k = strings.Replace(k, frag, "", 1)
			}
		}
		return k
	}
	if build() != keys {
		return keys // the shed is not a plain subset: leave it alone
	}
	want := held(keys)
	for i := len(order) - 1; i >= 0; i-- {
		frag := order[i]
		if !gone[frag] {
			continue
		}
		if frag == " · n/N" && gone[" · / search"] {
			continue // the walk keys ride only with the search they walk
		}
		gone[frag] = false
		if held(build()) < want {
			gone[frag] = true
			break
		}
	}
	return build()
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

// chapterKeyAboveTheWayIn is the shed order with `[ ] chapters` moved to
// just above the key that names the way in, and whether the row names one
// for it to stand above. It is the order the chapter key's yield spends
// its cells in, and the order a note's own yield is judged against: the
// two are taken on the same row, so a note measured against the unyielded
// order reads a key as lost that the yield brings back.
func chapterKeyAboveTheWayIn(drops []string) ([]string, bool) {
	const chapters = " · [ ] chapters"
	order := make([]string, 0, len(drops))
	moved := false
	for _, d := range drops {
		if d == chapters {
			continue
		}
		if !moved && (d == " · tab deeper" || d == " · tab reader") {
			order, moved = append(order, chapters), true
		}
		order = append(order, d)
	}
	return order, moved
}

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
	if gone[attachHint] {
		// The aside is not a key: it finishes a sentence a key already
		// began, so it comes back whenever it fits, whatever stayed shed
		// above it (#55) — a 152 footer stood twenty cells short.
		gone[attachHint] = false
		if !fits(build()) {
			gone[attachHint] = true
		}
	}
	return build()
}

// chapterYield is the drop order with `[ ] chapters` first to go, on a
// trail whose one chapter the trail already stands on: `[` answers `no
// earlier prompt` and `]` answers `no later prompt`, whichever is pressed
// and at every width, so the fifteen cells the widest optional key on the
// row spends buy a promise the next keypress refuses. #83 dropped the page
// keys from a trail that fits, #56 the whole set from a lane's page with
// no turns, #200 the scroll keys from a reader page all on screen: a key
// that cannot move is not on the row. The cells go to a key that acts and
// — as with #201's own yield and #207's step — only where one comes back,
// so where nothing is shed the key stands and a wide footer still names
// what `[` and `]` are (#193). Under a chapter key's own note the key the
// note is about stays where it is (#24, #57).
func (m *Model) chapterYield(whole string, drops []string, fits func(string) bool) []string {
	order := drops
	var head []string
	stuck := m.stuckKeys(whole)
	cannotMove := map[string]bool{}
	for _, k := range stuck {
		cannotMove[k] = true
	}
	// A stuck key is tried again once another has yielded. The trade is
	// judged against the row as it stands, and the row moves as each
	// yield is taken: at eighty the reader's `[ ] turns` bought nothing
	// beside `space unfold` at the row's head, and once the unfold key
	// had yielded and bought `/ search` the twelve cells were `enter
	// attach` — but the pass had already gone by. So the pass repeats
	// while any yield is taken; a key that buys nothing against the
	// final row is still refused (#193), and the order the cells are
	// spent in is unchanged.
	taken := map[string]bool{}
	for again := true; again; {
		again = false
		for _, yield := range stuck {
			if taken[yield] {
				continue
			}
			cand := append(append([]string(nil), head...), yield)
			next := append([]string(nil), cand...)
			for _, d := range drops {
				stood := false
				for _, c := range cand {
					if c == d {
						stood = true
					}
				}
				if !stood {
					next = append(next, d)
				}
			}
			// A key that cannot move is not the gain that buys the trade
			// (#216): the archive's list traded eleven cells of a movement
			// key that answers `the only session` for fifteen of
			// `[ ] chapters` on a trail of one prompt, where both chapter
			// keys refuse. A trade brings back more than one key, so the
			// stuck ones are passed over rather than the whole trade refused
			// on the first of them: a row that also gains a key that acts
			// has bought its cells back.
			was := shedKeys(whole, order, fits)
			if keysActGained(was, shedKeys(whole, next, fits), next, yield, cannotMove) == "" &&
				!rowAlreadyShed(whole, was, order, cannotMove) {
				continue
			}
			head, order = cand, next
			taken[yield], again = true, true
		}
	}
	return order
}

// rowAlreadyShed says whether the row, as it stands, has given up a key
// that acts. #210's gate — a stuck key yields only where a key that acts
// comes back — was written for the row where nothing is shed, so that "a
// wide footer still names what `[` and `]` are" (#193); where the row has
// already given a key that acts up for width that reason is spent, and
// the gate was the only thing left holding a key that cannot move. At
// eighty the reader stood at 79 of 80 on ` space unfold · [ ] turns · esc
// back · ? help · q quit`, where `[` answers `no earlier turn` and `]`
// answers `no later turn` and neither moves a drawn cell with colour on
// or off, having shed `/ search`, `n/N`, `r reply`, `a ask` and `enter
// attach`: nothing could come back — `enter attach` is one cell too wide
// and `a ask` is ranked under it (#39) — so twelve cells stayed on a key
// that refuses both halves. The cells are given up, not spent: no key
// comes back, so #39's rank is untouched, and the help still teaches the
// key (#43, #78).
// A key that is itself stuck is not a key that acts (#216), the attach
// aside is not a key (#55), and a key the row still draws in its other
// form — the head forms `enter attach · ` and `space unfold · ` name the
// same key as their separator-led fragments — has not been shed at all.
func rowAlreadyShed(whole, was string, order []string, stuck map[string]bool) bool {
	for _, d := range order {
		if d == attachHint || d == " · enter · no pane" || stuck[d] {
			continue
		}
		if strings.Contains(whole, d) && !strings.Contains(was, keyWord(d)) {
			return true
		}
	}
	return false
}

// keyWord is the fragment's key without the separator it is joined by or
// the attach aside it may carry, so a key is read as one key whichever
// form the row draws it in.
func keyWord(frag string) string {
	k := strings.TrimSuffix(strings.TrimPrefix(frag, " · "), " · ")
	return strings.TrimSpace(strings.ReplaceAll(k, attachHint, ""))
}

// stuckKeys are the keys this row offers that cannot move from where it
// stands, in the order their cells are spent: the chapter key first, then
// the row's own movement key. Both are one rule — a key that cannot move
// is not on the row — and each is taken only where a key that acts comes
// back for it, so where nothing is shed both keys stand (#193).
func (m *Model) stuckKeys(whole string) []string {
	var stuck []string
	if k := m.chapterKeyStuck(whole); k != "" {
		stuck = append(stuck, k)
	}
	if k := m.moveKeyStuck(whole); k != "" {
		stuck = append(stuck, k)
	}
	if k := m.unfoldKeyStuck(whole); k != "" {
		stuck = append(stuck, k)
	}
	if k := m.walkKeyStuck(whole); k != "" {
		stuck = append(stuck, k)
	}
	if k := m.hideKeyStuck(whole); k != "" {
		stuck = append(stuck, k)
	}
	if k := m.attachRefusalSaid(whole); k != "" {
		stuck = append(stuck, k)
	}
	return stuck
}

// attachRefusalSaid is the row's own `enter · no pane` under the note that
// already says it — or "" anywhere else. Every other stuck key stays under
// its own note because the note does not name it: `infra stays · it is
// asking` never says `x`, so the row refusing `x` must (#24, #57). The
// attach refusal is the one whose note does name it — #165 gave it the
// form `mirror needs 110 columns` already used, `attach needs a pane`,
// precisely so that naming the key would buy a key back — and beside that
// note the clause is the same sentence twice, twelve cells to its left.
// A refusal goes before a key that acts (#52, #198, #206), and this one is
// taken only where one comes back.
func (m *Model) attachRefusalSaid(whole string) string {
	if m.note != "attach needs a pane" {
		return ""
	}
	for _, k := range []string{" · enter · no pane", "enter · no pane · "} {
		if strings.Contains(whole, k) {
			return k
		}
	}
	return ""
}

// hideKeymap is the clause for `x` where the keys are now. `case "x"` is
// the deck's, not a level's: it takes the selected session off the board —
// or brings it back — at the board, at the list, in the session view and
// in the reader alike, and at the last two it swaps the frame under the
// person for another session's while no key on the row says so. A key that
// acts and is never named is the one thing a footer is for (#24, #175,
// #187, #277), and the row that refuses `x` must name `x` (#227);
// `shedOrder` has ranked `x hide` and `x unhide` among these two levels'
// own keys all along with nothing on the row to match. In the archive the
// key is the way back, the clause the archive's own list already wears —
// and everything `keymap` already says of it stands: a fleet of one has no
// session to hide, and a row that is not hidden has nothing to bring back.
func (m *Model) hideKeymap() string {
	if m.archiveView {
		return "x unhide"
	}
	return "x hide"
}

// hideKeyStuck is the board's or the list's `x hide` on a selection it
// cannot take off the board — or "" when the row does not offer it or the
// key acts. `toggleHidden` refuses what owes you an alarm (`infra stays ·
// it is asking`, `· it hangs`, `· it is looping`, `· dead on the API`),
// the same answer at every width and however many times it is pressed,
// and the keymap already drops the key outright where the fleet is one
// session — "the keys that move between sessions answer no question" —
// which is this rule one rung up, asked of the fleet rather than of the
// selection. The same rule as the chapter key, the movement key, the
// unfold key and the walk key at the one board key they did not reach
// (#210, #211, #213, #219, #222, #223). Under its own note the key stays
// (#24, #57): the row refusing `x` must name `x`.
func (m *Model) hideKeyStuck(whole string) string {
	if m.hideNote() || m.hideKeyMoves() {
		return ""
	}
	const hide = " · x hide"
	if strings.Contains(whole, hide) {
		return hide
	}
	return ""
}

// hideNote says whether the note is the hide key's own: a session taken
// off the board, one brought back, or any of `x`'s refusals (#24).
func (m *Model) hideNote() bool {
	return strings.Contains(m.note, " is hidden · A, then x") ||
		strings.HasSuffix(m.note, " is back on the board") ||
		strings.Contains(m.note, " stays · ") ||
		m.note == "the live one stays" ||
		m.note == "it is not hidden"
}

// hideKeyMoves reports whether `x` acts from where the row stands: a
// hidden session it brings back, or a live one `hideRefusal` lets go.
func (m *Model) hideKeyMoves() bool {
	s, ok := m.selected()
	if !ok {
		return false
	}
	if m.hidden[s.Info.Key()] {
		return true // the key brings it back
	}
	return m.hideRefusal(s) == ""
}

// unfoldKeyStuck is the reader's `space unfold` on a page where no row can
// fold or unfold — or "" when the row does not offer it or the key acts.
// The same rule as the chapter key and the movement key (#210, #211,
// #213, #219), at the one reader key they did not reach: on a conversation
// whose screen holds no tool result, Space answers `nothing to unfold on
// screen` whichever row is on top and at every width, while the reader's
// shed order ranks it above `/ search`, `r reply`, `a ask` and
// `enter attach` — the keys that act on the session it reads. Under its
// own note the key stays (#24, #57): the row refusing Space must name it.
func (m *Model) unfoldKeyStuck(whole string) string {
	if m.level < levelReader || m.unfoldNote() || m.unfoldKeyMoves() {
		return ""
	}
	for _, k := range []string{" · space unfold", "space unfold · "} {
		if strings.Contains(whole, k) {
			return k
		}
	}
	return ""
}

// unfoldNote says whether the note is Space's own: a fold it opened or
// closed, or its refusal (#24).
func (m *Model) unfoldNote() bool {
	return m.note == "nothing to unfold on screen" ||
		strings.HasPrefix(m.note, "unfolded ") || strings.HasPrefix(m.note, "folded ")
}

// unfoldKeyMoves reports whether Space acts from where the reader stands:
// a foldable row on the screen it draws — `toggleFold`'s own walk, which
// takes the first folded result from the top of the screen down.
func (m *Model) unfoldKeyMoves() bool {
	doc := m.doc(m.readerWidth())
	top := m.readerTop(doc)
	for i := top; i < len(doc) && i < top+m.readerHeight(); i++ {
		if doc[i].foldable() {
			return true
		}
	}
	return false
}

// walkKeyStuck is the reader's `n/N` with no search to walk — or "" when
// the row does not offer it or the key acts. Until `/` has been entered
// `jumpMatch` answers `no search — / starts one` to both keys, whichever
// is pressed and at every width, and the row already knows the pair rides
// only with the search it walks (`shedKeys`): the same rule as the chapter
// key and the movement key at the pair they did not reach (#210, #211,
// #213, #219). `/ search` stands beside it and says how a walk begins, so
// the row still teaches how to start one. Under its own note the key
// stays (#24, #57).
func (m *Model) walkKeyStuck(whole string) string {
	if m.level < levelReader || m.note == "no search — / starts one" || m.query != "" {
		return ""
	}
	const walk = " · n/N"
	if strings.Contains(whole, walk) {
		return walk
	}
	return ""
}

// moveKeyStuck is the movement key this row leads with that cannot move
// from where it stands — `j/k move` on the list and the board, `j/k rows`
// on a trail and in the reader — or "" when the row leads with none or the
// key acts. A fleet of one has nowhere to move to and answers `the only
// live one`; a trail whose one row the cursor is on answers `no leg to
// move to`, whichever of `j` and `k` is pressed and at every width. Under
// the movement key's own note the key stays, as a chapter key does under
// its own (#24, #57): the row refusing `j` must name `j`.
func (m *Model) moveKeyStuck(whole string) string {
	if m.moveNote() || m.moveKeysMove() {
		return ""
	}
	for _, k := range []string{"j/k move · ", "j/k rows · ", "j/k legs · ", "j/k scroll · "} {
		if strings.Contains(whole, k) {
			return k
		}
	}
	return ""
}

// moveNote says whether the note is the movement key's own: a move that
// moved nothing, at either end or with nowhere to go (#24).
func (m *Model) moveNote() bool {
	switch m.note {
	case "no leg to move to", "no row to move to", "no column to move to", "at the start", "at the present", "at the present · k goes back",
		"the only live one", "the only session", "the last session", "the first session":
		return true
	}
	return false
}

// moveKeysMove reports whether `j` or `k` moves anything from where the
// row stands: another row to land on. On the trail that is a second row
// (`TrailRows`, the list the cursor walks); on the list and the board a
// second session in view (`viewOrder`, the count `onlyOrLast` refuses on).
// In the reader the scroll keys are already gone from a page that is all
// on screen (#200), and where they stand the page scrolls.
func (m *Model) moveKeysMove() bool {
	switch {
	case m.level >= levelReader:
		return true
	case m.level >= levelWaypoints:
		return len(TrailRows(m.trail, m.level)) > 1
	default:
		return len(m.viewOrder()) > 1
	}
}

// chapterKeyStuck is the chapter key this row offers that cannot move from
// where it stands — `[ ] chapters` on the trail, `[ ] turns` in the reader
// — or "" when the row offers neither or the key acts. The reader's key is
// the same key one level in: the ❯ rows are the conversation's chapters as
// the prompts are the trail's (`readerChapter`), and standing on the only
// turn of a first prompt `[` answers `no earlier turn` and `]` answers
// `no later turn`, whichever is pressed and at every width. Under a
// chapter key's own note the key the note is about stays (#24, #57).
func (m *Model) chapterKeyStuck(whole string) string {
	if m.chapterNote() {
		return ""
	}
	const chapters = " · [ ] chapters"
	if strings.Contains(whole, chapters) && !m.chapterKeysMove() {
		return chapters
	}
	const turns = " · [ ] turns"
	if m.level >= levelReader && strings.Contains(whole, turns) && !m.turnKeysMove() {
		return turns
	}
	return ""
}

// keysActGained is the key that acts that `now` names and `was` does not,
// "" when there is none or when `now` drops a key `was` drew other than
// `yield`, the chapter key whose cells are being spent; an attach that cannot work is a refusal,
// not a key (#206); the attach aside is not a key at all (#55); and the
// page key is the first fragment the row gives up, a shortcut for a
// distance `j` covers and one the help teaches (#42, #51). None of the
// four is the gain that buys the trade.
func keysActGained(was, now string, drops []string, yield string, stuck map[string]bool) string {
	// The aside is not a key (#55), and it is the one fragment that
	// changes the *form* the attach key wears: a row that draws
	// `enter attach` mid-row and then wears `enter attach (prefix d
	// returns) · ` at its head has gained the parenthetical, not the key.
	// Both sides are read without it, so the head form and the
	// separator-led form name the same key on both.
	was = strings.Replace(was, attachHint, "", 1)
	now = strings.Replace(now, attachHint, "", 1)
	for _, d := range drops {
		// A key is on the row whichever form it wears: the shed order
		// carries both `" · enter attach"` and `"enter attach · "`, and a
		// key that led the row before the trade and stands mid-row after
		// it has not been lost — nor is a key that leads the row after
		// the trade one the trade did not bring back.
		name := strings.TrimSuffix(strings.TrimPrefix(d, " · "), " · ")
		in, had := strings.Contains(now, name), strings.Contains(was, name)
		if had && !in && d != yield {
			return "" // something the row drew is gone
		}
		if in && !had && d != " · enter · no pane" && d != attachHint && !stuck[d] {
			return d
		}
	}
	return ""
}

// chapterNote says whether the note is a chapter key's: a chapter counted,
// or a chapter key's refusal — either keeps the chapter keys (#24).
func (m *Model) chapterNote() bool {
	for _, p := range []string{glyphSaid, glyphPrompt, glyphBranch + " 1/", "no later prompt", "no earlier prompt", "no later turn", "no earlier turn"} {
		if strings.HasPrefix(m.note, p) {
			return true
		}
	}
	return false
}

// mirrorNote says whether the note is `m`'s own: the frame it was pressed on.
func (m *Model) mirrorNote() bool {
	return m.note == "the live pane" || m.note == "the conversation" || strings.HasPrefix(m.note, "mirror on")
}

// footerDrops is the shed order with `m` held back where the mirror owns the
// panel: `m` is the one key that replaces the panel, so its undo is named on
// the frames the mirror stands on, not only the frame it was pressed on —
// one keypress later the note is gone and at 120 columns the key went with
// it, over a panel the mirror still owned (#62's rule, off its own frame).
// Under another note the note's own keys come first (#168): the news is one
// keypress old, and `m` would cost the row `enter attach` and `a ask`.
func (m *Model) footerDrops(chapter bool) []string {
	drops := m.shedOrder(chapter)
	if !m.mirrorNote() && !(m.note == "" && m.mirrorShown()) {
		return drops
	}
	kept := drops[:0]
	for _, drop := range drops {
		if drop == " · m live pane" || drop == " · m conversation" {
			continue
		}
		kept = append(kept, drop)
	}
	return kept
}

// readerPageFits says whether the reader's whole document is on screen —
// the page where `j` and `k` move the cursor and never the viewport (#83,
// #300).
func (m *Model) readerPageFits() bool {
	return len(m.doc(m.readerWidth())) <= m.readerHeight()
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
	if m.showMirror && m.level < levelReader {
		mirror = " · m conversation" // the toggle's other label, the same rank
	}
	// The attach aside goes first, then the page key — a shortcut for a
	// distance `j` covers, which the help teaches (#42, #51).
	// The page key sheds at its own rank wherever it lands on the row:
	// where the movement key that leads the row has yielded (#213), the
	// page key becomes the row's head and the separator-led fragment
	// above matches nothing — the head form #56 gave the attach key and
	// #200 gave `space unfold`.
	order := []string{attachHint, " · ctrl+d/u half page", "ctrl+d/u half page · ", mirror, " · h/l session"}
	homeKey := m.level >= levelWaypoints && m.archiveView
	// The way in and the way out are not shared in the same sense as
	// `h/l session` or the attach hint — they are how you enter and leave
	// this level — so they stand with the level's own keys below, ranked
	// under the keys the level is for.
	var own []string
	switch {
	case m.level >= levelReader:
		// The archive door stands with the way out, as `A fleet` does
		// below the archive's list (#56, #62): it outlasts the reader's
		// own keys, which the help teaches, and goes before `esc`.
		own = []string{" · g grab", " · x hide", " · x unhide", " · tab deeper", " · tab reader", " · [ ] chapters", " · r reply",
			// An attach that cannot work is a refusal, not a key, and a
			// refusal goes before a key that acts — the rank the
			// archive's own list already gives it (#52, #198). In the
			// archive `a ask` is the reason to be there, and at eighty
			// the reader shed it to hold eighteen cells for a refusal
			// that could not be drawn at that width either. `/ search`
			// and the walk it starts are keys that act too, so the
			// refusal goes before them as it goes before `a ask`.
			" · enter · no pane", " · n/N", " · / search", " · a ask", " · enter attach",
			// A lane's page with no turns offers the attach key first and
			// the way out behind it, so the separator-led fragments above
			// match nothing and `esc back` was the only key left to shed
			// (#24: the way out is the last key to go). The attach key
			// sheds at its own rank wherever it leads the row.
			"enter attach (prefix d returns) · ", "enter attach · ", "enter · no pane · ",
			" · esc back", " · esc board", " · [ ] turns", " · space unfold",
			// A page that is all on screen offers no scroll key, so
			// `space unfold` leads the row and the separator-led form
			// above matches nothing — the same head-form the attach key
			// needs two lines up.
			"space unfold · ", " · A archive"}
	case m.level >= levelWaypoints:
		// The archive door stands with the way out here as it does in
		// the reader (#56, #62): last of the level's own keys.
		own = []string{" · g grab",
			// The refusal goes before the key that acts here too (#52).
			" · enter · no pane", " · n/N", " · / search", " · x hide", " · x unhide", " · space unfold", " · [ ] turns",
			" · a ask", " · enter attach", " · esc back", " · esc board", " · r reply", " · tab deeper", " · tab reader", " · [ ] chapters", " · A archive"}
	default:
		// A row whose movement key has yielded (#213, #216, #259) leads
		// with the attach key, and the separator-led fragment above then
		// matches nothing — the head forms the reader's own list has
		// carried since #56 and #200, at the level that never had them:
		// at eighty a line sent from an archive a search had emptied
		// could not shed `enter attach` once `j/k move` had left the
		// row's head, so the yielded row did not fit, the yield #259
		// folded was refused for width, and the sent row kept a move
		// that cannot move beside `↪ sent "please continue" · to
		// ⌁ main:0.0`. The head form sheds at the key's own rank.
		heads := func(own []string) []string {
			out := make([]string, 0, len(own)+3)
			for _, k := range own {
				out = append(out, k)
				switch k {
				case " · enter attach":
					out = append(out, "enter attach (prefix d returns) · ", "enter attach · ")
				case " · enter · no pane":
					out = append(out, "enter · no pane · ")
				}
			}
			return out
		}
		// The board and the list: the chapters belong to a trail that is
		// not open, and the way in outlasts the keys that act on a row.
		// In the archive `a` is the reason to be there — a claude on a
		// session you can no longer attach to — so it stands with the
		// archive's own keys; on the live list it is the trail's.
		own = heads([]string{" · [ ] chapters", " · [ ] turns", " · space unfold", " · a ask", " · n/N", " · / search", " · g grab", " · x hide", " · r reply", " · tab deeper", " · enter attach", " · enter · no pane", " · x unhide"})
		if m.archiveView {
			// "enter · no pane" is a refusal, and a refusal goes before
			// the way in: the archive's `tab deeper` outlasts it (#52).
			own = heads([]string{" · [ ] chapters", " · [ ] turns", " · space unfold", " · enter · no pane", " · n/N", " · / search", " · g grab", " · x hide", " · r reply", " · tab deeper", " · enter attach", " · a ask", " · x unhide"})
			if m.enterKeymap() != "enter · no pane" {
				// #52 ranks `a` with the archive's own keys because in
				// the archive it is "the reason to be there — a claude
				// on a session you can no longer attach to". Where the
				// row's own `enter` says `enter attach`, that reason is
				// not this frame's: the session is still there to
				// attach to, and `a ask` is a key that acts on a row,
				// which the way in outlasts (#39).
				own = heads([]string{" · [ ] chapters", " · [ ] turns", " · space unfold", " · a ask", " · enter · no pane", " · n/N", " · / search", " · g grab", " · x hide", " · r reply", " · tab deeper", " · enter attach", " · x unhide"})
			}
		}
	}
	order = append(order, own...)
	// The row's movement key gives way before the way out and before the
	// keys a note is about: the arrows move too, and a footer that kept
	// `j/k rows` and shed `esc back` left `q quit` as the only named exit.
	order = append(order, "j/k move · ", "j/k rows · ", "j/k legs · ", "j/k scroll · ")
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
	if homeKey {
		// Below the archive's list `A fleet` is the way home: it goes with
		// the way out, after the level's own keys (#56).
		order = append(order, " · A fleet")
	}
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
	if m.walkNote() {
		// The same rule at the walk key (#24, #57): under the note `n`
		// and `N` put there the row keeps the pair, as it keeps the
		// chapter keys under a chapter key's. A row that said the match
		// was on screen while shedding the key that had said so read as
		// the key having gone — the harm #223 pinned from the other side.
		var keep []string
		for i := 0; i < len(order); i++ {
			if order[i] == " · n/N" {
				keep = append(keep, order[i])
				order = append(order[:i], order[i+1:]...)
				i--
			}
		}
		order = append(order, keep...)
	}
	return append(order, " · ? help")
}

// walkNote says whether the note is the walk key's own: the answer `n` and
// `N` give — which match of how many they are standing on. Under it the
// pair stays, as the chapter keys stay under a chapter key's note (#24).
func (m *Model) walkNote() bool {
	return strings.HasPrefix(m.note, "match ")
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
	if strings.HasPrefix(note, glyphSaid) || strings.HasPrefix(note, glyphPrompt) || strings.HasPrefix(note, glyphBranch) {
		// The lane reader's note is a chapter note too (#59): its quote
		// goes before the level's keys do.
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
		return joinColumnsBelow(h, []column{
			{tw, m.trailColumn(tw, h)},
			{mw, companion(mw, h)},
		}, m.boxFloor())
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
		trail := m.trailColumn(tw, h)
		m.trailRows = trail
		defer func() { m.trailRows = nil }()
		if m.level >= levelWaypoints && !(m.sessionView() && m.showMirror) {
			// The middle is the reader: a navigator stands to the left of
			// what it navigates (#19), and the mirror's reason for the
			// right-hand trail — the middle is a rendering of something
			// else — is the mirror's alone. Fleet, trail, reader; and the
			// `tab` that drops the fleet then moves nothing (#46, #160).
			return joinColumnsBelow(h, []column{
				{fw, m.fleetColumn(fw, h)},
				{tw, trail},
				{mw, middle(mw, h)},
			}, m.boxFloor())
		}
		return joinColumnsBelow(h, []column{
			{fw, m.fleetColumn(fw, h)},
			{mw, middle(mw, h)},
			{tw, trail},
		}, m.boxFloor())
	}
	// Two columns. At Lv3 on a terminal too narrow for three, the conversation
	// takes the trail's place rather than going unread.
	second := m.trailColumn
	if m.level >= levelReader {
		second = m.readerColumn
	}
	right := second(tw, h)
	if m.level < levelReader {
		m.trailRows = right
		defer func() { m.trailRows = nil }()
	}
	return joinColumnsBelow(h, []column{
		{fw, m.fleetColumn(fw, h)},
		{tw, right},
	}, m.boxFloor())
}

// boxFloor is the row the reply box's last line lands on, one past it, or
// zero when no box stands.
func (m *Model) boxFloor() int {
	if !m.replyBox.on {
		return 0
	}
	return m.replyBox.top + m.replyBox.h
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
	return joinColumnsBelow(h, cols, 0)
}

// joinColumnsBelow is joinColumns with a floor under the hairline: the
// reply box is drawn over the deck after the columns are joined, so a box
// whose last row falls past every column's content left its own border
// standing where the hairline had already stopped. The hairline stops
// where the frame's content stops, the box's rows included.
func joinColumnsBelow(h int, cols []column, least int) []string {
	stop := least
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

// box is the reply panel's place on the body, remembered before the body is
// drawn so a column can tell whether a row of its own will be covered.
type box struct {
	on              bool
	left, top, w, h int
}

// panelHides says whether the reply box covers the cell at body row y in a
// column that begins at x and is w wide.
func (m *Model) panelHides(x, w, y int) bool {
	b := m.replyBox
	if !b.on {
		return false
	}
	// Horizontally the row is hidden only where the box begins inside
	// the trail's prefix — glyph and class — since a row the box starts
	// past still draws its sentence to the left of it (#108).
	return y >= b.top && y < b.top+b.h && x < b.left+b.w && b.left <= x+trailPrefixWidth
}

// noteLeavesTheQuoteToTheRow trims a trace note — `↪ sent "go on" · to ⌁
// main:0.0` — to its destination where a row of this very frame already
// draws the bytes. The row is the fuller copy: it keeps the quote whole
// or clipped and its own clock, so the note's copy is never the only one.
// noteLeavesTheWayBackToTheRow is the hide note's form of the same rule:
// where a drawn row carries `A, then x` — the strip's hidden count — the
// note says only what happened. The clause cost the eighty-column footer
// `tab deeper` and `x hide` beside a strip row with 24 cells free; where
// no drawn row carries the count the note keeps the way back (#173, #64).
func (m *Model) noteLeavesTheWayBackToTheRow(note string) string {
	const wayBack = " · A, then x"
	if !strings.Contains(note, " is hidden"+wayBack) {
		return note
	}
	for _, row := range m.bodyRows {
		if strings.Contains(ansi.Strip(row), "hidden"+wayBack) {
			// The namesake's pane clause, where the note carries one
			// (#31), stays: only the way back moves to the row.
			return strings.Replace(note, wayBack, "", 1)
		}
	}
	return note
}

// yieldKeepsTheRow says whether the row the yielded rank sheds to still
// names every key the plain rank's row named, bar a stuck one it gives up:
// the yield trades a key that cannot move for one that acts, and never a
// key that acts for another — #39's ranks stand under it (#210, #213).
func yieldKeepsTheRow(plain, yielded string, stuck []string) bool {
	gone := map[string]bool{}
	for _, frag := range stuck {
		gone[keyWord(frag)] = true
	}
	for _, word := range strings.Split(plain, " · ") {
		word = strings.TrimSpace(strings.ReplaceAll(word, attachHint, ""))
		if word == "" || gone[word] {
			continue
		}
		if !strings.Contains(yielded, word) {
			return false
		}
	}
	return true
}

// levelKeyLost says whether keys, shed for a note, lost a key naming a
// level that base — the keys shed for the note floor alone — kept (#175).
func levelKeyLost(base, keys string) bool {
	for _, k := range []string{"enter attach", "tab deeper", "tab session", "tab reader"} {
		if strings.Contains(base, k) && !strings.Contains(keys, k) {
			return true
		}
	}
	return false
}

// rowNamesTheArchive says whether a drawn row of this frame already names
// the archive door and the key that browses it — the band's header beside
// the reader or the fleet's own last line, in every form they shed to:
// `recent · 41 archived · A browses`, `0 of 300 archived · A browses`,
// `41 archived · 1 hidden · A`.
func (m *Model) rowNamesTheArchive() bool {
	for _, row := range m.bodyRows {
		for _, seg := range strings.Split(ansi.Strip(row), "│") {
			i := strings.Index(seg, "archived · ")
			if i < 0 {
				continue
			}
			rest := strings.Trim(seg[i+len("archived · "):], "─ ")
			if j := strings.LastIndex(rest, " · "); j >= 0 {
				rest = rest[j+len(" · "):] // the hidden count stands between the count and the key (#176)
			}
			if rest == "A" || rest == "A browses" {
				return true
			}
		}
	}
	return false
}

// rowCarriesTrace says whether a drawn row of this frame already begins with
// the trace's own head — "↪ answered 1".
func (m *Model) rowCarriesTrace(head string) bool {
	for _, row := range m.bodyRows {
		for _, seg := range strings.Split(ansi.Strip(row), "│") {
			if strings.HasPrefix(strings.TrimSpace(seg), head) {
				return true
			}
		}
	}
	return false
}

// noteLeavesTheChapterQuoteToTheRow is #128's rule for a chapter note — the
// prompt's, the turn's and the lane's alike. Where a drawn row of this frame
// says the sentence the note quotes, the note keeps what no row draws — its
// count and its clock — and leaves the sentence to the row. The row is on the
// frame by construction, `chapter` having moved the panel onto it, but it is
// compared for, not assumed: a row the reply box covers is not on the frame
// (#108), and where nothing draws the sentence the note keeps its quote and
// the footer clips it to its room as before (#36).
func (m *Model) noteLeavesTheChapterQuoteToTheRow(note string) (string, bool) {
	if !m.chapterNote() {
		return note, false
	}
	if !strings.HasPrefix(note, glyphPrompt+" ") && !strings.HasPrefix(note, glyphSaid+" ") && !strings.HasPrefix(note, glyphBranch+" ") {
		return note, false
	}
	clauses := strings.Split(note, " · ")
	said, at := "", -1
	for i, c := range clauses {
		if len(c) > 1 && strings.HasPrefix(c, `"`) && strings.HasSuffix(c, `"`) {
			said, at = strings.Trim(c, `"`), i
		}
	}
	if at < 0 {
		return note, false
	}
	for _, row := range m.bodyRows {
		// saysSame is the deck's own compare (#110, #116): two words the
		// floor, and the sentence looked for anywhere in the row, which is
		// where it stands — behind the trail's glyph, behind the reader's.
		if saysSame(said, ansi.Strip(row)) {
			return strings.Join(append(clauses[:at:at], clauses[at+1:]...), " · "), true
		}
	}
	return note, false
}

func (m *Model) noteLeavesTheQuoteToTheRow(note string) (string, bool) {
	if !strings.HasPrefix(note, "↪ ") || !strings.Contains(note, `"`) {
		return note, false
	}
	i := strings.LastIndex(note, " · ")
	if i <= 0 || !strings.HasPrefix(note[i+len(" · "):], "to "+mirrorMark) {
		return note, false
	}
	dest := note[i+len(" · "):]
	head := note[:strings.Index(note, `"`)]
	head = strings.TrimSuffix(strings.TrimSpace(head), " ·")
	short := head + " " + dest
	if strings.HasPrefix(note, "↪ answered") {
		short = head + " · " + dest
	}
	target := strings.TrimPrefix(dest, "to ")
	for _, row := range m.bodyRows {
		for _, seg := range strings.Split(ansi.Strip(row), "│") {
			t := strings.TrimSpace(seg)
			if !strings.HasPrefix(t, "↪ ") {
				continue
			}
			rowDest := ""
			if k := strings.Index(t, mirrorMark); k > 0 {
				// The row's right-aligned pane clause is its own — except
				// where it is the very pane the note is pointing at.
				rowDest = strings.TrimSpace(t[k:])
				t = strings.TrimSpace(t[:k])
			}
			if k := strings.LastIndex(t, "  "); k > 0 && strings.Contains(t[:k], `"`) {
				// Where the fleet runs two tools the row's right-aligned
				// clause is the tool word, not a pane (#50): it is the
				// row's own either way, and the bytes are still drawn.
				t = strings.TrimSpace(t[:k])
			}
			for {
				j := strings.LastIndex(t, " · ")
				if j <= 0 || strings.Contains(t[j:], `"`) {
					break
				}
				// The row's own clock — and the count its divider draws
				// beneath it (#129) — are not the note's business. The
				// trim stops at the quote: a row that draws no bytes is
				// never trimmed into one that seems to (#131).
				t = t[:j]
			}
			// The row's own pane clause was just taken off, so the row
			// is compared to the note's sentence, not to the whole note.
			if saysSame(t, note[:i]) || saysSame(t, note) {
				if rowDest == target {
					// The row draws the destination too, in the note's own
					// form: the note has nothing left to point at (#128's
					// rule, #205's condition) and keeps only its verb.
					return head, true
				}
				return short, true
			}
		}
	}
	return note, false
}
