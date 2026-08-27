package main

import (
	"flag"
	"fmt"

	"github.com/arshnah/detsim/rewrite/testdata/selectdemo"
	"github.com/arshnah/detsim/rt"
)

func main() {
	seed := flag.Int64("seed", 1, "scheduler seed")
	flag.Parse()

	sched := rt.NewSched(*seed)
	selectdemo.DetsimSetSched(sched)

	var firstResult int
	var trySendOK bool
	var merged []int

	sched.Go(func() {
		a := rt.NewChan[int](sched, 1)
		b := rt.NewChan[int](sched, 1)
		done := rt.NewChan[struct{}](sched, 1)
		a.Send(7)
		firstResult = selectdemo.FirstOf(a, b, done)
	})
	if err := sched.Run(); err != nil {
		panic(err)
	}

	sched2 := rt.NewSched(*seed)
	selectdemo.DetsimSetSched(sched2)
	sched2.Go(func() {
		ch := rt.NewChan[int](sched2, 1)
		trySendOK = selectdemo.TrySend(ch, 5)
	})
	if err := sched2.Run(); err != nil {
		panic(err)
	}

	sched3 := rt.NewSched(*seed)
	selectdemo.DetsimSetSched(sched3)
	sched3.Go(func() {
		a := rt.NewChan[int](sched3, 5)
		b := rt.NewChan[int](sched3, 5)
		out := rt.NewChan[int](sched3, 10)
		for i := 0; i < 5; i++ {
			a.Send(i)
			b.Send(i + 100)
		}
		a.Close()
		b.Close()
		sched3.Go(func() {
			selectdemo.Merge(a, b, out, 10)
		})
		for {
			v, ok := out.RecvOK()
			if !ok {
				break
			}
			merged = append(merged, v)
		}
	})
	if err := sched3.Run(); err != nil {
		panic(err)
	}

	fmt.Printf("first=%d trySendOK=%v merged=%v\n", firstResult, trySendOK, merged)
}
