package pipeline

import (
	"sync"
	"time"
)

type Result struct {
	ID  int
	Sum int
}

func Run(ids []int) []Result {
	jobs := make(chan int, len(ids))
	results := make(chan Result, len(ids))
	var wg sync.WaitGroup

	for _, id := range ids {
		jobs <- id
	}
	close(jobs)

	wg.Add(3)
	for i := 0; i < 3; i++ {
		go func() {
			defer wg.Done()
			for {
				id, ok := <-jobs
				if !ok {
					return
				}
				time.Sleep(time.Millisecond)
				results <- Result{ID: id, Sum: id * id}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var out []Result
	for {
		r, ok := <-results
		if !ok {
			break
		}
		out = append(out, r)
	}
	return out
}
