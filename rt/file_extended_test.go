package rt

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/arshnah/detsim"
)

func TestOpenNonexistentFileFailsWithNotExist(t *testing.T) {
	s := NewSched(1)
	fsys := NewFileSystem(s, detsim.FaultProfile{})

	_, err := fsys.Open("missing.txt")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected fs.ErrNotExist, got %v", err)
	}
}

func TestCreateTruncatesExistingFile(t *testing.T) {
	s := NewSched(1)
	fsys := NewFileSystem(s, detsim.FaultProfile{})

	w1, _ := fsys.Create("a.txt")
	w1.Write([]byte("original longer content"))
	w1.Sync()

	w2, _ := fsys.Create("a.txt")
	w2.Write([]byte("new"))
	w2.Sync()

	r, err := fsys.Open("a.txt")
	if err != nil {
		t.Fatalf("Open() = %v", err)
	}
	buf := make([]byte, 32)
	n, _ := r.Read(buf)
	if string(buf[:n]) != "new" {
		t.Fatalf("expected truncated content %q, got %q", "new", buf[:n])
	}
}

func TestRemoveThenOpenFails(t *testing.T) {
	s := NewSched(1)
	fsys := NewFileSystem(s, detsim.FaultProfile{})

	w, _ := fsys.Create("a.txt")
	w.Write([]byte("data"))
	w.Sync()

	if err := fsys.Remove("a.txt"); err != nil {
		t.Fatalf("Remove() = %v", err)
	}
	if _, err := fsys.Open("a.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected fs.ErrNotExist after Remove, got %v", err)
	}
}

func TestRemoveNonexistentFails(t *testing.T) {
	s := NewSched(1)
	fsys := NewFileSystem(s, detsim.FaultProfile{})
	if err := fsys.Remove("nope.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected fs.ErrNotExist, got %v", err)
	}
}

func TestRenameMovesContent(t *testing.T) {
	s := NewSched(1)
	fsys := NewFileSystem(s, detsim.FaultProfile{})

	w, _ := fsys.Create("old.txt")
	w.Write([]byte("hello"))
	w.Sync()

	if err := fsys.Rename("old.txt", "new.txt"); err != nil {
		t.Fatalf("Rename() = %v", err)
	}
	if _, err := fsys.Open("old.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected old.txt to no longer exist, got %v", err)
	}
	r, err := fsys.Open("new.txt")
	if err != nil {
		t.Fatalf("Open(new.txt) = %v", err)
	}
	buf := make([]byte, 5)
	n, _ := r.Read(buf)
	if string(buf[:n]) != "hello" {
		t.Fatalf("expected content to survive the rename, got %q", buf[:n])
	}
}

func TestStatReportsSizeAndNotExist(t *testing.T) {
	s := NewSched(1)
	fsys := NewFileSystem(s, detsim.FaultProfile{})

	if _, err := fsys.Stat("missing.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected fs.ErrNotExist, got %v", err)
	}

	w, _ := fsys.Create("a.txt")
	w.Write([]byte("hello"))
	w.Sync()

	info, err := fsys.Stat("a.txt")
	if err != nil {
		t.Fatalf("Stat() = %v", err)
	}
	if info.Name() != "a.txt" {
		t.Fatalf("expected name a.txt, got %q", info.Name())
	}
	if info.Size() != 5 {
		t.Fatalf("expected size 5, got %d", info.Size())
	}
	if info.IsDir() {
		t.Fatal("expected IsDir() = false")
	}
}

func TestReadFileAndWriteFileRoundTrip(t *testing.T) {
	s := NewSched(1)
	fsys := NewFileSystem(s, detsim.FaultProfile{})

	if err := fsys.WriteFile("a.txt", []byte("hello world")); err != nil {
		t.Fatalf("WriteFile() = %v", err)
	}
	got, err := fsys.ReadFile("a.txt")
	if err != nil {
		t.Fatalf("ReadFile() = %v", err)
	}
	if string(got) != "hello world" {
		t.Fatalf("got %q, want %q", got, "hello world")
	}
}

func TestReadFileOnMissingFileFails(t *testing.T) {
	s := NewSched(1)
	fsys := NewFileSystem(s, detsim.FaultProfile{})
	if _, err := fsys.ReadFile("missing.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected fs.ErrNotExist, got %v", err)
	}
}
