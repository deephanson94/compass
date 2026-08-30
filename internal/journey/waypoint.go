package journey

import "time"

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
