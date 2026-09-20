package fleet

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/deephanson94/compass/internal/journey"
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

// ResumePoint is one session's saved reading position and folded state: the
// machine's, and the fleet row's own — what it was doing and what it last
// finished — which a deck restored from the cache would otherwise show blank
// until the session's next event (round 71).
type ResumePoint struct {
	Mark  transcript.Mark `json:"mark"`
	Fold  state.Fold      `json:"fold"`
	Entry EntryFold       `json:"entry,omitzero"`
}

// EntryFold is what a fleet entry learns from the events and discovery does
// not tell it again: its class, its latest outcome, the rank of its title
// and the model that last answered.
type EntryFold struct {
	Class     journey.Class        `json:"class,omitempty"`
	HasClass  bool                 `json:"has_class,omitempty"`
	TitleRank int                  `json:"title_rank,omitempty"`
	Model     string               `json:"model,omitempty"`
	Outcomes  journey.OutcomesFold `json:"outcomes,omitzero"`
}

// JourneyPoint is one transcript's saved reading position and folded journey:
// where the board's feed stopped reading and what its segmenter had built.
// It is the deck's, recorded by the feed store rather than the Manager, and
// keyed by the same transcript path.
type JourneyPoint struct {
	Mark transcript.Mark `json:"mark"`
	Fold journey.Fold    `json:"fold"`
}

// ResumeCache maps a transcript path to where reading it left off, and carries
// the last discovery scan alongside it. Discovery is bounded per file — the
// head and the tail, never the middle — but 300 transcripts is still 300 opens
// and 600 reads, and archived ones have not changed since the last run.
//
// It is safe for concurrent use: the Manager records under its own lock, and
// the deck's feeds record from a worker per column.
type ResumeCache struct {
	mu       sync.Mutex
	path     string
	points   map[string]ResumePoint
	journeys map[string]JourneyPoint
	peeked   map[string]cachedInfo
	saved    time.Time // when Save last ran, for SaveEvery
}

// PeekedInfo is one transcript's discovery result and the (size, mtime) it was
// read at. A file whose stat still matches is not opened again.
type PeekedInfo struct {
	Size    int64       `json:"size"`
	ModTime time.Time   `json:"mtime"`
	Info    SessionInfo `json:"info"`
	// Wide says that peek read the whole tail for a question rather than
	// one window. Without it every restored entry looked narrow, so the
	// scan re-read every quiet transcript on every run — 135MB a run on a
	// real home directory, for a process tmux starts every few seconds
	// (round 61).
	Wide bool `json:"wide,omitempty"`
}

// cacheFile is what actually goes to disk. The two halves travel together
// because they are invalidated by the same thing: the file changing.
type cacheFile struct {
	Points   map[string]ResumePoint  `json:"points,omitempty"`
	Journeys map[string]JourneyPoint `json:"journeys,omitempty"`
	Peeked   map[string]PeekedInfo   `json:"peeked,omitempty"`
}

// OpenResumeCache reads the cache at path. A missing, unreadable or corrupt
// file is an empty cache, never an error: the only consequence is a slower
// first read.
func OpenResumeCache(path string) *ResumeCache {
	c := &ResumeCache{path: path, points: map[string]ResumePoint{}, journeys: map[string]JourneyPoint{}, peeked: map[string]cachedInfo{}}
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
	if f.Journeys != nil {
		c.journeys = f.Journeys
	}
	for path, p := range f.Peeked {
		c.peeked[path] = cachedInfo{size: p.Size, modTime: p.ModTime, info: p.Info, wide: p.Wide}
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
	c.mu.Lock()
	f := cacheFile{Points: c.points, Journeys: c.journeys, Peeked: make(map[string]PeekedInfo, len(c.peeked))}
	for path, p := range c.peeked {
		f.Peeked[path] = PeekedInfo{Size: p.size, ModTime: p.modTime, Info: p.info, Wide: p.wide}
	}
	raw, err := json.Marshal(f)
	c.saved = time.Now()
	c.mu.Unlock()
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

// SaveEvery writes the cache out if it has been at least d since the last
// save. A deck saves on the way out, but a deck that is killed never gets
// there: with this, what it loses is at most d of appended bytes, not the
// whole run (round 71).
func (c *ResumeCache) SaveEvery(d time.Duration) {
	if c == nil {
		return
	}
	c.mu.Lock()
	due := time.Since(c.saved) >= d
	c.mu.Unlock()
	if due {
		c.Save()
	}
}

// point returns the saved position for a transcript, if there is one.
func (c *ResumeCache) point(key string) (ResumePoint, bool) {
	if c == nil {
		return ResumePoint{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.points[key]
	return p, ok
}

// record stores where a session has been read to. Only live sessions are worth
// recording: an archived one is never tailed, so it has no position to keep.
func (c *ResumeCache) record(key string, p ResumePoint) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.points[key] = p
}

// sleepEntry drops the class from a session's saved point: the fleet says
// what a session is doing only while it is live, and a session that slept
// is not doing it any more. What it last finished still stands.
func (c *ResumeCache) sleepEntry(key string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if p, ok := c.points[key]; ok && p.Entry.HasClass {
		p.Entry.Class, p.Entry.HasClass = 0, false
		c.points[key] = p
	}
}

// Journey returns the saved journey for a transcript, if there is one.
func (c *ResumeCache) Journey(key string) (JourneyPoint, bool) {
	if c == nil {
		return JourneyPoint{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.journeys[key]
	return p, ok
}

// RecordJourney stores where a transcript's journey has been read to.
func (c *ResumeCache) RecordJourney(key string, p JourneyPoint) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.journeys[key] = p
}

// seed hands the Manager the last scan to start from, and takes back whatever
// the scan concluded. A nil cache seeds nothing, which is a cold scan.
func (c *ResumeCache) seed() map[string]cachedInfo {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.peeked
}

func (c *ResumeCache) keepScan(scan map[string]cachedInfo) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.peeked = scan
}

// retain drops every session the cache no longer sees, so a machine that has
// churned through thousands of sessions does not carry all of them forever.
func (c *ResumeCache) retain(keep map[string]bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.points {
		if !keep[key] {
			delete(c.points, key)
		}
	}
	for key := range c.journeys {
		if !keep[key] {
			delete(c.journeys, key)
		}
	}
}
