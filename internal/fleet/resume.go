package fleet

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/deephanson94/compass/internal/state"
	"github.com/deephanson94/compass/internal/transcript"
)

// A resume cache is how `compass status` stays cheap. Every invocation is a
// fresh process, and a live session's state is folded by replaying its whole
// transcript — which on a real machine means tens of megabytes of JSON, several
// times a minute, from a tmux status line. The cache stores where the last
// process stopped reading and what it had concluded so far, so the next one
// parses only what was appended since.
//
// It is a cache in the strict sense: losing it, or refusing to trust it, costs
// time and nothing else. Every path below falls back to a full replay.

// ResumePoint is one session's saved reading position and folded state.
type ResumePoint struct {
	Mark transcript.Mark `json:"mark"`
	Fold state.Fold      `json:"fold"`
}

// ResumeCache maps a transcript path to where reading it left off, and carries
// the last discovery scan alongside it. Discovery is bounded per file — the
// head and the tail, never the middle — but 300 transcripts is still 300 opens
// and 600 reads, and archived ones have not changed since the last run.
type ResumeCache struct {
	path   string
	points map[string]ResumePoint
	peeked map[string]cachedInfo
}

// PeekedInfo is one transcript's discovery result and the (size, mtime) it was
// read at. A file whose stat still matches is not opened again.
type PeekedInfo struct {
	Size    int64       `json:"size"`
	ModTime time.Time   `json:"mtime"`
	Info    SessionInfo `json:"info"`
}

// cacheFile is what actually goes to disk. The two halves travel together
// because they are invalidated by the same thing: the file changing.
type cacheFile struct {
	Points map[string]ResumePoint `json:"points,omitempty"`
	Peeked map[string]PeekedInfo  `json:"peeked,omitempty"`
}

// OpenResumeCache reads the cache at path. A missing, unreadable or corrupt
// file is an empty cache, never an error: the only consequence is a slower
// first read.
func OpenResumeCache(path string) *ResumeCache {
	c := &ResumeCache{path: path, points: map[string]ResumePoint{}, peeked: map[string]cachedInfo{}}
	raw, err := os.ReadFile(path)
	if err != nil {
		return c
	}
	var f cacheFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return c
	}
	if f.Points != nil {
		c.points = f.Points
	}
	for path, p := range f.Peeked {
		c.peeked[path] = cachedInfo{size: p.Size, modTime: p.ModTime, info: p.Info}
	}
	return c
}

// Save writes the cache out, atomically: a status line that is killed mid-write
// must not leave a half-written file for the next one to puzzle over. A failure
// to write is silent — the next process simply replays.
func (c *ResumeCache) Save() {
	if c == nil || c.path == "" {
		return
	}
	f := cacheFile{Points: c.points, Peeked: make(map[string]PeekedInfo, len(c.peeked))}
	for path, p := range c.peeked {
		f.Peeked[path] = PeekedInfo{Size: p.size, ModTime: p.modTime, Info: p.info}
	}
	raw, err := json.Marshal(f)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(c.path), ".resume-*")
	if err != nil {
		return
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}
	_ = os.Rename(tmp.Name(), c.path)
}

// point returns the saved position for a transcript, if there is one.
func (c *ResumeCache) point(key string) (ResumePoint, bool) {
	if c == nil {
		return ResumePoint{}, false
	}
	p, ok := c.points[key]
	return p, ok
}

// record stores where a session has been read to. Only live sessions are worth
// recording: an archived one is never tailed, so it has no position to keep.
func (c *ResumeCache) record(key string, p ResumePoint) {
	if c == nil {
		return
	}
	c.points[key] = p
}

// seed hands the Manager the last scan to start from, and takes back whatever
// the scan concluded. A nil cache seeds nothing, which is a cold scan.
func (c *ResumeCache) seed() map[string]cachedInfo {
	if c == nil {
		return nil
	}
	return c.peeked
}

func (c *ResumeCache) keepScan(scan map[string]cachedInfo) {
	if c == nil {
		return
	}
	c.peeked = scan
}

// retain drops every session the cache no longer sees, so a machine that has
// churned through thousands of sessions does not carry all of them forever.
func (c *ResumeCache) retain(keep map[string]bool) {
	if c == nil {
		return
	}
	for key := range c.points {
		if !keep[key] {
			delete(c.points, key)
		}
	}
}
