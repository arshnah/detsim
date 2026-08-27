package traceview_test

import (
	"fmt"
	"os"

	"github.com/arshnah/detsim/rt"
	"github.com/arshnah/detsim/traceview"
)

// A trace from a run where a producer and consumer deadlocked, rendered as an
// annotated timeline.
func ExampleView() {
	tr := rt.Trace{
		Seed:      847291,
		Decisions: []uint64{0, 1, 0, 0, 1},
		Labels:    []string{"producer", "consumer"},
		Steps: []rt.Step{
			{At: 0, Goroutine: 0, After: "chan send"},
			{At: 0, Goroutine: 1, After: "chan recv"},
			{At: 0, Goroutine: 0, After: "chan send"},
			{At: 0, Goroutine: 0, After: "finished"},
			{At: 0, Goroutine: 1, After: "chan recv"},
		},
	}
	if err := traceview.View(os.Stdout, tr, traceview.Options{}); err != nil {
		fmt.Println("render failed:", err)
	}
	// Output:
	// seed 847291 · 5 decisions · 2 goroutines · no time advanced
	//
	//   #1 producer blocked on chan send  at t=0s
	//   #2 consumer blocked on chan recv  at t=0s
	//   #3 producer blocked on chan send  at t=0s
	//   #4 producer ran to completion  at t=0s
	//   #5 consumer blocked on chan recv  at t=0s
}
