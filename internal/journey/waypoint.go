package journey

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// WaypointKind says what a waypoint is, so the renderer knows how to decorate
// it — the Text itself carries no glyphs or prefixes.
type WaypointKind int

const (
	// WaypointTestRun is a parsed test-run summary, e.g. "18 passed · 2 failed".
	WaypointTestRun WaypointKind = iota
	// WaypointTestFail is one failing test's name.
	WaypointTestFail
	// WaypointBug is the first line of a distinct error the leg hit.
	WaypointBug
	// WaypointCommit is a commit subject or PR URL from a ship result.
	WaypointCommit
)

// Waypoint is one Lv2 detail row under a leg: the things worth knowing about
// the work beyond its class. Extraction rules live in docs/dev/M2-CONTRACT.md.
type Waypoint struct {
	Kind WaypointKind
	Text string // ≤60 runes, undecorated
	At   time.Time
}

// maxWaypoints caps how many waypoints a leg keeps; overflow drops silently.
const maxWaypoints = 8

// waypointText is how wide a waypoint may read — the same 60 runes a prompt
// gets, because the panel gives them the same row.
const waypointText = 60

// maxTestFails and maxBugs cap the two kinds that come in floods: a broken
// suite fails a hundred tests and a broken build repeats one error until it is
// fixed. Three of each is enough to recognise what went wrong.
const (
	maxTestFails = 3
	maxBugs      = 3
)

// runner is the segmenter's memory of a Test or Ship tool_use (rule 2): only
// the results of remembered ids get test or commit parsing, and the keyword
// says which runner wrote the output that is coming back.
type runner struct {
	kind    Class  // Test or Ship
	keyword string // "pytest", "go", "cargo", "commit"…
}

// maxRunners bounds that memory. Entries are evicted when their result lands,
// so the map only holds calls still in flight; the cap is the backstop for a
// session whose results never arrive.
const maxRunners = 64

// testWaypoints parses a test runner's output (rule 3): the first family that
// recognises the text wins, and an unrecognised failure is still worth a row.
// keyword is the vote's runner word — the families barely overlap, so it only
// ever puts the likeliest one first.
func testWaypoints(text, keyword string, isErr bool, at time.Time) []Waypoint {
	lines := strings.Split(text, "\n")
	for _, i := range familyOrder(keyword) {
		if wps, ok := testFamilies[i].parse(lines, at); ok {
			return wps
		}
	}
	if isErr {
		return []Waypoint{{Kind: WaypointTestRun, Text: "failed", At: at}}
	}
	return nil
}

// testFamily is one runner's output shape: the vote keywords that point at it
// and a parser reporting whether the text was recognisably its own.
type testFamily struct {
	keywords []string
	parse    func(lines []string, at time.Time) ([]Waypoint, bool)
}

// testFamilies is the contract's order: pytest, go test, jest/vitest, cargo.
var testFamilies = []testFamily{
	{keywords: []string{"pytest", "tox"}, parse: parsePytest},
	{keywords: []string{"go"}, parse: parseGoTest},
	{keywords: []string{"jest", "vitest"}, parse: parseJest},
	{keywords: []string{"cargo"}, parse: parseCargo},
}

// familyOrder returns the family indices to try: the keyword's own family
// first, then the rest in contract order.
func familyOrder(keyword string) []int {
	hint := -1
	if keyword != "" {
		for i := range testFamilies {
			for _, k := range testFamilies[i].keywords {
				if k == keyword {
					hint = i
				}
			}
		}
	}
	order := make([]int, 0, len(testFamilies))
	if hint >= 0 {
		order = append(order, hint)
	}
	for i := range testFamilies {
		if i != hint {
			order = append(order, i)
		}
	}
	return order
}

// pytestWord is the vocabulary of pytest's terminal counts line; anchoring the
// whole line against it is what keeps jest's "Tests:" line out of this family.
const pytestWord = `(?:passed|failed|errors?|skipped|xfailed|xpassed|deselected|warnings?)`

var (
	// "===== 2 failed, 18 passed, 1 error in 0.42s =====" (the rule is optional
	// under -q: "2 failed, 18 passed in 0.42s").
	pytestSummary = regexp.MustCompile(`^=*\s*\d+ ` + pytestWord + `(?:,? \d+ ` + pytestWord + `)*(?: in [^=]*)?\s*=*$`)
	pytestCount   = regexp.MustCompile(`(\d+) (passed|failed|errors?)`)
	// "FAILED tests/test_loader.py::TestLoad::test_empty - AssertionError: …"
	pytestFailed = regexp.MustCompile(`^FAILED \S+?::(\S+)`)
)

// parsePytest reads pytest's summary counts and its FAILED lines. Errors count
// as failures: a test that could not run did not pass.
func parsePytest(lines []string, at time.Time) ([]Waypoint, bool) {
	matched := false
	summary := ""
	var fails []string
	for _, line := range lines {
		l := strings.TrimSpace(line)
		if summary == "" && pytestSummary.MatchString(l) {
			matched = true
			summary = composeCounts(pytestCounts(l))
			continue
		}
		if m := pytestFailed.FindStringSubmatch(l); m != nil {
			matched = true
			fails = append(fails, m[1])
		}
	}
	if !matched {
		return nil, false
	}
	return runWaypoints(summary, fails, at), true
}

func pytestCounts(line string) (passed, failed int) {
	for _, m := range pytestCount.FindAllStringSubmatch(line, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if m[2] == "passed" {
			passed += n
		} else {
			failed += n
		}
	}
	return passed, failed
}

// "--- FAIL: TestSegmenterSplits/subtest (0.00s)"
var goFail = regexp.MustCompile(`^\s*--- FAIL: (\S+)`)

// parseGoTest counts go test's per-test failures; a clean run only says "ok",
// which is all `go test` itself says.
func parseGoTest(lines []string, at time.Time) ([]Waypoint, bool) {
	var fails []string
	ok := false
	for _, line := range lines {
		if m := goFail.FindStringSubmatch(line); m != nil {
			fails = append(fails, m[1])
			continue
		}
		// "ok  	github.com/deephanson94/compass/internal/journey	0.01s"
		if l := strings.TrimSpace(line); l == "ok" || strings.HasPrefix(l, "ok ") || strings.HasPrefix(l, "ok\t") {
			ok = true
		}
	}
	fails = dedupe(fails)
	switch {
	case len(fails) > 0:
		return runWaypoints(strconv.Itoa(len(fails))+" failing", fails, at), true
	case ok:
		return runWaypoints("ok", nil, at), true
	}
	return nil, false
}

var (
	// "Tests:       2 failed, 18 passed, 20 total" (vitest drops the colon:
	// "Tests  2 failed | 18 passed (20)").
	jestTests = regexp.MustCompile(`^\s*Tests:?\s+\d`)
	jestCount = regexp.MustCompile(`(\d+) (passed|failed)`)
	// "✕ renders the header (3 ms)" — vitest uses × instead.
	jestFail = regexp.MustCompile(`^\s*[✕×]\s+(\S.*)$`)
)

// parseJest reads jest's and vitest's shared summary line and their failing
// test rows.
func parseJest(lines []string, at time.Time) ([]Waypoint, bool) {
	matched := false
	summary := ""
	var fails []string
	for _, line := range lines {
		if summary == "" && jestTests.MatchString(line) {
			matched = true
			summary = composeCounts(jestCounts(line))
			continue
		}
		if m := jestFail.FindStringSubmatch(line); m != nil {
			matched = true
			fails = append(fails, strings.TrimSpace(m[1]))
		}
	}
	if !matched {
		return nil, false
	}
	return runWaypoints(summary, fails, at), true
}

func jestCounts(line string) (passed, failed int) {
	for _, m := range jestCount.FindAllStringSubmatch(line, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if m[2] == "passed" {
			passed += n
		} else {
			failed += n
		}
	}
	return passed, failed
}

var (
	// "test result: FAILED. 18 passed; 2 failed; 0 ignored; 0 measured"
	cargoResult = regexp.MustCompile(`^\s*test result: (?:ok|FAILED)\.\s*(\d+) passed;\s*(\d+) failed`)
	// "test loader::tests::empty_input ... FAILED"
	cargoFail = regexp.MustCompile(`^\s*test (\S+) \.\.\. FAILED`)
)

// parseCargo reads cargo test's result line and its FAILED rows.
func parseCargo(lines []string, at time.Time) ([]Waypoint, bool) {
	matched := false
	summary := ""
	var fails []string
	for _, line := range lines {
		if m := cargoResult.FindStringSubmatch(line); summary == "" && m != nil {
			matched = true
			passed, _ := strconv.Atoi(m[1])
			failed, _ := strconv.Atoi(m[2])
			summary = composeCounts(passed, failed)
			continue
		}
		if m := cargoFail.FindStringSubmatch(line); m != nil {
			matched = true
			fails = append(fails, m[1])
		}
	}
	if !matched {
		return nil, false
	}
	return runWaypoints(summary, fails, at), true
}

// runWaypoints assembles one family's findings: the run summary first, then the
// tests that failed — deduped and capped, because a leg is a glance.
func runWaypoints(summary string, fails []string, at time.Time) []Waypoint {
	var wps []Waypoint
	if summary != "" {
		wps = append(wps, Waypoint{Kind: WaypointTestRun, Text: clip(summary, waypointText), At: at})
	}
	for i, name := range dedupe(fails) {
		if i >= maxTestFails {
			break
		}
		wps = append(wps, Waypoint{Kind: WaypointTestFail, Text: clip(name, waypointText), At: at})
	}
	return wps
}

// composeCounts writes the run summary the panel shows, omitting whichever
// half is zero: "18 passed · 2 failed", "18 passed", "2 failed".
func composeCounts(passed, failed int) string {
	var parts []string
	if passed > 0 {
		parts = append(parts, strconv.Itoa(passed)+" passed")
	}
	if failed > 0 {
		parts = append(parts, strconv.Itoa(failed)+" failed")
	}
	return strings.Join(parts, " · ")
}

var (
	// "[main (root-commit) 1a2b3c4] wire the segmenter into the panel"
	commitLine = regexp.MustCompile(`^\[[^\]]+ [0-9a-f]{7,40}\] (\S.*)$`)
	// any token carrying a pull-request URL, however the tool framed the line
	pullURL = regexp.MustCompile(`\S*github\.com/\S*/pull/\S*`)
)

// shipWaypoints reads what shipping produced (rule 5): the commit that landed
// or the pull request it opened.
func shipWaypoints(text string, at time.Time) []Waypoint {
	var wps []Waypoint
	for _, line := range strings.Split(text, "\n") {
		l := strings.TrimSpace(line)
		if m := commitLine.FindStringSubmatch(l); m != nil {
			wps = append(wps, Waypoint{Kind: WaypointCommit, Text: clip(strings.TrimSpace(m[1]), waypointText), At: at})
			continue
		}
		if url := pullURL.FindString(l); url != "" {
			wps = append(wps, Waypoint{Kind: WaypointCommit, Text: clip(url, waypointText), At: at})
		}
	}
	return wps
}

// firstNonEmptyLine is the one line worth keeping out of a wall of output: the
// error's own sentence, the subagent's verdict.
func firstNonEmptyLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if l := strings.TrimSpace(line); l != "" {
			return l
		}
	}
	return ""
}

// dedupe keeps first-seen order and drops repeats — one failing test named
// three times is still one failing test.
func dedupe(names []string) []string {
	if len(names) < 2 {
		return names
	}
	seen := make(map[string]bool, len(names))
	out := names[:0:0]
	for _, n := range names {
		if seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}
