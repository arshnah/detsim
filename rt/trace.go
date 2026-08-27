package rt

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Step describes one scheduling decision: the scheduler picked a goroutine at virtual
// time At, gave it a turn, and when the turn ended that goroutine either finished or
// parked with the reason recorded in After ("chan send", "mutex lock", "sleep", ...).
type Step struct {
	At        VirtualTime `json:"at"`
	Goroutine uint64      `json:"goroutine"`
	After     string      `json:"after,omitempty"`
}

// Trace is the exact sequence of scheduling decisions a Run made. Decisions alone are
// enough to replay a run byte-for-byte. Labels and Steps are optional enrichment for
// human readers: Labels maps goroutine id to the name it was spawned with (empty for
// unnamed goroutines, and absent entirely for traces recorded before labels existed),
// Steps records when each pick happened and what the goroutine blocked on afterward.
type Trace struct {
	Seed      int64    `json:"seed"`
	Decisions []uint64 `json:"decisions"`
	Labels    []string `json:"labels,omitempty"`
	Steps     []Step   `json:"steps,omitempty"`
}

// Name returns the recorded name for a goroutine id, or "g<N>" when the trace has no
// label for it.
func (t Trace) Name(id uint64) string {
	if int(id) < len(t.Labels) && t.Labels[id] != "" {
		return t.Labels[id]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "g%d", id)
	return b.String()
}

// SaveTrace persists a Trace as indented JSON.
func SaveTrace(path string, trace Trace) error {
	data, err := json.MarshalIndent(trace, "", "  ")
	if err != nil {
		return fmt.Errorf("detsim/rt: marshaling trace: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("detsim/rt: writing trace to %s: %w", path, err)
	}
	return nil
}

// LoadTrace reads a Trace previously written by SaveTrace.
func LoadTrace(path string) (Trace, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Trace{}, fmt.Errorf("detsim/rt: reading trace from %s: %w", path, err)
	}
	var trace Trace
	if err := json.Unmarshal(data, &trace); err != nil {
		return Trace{}, fmt.Errorf("detsim/rt: parsing trace from %s: %w", path, err)
	}
	return trace, nil
}

// Trace returns the sequence of decisions this Sched has made so far, plus the
// goroutine labels and per-pick steps recorded along the way.
func (s *Sched) Trace() Trace {
	labels := make([]string, len(s.goroutines))
	for _, g := range s.goroutines {
		labels[g.id] = g.name
	}
	return Trace{
		Seed:      s.Seed,
		Decisions: append([]uint64(nil), s.decisions...),
		Labels:    labels,
		Steps:     append([]Step(nil), s.steps...),
	}
}
