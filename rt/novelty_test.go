package rt

import (
	"errors"
	"testing"
)

var errRunFailure = errors.New("forced failure")

func smallScenarioTrial(seed int64) (Trace, error) {
	s := NewSched(seed)
	defer s.Shutdown()
	m := NewMutex(s)
	for i := 0; i < 3; i++ {
		s.Go(func() {
			m.Lock()
			m.Unlock()
		})
	}
	if err := s.Run(); err != nil {
		return Trace{}, err
	}
	return s.Trace(), nil
}

func TestNoveltySearchStopsOnceSchedulesStopChanging(t *testing.T) {
	result := NoveltySearch(NoveltySearchConfig{
		StartSeed: 1,
		MaxTrials: 5000,
		DryLimit:  30,
		PrefixLen: 6,
	}, smallScenarioTrial)

	if !result.StoppedDry {
		t.Fatalf("expected the search to stop once dry, ran all %d trials instead", result.TrialsRun)
	}
	if result.TrialsRun >= 5000 {
		t.Fatalf("expected early stop well before MaxTrials, ran %d", result.TrialsRun)
	}
	if result.DistinctTraces == 0 {
		t.Fatal("expected at least one distinct trace shape to be found")
	}
	t.Logf("ran %d trials, found %d distinct schedule shapes before going dry", result.TrialsRun, result.DistinctTraces)
}

func TestNoveltySearchReportsFirstFailingSeed(t *testing.T) {
	trial := func(seed int64) (Trace, error) {
		s := NewSched(seed)
		defer s.Shutdown()
		chaoticScenario(s)
		if err := s.Run(); err != nil {
			return Trace{}, err
		}
		if seed == 17 {
			return Trace{}, errRunFailure
		}
		return s.Trace(), nil
	}

	result := NoveltySearch(NoveltySearchConfig{StartSeed: 1, MaxTrials: 100}, trial)
	if result.FailedSeed != 17 {
		t.Fatalf("expected FailedSeed=17, got %d", result.FailedSeed)
	}
	if result.FailedErr != errRunFailure {
		t.Fatalf("expected errRunFailure, got %v", result.FailedErr)
	}
	if result.TrialsRun != 17 {
		t.Fatalf("expected to stop at trial 17, ran %d", result.TrialsRun)
	}
}
