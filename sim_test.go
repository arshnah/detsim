package detsim

import (
	"reflect"
	"sort"
	"testing"
)

func runTrace(seed int64) []string {
	var trace []string
	s := New(seed)
	net := NewNetwork(s)
	net.SetDropRate(0.2)
	net.SetDelayRange(1, 10)

	net.Register("A", func(from NodeID, msg any) {
		trace = append(trace, "A got "+msg.(string)+" from "+string(from)+" at t="+s.Now().String())
	})
	net.Register("B", func(from NodeID, msg any) {
		trace = append(trace, "B got "+msg.(string)+" from "+string(from)+" at t="+s.Now().String())
	})

	for i := 0; i < 20; i++ {
		net.Send("A", "B", "ping")
		net.Send("B", "A", "pong")
	}
	s.Run()
	return trace
}

func TestDeterministicReplay(t *testing.T) {
	traceA := runTrace(42)
	traceB := runTrace(42)
	if !reflect.DeepEqual(traceA, traceB) {
		t.Fatalf("same seed produced different traces:\nrun1: %v\nrun2: %v", traceA, traceB)
	}
	if len(traceA) == 0 {
		t.Fatal("trace was empty, test isn't exercising anything")
	}
}

func TestDifferentSeedsCanDiffer(t *testing.T) {
	traceA := runTrace(1)
	traceB := runTrace(2)
	if reflect.DeepEqual(traceA, traceB) {
		t.Skip("different seeds happened to produce the same trace, not a failure but worth knowing if it happens often")
	}
}

func TestVirtualTimeIsInstant(t *testing.T) {
	s := New(7)
	s.After(1000, func(s *Sim) {})
	steps, done := s.Run()
	if !done {
		t.Fatal("did not run to completion")
	}
	if steps != 1 {
		t.Fatalf("expected 1 step, got %d", steps)
	}
}

func TestRunUntilRelativeToStalledNowCanStall(t *testing.T) {
	s := New(1)
	fired := 0
	var reschedule func(*Sim)
	reschedule = func(s *Sim) {
		fired++
		s.After(50, reschedule)
	}
	s.After(50, reschedule)

	for i := 0; i < 10; i++ {
		s.RunUntil(s.Now() + 5)
	}
	if fired >= 10 {
		t.Fatalf("expected this stale pattern to under-fire due to Now() stalling, but it fired %d times, the footgun this test documents may no longer exist", fired)
	}
}

func TestRunForNeverStalls(t *testing.T) {
	s := New(1)
	fired := 0
	var reschedule func(*Sim)
	reschedule = func(s *Sim) {
		fired++
		s.After(50, reschedule)
	}
	s.After(50, reschedule)

	for i := 0; i < 10; i++ {
		s.RunFor(5)
	}
	if fired == 0 {
		t.Fatal("expected RunFor to make monotonic progress across repeated small steps, but nothing fired")
	}
}

func TestAtClampsPastTimesSoTimeNeverMovesBackwards(t *testing.T) {
	s := New(1)
	var seen []VirtualTime
	s.At(100, func(s *Sim) { seen = append(seen, s.Now()) })
	s.RunUntil(50) // now=50, nothing fires yet

	// Scheduling at t=10 from now=50 must not rewind the clock when it fires.
	s.At(10, func(s *Sim) { seen = append(seen, s.Now()) })
	s.At(60, func(s *Sim) { seen = append(seen, s.Now()) })
	s.Run()

	if len(seen) != 3 {
		t.Fatalf("expected 3 events to fire, got %d", len(seen))
	}
	if !sort.SliceIsSorted(seen, func(i, j int) bool { return seen[i] < seen[j] }) {
		t.Fatalf("virtual time moved backwards across events: %v", seen)
	}
}

func TestSchedulingNilFnPanicsImmediately(t *testing.T) {
	s := New(1)
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected After(nil) to panic at scheduling time, not mid-Run")
			}
		}()
		s.After(1, nil)
	}()
}
