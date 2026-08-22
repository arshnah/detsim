package kv

import (
	"fmt"
	"testing"

	"github.com/arshnah/detsim"
)

func TestBasicPutSyncRecover(t *testing.T) {
	fs := detsim.NewFaultyStorage(1, detsim.FaultProfile{})
	s := NewStore(fs, true)
	s.Put("a", "1")
	s.Put("b", "2")
	s.Sync()

	recovered := NewStore(fs, true)
	recovered.Recover(1 << 20)
	if v, ok := recovered.Get("a"); !ok || v != "1" {
		t.Fatalf("expected a=1, got %q ok=%v", v, ok)
	}
	if v, ok := recovered.Get("b"); !ok || v != "2" {
		t.Fatalf("expected b=2, got %q ok=%v", v, ok)
	}
}

func TestUnsyncedWritesLostButNotCorrupt(t *testing.T) {
	fs := detsim.NewFaultyStorage(1, detsim.FaultProfile{})
	s := NewStore(fs, true)
	s.Put("a", "1")
	s.Sync()
	s.Put("b", "2")
	fs.Crash()

	recovered := NewStore(fs, true)
	recovered.Recover(1 << 20)
	if v, ok := recovered.Get("a"); !ok || v != "1" {
		t.Fatalf("expected synced a=1 to survive, got %q ok=%v", v, ok)
	}
	if _, ok := recovered.Get("b"); ok {
		t.Fatal("expected unsynced b to be lost, but it was recovered")
	}
}

func TestChecksummedStoreNeverServesCorruptData(t *testing.T) {
	const trials = 2000
	for seed := int64(1); seed <= trials; seed++ {
		profile := detsim.FaultProfile{TornWriteRate: 0.3, CorruptByteRate: 0.2, SkipSyncRate: 0.1}
		fs := detsim.NewFaultyStorage(seed, profile)
		s := NewStore(fs, true)

		for i := 0; i < 20; i++ {
			s.Put(fmt.Sprintf("key%d", i), fmt.Sprintf("value-%d-%d", seed, i))
		}
		s.Sync()

		recovered := NewStore(fs, true)
		recovered.Recover(1 << 20)

		for i := 0; i < 20; i++ {
			key := fmt.Sprintf("key%d", i)
			want := fmt.Sprintf("value-%d-%d", seed, i)
			if got, ok := recovered.Get(key); ok && got != want {
				t.Fatalf("seed=%d: checksummed store served CORRUPTED data for %s: got %q want %q (or absent), reproduce with this seed", seed, key, got, want)
			}
		}
	}
}

func TestNoChecksumStoreCanServeCorruptData(t *testing.T) {
	const trials = 5000
	sawCorruption := false
	var badSeed int64

	for seed := int64(1); seed <= trials; seed++ {
		profile := detsim.FaultProfile{TornWriteRate: 0.3, CorruptByteRate: 0.4}
		fs := detsim.NewFaultyStorage(seed, profile)
		s := NewStore(fs, false)

		for i := 0; i < 10; i++ {
			s.Put(fmt.Sprintf("key%d", i), fmt.Sprintf("value-%d-%d", seed, i))
		}
		s.Sync()

		recovered := NewStore(fs, false)
		recovered.Recover(1 << 20)

		for i := 0; i < 10; i++ {
			key := fmt.Sprintf("key%d", i)
			want := fmt.Sprintf("value-%d-%d", seed, i)
			if got, ok := recovered.Get(key); ok && got != want {
				sawCorruption = true
				badSeed = seed
				break
			}
		}
		if sawCorruption {
			break
		}
	}

	if !sawCorruption {
		t.Fatal("expected the no-checksum store to eventually serve corrupted data under this fault profile, but it never did across 5000 seeds")
	}
	t.Logf("confirmed: no-checksum store served corrupted data at seed=%d, checksummed store never does under the same fault profile", badSeed)
}
