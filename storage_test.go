package detsim

import (
	"errors"
	"fmt"
	"io"
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

func TestSizeReflectsSyncedAndPendingWrites(t *testing.T) {
	fs := NewFaultyStorage(1, FaultProfile{})
	if got := fs.Size(); got != 0 {
		t.Fatalf("expected size 0 on a fresh disk, got %d", got)
	}

	fs.WriteAt([]byte("hello"), 0)
	if got := fs.Size(); got != 5 {
		t.Fatalf("expected size 5 with a pending write, got %d", got)
	}

	fs.Sync()
	if got := fs.Size(); got != 5 {
		t.Fatalf("expected size 5 after sync, got %d", got)
	}

	fs.WriteAt([]byte("world"), 10)
	if got := fs.Size(); got != 15 {
		t.Fatalf("expected size to grow to cover the furthest pending write, got %d", got)
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

func TestReadAtFollowsIoReaderAtContract(t *testing.T) {
	fs := NewFaultyStorage(1, FaultProfile{})
	fs.WriteAt([]byte("hello"), 0)
	fs.Sync()

	buf := make([]byte, 5)
	if n, err := fs.ReadAt(buf, 0); n != 5 || err != nil {
		t.Fatalf("full read: got (%d, %v), want (5, nil)", n, err)
	}
	if n, err := fs.ReadAt(buf, 1); n != 4 || !errors.Is(err, io.EOF) {
		t.Fatalf("short read: got (%d, %v), want (4, io.EOF)", n, err)
	}
	if n, err := fs.ReadAt(buf, 5); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("read at end: got (%d, %v), want (0, io.EOF)", n, err)
	}
	if n, err := fs.ReadAt(buf, 99); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("read past end: got (%d, %v), want (0, io.EOF)", n, err)
	}

	var r io.ReaderAt = fs // must satisfy the interface
	if r == nil {
		t.Fatal("unreachable")
	}
}

func TestNegativeOffsetsAreRejectedNotPanics(t *testing.T) {
	fs := NewFaultyStorage(1, FaultProfile{})
	buf := make([]byte, 4)

	if _, err := fs.ReadAt(buf, -1); err == nil {
		t.Fatal("expected ReadAt(-1) to return an error")
	}
	if _, err := fs.WriteAt([]byte("x"), -1); err == nil {
		t.Fatal("expected WriteAt(-1) to return an error")
	}
}
