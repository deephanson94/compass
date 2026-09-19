package ui

import (
	"testing"
	"time"
)

// longStand is the very-long scene with its lead session's trail grown to
// `legs` legs, opened on that session at the given level.
func longStand(legs, w, h int, keys ...string) *Model {
	sc := sceneVeryLong()
	k := sessionKey("auth")
	tr := sc.trails[k]
	grown := dayLongTrail("auth", legs, sceneNow.Add(-time.Duration(legs)*7*time.Minute), true)
	grown.Tasks, grown.Compactions = tr.Tasks, nil
	sc.trails[k] = grown
	m := sceneModel(sc, w, h)
	for _, key := range keys {
		pressKey(m, key)
	}
	return m
}

func benchWalk(b *testing.B, legs int, keys ...string) {
	m := longStand(legs, 200, 50, keys...)
	b.ReportMetric(float64(len(m.readerEvents())), "events")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			pressKey(m, "k")
		} else {
			pressKey(m, "j")
		}
		_ = m.View()
	}
}

func BenchmarkTrailWalkLv2_160(b *testing.B)   { benchWalk(b, 160, "1", "tab") }
func BenchmarkTrailWalkLv2_1000(b *testing.B)  { benchWalk(b, 1000, "1", "tab") }
func BenchmarkTrailWalkLv2_3000(b *testing.B)  { benchWalk(b, 3000, "1", "tab") }
func BenchmarkReaderWalkLv3_3000(b *testing.B) { benchWalk(b, 3000, "1", "tab", "tab", "G") }
func BenchmarkTrailWalkLv1_3000(b *testing.B)  { benchWalk(b, 3000, "1") }
