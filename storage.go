package detsim

import "math/rand"

type FaultProfile struct {
	TornWriteRate   float64
	CorruptByteRate float64
	SkipSyncRate    float64
	ReorderRate     float64
}

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

func NewFaultyStorage(seed int64, profile FaultProfile) *FaultyStorage {
	return &FaultyStorage{
		rng:     rand.New(rand.NewSource(seed)),
		profile: profile,
	}
}

func (f *FaultyStorage) WriteAt(data []byte, offset int64) (n int, err error) {
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

func (f *FaultyStorage) ReadAt(p []byte, offset int64) (n int, err error) {
	view := f.materializedView()
	if offset >= int64(len(view)) {
		return 0, nil
	}
	n = copy(p, view[offset:])
	return n, nil
}

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

func (f *FaultyStorage) Crash() {
	f.pending = nil
}

func (f *FaultyStorage) materializedView() []byte {
	size := int64(len(f.committed))
	for _, pw := range f.pending {
		if pw.dropped {
			continue
		}
		if end := pw.offset + int64(len(pw.data)); end > size {
			size = end
		}
	}
	view := make([]byte, size)
	copy(view, f.committed)
	for _, pw := range f.pending {
		if !pw.dropped {
			copy(view[pw.offset:], pw.data)
		}
	}
	return view
}
