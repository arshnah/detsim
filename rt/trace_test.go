package rt

import (
	"path/filepath"
	"reflect"
	"testing"
)

func chaoticScenario(s *Sched) *[]int {
	order := new([]int)
	m := NewMutex(s)
	wg := NewWaitGroup(s)
	wg.Add(5)
	for i := 0; i < 5; i++ {
		i := i
		s.Go(func() {
			defer wg.Done()
			s.Sleep(VirtualTime(i % 3))
			m.Lock()
			*order = append(*order, i)
			m.Unlock()
		})
	}
	s.Go(func() {
		wg.Wait()
	})
	return order
}

func TestReplayFromTraceReproducesSameOutcome(t *testing.T) {
	s1 := NewSched(99)
	order1 := chaoticScenario(s1)
	if err := s1.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	trace := s1.Trace()
	if len(trace.Decisions) == 0 {
		t.Fatal("expected a non-empty decision trace")
	}

	s2 := NewSchedFromTrace(trace)
	order2 := chaoticScenario(s2)
	if err := s2.Run(); err != nil {
		t.Fatalf("replay Run() = %v", err)
	}

	if !reflect.DeepEqual(*order1, *order2) {
		t.Fatalf("replay diverged: original=%v replay=%v", *order1, *order2)
	}
}

func TestSaveAndLoadTraceRoundTrips(t *testing.T) {
	s1 := NewSched(7)
	chaoticScenario(s1)
	if err := s1.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}

	path := filepath.Join(t.TempDir(), "trace.json")
	if err := SaveTrace(path, s1.Trace()); err != nil {
		t.Fatalf("SaveTrace() = %v", err)
	}

	loaded, err := LoadTrace(path)
	if err != nil {
		t.Fatalf("LoadTrace() = %v", err)
	}
	if !reflect.DeepEqual(loaded, s1.Trace()) {
		t.Fatalf("loaded trace does not match original:\n%+v\n%+v", loaded, s1.Trace())
	}
}

func TestLenientReplayNeverErrorsOnAMangledTrace(t *testing.T) {
	s1 := NewSched(11)
	chaoticScenario(s1)
	if err := s1.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	trace := s1.Trace()

	mangled := Trace{Seed: trace.Seed, Decisions: trace.Decisions[:len(trace.Decisions)/2]}
	s2 := NewSchedFromTraceLenient(mangled)
	chaoticScenario(s2)
	if err := s2.Run(); err != nil {
		t.Fatalf("lenient replay of a truncated trace should never fail, got %v", err)
	}
}

func TestReplayExhaustedTraceIsReported(t *testing.T) {
	s := NewSchedFromTrace(Trace{Seed: 1, Decisions: nil})
	s.Go(func() {
		s.Sleep(1)
	})
	err := s.Run()
	if err != ErrTraceExhausted {
		t.Fatalf("expected ErrTraceExhausted, got %v", err)
	}
}

func TestNamedGoroutinesShowUpInTraceLabels(t *testing.T) {
	s := NewSched(3)
	s.GoNamed("producer", func() {
		s.Sleep(5)
	})
	s.GoNamed("consumer", func() {})
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}

	tr := s.Trace()
	if len(tr.Labels) != 2 || tr.Labels[0] != "producer" || tr.Labels[1] != "consumer" {
		t.Fatalf("labels not recorded as spawned: %v", tr.Labels)
	}
	if got := tr.Name(0); got != "producer" {
		t.Fatalf("Name(0) = %q, want producer", got)
	}
	if got := tr.Name(9); got != "g9" {
		t.Fatalf("Name(9) on an out-of-range id = %q, want g9", got)
	}
}

func TestStepsRecordTimeAndBlockReasons(t *testing.T) {
	s := NewSched(5)
	s.GoNamed("sleeper", func() {
		s.Sleep(100)
	})
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}

	tr := s.Trace()
	if len(tr.Steps) != len(tr.Decisions) {
		t.Fatalf("steps and decisions out of step: %d steps, %d decisions", len(tr.Steps), len(tr.Decisions))
	}
	last := tr.Steps[len(tr.Steps)-1]
	if last.After != "finished" {
		t.Fatalf("final step After = %q, want finished", last.After)
	}
	var sawSleep bool
	for _, st := range tr.Steps {
		if st.After == "sleep" {
			sawSleep = true
		}
	}
	if !sawSleep {
		t.Fatalf("expected a step blocked on sleep, got %+v", tr.Steps)
	}
}

func TestSaveAndLoadRoundTripsLabelsAndSteps(t *testing.T) {
	s := NewSched(9)
	s.GoNamed("worker", func() {
		s.Sleep(1)
	})
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}

	path := filepath.Join(t.TempDir(), "trace.json")
	if err := SaveTrace(path, s.Trace()); err != nil {
		t.Fatalf("SaveTrace() = %v", err)
	}
	loaded, err := LoadTrace(path)
	if err != nil {
		t.Fatalf("LoadTrace() = %v", err)
	}
	want := s.Trace()
	if !reflect.DeepEqual(loaded, want) {
		t.Fatalf("round trip mismatch:\nloaded: %+v\nwant:   %+v", loaded, want)
	}
	if loaded.Name(0) != "worker" {
		t.Fatalf("loaded trace lost its label: %q", loaded.Name(0))
	}
}

func TestReplayStillReproducesWithStepsRecorded(t *testing.T) {
	s1 := NewSched(99)
	order1 := chaoticScenario(s1)
	if err := s1.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}

	s2 := NewSchedFromTrace(s1.Trace())
	order2 := chaoticScenario(s2)
	if err := s2.Run(); err != nil {
		t.Fatalf("replay Run() = %v", err)
	}
	if !reflect.DeepEqual(*order1, *order2) {
		t.Fatalf("step recording broke replay: original=%v replay=%v", *order1, *order2)
	}
}
