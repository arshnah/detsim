package rt

import (
	"fmt"
	"strings"
)

// NoveltySearchConfig configures NoveltySearch. Zero values fall back to defaults.
type NoveltySearchConfig struct {
	StartSeed int64
	MaxTrials int
	DryLimit  int
	PrefixLen int
}

// NoveltySearchResult reports how a NoveltySearch run went.
type NoveltySearchResult struct {
	TrialsRun      int
	DistinctTraces int
	StoppedDry     bool
	FailedSeed     int64
	FailedErr      error
}

// NoveltySearch tries increasing seeds until DryLimit consecutive trials produce no new
// schedule shape, calling trial once per seed.
func NoveltySearch(cfg NoveltySearchConfig, trial func(seed int64) (Trace, error)) NoveltySearchResult {
	prefixLen := cfg.PrefixLen
	if prefixLen <= 0 {
		prefixLen = 8
	}
	dryLimit := cfg.DryLimit
	if dryLimit <= 0 {
		dryLimit = 20
	}
	maxTrials := cfg.MaxTrials
	if maxTrials <= 0 {
		maxTrials = 10000
	}

	seen := make(map[string]bool)
	var result NoveltySearchResult
	dryStreak := 0

	for i := 0; i < maxTrials; i++ {
		seed := cfg.StartSeed + int64(i)
		trace, err := trial(seed)
		result.TrialsRun++

		if err != nil {
			result.FailedSeed = seed
			result.FailedErr = err
			return result
		}

		key := tracePrefixKey(trace.Decisions, prefixLen)
		if seen[key] {
			dryStreak++
		} else {
			seen[key] = true
			result.DistinctTraces++
			dryStreak = 0
		}

		if dryStreak >= dryLimit {
			result.StoppedDry = true
			break
		}
	}

	return result
}

func tracePrefixKey(decisions []uint64, n int) string {
	if n > len(decisions) {
		n = len(decisions)
	}
	var b strings.Builder
	for _, d := range decisions[:n] {
		fmt.Fprintf(&b, "%d,", d)
	}
	return b.String()
}
