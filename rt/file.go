package rt

import (
	"io"
	iofs "io/fs"
	"time"

	"github.com/arshnah/detsim"
)

// FileSystem is a deterministic stand-in for os.Open/os.Create, backed by FaultyStorage.
type FileSystem struct {
	sched   *Sched
	profile detsim.FaultProfile
	files   map[string]*detsim.FaultyStorage
}

// NewFileSystem builds a FileSystem where every created file shares profile.
func NewFileSystem(s *Sched, profile detsim.FaultProfile) *FileSystem {
	return &FileSystem{
		sched:   s,
		profile: profile,
		files:   make(map[string]*detsim.FaultyStorage),
	}
}

// Open opens an existing file, failing with fs.ErrNotExist if it was never created.
func (fs *FileSystem) Open(name string) (*File, error) {
	storage, ok := fs.files[name]
	if !ok {
		return nil, &iofs.PathError{Op: "open", Path: name, Err: iofs.ErrNotExist}
	}
	return &File{name: name, storage: storage}, nil
}

// Create creates or truncates a file backed by a fresh FaultyStorage.
func (fs *FileSystem) Create(name string) (*File, error) {
	storage := detsim.NewFaultyStorage(fs.sched.Rand.Int63(), fs.profile)
	fs.files[name] = storage
	return &File{name: name, storage: storage}, nil
}

// Remove deletes a file, failing with fs.ErrNotExist if it doesn't exist.
func (fs *FileSystem) Remove(name string) error {
	if _, ok := fs.files[name]; !ok {
		return &iofs.PathError{Op: "remove", Path: name, Err: iofs.ErrNotExist}
	}
	delete(fs.files, name)
	return nil
}

// Rename moves a file, failing with fs.ErrNotExist if the source doesn't exist.
func (fs *FileSystem) Rename(oldName, newName string) error {
	storage, ok := fs.files[oldName]
	if !ok {
		return &iofs.PathError{Op: "rename", Path: oldName, Err: iofs.ErrNotExist}
	}
	fs.files[newName] = storage
	delete(fs.files, oldName)
	return nil
}

// Stat returns size and name info for an existing file.
func (fs *FileSystem) Stat(name string) (FileInfo, error) {
	storage, ok := fs.files[name]
	if !ok {
		return FileInfo{}, &iofs.PathError{Op: "stat", Path: name, Err: iofs.ErrNotExist}
	}
	return FileInfo{name: name, size: storage.Size()}, nil
}

// ReadFile is a whole-file convenience helper, equivalent to os.ReadFile.
func (fs *FileSystem) ReadFile(name string) ([]byte, error) {
	f, err := fs.Open(name)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(f)
}

// WriteFile is a whole-file convenience helper, equivalent to os.WriteFile, and syncs.
func (fs *FileSystem) WriteFile(name string, data []byte) error {
	f, err := fs.Create(name)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	return f.Sync()
}

// FileInfo is a minimal io/fs.FileInfo-shaped value.
type FileInfo struct {
	name string
	size int64
}

// Name returns the file's name.
func (fi FileInfo) Name() string { return fi.name }

// Size returns the file's size in bytes.
func (fi FileInfo) Size() int64 { return fi.size }

// Mode returns a fixed stand-in mode, 0644.
func (fi FileInfo) Mode() iofs.FileMode { return 0o644 }

// ModTime returns a fixed stand-in zero time.
func (fi FileInfo) ModTime() time.Time { return time.Time{} }

// IsDir always returns false.
func (fi FileInfo) IsDir() bool { return false }

// Sys always returns nil.
func (fi FileInfo) Sys() any { return nil }

// File implements Read, Write, Sync, Close against a backing FaultyStorage.
type File struct {
	name    string
	storage *detsim.FaultyStorage
	offset  int64
}

// Name returns the file's name, matching os.File.Name.
func (f *File) Name() string { return f.name }

// Read returns io.EOF at end of file, matching os.File: a short read due to EOF reports
// nil and the following read returns (0, io.EOF).
func (f *File) Read(p []byte) (int, error) {
	n, err := f.storage.ReadAt(p, f.offset)
	f.offset += int64(n)
	if err == io.EOF && n > 0 {
		err = nil
	}
	return n, err
}

// Write writes at the file's current offset.
func (f *File) Write(p []byte) (int, error) {
	n, err := f.storage.WriteAt(p, f.offset)
	f.offset += int64(n)
	return n, err
}

// Seek repositions the file offset, supporting io.SeekStart, io.SeekCurrent, and
// io.SeekEnd relative to the materialized size.
func (f *File) Seek(offset int64, whence int) (int64, error) {
	var next int64
	switch whence {
	case io.SeekStart:
		next = offset
	case io.SeekCurrent:
		next = f.offset + offset
	case io.SeekEnd:
		next = f.storage.Size() + offset
	default:
		return 0, &iofs.PathError{Op: "seek", Path: f.name, Err: iofs.ErrInvalid}
	}
	if next < 0 {
		return 0, &iofs.PathError{Op: "seek", Path: f.name, Err: iofs.ErrInvalid}
	}
	f.offset = next
	return next, nil
}

// ReadAt reads at an absolute offset without touching the file offset.
func (f *File) ReadAt(p []byte, off int64) (int, error) {
	return f.storage.ReadAt(p, off)
}

// WriteAt writes at an absolute offset without touching the file offset.
func (f *File) WriteAt(p []byte, off int64) (int, error) {
	return f.storage.WriteAt(p, off)
}

// Sync commits pending writes to the backing storage.
func (f *File) Sync() error {
	return f.storage.Sync()
}

// Close is a no-op, the backing storage isn't tied to a real OS handle.
func (f *File) Close() error {
	return nil
}
