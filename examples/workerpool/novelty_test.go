package workerpool

import (
	"testing"

	"github.com/arshnah/detsim/rt"
)

func TestNoveltySearchExploresDistinctSchedules(t *testing.T) {
	trial := func(seed int64) (rt.Trace, error) {
		s := rt.NewSched(seed)
		defer s.Shutdown()
		results, err := Run(s, 4, makeJobs(20))
		if err != nil {
			return rt.Trace{}, err
		}
		if len(results) != 20 {
			t.Fatalf("seed %d: expected 20 results, got %d", seed, len(results))
		}
		return s.Trace(), nil
	}

	result := rt.NoveltySearch(rt.NoveltySearchConfig{
		StartSeed: 1,
		MaxTrials: 2000,
		DryLimit:  50,
		PrefixLen: 10,
	}, trial)

	if result.FailedErr != nil {
		t.Fatalf("seed %d failed: %v", result.FailedSeed, result.FailedErr)
	}
	if result.DistinctTraces == 0 {
		t.Fatal("expected at least one distinct schedule shape")
	}
	t.Logf("ran %d trials, found %d distinct schedule shapes, stopped dry=%v", result.TrialsRun, result.DistinctTraces, result.StoppedDry)
}
