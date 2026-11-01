package fleet

import "time"

// What `compass perf` needs from the fleet: the discovery walk with every file
// reported, and the resume cache read without ever being written. Nothing here
// is on the deck's path.

// ProbeScan runs the discovery walk once and reports every transcript it kept.
// A nil cache is a cold walk, every file peeked; otherwise the walk starts from
// the cache's last scan, as a launching deck does. The cache is not updated.
func ProbeScan(root string, c *ResumeCache, quiet time.Time) ([]SessionInfo, []ScanFile, error) {
	var files []ScanFile
	infos, _, err := scanProjects(root, c.seed(), quiet, func(f ScanFile) { files = append(files, f) })
	return infos, files, err
}

// OpenResumeCacheReadOnly reads the cache at path as OpenResumeCache does, but
// Save on it does nothing: a diagnostic that measures the cache must not be
// what changes it.
func OpenResumeCacheReadOnly(path string) *ResumeCache {
	c := OpenResumeCache(path)
	c.path = ""
	return c
}

// CacheStats counts what a resume cache holds: a saved read position for each
// live session, a saved journey for each board column, and a discovery peek
// for each transcript.
type CacheStats struct {
	Points, Journeys, Peeked int
}

// Stats counts the cache's entries.
func (c *ResumeCache) Stats() CacheStats {
	if c == nil {
		return CacheStats{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return CacheStats{Points: len(c.points), Journeys: len(c.journeys), Peeked: len(c.peeked)}
}

// Point returns the saved read position for a transcript, if there is one.
func (c *ResumeCache) Point(key string) (ResumePoint, bool) { return c.point(key) }
