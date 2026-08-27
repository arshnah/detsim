package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/arshnah/detsim/rt"
)

type noveltyResult struct {
	peekTrials     int64
	fullTrials     int64
	distinctTraces int
	stoppedDry     bool
	failedSeed     int64
	failedOutput   string
}

func runNoveltySweep(cfg sweepConfig, traceEnv, peekEnv string, startSeed, maxTrials int64, prefixLen, dryLimit int) (noveltyResult, error) {
	tmpDir, err := os.MkdirTemp("", "detsim-novelty-")
	if err != nil {
		return noveltyResult{}, err
	}
	defer os.RemoveAll(tmpDir)

	tracePath := filepath.Join(tmpDir, "trace.json")
	seen := make(map[string]bool)
	dryStreak := 0
	var result noveltyResult

	for i := int64(0); i < maxTrials; i++ {
		seed := startSeed + i
		os.Remove(tracePath)

		seedEnvKV := fmt.Sprintf("%s=%d", cfg.seedEnv, seed)
		traceEnvKV := fmt.Sprintf("%s=%s", traceEnv, tracePath)
		peekEnvKV := fmt.Sprintf("%s=%d", peekEnv, prefixLen)

		ok, out := runGoTestOutput(cfg.dir, cfg.overlayPath, cfg.target, cfg.runPattern, seedEnvKV, traceEnvKV, peekEnvKV)
		result.peekTrials++
		if !ok {
			result.failedSeed = seed
			result.failedOutput = out
			return result, nil
		}

		trace, err := rt.LoadTrace(tracePath)
		if err != nil {
			return result, fmt.Errorf("seed %d produced no trace at %s, does the test call rt.DumpTraceToEnvPath(t, sched, %q)? (%w)", seed, tracePath, traceEnv, err)
		}

		key := tracePrefixKey(trace.Decisions, prefixLen)
		if seen[key] {
			dryStreak++
			if dryStreak >= dryLimit {
				result.stoppedDry = true
				break
			}
			continue
		}

		seen[key] = true
		result.distinctTraces++
		dryStreak = 0

		os.Remove(tracePath)
		fullOK, fullOut := runGoTestOutput(cfg.dir, cfg.overlayPath, cfg.target, cfg.runPattern, seedEnvKV, traceEnvKV)
		result.fullTrials++
		if !fullOK {
			result.failedSeed = seed
			result.failedOutput = fullOut
			return result, nil
		}
	}

	return result, nil
}

func tracePrefixKey(decisions []uint64, n int) string {
	if n > len(decisions) {
		n = len(decisions)
	}
	key := ""
	for _, d := range decisions[:n] {
		key += fmt.Sprintf("%d,", d)
	}
	return key
}
