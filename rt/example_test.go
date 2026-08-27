package rt_test

import (
	"fmt"

	"github.com/arshnah/detsim/rt"
)

func Example() {
	sched := rt.NewSched(1)
	ch := rt.NewChan[int](sched, 0)

	sched.Go(func() {
		for i := 1; i <= 3; i++ {
			ch.Send(i * 10)
		}
		ch.Close()
	})

	sched.Go(func() {
		for {
			v, ok := ch.RecvOK()
			if !ok {
				return
			}
			fmt.Println("received:", v)
		}
	})

	sched.Run()
}

func ExampleSched_deadlock() {
	sched := rt.NewSched(1)
	a := rt.NewChan[int](sched, 0)
	b := rt.NewChan[int](sched, 0)

	sched.Go(func() {
		a.Send(1)
		b.Recv()
	})
	sched.Go(func() {
		b.Send(1)
		a.Recv()
	})

	err := sched.Run()
	if _, ok := err.(*rt.DeadlockError); ok {
		fmt.Println("deadlock detected")
	}
}
