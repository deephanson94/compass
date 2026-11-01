package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
	"github.com/deephanson94/compass/internal/tmuxop"
	"github.com/deephanson94/compass/internal/transcript"
)

// printPerf is `compass perf`: every stage of a deck's launch, timed on this
// machine's own transcripts, in one screen of plain text meant to be pasted
// or screenshotted into a conversation about why compass is slow here.
//
// The launch is measured twice, the way it happens: from the resume cache, as
// every launch after the first does, and cold, as the first launch after the
// cache is lost does. The warm stages run first, so the cold ones are the ones
// that find the files in the page cache. Nothing is written: the resume cache
// is opened read-only, and tmux is only listed.
//
// fresh builds a Manager as the deck builds its own — live window, OpenCode
// store, exclusions — so the refreshes timed here are the deck's; window is
// that same live window, for the scans that run without a Manager.
func printPerf(w io.Writer, root, cachePath string, window time.Duration, fresh func() *fleet.Manager, now time.Time) {
	fmt.Fprintf(w, "compass perf · %s · %s/%s · %d cores · %s\n", version, runtime.GOOS, runtime.GOARCH, runtime.GOMAXPROCS(0), now.Format("2006-01-02 15:04"))

	// The cache, as the next launch will find it.
	t0 := time.Now()
	cache := fleet.OpenResumeCacheReadOnly(cachePath)
	openTook := time.Since(t0)
	// The refresh below records into the cache it is given; the marks the
	// launch would have resumed from are read off a copy it never touches.
	before := fleet.OpenResumeCacheReadOnly(cachePath)
	stats := cache.Stats()
	cacheLine := "missing"
	if fi, err := os.Stat(cachePath); err == nil {
		cacheLine = fmt.Sprintf("%s · saved %s ago · %d peeked · %d live marks · %d journeys",
			perfBytes(fi.Size()), perfAge(now.Sub(fi.ModTime())), stats.Peeked, stats.Points, stats.Journeys)
	}

	quiet := now.Add(-window) // the deck's own quiet mark, which -live-within moves
	t0 = time.Now()
	scanned, warmFiles, err := fleet.ProbeScan(root, cache, quiet)
	warmScan := time.Since(t0)
	if err != nil {
		fmt.Fprintf(w, "root   %s: %v\n", tilde(root), err)
		return
	}
	var total int64
	projects := map[string]bool{}
	for _, f := range warmFiles {
		total += f.Size
		projects[filepath.Dir(f.Path)] = true
	}
	fmt.Fprintf(w, "root   %s · %d transcripts in %d projects · %s\n", tilde(root), len(warmFiles), len(projects), perfBytes(total))
	fmt.Fprintf(w, "cache  %s · %s\n\n", tilde(cachePath), cacheLine)

	// The panes come first: a session in a pane is live whatever its clock
	// says, and the deck tells its Manager which ones are before it judges
	// them. A refresh timed without them would leave those sessions archived
	// and skip the replays that make a launch cost what it costs.
	t0 = time.Now()
	panes, paneErr := tmuxop.ListPanes(tmuxop.RealRunner{})
	listTook := time.Since(t0)
	var pidsTook, pairTook time.Duration
	paired := map[string]bool{}
	if paneErr == nil {
		t0 = time.Now()
		byPID := fleet.RunningPIDs(root)
		pidsTook = time.Since(t0)
		t0 = time.Now()
		for key := range tmuxop.MapSessionsBy(scanned, panes, tmuxop.RealProc{}, byPID) {
			paired[key] = true
		}
		pairTook = time.Since(t0)
	}

	warm := fresh()
	warm.MarkPaneMapped(paired)
	warm.UseResumeCache(cache)
	t0 = time.Now()
	sessions, err := warm.Refresh(now)
	warmRefresh := time.Since(t0)
	if err != nil {
		fmt.Fprintln(w, "refresh:", err)
		return
	}
	live, archived, fromFiles := 0, 0, 0
	for _, s := range sessions {
		if s.Live {
			live++
		} else {
			archived++
		}
		if transcript.SchemeOf(s.Info.TranscriptPath) == "" {
			fromFiles++
		}
	}
	// Transcripts the scan read that the fleet then drops: sessions run in
	// an excluded directory, which is chiefly the narrator's own.
	dropped := len(warmFiles) - fromFiles

	t0 = time.Now()
	_, coldFiles, _ := fleet.ProbeScan(root, nil, quiet)
	coldScan := time.Since(t0)

	t0 = time.Now()
	cold := fresh()
	cold.MarkPaneMapped(paired)
	_, _ = cold.Refresh(now)
	coldRefresh := time.Since(t0)

	misses := map[string]int{}
	cached := 0
	for _, f := range warmFiles {
		if f.Cached {
			cached++
		} else {
			misses[f.Why]++
		}
	}

	fmt.Fprintln(w, "LAUNCH  in the order run: the first stage to read a file pays the disk")
	stage := func(name string, d time.Duration, note string) {
		fmt.Fprintf(w, "  %-20s %9s   %s\n", name, perfDur(d), note)
	}
	stage("open resume cache", openTook, "")
	stage("scan, from cache", warmScan, fmt.Sprintf("%d cached · %d peeked%s", cached, len(warmFiles)-cached, perfMisses(misses)))
	note := fmt.Sprintf("the deck's launch · %d live · %d archived", live, archived)
	if dropped > 0 {
		note += fmt.Sprintf(" · %d excluded", dropped)
	}
	stage("refresh, from cache", warmRefresh, note)
	stage("scan, cold", coldScan, fmt.Sprintf("%d peeked", len(coldFiles)))
	stage("refresh, cold", coldRefresh, "the first launch without a cache")

	// Each live session alone, replayed from its first byte the way a cold
	// launch, a woken session and the reader replay it. The launch runs them
	// side by side; here each is timed on its own so the heavy one shows.
	rows := perfLive(sessions, before)
	if len(rows) > 0 {
		fmt.Fprintln(w, "\nLIVE SESSIONS  full replay, one core · \"unread\" is what the cache leaves to read")
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "  session\tsize\tevents\tread+parse\tmachine\tjourney\tunread fleet\tunread board")
		for _, r := range rows {
			fmt.Fprintf(tw, "  %s\t%s\t%d\t%s\t%s\t%s\t%s\t%s\n", r.name, perfBytes(r.size), r.events,
				perfDur(r.parse), perfDur(r.machine), perfDur(r.journey), r.unreadFleet, r.unreadBoard)
		}
		tw.Flush()
	}

	// The peeks that cost the most when nothing is cached.
	sort.Slice(coldFiles, func(i, j int) bool { return coldFiles[i].Took > coldFiles[j].Took })
	if len(coldFiles) > 0 {
		fmt.Fprintln(w, "\nSLOWEST PEEKS  cold scan, head and tail of each file")
		for _, f := range coldFiles[:min(5, len(coldFiles))] {
			fmt.Fprintf(w, "  %9s %9s   %s\n", perfDur(f.Took), perfBytes(f.Size), perfShortPath(root, f.Path))
		}
	}

	fmt.Fprintln(w, "\nTMUX")
	if paneErr != nil {
		fmt.Fprintf(w, "  list panes %s · %v\n", perfDur(listTook), paneErr)
	} else {
		fmt.Fprintf(w, "  list panes %s · %d panes · running registry %s · pair with sessions %s · %d paired\n",
			perfDur(listTook), len(panes), perfDur(pidsTook), perfDur(pairTook), len(paired))
	}

	hints := perfHints(cachePath, len(warmFiles), cached, warmRefresh, rows)
	if len(hints) > 0 {
		fmt.Fprintln(w, "\nHINTS")
		for _, h := range hints {
			for i, line := range h {
				lead := "  -"
				if i > 0 {
					lead = "   "
				}
				fmt.Fprintln(w, lead, line)
			}
		}
	}
}

// perfRow is one live session's replay, timed stage by stage.
type perfRow struct {
	name                      string
	size                      int64
	events                    int
	parse, machine, journey   time.Duration
	unreadFleet, unreadBoard  string
	missingMark, missingTrail bool
}

func perfLive(sessions []fleet.Session, cache *fleet.ResumeCache) []perfRow {
	var rows []perfRow
	for _, s := range sessions {
		if !s.Live {
			continue
		}
		path := s.Info.TranscriptPath
		r := perfRow{name: perfName(s.Info)}
		if fi, err := os.Stat(path); err == nil {
			r.size = fi.Size()
		}
		t0 := time.Now()
		events, _ := transcript.NewTailer(path).Poll()
		r.parse = time.Since(t0)
		r.events = len(events)

		t0 = time.Now()
		m := state.NewMachine()
		for _, ev := range events {
			m.Observe(ev)
		}
		r.machine = time.Since(t0)

		t0 = time.Now()
		seg := journey.NewSegmenter()
		for _, ev := range events {
			seg.Observe(ev)
		}
		seg.Trail()
		r.journey = time.Since(t0)

		if transcript.SchemeOf(path) != "" {
			// A store's session, not a file: it has no byte to resume at.
			r.unreadFleet, r.unreadBoard = "store", "store"
			rows = append(rows, r)
			continue
		}
		var mark, trail *transcript.Mark
		if p, ok := cache.Point(s.Info.Key()); ok {
			mark = &p.Mark
		}
		if p, ok := cache.Journey(s.Info.Key()); ok {
			trail = &p.Mark
		}
		r.unreadFleet, r.missingMark = perfUnread(mark, r.size)
		r.unreadBoard, r.missingTrail = perfUnread(trail, r.size)
		rows = append(rows, r)
	}
	return rows
}

// perfUnread is what resuming at a saved mark leaves to read, and whether the
// whole file must be replayed because there is no mark it can use.
func perfUnread(m *transcript.Mark, size int64) (string, bool) {
	switch {
	case m == nil || m.Offset <= 0:
		return "all · no mark", true
	case m.Offset > size:
		return "all · file shrank", true
	}
	return perfBytes(size - m.Offset), false
}

// perfHints names what the numbers above point at, in the order a person
// would fix them. Each hint is its lines, kept short enough for a screenshot.
func perfHints(cachePath string, files, cached int, warm time.Duration, rows []perfRow) [][]string {
	var out [][]string
	if _, err := os.Stat(cachePath); err != nil {
		// Everything below follows from this one, so it is said alone.
		out = append(out, []string{"no resume cache: every launch is cold. A deck writes it on its first refresh;",
			"check the directory is writable and the deck sees the same $XDG_CACHE_HOME as this command."})
		return out
	}
	if files > 0 && cached*10 < files*9 {
		out = append(out, []string{fmt.Sprintf("the cache answered for %d of %d transcripts. \"touched\" means something rewrites", cached, files),
			"mtimes (a sync tool, a backup, a copy without -p); \"grew\" is a file still being written."})
	}
	var bare []string
	var cost time.Duration
	for _, r := range rows {
		if r.missingMark {
			bare = append(bare, r.name)
			cost += r.parse + r.machine
		}
	}
	if len(bare) > 0 {
		out = append(out, []string{fmt.Sprintf("%d live with no usable mark, replayed in full at every launch (%s on one core): %s.",
			len(bare), perfDur(cost), strings.Join(bare, ", ")),
			"A mark is saved once a deck or `compass status` has read the session."})
	}
	cols, replay := 0, time.Duration(0)
	for _, r := range rows {
		if r.missingTrail && !r.missingMark {
			cols++
			replay += r.parse + r.journey
		}
	}
	if replay > 200*time.Millisecond {
		out = append(out, []string{fmt.Sprintf("%d live with no board mark: their columns replay in full when the board opens (%s on one core).", cols, perfDur(replay)),
			"Only the deck saves board marks; `compass status` does not."})
	}
	if warm > 500*time.Millisecond {
		out = append(out, []string{fmt.Sprintf("the launch from cache took %s, over the 0.5 s it should take.", perfDur(warm))})
	}
	return out
}

func perfMisses(m map[string]int) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%d %s", m[k], k))
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

// perfName is a session as a person knows it, with enough of its id to find
// the file.
func perfName(info fleet.SessionInfo) string {
	name := info.Name
	if name == "" {
		name = filepath.Base(info.CWD)
	}
	id := info.ID
	if len(id) > 8 {
		id = id[:8]
	}
	return name + " " + id
}

func perfShortPath(root, path string) string {
	if rel, err := filepath.Rel(filepath.Join(root, "projects"), path); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return tilde(path)
}

func tilde(path string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~" + path[len(home):]
	}
	return path
}

func perfDur(d time.Duration) string {
	switch {
	case d < 10*time.Millisecond:
		return fmt.Sprintf("%.1f ms", float64(d)/float64(time.Millisecond))
	case d < time.Second:
		return fmt.Sprintf("%d ms", d.Milliseconds())
	}
	return fmt.Sprintf("%.2f s", d.Seconds())
}

func perfBytes(n int64) string {
	switch {
	case n < 1<<10:
		return fmt.Sprintf("%d B", n)
	case n < 1<<20:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	case n < 1<<30:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	}
	return fmt.Sprintf("%.2f GB", float64(n)/(1<<30))
}

func perfAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
