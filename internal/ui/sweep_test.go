package ui

import "testing"

// sweep marks a panel sweep: a pin that presses every key on every scene at
// five widths, often under both colour profiles, to hold a decision as a
// rule rather than on one frame. Thirty-two of them take three quarters of
// the package's four hundred seconds, so they step aside under -short and
// the default developer run stays under two minutes. CI and any run that
// wants the rules held runs without -short (with -timeout 40m).
func sweep(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("a panel sweep: run without -short to hold the rule")
	}
}
