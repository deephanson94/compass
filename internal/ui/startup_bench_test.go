package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
)

// startupHome writes a Claude home the shape a real one has at launch: `live`
// sessions still writing, each `lines` lines long with tool results of
// `result` bytes (8 live × 4000 lines × 20KB results is ~170MB of
// transcript), and `archived` short sessions that went quiet days ago. It
// returns the root and the transcript paths of the live sessions.
func startupHome(tb testing.TB, live, lines, result, archived int) (string, []string) {
	tb.Helper()
	root := tb.TempDir()
	now := time.Now()
	payload := strings.Repeat("ok=1 line of tool output that says nothing much at all\n", result/55+1)[:result]
	var paths []string
	write := func(slug, id string, n int, end time.Time) string {
		dir := filepath.Join(root, "projects", slug)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			tb.Fatal(err)
		}
		var sb strings.Builder
		start := end.Add(-time.Duration(n) * 3 * time.Second)
		line := func(o map[string]any) {
			raw, _ := json.Marshal(o)
			sb.Write(raw)
			sb.WriteByte('\n')
		}
		common := func(i int) map[string]any {
			return map[string]any{
				"parentUuid": fmt.Sprintf("%s-%d", id, i-1), "uuid": fmt.Sprintf("%s-%d", id, i),
				"isSidechain": false, "userType": "external",
				"timestamp": start.Add(time.Duration(i) * 3 * time.Second).UTC().Format("2006-01-02T15:04:05.000Z"),
				"cwd":       "/home/user/" + slug, "sessionId": id, "version": "2.1.251", "gitBranch": "main",
			}
		}
		for i := 0; i < n; i++ {
			o := common(i)
			switch i % 8 {
			case 0:
				o["type"] = "user"
				o["message"] = map[string]any{"role": "user", "content": fmt.Sprintf("please fix the %dth bug in the parser", i)}
			case 1, 5:
				o["type"] = "assistant"
				o["message"] = map[string]any{"role": "assistant", "model": "claude-fable-5", "id": fmt.Sprintf("msg_%d", i),
					"content": []any{map[string]any{"type": "text", "text": "Looking at the parser now."}}}
			case 2, 4, 6:
				o["type"] = "assistant"
				o["message"] = map[string]any{"role": "assistant", "model": "claude-fable-5", "id": fmt.Sprintf("msg_%d", i),
					"content": []any{map[string]any{"type": "tool_use", "id": fmt.Sprintf("tu_%d", i), "name": "Bash",
						"input": map[string]any{"command": "go test ./...", "description": "Run tests"}}}}
			case 3, 7:
				o["type"] = "user"
				o["message"] = map[string]any{"role": "user", "content": []any{map[string]any{
					"type": "tool_result", "tool_use_id": fmt.Sprintf("tu_%d", i-1), "is_error": false, "content": payload}}}
				o["toolUseResult"] = map[string]any{"stdout": payload, "stderr": "", "interrupted": false}
			}
			line(o)
		}
		path := filepath.Join(dir, id+".jsonl")
		if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
			tb.Fatal(err)
		}
		os.Chtimes(path, end, end)
		return path
	}
	for i := 0; i < live; i++ {
		paths = append(paths, write(fmt.Sprintf("-home-user-live%d", i), fmt.Sprintf("11111111-0000-4000-8000-%012d", i), lines, now.Add(-time.Duration(i)*10*time.Second)))
	}
	for i := 0; i < archived; i++ {
		write(fmt.Sprintf("-home-user-old%d", i%10), fmt.Sprintf("22222222-0000-4000-8000-%012d", i), 40, now.Add(-time.Duration(i+1)*24*time.Hour))
	}
	return root, paths
}

// BenchmarkStartupRefresh is the deck's first poll: a cold Manager discovers
// every transcript and replays every live one through its state machine.
func BenchmarkStartupRefresh(b *testing.B) {
	root, _ := startupHome(b, 8, 4000, 20000, 150)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mgr := fleet.NewManager(root)
		if _, err := mgr.Refresh(time.Now()); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStartupBoard is the board's first poll: every column's feed
// replays its whole transcript into a segmenter, the way refresh polls them.
func BenchmarkStartupBoard(b *testing.B) {
	_, paths := startupHome(b, 8, 4000, 20000, 0)
	targets := make([]boardTarget, len(paths))
	for i, p := range paths {
		targets[i] = boardTarget{key: p, path: p}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		newFeedStore().pollEach(targets)
	}
}

// BenchmarkStartupRefreshWarm is the deck's first poll when a resume cache
// from the last run is on disk: nothing is replayed, the scan opens no file
// whose stat has not moved, and only what was appended since is read.
func BenchmarkStartupRefreshWarm(b *testing.B) {
	root, _ := startupHome(b, 8, 4000, 20000, 150)
	cachePath := filepath.Join(b.TempDir(), "resume.json")
	// The last run: it read everything, recorded where it stopped, and
	// saved on the way out.
	last := fleet.OpenResumeCache(cachePath)
	warm := fleet.NewManager(root)
	warm.UseResumeCache(last)
	if _, err := warm.Refresh(time.Now()); err != nil {
		b.Fatal(err)
	}
	last.Save()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mgr := fleet.NewManager(root)
		mgr.UseResumeCache(fleet.OpenResumeCache(cachePath))
		if _, err := mgr.Refresh(time.Now()); err != nil {
			b.Fatal(err)
		}
	}
}
