package lowdiversity

import (
	"errors"
	"testing"

	"github.com/arshnah/detsim/rt"
)

func TestSingleGoroutineNoDiversity(t *testing.T) {
	sched, err := rt.SchedFromEnv("DETSIM_SEED", "DETSIM_TRACE", 1)
	if err != nil {
		t.Fatalf("rt.SchedFromEnv() = %v", err)
	}
	if n := rt.DecisionLimitFromEnv("DETSIM_PEEK_DECISIONS"); n > 0 {
		sched.SetDecisionLimit(n)
	}
	rt.DumpTraceToEnvPath(t, sched, "DETSIM_TRACE_OUT")

	sum := 0
	sched.Go(func() {
		for i := 0; i < 5; i++ {
			sum += i
		}
	})
	err = sched.Run()
	if errors.Is(err, rt.ErrDecisionLimit) {
		return
	}
	if err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if sum != 10 {
		t.Fatalf("expected 10, got %d", sum)
	}
}
