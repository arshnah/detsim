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
	mode := flag.String("mode", "raft", "raft or kv")
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
	default:
		fmt.Fprintf(os.Stderr, "unknown mode %q, use raft or kv\n", *mode)
		os.Exit(1)
	}
}

func runRaft(startSeed int64, trials, nodeCount int, dropRate float64) {
	for seed := startSeed; seed < startSeed+int64(trials); seed++ {
		sim := detsim.New(seed)
		net := detsim.NewNetwork(sim)
		net.SetDropRate(dropRate)
		net.SetDelayRange(1_000_000, 10_000_000)

		ids := make([]detsim.NodeID, nodeCount)
		for i := range ids {
			ids[i] = detsim.NodeID(rune('A' + i))
		}
		nodesByID := make(map[detsim.NodeID]*raft.Node, nodeCount)
		for i, id := range ids {
			var peers []detsim.NodeID
			for _, other := range ids {
				if other != id {
					peers = append(peers, other)
				}
			}
			nodesByID[id] = raft.NewNode(id, peers, net, sim, seed+int64(i)+1)
		}
		for _, id := range ids {
			nodesByID[id].Start()
		}

		sim.RunUntil(3_000_000_000)
		net.Partition(ids[:nodeCount/2], ids[nodeCount/2:])
		sim.RunUntil(sim.Now() + 3_000_000_000)
		net.HealAll()
		sim.RunUntil(sim.Now() + 2_000_000_000)

		leadersByTerm := make(map[int][]detsim.NodeID)
		for _, id := range ids {
			state, term := nodesByID[id].State()
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
