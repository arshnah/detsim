package kv

import (
	"testing"

	"github.com/arshnah/detsim"
)

func FuzzRecoverNeverPanics(f *testing.F) {
	fs1 := detsim.NewFaultyStorage(1, detsim.FaultProfile{})
	s1 := NewStore(fs1, true)
	s1.Put("a", "1")
	s1.Put("b", "22")
	s1.Sync()
	seedGood := rawBytesOf(fs1)
	f.Add(seedGood)

	f.Add([]byte{})
	f.Add([]byte{0, 0, 0, 1})
	f.Add(seedGood[:len(seedGood)/2])
	f.Add(append(append([]byte(nil), seedGood...), 0xff, 0xff, 0xff, 0xff))

	f.Fuzz(func(t *testing.T, data []byte) {
		for _, checksums := range []bool{true, false} {
			fs := detsim.NewFaultyStorage(1, detsim.FaultProfile{})
			fs.SeedRaw(data)
			s := NewStore(fs, checksums)
			s.Recover(int64(len(data)))
			_ = s.Len()
		}
	})
}

func rawBytesOf(fs *detsim.FaultyStorage) []byte {
	buf := make([]byte, 1<<16)
	n, _ := fs.ReadAt(buf, 0)
	return buf[:n]
}
