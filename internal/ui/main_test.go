package ui

import (
	"os"
	"testing"
	"time"
)

// The golden frames carry clocks — the reader's header, the trail's day
// stamps — and the deck draws those in local time, as it should. The
// fixtures were recorded in UTC, so on a machine eight hours east every
// frame with a clock in it read as a mismatch. The suite pins the zone
// rather than the frames: a golden is a picture of the deck at one
// instant, and the instant should not move with the machine.
func TestMain(m *testing.M) {
	time.Local = time.UTC
	os.Exit(m.Run())
}
