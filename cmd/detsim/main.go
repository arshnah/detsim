package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/arshnah/detsim"
	"github.com/arshnah/detsim/examples/kv"
	"github.com/arshnah/detsim/examples/raft"
)

func main() {
	mode := flag.String("mode", "raft", "raft, kv, read, or membership")
	seed := flag.Int64("seed", 1, "starting simulation seed")
	trials := flag.Int("trials", 1, "number of seeds to try, starting at -seed")
	nodes := flag.Int("nodes", 5, "number of raft nodes (raft mode)")
	drop := flag.Float64("drop", 0.1, "network drop rate (raft mode)")
	tornRate := flag.Float64("torn", 0.3, "torn write rate (kv mode)")
	corruptRate := flag.Float64("corrupt", 0.4, "byte corruption rate (kv mode)")
	checksums := flag.Bool("checksums", false, "enable per-entry checksums (kv mode)")
	flag.Parse()

	switch *mode {
	case "raft":
		runRaft(*seed, *trials, *nodes, *drop)
	case "kv":
		runKV(*seed, *trials, *tornRate, *corruptRate, *checksums)
	case "read":
		runRead(*seed, *trials, *nodes)
	case "membership":
		runMembership(*seed, *trials, *nodes)
	default:
		fmt.Fprintf(os.Stderr, "unknown mode %q, use raft, kv, read, or membership\n", *mode)
		os.Exit(1)
	}
}

type raftCluster struct {
	sim   *detsim.Sim
	net   *detsim.Network
	ids   []detsim.NodeID
	nodes map[detsim.NodeID]*raft.Node
}

func newRaftCluster(seed int64, nodeCount int, dropRate float64) raftCluster {
	sim := detsim.New(seed)
	net := detsim.NewNetwork(sim)
	net.SetDropRate(dropRate)
	net.SetDelayRange(1_000_000, 10_000_000)

	ids := make([]detsim.NodeID, nodeCount)
	for i := range ids {
		ids[i] = detsim.NodeID(fmt.Sprintf("node%d", i))
	}
	nodes := make(map[detsim.NodeID]*raft.Node, nodeCount)
	for i, id := range ids {
		var peers []detsim.NodeID
		for _, other := range ids {
			if other != id {
				peers = append(peers, other)
			}
		}
		nodes[id] = raft.NewNode(id, peers, net, sim, seed+int64(i)+1)
	}
	for _, id := range ids {
		nodes[id].Start()
	}
	return raftCluster{sim: sim, net: net, ids: ids, nodes: nodes}
}

func runRaft(startSeed int64, trials, nodeCount int, dropRate float64) {
	for seed := startSeed; seed < startSeed+int64(trials); seed++ {
		c := newRaftCluster(seed, nodeCount, dropRate)

		sim, net, ids := c.sim, c.net, c.ids
		sim.RunUntil(3_000_000_000)
		net.Partition(ids[:nodeCount/2], ids[nodeCount/2:])
		sim.RunFor(3_000_000_000)
		net.HealAll()
		sim.RunFor(2_000_000_000)

		leadersByTerm := make(map[int][]detsim.NodeID)
		for _, id := range ids {
			state, term := c.nodes[id].State()
			if state == raft.Leader {
				leadersByTerm[term] = append(leadersByTerm[term], id)
			}
		}

		splitBrain := false
		for term, leaders := range leadersByTerm {
			if len(leaders) > 1 {
				splitBrain = true
				fmt.Printf("seed=%d SPLIT BRAIN: term %d has leaders %v\n", seed, term, leaders)
			}
		}
		if !splitBrain {
			fmt.Printf("seed=%d ok, no split brain, %d nodes, drop rate %.2f, final state: %v\n", seed, nodeCount, dropRate, leadersByTerm)
		}
	}
}

func runMembership(startSeed int64, trials, nodeCount int) {
	for seed := startSeed; seed < startSeed+int64(trials); seed++ {
		c := newRaftCluster(seed, nodeCount, 0.05)

		sim, ids := c.sim, c.ids
		sim.RunUntil(1_500_000_000)

		var leader *raft.Node
		for _, id := range ids {
			if state, _ := c.nodes[id].State(); state == raft.Leader {
				leader = c.nodes[id]
				break
			}
		}
		if leader == nil {
			fmt.Printf("seed=%d skipped, no leader elected\n", seed)
			continue
		}

		toRemove := ids[len(ids)-1]
		if toRemove == leader.ID() {
			toRemove = ids[len(ids)-2]
		}
		leader.ProposeRemoveNode(toRemove)
		sim.RunFor(1_500_000_000)

		splitBrain := false
		leadersByTerm := make(map[int][]detsim.NodeID)
		for _, id := range ids {
			state, term := c.nodes[id].State()
			if state == raft.Leader {
				leadersByTerm[term] = append(leadersByTerm[term], id)
			}
		}
		for term, leaders := range leadersByTerm {
			if len(leaders) > 1 {
				splitBrain = true
				fmt.Printf("seed=%d SPLIT BRAIN after removing %s: term %d has leaders %v\n", seed, toRemove, term, leaders)
			}
		}
		if !splitBrain {
			fmt.Printf("seed=%d ok, removed %s, no split brain, cluster still healthy\n", seed, toRemove)
		}
	}
}

func runRead(startSeed int64, trials, nodeCount int) {
	for seed := startSeed; seed < startSeed+int64(trials); seed++ {
		c := newRaftCluster(seed, nodeCount, 0)

		sim, ids := c.sim, c.ids
		sim.RunUntil(2_000_000_000)

		var staleLeader *raft.Node
		var staleLeaderID detsim.NodeID
		for _, id := range ids {
			if state, _ := c.nodes[id].State(); state == raft.Leader {
				staleLeader = c.nodes[id]
				staleLeaderID = id
				break
			}
		}
		if staleLeader == nil {
			fmt.Printf("seed=%d skipped, no leader elected before partition\n", seed)
			continue
		}

		var isolated, rest []detsim.NodeID
		for _, id := range ids {
			if id == staleLeaderID {
				isolated = append(isolated, id)
			} else {
				rest = append(rest, id)
			}
		}
		net := c.net
		net.Partition(isolated, rest)

		for i := 0; i < 5; i++ {
			for _, id := range rest {
				if state, _ := c.nodes[id].State(); state == raft.Leader {
					c.nodes[id].Submit(fmt.Sprintf("post-partition-%d", i))
					break
				}
			}
			sim.RunFor(300_000_000)
		}

		var result *bool
		staleLeader.Read(func(ok bool) { result = &ok })
		sim.RunFor(2_000_000_000)

		switch {
		case result == nil:
			fmt.Printf("seed=%d ok, isolated leader's read never resolved (correct, it can't reach quorum)\n", seed)
		case *result:
			fmt.Printf("seed=%d SPLIT READ: isolated leader %s served a stale read as successful\n", seed, staleLeaderID)
		default:
			fmt.Printf("seed=%d ok, isolated leader %s correctly refused to serve a stale read\n", seed, staleLeaderID)
		}
	}
}

func runKV(startSeed int64, trials int, tornRate, corruptRate float64, checksums bool) {
	profile := detsim.FaultProfile{TornWriteRate: tornRate, CorruptByteRate: corruptRate}
	for seed := startSeed; seed < startSeed+int64(trials); seed++ {
		fs := detsim.NewFaultyStorage(seed, profile)
		s := kv.NewStore(fs, checksums)

		for i := 0; i < 10; i++ {
			s.Put(fmt.Sprintf("key%d", i), fmt.Sprintf("value-%d-%d", seed, i))
		}
		s.Sync()

		recovered := kv.NewStore(fs, checksums)
		recovered.Recover(1 << 20)

		corrupted := false
		for i := 0; i < 10; i++ {
			key := fmt.Sprintf("key%d", i)
			want := fmt.Sprintf("value-%d-%d", seed, i)
			if got, ok := recovered.Get(key); ok && got != want {
				corrupted = true
				fmt.Printf("seed=%d CORRUPTED: %s got %q want %q\n", seed, key, got, want)
			}
		}
		if !corrupted {
			fmt.Printf("seed=%d ok, checksums=%v, %d keys recovered clean\n", seed, checksums, recovered.Len())
		}
	}
}
