package journey

import "time"

// Fold is everything a Segmenter keeps, in a shape that survives a trip
// through JSON and back: the legs as they are being built, the pressure
// gauge, the runner memory, the lanes and the plan. It exists so a launch can
// pick every board column up where the last process stopped reading instead
// of replaying its transcript from byte zero — the state machine has had the
// same (state.Fold) since round 61; the board's trails were the one thing a
// warm launch still paid for in full (round 71).
//
// Every field is exported because this is a wire format. A Fold that is
// wrong makes a trail wrong, never the program unsafe.
type Fold struct {
	Prompts     []Prompt     `json:"prompts,omitempty"`
	Compactions []time.Time  `json:"compactions,omitempty"`
	Legs        []LegFold    `json:"legs,omitempty"`
	Branches    []Branch     `json:"branches,omitempty"`
	Open        bool         `json:"open,omitempty"`
	Runners     []RunnerFold `json:"runners,omitempty"`
	Tasks       []Task       `json:"tasks,omitempty"`
	// ByCreate is the TaskCreate calls whose result — and so whose id — has
	// not landed yet: call id → index into Tasks.
	ByCreate map[string]int `json:"by_create,omitempty"`
	Press    []PressFold    `json:"press,omitempty"`
}

// LegFold is a leg while it is still being built (legState, exported).
type LegFold struct {
	Class     Class      `json:"class"`
	Voted     Class      `json:"voted"`
	Start     time.Time  `json:"start,omitempty"`
	End       time.Time  `json:"end,omitempty"`
	Votes     int        `json:"votes,omitempty"`
	Files     []FileFold `json:"files,omitempty"`
	Keyword   string     `json:"keyword,omitempty"`
	HadError  bool       `json:"had_error,omitempty"`
	Waypoints []Waypoint `json:"waypoints,omitempty"`
}

// FileFold is one basename a leg touched and how often.
type FileFold struct {
	Name string `json:"name"`
	N    int    `json:"n"`
}

// RunnerFold is one Test or Ship call still waiting for its result, in the
// order it was remembered.
type RunnerFold struct {
	ID      string `json:"id"`
	Kind    Class  `json:"kind"`
	Keyword string `json:"keyword,omitempty"`
}

// PressFold is one buffered pressure vote.
type PressFold struct {
	Class   Class     `json:"class"`
	File    string    `json:"file,omitempty"`
	Keyword string    `json:"keyword,omitempty"`
	ID      string    `json:"id,omitempty"`
	At      time.Time `json:"at,omitempty"`
}

// Fold returns the segmenter's whole state. Maps are written in insertion
// order where one exists, so the round trip is byte-stable.
func (s *Segmenter) Fold() Fold {
	f := Fold{
		Prompts:     append([]Prompt(nil), s.prompts...),
		Compactions: append([]time.Time(nil), s.compactions...),
		Branches:    append([]Branch(nil), s.branches...),
		Open:        s.open,
		Tasks:       append([]Task(nil), s.tasks...),
	}
	for i := range s.legs {
		l := &s.legs[i]
		lf := LegFold{
			Class: l.class, Voted: l.voted, Start: l.start, End: l.end, Votes: l.votes,
			Keyword: l.keyword, HadError: l.hadError,
			Waypoints: append([]Waypoint(nil), l.waypoints...),
		}
		for _, fc := range l.files {
			lf.Files = append(lf.Files, FileFold{Name: fc.name, N: fc.n})
		}
		f.Legs = append(f.Legs, lf)
	}
	for _, id := range s.runnerOrder {
		if r, ok := s.runners[id]; ok {
			f.Runners = append(f.Runners, RunnerFold{ID: id, Kind: r.kind, Keyword: r.keyword})
		}
	}
	if len(s.byCreate) > 0 {
		f.ByCreate = make(map[string]int, len(s.byCreate))
		for id, i := range s.byCreate {
			f.ByCreate[id] = i
		}
	}
	for _, p := range s.press {
		f.Press = append(f.Press, PressFold{Class: p.v.class, File: p.v.file, Keyword: p.v.keyword, ID: p.v.id, At: p.at})
	}
	return f
}

// RestoreSegmenter rebuilds a segmenter from a Fold. Feeding it the events
// after the fold's mark must produce the same Trail as replaying the whole
// transcript would. byBranch and byTask are indexes over what the fold
// already carries, so they are rebuilt rather than written.
func RestoreSegmenter(f Fold) *Segmenter {
	s := &Segmenter{
		prompts:     append([]Prompt(nil), f.Prompts...),
		compactions: append([]time.Time(nil), f.Compactions...),
		branches:    append([]Branch(nil), f.Branches...),
		open:        f.Open,
		tasks:       append([]Task(nil), f.Tasks...),
	}
	for _, lf := range f.Legs {
		l := legState{
			class: lf.Class, voted: lf.Voted, start: lf.Start, end: lf.End, votes: lf.Votes,
			keyword: lf.Keyword, hadError: lf.HadError,
			waypoints: append([]Waypoint(nil), lf.Waypoints...),
		}
		for _, ff := range lf.Files {
			l.files = append(l.files, fileCount{name: ff.Name, n: ff.N})
		}
		s.legs = append(s.legs, l)
	}
	if len(f.Runners) > 0 {
		s.runners = make(map[string]runner, len(f.Runners))
		for _, r := range f.Runners {
			if _, dup := s.runners[r.ID]; dup {
				continue
			}
			s.runners[r.ID] = runner{kind: r.Kind, keyword: r.Keyword}
			s.runnerOrder = append(s.runnerOrder, r.ID)
		}
	}
	for i, b := range s.branches {
		if b.ToolUseID != "" {
			if s.byBranch == nil {
				s.byBranch = make(map[string]int)
			}
			s.byBranch[b.ToolUseID] = i
		}
	}
	if len(s.tasks) > 0 || len(f.ByCreate) > 0 {
		s.byTask = make(map[string]int, len(s.tasks))
		s.byCreate = make(map[string]int, len(f.ByCreate))
		for i, t := range s.tasks {
			if t.ID != "" {
				s.byTask[t.ID] = i
			}
		}
		for id, i := range f.ByCreate {
			if i >= 0 && i < len(s.tasks) {
				s.byCreate[id] = i
			}
		}
	}
	for _, p := range f.Press {
		s.press = append(s.press, pended{v: vote{class: p.Class, file: p.File, keyword: p.Keyword, id: p.ID}, at: p.At})
	}
	return s
}

// OutcomesFold is everything an Outcomes keeps: the calls whose results are
// still to come, and the last result. A fleet row restored from the resume
// cache says "18✓ 2✗" again without replaying the run that produced it.
type OutcomesFold struct {
	Runners []RunnerFold `json:"runners,omitempty"`
	Latest  Outcome      `json:"latest,omitzero"`
	Has     bool         `json:"has,omitempty"`
}

// Fold returns the outcomes' whole state.
func (o *Outcomes) Fold() OutcomesFold {
	f := OutcomesFold{Latest: o.latest, Has: o.has}
	for _, id := range o.runnerOrder {
		if r, ok := o.runners[id]; ok {
			f.Runners = append(f.Runners, RunnerFold{ID: id, Kind: r.kind, Keyword: r.keyword})
		}
	}
	return f
}

// RestoreOutcomes rebuilds an Outcomes from its fold.
func RestoreOutcomes(f OutcomesFold) *Outcomes {
	o := NewOutcomes()
	o.latest, o.has = f.Latest, f.Has
	for _, r := range f.Runners {
		if _, dup := o.runners[r.ID]; dup {
			continue
		}
		o.runners[r.ID] = runner{kind: r.Kind, keyword: r.Keyword}
		o.runnerOrder = append(o.runnerOrder, r.ID)
	}
	return o
}
