package rt

import (
	"errors"
	"reflect"
	"runtime"
	"testing"
	"time"
)

func TestGoroutinesRunAndFinish(t *testing.T) {
	s := NewSched(1)
	var order []int
	m := NewMutex(s)
	for i := 0; i < 5; i++ {
		i := i
		s.Go(func() {
			m.Lock()
			order = append(order, i)
			m.Unlock()
		})
	}
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if len(order) != 5 {
		t.Fatalf("expected 5 entries, got %v", order)
	}
}

func traceSeed(seed int64) []int {
	s := NewSched(seed)
	var trace []int
	ch := NewChan[int](s, 0)
	done := NewWaitGroup(s)
	done.Add(3)
	for i := 0; i < 3; i++ {
		i := i
		s.Go(func() {
			defer done.Done()
			ch.Send(i)
		})
	}
	s.Go(func() {
		done.Wait()
		ch.Close()
	})
	s.Go(func() {
		for {
			v, ok := ch.RecvOK()
			if !ok {
				return
			}
			trace = append(trace, v)
		}
	})
	if err := s.Run(); err != nil {
		panic(err)
	}
	return trace
}

func TestSameSeedIsDeterministic(t *testing.T) {
	a := traceSeed(42)
	b := traceSeed(42)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("same seed produced different traces:\n%v\n%v", a, b)
	}
	if len(a) != 3 {
		t.Fatalf("expected 3 values received, got %v", a)
	}
}

func TestDifferentSeedsCanDiffer(t *testing.T) {
	a := traceSeed(1)
	b := traceSeed(2)
	if reflect.DeepEqual(a, b) {
		t.Skip("different seeds happened to produce the same trace, not a failure but worth knowing if it happens often")
	}
}

func TestNoGoroutinesFinishesImmediately(t *testing.T) {
	s := NewSched(1)
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
}

func TestPanicPropagates(t *testing.T) {
	s := NewSched(1)
	s.Go(func() {
		panic("boom")
	})
	err := s.Run()
	var perr *PanicError
	if err == nil {
		t.Fatal("expected PanicError, got nil")
	}
	perr, ok := err.(*PanicError)
	if !ok {
		t.Fatalf("expected *PanicError, got %T: %v", err, err)
	}
	if perr.Value != "boom" {
		t.Fatalf("expected panic value \"boom\", got %v", perr.Value)
	}
	if perr.Stack == "" {
		t.Fatal("expected non-empty stack trace")
	}
}

func TestRtGoOutsideSchedPanics(t *testing.T) {
	s := NewSched(1)
	m := NewMutex(s)
	m.locked = true
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic calling rt primitive outside a scheduled goroutine")
		}
	}()
	m.Lock()
}

func TestSpawnFromWithinGoroutine(t *testing.T) {
	s := NewSched(1)
	done := NewWaitGroup(s)
	done.Add(1)
	s.Go(func() {
		s.Go(func() {
			done.Done()
		})
	})
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	done.Wait()
}

func TestShutdownReleasesGoroutinesALostRunWouldLeak(t *testing.T) {
	before := runtime.NumGoroutine()
	s := NewSched(1)
	c := NewChan[int](s, 0)
	resumedAfterRelease := 0
	for i := 0; i < 25; i++ {
		s.Go(func() {
			c.Recv()
			resumedAfterRelease++
		})
	}
	err := s.Run()
	if _, ok := err.(*DeadlockError); !ok {
		t.Fatalf("expected DeadlockError, got %v", err)
	}

	s.Shutdown()
	s.Shutdown() // must be idempotent

	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before+5 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before+5 {
		t.Fatalf("Shutdown left goroutines behind: %d still live, started at %d", n, before)
	}
	if resumedAfterRelease != 0 {
		t.Fatalf("code under test resumed %d times after Shutdown, it must never run again", resumedAfterRelease)
	}
}

func TestShutdownReleasesGoroutinesOnDecisionLimitAndFreshSched(t *testing.T) {
	before := runtime.NumGoroutine()

	s := NewSched(1)
	s.SetDecisionLimit(1)
	for i := 0; i < 10; i++ {
		s.Go(func() { s.Sleep(time.Hour) })
	}
	if err := s.Run(); !errors.Is(err, ErrDecisionLimit) {
		t.Fatalf("expected ErrDecisionLimit, got %v", err)
	}
	s.Shutdown()

	NewSched(2).Shutdown() // no goroutines at all, must not panic

	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before+5 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before+5 {
		t.Fatalf("Shutdown left goroutines behind: %d still live, started at %d", n, before)
	}
}
