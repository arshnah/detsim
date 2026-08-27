package raft

import (
	"fmt"
	"testing"

	"github.com/arshnah/detsim"
)

func TestReadResolvesTrueOnStableLeader(t *testing.T) {
	c := newCluster(9000, 5, 0.0)
	c.start()
	c.sim.RunUntil(2_000_000_000)

	l := c.leader()
	if l == nil {
		t.Fatal("no leader elected")
	}

	idx, ok := l.Submit("hello")
	if !ok {
		t.Fatal("leader rejected submit")
	}
	c.sim.RunFor(500_000_000)

	var result *bool
	l.Read(func(ok bool) { result = &ok })
	c.sim.RunFor(500_000_000)

	if result == nil {
		t.Fatal("read never resolved")
	}
	if !*result {
		t.Fatal("expected read to resolve true on a stable, connected leader")
	}
	if l.lastApplied < idx {
		t.Fatalf("expected lastApplied (%d) to have caught up to the submitted entry (%d) by the time the read resolved", l.lastApplied, idx)
	}
}

func TestReadFailsImmediatelyOnFollower(t *testing.T) {
	c := newCluster(9001, 5, 0.0)
	c.start()
	c.sim.RunUntil(2_000_000_000)

	l := c.leader()
	if l == nil {
		t.Fatal("no leader elected")
	}
	var follower *Node
	for _, id := range c.ids {
		if c.nodes[id] != l {
			follower = c.nodes[id]
			break
		}
	}

	var result *bool
	follower.Read(func(ok bool) { result = &ok })

	if result == nil || *result {
		t.Fatal("expected Read on a non-leader to fail immediately, not just eventually")
	}
}

func TestIsolatedStaleLeaderNeverServesAReadAfterNewLeaderCommits(t *testing.T) {
	const trials = 300
	stillBelievedLeader := 0
	for seed := int64(3000); seed < 3000+trials; seed++ {
		c := newCluster(seed, 5, 0.0)
		c.start()
		c.sim.RunUntil(2_000_000_000)

		staleLeader := c.leader()
		if staleLeader == nil {
			continue
		}
		staleLeaderID := staleLeader.id

		var isolatedGroup, restGroup []detsim.NodeID
		for _, id := range c.ids {
			if id == staleLeaderID {
				isolatedGroup = append(isolatedGroup, id)
			} else {
				restGroup = append(restGroup, id)
			}
		}
		c.net.Partition(isolatedGroup, restGroup)

		for i := 0; i < 5; i++ {
			l := c.leaderExcluding(staleLeaderID)
			if l != nil {
				l.Submit(fmt.Sprintf("post-partition-%d", i))
			}
			c.sim.RunFor(300_000_000)
		}

		if staleLeader.state == Leader {
			stillBelievedLeader++
		}

		var readResult *bool
		staleLeader.Read(func(ok bool) { readResult = &ok })
		c.sim.RunFor(2_000_000_000)

		if readResult != nil && *readResult {
			t.Fatalf("seed=%d: SPLIT READ, an isolated stale leader served a read as successful after the majority elected a new leader and committed writes it never saw, reproduce with this exact seed", seed)
		}

		c.net.HealAll()
	}

	if stillBelievedLeader == 0 {
		t.Fatal("across all trials, the isolated node never once still self-reported as Leader at read time, this test isn't actually exercising the interesting case it's meant to")
	}
	t.Logf("%d/%d trials had the isolated node still self-reporting Leader when Read was called, that's the interesting case this test targets", stillBelievedLeader, trials)
}
