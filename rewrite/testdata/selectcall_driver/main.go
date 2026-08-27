package main

import (
	"flag"
	"fmt"

	"github.com/arshnah/detsim/rewrite/testdata/selectcall"
	"github.com/arshnah/detsim/rt"
)

func main() {
	seed := flag.Int64("seed", 1, "scheduler seed")
	flag.Parse()

	sched := rt.NewSched(*seed)
	selectcall.DetsimSetSched(sched)

	var got, sent int
	sched.Go(func() {
		got = selectcall.RecvFirst()
	})
	if err := sched.Run(); err != nil {
		panic(err)
	}

	sched2 := rt.NewSched(*seed)
	selectcall.DetsimSetSched(sched2)
	sched2.Go(func() {
		if selectcall.SendFirst() {
			sent = 1
		}
	})
	if err := sched2.Run(); err != nil {
		panic(err)
	}

	fmt.Printf("got=%d sent=%v evals=%d\n", got, sent == 1, selectcall.Evals())
}
