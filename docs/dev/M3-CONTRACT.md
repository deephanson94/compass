# M3 Contract — narrator, Lv3 reader, ask the trail

Binding API contract for milestone M3, same rules as before: implementation and
tests are written in parallel against this document; deviations amend it first.

M3 goal (ARCHITECTURE §7): *the panel becomes conversational.* Haiku narration
upgrades heuristic labels to prose through the user's own `claude` auth; Lv3
opens the full conversation as a reader; `a` hands the terminal to a historian.

## package narrator (new)

The narrator never talks to an API directly: it shells out to the `claude`
binary in headless mode, inheriting whatever auth the user already has
(subscription or Bedrock). Default model: the CLI alias `haiku` (resolves to
the current Haiku, today `claude-haiku-4-5`); overridable.

```go
// Digest is what the model sees about one closed leg — enough to name it,
// small enough to batch.
type Digest struct {
    Key       string   // stable identity (LegKey)
    Class     string   // "fix", "test", …
    Label     string   // current heuristic label ("" ok)
    Files     []string // Leg.Files
    Waypoints []string // Waypoint texts, in order
    Prompt    string   // latest user prompt before the leg (context, ≤120 runes)
}

// Label is one narrated result.
type Label struct {
    Key  string
    Text string // ≤32 runes, lowercase-first prose, no trailing period
}

// LegKey is the cache/dedupe identity of a leg: sessionID + "/" +
// Start.UnixNano (base 10) + "/" + Class.String().
func LegKey(sessionID string, l journey.Leg) string

// Runner produces labels for digests. CLIRunner is the real one; tests fake it.
type Runner interface {
    Narrate(digests []Digest) ([]Label, error)
}

// CLIRunner shells: <Bin> -p --model <Model> --output-format json <prompt>
// with its working directory set to Dir (a compass-private dir, so the
// narration session's own transcript never pollutes the fleet — see the fleet
// section). 60s timeout. Bin default "claude", Model default "haiku".
type CLIRunner struct {
    Bin   string
    Model string
    Dir   string
}

// Args returns the exact argv (excluding Bin) for a batch — pure, testable.
func (r *CLIRunner) Args(digests []Digest) []string

// ParseResponse extracts labels from the CLI's JSON envelope: the envelope's
// "result" field holds the model's text, which must contain a JSON array of
// {"key","label"} objects (optionally inside ``` fences). Unknown keys are
// ignored; labels are clipped to 32 runes; a malformed envelope or array is an
// error and applies nothing.
func ParseResponse(out []byte) ([]Label, error)

// Cache is a file-backed label store (append-only JSONL of {key,label}; last
// write wins on load; malformed lines skipped). Concurrent-safe.
type Cache struct{ /* opaque */ }

func OpenCache(path string) (*Cache, error)
func (c *Cache) Get(key string) (string, bool)
func (c *Cache) Put(key, label string) error

// Narrator orchestrates: dedupe, cache, one in-flight batch at a time.
type Narrator struct{ /* opaque */ }

// New builds a narrator; notify is called (any goroutine) after new labels
// land in the cache — the UI uses it to trigger a redraw.
func New(r Runner, c *Cache, notify func()) *Narrator

// Labels returns the cached labels for the trail's CLOSED legs, keyed by
// LegKey. Pure lookup, no I/O beyond the in-memory cache.
func (n *Narrator) Labels(sessionID string, tr journey.Trail) map[string]string

// Request narrates the trail's closed, uncached, not-in-flight legs in one
// async batch (goroutine; at most one batch in flight per Narrator; a leg is
// never narrated twice — in-flight keys are remembered even on failure until
// a later Request retries them after backoff of one call). No-op when
// everything is cached or a batch is already running. prompt is the latest
// user prompt (context for the whole batch).
func (n *Narrator) Request(sessionID string, tr journey.Trail, prompt string)
```

The narration prompt (built by Args) instructs: return ONLY a JSON array of
{"key","label"}; ≤5 words per label; label describes the work, not the class.

## package fleet — exclusion

```go
// ExcludeCWD hides sessions whose CWD equals path (the narrator's Dir):
// compass must never watch itself narrate.
func (m *Manager) ExcludeCWD(path string)
```

## package ui — M3 additions

```go
// TrailOpts replaces RenderTrail's positional tail. SIGNATURE CHANGE — call
// sites and goldens update with it.
type TrailOpts struct {
    Todos  []todo.Item
    Labels map[string]string // narrated overlay, keyed by LegKey; nil ok
    Now    time.Time
    Width, Height int
    Level  int // 1, 2 or 3 (trail renders identically at 2 and 3)
    Cursor int // trail-row cursor at Lv2+ over selectable rows; -1 = none
}

func RenderTrail(tr journey.Trail, o TrailOpts) string

// RenderReader renders the Lv3 conversation panel — the session's events as a
// scrollable, foldable document. Pure, golden-testable.
type ReaderOpts struct {
    Width, Height int
    Scroll        int          // top line index into the flattened document
    Unfolded      map[int]bool // event indices whose tool output is unfolded
    Query         string       // current search ("" = none); matches highlighted
}
func RenderReader(events []transcript.Event, o ReaderOpts) string
```

- **Narrated labels** overlay heuristic ones at render time (all levels):
  `label · file.go` becomes the narrated text alone. HEAD (open leg) always
  keeps its live heuristic label — narration is for history.
- **Lv3 layout**: the session view (SPEC #18, #19): trail (45%, 38–96) on
  the left | reader (the rest) on the right; the mirror gives way to the
  reader at Lv3. Below 110 cols the reader has the whole width.
- **Reader document**, newest LAST (chronological, like the CLI itself):
  prompts as `❯ <text>` header rows (accent), assistant text wrapped to width,
  tool calls as one-liners `⏺ Bash(pytest -x)` (input summary like the M0
  Activity derivation, paths relative to the session's cwd, the argument dim),
  each result folded to `  ⎿ <first line that says something> · +<n> lines`
  (dim; a Read or a listing is counted, `⎿ 120 lines`; errors `⎿ ✗ first error
  line` in red accent, still folded); unfolding shows up to 20 result lines
  under a `⎿ <n> lines` row. Prose wraps at 100 columns however wide the panel
  is; headings bold, fenced code dim, `**` dropped; a `❯` turn carries its
  clock on the right. Sidechain events are skipped. Thinking is
  already absent from Event.Text.
- **Keys by level** — j/k: Lv1 fleet · Lv2 trail cursor (selectable rows =
  legs, waypoints, branches; cursor row inverted) · Lv3 reader scroll (plus
  ctrl+d/ctrl+u half page, g/G ends). `Enter` at Lv2 on a row → Lv3 with the
  reader scrolled so that row's moment (first event at/after its time) is the
  top line. `Space` Lv3: toggle fold of the tool result at/above the top line…
  no — of the FIRST folded result visible on screen at/below the cursor line
  (cursor = top line; keep it simple and document it in help). `/` Lv3: type a
  query in the footer, Enter commits, n/N jump matches, Esc clears the query;
  Esc with no query zooms out. Tab at Lv3: footer note "this is the deepest
  level". Shift+Tab: 3→2.
- **ask the trail** (`a`, any level): suspend the TUI (tea.ExecProcess) and run
  `claude --append-system-prompt <historian>` with cwd = the session's CWD.
  The historian preamble names the session (title, branch, state) and the
  transcript path, and instructs: read the transcript first, answer questions
  about this session's journey, cite timestamps. Exit returns to compass. A
  missing binary → footer note, no crash.
  `BuildAsk(info fleet.SessionInfo) *exec.Cmd` is exported for testing (pure
  construction; no start).
- **Wiring**: cmd/compass gains `-narrator` (default "haiku"; "off" disables)
  and threads it in. Narrator Dir = `<user cache dir>/compass/narrator`;
  cache file = `<user cache dir>/compass/labels.jsonl` (os.UserCacheDir;
  fallback to the -root dir). Manager.ExcludeCWD(narrator Dir). App calls
  narrator.Request for the selected session when its trail changes, and
  overlays Labels on every render. notify → redraw message.

## Test contract

| # | Scenario | Expects |
|---|----------|---------|
| T45 | CLIRunner.Args: 2 digests | argv exactly `-p --model haiku --output-format json <prompt>`; prompt contains marshaled digests and the JSON-only instruction; custom Model honored |
| T46 | ParseResponse: clean envelope; fenced array; extra keys; 40-rune label clipped to 32; malformed envelope / non-JSON result / non-array → error, nil labels | narrator section |
| T47 | Cache: put/get; reopen reads back; last-write-wins; malformed line skipped; concurrent puts race-free | narrator section |
| T48 | Narrator: only closed legs digested (HEAD excluded); cached keys skipped; second Request while in flight = no second runner call; failed batch retried on a later Request; notify fired after labels land | narrator section |
| T49 | Manager.ExcludeCWD hides a session at that cwd; others unaffected | fleet section |
| T50 | RenderReader golden 60×24: prompt, assistant text (wrapped), tool one-liner, folded result with line count, error result marker; unfolded variant; search highlight variant | ui section |
| T51 | Lv2 cursor: selectable-row enumeration over the T43 trail (legs+waypoints+branches, top-down), cursor render, Enter → anchor time of each row maps to the first event at/after it | ui section |
| T52 | BuildAsk: cmd path "claude", args contain --append-system-prompt with the transcript path and title inside; Dir = session CWD | ui section |
| T53 | Labels overlay golden: T31 trail + a labels map → narrated text replaces `verb label` for closed legs, HEAD unchanged | ui section |

Goldens under `testdata/golden/`, ASCII profile forced. All offline — the real
CLI is never invoked in tests.
