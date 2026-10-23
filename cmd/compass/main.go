// Command compass observes Claude Code sessions: a full-screen deck by
// default, or a one-shot `compass status` line for your own tmux status bar.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/narrator"
	"github.com/deephanson94/compass/internal/opencode"
	"github.com/deephanson94/compass/internal/prstate"
	"github.com/deephanson94/compass/internal/ui"
)

// defaultLiveWithin is the recency door's default width: a session with no
// tmux pane still counts as live while it spoke this recently (M5 contract).
const defaultLiveWithin = "5m"

// version is the build's name. It is "dev" for a binary built from a working
// tree; a release build stamps the tag over it with
// -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	args := os.Args[1:]

	// A subcommand, when present, comes first: `compass status -root …`.
	sub := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}
	// trail reads one transcript and never the fleet: it has flags of its
	// own and skips the deck's setup entirely.
	if sub == "trail" {
		os.Exit(runTrail(args))
	}
	// A word compass does not know is a mistake, not a request for the
	// deck: a reader probing for a subcommand (a mod asking for `trail`)
	// must get an error back, never a full-screen deck with no terminal.
	switch sub {
	case "", "status", "panes", "prs", "help":
	default:
		fmt.Fprintf(os.Stderr, "compass: unknown command %q (try compass help)\n", sub)
		os.Exit(2)
	}

	cfg := loadConfig()
	rootDefault := defaultRoot()
	if cfg.Root != "" {
		rootDefault = expandHome(cfg.Root)
	}
	narratorDefault := "haiku"
	if cfg.Narrator != "" {
		narratorDefault = cfg.Narrator
	}
	// An unreadable live_within is ignored like any other bad config line; a bad
	// -live-within on the command line is a typo worth stopping for.
	liveDefault := defaultLiveWithin
	if _, err := time.ParseDuration(cfg.LiveWithin); err == nil {
		liveDefault = cfg.LiveWithin
	}

	fs := flag.NewFlagSet("compass", flag.ExitOnError)
	root := fs.String("root", rootDefault, "Claude home directory to observe")
	readonly := fs.Bool("readonly", cfg.Readonly, "never write to tmux: enter no longer attaches")
	mirror := fs.Bool("mirror", cfg.Mirror, "open the live mirror of the selected pane at Lv1 (m toggles it)")
	dim := fs.String("dim", cfg.Dim, `grey for the quiet rows: "#c0c0c0" or a palette index (empty = compass's own)`)
	model := fs.String("narrator", narratorDefault, `narration model for leg labels ("off" disables)`)
	liveWithin := fs.String("live-within", liveDefault,
		`how recently a paneless session must have spoken to count as live ("0" = tmux panes only, questions included)`)
	opencodeDB := fs.String("opencode-db", opencode.DefaultDB(), `OpenCode's store, whose sessions join the fleet when the file exists ("" = never)`)
	prInterval := fs.Duration("pr-interval", prstate.DefaultInterval,
		`how often to ask Gitea (through tea) whose move each session's pull request is ("0" turns it off)`)
	teaLogin := fs.String("tea-login", "", `the tea login to ask as ("" = tea's own default)`)
	showVersion := fs.Bool("version", false, "print the build's version and exit")
	fs.Usage = usage(fs)
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	// -version answers and stops: it reads nothing and watches nothing.
	if *showVersion {
		// The trail protocol rides on the version line: -version is the one
		// question every compass answers without starting anything, so a
		// mod asks it before it ever runs `trail`.
		fmt.Printf("compass %s (trail %d)\n", version, trailProtocol)
		os.Exit(0)
	}
	window, err := time.ParseDuration(*liveWithin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "compass: -live-within %q is not a duration (try 5m, 90s, 0)\n", *liveWithin)
		os.Exit(2)
	}

	mgr := fleet.NewManager(*root)
	mgr.SetLiveWindow(window)
	if *opencodeDB != "" {
		// OpenCode's sessions sit in the fleet beside Claude's when its
		// store is on this machine; a store that will not open is left
		// out without a word, since most machines have none.
		if store, err := opencode.Open(*opencodeDB); err == nil {
			store.Attach(mgr)
		}
	}
	build := buildNarrator(mgr, *root, *model)
	prs := openPRs(*root, *teaLogin, *prInterval)

	switch sub {
	case "status":
		// The status line runs from tmux every few seconds, as a fresh process
		// each time, and the sessions worth reporting on are the long live ones
		// that cost the most to replay. Resume where the last run stopped.
		resume := openResume(mgr, *root)
		now := time.Now()
		line := "○ all quiet"
		if sessions, err := mgr.Refresh(now); err == nil {
			line = fleet.StatusOf(sessions)
			if prs != nil {
				// A poll is due once every -pr-interval; between them
				// the cache answers and nothing leaves the machine.
				prs.PollIfDue(liveDirs(sessions), now)
				for _, seg := range []string{
					prstate.Segment(livePRs(sessions, prs.States(now))),
					prstate.ReviewSegment(prs.Reviews(now)),
				} {
					if seg != "" {
						line += " " + seg
					}
				}
			}
		}
		fmt.Println(line)
		resume.Save()
	case "panes":
		printPanes(os.Stdout, mgr, *root, time.Now())
		fmt.Println()
		fmt.Println(prstate.Probe(prstate.RealRunner{Timeout: teaTimeout}, *teaLogin, *prInterval))
	case "prs":
		resume := openResume(mgr, *root)
		now := time.Now()
		sessions, err := livePaned(mgr, now)
		if err != nil {
			fmt.Fprintln(os.Stderr, "compass:", err)
			os.Exit(1)
		}
		printPRs(os.Stdout, sessions, prs, now)
		resume.Save()
	case "help":
		fs.Usage()
	default:
		// The palette is set before the first frame is drawn: an unreadable
		// value is ignored inside SetDim, so a typo costs nothing.
		ui.SetDim(*dim)
		// The deck resumes where the last process — a deck or a status
		// line — stopped reading, as `status` does: at launch every live
		// session is otherwise replayed from its first byte, and the scan
		// opens every archived transcript again (round 70). Saved on the
		// way out; a deck that is killed leaves the last save, which the
		// next start replays forward from.
		resume := openResume(mgr, *root)
		err := ui.Run(mgr, *readonly, *mirror, cfg.Replies, cfg.Hook, build, prs)
		resume.Save()
		if err != nil {
			fmt.Fprintln(os.Stderr, "compass:", err)
			os.Exit(1)
		}
	}
}

// buildNarrator assembles the narration service: the claude CLI in headless
// mode, working out of a compass-private dir the fleet is told to ignore, and
// a label cache beside it. "off", or an unusable cache path, leaves the trail
// on its heuristic labels — narration is polish, never a requirement.
func buildNarrator(mgr *fleet.Manager, root, model string) func(notify func()) ui.Narrator {
	if model == "off" {
		return nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base = root
	}
	dir := filepath.Join(base, "compass", "narrator")
	mgr.ExcludeCWD(dir)

	cache, err := narrator.OpenCache(filepath.Join(base, "compass", "labels.jsonl"))
	if err != nil {
		return nil
	}
	runner := &narrator.CLIRunner{Model: model, Dir: dir}
	return func(notify func()) ui.Narrator {
		return narrator.New(runner, cache, notify)
	}
}

// teaTimeout is how long one tea call may take before it is given up on.
const teaTimeout = 30 * time.Second

// openPRs is the pull-request tracker, its cache beside the others. An
// interval of zero is no tracker at all.
func openPRs(root, login string, interval time.Duration) *prstate.Tracker {
	if interval <= 0 {
		return nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base = root
	}
	return prstate.New(prstate.RealRunner{Timeout: teaTimeout}, login, interval, filepath.Join(base, "compass", "prs.json"))
}

// liveDirs is where each live session works, keyed as the fleet keys it:
// its cwd first, then the directories its tool calls worked in.
func liveDirs(sessions []fleet.Session) map[string][]string {
	out := map[string][]string{}
	for _, s := range sessions {
		if s.Live {
			out[s.Info.Key()] = ui.WorkDirs(s)
		}
	}
	return out
}

// livePRs keeps the pull requests of the live sessions: the status line
// counts what is happening, not the archive.
func livePRs(sessions []fleet.Session, states map[string][]prstate.Status) map[string][]prstate.Status {
	out := map[string][]prstate.Status{}
	for _, s := range sessions {
		if st, ok := states[s.Info.Key()]; ok && s.Live {
			out[s.Info.Key()] = st
		}
	}
	return out
}

// openResume points the manager at a resume cache beside the narrator's, and
// hands it back for saving once the work is done. A cache path that cannot be
// worked out is simply no cache: the run replays, which is what it did before.
func openResume(mgr *fleet.Manager, root string) *fleet.ResumeCache {
	base, err := os.UserCacheDir()
	if err != nil {
		base = root
	}
	c := fleet.OpenResumeCache(filepath.Join(base, "compass", "resume.json"))
	mgr.UseResumeCache(c)
	return c
}

// expandHome turns a leading ~/ into the user's home directory.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}

// defaultRoot is $COMPASS_ROOT, else ~/.claude.
func defaultRoot() string {
	if r := os.Getenv("COMPASS_ROOT"); r != "" {
		return r
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".claude"
	}
	return filepath.Join(home, ".claude")
}

func usage(fs *flag.FlagSet) func() {
	return func() {
		fmt.Fprintln(fs.Output(), "compass — a panel next to your agents")
		fmt.Fprintln(fs.Output(), "\n  compass           the deck: every session, one glance")
		fmt.Fprintln(fs.Output(), "  compass status    one-shot fleet summary (▲1 ◍1 ●3)")
		fmt.Fprintln(fs.Output(), "  compass panes     which tmux pane holds which session (diagnostic)")
		fmt.Fprintln(fs.Output(), "  compass prs       poll Gitea now: each session's checkouts and pull requests (diagnostic)")
		fmt.Fprintln(fs.Output(), "  compass trail     one session's journey as JSON (-session ID [-follow])")
		fmt.Fprintln(fs.Output(), "\nflags:")
		fs.PrintDefaults()
	}
}
