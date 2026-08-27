package detsim

import "testing"

func TestWriteAtBeyondMaxSizeFailsCleanly(t *testing.T) {
	fs := NewFaultyStorage(1, FaultProfile{MaxSize: 100})

	n, err := fs.WriteAt([]byte("this fits fine"), 0)
	if err != nil {
		t.Fatalf("expected a write within capacity to succeed, got %v", err)
	}
	if n != len("this fits fine") {
		t.Fatalf("expected n=%d, got %d", len("this fits fine"), n)
	}

	_, err = fs.WriteAt([]byte("this one does not fit"), 90)
	if err != ErrDiskFull {
		t.Fatalf("expected ErrDiskFull for a write that would exceed MaxSize, got %v", err)
	}
}

func TestDiskFullDoesNotSilentlyQueueTheRejectedWrite(t *testing.T) {
	fs := NewFaultyStorage(1, FaultProfile{MaxSize: 10})

	fs.WriteAt([]byte("0123456789"), 0)
	fs.Sync()

	_, err := fs.WriteAt([]byte("overflow"), 10)
	if err != ErrDiskFull {
		t.Fatalf("expected ErrDiskFull, got %v", err)
	}
	fs.Sync()

	buf := make([]byte, 20)
	n, _ := fs.ReadAt(buf, 0)
	if n != 10 {
		t.Fatalf("expected the rejected write to never have landed on disk, read back %d bytes beyond the original 10", n)
	}
}

func TestUnlimitedByDefault(t *testing.T) {
	fs := NewFaultyStorage(1, FaultProfile{})
	_, err := fs.WriteAt(make([]byte, 1<<20), 0)
	if err != nil {
		t.Fatalf("expected no MaxSize (zero value) to mean unlimited, got %v", err)
	}
}
