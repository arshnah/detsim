// Package rt is a cooperative, single-stepping scheduler for real Go goroutines. Every
// spawned goroutine is a genuine go statement, but a turnstile only ever lets one run at
// a time, so an entire program's interleaving becomes seeded and replayable.
package rt

import (
	"errors"
	"fmt"
	"math/rand"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

// ErrDecisionLimit is returned by Run when SetDecisionLimit cuts a run short.
var ErrDecisionLimit = errors.New("detsim/rt: scheduler stopped after reaching its decision limit (peek mode)")

// VirtualTime is simulated time, an alias for time.Duration.
type VirtualTime = time.Duration

type status int

const (
	statusNew status = iota
	statusRunnable
	statusBlocked
	statusFinished
)

type gState struct {
	id      uint64
	name    string
	resume  chan struct{}
	parked  chan struct{}
	status  status
	stopped bool
	pred    func() bool
	isSleep bool
	wakeAt  VirtualTime
	reason  string
	stack   []byte
	panic   any
}

// Sched is the deterministic goroutine scheduler.
type Sched struct {
	Seed int64
	Rand *rand.Rand

	now        VirtualTime
	goroutines []*gState
	nextID     uint64
	currentG   *gState

	decisions     []uint64
	steps         []Step
	replaying     bool
	lenient       bool
	replay        []uint64
	replayPos     int
	decisionLimit int
}

// SetDecisionLimit caps Run to n scheduling decisions. n <= 0 means unlimited.
func (s *Sched) SetDecisionLimit(n int) { s.decisionLimit = n }

// NewSched builds a Sched that picks among ready goroutines uniformly at random, seeded.
func NewSched(seed int64) *Sched {
	return &Sched{
		Seed: seed,
		Rand: rand.New(rand.NewSource(seed)),
	}
}

// Now returns the scheduler's current virtual time.
func (s *Sched) Now() VirtualTime { return s.now }

func (s *Sched) self() *gState {
	if s.currentG == nil {
		panic("detsim/rt: called from outside a scheduled goroutine (forgot Sched.Go?)")
	}
	return s.currentG
}

// Go spawns fn as a new scheduled goroutine, eligible to run once Run picks it.
func (s *Sched) Go(fn func()) {
	s.GoNamed("", fn)
}

// GoNamed is Go with a human-readable name for the goroutine. Names show up in traces
// (and therefore in detsim-trace output), which is the difference between reading
// "g7 ran" and "g7 sender ran" when staring at a minimized failure.
func (s *Sched) GoNamed(name string, fn func()) {
	g := &gState{
		id:     s.nextID,
		name:   name,
		resume: make(chan struct{}),
		parked: make(chan struct{}, 1),
		status: statusNew,
	}
	s.nextID++
	s.goroutines = append(s.goroutines, g)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				g.panic = r
				g.stack = debug.Stack()
			}
			g.status = statusFinished
			select {
			case g.parked <- struct{}{}:
			default:
			}
		}()
		g.waitTurn()
		fn()
	}()
}

// waitTurn blocks until the scheduler hands this goroutine a turn. If the scheduler was
// shut down while the goroutine was parked, the goroutine unwinds via runtime.Goexit so
// none of the code under test runs after Shutdown.
func (g *gState) waitTurn() {
	<-g.resume
	if g.stopped {
		runtime.Goexit()
	}
}

func (s *Sched) parkCurrent(pred func() bool, reason string) {
	g := s.self()
	g.status = statusBlocked
	g.pred = pred
	g.isSleep = false
	g.reason = reason
	g.stack = debug.Stack()
	g.parked <- struct{}{}
	g.waitTurn()
}

func (s *Sched) sleepUntil(t VirtualTime) {
	g := s.self()
	g.status = statusBlocked
	g.isSleep = true
	g.wakeAt = t
	g.pred = func() bool { return s.now >= t }
	g.reason = "sleep"
	g.stack = debug.Stack()
	g.parked <- struct{}{}
	g.waitTurn()
}

func (s *Sched) readyGoroutines() []*gState {
	var ready []*gState
	for _, g := range s.goroutines {
		switch g.status {
		case statusNew:
			ready = append(ready, g)
		case statusBlocked:
			if g.pred() {
				ready = append(ready, g)
			}
		}
	}
	return ready
}

func (s *Sched) advanceClock() bool {
	found := false
	var min VirtualTime
	for _, g := range s.goroutines {
		if g.status == statusBlocked && g.isSleep {
			if !found || g.wakeAt < min {
				min = g.wakeAt
				found = true
			}
		}
	}
	if !found {
		return false
	}
	s.now = min
	return true
}

func (s *Sched) allFinished() bool {
	for _, g := range s.goroutines {
		if g.status != statusFinished {
			return false
		}
	}
	return true
}

// Run drives the scheduler until every goroutine finishes, a deadlock is detected, a
// goroutine panics, or the decision limit is hit.
func (s *Sched) Run() error {
	for {
		if s.decisionLimit > 0 && len(s.decisions) >= s.decisionLimit {
			return ErrDecisionLimit
		}

		ready := s.readyGoroutines()
		if len(ready) == 0 {
			if s.advanceClock() {
				continue
			}
			if s.allFinished() {
				return nil
			}
			return s.deadlockError()
		}

		pick, err := s.pickNext(ready)
		if err != nil {
			return err
		}
		s.currentG = pick
		pick.status = statusRunnable
		pick.resume <- struct{}{}
		<-pick.parked
		s.currentG = nil

		// Once parked has been received, the goroutine has either finished or set its
		// park reason (parkCurrent sets reason before sending), so both reads below are
		// ordered by the channel handshake.
		after := pick.reason
		if pick.status == statusFinished {
			after = "finished"
		}
		s.steps = append(s.steps, Step{At: s.now, Goroutine: pick.id, After: after})

		if pick.status == statusFinished && pick.panic != nil {
			return &PanicError{Value: pick.panic, Stack: string(pick.stack)}
		}
	}
}

// Shutdown releases every goroutine the scheduler spawned that hasn't finished, so a
// run that ended in deadlock, a panic, or a decision limit doesn't leak them for the
// life of the process. Call it after Run returns, never while it's running; it's safe
// to call repeatedly and on an already-quiesced scheduler. Shutdown is terminal: none
// of the released goroutines resume their code under test.
func (s *Sched) Shutdown() {
	for _, g := range s.goroutines {
		// Check stopped first: a goroutine released by an earlier Shutdown may still be
		// unwinding and writing its final status, so reading status here would race.
		if g.stopped || g.status == statusFinished {
			continue
		}
		g.stopped = true
		g.resume <- struct{}{}
	}
}

// DeadlockGoroutine describes one goroutine stuck when a DeadlockError was returned.
type DeadlockGoroutine struct {
	ID     uint64
	Reason string
	Stack  string
}

// DeadlockError is returned by Run when nothing is runnable and something is unfinished.
type DeadlockError struct {
	Goroutines []DeadlockGoroutine
}

// Error renders one block per stuck goroutine, its ID, blocking reason, and stack trace.
func (e *DeadlockError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "detsim/rt: deadlock, %d goroutine(s) never became runnable:\n", len(e.Goroutines))
	for _, g := range e.Goroutines {
		reason := g.Reason
		if reason == "" {
			reason = "unknown"
		}
		fmt.Fprintf(&b, "--- goroutine %d blocked on %s ---\n%s\n", g.ID, reason, g.Stack)
	}
	return b.String()
}

func (s *Sched) deadlockError() error {
	var dg []DeadlockGoroutine
	for _, g := range s.goroutines {
		if g.status != statusFinished {
			dg = append(dg, DeadlockGoroutine{ID: g.id, Reason: g.reason, Stack: string(g.stack)})
		}
	}
	return &DeadlockError{Goroutines: dg}
}

// PanicError is returned by Run when a scheduled goroutine panics.
type PanicError struct {
	Value any
	Stack string
}

// Error reports the recovered panic value and its stack trace.
func (e *PanicError) Error() string {
	return fmt.Sprintf("detsim/rt: goroutine panicked: %v\n%s", e.Value, e.Stack)
}
