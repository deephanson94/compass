package ui

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
	"github.com/deephanson94/compass/internal/tmuxop"
)

// archivedSnap is the snapshot the Manager pins on every archived session
// (M5 contract, fleet rule 2): the archive can never be amber.
func archivedSnap(at time.Time) state.Snapshot {
	return state.Snapshot{State: state.Idle, Since: at, Reason: "archived", Activity: "idle"}
}

// fixtureGroupedFleet is the M5 fleet: five live sessions spread over two tmux
// sessions and one that tmux cannot account for, plus five archived ones across
// four projects. It is the same fixture for both views — the live fleet and the
// archive are two readings of one Refresh.
func fixtureGroupedFleet(base time.Time) []fleet.Session {
	live := func(id, cwd, branch, title string, class journey.Class, outcome string, snap state.Snapshot) fleet.Session {
		return fleet.Session{
			Info: fleet.SessionInfo{
				ID: id, TranscriptPath: sessionKey(id), ProjectSlug: "-home-user-" + id,
				CWD: cwd, GitBranch: branch, Title: title, StartedAt: base, LastEventAt: snap.Since,
			},
			Snap:  snap,
			Live:  true,
			Class: class, HasClass: true,
			Outcome: outcome,
		}
	}
	gone := func(id, cwd, branch, title string, at time.Time) fleet.Session {
		return fleet.Session{
			Info: fleet.SessionInfo{
				ID: id, TranscriptPath: sessionKey(id), ProjectSlug: "-home-user-" + id,
				CWD: cwd, GitBranch: branch, Title: title, StartedAt: at.Add(-time.Hour), LastEventAt: at,
			},
			Snap: archivedSnap(at),
		}
	}

	// Fleet order, as the Manager hands it over: needs-you longest-wait, stuck,
	// working, idle — then the archive, newest first.
	return []fleet.Session{
		live("s-infra", "/home/user/infra", "tf/vpc", "tighten the vpc security groups", journey.Design, "",
			state.Snapshot{State: state.NeedsYou, Since: base.Add(38 * time.Minute), Reason: "waiting on your answer", Activity: "AskUserQuestion"}),
		live("s-api", "/home/user/api", "claude/auth-fx", "fix the 401 bug", journey.Test, "1216✓ 2✗",
			state.Snapshot{State: state.Working, Since: base.Add(37 * time.Minute), Reason: "tool call in flight", Activity: "Bash: pytest tests/auth -x"}),
		live("s-webapp", "/home/user/webapp", "main", "flake in the checkout suite", journey.Test, "18✓ 2✗",
			state.Snapshot{State: state.Working, Since: base.Add(39*time.Minute + 20*time.Second), Reason: "tool call in flight", Activity: "tests 18✓ 2✗"}),
		live("s-tfstate", "/home/user/tfstate", "main", "reconcile the state file", journey.Build, "1190✓",
			state.Snapshot{State: state.Idle, Since: base.Add(25 * time.Minute), Reason: "turn complete", Activity: "idle"}),
		live("s-scratch", "/home/user/scratch", "main", "try the streaming api", journey.Scout, "",
			state.Snapshot{State: state.Idle, Since: base.Add(18 * time.Minute), Reason: "turn complete", Activity: "idle"}),

		gone("a-docs", "/home/user/docs", "main", "update the readme", base.Add(-time.Hour)),
		gone("a-infra", "/home/user/infra", "tf/dns", "move dns to route53", base.Add(-2*time.Hour)),
		gone("a-api", "/home/user/api", "claude/rate-limit", "add rate limiting", base.Add(-3*time.Hour)),
		gone("a-api-old", "/home/user/api", "main", "port the client to httpx", base.Add(-26*time.Hour)),
		gone("a-scratch", "/home/user/scratch", "main", "try the new sdk", base.Add(-50*time.Hour)),
	}
}

// fixtureGroupedPanes is what tmux says: two sessions, panes in index order, a
// pane with no claude in it (dev:9.0) and a whole tmux session compass has
// nothing in (misc). The list is the group order; the map is the pairing.
func fixtureGroupedPanes() (map[string]tmuxop.Pane, []tmuxop.Pane) {
	list := []tmuxop.Pane{
		{Target: "dev:1.0", ID: "%1", PID: 4242, Command: "claude", Window: "auth-fix"},
		{Target: "dev:2.1", ID: "%2", PID: 4243, Command: "claude", Window: "webapp"},
		{Target: "dev:9.0", ID: "%3", PID: 4244, Command: "nvim", Window: "notes"},
		{Target: "ops:0.0", ID: "%4", PID: 4245, Command: "claude", Window: "tf_state"},
		{Target: "ops:1.0", ID: "%5", PID: 4246, Command: "claude", Window: "vpc"},
		{Target: "misc:0.0", ID: "%6", PID: 4247, Command: "zsh", Window: "scratch"},
	}
	panes := map[string]tmuxop.Pane{
		sessionKey("s-api"):     list[0],
		sessionKey("s-webapp"):  list[1],
		sessionKey("s-tfstate"): list[3],
		sessionKey("s-infra"):   list[4],
	}
	return panes, list
}

// groupedModel is the M5 deck: the grouped fleet, the api session selected, its
// trail and its mirrored pane.
func groupedModel(w, h int) *Model {
	m := New(nil)
	m.SetSize(w, h)
	m.SetSessions(fixtureGroupedFleet(fixtureBase), fixtureBase.Add(40*time.Minute))
	panes, list := fixtureGroupedPanes()
	m.SetPanes(panes)
	m.SetPaneOrder(list)
	m.point(sessionKey("s-api"))
	m.SetTrail(fixtureTrail(fixtureBase))
	m.SetMirror(fixtureFrame)
	openTrail(m)
	return m
}

// press sends one key the way a terminal would.
func press(m *Model, key string) {
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
}

// pressEnter sends Enter, which no rune stands for.
func pressEnter(m *Model) {
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
}

// pressTab sends Tab, which deepens the zoom.
func pressTab(m *Model) {
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
}

// openTrail takes a deck that opened on the board (the default on a wide
// terminal, decision #16) to the single trail, the level the M1–M7 goldens
// and contracts were written against. A narrow deck is already there.
func openTrail(m *Model) {
	if m.level == levelBoard {
		pressTab(m)
	}
}

// pressCtrl sends a control key — ctrl+d and ctrl+u are no runes either.
func pressCtrl(m *Model, k tea.KeyType) {
	m.Update(tea.KeyMsg{Type: k})
}

// T58 — the live view at 120x30: two named tmux groups and `elsewhere`, the
// needs-you session floated to the top of ITS group (never out of it), the
// group's amber echo on its header, the location lines reduced to `:w.p`, and
// the archive advertising itself on the last row.
func TestT58LiveFleetGolden(t *testing.T) {
	forceASCII(t)

	m := groupedModel(120, 30)
	got := m.View()
	compareGolden(t, "fleet-live-120x30.txt", got)
	if *update {
		return
	}

	for _, want := range []string{
		" dev",                     // the tmux session, unnumbered and dim
		" ops",                     // …in the order the pane list names them
		" elsewhere",               // …and the live sessions tmux cannot place
		"◆ test   1216✓ 2✗",        // the result, in the trail's own words
		"◆ design AskUserQuestion", // the call in flight, when nothing has finished
		"◆ build  1190✓",           // and a quiet row still says how it went
		"5 archived · A browses",   // the last fleet row
		"FLEET · live",             // which fleet this is
	} {
		if !strings.Contains(got, want) {
			t.Errorf("live view is missing %q", want)
		}
	}
	if strings.Contains(got, " misc") {
		t.Error("a tmux session compass has nothing in must not become a group")
	}

	fleetCol := fleetText(m, 120, 30)
	infra := indexOfLine(fleetCol, "infra")
	tfstate := indexOfLine(fleetCol, "tfstate")
	ops := indexOfLine(fleetCol, " ops")
	elsewhere := indexOfLine(fleetCol, " elsewhere")
	if !(ops < infra && infra < tfstate && tfstate < elsewhere) {
		t.Errorf("needs-you must float to the top of its own group only: ops=%d infra=%d tfstate=%d elsewhere=%d",
			ops, infra, tfstate, elsewhere)
	}
	// The header's right edge is the age, in the column the session rows put
	// theirs; the echo sits just left of it. Both facts, neither displaced.
	opsHeader := strings.TrimRight(fleetCol[ops], " ")
	if !strings.HasSuffix(opsHeader, "2m") {
		t.Errorf("the ops header should end with its freshest age: %q", opsHeader)
	}
	if !strings.Contains(opsHeader, fleet.Glyph(state.NeedsYou)) {
		t.Errorf("the ops header should echo its needs-you session: %q", opsHeader)
	}
	if strings.Contains(fleetCol[indexOfLine(fleetCol, " dev")], fleet.Glyph(state.NeedsYou)) {
		t.Error("a group with nothing waiting must carry no echo")
	}
}

// T59 — `A` opens the archive: the same column, grouped by project, newest
// group first and newest first inside it. Toggling back restores the live
// selection, and toggling forward again restores the archive's.
func TestT59ArchiveViewGolden(t *testing.T) {
	forceASCII(t)

	m := groupedModel(120, 30)
	press(m, "A")
	got := m.View()
	compareGolden(t, "fleet-archive-120x30.txt", got)

	if !*update {
		for _, want := range []string{
			"FLEET · archive", // the header names the view
			"A live fleet",    // and the footer names the way back
			" docs",           // project groups, newest member first
			" api",
			" scratch",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("archive view is missing %q", want)
			}
		}
		if strings.Contains(got, "archived · A browses") {
			t.Error("the archive does not advertise itself to itself")
		}
	}

	fleetCol := fleetText(m, 120, 30)
	for _, pair := range [][2]string{
		{" docs", " infra"}, {" infra", " api"}, {" api", " scratch"}, // groups by newest member
		{"add rate", "port the"}, // newest first inside a group
	} {
		if a, b := indexOfLine(fleetCol, pair[0]), indexOfLine(fleetCol, pair[1]); a < 0 || b < 0 || a > b {
			t.Errorf("archive order: %q (%d) should come before %q (%d)", pair[0], a, pair[1], b)
		}
	}

	// The selection is remembered per view, and it is an id — so the trail, the
	// reader and everything else downstream simply follow it.
	if m.selectedKey != sessionKey("a-docs") {
		t.Errorf("the archive opens on its first entry, got %q", m.selectedKey)
	}
	press(m, "3") // third rendered archive entry: the api group's newest
	if m.selectedKey != sessionKey("a-api") {
		t.Errorf("3 selects the third rendered archive session, got %q", m.selectedKey)
	}
	press(m, "A")
	if m.archiveView {
		t.Fatal("A must return to the live fleet")
	}
	if m.selectedKey != sessionKey("s-api") {
		t.Errorf("returning to the live fleet must restore its selection, got %q", m.selectedKey)
	}
	press(m, "A")
	if !m.archiveView || m.selectedKey != sessionKey("a-api") {
		t.Errorf("the archive must restore its own selection, got archive=%v id=%q", m.archiveView, m.selectedKey)
	}

	// A fleet with no history behind it has nothing to browse, and says so
	// rather than opening an empty column.
	fresh := deckModel(120, 30, nil, "")
	press(fresh, "A")
	if fresh.archiveView {
		t.Error("A opened an empty archive")
	}
	if fresh.note == "" {
		t.Error("A on an empty archive should say why nothing happened")
	}
}

// T60 — the fleet scrolls in both views: walking the selection past the fold
// keeps its two rows whole on screen, the window is a true window onto the
// list, and headers travel with their groups.
func TestT60FleetScrolling(t *testing.T) {
	forceASCII(t)

	for _, tc := range []struct {
		name    string
		archive bool
		height  int
	}{
		{"live", false, 11},
		{"archive", true, 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := groupedModel(120, 30)
			if tc.archive {
				m.toggleArchive()
			}
			w, h := fleetWidth, tc.height

			rows := m.fleetRows()
			full, _, _ := m.fleetBlock(rows, w)
			if len(full) <= h {
				t.Fatalf("the fixture must be taller than the column: %d lines in %d", len(full), h)
			}
			assertHeadersLeadTheirGroups(t, m, rows, full)

			for step := 0; step < len(m.fleetOrder())+2; step++ {
				// The list is redrawn every frame: the selection marker moves with
				// the cursor, so the expectation is rebuilt with it.
				full, selStart, selEnd := m.fleetBlock(m.fleetRows(), w)
				lines := m.fleetLines(w, h)
				off := m.fleetScroll

				// The archive row is the column's last word, not part of the list.
				tail := 0
				if !m.archiveView && m.archivedCount() > 0 {
					tail = 2
					if !strings.Contains(lines[len(lines)-1], "archived · A browses") {
						t.Fatalf("step %d: the archive row was scrolled away: %q", step, lines[len(lines)-1])
					}
				}
				visible := len(lines) - tail
				if visible > h-tail || visible < 1 {
					t.Fatalf("step %d: column rendered %d list lines in a height of %d", step, visible, h)
				}
				for i := 0; i < visible; i++ {
					if lines[i] != full[off+i] {
						t.Fatalf("step %d line %d: window is not the list at offset %d\n got %q\nwant %q",
							step, i, off, lines[i], full[off+i])
					}
				}
				if selStart < off || selEnd >= off+visible {
					t.Fatalf("step %d: selected rows %d-%d fall outside the window %d-%d",
						step, selStart, selEnd, off, off+visible-1)
				}
				marked := 0
				for _, l := range lines[:visible] {
					if strings.HasPrefix(l, "▸") {
						marked++
					}
				}
				if marked != 1 {
					t.Fatalf("step %d: %d selection markers on screen, want 1", step, marked)
				}
				m.move(1)
			}
		})
	}
}

// assertHeadersLeadTheirGroups is the structural half of T60: every header sits
// immediately above its own group's first entry, with no air in between.
func assertHeadersLeadTheirGroups(t *testing.T, m *Model, rows []fleetRow, full []string) {
	t.Helper()
	line := 0
	for i, r := range rows {
		if i > 0 && !rows[i-1].header {
			line++ // the blank line of air between blocks
		}
		if r.header {
			if i+1 >= len(rows) || rows[i+1].header {
				t.Fatalf("group %q has no entries under it", r.label)
			}
			if full[line+1] == "" {
				t.Fatalf("group %q is separated from its entries", r.label)
			}
			line++
			continue
		}
		line += entryLines
	}
}

// fakeTmux answers ListPanes with a fixed screenful of panes.
type fakeTmux struct{ out string }

func (f fakeTmux) Output(args ...string) ([]byte, error) { return []byte(f.out), nil }

// fakeProc models the usual pane: a shell you typed `claude` into. The map is
// keyed by the pane's pid and holds the cwd its claude child reports; the child
// is that pid + 1000.
type fakeProc map[int]string

func (p fakeProc) Children(pid int) []int {
	if _, ok := p[pid]; ok {
		return []int{pid + 1000}
	}
	return nil
}

func (p fakeProc) Comm(pid int) string {
	if _, isPane := p[pid]; isPane {
		return "zsh" // the pane's own process is the shell
	}
	return "claude"
}

func (p fakeProc) Cmdline(pid int) string { return "-" + p.Comm(pid) }
func (p fakeProc) Cwd(pid int) string     { return p[pid-1000] }

// StartTime says nothing: these fixtures are about which pane holds which
// session, not about how long anything has been running.
func (p fakeProc) StartTime(int) time.Time { return time.Time{} }

// The first pane poll happens before the fleet exists — Init fires both at
// once — so it has nothing to pair. The fleet's arrival must re-pair, or every
// session reads "no pane" (and the mirror falls back to the transcript) until
// the 5s pane tick. Caught in a live tmux, not by a golden.
func TestFirstFleetRepairsPanes(t *testing.T) {
	m := New(nil)
	m.runner = fakeTmux{out: "dev:1.0\t%2\t22\tclaude\tapi\n"}
	m.proc = fakeProc{22: "/home/user/api"}

	// Init's poll: tmux has the pane, but the deck has no sessions yet.
	if msg := m.relistPanes()().(panesMsg); len(msg.panes) != 0 {
		t.Fatalf("a fleetless deck paired something: %+v", msg.panes)
	}

	sessions := []fleet.Session{{
		Info: fleet.SessionInfo{ID: "s-api", TranscriptPath: sessionKey("s-api"), CWD: "/home/user/api"},
		Live: true,
	}}
	_, cmd := m.Update(fleetMsg{sessions: sessions, at: fixtureBase})
	if cmd == nil {
		t.Fatal("the first fleet issued no follow-up command; the panes stay unpaired")
	}
	if !producesPanes(t, cmd, sessionKey("s-api"), "dev:1.0") {
		t.Error("the first fleet did not re-list panes; sessions read \"no pane\" until the pane tick")
	}

	// A later fleet is not a first one: the pane tick owns the cadence from here.
	_, cmd = m.Update(fleetMsg{sessions: sessions, at: fixtureBase.Add(time.Second)})
	if cmd != nil && producesPanes(t, cmd, sessionKey("s-api"), "dev:1.0") {
		t.Error("every fleet refresh re-lists panes; that is the pane tick's job, once per 5s")
	}
}

// producesPanes runs cmd (and any batch it stands for) and reports whether a
// panesMsg came back pairing key with target.
func producesPanes(t *testing.T, cmd tea.Cmd, key, target string) bool {
	t.Helper()
	for _, msg := range drain(cmd) {
		if p, ok := msg.(panesMsg); ok && p.panes[key].Target == target {
			return true
		}
	}
	return false
}

// drain flattens a command into the messages it yields, one level of
// tea.Batch deep — which is as deep as the deck ever nests them.
func drain(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, drain(c)...)
		}
		return out
	case nil:
		return nil
	default:
		return []tea.Msg{msg}
	}
}

// T58 — the pane poll carries both shapes of the truth: the map the deck looks
// panes up in, and tmux's own ordering the live view groups by. It runs with no
// Manager at all (a harness), which must not panic where MarkPaneMapped is fed.
func TestT58PanesMsgCarriesTmuxOrder(t *testing.T) {
	m := New(nil) // no Manager: MarkPaneMapped has nobody to tell
	m.runner = fakeTmux{out: "ops:0.0\t%1\t11\tclaude\tops\n" +
		"dev:1.0\t%2\t22\tclaude\tapi\n"}
	m.proc = fakeProc{11: "/home/user/ops", 22: "/home/user/api"}
	m.SetSessions([]fleet.Session{
		{Info: fleet.SessionInfo{ID: "s-api", TranscriptPath: sessionKey("s-api"), CWD: "/home/user/api"}, Live: true},
		{Info: fleet.SessionInfo{ID: "s-ops", TranscriptPath: sessionKey("s-ops"), CWD: "/home/user/ops"}, Live: true},
	}, fixtureBase)

	msg, ok := m.relistPanes()().(panesMsg)
	if !ok {
		t.Fatal("relistPanes did not produce a panesMsg")
	}
	if len(msg.list) != 2 || msg.list[0].Target != "ops:0.0" || msg.list[1].Target != "dev:1.0" {
		t.Fatalf("the ordered pane list did not survive the poll: %+v", msg.list)
	}
	if msg.panes[sessionKey("s-api")].Target != "dev:1.0" || msg.panes[sessionKey("s-ops")].Target != "ops:0.0" {
		t.Fatalf("the pairing did not survive the poll: %+v", msg.panes)
	}

	m.Update(msg)
	rows := m.fleetRows()
	if len(rows) < 2 || !rows[0].header || rows[0].label != "ops" {
		t.Errorf("the live view should group in pane-list order, got %+v", rows)
	}
}

// T61 — group order is the pane list's first-occurrence order, not the fleet's
// and not the alphabet; inside a group the order is numeric window.pane, so
// window 10 follows window 9 rather than window 1.
func TestT61GroupAndPaneOrder(t *testing.T) {
	forceASCII(t)

	base := fixtureBase
	sess := func(id, cwd string, at time.Time) fleet.Session {
		return fleet.Session{
			Info: fleet.SessionInfo{ID: id, TranscriptPath: sessionKey(id), CWD: cwd, GitBranch: "main", LastEventAt: at},
			Snap: state.Snapshot{State: state.Idle, Since: at, Reason: "turn complete"},
			Live: true,
		}
	}
	m := New(nil)
	m.SetSize(120, 30)
	// Fleet order is w10, w9, ops — none of which is the rendered order.
	m.SetSessions([]fleet.Session{
		sess("s-w10", "/home/user/ten", base),
		sess("s-w9", "/home/user/nine", base),
		sess("s-ops", "/home/user/ops", base),
	}, base)

	// tmux mentions ops first, then dev — and dev's window 10 sits after its 9.
	list := []tmuxop.Pane{
		{Target: "ops:0.0", ID: "%1", PID: 1, Command: "claude"},
		{Target: "dev:9.0", ID: "%2", PID: 2, Command: "claude"},
		{Target: "dev:10.0", ID: "%3", PID: 3, Command: "claude"},
	}
	m.SetPaneOrder(list)
	m.SetPanes(map[string]tmuxop.Pane{sessionKey("s-ops"): list[0], sessionKey("s-w9"): list[1], sessionKey("s-w10"): list[2]})

	rows := m.fleetRows()
	var got []string
	for _, r := range rows {
		if r.header {
			got = append(got, "["+r.label+"]")
			continue
		}
		got = append(got, m.sessions[r.sess].Info.ID)
	}
	want := []string{"[ops]", "s-ops", "[dev]", "s-w9", "s-w10"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("rendered order = %v, want %v", got, want)
	}

	// Numbering runs flat down the sessions, groups ignored.
	nums := map[string]int{}
	for _, r := range rows {
		if !r.header {
			nums[m.sessions[r.sess].Info.ID] = r.num
		}
	}
	for id, want := range map[string]int{"s-ops": 1, "s-w9": 2, "s-w10": 3} {
		if nums[id] != want {
			t.Errorf("%s is numbered %d, want %d", id, nums[id], want)
		}
	}
	m.selectIndex(2) // the third rendered session
	if m.selectedKey != sessionKey("s-w10") {
		t.Errorf("3 selected %q, want s-w10", m.selectedKey)
	}
}

// T64 — two sessions sharing one id are two sessions. The deck keys identity by
// the transcript path (M6 contract), so the twin draws its own row, borrows
// nobody's pane, and selecting one leaves the other standing where it was.
func TestT64DuplicateIDsAreTwoSessions(t *testing.T) {
	forceASCII(t)

	// The dogfood machine's own case: one id, two transcripts under two project
	// slugs, because the session changed directory and kept writing.
	twin := func(path, cwd, slug string, at time.Time) fleet.Session {
		return fleet.Session{
			Info: fleet.SessionInfo{
				ID: "5f0cd1ed", TranscriptPath: path, ProjectSlug: slug,
				CWD: cwd, GitBranch: "main", Title: "one id, two journeys",
				StartedAt: fixtureBase, LastEventAt: at,
			},
			Snap: state.Snapshot{State: state.Idle, Since: at, Reason: "turn complete", Activity: "idle"},
			Live: true,
		}
	}
	alpha := twin("/x/alpha.jsonl", "/home/user/alpha", "-home-user-alpha", fixtureBase.Add(30*time.Minute))
	beta := twin("/x/beta.jsonl", "/home/user/beta", "-home-user-beta", fixtureBase.Add(20*time.Minute))
	if alpha.Info.Key() == beta.Info.Key() {
		t.Fatal("the fixture must be two keys under one id")
	}
	pane := tmuxop.Pane{Target: "dev:1.0", ID: "%1", PID: 7, Command: "claude"}

	m := New(nil)
	m.SetSize(120, 30)
	m.SetSessions([]fleet.Session{alpha, beta}, fixtureBase.Add(40*time.Minute))
	m.SetPanes(map[string]tmuxop.Pane{alpha.Info.Key(): pane})
	m.SetPaneOrder([]tmuxop.Pane{pane})
	m.point(alpha.Info.Key())

	col := fleetText(m, 120, 30)
	if got := countLines(col, "▸"); got != 1 {
		t.Errorf("%d selection markers on screen, want exactly 1", got)
	}
	// The grouping is what says which twin tmux can place: the one holding the
	// pane sits under its tmux session, the other under `elsewhere`.
	if got := countLines(col, " dev"); got != 1 {
		t.Errorf("%d rows sit under the pane's tmux session, want exactly 1", got)
	}
	if got := countLines(col, "elsewhere"); got != 1 {
		t.Errorf("%d twins were placed nowhere, want exactly 1", got)
	}
	if row := markedRow(col); !strings.Contains(row, "alpha") {
		t.Errorf("the marker sits on %q, want the alpha twin", row)
	}

	// Selecting the twin moves everything the selection owns — and only it.
	m.point(beta.Info.Key())
	if s, _ := m.selected(); s.Info.CWD != beta.Info.CWD {
		t.Errorf("selected %q, want the beta twin", s.Info.CWD)
	}
	if _, ok := m.selectedPane(); ok {
		t.Error("the paneless twin borrowed its sibling's pane")
	}
	col = fleetText(m, 120, 30)
	if got := countLines(col, "▸"); got != 1 {
		t.Errorf("%d selection markers after moving, want exactly 1", got)
	}
	if row := markedRow(col); !strings.Contains(row, "beta") {
		t.Errorf("the marker sits on %q, want the beta twin", row)
	}
}

// T67 — Enter hands the terminal to the session (M6 contract). A mapped session
// builds the attach command; an unmapped one gets the note it has always had
// and no command; -readonly refuses, because compass issues no tmux command
// that changes state.
func TestT67EnterAttaches(t *testing.T) {
	// spawned is what the deck was about to run. Standing in for both ways a
	// command reaches the world lets the test read exactly what Enter built,
	// with no tmux server in sight.
	type spawned struct {
		cmd    *exec.Cmd
		inside bool
	}
	record := func(m *Model) **spawned {
		got := new(*spawned)
		m.spawn = func(cmd *exec.Cmd, inside bool, done func(error) tea.Msg) tea.Cmd {
			*got = &spawned{cmd: cmd, inside: inside}
			return nil
		}
		return got
	}

	t.Run("mapped", func(t *testing.T) {
		m := groupedModel(120, 30)
		got := record(m)
		pressEnter(m)

		if *got == nil {
			t.Fatal("enter on a mapped session built no command")
		}
		args := strings.Join((*got).cmd.Args, " ")
		for _, want := range []string{"tmux", "select-window", "dev:1", "select-pane", "%1", "attach-session"} {
			if !strings.Contains(args, want) {
				t.Errorf("the attach command is missing %q: %v", want, (*got).cmd.Args)
			}
		}
		if (*got).inside {
			t.Error("outside tmux the deck suspends itself; it does not switch a client")
		}
		if m.note != "" {
			t.Errorf("an attach that worked says nothing: %q", m.note)
		}
	})

	t.Run("inside tmux", func(t *testing.T) {
		m := groupedModel(120, 30)
		m.inTmux = true
		got := record(m)
		pressEnter(m)

		if *got == nil {
			t.Fatal("enter inside tmux built no command")
		}
		args := strings.Join((*got).cmd.Args, " ")
		if !strings.Contains(args, "switch-client") || strings.Contains(args, "attach-session") {
			t.Errorf("inside tmux the client switches: %v", (*got).cmd.Args)
		}
		if !(*got).inside {
			t.Error("inside tmux there is nothing to suspend")
		}
		// Nothing was suspended, so the deck says where the client went.
		m.Update(attachDoneMsg{target: "dev:1.0", inside: true})
		if m.note != "switched to dev:1.0" {
			t.Errorf("note = %q, want the switch named", m.note)
		}
	})

	t.Run("unmapped", func(t *testing.T) {
		m := groupedModel(120, 30)
		got := record(m)
		m.point(sessionKey("s-scratch")) // live, and in no pane at all
		pressEnter(m)

		if *got != nil {
			t.Fatalf("a session with no pane must build no command: %v", (*got).cmd.Args)
		}
		if m.note != "no tmux pane for this session" {
			t.Errorf("note = %q, want the no-pane note", m.note)
		}
	})

	t.Run("readonly", func(t *testing.T) {
		m := groupedModel(120, 30)
		m.readonly = true
		got := record(m)
		pressEnter(m)

		if *got != nil {
			t.Fatalf("-readonly must issue no tmux command: %v", (*got).cmd.Args)
		}
		if !strings.Contains(m.note, "read-only") {
			t.Errorf("note = %q, want the read-only refusal", m.note)
		}
	})

	t.Run("g grabs and attaches", func(t *testing.T) {
		m := groupedModel(120, 30)
		got := record(m)
		press(m, "g")

		if m.selectedKey != sessionKey("s-infra") {
			t.Fatalf("g selected %q, want the session waiting longest", m.selectedKey)
		}
		if *got == nil {
			t.Fatal("g grabbed the session but did not attach to it")
		}
		if args := strings.Join((*got).cmd.Args, " "); !strings.Contains(args, "%5") {
			t.Errorf("g attached to %v, want the infra pane", (*got).cmd.Args)
		}
	})
}

// countLines is how many of lines contain sub.
func countLines(lines []string, sub string) int {
	n := 0
	for _, l := range lines {
		if strings.Contains(l, sub) {
			n++
		}
	}
	return n
}

// markedRow is the line carrying the selection marker, or "".
func markedRow(lines []string) string {
	for _, l := range lines {
		if strings.HasPrefix(l, "▸") {
			return l
		}
	}
	return ""
}

// fleetText renders just the fleet column, unpadded, for order assertions.
func fleetText(m *Model, w, h int) []string {
	return m.fleetColumn(fleetWidth, h-5)
}

// indexOfLine is the first line containing sub, or -1.
func indexOfLine(lines []string, sub string) int {
	for i, l := range lines {
		if strings.Contains(l, sub) {
			return i
		}
	}
	return -1
}

// A trail scrolled off the present says so in its title, and names the key
// back: without it, a scrolled panel is indistinguishable from a short journey
// (M7 contract, scrolling).
func TestScrolledTrailSaysItIsBehind(t *testing.T) {
	forceASCII(t)

	m := followModel(120, 14)
	if strings.Contains(m.trailTitle(30), "G") {
		t.Error("a pinned trail must not advertise the way back to where it already is")
	}

	pressCtrl(m, tea.KeyCtrlU)
	if m.trailPinned {
		t.Fatal("ctrl+u did not unpin the panel; nothing to announce")
	}
	title := m.trailTitle(30)
	for _, want := range []string{"G", "[Lv1]"} {
		if !strings.Contains(title, want) {
			t.Errorf("a scrolled trail's title %q is missing %q", title, want)
		}
	}
	if got := lipgloss.Width(title); got != 30 {
		t.Errorf("the cue broke the title's width: %d columns, want 30", got)
	}

	press(m, "G")
	if strings.Contains(m.trailTitle(30), "G") {
		t.Error("G re-pinned the trail; the cue must go with it")
	}
}

// The Lv1 trail scrolls without ever becoming the object: ctrl+d and ctrl+u
// move the panel, G re-pins it to the present, and j/k still walk the fleet
// (M7 contract, scrolling).
func TestLv1ScrollsTheTrail(t *testing.T) {
	forceASCII(t)

	m := followModel(120, 14)
	w, h := m.trailBox()
	if len(TrailLines(m.trail, m.trailOpts(w, h))) <= h {
		t.Fatalf("the fixture trail fits its panel (%d rows); nothing to scroll", h)
	}
	if !m.trailPinned {
		t.Fatal("a fresh deck must be pinned to the newest row")
	}

	_, _, floor := m.trailView() // the offset a pinned panel is showing
	pressCtrl(m, tea.KeyCtrlU)
	if m.trailPinned {
		t.Error("scrolling up must unpin the panel")
	}
	if m.trailScroll >= floor {
		t.Errorf("ctrl+u left the panel at %d, want above the floor at %d", m.trailScroll, floor)
	}

	pressCtrl(m, tea.KeyCtrlD)
	if !m.trailPinned {
		t.Errorf("scrolling back to the bottom must re-pin: offset %d", m.trailScroll)
	}

	pressCtrl(m, tea.KeyCtrlU)
	press(m, "G")
	if !m.trailPinned {
		t.Error("G must re-pin the trail to the newest row")
	}

	// j and k are the fleet's at Lv1: the trail is not the object there.
	key := m.selectedKey
	press(m, "j")
	if m.selectedKey == key {
		t.Error("j at Lv1 must walk the fleet, not the trail")
	}
	if m.level != levelTrail || m.cursor != -1 {
		t.Errorf("Lv1 grew a trail cursor: level %d, cursor %d", m.level, m.cursor)
	}
}

// T77 — Enter means one thing (M7 contract): at the trail, at the waypoints and
// inside the reader it hands the terminal to the session, and it does nothing
// else — the level it was pressed at is the level it leaves behind.
func TestT77EnterAttachesAtEveryLevel(t *testing.T) {
	forceASCII(t)

	for _, tc := range []struct {
		name  string
		tabs  int
		level int
	}{
		{"Lv0", -1, levelBoard},
		{"Lv1", 0, levelTrail},
		{"Lv2", 1, levelWaypoints},
		{"Lv3", 2, levelReader},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := groupedModel(120, 30)
			var got *exec.Cmd
			m.spawn = func(cmd *exec.Cmd, inside bool, done func(error) tea.Msg) tea.Cmd {
				got = cmd
				return nil
			}
			if tc.tabs < 0 {
				m.zoomOut() // back to the board the deck opened on
			}
			for i := 0; i < tc.tabs; i++ {
				pressTab(m)
			}
			if m.level != tc.level {
				t.Fatalf("%d tabs left the deck at Lv%d", tc.tabs, m.level)
			}
			cursor := m.cursor

			pressEnter(m)
			if got == nil {
				t.Fatalf("enter at %s built no attach command (note %q)", tc.name, m.note)
			}
			args := strings.Join(got.Args, " ")
			for _, want := range []string{"tmux", "select-pane", "%1", "attach-session"} {
				if !strings.Contains(args, want) {
					t.Errorf("the %s attach command is missing %q: %v", tc.name, want, got.Args)
				}
			}
			if m.level != tc.level {
				t.Errorf("enter zoomed the deck to Lv%d; enter attaches, tab deepens", m.level)
			}
			if m.cursor != cursor {
				t.Errorf("enter moved the trail cursor to %d, want %d", m.cursor, cursor)
			}
			if m.note != "" {
				t.Errorf("an attach that worked says nothing: %q", m.note)
			}
		})
	}

	// The footer says so at every depth, and says it whole: the keymap is
	// clipped to the deck's inner width, and eighty columns is the floor.
	t.Run("the footers promise it", func(t *testing.T) {
		m := groupedModel(80, 24)
		for _, want := range []string{
			"j/k move · enter attach (prefix d returns) · g grab · ? help · q quit",
			"j/k rows · enter attach · tab deeper · a ask · esc back",
			"j/k scroll · space fold · / search · n/N · a ask · enter attach · esc back",
		} {
			if got := m.View(); !strings.Contains(got, want) {
				t.Errorf("the Lv%d footer does not fit an 80-column deck: %q", m.level, want)
			}
			pressTab(m)
		}
	})
}

// A fleet row says what a session is doing, in the same words the trail uses.
// It used to spend its second line on ":0.0 claude · HEAD" — the tmux address
// and a branch reading HEAD — which answered a question nobody asks: the
// address is what `enter` spends, not what a reader does, and the selected
// session's is in the mirror's own header.
func TestFleetRowSaysWhatTheSessionIsDoing(t *testing.T) {
	forceASCII(t)

	m := New(nil)
	m.SetSize(120, 30)
	m.SetSessions(fixtureGroupedFleet(fixtureBase), fixtureBase.Add(40*time.Minute))
	panes, list := fixtureGroupedPanes()
	m.SetPanes(panes)
	m.SetPaneOrder(list)
	col := strings.Join(fleetText(m, 120, 30), "\n")

	for _, want := range []string{
		"● api ",            // the glyph is the state; the word would repeat it
		"◆ test   1216✓ 2✗", // the second line carries the result
		"▲ infra",           // …but the two states worth catching keep their words
		"needs you",         //
		"◆ build  1190✓",    // a quiet row still says how its last run went
	} {
		if !strings.Contains(col, want) {
			t.Errorf("fleet is missing %q:\n%s", want, col)
		}
	}
	// The address is gone from every row, and so is the state word on the two
	// states the glyph already tells you about.
	if strings.Contains(col, ":1.0") || strings.Contains(col, "no pane") {
		t.Errorf("a fleet row is still spending a line on the tmux address:\n%s", col)
	}
	for _, spelled := range []string{"working", "idle"} {
		if strings.Contains(col, spelled) {
			t.Errorf("a row spells %q, which its glyph already said:\n%s", spelled, col)
		}
	}
}

// A session compass has not classified yet still says something.
func TestFleetRowWithoutAClass(t *testing.T) {
	forceASCII(t)

	s := fixtureGroupedFleet(fixtureBase)[0]
	s.HasClass = false
	m := New(nil)
	m.SetSize(120, 30)
	m.SetSessions([]fleet.Session{s}, fixtureBase.Add(40*time.Minute))
	col := strings.Join(fleetText(m, 120, 30), "\n")
	if !strings.Contains(col, "AskUserQuestion") {
		t.Errorf("an unclassified row lost its activity:\n%s", col)
	}
	if strings.Contains(col, "◆ ") {
		t.Errorf("an unclassified row invented a class:\n%s", col)
	}
}

// The side panels are what only compass draws; the mirror is a rendering of a
// pane the user can look at directly. Past the mirror's floor the surplus goes
// to the sides, and no further than their caps.
func TestWideTerminalsFeedTheSidePanels(t *testing.T) {
	narrowF, narrowT := sidePanelWidths(118)
	if narrowF != fleetWidth || narrowT != trailWidth {
		t.Errorf("at 118 the panels are %d/%d, want the floors %d/%d",
			narrowF, narrowT, fleetWidth, trailWidth)
	}

	last := 0
	for w := 118; w <= 260; w += 2 {
		f, tr := sidePanelWidths(w)
		mirror := w - f - tr - 2*gutterWidth
		if f < fleetWidth || tr < trailWidth {
			t.Fatalf("at %d a panel fell below its floor: %d/%d", w, f, tr)
		}
		if f > fleetWidthMax || tr > trailWidthMax {
			t.Fatalf("at %d a panel passed its cap: %d/%d", w, f, tr)
		}
		if mirror < mirrorEnough && (f > fleetWidth || tr > trailWidth) {
			t.Fatalf("at %d the panels grew while the mirror had only %d", w, mirror)
		}
		if f+tr < last {
			t.Fatalf("at %d the panels shrank as the terminal grew", w)
		}
		last = f + tr
	}

	if f, tr := sidePanelWidths(200); f != fleetWidthMax || tr != trailWidthMax {
		t.Errorf("at 200 the panels are %d/%d, want both capped at %d/%d",
			f, tr, fleetWidthMax, trailWidthMax)
	}
}

// Below the two-column floor the trail is the one panel worth keeping: the only
// reason to run compass this narrow is to sit it beside a CLI in your own tmux,
// and beside a CLI the trail is the half that is not already on screen.
func TestANarrowPaneKeepsTheTrail(t *testing.T) {
	forceASCII(t)

	m := New(nil)
	m.SetSize(46, 24)
	m.SetSessions(fixtureGroupedFleet(fixtureBase), fixtureBase.Add(40*time.Minute))
	m.point(sessionKey("s-api"))
	m.SetTrail(fixtureTrail(fixtureBase))

	got := m.View()
	if !strings.Contains(got, "TRAIL · api") {
		t.Errorf("a 46-column pane dropped the trail:\n%s", got)
	}
	if strings.Contains(got, "FLEET · live") {
		t.Errorf("a 46-column pane kept the fleet instead:\n%s", got)
	}
	// The alarm is not lost with it: the header still counts the fleet.
	if !strings.Contains(got, "▲") {
		t.Errorf("the header stopped carrying the fleet's alarm:\n%s", got)
	}
}

// And the deck actually spends a wide terminal on the panels, rather than
// computing widths it does not use: at 200 columns both the fleet and the
// trail are wider than at 118, measured off the rendered frame. Two columns
// by default (decision #15); the mirror's three are measured when `m` asks.
func TestAWideDeckDrawsWiderSidePanels(t *testing.T) {
	forceASCII(t)

	build := func(w int) *Model {
		m := New(nil)
		m.SetSize(w, 30)
		m.SetSessions(fixtureGroupedFleet(fixtureBase), fixtureBase.Add(40*time.Minute))
		panes, list := fixtureGroupedPanes()
		m.SetPanes(panes)
		m.SetPaneOrder(list)
		m.point(sessionKey("s-api"))
		m.SetTrail(fixtureTrail(fixtureBase))
		openTrail(m)
		return m
	}

	// 90 rather than 118: by 118 the two-column fleet is already at its cap.
	narrow := columnWidths(t, build(90).View())
	wide := columnWidths(t, build(200).View())
	if len(narrow) != 2 || len(wide) != 2 {
		t.Fatalf("expected two columns, got %v and %v", narrow, wide)
	}
	if wide[0] <= narrow[0] {
		t.Errorf("the fleet stayed at %d columns on a 200-column terminal", wide[0])
	}
	if wide[1] <= narrow[1] {
		t.Errorf("the trail stayed at %d columns on a 200-column terminal", wide[1])
	}

	// With the mirror on, the middle takes the growth first and the sides
	// still grow past their floors.
	withMirror := func(w int) *Model { m := build(w); press(m, "m"); return m }
	narrow3 := columnWidths(t, withMirror(118).View())
	wide3 := columnWidths(t, withMirror(200).View())
	if len(narrow3) != 3 || len(wide3) != 3 {
		t.Fatalf("with the mirror on, expected three columns, got %v and %v", narrow3, wide3)
	}
	if wide3[0] <= narrow3[0] || wide3[2] <= narrow3[2] {
		t.Errorf("the side panels did not grow with the mirror on: %v → %v", narrow3, wide3)
	}
}

// columnWidths measures a rendered deck's columns from the hairlines between
// them — what is actually on screen, not what the arithmetic intended.
func columnWidths(t *testing.T, frame string) []int {
	t.Helper()
	for _, line := range strings.Split(frame, "\n") {
		if !strings.Contains(line, "FLEET") {
			continue
		}
		var out []int
		for _, part := range strings.Split(line, "│") {
			out = append(out, lipgloss.Width(part))
		}
		return out
	}
	t.Fatal("no fleet header in the frame")
	return nil
}

// A session the API refused still carries whatever it last finished. That
// result is true and useless: it describes work that landed before the wall
// went up. Showing it puts a green tick on a row whose session is dead until
// someone logs in again — which is the row this whole feature exists to fix.
func TestTheFleetShowsTheAPIErrorRatherThanTheStaleOutcome(t *testing.T) {
	forceASCII(t)

	base := fixtureBase
	blocked := fleet.Session{
		Info: fleet.SessionInfo{
			ID: "s-porter", TranscriptPath: sessionKey("s-porter"), ProjectSlug: "-home-user-porter",
			CWD: "/home/user/porter", GitBranch: "main", Title: "port the client",
			StartedAt: base, LastEventAt: base.Add(30 * time.Minute),
		},
		Snap: state.Snapshot{
			State: state.NeedsYou, Since: base.Add(30 * time.Minute), APIError: true,
			Reason: "api error 403 · authentication_failed",
			// What the machine hands over: its own "API Error: 403" marker
			// already spent, so the gateway's words start as early as they can.
			Activity: "Please run /login · your daily quota is exhausted",
		},
		Live:  true,
		Class: journey.Test, HasClass: true,
		// The session ran a green suite an hour before the quota ran out.
		Outcome: "1216✓",
	}

	m := New(nil)
	m.SetSize(120, 24)
	m.SetSessions([]fleet.Session{blocked}, base.Add(40*time.Minute))
	openTrail(m)
	press(m, "m") // the mirror's no-pane fallback is where the words are read

	got := m.View()
	if !strings.Contains(got, "api error 403") {
		t.Errorf("the fleet row does not say why the session is blocked:\n%s", got)
	}
	// The words a person recognises are too long for a fleet row but must
	// still reach the panel beside it, whole.
	if !strings.Contains(got, "daily quota") {
		t.Errorf("the error's own words never reach the reader:\n%s", got)
	}
	if strings.Contains(got, "1216") {
		t.Errorf("the fleet row still shows the result from before the wall went up:\n%s", got)
	}
	if !strings.Contains(got, "needs you") {
		t.Errorf("the row does not read needs-you:\n%s", got)
	}
}

// Inside a group the rows are in window.pane order, so the list mirrors the
// screen `enter` takes you to. That order has nothing to do with time: a fleet
// of nine dormant sessions reads 2d, 9h, 4d, 9h, 7d — and there is no way to
// find the one you touched this morning. The header carries the freshest, so
// the groups can be scanned by recency without reordering anything inside them.
func TestAGroupHeaderCarriesItsFreshestAge(t *testing.T) {
	forceASCII(t)

	base := fixtureBase
	now := base.Add(8 * 24 * time.Hour)
	sess := func(id string, at time.Time) fleet.Session {
		return fleet.Session{
			Info: fleet.SessionInfo{ID: id, TranscriptPath: sessionKey(id), CWD: "/home/user/" + id,
				GitBranch: "main", LastEventAt: at},
			Snap: state.Snapshot{State: state.Idle, Since: at, Reason: "turn complete"},
			Live: true,
		}
	}
	m := New(nil)
	m.SetSize(120, 30)
	// Pane order puts the stalest first, exactly as tmux would.
	m.SetSessions([]fleet.Session{
		sess("s-old", now.Add(-7*24*time.Hour)),
		sess("s-new", now.Add(-9*time.Hour)),
	}, now)
	list := []tmuxop.Pane{
		{Target: "tinker:0.0", ID: "%1", PID: 1, Command: "claude"},
		{Target: "tinker:1.0", ID: "%2", PID: 2, Command: "claude"},
	}
	m.SetPaneOrder(list)
	m.SetPanes(map[string]tmuxop.Pane{sessionKey("s-old"): list[0], sessionKey("s-new"): list[1]})
	openTrail(m) // the fleet list is a Lv1 panel; the board does not draw it

	var header fleetRow
	for _, r := range m.fleetRows() {
		if r.header {
			header = r
			break
		}
	}
	if header.label != "tinker" {
		t.Fatalf("first header is %q, want the tmux session", header.label)
	}
	if want := "9h"; header.age != want {
		t.Errorf("header age = %q, want %q — the freshest in the group, not the first pane's", header.age, want)
	}

	// And it reaches the frame, in the column the session rows put their own
	// age in, so the two read as one column rather than two.
	frame := m.View()
	var headerLine string
	for _, line := range strings.Split(frame, "\n") {
		// The fleet column only — the mirror's own header names the pane too.
		col, _, _ := strings.Cut(line, "│")
		if strings.Contains(col, "tinker") {
			headerLine = col
			break
		}
	}
	if !strings.Contains(headerLine, "9h") {
		t.Errorf("the rendered header does not carry the age:\n%q", headerLine)
	}
	if strings.Contains(headerLine, "7d") {
		t.Errorf("the header carries the stalest age instead of the freshest:\n%q", headerLine)
	}
}

// An idle session with nothing finished and nothing asked used to fall through
// to its state reason — "turn complete", "no activity yet" — which is what the
// ○ beside its name already said.
func TestAnIdleRowDoesNotRepeatItsStateAsASecondLine(t *testing.T) {
	forceASCII(t)
	base := fixtureBase
	quiet := fleet.Session{
		Info: fleet.SessionInfo{ID: "s-deep", TranscriptPath: sessionKey("s-deep"), CWD: "/home/user/deep",
			GitBranch: "main", LastEventAt: base},
		Snap:  state.Snapshot{State: state.Idle, Since: base, Reason: "turn complete", Activity: "idle"},
		Live:  true,
		Class: journey.Scout, HasClass: true,
	}
	m := New(nil)
	m.SetSize(120, 24)
	m.SetSessions([]fleet.Session{quiet}, base.Add(6*24*time.Hour))
	col := strings.Join(fleetText(m, 120, 24), "\n")
	if strings.Contains(col, "turn complete") {
		t.Errorf("the idle row repeats its state:\n%s", col)
	}

	// The reason still earns its line when the state is one you must not miss.
	quiet.Snap = state.Snapshot{State: state.NeedsYou, Since: base, Reason: "waiting on your answer"}
	m.SetSessions([]fleet.Session{quiet}, base.Add(time.Hour))
	// Clipped to the column, but there.
	if col := strings.Join(fleetText(m, 120, 24), "\n"); !strings.Contains(col, "waiting on your") {
		t.Errorf("the needs-you row lost its reason:\n%s", col)
	}
}
