package rt

import (
	"os"
	"path/filepath"
	"testing"
)

type fakeT struct {
	failed   bool
	cleanups []func()
	logs     []string
}

func (f *fakeT) Failed() bool      { return f.failed }
func (f *fakeT) Cleanup(fn func()) { f.cleanups = append(f.cleanups, fn) }
func (f *fakeT) Logf(format string, args ...any) {
	f.logs = append(f.logs, format)
}
func (f *fakeT) runCleanups() {
	for _, fn := range f.cleanups {
		fn()
	}
}

func TestDumpTraceToEnvPathWritesRegardlessOfOutcome(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.json")
	t.Setenv("DETSIM_TEST_TRACE_OUT", path)

	s := NewSched(1)
	s.Go(func() {})
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}

	ft := &fakeT{failed: false}
	DumpTraceToEnvPath(ft, s, "DETSIM_TEST_TRACE_OUT")
	ft.runCleanups()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected trace file to exist even on a passing run: %v", err)
	}

	loaded, err := LoadTrace(path)
	if err != nil {
		t.Fatalf("LoadTrace() = %v", err)
	}
	if loaded.Seed != s.Seed {
		t.Fatalf("expected seed %d, got %d", s.Seed, loaded.Seed)
	}
}

func TestDumpTraceToEnvPathNoopWhenEnvUnset(t *testing.T) {
	os.Unsetenv("DETSIM_TEST_TRACE_OUT_UNSET")
	s := NewSched(1)
	ft := &fakeT{}
	DumpTraceToEnvPath(ft, s, "DETSIM_TEST_TRACE_OUT_UNSET")
	ft.runCleanups()
	if len(ft.logs) != 0 {
		t.Fatalf("expected no cleanup work when env var unset, got logs: %v", ft.logs)
	}
}
