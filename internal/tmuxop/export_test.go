package tmuxop

import "time"

// Exported for tests: the two halves of StartTime that a real /proc cannot
// exercise — the tick arithmetic needs years of uptime, and the boot-time
// cache needs a /proc/stat that fails.

func StartFromTicks(boot time.Time, ticks int64) time.Time { return startFromTicks(boot, ticks) }

func BootTimeFrom(path string) time.Time { return bootTimeFrom(path) }

func ResetBootTime() {
	boot.Lock()
	defer boot.Unlock()
	boot.at = time.Time{}
}
