package detsim

import (
	"container/heap"
	"math/rand"
	"time"
)

type VirtualTime = time.Duration

type Sim struct {
	Seed int64
	Rand *rand.Rand

	now      VirtualTime
	queue    eventQueue
	nextSeq  uint64
	maxSteps int
	steps    int

	horizonSet bool
	horizon    VirtualTime
}

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

func (s *Sim) Now() VirtualTime { return s.now }

func (s *Sim) After(d VirtualTime, fn func(*Sim)) {
	if d < 0 {
		d = 0
	}
	s.nextSeq++
	heap.Push(&s.queue, &event{at: s.now + d, seq: s.nextSeq, fn: fn})
}

func (s *Sim) At(t VirtualTime, fn func(*Sim)) {
	s.nextSeq++
	heap.Push(&s.queue, &event{at: t, seq: s.nextSeq, fn: fn})
}

func (s *Sim) Immediately(fn func(*Sim)) {
	s.After(0, fn)
}

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

func (s *Sim) SetMaxSteps(n int) { s.maxSteps = n }

func (s *Sim) RunUntil(t VirtualTime) (steps int) {
	s.horizonSet = true
	s.horizon = t
	steps, _ = s.Run()
	s.horizonSet = false
	return steps
}
