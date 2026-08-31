package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/deephanson94/compass/internal/fleet"
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
	live := func(id, cwd, branch, title string, snap state.Snapshot) fleet.Session {
		return fleet.Session{
			Info: fleet.SessionInfo{
				ID: id, TranscriptPath: "/x/" + id + ".jsonl", ProjectSlug: "-home-user-" + id,
				CWD: cwd, GitBranch: branch, Title: title, StartedAt: base, LastEventAt: snap.Since,
			},
			Snap: snap,
			Live: true,
		}
	}
	gone := func(id, cwd, branch, title string, at time.Time) fleet.Session {
		return fleet.Session{
			Info: fleet.SessionInfo{
				ID: id, TranscriptPath: "/x/" + id + ".jsonl", ProjectSlug: "-home-user-" + id,
				CWD: cwd, GitBranch: branch, Title: title, StartedAt: at.Add(-time.Hour), LastEventAt: at,
			},
			Snap: archivedSnap(at),
		}
	}

	// Fleet order, as the Manager hands it over: needs-you longest-wait, stuck,
	// working, idle — then the archive, newest first.
	return []fleet.Session{
		live("s-infra", "/home/user/infra", "tf/vpc", "tighten the vpc security groups",
			state.Snapshot{State: state.NeedsYou, Since: base.Add(38 * time.Minute), Reason: "waiting on your answer", Activity: "AskUserQuestion"}),
		live("s-api", "/home/user/api", "claude/auth-fx", "fix the 401 bug",
			state.Snapshot{State: state.Working, Since: base.Add(37 * time.Minute), Reason: "tool call in flight", Activity: "Bash: pytest tests/auth -x"}),
		live("s-webapp", "/home/user/webapp", "main", "flake in the checkout suite",
			state.Snapshot{State: state.Working, Since: base.Add(39*time.Minute + 20*time.Second), Reason: "tool call in flight", Activity: "tests 18✓ 2✗"}),
		live("s-tfstate", "/home/user/tfstate", "main", "reconcile the state file",
			state.Snapshot{State: state.Idle, Since: base.Add(25 * time.Minute), Reason: "turn complete", Activity: "idle"}),
		live("s-scratch", "/home/user/scratch", "main", "try the streaming api",
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
		{Target: "dev:1.0", ID: "%1", PID: 4242, Path: "/home/user/api", Command: "claude"},
		{Target: "dev:2.1", ID: "%2", PID: 4243, Path: "/home/user/webapp", Command: "claude"},
		{Target: "dev:9.0", ID: "%3", PID: 4244, Path: "/home/user/api", Command: "nvim"},
		{Target: "ops:0.0", ID: "%4", PID: 4245, Path: "/home/user/tfstate", Command: "claude"},
		{Target: "ops:1.0", ID: "%5", PID: 4246, Path: "/home/user/infra", Command: "claude"},
		{Target: "misc:0.0", ID: "%6", PID: 4247, Path: "/home/user", Command: "zsh"},
	}
	panes := map[string]tmuxop.Pane{
		"s-api":     list[0],
		"s-webapp":  list[1],
		"s-tfstate": list[3],
		"s-infra":   list[4],
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
	m.point("s-api")
	m.SetTrail(fixtureTrail(fixtureBase))
	m.SetMirror(fixtureFrame)
	return m
}

// press sends one key the way a terminal would.
func press(m *Model, key string) {
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
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
		" dev",                   // the tmux session, unnumbered and dim
		" ops",                   // …in the order the pane list names them
		" elsewhere",             // …and the live sessions tmux cannot place
		":1.0 · claude/auth-fx",  // the location line drops the redundant prefix
		"no pane · main",         // and says so plainly when there is none
		"5 archived · A browses", // the last fleet row
		"FLEET · live",           // which fleet this is
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
	if !strings.HasSuffix(strings.TrimRight(fleetCol[ops], " "), fleet.Glyph(state.NeedsYou)) {
		t.Errorf("the ops header should echo its needs-you session: %q", fleetCol[ops])
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
	if m.selectedID != "a-docs" {
		t.Errorf("the archive opens on its first entry, got %q", m.selectedID)
	}
	press(m, "3") // third rendered archive entry: the api group's newest
	if m.selectedID != "a-api" {
		t.Errorf("3 selects the third rendered archive session, got %q", m.selectedID)
	}
	press(m, "A")
	if m.archiveView {
		t.Fatal("A must return to the live fleet")
	}
	if m.selectedID != "s-api" {
		t.Errorf("returning to the live fleet must restore its selection, got %q", m.selectedID)
	}
	press(m, "A")
	if !m.archiveView || m.selectedID != "a-api" {
		t.Errorf("the archive must restore its own selection, got archive=%v id=%q", m.archiveView, m.selectedID)
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

// fakeProc puts one claude process in every pane, at a cwd keyed by its pid.
type fakeProc map[int]string

func (p fakeProc) Children(pid int) []int {
	if _, ok := p[pid]; ok {
		return []int{pid + 1000}
	}
	return nil
}
func (p fakeProc) Comm(pid int) string { return "claude" }
func (p fakeProc) Cwd(pid int) string  { return p[pid-1000] }

// T58 — the pane poll carries both shapes of the truth: the map the deck looks
// panes up in, and tmux's own ordering the live view groups by. It runs with no
// Manager at all (a harness), which must not panic where MarkPaneMapped is fed.
func TestT58PanesMsgCarriesTmuxOrder(t *testing.T) {
	m := New(nil) // no Manager: MarkPaneMapped has nobody to tell
	m.runner = fakeTmux{out: "ops:0.0\t%1\t11\t/home/user/ops\tclaude\n" +
		"dev:1.0\t%2\t22\t/home/user/api\tclaude\n"}
	m.proc = fakeProc{11: "/home/user/ops", 22: "/home/user/api"}
	m.SetSessions([]fleet.Session{
		{Info: fleet.SessionInfo{ID: "s-api", CWD: "/home/user/api"}, Live: true},
		{Info: fleet.SessionInfo{ID: "s-ops", CWD: "/home/user/ops"}, Live: true},
	}, fixtureBase)

	msg, ok := m.relistPanes()().(panesMsg)
	if !ok {
		t.Fatal("relistPanes did not produce a panesMsg")
	}
	if len(msg.list) != 2 || msg.list[0].Target != "ops:0.0" || msg.list[1].Target != "dev:1.0" {
		t.Fatalf("the ordered pane list did not survive the poll: %+v", msg.list)
	}
	if msg.panes["s-api"].Target != "dev:1.0" || msg.panes["s-ops"].Target != "ops:0.0" {
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
			Info: fleet.SessionInfo{ID: id, CWD: cwd, GitBranch: "main", LastEventAt: at},
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
		{Target: "ops:0.0", ID: "%1", PID: 1, Path: "/home/user/ops", Command: "claude"},
		{Target: "dev:9.0", ID: "%2", PID: 2, Path: "/home/user/nine", Command: "claude"},
		{Target: "dev:10.0", ID: "%3", PID: 3, Path: "/home/user/ten", Command: "claude"},
	}
	m.SetPaneOrder(list)
	m.SetPanes(map[string]tmuxop.Pane{"s-ops": list[0], "s-w9": list[1], "s-w10": list[2]})

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
	if m.selectedID != "s-w10" {
		t.Errorf("3 selected %q, want s-w10", m.selectedID)
	}
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
