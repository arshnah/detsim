package aliasedstdlib

import (
	stdrand "math/rand"
	mysync "sync"
	mytime "time"
)

// CountTo spawns n goroutines through aliased stdlib imports: mysync for the WaitGroup
// and Mutex, mytime.Sleep for the stagger, stdrand.Intn for the delays. Every one of
// those must be rewritten or the scheduler hangs on a real (non-virtual) blocking call.
func CountTo(n int) []int {
	var mu mysync.Mutex
	var wg mysync.WaitGroup
	order := make([]int, 0, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			mytime.Sleep(mytime.Duration(stdrand.Intn(10)))
			mu.Lock()
			order = append(order, i)
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	return order
}

// ShadowClose calls a local variable that shadows the close builtin. The rewriter must
// leave it alone: only genuine builtin closes become .Close() method calls.
func ShadowClose() string {
	close := func(v ...int) int { return len(v) }
	if close(1, 2) != 2 {
		panic("shadowed close misbehaved")
	}
	return "shadow intact"
}

// RealClose uses the actual builtin on an actual channel.
func RealClose() bool {
	ch := make(chan int, 1)
	close(ch)
	_, open := <-ch
	return !open
}
