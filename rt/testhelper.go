package rt

import "os"

// TestingT is the minimal subset of *testing.T DumpTraceOnFailure/DumpTraceToEnvPath need.
type TestingT interface {
	Failed() bool
	Cleanup(func())
	Logf(format string, args ...any)
}

// DumpTraceOnFailure saves sched's trace to path only if t ends up failed.
func DumpTraceOnFailure(t TestingT, sched *Sched, path string) {
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		if err := SaveTrace(path, sched.Trace()); err != nil {
			t.Logf("detsim/rt: could not save failure trace: %v", err)
			return
		}
		t.Logf("detsim/rt: test failed at seed=%d, trace saved to %s", sched.Seed, path)
	})
}

// DumpTraceToEnvPath saves sched's trace to the path named by envVar, regardless of pass/fail.
func DumpTraceToEnvPath(t TestingT, sched *Sched, envVar string) {
	path := os.Getenv(envVar)
	if path == "" {
		return
	}
	t.Cleanup(func() {
		if err := SaveTrace(path, sched.Trace()); err != nil {
			t.Logf("detsim/rt: could not save trace to %s: %v", path, err)
		}
	})
}
