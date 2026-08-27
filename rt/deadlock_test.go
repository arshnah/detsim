package rt

import (
	"strings"
	"testing"
)

func TestDeadlockDetected(t *testing.T) {
	s := NewSched(1)
	a := NewChan[int](s, 0)
	b := NewChan[int](s, 0)

	s.Go(func() {
		a.Send(1)
	})
	s.Go(func() {
		b.Send(1)
	})

	err := s.Run()
	if err == nil {
		t.Fatal("expected DeadlockError, got nil")
	}
	derr, ok := err.(*DeadlockError)
	if !ok {
		t.Fatalf("expected *DeadlockError, got %T: %v", err, err)
	}
	if len(derr.Goroutines) != 2 {
		t.Fatalf("expected 2 blocked goroutines, got %d", len(derr.Goroutines))
	}
	for _, g := range derr.Goroutines {
		if g.Reason != "chan send" {
			t.Fatalf("expected reason %q, got %q", "chan send", g.Reason)
		}
		if !strings.Contains(g.Stack, "chan.go") {
			t.Fatalf("expected captured stack to mention chan.go, got:\n%s", g.Stack)
		}
	}
}

func TestDeadlockNotFalsePositive(t *testing.T) {
	s := NewSched(1)
	ch := NewChan[int](s, 0)
	s.Go(func() {
		ch.Send(1)
	})
	s.Go(func() {
		ch.Recv()
	})
	if err := s.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
