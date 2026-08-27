package detsim

import (
	"errors"
	"io"
	"math/rand"
)

// ErrDiskFull is returned by FaultyStorage.WriteAt when MaxSize would be exceeded.
var ErrDiskFull = errors.New("detsim: disk full")

// FaultProfile controls which faults FaultyStorage injects and how often.
type FaultProfile struct {
	TornWriteRate   float64
	CorruptByteRate float64
	SkipSyncRate    float64
	ReorderRate     float64
	MaxSize         int64
}

// FaultyStorage is a byte-addressable virtual disk that injects torn writes, corruption,
// dropped syncs, and reordered commits.
type FaultyStorage struct {
	rng     *rand.Rand
	profile FaultProfile

	committed []byte
	pending   []pendingWrite
}

type pendingWrite struct {
	offset  int64
	data    []byte
	dropped bool
}

// NewFaultyStorage builds a FaultyStorage seeded for reproducible fault injection.
func NewFaultyStorage(seed int64, profile FaultProfile) *FaultyStorage {
	return &FaultyStorage{
		rng:     rand.New(rand.NewSource(seed)),
		profile: profile,
	}
}

// WriteAt stages a write at offset, subject to TornWriteRate and CorruptByteRate.
func (f *FaultyStorage) WriteAt(data []byte, offset int64) (n int, err error) {
	if offset < 0 {
		return 0, errors.New("detsim: negative offset")
	}
	if f.profile.MaxSize > 0 && offset+int64(len(data)) > f.profile.MaxSize {
		return 0, ErrDiskFull
	}

	buf := make([]byte, len(data))
	copy(buf, data)

	if f.rng.Float64() < f.profile.TornWriteRate && len(buf) > 1 {
		keep := 1 + f.rng.Intn(len(buf)-1)
		buf = buf[:keep]
	}
	if f.rng.Float64() < f.profile.CorruptByteRate && len(buf) > 0 {
		i := f.rng.Intn(len(buf))
		buf[i] ^= 0xFF
	}

	pw := pendingWrite{offset: offset, data: buf}
	if f.rng.Float64() < f.profile.SkipSyncRate {
		pw.dropped = true
	}
	f.pending = append(f.pending, pw)
	return len(data), nil
}

// ReadAt reads from the current materialized view, committed data plus any pending writes.
// It follows the io.ReaderAt contract: io.EOF is returned when fewer than len(p) bytes are
// available, and (0, io.EOF) at or past the end.
func (f *FaultyStorage) ReadAt(p []byte, offset int64) (n int, err error) {
	if offset < 0 {
		return 0, errors.New("detsim: negative offset")
	}
	view := f.materializedView()
	if offset >= int64(len(view)) {
		return 0, io.EOF
	}
	n = copy(p, view[offset:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// Sync commits pending writes, subject to SkipSyncRate and ReorderRate.
func (f *FaultyStorage) Sync() error {
	order := make([]int, len(f.pending))
	for i := range order {
		order[i] = i
	}
	if f.profile.ReorderRate > 0 && f.rng.Float64() < f.profile.ReorderRate {
		f.rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	}
	for _, idx := range order {
		pw := f.pending[idx]
		if pw.dropped {
			continue
		}
		f.writeCommitted(pw.offset, pw.data)
	}
	f.pending = nil
	return nil
}

func (f *FaultyStorage) writeCommitted(offset int64, data []byte) {
	end := offset + int64(len(data))
	if end > int64(len(f.committed)) {
		grown := make([]byte, end)
		copy(grown, f.committed)
		f.committed = grown
	}
	copy(f.committed[offset:end], data)
}

// Size returns the length of the current materialized view.
func (f *FaultyStorage) Size() int64 {
	return f.materializedLen()
}

// Crash discards anything staged but not yet synced.
func (f *FaultyStorage) Crash() {
	f.pending = nil
}

// SeedRaw replaces the committed buffer wholesale and clears anything staged.
func (f *FaultyStorage) SeedRaw(data []byte) {
	f.committed = append([]byte(nil), data...)
	f.pending = nil
}

func (f *FaultyStorage) materializedLen() int64 {
	size := int64(len(f.committed))
	for _, pw := range f.pending {
		if pw.dropped {
			continue
		}
		if end := pw.offset + int64(len(pw.data)); end > size {
			size = end
		}
	}
	return size
}

func (f *FaultyStorage) materializedView() []byte {
	size := f.materializedLen()
	view := make([]byte, size)
	copy(view, f.committed)
	for _, pw := range f.pending {
		if !pw.dropped {
			copy(view[pw.offset:], pw.data)
		}
	}
	return view
}
