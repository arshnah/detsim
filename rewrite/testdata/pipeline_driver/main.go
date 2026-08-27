package main

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/arshnah/detsim/rewrite/testdata/pipeline"
	"github.com/arshnah/detsim/rt"
)

func main() {
	seed := flag.Int64("seed", 1, "scheduler seed")
	flag.Parse()

	sched := rt.NewSched(*seed)
	pipeline.DetsimSetSched(sched)

	ids := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	var out []pipeline.Result
	sched.Go(func() {
		out = pipeline.Run(ids)
	})

	if err := sched.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	for _, r := range out {
		fmt.Printf("%d %d\n", r.ID, r.Sum)
	}
}
