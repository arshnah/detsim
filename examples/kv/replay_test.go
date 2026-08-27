package kv

import (
	"fmt"
	"testing"

	"github.com/arshnah/detsim"
)

func TestRecoverIsIdempotent(t *testing.T) {
	fs := detsim.NewFaultyStorage(1, detsim.FaultProfile{})
	s := NewStore(fs, true)
	s.Put("a", "1")
	s.Put("b", "2")
	s.Sync()

	first := NewStore(fs, true)
	first.Recover(1 << 20)

	second := NewStore(fs, true)
	second.Recover(1 << 20)

	if first.Len() != second.Len() {
		t.Fatalf("two recoveries of the same unchanged storage produced different sizes: %d vs %d", first.Len(), second.Len())
	}
	for _, key := range []string{"a", "b"} {
		v1, ok1 := first.Get(key)
		v2, ok2 := second.Get(key)
		if ok1 != ok2 || v1 != v2 {
			t.Fatalf("recovery is not idempotent for key %q: first=%q(%v) second=%q(%v)", key, v1, ok1, v2, ok2)
		}
	}
}

func TestRecoverThenContinueWritingAppendsCorrectly(t *testing.T) {
	fs := detsim.NewFaultyStorage(2, detsim.FaultProfile{})
	s := NewStore(fs, true)
	s.Put("before-crash-1", "a")
	s.Put("before-crash-2", "b")
	s.Sync()
	fs.Crash()

	recovered := NewStore(fs, true)
	recovered.Recover(1 << 20)
	if recovered.Len() != 2 {
		t.Fatalf("expected 2 entries recovered before continuing, got %d", recovered.Len())
	}

	recovered.Put("after-recovery-1", "c")
	recovered.Put("after-recovery-2", "d")
	recovered.Sync()

	final := NewStore(fs, true)
	final.Recover(1 << 20)

	want := map[string]string{
		"before-crash-1":   "a",
		"before-crash-2":   "b",
		"after-recovery-1": "c",
		"after-recovery-2": "d",
	}
	if final.Len() != len(want) {
		t.Fatalf("expected %d entries after write-post-recovery then re-recovering, got %d", len(want), final.Len())
	}
	for k, wantV := range want {
		gotV, ok := final.Get(k)
		if !ok || gotV != wantV {
			t.Fatalf("key %q: got %q(%v) want %q, writes after recovery must not corrupt or overwrite what was recovered", k, gotV, ok, wantV)
		}
	}
}

func TestRepeatedCrashRecoverCyclesConverge(t *testing.T) {
	const trials = 200
	for seed := int64(1); seed <= trials; seed++ {
		profile := detsim.FaultProfile{TornWriteRate: 0.1}
		fs := detsim.NewFaultyStorage(seed, profile)
		s := NewStore(fs, true)

		confirmed := make(map[string]string)
		for cycle := 0; cycle < 4; cycle++ {
			for i := 0; i < 3; i++ {
				key := fmt.Sprintf("cycle%d-key%d", cycle, i)
				value := fmt.Sprintf("value-%d-%d-%d", seed, cycle, i)
				s.Put(key, value)
			}
			s.Sync()
			fs.Crash()

			s = NewStore(fs, true)
			s.Recover(1 << 20)

			for key, wantValue := range confirmed {
				gotValue, ok := s.Get(key)
				if !ok {
					t.Fatalf("seed=%d cycle=%d: key %q was successfully recovered in an earlier cycle but has now disappeared, recovery must be monotonic, reproduce with this seed", seed, cycle, key)
				}
				if gotValue != wantValue {
					t.Fatalf("seed=%d cycle=%d: key %q changed value across cycles: had %q, now %q, reproduce with this seed", seed, cycle, key, wantValue, gotValue)
				}
			}

			for i := 0; i < 3; i++ {
				key := fmt.Sprintf("cycle%d-key%d", cycle, i)
				if value, ok := s.Get(key); ok {
					confirmed[key] = value
				}
			}
		}
	}
}
