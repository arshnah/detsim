package rt

import "testing"

func TestSelectPicksTheReadyCase(t *testing.T) {
	s := NewSched(1)
	a := NewChan[int](s, 1)
	b := NewChan[int](s, 1)
	a.Send(5)

	var got string
	s.Go(func() {
		s.Select(
			RecvCase(a, func() { got = "a" }),
			RecvCase(b, func() { got = "b" }),
		)
	})
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if got != "a" {
		t.Fatalf("expected case a to win, got %q", got)
	}
}

func TestSelectBlocksUntilACaseIsReady(t *testing.T) {
	s := NewSched(1)
	ch := NewChan[int](s, 0)
	var got int
	s.Go(func() {
		s.Select(RecvCase(ch, func() { got = ch.Recv() + 1 }))
	})
	s.Go(func() {
		ch.Send(41)
	})
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
}

func TestSelectTakesDefaultWhenNothingReady(t *testing.T) {
	s := NewSched(1)
	ch := NewChan[int](s, 0)
	var got string
	s.Go(func() {
		s.Select(
			RecvCase(ch, func() { got = "recv" }),
			DefaultCase(func() { got = "default" }),
		)
	})
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if got != "default" {
		t.Fatalf("expected default, got %q", got)
	}
}

func TestSelectSendCaseActuallySendsOnCommit(t *testing.T) {
	s := NewSched(1)
	ch := NewChan[int](s, 1)
	committed := false
	s.Go(func() {
		s.Select(SendCase(ch, func() {
			ch.Send(9)
			committed = true
		}))
	})
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if !committed {
		t.Fatal("send case never committed")
	}
	v, ok := ch.TryRecv()
	if !ok {
		t.Fatal("expected the select's send to have actually gone through")
	}
	_ = v
}

func TestSelectAmongMultipleReadyCasesEventuallyPicksEachOne(t *testing.T) {
	seenA, seenB := false, false
	for seed := int64(1); seed <= 200 && !(seenA && seenB); seed++ {
		s := NewSched(seed)
		a := NewChan[int](s, 1)
		b := NewChan[int](s, 1)
		a.Send(1)
		b.Send(1)
		s.Go(func() {
			s.Select(
				RecvCase(a, func() { seenA = true }),
				RecvCase(b, func() { seenB = true }),
			)
		})
		if err := s.Run(); err != nil {
			t.Fatalf("Run() = %v", err)
		}
	}
	if !seenA || !seenB {
		t.Fatalf("expected both cases to win at least once across seeds, seenA=%v seenB=%v", seenA, seenB)
	}
}
