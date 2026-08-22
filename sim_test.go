package detsim

import (
	"reflect"
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
