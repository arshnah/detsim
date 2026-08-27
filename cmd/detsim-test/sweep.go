package main

import (
	"fmt"
	"os"
	"os/exec"
)

type sweepConfig struct {
	dir         string
	overlayPath string
	target      string
	runPattern  string
	seedEnv     string
	keepGoing   bool
}

type sweepResult struct {
	passed      int64
	tried       int64
	failedSeeds []int64
}

func runSweep(cfg sweepConfig, startSeed, trials int64) sweepResult {
	var res sweepResult
	for seed := startSeed; seed < startSeed+trials; seed++ {
		env := fmt.Sprintf("%s=%d", cfg.seedEnv, seed)
		if runGoTest(cfg.dir, cfg.overlayPath, cfg.target, cfg.runPattern, env) {
			res.passed++
		} else {
			fmt.Printf("seed=%d FAILED\n", seed)
			res.failedSeeds = append(res.failedSeeds, seed)
			if !cfg.keepGoing {
				res.tried = res.passed + int64(len(res.failedSeeds))
				return res
			}
		}
	}
	res.tried = res.passed + int64(len(res.failedSeeds))
	return res
}

func runGoTest(dir, overlayPath, target, runPattern string, envs ...string) bool {
	ok, out := runGoTestOutput(dir, overlayPath, target, runPattern, envs...)
	if !ok {
		fmt.Print(out)
	}
	return ok
}

func runGoTestOutput(dir, overlayPath, target, runPattern string, envs ...string) (bool, string) {
	args := []string{"test", "-overlay=" + overlayPath, "-count=1"}
	if runPattern != "" {
		args = append(args, "-run="+runPattern)
	}
	args = append(args, target)

	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), envs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, string(out)
	}
	return true, string(out)
}
