package fleet

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"syscall"
	"time"
)

// Saving merges. Every compass of one person — a deck, a second deck in
// another terminal, `compass status` from a status line — keeps one resume
// cache, and each used to write its whole in-memory copy over the file. The
// last to save won: a deck left running for three days rewrote, every few
// minutes, the copy it had loaded three days before, and every session name
// a newer compass had saved since was gone again on the next restart.
//
// So a save re-reads the file under a lock and merges:
//
//   - a session only the file has is another compass's, and stays;
//   - a session both have goes to the copy that has read further into its
//     transcript (the larger mark), the file's on a tie being no newer;
//   - a discovery peek goes to the one taken of the newer file;
//   - where this process's copy wins, it is laid over the file's, so a
//     field this build does not know — a newer build's — survives it;
//   - a session this process dropped on purpose (its transcript is gone) is
//     not brought back.

// lockCache takes the cache file's lock: the read, the merge and the rename
// are one step for every compass that saves. It returns the unlock; a lock
// that cannot be taken is no lock, and the save goes ahead without one.
func lockCache(path string) func() {
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return func() {}
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return func() {}
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}
}

// mergeCache is the file to write: what is on disk (onDisk, "" or unreadable
// is nothing) with this process's sections laid over it by the rules above.
func mergeCache(onDisk []byte, ours cacheFile, dropped map[string]bool) ([]byte, error) {
	var disk map[string]json.RawMessage
	if len(onDisk) > 0 {
		_ = json.Unmarshal(onDisk, &disk) // a corrupt file is no file
	}
	if disk == nil {
		disk = map[string]json.RawMessage{}
	}
	sections := []struct {
		name  string
		ours  any
		typ   reflect.Type // one entry's type: what this build knows of it
		newer func(ours, theirs json.RawMessage) bool
	}{
		{"points", ours.Points, reflect.TypeOf(ResumePoint{}), byMark},
		{"journeys", ours.Journeys, reflect.TypeOf(JourneyPoint{}), byMark},
		{"peeked", ours.Peeked, reflect.TypeOf(PeekedInfo{}), byModTime},
	}
	for _, sec := range sections {
		raw, err := json.Marshal(sec.ours)
		if err != nil {
			return nil, err
		}
		var mine, theirs map[string]json.RawMessage
		_ = json.Unmarshal(raw, &mine)
		_ = json.Unmarshal(disk[sec.name], &theirs)
		merged := map[string]json.RawMessage{}
		for key, t := range theirs {
			if !dropped[key] {
				merged[key] = t
			}
		}
		for key, m := range mine {
			t, both := theirs[key]
			switch {
			case !both:
				merged[key] = m
			case sec.newer(m, t):
				merged[key] = overlay(t, m, sec.typ)
			}
		}
		out, err := json.Marshal(merged)
		if err != nil {
			return nil, err
		}
		if len(merged) == 0 {
			delete(disk, sec.name)
			continue
		}
		disk[sec.name] = out
	}
	return json.Marshal(disk)
}

// byMark says ours has read at least as far into the transcript as theirs.
func byMark(ours, theirs json.RawMessage) bool {
	var a, b struct {
		Mark struct {
			Offset int64 `json:"offset"`
		} `json:"mark"`
	}
	_ = json.Unmarshal(ours, &a)
	_ = json.Unmarshal(theirs, &b)
	return a.Mark.Offset >= b.Mark.Offset
}

// byModTime says ours was peeked from a file at least as new as theirs.
func byModTime(ours, theirs json.RawMessage) bool {
	var a, b struct {
		ModTime time.Time `json:"mtime"`
		Size    int64     `json:"size"`
	}
	_ = json.Unmarshal(ours, &a)
	_ = json.Unmarshal(theirs, &b)
	if !a.ModTime.Equal(b.ModTime) {
		return a.ModTime.After(b.ModTime)
	}
	return a.Size >= b.Size
}

// overlay is ours, keeping the fields of theirs that this build does not
// know — written by a newer one. A field this build knows and ours leaves
// out is zero (omitempty), not unknown: theirs does not get it back. Known
// struct fields are overlaid field by field; anything else is ours whole.
func overlay(theirs, ours json.RawMessage, typ reflect.Type) json.RawMessage {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return ours
	}
	var t, o map[string]json.RawMessage
	if json.Unmarshal(theirs, &t) != nil || json.Unmarshal(ours, &o) != nil || t == nil || o == nil {
		return ours
	}
	known := jsonFields(typ)
	out := map[string]json.RawMessage{}
	for k, v := range t {
		if _, ok := known[k]; !ok {
			out[k] = v // a newer build's field
		}
	}
	for k, v := range o {
		if prev, ok := t[k]; ok {
			out[k] = overlay(prev, v, known[k])
			continue
		}
		out[k] = v
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return ours
	}
	return raw
}

// jsonFields is a struct's fields by the names encoding/json gives them.
func jsonFields(typ reflect.Type) map[string]reflect.Type {
	fields := map[string]reflect.Type{}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if !f.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		switch name {
		case "-":
			continue
		case "":
			name = f.Name
		}
		fields[name] = f.Type
	}
	return fields
}
