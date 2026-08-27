package main

import (
	"flag"
	"fmt"
	"sort"

	"github.com/arshnah/detsim/rewrite/testdata/aliasedstdlib"
	"github.com/arshnah/detsim/rt"
)

func main() {
	seed := flag.Int64("seed", 1, "scheduler seed")
	flag.Parse()

	sched := rt.NewSched(*seed)
	aliasedstdlib.DetsimSetSched(sched)

	var order []int
	var shadow string
	var closedOK bool
	sched.Go(func() {
		order = aliasedstdlib.CountTo(5)
		shadow = aliasedstdlib.ShadowClose()
		closedOK = aliasedstdlib.RealClose()
	})
	if err := sched.Run(); err != nil {
		panic(err)
	}

	sort.Ints(order)
	fmt.Printf("count=%d order=%v %s closedOK=%v\n", len(order), order, shadow, closedOK)
}
