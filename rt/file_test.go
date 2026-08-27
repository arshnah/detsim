package rt

import (
	"io"
	"testing"

	"github.com/arshnah/detsim"
)

func TestFileWriteThenReadRoundTrips(t *testing.T) {
	s := NewSched(1)
	fs := NewFileSystem(s, detsim.FaultProfile{})

	w, err := fs.Create("a.txt")
	if err != nil {
		t.Fatalf("Create() = %v", err)
	}
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatalf("Write() = %v", err)
	}
	if err := w.Sync(); err != nil {
		t.Fatalf("Sync() = %v", err)
	}

	r, err := fs.Open("a.txt")
	if err != nil {
		t.Fatalf("Open() = %v", err)
	}
	buf := make([]byte, 5)
	n, err := r.Read(buf)
	if err != nil {
		t.Fatalf("Read() = %v", err)
	}
	if n != 5 || string(buf) != "hello" {
		t.Fatalf("got %q, want %q", buf[:n], "hello")
	}
}

func TestFileReadReturnsEOFAtEnd(t *testing.T) {
	s := NewSched(1)
	fs := NewFileSystem(s, detsim.FaultProfile{})

	w, _ := fs.Create("b.txt")
	w.Write([]byte("hi"))
	w.Sync()

	r, _ := fs.Open("b.txt")
	buf := make([]byte, 2)
	if _, err := r.Read(buf); err != nil {
		t.Fatalf("first Read() = %v", err)
	}
	if _, err := r.Read(buf); err != io.EOF {
		t.Fatalf("expected io.EOF at end, got %v", err)
	}
}

func TestDifferentFilenamesAreIndependentDisks(t *testing.T) {
	s := NewSched(1)
	fs := NewFileSystem(s, detsim.FaultProfile{})

	a, _ := fs.Create("a.txt")
	a.Write([]byte("AAAA"))
	a.Sync()

	b, _ := fs.Create("b.txt")
	b.Write([]byte("BBBB"))
	b.Sync()

	ra, _ := fs.Open("a.txt")
	bufA := make([]byte, 4)
	ra.Read(bufA)
	if string(bufA) != "AAAA" {
		t.Fatalf("a.txt got %q", bufA)
	}

	rb, _ := fs.Open("b.txt")
	bufB := make([]byte, 4)
	rb.Read(bufB)
	if string(bufB) != "BBBB" {
		t.Fatalf("b.txt got %q", bufB)
	}
}

func TestFileFaultInjectionIsDeterministicPerSeed(t *testing.T) {
	run := func(seed int64) string {
		s := NewSched(seed)
		fs := NewFileSystem(s, detsim.FaultProfile{CorruptByteRate: 0.9})
		w, _ := fs.Create("c.txt")
		w.Write([]byte("determinism"))
		w.Sync()
		r, _ := fs.Open("c.txt")
		buf := make([]byte, 11)
		r.Read(buf)
		return string(buf)
	}

	a := run(5)
	b := run(5)
	if a != b {
		t.Fatalf("same seed produced different fault injection: %q vs %q", a, b)
	}
}

func TestFileNameSeekAndPositionedIO(t *testing.T) {
	s := NewSched(1)
	fs := NewFileSystem(s, detsim.FaultProfile{})

	f, err := fs.Create("seek.txt")
	if err != nil {
		t.Fatalf("Create() = %v", err)
	}
	if got := f.Name(); got != "seek.txt" {
		t.Fatalf("Name() = %q, want %q", got, "seek.txt")
	}

	if _, err := f.WriteAt([]byte("hello world"), 0); err != nil {
		t.Fatalf("WriteAt() = %v", err)
	}
	if err := f.Sync(); err != nil {
		t.Fatalf("Sync() = %v", err)
	}

	buf := make([]byte, 5)
	if n, err := f.ReadAt(buf, 6); n != 5 || string(buf) != "world" || (err != nil && err != io.EOF) {
		t.Fatalf("ReadAt(6) = (%d, %q, %v)", n, buf, err)
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("Seek(start) = %v", err)
	}
	n, err := f.Read(buf)
	if n != 5 || string(buf) != "hello" || err != nil {
		t.Fatalf("Read after Seek(start) = (%d, %q, %v), want (5, hello, nil)", n, buf, err)
	}

	pos, err := f.Seek(-3, io.SeekEnd)
	if err != nil || pos != 8 {
		t.Fatalf("Seek(-3, end) = (%d, %v), want (8, nil)", pos, err)
	}
	n, _ = f.Read(buf)
	if n != 3 || string(buf[:n]) != "rld" {
		t.Fatalf("Read after Seek(end-3) = (%d, %q)", n, buf)
	}

	if _, err := f.Seek(-100, io.SeekStart); err == nil {
		t.Fatal("expected Seek before start of file to fail")
	}
}
