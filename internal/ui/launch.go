package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/deephanson94/compass/internal/launch"
	"github.com/deephanson94/compass/internal/tmuxop"
)

// launchForm is the `n` panel: what the new work runs, whether it is a
// window, a pane or a tmux session of its own, where, under what name, in a
// worktree or not, and the first prompt. Everything but the prompt arrives
// filled in from the selected session, so the common launch is `n`, the
// prompt, enter.
type launchForm struct {
	field    int
	harness  int      // into Model.harnesses
	place    int      // into tmuxop.Places
	sessions []string // the tmux sessions, the selected card's first
	session  int
	pane     tmuxop.Pane // the selected card's pane, which a split goes beside
	hasPane  bool
	dirs     []string        // the selected card's directory, the favourites, then where the fleet has worked
	favs     map[string]bool // which of dirs the config names
	dir      int             // the candidate ←/→ last landed on
	dirText  string          // the directory as it stands: a candidate, or typed
	name     string
	worktree bool
	goThere  bool // then: go — attach to the new pane once it opens
	goSet    bool // …and the person chose it, so a harness change leaves it be
	prompt   string
}

// The panel's rows, top to bottom; tab walks them in this order.
const (
	launchWith = iota
	launchPlace
	launchSession
	launchDir
	launchName
	launchWorktree
	launchThen
	launchPrompt
	launchFields
)

// launchDirsMax is how many directories ←/→ walks through: the recent ones
// are the ones a new task is started in.
const launchDirsMax = 20

// launchLandFor is how long the deck waits for a launched window's session
// to join the fleet before it stops meaning to select it. A shell never
// joins; a claude that has not written by then is not one to jump to.
const launchLandFor = 2 * time.Minute

// launchDoneMsg says the work opened, or did not.
type launchDoneMsg struct {
	res     launch.Result
	harness string
	goThere bool // hand the terminal to the new pane
	err     error
}

// goesThere is the then row's default: go to what will never be a card on
// the board — a shell, a harness the fleet does not read — and stay for
// claude, whose card the deck selects once it joins.
func goesThere(h launch.Harness) bool { return h.Name != "claude" }

// offerLaunch is `n`: the panel opens over the deck. Nothing runs until
// enter. It is compass's third write, and like the other two it is gated
// on a keypress and switched off by read-only.
//
// It opens on a window in the selected card's tmux session. With no tmux
// session anywhere it opens on a session of its own instead: that is the
// one place there is room for.
func (m *Model) offerLaunch() {
	if m.readonly {
		m.note = "read-only · launch is off"
		return
	}
	if len(m.harnesses) == 0 {
		m.harnesses = launch.Harnesses(nil)
	}
	f := &launchForm{sessions: tmuxop.Sessions(m.runner)}
	f.dirs, f.favs = m.launchDirs()
	if len(f.sessions) == 0 {
		f.place = indexOfPlace(tmuxop.PlaceSession)
	}
	if pane, ok := m.selectedPane(); ok {
		// The selected card's tmux session: a new task most often joins
		// the work it sits beside.
		f.pane, f.hasPane = pane, true
		if i := strings.LastIndexByte(pane.Target, ':'); i > 0 {
			f.session = indexOf(f.sessions, pane.Target[:i])
		}
	}
	if len(f.dirs) > 0 {
		f.dirText = f.dirs[0]
	} else if wd, err := os.Getwd(); err == nil {
		f.dirText = wd
	}
	f.goThere = goesThere(m.harnesses[0])
	f.field = launchPrompt
	if !m.harnesses[0].TakesPrompt() {
		f.field = launchDir
	}
	m.launching = f
}

// launchDirs is what ←/→ walks on the dir row: the selected session's
// directory, then the config's favourites in their order, then where the
// fleet has been working, most recent first. A favourite stays though it is
// not there yet — the person named it, and the panel will offer to make it;
// a directory only the fleet remembers is offered only while it is on disk.
func (m *Model) launchDirs() ([]string, map[string]bool) {
	var first string
	if s, ok := m.selected(); ok {
		first = sessionDir(s.Info.CWD, s.Info.OriginCWD)
	}
	order := make([]int, len(m.sessions))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return m.sessions[order[a]].Info.LastEventAt.After(m.sessions[order[b]].Info.LastEventAt)
	})
	seen, favs := map[string]bool{}, map[string]bool{}
	var out []string
	add := func(d string, fav bool) {
		d = filepath.Clean(d)
		if d == "." || len(out) >= launchDirsMax {
			return
		}
		if fav {
			favs[d] = true // the selected card's directory can be a favourite too
		}
		if seen[d] {
			return
		}
		seen[d] = true
		if missing, err := launch.Missing(d); err == nil && (fav || !missing) {
			out = append(out, d)
		}
	}
	if first != "" {
		add(first, false)
	}
	for _, d := range m.favDirs {
		if d, err := launch.ExpandHome(d); err == nil && d != "" {
			add(d, true)
		}
	}
	for _, i := range order {
		s := m.sessions[i].Info
		if d := sessionDir(s.CWD, s.OriginCWD); d != "" {
			add(d, false)
		}
	}
	return out, favs
}

// sessionDir is where a session works: where it is now, else where it began.
func sessionDir(cwd, origin string) string {
	if cwd != "" {
		return cwd
	}
	return origin
}

func indexOfPlace(p tmuxop.Place) int {
	for i, q := range tmuxop.Places {
		if q == p {
			return i
		}
	}
	return 0
}

// placeOf is the place the panel stands on.
func (f *launchForm) placeOf() tmuxop.Place { return tmuxop.Places[f.place] }

func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return 0
}

// launchKey handles a key while the panel is up. Tab and the arrows move
// between rows and choices, the text rows take what is typed, enter opens
// the window, esc puts the panel away.
func (m *Model) launchKey(msg tea.KeyMsg) tea.Cmd {
	f := m.launching
	switch key := msg.String(); key {
	case "ctrl+c":
		return tea.Quit
	case "esc":
		m.launching = nil
	case "enter":
		return m.submitLaunch()
	case "tab", "down":
		m.launchStep(1)
	case "shift+tab", "up":
		m.launchStep(-1)
	case "left", "right":
		d := 1
		if key == "left" {
			d = -1
		}
		switch f.field {
		case launchWith:
			f.harness = wrapIndex(f.harness+d, len(m.harnesses))
			if !f.goSet {
				f.goThere = goesThere(m.harnesses[f.harness])
			}
		case launchThen:
			f.goThere, f.goSet = !f.goThere, true
		case launchPlace:
			f.place = wrapIndex(f.place+d, len(tmuxop.Places))
		case launchSession:
			f.session = wrapIndex(f.session+d, len(f.sessions))
		case launchDir:
			if len(f.dirs) > 0 {
				if f.dirText == f.dirs[f.dir] {
					f.dir = wrapIndex(f.dir+d, len(f.dirs))
				}
				f.dirText = f.dirs[f.dir]
			}
		case launchWorktree:
			f.worktree = !f.worktree
		}
	case "backspace":
		if t := f.text(); t != nil {
			if r := []rune(*t); len(r) > 0 {
				*t = string(r[:len(r)-1])
			}
		}
	case "ctrl+u":
		if t := f.text(); t != nil {
			*t = ""
		}
	case " ", "space":
		if f.field == launchWorktree {
			f.worktree = !f.worktree
		} else if f.field == launchThen {
			f.goThere, f.goSet = !f.goThere, true
		} else if t := f.text(); t != nil {
			*t += " "
		}
	default:
		if msg.Type == tea.KeyRunes {
			if t := f.text(); t != nil {
				in := string(msg.Runes)
				if f.field != launchPrompt {
					// A pasted line break means nothing in a path or a name;
					// a prompt keeps its own.
					in = strings.NewReplacer("\r", "", "\n", "").Replace(in)
				}
				*t += in
			}
		}
	}
	return nil
}

// launchStep moves the panel's focus over the rows that have a choice to
// make: past the prompt when the harness takes none, and past the session
// row unless the place is a window — a pane's is the card's, a new
// session's is its name.
func (m *Model) launchStep(d int) {
	f := m.launching
	for i := 0; i < launchFields; i++ {
		f.field = wrapIndex(f.field+d, launchFields)
		if m.launchRowOpen(f.field) {
			return
		}
	}
}

// launchRowOpen says whether the row has anything to choose or type.
func (m *Model) launchRowOpen(field int) bool {
	f := m.launching
	switch field {
	case launchPrompt:
		return m.harnesses[f.harness].TakesPrompt()
	case launchSession:
		return f.placeOf() == tmuxop.PlaceWindow && len(f.sessions) > 1
	}
	return true
}

// text is the row being typed into, nil on a row of choices.
func (f *launchForm) text() *string {
	switch f.field {
	case launchDir:
		return &f.dirText
	case launchName:
		return &f.name
	case launchPrompt:
		return &f.prompt
	}
	return nil
}

func wrapIndex(i, n int) int {
	if n == 0 {
		return 0
	}
	return ((i % n) + n) % n
}

// submitLaunch closes the panel and opens the work off the render loop:
// a worktree is a git checkout, which can take a moment. A split needs the
// card's pane, so without one the panel stays up and says so.
func (m *Model) submitLaunch() tea.Cmd {
	f := m.launching
	place := f.placeOf()
	target := ""
	switch place {
	case tmuxop.PlaceWindow:
		if len(f.sessions) > 0 {
			target = f.sessions[f.session]
		}
	case tmuxop.PlacePane:
		if !f.hasPane {
			m.note = "a split needs a card with a pane · choose window or session"
			return nil
		}
		target = f.pane.ID
	}
	m.launching = nil
	h := m.harnesses[f.harness]
	req := launch.Request{
		Place:    place,
		Target:   target,
		Dir:      strings.TrimSpace(f.dirText),
		Name:     f.name,
		Harness:  h,
		Worktree: f.worktree,
		MakeDir:  true, // the dir row has said "new · will be created" since it was typed
	}
	if h.TakesPrompt() {
		req.Prompt = f.prompt
	}
	runner, git, goThere := m.runner, m.git, f.goThere
	if git == nil {
		git = launch.RealGit{}
	}
	m.note = "opening " + req.TaskName() + " as a " + place.String() + "…"
	return func() tea.Msg {
		res, err := launch.Launch(runner, git, req)
		return launchDoneMsg{res: res, harness: h.Name, goThere: goThere, err: err}
	}
}

// launched is where the work landed: the footer says so, and the deck
// means to select its session once it joins the fleet — unless the person
// has moved on by then. With then: go, the terminal goes to the new pane
// at once, the way Enter hands over a card's.
func (m *Model) launched(msg launchDoneMsg) tea.Cmd {
	if msg.err != nil {
		m.note = "could not launch: " + firstLine(msg.err.Error())
		return nil
	}
	m.note = "↪ opened " + msg.res.Name + " · " + msg.harness + " " + mirrorMark + " " + msg.res.Pane.Target
	m.landing = landing{pane: msg.res.Pane.ID, from: m.selectedKey, until: m.now.Add(launchLandFor)}
	if msg.goThere {
		return m.attachPane(msg.res.Pane)
	}
	return nil
}

// landing is launched work the deck is waiting to see join the fleet.
type landing struct {
	pane  string    // its tmux pane id
	from  string    // the selection when it was launched
	until time.Time // when the deck stops waiting
}

// land selects the launched session once a pane listing pairs it with one.
func (m *Model) land() {
	l := m.landing
	if l.pane == "" {
		return
	}
	if m.selectedKey != l.from || (!l.until.IsZero() && m.now.After(l.until)) {
		m.landing = landing{} // the person moved, or it never came
		return
	}
	for key, p := range m.panes {
		if p.ID == l.pane {
			m.landing = landing{}
			m.point(key)
			return
		}
	}
}

// launchPanel is the `n` panel as a boxed form.
func (m *Model) launchPanel(inner int) []string {
	f := m.launching
	h := m.harnesses[f.harness]
	body := replyPanelMax
	if max := inner - 8; body > max {
		body = max
	}
	if body < 30 {
		body = 30
	}
	const labelW = 10
	var rows []readerLine
	row := func(field int, label, value string, hint string) {
		mark := "  "
		kind := readerText
		if f.field == field {
			mark, kind = "▸ ", readerSaid
		}
		text := mark + pad(label, labelW) + value
		if hint != "" && f.field == field {
			text += "   " + hint
		}
		rows = append(rows, readerLine{text: text, kind: kind})
	}
	choice := func(v string) string { return "‹ " + v + " ›" }
	cursor := func(field int) string {
		if f.field == field {
			return "▏"
		}
		return ""
	}

	task := launch.Request{Harness: h, Prompt: f.prompt, Name: f.name}.TaskName()
	row(launchWith, "run", choice(h.Name), "")
	row(launchPlace, "as", choice(f.placeOf().String()), "")
	switch f.placeOf() {
	case tmuxop.PlaceWindow:
		if len(f.sessions) == 0 {
			row(launchSession, "in tmux", "no session to open it in", "")
		} else {
			row(launchSession, "in tmux", choice(f.sessions[f.session]), "")
		}
	case tmuxop.PlacePane:
		if f.hasPane {
			row(launchSession, "beside", mirrorMark+" "+f.pane.Target+" · the card's pane", "")
		} else {
			row(launchSession, "beside", "this card has no pane to split", "")
		}
	case tmuxop.PlaceSession:
		row(launchSession, "in tmux", "a new session · "+task, "")
	}
	// The path keeps its tail where the row is short: the end of a path is
	// what tells two directories apart.
	hint := dirHint(f)
	room := body - 2 - labelW - 1
	if hint != "" && f.field == launchDir {
		room -= lipgloss.Width(hint) + 3
	}
	dir, warn := keepTail(homeShort(f.dirText), room), ""
	if d, err := launch.ExpandHome(strings.TrimSpace(f.dirText)); err == nil && strings.TrimSpace(f.dirText) != "" {
		missing, err := launch.Missing(d)
		switch {
		case err != nil:
			warn = "not a directory"
		case missing && f.worktree:
			warn = "new · there is no repo to make a worktree of"
		case missing:
			// Its own row, whatever the path's length and wherever the
			// focus is: the warning is the confirmation, so it has to be
			// on screen when enter is pressed.
			warn = "new · enter creates it, empty"
		case f.favs[filepath.Clean(d)] && f.field != launchDir:
			dir += " ★"
		}
	}
	row(launchDir, "dir", dir+cursor(launchDir), hint)
	if warn != "" {
		rows = append(rows, readerLine{text: strings.Repeat(" ", 2+labelW) + "↳ " + warn, kind: readerSaid})
	}
	name := f.name + cursor(launchName)
	if f.name == "" && f.field != launchName {
		name = task
	}
	row(launchName, "name", name, "")
	wt := "off"
	if f.worktree {
		wt = "on · a new branch beside the repo"
	}
	row(launchWorktree, "worktree", wt, "")
	then := choice("stay") + "  opens in the background"
	if f.goThere {
		then = choice("go") + "  takes you there · prefix d returns"
	}
	row(launchThen, "then", then, "")
	rows = append(rows, readerLine{kind: readerBlank})
	if h.TakesPrompt() {
		kind := readerText
		if f.field == launchPrompt {
			kind = readerSaid
		}
		prompt := strings.ReplaceAll(f.prompt, "\n", " ⏎ ") + cursor(launchPrompt)
		for _, line := range wrapPrefix(prompt, "› ", "  ", body) {
			rows = append(rows, readerLine{text: line, kind: kind})
		}
	} else {
		rows = append(rows, readerLine{text: h.Name + " takes no prompt", kind: readerBody})
	}
	rows = append(rows, readerLine{kind: readerBlank},
		readerLine{text: "tab next · ←/→ choose · enter opens · esc closes", kind: readerBody})

	title := " new " + f.placeOf().String() + " "
	box := func(s string, style lipgloss.Style) string {
		return " " + ruleStyle.Render("│") + " " + style.Render(pad(clip(s, body), body)) + " " + ruleStyle.Render("│") + " "
	}
	out := []string{" " + ruleStyle.Render("┌"+title+strings.Repeat("─", body+2-lipgloss.Width(title))+"┐") + " "}
	for _, r := range rows {
		out = append(out, box(r.text, readerStyle(r.kind)))
	}
	return append(out, " "+ruleStyle.Render("└"+strings.Repeat("─", body+2)+"┘")+" ")
}

// dirHint says where ←/→ stands among the directories, and whether the one
// it stands on is a favourite.
func dirHint(f *launchForm) string {
	if len(f.dirs) < 2 || f.dirText != f.dirs[f.dir] {
		return "" // a typed path stands on no candidate
	}
	hint := "←/→ " + strconv.Itoa(f.dir+1) + " of " + strconv.Itoa(len(f.dirs))
	if f.favs[f.dirs[f.dir]] {
		hint += " ★"
	}
	return hint
}

// homeShort writes the home directory as "~".
func homeShort(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") && filepath.IsAbs(p) {
		return "~/" + rel
	}
	return p
}

// typing says a line has the keyboard — a search, the quick replies, the
// launch panel — so the deck's own keys are not the deck's to offer.
func (m *Model) typing() bool {
	return m.searching || m.replying || m.launching != nil
}

// overlayCentered draws the panel over rows, centred across width, a row
// below the top. Unlike the reply box it belongs to no column, so it has no
// column's cut to mark: a row that runs under it is cut with "…" only where
// the panel covered words, and a blank row stays blank.
func overlayCentered(rows, panel []string, width int) {
	pw := panelWidth(panel)
	left, top := (width-pw)/2, 1
	if left < 0 {
		left = 0
	}
	if top+len(panel) > len(rows) {
		top = len(rows) - len(panel)
	}
	if top < 0 {
		top = 0
	}
	for i, p := range panel {
		if top+i >= len(rows) {
			break
		}
		line := rows[top+i]
		before := pad(ansi.Truncate(line, left, ""), left)
		covered := ansi.Strip(ansi.TruncateLeft(ansi.Truncate(line, left+pw, ""), left, ""))
		if strings.TrimSpace(covered) != "" && left > 1 {
			before = pad(ansi.Truncate(line, left-1, ""), left-1) + "…"
		}
		rows[top+i] = strings.TrimRight(before+p+ansi.TruncateLeft(line, left+pw, ""), " ")
	}
}

// keepTail fits a path into w cells by its end: "…/work/api/pkg".
func keepTail(p string, w int) string {
	if w < 2 || lipgloss.Width(p) <= w {
		return p
	}
	r := []rune(p)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > w {
		r = r[1:]
	}
	return "…" + string(r)
}
