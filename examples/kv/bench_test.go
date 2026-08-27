package kv

import (
	"fmt"
	"testing"

	"github.com/arshnah/detsim"
)

func BenchmarkPutSyncRecoverCycle(b *testing.B) {
	profile := detsim.FaultProfile{TornWriteRate: 0.3, CorruptByteRate: 0.2, SkipSyncRate: 0.1}
	for i := 0; i < b.N; i++ {
		fs := detsim.NewFaultyStorage(int64(i)+1, profile)
		s := NewStore(fs, true)
		for j := 0; j < 20; j++ {
			s.Put(fmt.Sprintf("key%d", j), fmt.Sprintf("value-%d-%d", i, j))
		}
		s.Sync()

		recovered := NewStore(fs, true)
		recovered.Recover(1 << 20)
	}
}
