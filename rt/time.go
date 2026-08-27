package rt

// Sleep blocks the calling goroutine until virtual time has advanced by at least d.
func (s *Sched) Sleep(d VirtualTime) {
	if d < 0 {
		d = 0
	}
	s.sleepUntil(s.now + d)
}

// After is the rt equivalent of time.After: sends the wakeup time on a fresh channel.
func (s *Sched) After(d VirtualTime) *Chan[VirtualTime] {
	ch := NewChan[VirtualTime](s, 1)
	s.Go(func() {
		s.Sleep(d)
		ch.Send(s.Now())
	})
	return ch
}
