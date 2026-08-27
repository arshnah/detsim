package rt

// SelectCase is one case in a Select, built by RecvCase, SendCase, or DefaultCase.
type SelectCase struct {
	isDefault  bool
	ready      func() bool
	register   func()
	unregister func()
	commit     func()
}

// RecvCase is ready when ch has a value or is closed. commit performs the actual receive.
func RecvCase[T any](ch *Chan[T], commit func()) SelectCase {
	return SelectCase{
		ready:      func() bool { return len(ch.buf) > 0 || ch.closed },
		register:   func() { ch.recvWaiting++ },
		unregister: func() { ch.recvWaiting-- },
		commit:     commit,
	}
}

// SendCase is ready when ch has room and isn't closed. commit performs the actual send.
func SendCase[T any](ch *Chan[T], commit func()) SelectCase {
	return SelectCase{
		ready: func() bool {
			return !ch.closed && (len(ch.buf) < ch.cap || (ch.cap == 0 && ch.recvWaiting > 0))
		},
		register:   func() {},
		unregister: func() {},
		commit:     commit,
	}
}

// DefaultCase fires if no other case is ready when Select is first evaluated.
func DefaultCase(commit func()) SelectCase {
	return SelectCase{isDefault: true, commit: commit}
}

// Select evaluates every case, picking uniformly at random among the ready ones.
func (s *Sched) Select(cases ...SelectCase) {
	for {
		var ready []int
		defaultIdx := -1
		for i, c := range cases {
			if c.isDefault {
				defaultIdx = i
				continue
			}
			if c.ready() {
				ready = append(ready, i)
			}
		}

		if len(ready) > 0 {
			cases[ready[s.Rand.Intn(len(ready))]].commit()
			return
		}
		if defaultIdx >= 0 {
			cases[defaultIdx].commit()
			return
		}

		for _, c := range cases {
			if !c.isDefault {
				c.register()
			}
		}
		s.parkCurrent(func() bool {
			for _, c := range cases {
				if !c.isDefault && c.ready() {
					return true
				}
			}
			return false
		}, "select")
		for _, c := range cases {
			if !c.isDefault {
				c.unregister()
			}
		}
	}
}
