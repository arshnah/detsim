package workerpool

import (
	"testing"

	"github.com/arshnah/detsim/rt"
)

func TestCircularChannelWaitIsDetectedAsDeadlockNotTimeout(t *testing.T) {
	s := rt.NewSched(1)
	a := rt.NewChan[int](s, 0)
	b := rt.NewChan[int](s, 0)

	s.Go(func() {
		a.Send(1)
		b.Recv()
	})
	s.Go(func() {
		b.Send(1)
		a.Recv()
	})

	err := s.Run()
	derr, ok := err.(*rt.DeadlockError)
	if !ok {
		t.Fatalf("expected *rt.DeadlockError, got %T: %v", err, err)
	}
	if len(derr.Goroutines) != 2 {
		t.Fatalf("expected both goroutines reported blocked, got %d", len(derr.Goroutines))
	}
	for _, g := range derr.Goroutines {
		if g.Stack == "" {
			t.Fatal("expected a captured stack trace for each deadlocked goroutine")
		}
	}
}
