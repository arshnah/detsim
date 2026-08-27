package rt

import "testing"

func TestThreeNodeMeshPartitionOnlyIsolatesTheBlockedPair(t *testing.T) {
	s := NewSched(1)
	net := NewNetwork(s)
	net.SetDelayRange(1, 5)

	var receivedByA, receivedByB, receivedByC []string
	mA, mB, mC := NewMutex(s), NewMutex(s), NewMutex(s)
	lA := listenAndCollect(s, net, "A", &receivedByA, mA)
	lB := listenAndCollect(s, net, "B", &receivedByB, mB)
	lC := listenAndCollect(s, net, "C", &receivedByC, mC)

	net.Partition([]string{"A"}, []string{"B"})

	broadcastsDone := NewWaitGroup(s)
	broadcastsDone.Add(3)
	s.Go(func() {
		defer broadcastsDone.Done()
		broadcast(net, "A", []string{"B", "C"}, []byte("from-A"))
	})
	s.Go(func() {
		defer broadcastsDone.Done()
		broadcast(net, "B", []string{"A", "C"}, []byte("from-B"))
	})
	s.Go(func() {
		defer broadcastsDone.Done()
		broadcast(net, "C", []string{"A", "B"}, []byte("from-C"))
	})
	s.Go(func() {
		broadcastsDone.Wait()
		s.Sleep(20)
		lA.Close()
		lB.Close()
		lC.Close()
	})

	if err := s.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}

	for _, msg := range receivedByA {
		if msg == "from-B" {
			t.Fatal("A should never receive from B, they're partitioned")
		}
	}
	for _, msg := range receivedByB {
		if msg == "from-A" {
			t.Fatal("B should never receive from A, they're partitioned")
		}
	}

	foundAtoC, foundBtoC, foundCtoA, foundCtoB := false, false, false, false
	for _, msg := range receivedByC {
		if msg == "from-A" {
			foundAtoC = true
		}
		if msg == "from-B" {
			foundBtoC = true
		}
	}
	for _, msg := range receivedByA {
		if msg == "from-C" {
			foundCtoA = true
		}
	}
	for _, msg := range receivedByB {
		if msg == "from-C" {
			foundCtoB = true
		}
	}
	if !foundAtoC || !foundBtoC || !foundCtoA || !foundCtoB {
		t.Fatalf("expected all non-partitioned pairs to still communicate: A->C=%v B->C=%v C->A=%v C->B=%v",
			foundAtoC, foundBtoC, foundCtoA, foundCtoB)
	}
}

func broadcast(net *Network, from string, peers []string, msg []byte) {
	for _, peer := range peers {
		conn, err := net.DialFrom(from, peer)
		if err != nil {
			continue
		}
		conn.Write(msg)
		conn.Close()
	}
}

func listenAndCollect(s *Sched, net *Network, addr string, received *[]string, m *Mutex) *Listener {
	l, err := net.Listen(addr)
	if err != nil {
		return nil
	}
	s.Go(func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			s.Go(func() {
				buf := make([]byte, 32)
				n, err := conn.Read(buf)
				if err != nil {
					return
				}
				m.Lock()
				*received = append(*received, string(buf[:n]))
				m.Unlock()
			})
		}
	})
	return l
}
