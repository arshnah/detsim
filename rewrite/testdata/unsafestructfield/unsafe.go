package unsafestructfield

import "sync"

type Counter struct {
	mu    sync.Mutex
	value int
}

func (c *Counter) Add(n int) {
	c.mu.Lock()
	c.value += n
	c.mu.Unlock()
}

func RunConcurrently(c *Counter) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		c.Add(1)
	}()
	go func() {
		defer wg.Done()
		c.Add(2)
	}()
	wg.Wait()
}
