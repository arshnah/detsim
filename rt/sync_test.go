package rt

import "testing"

func TestMutexExcludesConcurrentAccess(t *testing.T) {
	s := NewSched(1)
	m := NewMutex(s)
	counter := 0
	for i := 0; i < 20; i++ {
		s.Go(func() {
			m.Lock()
			cur := counter
			counter = cur + 1
			m.Unlock()
		})
	}
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if counter != 20 {
		t.Fatalf("expected 20, got %d (lost update means mutual exclusion broke)", counter)
	}
}

func TestUnlockUnlockedMutexPanics(t *testing.T) {
	m := NewMutex(NewSched(1))
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	m.Unlock()
}

func TestRWMutexAllowsConcurrentReadersButExcludesWriter(t *testing.T) {
	s := NewSched(1)
	m := NewRWMutex(s)
	var readersActive int
	var maxReadersActive int
	wg := NewWaitGroup(s)
	wg.Add(2)
	for i := 0; i < 2; i++ {
		s.Go(func() {
			m.RLock()
			readersActive++
			if readersActive > maxReadersActive {
				maxReadersActive = readersActive
			}
			readersActive--
			m.RUnlock()
			wg.Done()
		})
	}
	s.Go(func() {
		m.Lock()
		if m.readers != 0 {
			t.Errorf("writer holds lock with %d readers still counted", m.readers)
		}
		m.Unlock()
	})
	s.Go(func() {
		wg.Wait()
	})
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
}

func TestRUnlockUnlockedRWMutexPanics(t *testing.T) {
	m := NewRWMutex(NewSched(1))
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	m.RUnlock()
}

func TestWaitGroupBlocksUntilZero(t *testing.T) {
	s := NewSched(1)
	wg := NewWaitGroup(s)
	wg.Add(3)
	doneOrder := 0
	s.Go(func() {
		wg.Wait()
		doneOrder = 1
	})
	for i := 0; i < 3; i++ {
		s.Go(func() {
			wg.Done()
		})
	}
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if doneOrder != 1 {
		t.Fatal("waiter never woke")
	}
}

func TestWaitGroupNegativePanics(t *testing.T) {
	wg := NewWaitGroup(NewSched(1))
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	wg.Add(-1)
}

func TestOnceRunsExactlyOnce(t *testing.T) {
	s := NewSched(1)
	o := NewOnce(s)
	count := 0
	for i := 0; i < 10; i++ {
		s.Go(func() {
			o.Do(func() { count++ })
		})
	}
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1, got %d", count)
	}
}

func TestCondSignalWakesExactlyOneNotAll(t *testing.T) {
	s := NewSched(1)
	m := NewMutex(s)
	c := NewCond(s, m)
	woken := 0
	for i := 0; i < 3; i++ {
		s.Go(func() {
			m.Lock()
			c.Wait()
			woken++
			m.Unlock()
		})
	}
	s.Go(func() {
		m.Lock()
		c.Signal()
		m.Unlock()
	})
	err := s.Run()
	derr, ok := err.(*DeadlockError)
	if !ok {
		t.Fatalf("expected DeadlockError (2 waiters should stay parked forever after a single Signal woke only 1 of 3), got err=%v woken=%d", err, woken)
	}
	if woken != 1 {
		t.Fatalf("expected exactly 1 waiter woken before deadlock, got %d", woken)
	}
	if len(derr.Goroutines) != 2 {
		t.Fatalf("expected 2 still-blocked goroutines, got %d", len(derr.Goroutines))
	}
}

func TestCondBroadcastWakesAll(t *testing.T) {
	s := NewSched(1)
	m := NewMutex(s)
	c := NewCond(s, m)
	arrived := NewWaitGroup(s)
	arrived.Add(3)
	done := NewWaitGroup(s)
	done.Add(3)
	for i := 0; i < 3; i++ {
		s.Go(func() {
			m.Lock()
			arrived.Done()
			c.Wait()
			m.Unlock()
			done.Done()
		})
	}
	s.Go(func() {
		arrived.Wait()
		m.Lock()
		c.Broadcast()
		m.Unlock()
	})
	s.Go(func() {
		done.Wait()
	})
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
}

func TestSleepAdvancesVirtualTimeInstantly(t *testing.T) {
	s := NewSched(1)
	var woke VirtualTime
	s.Go(func() {
		s.Sleep(1000)
		woke = s.Now()
	})
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if woke != 1000 {
		t.Fatalf("expected 1000, got %v", woke)
	}
}

func TestAfterFiresAtRightTime(t *testing.T) {
	s := NewSched(1)
	var got VirtualTime
	s.Go(func() {
		got = s.After(500).Recv()
	})
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if got != 500 {
		t.Fatalf("expected 500, got %v", got)
	}
}

func TestOnceMarksDoneEvenWhenFnPanics(t *testing.T) {
	s := NewSched(1)
	defer s.Shutdown()
	o := NewOnce(s)
	ran := 0
	ranAgain := 0
	handoff := NewChan[struct{}](s, 0)

	s.Go(func() {
		func() {
			defer func() { _ = recover() }()
			o.Do(func() {
				ran++
				panic("boom")
			})
		}()
		handoff.Send(struct{}{})
	})
	s.Go(func() {
		handoff.Recv()
		o.Do(func() { ranAgain++ })
	})

	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if ran != 1 {
		t.Fatalf("expected the first Do to run exactly once, got %d", ran)
	}
	if ranAgain != 0 {
		t.Fatal("expected a Once that panicked to still count as done, but the second Do ran its function")
	}
}
