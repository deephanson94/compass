package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// clip decodes only the runes up to its cut, measures printable ASCII without
// asking lipgloss, and stops measuring a long string once its start is already
// too wide. None of that may change a single clipped row: every case below is
// clipped by clip and by the implementation it replaced, at every width.
func TestClipCutsWhereItAlwaysCut(t *testing.T) {
	long := strings.Repeat("ok=1 line of tool output ", 400)
	cases := []string{
		"", "a", "go test ./...", "API Error: 429 rate limited", "API Error: 4031234",
		"porter_tui — the rest", "--model x -y", `checkout-flake-hunt · "quoted words`,
		"backfill.", "a, b; c", "日本語のテキストです、とても長い", "emoji 🧭🧭🧭 compass 👩‍💻 dev",
		"\x1b[38;5;2mgreen\x1b[0m text after the colour", "tab\there and\x7fdel",
		"e\u0301 combining marks e\u0301e\u0301", long, "日本" + long, long + "日本語",
		strings.Repeat("9", 5000), strings.Repeat("\x1b[1m", 3000) + "short",
	}
	for _, s := range cases {
		for w := 0; w <= 130; w++ {
			if got, want := clip(s, w), clipReference(s, w); got != want {
				t.Fatalf("clip(%.40q, %d) = %q, the old clip gave %q", s, w, got, want)
			}
		}
	}
}

// clipReference is clip as it was before it learned to stop early, kept
// verbatim as the measure of the new one.
func clipReference(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	// Cells, not runes: one CJK ideograph or emoji occupies two columns, and a
	// panel measured in runes overflows into its neighbour. Session titles,
	// branches and tmux window names are all arbitrary user text.
	var b strings.Builder
	used := 0
	runes := []rune(s)
	kept := 0
	for _, r := range runes {
		cw := lipgloss.Width(string(r))
		if used+cw > w-1 {
			break
		}
		b.WriteRune(r)
		used += cw
		kept++
	}
	if kept < len(runes) && kept > 0 && isDigit(runes[kept-1]) && isDigit(runes[kept]) {
		// Never cut a number in half: "API Error: 4…" read as a one-digit
		// status, and 403 against 429 is the difference the row is for.
		// The whole number goes instead (#53) — and only then: a number
		// whole on screen stays, however the row ends after it (#59).
		for kept > 0 && isDigit(runes[kept-1]) {
			kept--
		}
	}
	// "go test ./...…", "go test ./…": a mark after a dot or a slash reads
	// as more of the token it cut, whichever rune the cut fell on. The
	// trailing run of dots and slashes goes — "backfill." is "backfill…",
	// "go test ./..." is "go test…" — never the token, which cost a
	// wider column the name of the hung run (#58, #59, #61). The
	// separator, a bracket and the spaces they stood on go with it.
	// A dash that stands alone is a separator too ("porter_tui —…"
	// promised a phrase), where a hyphen inside a token is the token's
	// (`--all`, `-run`): the spaced dashes go, the hyphen stays (#63).
	// A comma or a semicolon promises the clause after it the same way (#64).
	// And a flag's leading dash with nothing after it: "--model x -…"
	// promised a flag; the hyphen inside a token stays (#72).
	head := strings.TrimRight(string(runes[:kept]), " ·(./—–,;")
	for strings.HasSuffix(head, " -") || strings.HasSuffix(head, " --") {
		head = strings.TrimRight(strings.TrimRight(head, "-"), " ·(./—–,;")
	}
	// An opening quote with nothing after it promises the words it was
	// about to hold: `checkout-flake-hunt · "…` spent four cells on no
	// prompt at all. The quote goes with the separator that led it, and
	// only while it is unclosed (#80).
	for strings.HasSuffix(head, `"`) && strings.Count(head, `"`)%2 == 1 {
		head = strings.TrimRight(strings.TrimSuffix(head, `"`), " ·(./—–,;")
	}
	return head + "…"
}
