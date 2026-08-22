package detsim

import (
	"fmt"
	"testing"
)

func TestCrashLosesUnsyncedWrites(t *testing.T) {
	fs := NewFaultyStorage(1, FaultProfile{})
	fs.WriteAt([]byte("hello"), 0)
	fs.Crash()

	buf := make([]byte, 5)
	n, _ := fs.ReadAt(buf, 0)
	if n != 0 {
		t.Fatalf("expected unsynced write to be lost on crash, read back %d bytes", n)
	}
}

func TestSyncedWritesSurviveCrash(t *testing.T) {
	fs := NewFaultyStorage(1, FaultProfile{})
	fs.WriteAt([]byte("hello"), 0)
	fs.Sync()
	fs.Crash()

	buf := make([]byte, 5)
	n, _ := fs.ReadAt(buf, 0)
	if n != 5 || string(buf) != "hello" {
		t.Fatalf("expected synced write to survive crash, got %q", buf[:n])
	}
}

type naiveKV struct{ s *FaultyStorage }

func (k *naiveKV) Put(offset int64, value string) { k.s.WriteAt([]byte(value), offset) }
func (k *naiveKV) Get(offset int64, n int) string {
	buf := make([]byte, n)
	m, _ := k.s.ReadAt(buf, offset)
	return string(buf[:m])
}

func TestNaiveKVLosesDataOnCrash(t *testing.T) {
	fs := NewFaultyStorage(1, FaultProfile{})
	kv := &naiveKV{s: fs}
	kv.Put(0, "important")
	fs.Crash()

	got := kv.Get(0, len("important"))
	if got == "important" {
		t.Fatal("naiveKV unexpectedly survived a crash without ever calling Sync, so the fault harness isn't actually exercising anything")
	}
}

func TestTornWritesEventuallyOccurUnderFaultProfile(t *testing.T) {
	profile := FaultProfile{TornWriteRate: 0.5}
	fs := NewFaultyStorage(3, profile)

	sawTorn := false
	for i := 0; i < 100; i++ {
		want := fmt.Sprintf("%010d", i)
		fs.WriteAt([]byte(want), 0)
		fs.Sync()
		buf := make([]byte, 10)
		n, _ := fs.ReadAt(buf, 0)
		if n < 10 || string(buf[:n]) != want {
			sawTorn = true
			break
		}
	}
	if !sawTorn {
		t.Fatal("expected TornWriteRate=0.5 to produce at least one torn write across 100 attempts")
	}
}
