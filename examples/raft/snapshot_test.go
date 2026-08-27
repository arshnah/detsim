package raft

import (
	"fmt"
	"testing"
)

func TestLaggingFollowerCatchesUpViaSnapshot(t *testing.T) {
	c := newCluster(5000, 5, 0.0)
	c.start()
	c.sim.RunUntil(2_000_000_000)

	lagging := c.ids[len(c.ids)-1]

	l := c.leaderExcluding(lagging)
	if l == nil {
		t.Fatal("no leader elected before partition")
	}

	c.net.Partition(c.ids[len(c.ids)-1:], c.ids[:len(c.ids)-1])

	for i := 0; i < snapshotThreshold+20; i++ {
		l = c.leaderExcluding(lagging)
		if l == nil {
			t.Fatalf("lost leader mid-run at submit %d", i)
		}
		if _, ok := l.Submit(fmt.Sprintf("cmd-%d", i)); !ok {
			t.Fatalf("submit %d rejected, node claiming leader isn't accepting writes", i)
		}
		c.sim.RunFor(20_000_000)
	}

	if l.baseIndex() == 0 {
		t.Fatalf("expected the leader to have compacted its log past index 0 after %d entries, baseIndex=%d", snapshotThreshold+20, l.baseIndex())
	}

	c.net.HealAll()
	c.sim.RunFor(3_000_000_000)

	l = c.leaderExcluding(lagging)
	if l == nil {
		t.Fatal("no leader after heal")
	}
	if len(l.Committed) == 0 {
		t.Fatal("leader has no committed entries to compare against")
	}

	laggingNode := c.nodes[lagging]
	if len(laggingNode.Committed) == 0 {
		t.Fatal("lagging node never caught up any committed entries after heal, InstallSnapshot catch-up may have failed")
	}
	last := laggingNode.Committed[len(laggingNode.Committed)-1]
	found := false
	for _, e := range l.Committed {
		if e.Index == last.Index && e.Command == last.Command {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("lagging node's last committed entry (index=%d cmd=%s) doesn't match anything in the leader's committed log", last.Index, last.Command)
	}
}

func TestManySeedsSnapshotCatchUpNoSplitBrain(t *testing.T) {
	const trials = 300
	for seed := int64(2000); seed < 2000+trials; seed++ {
		c := newCluster(seed, 5, 0.05)
		c.start()
		c.sim.RunUntil(1_500_000_000)
		assertNoSplitBrain(t, seed, c)

		lagging := c.ids[len(c.ids)-1]
		c.net.Partition(c.ids[len(c.ids)-1:], c.ids[:len(c.ids)-1])

		for i := 0; i < snapshotThreshold+10; i++ {
			l := c.leaderExcluding(lagging)
			if l != nil {
				l.Submit(fmt.Sprintf("cmd-%d", i))
			}
			c.sim.RunFor(15_000_000)
		}
		assertNoSplitBrain(t, seed, c)

		c.net.HealAll()
		c.sim.RunFor(3_000_000_000)
		assertNoSplitBrain(t, seed, c)

		l := c.leaderExcluding(lagging)
		if l == nil {
			continue
		}
		laggingNode := c.nodes[lagging]
		if len(laggingNode.Committed) == 0 || len(l.Committed) == 0 {
			continue
		}
		last := laggingNode.Committed[len(laggingNode.Committed)-1]
		for _, e := range l.Committed {
			if e.Index == last.Index && e.Command != last.Command {
				t.Fatalf("seed=%d: divergent committed entry at index %d: lagging node has %q, leader has %q", seed, e.Index, last.Command, e.Command)
			}
		}
	}
}
