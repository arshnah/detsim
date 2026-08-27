package unsupportedselect

func FirstReady(a, b chan int) int {
	select {
	case v := <-a:
		return v
	case v := <-b:
		return v
	}
}

func SpawnAndSum(a, b int) int {
	result := make(chan int, 1)
	go func() {
		result <- a + b
	}()
	return <-result
}
