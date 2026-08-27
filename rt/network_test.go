package rt

import "testing"

func TestDialAcceptRoundTrip(t *testing.T) {
	s := NewSched(1)
	net := NewNetwork(s)
	l, err := net.Listen("server")
	if err != nil {
		t.Fatalf("Listen() = %v", err)
	}
	var got string

	s.Go(func() {
		conn, err := l.Accept()
		if err != nil {
			t.Errorf("Accept() = %v", err)
			return
		}
		buf := make([]byte, 5)
		n, err := conn.Read(buf)
		if err != nil {
			t.Errorf("Read() = %v", err)
			return
		}
		got = string(buf[:n])
	})
	s.Go(func() {
		conn, err := net.Dial("server")
		if err != nil {
			t.Errorf("Dial() = %v", err)
			return
		}
		conn.Write([]byte("hello"))
	})

	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if got != "hello" {
		t.Fatalf("got %q, want %q", got, "hello")
	}
}

func TestDialWithNoListenerFails(t *testing.T) {
	s := NewSched(1)
	net := NewNetwork(s)
	var dialErr error
	s.Go(func() {
		_, dialErr = net.Dial("nobody")
	})
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if dialErr == nil {
		t.Fatal("expected an error dialing an address with no listener")
	}
}

func TestPartitionedAddressCannotBeDialed(t *testing.T) {
	s := NewSched(1)
	net := NewNetwork(s)
	l, err := net.Listen("server")
	if err != nil {
		t.Fatalf("Listen() = %v", err)
	}
	net.PartitionAddr("server")

	s.Go(func() {
		l.Accept()
	})

	var dialErr error
	s.Go(func() {
		_, dialErr = net.Dial("server")
	})

	err = s.Run()
	derr, ok := err.(*DeadlockError)
	if !ok {
		t.Fatalf("expected deadlock (accept never satisfied), got %v", err)
	}
	if len(derr.Goroutines) != 1 {
		t.Fatalf("expected exactly the accepting goroutine still blocked, got %d", len(derr.Goroutines))
	}
	if dialErr == nil {
		t.Fatal("expected Dial to a partitioned address to fail")
	}
}

func TestHealAllowsDialAgain(t *testing.T) {
	s := NewSched(1)
	net := NewNetwork(s)
	l, err := net.Listen("server")
	if err != nil {
		t.Fatalf("Listen() = %v", err)
	}
	net.PartitionAddr("server")
	net.HealAddr("server")

	var dialErr error
	s.Go(func() {
		l.Accept()
	})
	s.Go(func() {
		_, dialErr = net.Dial("server")
	})
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if dialErr != nil {
		t.Fatalf("Dial() = %v after Heal", dialErr)
	}
}

func TestWriteAfterCloseErrors(t *testing.T) {
	s := NewSched(1)
	net := NewNetwork(s)
	l, err := net.Listen("server")
	if err != nil {
		t.Fatalf("Listen() = %v", err)
	}
	var writeErr error

	s.Go(func() {
		conn, err := l.Accept()
		if err != nil {
			t.Errorf("Accept() = %v", err)
			return
		}
		conn.Close()
	})
	s.Go(func() {
		conn, err := net.Dial("server")
		if err != nil {
			t.Errorf("Dial() = %v", err)
			return
		}
		s.Sleep(100)
		_, writeErr = conn.Write([]byte("x"))
	})

	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if writeErr != nil {
		t.Fatalf("write to a live (not-yet-closed-locally) connection should not itself error just because the peer closed, got %v", writeErr)
	}
}

func TestDropRateActuallyDropsSomeMessages(t *testing.T) {
	s := NewSched(1)
	net := NewNetwork(s)
	net.SetDropRate(1.0)
	l, err := net.Listen("server")
	if err != nil {
		t.Fatalf("Listen() = %v", err)
	}

	received := false
	s.Go(func() {
		conn, err := l.Accept()
		if err != nil {
			t.Errorf("Accept() = %v", err)
			return
		}
		s.Go(func() {
			buf := make([]byte, 1)
			conn.Read(buf)
			received = true
		})
	})
	s.Go(func() {
		conn, err := net.Dial("server")
		if err != nil {
			t.Errorf("Dial() = %v", err)
			return
		}
		conn.Write([]byte("x"))
		conn.Close()
	})

	if err := s.Run(); err != nil {
		if _, ok := err.(*DeadlockError); !ok {
			t.Fatalf("Run() = %v", err)
		}
	}
	if received {
		t.Fatal("expected the message to be dropped at drop rate 1.0")
	}
}

func TestSameSeedNetworkIsDeterministic(t *testing.T) {
	run := func(seed int64) []string {
		s := NewSched(seed)
		net := NewNetwork(s)
		net.SetDropRate(0.3)
		net.SetDelayRange(1, 20)
		l, err := net.Listen("server")
		if err != nil {
			t.Fatalf("Listen() = %v", err)
		}
		var order []string
		m := NewMutex(s)

		s.Go(func() {
			for i := 0; i < 5; i++ {
				conn, err := l.Accept()
				if err != nil {
					return
				}
				s.Go(func() {
					buf := make([]byte, 8)
					n, err := conn.Read(buf)
					if err != nil {
						return
					}
					m.Lock()
					order = append(order, string(buf[:n]))
					m.Unlock()
				})
			}
		})
		for i := 0; i < 5; i++ {
			i := i
			s.Go(func() {
				conn, err := net.Dial("server")
				if err != nil {
					return
				}
				conn.Write([]byte{byte('a' + i)})
			})
		}
		s.Run()
		return order
	}

	a := run(3)
	b := run(3)
	if len(a) != len(b) {
		t.Fatalf("same seed produced different message counts: %v vs %v", a, b)
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("same seed produced different order: %v vs %v", a, b)
		}
	}
}

func TestAddrSatisfiesNetAddrShape(t *testing.T) {
	s := NewSched(1)
	net := NewNetwork(s)
	l, err := net.Listen("server")
	if err != nil {
		t.Fatalf("Listen() = %v", err)
	}
	if got := l.Addr().String(); got != "server" {
		t.Fatalf("expected listener addr %q, got %q", "server", got)
	}

	var got *Addr
	s.Go(func() {
		l.Accept()
	})
	s.Go(func() {
		conn, err := net.DialFrom("client", "server")
		if err != nil {
			t.Errorf("DialFrom() = %v", err)
			return
		}
		got = conn.RemoteAddr()
	})
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if got.String() != "server" {
		t.Fatalf("expected client's RemoteAddr() = %q, got %q", "server", got.String())
	}
	if got.Network() == "" {
		t.Fatal("expected a non-empty Network() to satisfy the net.Addr shape")
	}
}
