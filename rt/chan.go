package rt

// Chan is a generic channel that blocks through the scheduler instead of the Go runtime.
type Chan[T any] struct {
	sched       *Sched
	cap         int
	buf         []T
	closed      bool
	recvWaiting int
}

// NewChan creates a channel with the given buffer capacity, 0 for unbuffered.
func NewChan[T any](s *Sched, cap int) *Chan[T] {
	return &Chan[T]{sched: s, cap: cap}
}

// Send blocks until there's room, or a receiver is waiting on an unbuffered channel.
func (c *Chan[T]) Send(v T) {
	for {
		if c.closed {
			panic("detsim/rt: send on closed channel")
		}
		if len(c.buf) < c.cap || (c.cap == 0 && c.recvWaiting > 0) {
			c.buf = append(c.buf, v)
			return
		}
		c.sched.parkCurrent(func() bool {
			return c.closed || len(c.buf) < c.cap || (c.cap == 0 && c.recvWaiting > 0)
		}, "chan send")
	}
}

// Recv blocks until a value is available, returning the zero value if closed and drained.
func (c *Chan[T]) Recv() T {
	v, _ := c.RecvOK()
	return v
}

// RecvOK blocks until a value is available or the channel closes.
func (c *Chan[T]) RecvOK() (T, bool) {
	for {
		if len(c.buf) > 0 {
			v := c.buf[0]
			var zero T
			c.buf[0] = zero
			c.buf = c.buf[1:]
			return v, true
		}
		if c.closed {
			var zero T
			return zero, false
		}
		c.recvWaiting++
		c.sched.parkCurrent(func() bool {
			return len(c.buf) > 0 || c.closed
		}, "chan recv")
		c.recvWaiting--
	}
}

// TryRecv attempts a non-blocking receive.
func (c *Chan[T]) TryRecv() (T, bool) {
	if len(c.buf) > 0 {
		v := c.buf[0]
		var zero T
		c.buf[0] = zero
		c.buf = c.buf[1:]
		return v, true
	}
	var zero T
	return zero, false
}

// Close closes the channel, panicking if already closed.
func (c *Chan[T]) Close() {
	if c.closed {
		panic("detsim/rt: close of closed channel")
	}
	c.closed = true
}

// Closed reports whether the channel has been closed.
func (c *Chan[T]) Closed() bool { return c.closed }

// Len returns the number of buffered elements.
func (c *Chan[T]) Len() int { return len(c.buf) }

// Cap returns the configured buffer capacity.
func (c *Chan[T]) Cap() int { return c.cap }
