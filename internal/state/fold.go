package state

import (
	"time"

	"github.com/deephanson94/compass/internal/transcript"
)

// Fold is everything a Machine keeps: the handful of facts the state rules
// need, in a shape that survives a trip through JSON and back.
//
// It exists so a fresh process can pick up where an earlier one stopped
// reading instead of replaying a transcript from byte zero. `compass status`
// runs from a tmux status line every few seconds, and the sessions it most
// needs to report on are exactly the long, live ones that cost the most to
// replay.
//
// Every field is exported because this is a wire format. It is written by
// whoever holds the cache, and read back by a process that must treat it as
// data — a Fold that is wrong makes the state wrong, never the program unsafe.
type Fold struct {
	Pending []PendingUse `json:"pending,omitempty"`
	Seq     int          `json:"seq,omitempty"`

	LastEventAt time.Time `json:"last_event_at,omitempty"`

	SawSubstantive  bool                 `json:"saw_substantive,omitempty"`
	SubstantiveKind transcript.EventType `json:"substantive_kind,omitempty"`
	SubstantiveAt   time.Time            `json:"substantive_at,omitempty"`
	SubstantiveText string               `json:"substantive_text,omitempty"`

	LastUse    transcript.ToolUse `json:"last_use,omitzero"`
	HasLastUse bool               `json:"has_last_use,omitempty"`

	AwaitingModel bool      `json:"awaiting_model,omitempty"`
	AwaitingSince time.Time `json:"awaiting_since,omitempty"`
}

// PendingUse is one tool call the machine is still waiting on.
type PendingUse struct {
	Use transcript.ToolUse `json:"use"`
	At  time.Time          `json:"at"`
	Seq int                `json:"seq"`
}

// Fold returns the machine's whole state. The pending calls are ordered by the
// sequence they arrived in, so a restored machine breaks ties the same way the
// original would.
func (m *Machine) Fold() Fold {
	f := Fold{
		Seq:             m.seq,
		LastEventAt:     m.lastEventAt,
		SawSubstantive:  m.sawSubstantive,
		SubstantiveKind: m.substantiveKind,
		SubstantiveAt:   m.substantiveAt,
		SubstantiveText: m.substantiveText,
		LastUse:         m.lastUse,
		HasLastUse:      m.hasLastUse,
		AwaitingModel:   m.awaitingModel,
		AwaitingSince:   m.awaitingSince,
	}
	for id, p := range m.pending {
		use := p.use
		use.ID = id
		f.Pending = append(f.Pending, PendingUse{Use: use, At: p.at, Seq: p.seq})
	}
	sortPending(f.Pending)
	return f
}

// Restore rebuilds a machine from a Fold. Feeding it further events afterwards
// must reach the same verdict as replaying the whole transcript would.
func RestoreMachine(f Fold) *Machine {
	m := NewMachine()
	m.seq = f.Seq
	m.lastEventAt = f.LastEventAt
	m.sawSubstantive = f.SawSubstantive
	m.substantiveKind = f.SubstantiveKind
	m.substantiveAt = f.SubstantiveAt
	m.substantiveText = f.SubstantiveText
	m.lastUse = f.LastUse
	m.hasLastUse = f.HasLastUse
	m.awaitingModel = f.AwaitingModel
	m.awaitingSince = f.AwaitingSince
	for _, p := range f.Pending {
		m.pending[p.Use.ID] = pendingUse{use: p.Use, at: p.At, seq: p.Seq}
	}
	return m
}

// sortPending orders by arrival, so the round trip is byte-stable and a cache
// file does not churn just because Go walked a map differently.
func sortPending(p []PendingUse) {
	for i := 1; i < len(p); i++ {
		for j := i; j > 0 && p[j].Seq < p[j-1].Seq; j-- {
			p[j], p[j-1] = p[j-1], p[j]
		}
	}
}
