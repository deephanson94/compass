package narrator

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// Cache is a file-backed label store: an append-only JSONL of {key,label}
// lines, read whole on open (last write wins) and appended to on every Put.
// Append-only is the point — narration is expensive enough that a label, once
// earned, should survive a crash mid-write of everything after it.
type Cache struct {
	mu     sync.RWMutex
	path   string
	labels map[string]string
}

// cacheLine is one record on disk.
type cacheLine struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// maxCacheLine bounds one JSONL record, so a corrupted file cannot make the
// scanner allocate without limit.
const maxCacheLine = 1 << 16

// OpenCache loads the labels at path, creating nothing: a file that is not
// there yet is simply an empty cache, and the first Put brings it into being.
// Malformed lines are skipped — a half-written tail costs the labels above it
// nothing.
func OpenCache(path string) (*Cache, error) {
	if path == "" {
		return nil, errors.New("narrator: empty cache path")
	}
	c := &Cache{path: path, labels: make(map[string]string)}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 4096), maxCacheLine)
	for sc.Scan() {
		var line cacheLine
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil || line.Key == "" {
			continue // unreadable line: the rest of the file is still good
		}
		c.labels[line.Key] = line.Label // later lines overwrite earlier ones
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return c, nil
}

// Get returns the cached label for a key.
func (c *Cache) Get(key string) (string, bool) {
	if c == nil {
		return "", false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	label, ok := c.labels[key]
	return label, ok
}

// Put records a label in memory and appends it to the file. The append is one
// write of one line under the lock, which is what keeps concurrent puts from
// interleaving halves of each other.
func (c *Cache) Put(key, label string) error {
	if c == nil {
		return errors.New("narrator: nil cache")
	}
	if key == "" {
		return errors.New("narrator: empty cache key")
	}
	rec, err := json.Marshal(cacheLine{Key: key, Label: label})
	if err != nil {
		return err
	}
	rec = append(rec, '\n')

	c.mu.Lock()
	defer c.mu.Unlock()
	c.labels[key] = label

	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(c.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(rec); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
