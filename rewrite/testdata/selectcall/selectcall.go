package selectcall

var evals int

// Next hands out a fresh buffered channel carrying its evaluation ordinal, counting
// calls so tests can prove a select case's channel expression is evaluated once.
func Next() chan int {
	evals++
	ch := make(chan int, 1)
	ch <- evals
	return ch
}

// Evals reports how many times Next has been called.
func Evals() int { return evals }

// RecvFirst selects over the Next() call expression itself. Real Go semantics evaluate
// the operand once; a rewriter that splices the expression into both the ready check
// and the commit closure would evaluate it twice.
func RecvFirst() (got int) {
	select {
	case v := <-Next():
		got = v
	default:
		got = -1
	}
	return
}

// SendFirst selects a send over the Next() call expression, draining the ordinal first
// so the send case is actually ready.
func SendFirst() (sent bool) {
	ch := Next()
	<-ch
	select {
	case ch <- 99:
		sent = true
	default:
		sent = false
	}
	return
}
