package rt

import (
	"errors"
	"testing"
)

func TestDecisionLimitStopsEarly(t *testing.T) {
	s := NewSched(1)
	s.SetDecisionLimit(3)
	m := NewMutex(s)
	for i := 0; i < 10; i++ {
		s.Go(func() {
			m.Lock()
			m.Unlock()
		})
	}

	err := s.Run()
	if !errors.Is(err, ErrDecisionLimit) {
		t.Fatalf("expected ErrDecisionLimit, got %v", err)
	}
	if len(s.decisions) != 3 {
		t.Fatalf("expected exactly 3 decisions recorded, got %d", len(s.decisions))
	}
	if s.allFinished() {
		t.Fatal("expected the scheduler to have stopped before all goroutines finished")
	}
}

func TestDecisionLimitZeroMeansUnlimited(t *testing.T) {
	s := NewSched(1)
	for i := 0; i < 5; i++ {
		s.Go(func() {})
	}
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
}

func TestDecisionLimitStillCatchesEarlyDeadlock(t *testing.T) {
	s := NewSched(1)
	s.SetDecisionLimit(100)
	a := NewChan[int](s, 0)
	b := NewChan[int](s, 0)
	s.Go(func() { a.Send(1); b.Recv() })
	s.Go(func() { b.Send(1); a.Recv() })

	err := s.Run()
	if _, ok := err.(*DeadlockError); !ok {
		t.Fatalf("expected a real deadlock to still be caught even with a high decision limit, got %v", err)
	}
}

func TestReplayPrefixReproducesSameDecisionsAsPeek(t *testing.T) {
	full := NewSched(9)
	chaoticScenario(full)
	if err := full.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	fullTrace := full.Trace()
	if len(fullTrace.Decisions) < 4 {
		t.Skip("scenario didn't produce enough decisions to test a meaningful prefix")
	}

	peek := NewSched(9)
	peek.SetDecisionLimit(4)
	chaoticScenario(peek)
	err := peek.Run()
	if !errors.Is(err, ErrDecisionLimit) {
		t.Fatalf("expected ErrDecisionLimit, got %v", err)
	}

	peekTrace := peek.Trace()
	if len(peekTrace.Decisions) != 4 {
		t.Fatalf("expected exactly 4 decisions from the peek run, got %d", len(peekTrace.Decisions))
	}
	for i := range peekTrace.Decisions {
		if peekTrace.Decisions[i] != fullTrace.Decisions[i] {
			t.Fatalf("peek run diverged from the full run's own prefix at decision %d: peek=%v full=%v",
				i, peekTrace.Decisions, fullTrace.Decisions[:len(peekTrace.Decisions)])
		}
	}
}
