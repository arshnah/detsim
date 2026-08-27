package rt

import "testing"

func TestMeshPartitionBlocksOnlyThePartitionedPair(t *testing.T) {
	s := NewSched(1)
	net := NewNetwork(s)
	l, err := net.Listen("nodeB")
	if err != nil {
		t.Fatalf("Listen() = %v", err)
	}
	net.Partition([]string{"nodeA"}, []string{"nodeB"})

	var dialErrFromA, dialErrFromC error
	s.Go(func() {
		l.Accept()
	})
	s.Go(func() {
		_, dialErrFromA = net.DialFrom("nodeA", "nodeB")
	})

	err = s.Run()
	if _, ok := err.(*DeadlockError); !ok {
		t.Fatalf("expected deadlock (accept never satisfied by the blocked pair), got %v", err)
	}
	if dialErrFromA == nil {
		t.Fatal("expected nodeA -> nodeB to fail, they're partitioned")
	}

	s2 := NewSched(1)
	net2 := NewNetwork(s2)
	l2, err := net2.Listen("nodeB")
	if err != nil {
		t.Fatalf("Listen() = %v", err)
	}
	net2.Partition([]string{"nodeA"}, []string{"nodeB"})
	s2.Go(func() {
		conn, err := l2.Accept()
		if err != nil {
			t.Errorf("Accept() = %v", err)
			return
		}
		conn.Close()
	})
	s2.Go(func() {
		_, dialErrFromC = net2.DialFrom("nodeC", "nodeB")
	})
	if err := s2.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if dialErrFromC != nil {
		t.Fatalf("nodeC -> nodeB should still work, only nodeA <-> nodeB is partitioned, got %v", dialErrFromC)
	}
}

func TestMeshPartitionIsSymmetric(t *testing.T) {
	s := NewSched(1)
	net := NewNetwork(s)
	l, err := net.Listen("nodeA")
	if err != nil {
		t.Fatalf("Listen() = %v", err)
	}
	net.Partition([]string{"nodeA"}, []string{"nodeB"})

	var dialErr error
	s.Go(func() {
		l.Accept()
	})
	s.Go(func() {
		_, dialErr = net.DialFrom("nodeB", "nodeA")
	})

	err = s.Run()
	if _, ok := err.(*DeadlockError); !ok {
		t.Fatalf("expected deadlock, got %v", err)
	}
	if dialErr == nil {
		t.Fatal("expected nodeB -> nodeA to fail too, partitions are symmetric")
	}
}

func TestHealAllRestoresMeshConnectivity(t *testing.T) {
	s := NewSched(1)
	net := NewNetwork(s)
	l, err := net.Listen("nodeB")
	if err != nil {
		t.Fatalf("Listen() = %v", err)
	}
	net.Partition([]string{"nodeA"}, []string{"nodeB"})
	net.HealAll()

	var dialErr error
	s.Go(func() {
		l.Accept()
	})
	s.Go(func() {
		_, dialErr = net.DialFrom("nodeA", "nodeB")
	})
	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if dialErr != nil {
		t.Fatalf("Dial() = %v after HealAll", dialErr)
	}
}

func TestMeshPartitionDuringFlightDropsInTransitMessages(t *testing.T) {
	s := NewSched(1)
	net := NewNetwork(s)
	net.SetDelayRange(50, 51)
	l, err := net.Listen("nodeB")
	if err != nil {
		t.Fatalf("Listen() = %v", err)
	}

	var received bool
	s.Go(func() {
		conn, err := l.Accept()
		if err != nil {
			t.Errorf("Accept() = %v", err)
			return
		}
		s.Go(func() {
			buf := make([]byte, 1)
			_, err := conn.Read(buf)
			if err == nil {
				received = true
			}
		})
	})
	s.Go(func() {
		conn, err := net.DialFrom("nodeA", "nodeB")
		if err != nil {
			t.Errorf("DialFrom() = %v", err)
			return
		}
		conn.Write([]byte("x"))
		net.Partition([]string{"nodeA"}, []string{"nodeB"})
	})

	if err := s.Run(); err != nil {
		if _, ok := err.(*DeadlockError); !ok {
			t.Fatalf("Run() = %v", err)
		}
	}
	if received {
		t.Fatal("expected the in-flight message to be dropped once the pair partitioned mid-delay")
	}
}

func TestDialFromRecordsBothEndpointsForFutureWrites(t *testing.T) {
	s := NewSched(1)
	net := NewNetwork(s)
	l, err := net.Listen("nodeB")
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
		conn, err := net.DialFrom("nodeA", "nodeB")
		if err != nil {
			t.Errorf("DialFrom() = %v", err)
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
