package rt

import (
	"errors"
	"math/rand"
)

// ErrTraceExhausted and ErrTraceMismatch mean the code under test has changed since the
// trace was recorded.
var (
	ErrTraceExhausted = errors.New("detsim/rt: replay trace ended before the scheduler quiesced, the code under test has changed")
	ErrTraceMismatch  = errors.New("detsim/rt: replay trace expected a goroutine that was never ready, the code under test has changed")
)

// NewSchedFromTrace builds a Sched that replays trace exactly, erroring on any mismatch.
func NewSchedFromTrace(trace Trace) *Sched {
	return newTraceSched(trace, false)
}

// NewSchedFromTraceLenient replays trace but falls back to a random pick on mismatch
// instead of erroring.
func NewSchedFromTraceLenient(trace Trace) *Sched {
	return newTraceSched(trace, true)
}

func newTraceSched(trace Trace, lenient bool) *Sched {
	return &Sched{
		Seed:      trace.Seed,
		Rand:      rand.New(rand.NewSource(trace.Seed)),
		replaying: true,
		lenient:   lenient,
		replay:    append([]uint64(nil), trace.Decisions...),
	}
}

func (s *Sched) pickNext(ready []*gState) (*gState, error) {
	if !s.replaying {
		return s.pickRandom(ready), nil
	}

	if s.replayPos < len(s.replay) {
		want := s.replay[s.replayPos]
		s.replayPos++
		for _, g := range ready {
			if g.id == want {
				s.decisions = append(s.decisions, g.id)
				return g, nil
			}
		}
		if !s.lenient {
			return nil, ErrTraceMismatch
		}
		return s.pickRandom(ready), nil
	}

	if !s.lenient {
		return nil, ErrTraceExhausted
	}
	return s.pickRandom(ready), nil
}

func (s *Sched) pickRandom(ready []*gState) *gState {
	idx := s.Rand.Intn(len(ready))
	pick := ready[idx]
	s.decisions = append(s.decisions, pick.id)
	return pick
}
