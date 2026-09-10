package transcript

import (
	"bytes"
	"io"
	"os"
	"strings"
)

// Tailer incrementally reads one transcript file. It remembers how far it has
// read and holds any trailing partial line until its newline arrives, so a line
// that is written in two flushes is delivered exactly once, whole.
//
// A Tailer is not safe for concurrent use.
type Tailer struct {
	path    string
	offset  int64
	carry   []byte
	skipped int

	// src, when set, is a registered scheme's source standing in for the
	// file: a session whose record is not a JSONL file (an opencode
	// session in its SQLite store) is tailed through it, and the file
	// fields above stay idle.
	src Source
}

// Source is what a non-file transcript answers to: each Poll returns the
// events written since the last, in order, and nil when nothing has.
type Source interface {
	Poll() ([]Event, error)
}

// schemes are the registered openers for paths of the form scheme://rest.
var schemes = map[string]func(path string) Source{}

// RegisterScheme names a source for paths beginning scheme://. NewTailer
// hands such a path to the opener and every Poll goes through it, so the
// rest of compass keys and tails the session exactly as it would a file.
func RegisterScheme(scheme string, open func(path string) Source) {
	schemes[scheme] = open
}

// SchemeOf is the registered scheme a path names, or "" for a file.
func SchemeOf(path string) string {
	i := strings.Index(path, "://")
	if i <= 0 {
		return ""
	}
	if _, ok := schemes[path[:i]]; ok {
		return path[:i]
	}
	return ""
}

// NewTailer returns a Tailer positioned at the start of path. The file need not
// exist yet.
func NewTailer(path string) *Tailer {
	t := &Tailer{path: path}
	if scheme := SchemeOf(path); scheme != "" {
		t.src = schemes[scheme](path)
	}
	return t
}

// Path is the transcript file this Tailer follows.
func (t *Tailer) Path() string { return t.path }

// Poll reads newly appended bytes since the last call and returns the parsed
// events of every complete ("\n"-terminated) line. A trailing partial line is
// held back until its newline arrives. Malformed-JSON lines are skipped and
// counted (see Skipped), never returned and never fatal to the batch. If the
// file shrank below the stored offset (truncate or rotate) the offset resets to
// zero and the file is re-read from the start. A missing file returns
// (nil, nil): the session may simply not have flushed yet.
func (t *Tailer) Poll() ([]Event, error) {
	if t.src != nil {
		return t.src.Poll()
	}
	fi, err := os.Stat(t.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	size := fi.Size()
	if size < t.offset {
		t.offset = 0
		t.carry = nil
	}
	if size == t.offset {
		return nil, nil
	}

	f, err := os.Open(t.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	if _, err := f.Seek(t.offset, io.SeekStart); err != nil {
		return nil, err
	}
	chunk, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	t.offset += int64(len(chunk))
	if len(chunk) == 0 {
		return nil, nil
	}

	buf := chunk
	if len(t.carry) > 0 {
		buf = append(append([]byte(nil), t.carry...), chunk...)
		t.carry = nil
	}

	var events []Event
	for {
		i := bytes.IndexByte(buf, '\n')
		if i < 0 {
			break
		}
		line := buf[:i]
		buf = buf[i+1:]
		if ev, ok := t.parse(line); ok {
			events = append(events, ev)
		}
	}
	if len(buf) > 0 {
		t.carry = append([]byte(nil), buf...)
	}
	return events, nil
}

// parse turns one complete line into an event, counting malformed JSON. Blank
// lines are not malformed — they are nothing — so they are dropped uncounted.
func (t *Tailer) parse(line []byte) (Event, bool) {
	line = bytes.TrimRight(line, "\r")
	if len(bytes.TrimSpace(line)) == 0 {
		return Event{}, false
	}
	ev, err := ParseLine(line)
	if err != nil {
		t.skipped++
		return Event{}, false
	}
	return ev, true
}

// Skipped is the number of malformed-JSON lines dropped so far.
func (t *Tailer) Skipped() int { return t.skipped }

// Mark is where a Tailer has read to: the byte offset, and any partial last
// line it is still holding. It lets a later process resume a transcript
// instead of re-reading it from the start.
type Mark struct {
	Offset int64  `json:"offset"`
	Carry  string `json:"carry,omitempty"`
}

// Mark returns the Tailer's position.
func (t *Tailer) Mark() Mark {
	if t.src != nil {
		return Mark{} // a source has no byte to resume at
	}
	return Mark{Offset: t.offset, Carry: string(t.carry)}
}

// Resume places a Tailer at a mark taken earlier, possibly by another process.
// A mark past the end of the file — the transcript was truncated or replaced
// since — is refused, and the Tailer stays at the start: re-reading a file is
// slow, but resuming into the middle of one that is no longer the same file
// would report a state that never existed. Reports whether the mark was taken.
func (t *Tailer) Resume(m Mark) bool {
	if m.Offset <= 0 || t.src != nil {
		return false
	}
	fi, err := os.Stat(t.path)
	if err != nil || fi.Size() < m.Offset {
		return false
	}
	t.offset, t.carry = m.Offset, []byte(m.Carry)
	return true
}
