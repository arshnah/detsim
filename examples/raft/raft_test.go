package raft

import (
	"fmt"
	"testing"

	"github.com/arshnah/detsim"
)

type cluster struct {
	sim   *detsim.Sim
	net   *detsim.Network
	nodes map[detsim.NodeID]*Node
	ids   []detsim.NodeID
}

func newCluster(seed int64, n int, dropRate float64) *cluster {
	sim := detsim.New(seed)
	net := detsim.NewNetwork(sim)
	net.SetDropRate(dropRate)
	net.SetDelayRange(1_000_000, 10_000_000)

	ids := make([]detsim.NodeID, n)
	for i := range ids {
		ids[i] = detsim.NodeID(fmt.Sprintf("node%d", i))
	}

	nodes := make(map[detsim.NodeID]*Node, n)
	for i, id := range ids {
		var peers []detsim.NodeID
		for _, other := range ids {
			if other != id {
				peers = append(peers, other)
			}
		}
		nodes[id] = NewNode(id, peers, net, sim, seed+int64(i)+1)
	}
	return &cluster{sim: sim, net: net, nodes: nodes, ids: ids}
}

func (c *cluster) start() {
	for _, id := range c.ids {
		c.nodes[id].Start()
	}
}

func (c *cluster) leader() *Node {
	return c.leaderExcluding("")
}

func (c *cluster) leaderExcluding(skip detsim.NodeID) *Node {
	for _, id := range c.ids {
		if id == skip {
			continue
		}
		if state, _ := c.nodes[id].State(); state == Leader {
			return c.nodes[id]
		}
	}
	return nil
}

func (c *cluster) leadersByTerm() map[int][]detsim.NodeID {
	out := make(map[int][]detsim.NodeID)
	for _, id := range c.ids {
		n := c.nodes[id]
		state, term := n.State()
		if state == Leader {
			out[term] = append(out[term], id)
		}
	}
	return out
}

func assertNoSplitBrain(t *testing.T, seed int64, c *cluster) {
	t.Helper()
	for term, leaders := range c.leadersByTerm() {
		if len(leaders) > 1 {
			t.Fatalf("SPLIT BRAIN at seed=%d: term %d has leaders %v, reproduce with this exact seed", seed, term, leaders)
		}
	}
}

func TestClusterLargerThan26NodesWorks(t *testing.T) {
	c := newCluster(1, 30, 0.0)
	c.start()
	c.sim.RunUntil(3_000_000_000)

	l := c.leader()
	if l == nil {
		t.Fatal("no leader elected in a 30-node cluster")
	}
	idx, ok := l.Submit("hello-from-a-big-cluster")
	if !ok {
		t.Fatal("leader rejected submit")
	}
	c.sim.RunFor(1_000_000_000)
	if l.lastApplied < idx {
		t.Fatalf("expected the entry to commit even with 30 nodes, lastApplied=%d want >= %d", l.lastApplied, idx)
	}
}

func TestSingleRunElectsLeaderAndReplicates(t *testing.T) {
	c := newCluster(1, 5, 0.0)
	c.start()
	c.sim.RunUntil(2_000_000_000)

	var leader *Node
	for _, n := range c.nodes {
		if state, _ := n.State(); state == Leader {
			leader = n
		}
	}
	if leader == nil {
		t.Fatal("no leader elected in 2s virtual time with a perfect network")
	}

	idx, ok := leader.Submit("hello")
	if !ok {
		t.Fatal("leader rejected submit")
	}
	c.sim.RunFor(1_000_000_000)

	for id, n := range c.nodes {
		found := false
		for _, e := range n.Committed {
			if e.Index == idx && e.Command == "hello" {
				found = true
			}
		}
		if !found {
			t.Fatalf("node %s never committed entry %d", id, idx)
		}
	}
}

func TestThousandsOfSeedsNoSplitBrain(t *testing.T) {
	const trials = 5000
	for seed := int64(1000); seed < 1000+trials; seed++ {
		c := newCluster(seed, 5, 0.1)
		c.start()
		c.sim.RunUntil(3_000_000_000)
		assertNoSplitBrain(t, seed, c)

		ids := c.ids
		c.net.Partition(ids[:2], ids[2:])
		c.sim.RunFor(3_000_000_000)
		assertNoSplitBrain(t, seed, c)

		c.net.HealAll()
		c.sim.RunFor(2_000_000_000)
		assertNoSplitBrain(t, seed, c)
	}
}

func TestSeedIsExactlyReproducible(t *testing.T) {
	trace := func(seed int64) string {
		c := newCluster(seed, 5, 0.2)
		c.start()
		c.sim.RunUntil(2_000_000_000)
		return fmt.Sprintf("%v", c.leadersByTerm())
	}
	a := trace(4242)
	b := trace(4242)
	if a != b {
		t.Fatalf("same seed produced different outcomes:\nrun1: %s\nrun2: %s", a, b)
	}
}
