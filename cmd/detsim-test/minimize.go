package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/arshnah/detsim/minimize"
	"github.com/arshnah/detsim/rt"
)

func minimizeAndReport(cfg sweepConfig, packageDir, traceFile, seedEnv, traceEnv string, seed int64) error {
	tracePath := filepath.Join(packageDir, traceFile)
	os.Remove(tracePath)

	seedEnvKV := fmt.Sprintf("%s=%d", seedEnv, seed)
	if runGoTest(cfg.dir, cfg.overlayPath, cfg.target, cfg.runPattern, seedEnvKV) {
		return errors.New("re-running the first failing seed to capture its trace unexpectedly passed, the scheduler may not be fully deterministic")
	}

	original, err := rt.LoadTrace(tracePath)
	if err != nil {
		return fmt.Errorf("reading failure trace at %s (did the test write one via rt.DumpTraceOnFailure?): %w", tracePath, err)
	}

	fmt.Printf("minimizing trace of %d decisions from seed %d...\n", len(original.Decisions), seed)

	tmpDir, err := os.MkdirTemp("", "detsim-minimize-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	stillFails := func(candidate []uint64) bool {
		return !tryTrace(cfg, tmpDir, traceEnv, rt.Trace{Seed: original.Seed, Decisions: candidate})
	}

	minimized := minimize.Ddmin(original.Decisions, stillFails)

	outPath := filepath.Join(packageDir, "detsim_minimized_trace.json")
	if err := rt.SaveTrace(outPath, withMinimizedDetail(original, minimized)); err != nil {
		return err
	}

	fmt.Printf("minimized %d decisions down to %d\n", len(original.Decisions), len(minimized))
	fmt.Printf("minimal reproducer: %s=%s go test -overlay=%s -run=%s %s\n",
		traceEnv, outPath, cfg.overlayPath, cfg.runPattern, cfg.target)
	return nil
}

func tryTrace(cfg sweepConfig, tmpDir, traceEnv string, trace rt.Trace) bool {
	path := filepath.Join(tmpDir, fmt.Sprintf("candidate-%d.json", len(trace.Decisions)))
	if err := rt.SaveTrace(path, trace); err != nil {
		return true
	}
	env := fmt.Sprintf("%s=%s", traceEnv, path)
	return runGoTest(cfg.dir, cfg.overlayPath, cfg.target, cfg.runPattern, env)
}

// withMinimizedDetail carries the original trace's goroutine labels and steps into the
// minimized trace, so detsim-trace can show names and block reasons instead of bare
// ids. Steps follow their decisions: a step is kept only when the decision before it
// survived, and if the minimized sequence somehow isn't a subsequence of the original
// the steps are dropped rather than misaligned.
func withMinimizedDetail(original rt.Trace, minimized []uint64) rt.Trace {
	out := rt.Trace{Seed: original.Seed, Decisions: minimized, Labels: original.Labels}
	j := 0
	for i, d := range original.Decisions {
		if j < len(minimized) && d == minimized[j] {
			if i < len(original.Steps) {
				out.Steps = append(out.Steps, original.Steps[i])
			}
			j++
		}
	}
	if j != len(minimized) {
		out.Steps = nil
	}
	return out
}
