package detsim

import (
	"container/heap"
	"math/rand"
	"time"
)

// VirtualTime is simulated time, an alias for time.Duration.
type VirtualTime = time.Duration

// Sim is the deterministic event kernel.
type Sim struct {
	Seed int64
	Rand *rand.Rand

	now      VirtualTime
	queue    eventQueue
	nextSeq  uint64
	maxSteps int
	steps    int

	horizonSet   bool
	horizon      VirtualTime
	maxRequested VirtualTime
}

// New builds a Sim seeded for reproducible event ordering.
func New(seed int64) *Sim {
	s := &Sim{
		Seed:     seed,
		Rand:     rand.New(rand.NewSource(seed)),
		queue:    eventQueue{},
		maxSteps: 5_000_000,
	}
	heap.Init(&s.queue)
	return s
}

// Now returns the current virtual time.
func (s *Sim) Now() VirtualTime { return s.now }

// After schedules fn to run d after the current virtual time. A negative d is clamped
// to zero. fn must not be nil; scheduling one panics immediately instead of mid-Run.
func (s *Sim) After(d VirtualTime, fn func(*Sim)) {
	if fn == nil {
		panic("detsim: After called with a nil event function")
	}
	if d < 0 {
		d = 0
	}
	s.nextSeq++
	heap.Push(&s.queue, &event{at: s.now + d, seq: s.nextSeq, fn: fn})
}

// At schedules fn to run at absolute virtual time t. A t earlier than the current time
// is clamped to Now so virtual time never moves backwards.
func (s *Sim) At(t VirtualTime, fn func(*Sim)) {
	if fn == nil {
		panic("detsim: At called with a nil event function")
	}
	if t < s.now {
		t = s.now
	}
	s.nextSeq++
	heap.Push(&s.queue, &event{at: t, seq: s.nextSeq, fn: fn})
}

// Immediately schedules fn as the next event.
func (s *Sim) Immediately(fn func(*Sim)) {
	s.After(0, fn)
}

// Run drains the event queue until it's empty or the step budget runs out.
func (s *Sim) Run() (steps int, ranToCompletion bool) {
	for s.queue.Len() > 0 {
		if s.steps >= s.maxSteps {
			return s.steps, false
		}
		if s.horizonSet && s.queue[0].at > s.horizon {
			return s.steps, true
		}
		e := heap.Pop(&s.queue).(*event)
		s.now = e.at
		s.steps++
		e.fn(s)
	}
	return s.steps, true
}

// SetMaxSteps overrides the step budget Run enforces.
func (s *Sim) SetMaxSteps(n int) { s.maxSteps = n }

// RunUntil runs events up to and including virtual time t.
func (s *Sim) RunUntil(t VirtualTime) (steps int) {
	if t > s.maxRequested {
		s.maxRequested = t
	}
	s.horizonSet = true
	s.horizon = t
	steps, _ = s.Run()
	s.horizonSet = false
	return steps
}

// RunFor runs d more virtual time from the highest horizon requested so far, not from Now.
func (s *Sim) RunFor(d VirtualTime) (steps int) {
	return s.RunUntil(s.maxRequested + d)
}
