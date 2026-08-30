package ui

import (
	"sync"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/transcript"
)

// feed is one session's journey: its own tailer, reading the transcript from
// the top the first time it is asked, and the segmenter it pours into. The
// fleet Manager has a tailer of its own for the state machine; this one is
// separate so a session that is never selected costs nothing.
type feed struct {
	tailer *transcript.Tailer
	seg    *journey.Segmenter
	trail  journey.Trail

	// events is the transcript itself, kept because the Lv3 reader renders it
	// as a document. A very long session is held to the newest feedEventCap
	// events: the reader is a conversation, not an archive.
	events []transcript.Event
}

// feedEventCap bounds what one session's reader can hold. Twenty thousand
// events is far more than any session a person reads through, and the events
// themselves are already bounded (transcript clamps result text), so the ring
// costs tens of megabytes at worst on the one session that hits it.
const feedEventCap = 20000

// feedStore keeps one feed per session. The deck's commands run off the render
// loop and can overlap, so the store carries its own lock.
type feedStore struct {
	mu    sync.Mutex
	feeds map[string]*feed
}

func newFeedStore() *feedStore {
	return &feedStore{feeds: make(map[string]*feed)}
}

// poll reads whatever the session has written since the last call and returns
// its trail and the events behind it — the graph for the right-hand column, the
// document for the Lv3 reader. The first call replays the whole transcript — a
// session's journey starts at its first line, not at the moment compass looked
// at it.
func (fs *feedStore) poll(id, path string) (journey.Trail, []transcript.Event) {
	if fs == nil || id == "" || path == "" {
		return journey.Trail{}, nil
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()

	f := fs.feeds[id]
	if f == nil || f.tailer.Path() != path {
		f = &feed{tailer: transcript.NewTailer(path), seg: journey.NewSegmenter()}
		fs.feeds[id] = f
	}

	events, err := f.tailer.Poll()
	if err != nil || len(events) == 0 {
		// A file we cannot read keeps the trail we already drew.
		return f.trail, f.events
	}
	for _, ev := range events {
		f.seg.Observe(ev)
	}
	f.trail = f.seg.Trail()
	f.remember(events)
	return f.trail, f.events
}

// remember appends the new events, dropping the oldest once the ring is full.
// The trim copies into a fresh slice rather than sliding the old one: a
// previous caller may still be rendering the events it was handed, and the
// deck's commands run off the render loop.
func (f *feed) remember(events []transcript.Event) {
	f.events = append(f.events, events...)
	if over := len(f.events) - feedEventCap; over > 0 {
		f.events = append([]transcript.Event(nil), f.events[over:]...)
	}
}

// retain drops the feeds of sessions that are no longer in the fleet.
func (fs *feedStore) retain(sessions []fleet.Session) {
	if fs == nil {
		return
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()

	live := make(map[string]bool, len(sessions))
	for _, s := range sessions {
		live[s.Info.ID] = true
	}
	for id := range fs.feeds {
		if !live[id] {
			delete(fs.feeds, id)
		}
	}
}
