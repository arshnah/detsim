package rt

// Mutex is a deterministic counterpart to sync.Mutex.
type Mutex struct {
	sched  *Sched
	locked bool
}

// NewMutex builds a Mutex bound to s.
func NewMutex(s *Sched) *Mutex { return &Mutex{sched: s} }

// Lock blocks while the mutex is held.
func (m *Mutex) Lock() {
	for m.locked {
		m.sched.parkCurrent(func() bool { return !m.locked }, "mutex lock")
	}
	m.locked = true
}

// TryLock acquires the mutex without blocking, reporting whether it succeeded.
func (m *Mutex) TryLock() bool {
	if m.locked {
		return false
	}
	m.locked = true
	return true
}

// Unlock releases the mutex, panicking if it isn't locked.
func (m *Mutex) Unlock() {
	if !m.locked {
		panic("detsim/rt: unlock of unlocked mutex")
	}
	m.locked = false
}

// RWMutex is a deterministic counterpart to sync.RWMutex.
type RWMutex struct {
	sched   *Sched
	writer  bool
	readers int
}

// NewRWMutex builds an RWMutex bound to s.
func NewRWMutex(s *Sched) *RWMutex { return &RWMutex{sched: s} }

// Lock blocks for exclusive access, excluding all readers and other writers.
func (m *RWMutex) Lock() {
	for m.writer || m.readers > 0 {
		m.sched.parkCurrent(func() bool { return !m.writer && m.readers == 0 }, "rwmutex lock")
	}
	m.writer = true
}

// Unlock releases exclusive access, panicking if not write-locked.
func (m *RWMutex) Unlock() {
	if !m.writer {
		panic("detsim/rt: unlock of unlocked rwmutex")
	}
	m.writer = false
}

// RLock blocks for shared read access, excluded only by a current writer.
func (m *RWMutex) RLock() {
	for m.writer {
		m.sched.parkCurrent(func() bool { return !m.writer }, "rwmutex rlock")
	}
	m.readers++
}

// RUnlock releases one reader, panicking if there is no outstanding reader.
func (m *RWMutex) RUnlock() {
	if m.readers == 0 {
		panic("detsim/rt: runlock of unlocked rwmutex")
	}
	m.readers--
}

// WaitGroup is a deterministic counterpart to sync.WaitGroup.
type WaitGroup struct {
	sched *Sched
	count int
}

// NewWaitGroup builds a WaitGroup bound to s.
func NewWaitGroup(s *Sched) *WaitGroup { return &WaitGroup{sched: s} }

// Add adjusts the counter, panicking if it goes negative.
func (w *WaitGroup) Add(delta int) {
	w.count += delta
	if w.count < 0 {
		panic("detsim/rt: negative WaitGroup counter")
	}
}

// Done is equivalent to Add(-1).
func (w *WaitGroup) Done() { w.Add(-1) }

// Wait blocks until the counter reaches zero.
func (w *WaitGroup) Wait() {
	for w.count > 0 {
		w.sched.parkCurrent(func() bool { return w.count == 0 }, "waitgroup wait")
	}
}

// Once is a deterministic counterpart to sync.Once.
type Once struct {
	sched *Sched
	done  bool
	doing bool
}

// NewOnce builds a Once bound to s.
func NewOnce(s *Sched) *Once { return &Once{sched: s} }

// Do runs fn exactly once across however many goroutines call it. If fn panics, Do
// considers the Once done anyway, matching sync.Once.
func (o *Once) Do(fn func()) {
	if o.done {
		return
	}
	if o.doing {
		o.sched.parkCurrent(func() bool { return o.done }, "once wait")
		return
	}
	o.doing = true
	defer func() {
		o.doing = false
		o.done = true
	}()
	fn()
}

// Cond is a deterministic counterpart to sync.Cond.
type Cond struct {
	sched   *Sched
	L       *Mutex
	waiting int
	tokens  int
}

// NewCond builds a Cond bound to s, guarded by l.
func NewCond(s *Sched, l *Mutex) *Cond { return &Cond{sched: s, L: l} }

// Wait unlocks L, blocks until signaled, then reacquires L. The caller must hold L.
func (c *Cond) Wait() {
	c.L.Unlock()
	c.waiting++
	c.sched.parkCurrent(func() bool { return c.tokens > 0 }, "cond wait")
	c.waiting--
	c.tokens--
	c.L.Lock()
}

// Signal wakes at most one waiter.
func (c *Cond) Signal() {
	if c.waiting > c.tokens {
		c.tokens++
	}
}

// Broadcast wakes all current waiters.
func (c *Cond) Broadcast() {
	if c.waiting > c.tokens {
		c.tokens = c.waiting
	}
}
