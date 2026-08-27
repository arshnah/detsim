package rt

import "testing"

func TestBufferedChan(t *testing.T) {
	s := NewSched(1)
	ch := NewChan[int](s, 2)
	var got []int
	s.Go(func() {
		ch.Send(1)
		ch.Send(2)
		ch.Close()
	})
	s.Go(func() {
		for {
			v, ok := ch.RecvOK()
			if !ok {
				return
			}
			got = append(got, v)
		}
	})
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("got %v", got)
	}
}

func TestUnbufferedChanRendezvous(t *testing.T) {
	s := NewSched(1)
	ch := NewChan[int](s, 0)
	sent := false
	received := -1
	s.Go(func() {
		ch.Send(7)
		sent = true
	})
	s.Go(func() {
		received = ch.Recv()
	})
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if !sent || received != 7 {
		t.Fatalf("sent=%v received=%v", sent, received)
	}
}

func TestSendOnClosedChanPanics(t *testing.T) {
	s := NewSched(1)
	ch := NewChan[int](s, 1)
	ch.Close()
	s.Go(func() {
		defer func() {
			if recover() == nil {
				t.Error("expected panic sending on closed channel")
			}
		}()
		ch.Send(1)
	})
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
}

func TestDoubleCloseChanPanics(t *testing.T) {
	s := NewSched(1)
	ch := NewChan[int](s, 1)
	ch.Close()
	defer func() {
		if recover() == nil {
			t.Error("expected panic on double close")
		}
	}()
	ch.Close()
}

func TestRecvOnClosedEmptyChanReturnsZeroFalse(t *testing.T) {
	s := NewSched(1)
	ch := NewChan[int](s, 1)
	ch.Close()
	s.Go(func() {
		v, ok := ch.RecvOK()
		if ok {
			t.Errorf("expected ok=false, got v=%d ok=%v", v, ok)
		}
		if v != 0 {
			t.Errorf("expected zero value, got %d", v)
		}
	})
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
}

func TestTryRecv(t *testing.T) {
	s := NewSched(1)
	ch := NewChan[int](s, 1)
	if _, ok := ch.TryRecv(); ok {
		t.Fatal("expected TryRecv to fail on empty channel")
	}
	s.Go(func() {
		ch.Send(9)
	})
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	v, ok := ch.TryRecv()
	if !ok || v != 9 {
		t.Fatalf("v=%d ok=%v", v, ok)
	}
}
