package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/arshnah/detsim/rewrite"
)

func main() {
	os.Exit(run())
}

func run() int {
	dir := flag.String("dir", ".", "directory to load the package from")
	pkg := flag.String("pkg", ".", "package pattern to rewrite")
	goPkg := flag.String("go-pkg", "", "go test package pattern, defaults to -pkg")
	run := flag.String("run", "", "go test -run pattern")
	seedEnv := flag.String("seed-env", "DETSIM_SEED", "environment variable the test harness reads its seed from")
	traceEnv := flag.String("trace-env", "DETSIM_TRACE", "environment variable the test harness reads a replay trace path from")
	traceFile := flag.String("trace-file", "detsim_failure_trace.json", "filename the harness writes a failure trace to, relative to the package directory")
	startSeed := flag.Int64("seed", 1, "first seed to try")
	trials := flag.Int64("trials", 100, "number of seeds to try (novelty mode: the trial cap, not a target)")
	keepGoing := flag.Bool("keep-going", false, "keep sweeping after the first failing seed instead of stopping")
	doMinimize := flag.Bool("minimize", false, "delta-debug the first failing seed's trace down to a minimal reproducer")

	novelty := flag.Bool("novelty", false, "sweep seeds by schedule novelty instead of a fixed trial count, stopping once new schedule shapes stop appearing")
	noveltyTraceEnv := flag.String("novelty-trace-env", "DETSIM_TRACE_OUT", "environment variable the test harness reads an always-dump trace path from (novelty mode); the test must call rt.DumpTraceToEnvPath(t, sched, this-var)")
	noveltyPeekEnv := flag.String("novelty-peek-env", "DETSIM_PEEK_DECISIONS", "environment variable the test harness reads a decision limit from to stop the scheduler early (novelty mode); the test must call sched.SetDecisionLimit(n) when this is set, see rt.DecisionLimitFromEnv")
	noveltyPrefix := flag.Int("novelty-prefix", 8, "number of leading schedule decisions used to fingerprint a trace as novel; also the decision limit used for the cheap peek pass (novelty mode)")
	noveltyDry := flag.Int("novelty-dry", 20, "stop after this many consecutive trials produce no new schedule shape (novelty mode)")
	flag.Parse()

	target := *goPkg
	if target == "" {
		target = *pkg
	}

	res, err := rewrite.Rewrite(*dir, *pkg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer res.Close()

	for _, w := range res.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s: %s\n", w.Pos, w.Message)
	}

	sw := sweepConfig{
		dir:         *dir,
		overlayPath: res.OverlayPath,
		target:      target,
		runPattern:  *run,
		seedEnv:     *seedEnv,
		keepGoing:   *keepGoing,
	}

	if *novelty {
		return runNoveltyMode(sw, res, *noveltyTraceEnv, *noveltyPeekEnv, *seedEnv, *traceEnv, *traceFile, *startSeed, *trials, *noveltyPrefix, *noveltyDry, *doMinimize)
	}

	result := runSweep(sw, *startSeed, *trials)

	fmt.Printf("%d/%d seeds passed\n", result.passed, result.tried)
	if len(result.failedSeeds) == 0 {
		return 0
	}
	fmt.Printf("failing seeds: %v\n", result.failedSeeds)

	firstFailSeed := result.failedSeeds[0]
	fmt.Printf("reproduce with: %s=%d go test -overlay=%s -run=%s %s\n",
		*seedEnv, firstFailSeed, res.OverlayPath, *run, target)

	if *doMinimize {
		if err := minimizeAndReport(sw, res.PackageDir, *traceFile, *seedEnv, *traceEnv, firstFailSeed); err != nil {
			fmt.Fprintln(os.Stderr, "minimize:", err)
		}
	}

	return 1
}

func runNoveltyMode(sw sweepConfig, res *rewrite.Result, noveltyTraceEnv, noveltyPeekEnv, seedEnv, traceEnv, traceFile string, startSeed, trials int64, prefix, dry int, doMinimize bool) int {
	result, err := runNoveltySweep(sw, noveltyTraceEnv, noveltyPeekEnv, startSeed, trials, prefix, dry)
	if err != nil {
		fmt.Fprintln(os.Stderr, "novelty:", err)
		return 1
	}

	fmt.Printf("%d peek trials, %d full trials, %d distinct schedule shapes found, stopped dry=%v\n",
		result.peekTrials, result.fullTrials, result.distinctTraces, result.stoppedDry)

	if result.failedOutput == "" {
		return 0
	}

	fmt.Print(result.failedOutput)
	fmt.Printf("seed=%d FAILED\n", result.failedSeed)
	fmt.Printf("reproduce with: %s=%d go test -overlay=%s -run=%s %s\n",
		seedEnv, result.failedSeed, res.OverlayPath, sw.runPattern, sw.target)

	if doMinimize {
		if err := minimizeAndReport(sw, res.PackageDir, traceFile, seedEnv, traceEnv, result.failedSeed); err != nil {
			fmt.Fprintln(os.Stderr, "minimize:", err)
		}
	}

	return 1
}
