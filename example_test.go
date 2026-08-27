package detsim_test

import (
	"fmt"

	detsim "github.com/arshnah/detsim"
)

func Example() {
	sim := detsim.New(1)
	net := detsim.NewNetwork(sim)

	net.Register("a", func(from detsim.NodeID, msg any) {
		fmt.Println("a received:", msg, "from", from)
	})
	net.Register("b", func(from detsim.NodeID, msg any) {
		fmt.Println("b received:", msg, "from", from)
		net.Send("b", "a", "pong")
	})

	net.Send("a", "b", "ping")
	sim.RunFor(100)
}

func ExampleFaultyStorage() {
	profile := detsim.FaultProfile{TornWriteRate: 0.3, CorruptByteRate: 0.4}
	for seed := int64(1); seed <= 5; seed++ {
		fs := detsim.NewFaultyStorage(seed, profile)
		fs.WriteAt([]byte("hello world"), 0)
		fs.Sync()

		got := make([]byte, 11)
		fs.ReadAt(got, 0)
		if string(got) == "hello world" {
			fmt.Printf("seed %d: clean\n", seed)
		} else {
			fmt.Printf("seed %d: corrupted\n", seed)
		}
	}
}
