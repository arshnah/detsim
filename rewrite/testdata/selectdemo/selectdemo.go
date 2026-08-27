package selectdemo

func FirstOf(a, b chan int, done chan struct{}) int {
	result := -1
	select {
	case v := <-a:
		result = v
	case v := <-b:
		result = v
	case <-done:
		result = -1
	}
	return result
}

func TrySend(ch chan int, v int) bool {
	sent := false
	select {
	case ch <- v:
		sent = true
	default:
		sent = false
	}
	return sent
}

func Merge(a, b chan int, out chan int, n int) {
	for i := 0; i < n; i++ {
		select {
		case v, ok := <-a:
			if ok {
				out <- v
			}
		case v, ok := <-b:
			if ok {
				out <- v
			}
		}
	}
	close(out)
}
