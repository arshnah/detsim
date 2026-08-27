package raft

import (
	"fmt"
	"testing"
)

func BenchmarkClusterElectionAndPartitionCycle(b *testing.B) {
	for i := 0; i < b.N; i++ {
		c := newCluster(int64(i)+1, 5, 0.1)
		c.start()
		c.sim.RunUntil(3_000_000_000)
		ids := c.ids
		c.net.Partition(ids[:2], ids[2:])
		c.sim.RunFor(3_000_000_000)
		c.net.HealAll()
		c.sim.RunFor(2_000_000_000)
	}
}

func BenchmarkSubmitAndCommit100Entries(b *testing.B) {
	for i := 0; i < b.N; i++ {
		c := newCluster(int64(i)+1, 5, 0.0)
		c.start()
		c.sim.RunUntil(2_000_000_000)
		l := c.leader()
		if l == nil {
			b.Fatal("no leader elected")
		}
		for j := 0; j < 100; j++ {
			l.Submit(fmt.Sprintf("bench-%d", j))
		}
		c.sim.RunFor(2_000_000_000)
	}
}

func BenchmarkSnapshotCompactionCycle(b *testing.B) {
	for i := 0; i < b.N; i++ {
		c := newCluster(int64(i)+1, 5, 0.0)
		c.start()
		c.sim.RunUntil(2_000_000_000)
		l := c.leader()
		if l == nil {
			b.Fatal("no leader elected")
		}
		for j := 0; j < snapshotThreshold+20; j++ {
			l.Submit(fmt.Sprintf("bench-%d", j))
			c.sim.RunFor(20_000_000)
		}
	}
}
