package workerpool

import (
	"errors"
	"testing"

	"github.com/arshnah/detsim/rt"
)

func TestSweepableRun(t *testing.T) {
	sched, err := rt.SchedFromEnv("DETSIM_SEED", "DETSIM_TRACE", 1)
	if err != nil {
		t.Fatalf("rt.SchedFromEnv() = %v", err)
	}
	if n := rt.DecisionLimitFromEnv("DETSIM_PEEK_DECISIONS"); n > 0 {
		sched.SetDecisionLimit(n)
	}
	rt.DumpTraceOnFailure(t, sched, "detsim_failure_trace.json")
	rt.DumpTraceToEnvPath(t, sched, "DETSIM_TRACE_OUT")

	results, err := Run(sched, 4, makeJobs(20))
	if errors.Is(err, rt.ErrDecisionLimit) {
		return
	}
	if err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if len(results) != 20 {
		t.Fatalf("expected 20 results, got %d", len(results))
	}
}
