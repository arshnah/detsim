package raft

import (
	"fmt"
	"testing"

	"github.com/arshnah/detsim"
)

func TestAddNodeJoinsAndCatchesUpViaReplication(t *testing.T) {
	c := newCluster(6000, 3, 0.0)
	c.start()
	c.sim.RunUntil(2_000_000_000)

	l := c.leader()
	if l == nil {
		t.Fatal("no leader elected")
	}
	for i := 0; i < 5; i++ {
		l.Submit(fmt.Sprintf("pre-join-%d", i))
	}
	c.sim.RunFor(1_000_000_000)

	newID := detsim.NodeID("D")
	newPeers := append([]detsim.NodeID{}, c.ids...)
	newNode := NewNode(newID, newPeers, c.net, c.sim, 999)
	c.net.Register(newID, newNode.handle)
	newNode.Start()
	c.nodes[newID] = newNode
	c.ids = append(c.ids, newID)

	idx, ok := l.ProposeAddNode(newID)
	if !ok {
		t.Fatal("leader rejected ProposeAddNode")
	}
	c.sim.RunFor(2_000_000_000)

	if l.lastApplied < idx {
		t.Fatalf("expected the config-change entry itself (index %d) to commit, leader lastApplied=%d", idx, l.lastApplied)
	}

	found := false
	for _, p := range newNode.peers {
		if p != newID {
			found = true
		}
	}
	if !found || len(newNode.peers) == 0 {
		t.Fatal("expected the new node to have learned about its peers by replicating the config-change entry")
	}

	if len(newNode.Committed) == 0 {
		t.Fatal("expected the new node to have caught up and applied the pre-join entries via normal replication")
	}
}

func TestRemoveNodeStopsParticipatingInQuorum(t *testing.T) {
	c := newCluster(6001, 5, 0.0)
	c.start()
	c.sim.RunUntil(2_000_000_000)

	l := c.leader()
	if l == nil {
		t.Fatal("no leader elected")
	}

	toRemove := c.ids[len(c.ids)-1]
	if toRemove == l.id {
		toRemove = c.ids[len(c.ids)-2]
	}

	idx, ok := l.ProposeRemoveNode(toRemove)
	if !ok {
		t.Fatal("leader rejected ProposeRemoveNode")
	}
	c.sim.RunFor(1_000_000_000)

	if l.lastApplied < idx {
		t.Fatalf("expected the removal config-change entry (index %d) to commit, leader lastApplied=%d", idx, l.lastApplied)
	}
	for _, p := range l.peers {
		if p == toRemove {
			t.Fatalf("expected %s to no longer be in the leader's peer list after removal", toRemove)
		}
	}
	if _, exists := l.nextIndex[toRemove]; exists {
		t.Fatalf("expected %s to be cleaned out of nextIndex after removal", toRemove)
	}
}

func TestManySeedsMembershipChangeNoSplitBrain(t *testing.T) {
	const trials = 300
	removedNodeStillCampaigned := 0
	for seed := int64(7000); seed < 7000+trials; seed++ {
		c := newCluster(seed, 5, 0.05)
		c.start()
		c.sim.RunUntil(1_500_000_000)
		assertNoSplitBrain(t, seed, c)

		l := c.leader()
		if l == nil {
			continue
		}
		toRemove := c.ids[len(c.ids)-1]
		if toRemove == l.id {
			toRemove = c.ids[len(c.ids)-2]
		}
		l.ProposeRemoveNode(toRemove)
		c.sim.RunFor(1_500_000_000)
		assertNoSplitBrain(t, seed, c)

		removedNode := c.nodes[toRemove]
		if state, _ := removedNode.State(); state == Candidate || state == Leader {
			removedNodeStillCampaigned++
		}

		l2 := c.leader()
		if l2 != nil {
			for i := 0; i < 5; i++ {
				l2.Submit(fmt.Sprintf("post-removal-%d", i))
			}
		}
		c.sim.RunFor(1_500_000_000)
		assertNoSplitBrain(t, seed, c)
	}

	if removedNodeStillCampaigned == 0 {
		t.Fatal("across all trials, the removed node never once still campaigned after removal (it isn't told it's been removed, so it should keep timing out and calling elections), this test isn't actually exercising the interesting case it's meant to")
	}
	t.Logf("%d/%d trials had the removed node still campaigning for votes after removal, and no split brain occurred in any of them", removedNodeStillCampaigned, trials)
}
