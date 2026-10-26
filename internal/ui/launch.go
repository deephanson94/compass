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

// launchForm is the `n` panel: what a new window runs, which tmux session
// it opens in, where, under what name, in a worktree or not, and the first
// prompt. Everything but the prompt arrives filled in from the selected
// session, so the common launch is `n`, the prompt, enter.
type launchForm struct {
	field    int
	harness  int      // into Model.harnesses
	sessions []string // the tmux sessions, the selected card's first
	session  int
	dirs     []string // the directories the fleet has worked in, the selected card's first
	dir      int      // the candidate ←/→ last landed on
	dirText  string   // the directory as it stands: a candidate, or typed
	name     string
	worktree bool
	prompt   string
}

// The panel's rows, top to bottom; tab walks them in this order.
const (
	launchWith = iota
	launchSession
	launchDir
	launchName
	launchWorktree
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

// launchDoneMsg says a window opened, or did not.
type launchDoneMsg struct {
	res     launch.Result
	harness string
	err     error
}

// offerLaunch is `n`: the panel opens over the deck. Nothing runs until
// enter. It is compass's third write, and like the other two it is gated
// on a keypress and switched off by read-only.
func (m *Model) offerLaunch() {
	if m.readonly {
		m.note = "read-only · launch is off"
		return
	}
	if len(m.harnesses) == 0 {
		m.harnesses = launch.Harnesses(nil)
	}
	sessions := tmuxop.Sessions(m.runner)
	if len(sessions) == 0 {
		m.note = "launch needs a tmux session to open a window in"
		return
	}
	f := &launchForm{sessions: sessions, dirs: m.launchDirs()}
	if pane, ok := m.selectedPane(); ok {
		// The selected card's tmux session: a new task most often joins
		// the work it sits beside.
		if i := strings.LastIndexByte(pane.Target, ':'); i > 0 {
			f.session = indexOf(sessions, pane.Target[:i])
		}
	}
	if len(f.dirs) > 0 {
		f.dirText = f.dirs[0]
	} else if wd, err := os.Getwd(); err == nil {
		f.dirText = wd
	}
	f.field = launchPrompt
	if !m.harnesses[0].TakesPrompt() {
		f.field = launchDir
	}
	m.launching = f
}

// launchDirs is where the fleet has been working, most recent first, the
// selected session's directory leading. Only directories still on disk.
func (m *Model) launchDirs() []string {
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
	seen := map[string]bool{}
	var out []string
	add := func(d string) {
		if d == "" || seen[d] || len(out) >= launchDirsMax {
			return
		}
		seen[d] = true
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			out = append(out, d)
		}
	}
	add(first)
	for _, i := range order {
		s := m.sessions[i].Info
		add(sessionDir(s.CWD, s.OriginCWD))
	}
	return out
}

// sessionDir is where a session works: where it is now, else where it began.
func sessionDir(cwd, origin string) string {
	if cwd != "" {
		return cwd
	}
	return origin
}

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
	if m.launching != nil && !m.harnesses[m.launching.harness].TakesPrompt() && m.launching.field == launchPrompt {
		m.launching.field = launchWorktree // the harness changed under the prompt row
	}
	return nil
}

// launchStep moves the panel's focus, past the prompt row when the harness
// takes no prompt.
func (m *Model) launchStep(d int) {
	f := m.launching
	f.field = wrapIndex(f.field+d, launchFields)
	if f.field == launchPrompt && !m.harnesses[f.harness].TakesPrompt() {
		f.field = wrapIndex(f.field+d, launchFields)
	}
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

// submitLaunch closes the panel and opens the window off the render loop:
// a worktree is a git checkout, which can take a moment.
func (m *Model) submitLaunch() tea.Cmd {
	f := m.launching
	m.launching = nil
	h := m.harnesses[f.harness]
	req := launch.Request{
		Session:  f.sessions[f.session],
		Dir:      strings.TrimSpace(f.dirText),
		Name:     f.name,
		Harness:  h,
		Worktree: f.worktree,
	}
	if h.TakesPrompt() {
		req.Prompt = f.prompt
	}
	runner, git := m.runner, m.git
	if git == nil {
		git = launch.RealGit{}
	}
	m.note = "opening " + req.WindowName() + " in " + req.Session + "…"
	return func() tea.Msg {
		res, err := launch.Launch(runner, git, req)
		return launchDoneMsg{res: res, harness: h.Name, err: err}
	}
}

// launched is where a window landed: the footer says so, and the deck
// means to select its session once it joins the fleet — unless the person
// has moved on by then.
func (m *Model) launched(msg launchDoneMsg) {
	if msg.err != nil {
		m.note = "could not launch: " + firstLine(msg.err.Error())
		return
	}
	m.note = "↪ opened " + msg.res.Name + " · " + msg.harness + " " + mirrorMark + " " + msg.res.Pane.Target
	m.landing = landing{pane: msg.res.Pane.ID, from: m.selectedKey, until: m.now.Add(launchLandFor)}
}

// landing is a launched window the deck is waiting to see join the fleet.
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

	row(launchWith, "run", choice(h.Name), "")
	row(launchSession, "in tmux", choice(f.sessions[f.session]), "a new window")
	dir := homeShort(f.dirText) + cursor(launchDir)
	row(launchDir, "dir", dir, dirHint(f))
	name := f.name + cursor(launchName)
	if f.name == "" && f.field != launchName {
		name = launch.Request{Harness: h, Prompt: f.prompt}.WindowName()
	}
	row(launchName, "name", name, "")
	wt := "off"
	if f.worktree {
		wt = "on · a new branch beside the repo"
	}
	row(launchWorktree, "worktree", wt, "")
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

	title := " new window "
	box := func(s string, style lipgloss.Style) string {
		return " " + ruleStyle.Render("│") + " " + style.Render(pad(clip(s, body), body)) + " " + ruleStyle.Render("│") + " "
	}
	out := []string{" " + ruleStyle.Render("┌"+title+strings.Repeat("─", body+2-lipgloss.Width(title))+"┐") + " "}
	for _, r := range rows {
		out = append(out, box(r.text, readerStyle(r.kind)))
	}
	return append(out, " "+ruleStyle.Render("└"+strings.Repeat("─", body+2)+"┘")+" ")
}

// dirHint says where ←/→ stands among the fleet's directories.
func dirHint(f *launchForm) string {
	if len(f.dirs) < 2 {
		return ""
	}
	return "←/→ " + strconv.Itoa(f.dir+1) + " of " + strconv.Itoa(len(f.dirs))
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
