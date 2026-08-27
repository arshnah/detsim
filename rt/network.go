package rt

import (
	"errors"
	"fmt"
	"io"
)

// Network is a deterministic, in-process stand-in for net.Dial/net.Listen.
type Network struct {
	sched          *Sched
	dropRate       float64
	minDelay       VirtualTime
	maxDelay       VirtualTime
	listeners      map[string]*Listener
	addrPartitions map[string]bool
	meshPartitions map[string]map[string]bool
}

// NewNetwork builds a Network on s's clock with a default 1-5 delivery delay range.
func NewNetwork(s *Sched) *Network {
	return &Network{
		sched:          s,
		minDelay:       1,
		maxDelay:       5,
		listeners:      make(map[string]*Listener),
		addrPartitions: make(map[string]bool),
		meshPartitions: make(map[string]map[string]bool),
	}
}

// SetDropRate sets the probability a write is silently dropped.
func (n *Network) SetDropRate(rate float64) { n.dropRate = rate }

// SetDelayRange sets the random delay range applied to delivered writes.
func (n *Network) SetDelayRange(min, max VirtualTime) { n.minDelay, n.maxDelay = min, max }

// PartitionAddr fully isolates addr from every other address, in both directions.
func (n *Network) PartitionAddr(addr string) { n.addrPartitions[addr] = true }

// HealAddr restores an address isolated by PartitionAddr.
func (n *Network) HealAddr(addr string) { n.addrPartitions[addr] = false }

// Partition cuts off every address in groupA from every address in groupB, both directions.
func (n *Network) Partition(groupA, groupB []string) {
	for _, a := range groupA {
		for _, b := range groupB {
			n.block(a, b)
			n.block(b, a)
		}
	}
}

func (n *Network) block(a, b string) {
	if n.meshPartitions[a] == nil {
		n.meshPartitions[a] = make(map[string]bool)
	}
	n.meshPartitions[a][b] = true
}

// HealAll clears every mesh partition set by Partition, not PartitionAddr isolation.
func (n *Network) HealAll() {
	n.meshPartitions = make(map[string]map[string]bool)
}

func (n *Network) blocked(local, remote string) bool {
	if n.addrPartitions[remote] {
		return true
	}
	if local == "" {
		return false
	}
	if n.addrPartitions[local] {
		return true
	}
	return n.meshPartitions[local][remote]
}

func (n *Network) randomDelay() VirtualTime {
	if n.maxDelay <= n.minDelay {
		return n.minDelay
	}
	return n.minDelay + VirtualTime(n.sched.Rand.Int63n(int64(n.maxDelay-n.minDelay)))
}

// Addr is a network address, matching the shape of net.Addr.
type Addr struct {
	addr string
}

// Network returns the fixed network type name "detsim".
func (a *Addr) Network() string { return "detsim" }

// String returns the address string.
func (a *Addr) String() string { return a.addr }

func newAddr(addr string) *Addr { return &Addr{addr: addr} }

// Listener accepts incoming connections at a registered address.
type Listener struct {
	net      *Network
	addr     string
	incoming *Chan[*Conn]
	closed   bool
}

// Listen registers a listener at addr, failing if it's already in use.
func (n *Network) Listen(addr string) (*Listener, error) {
	if _, exists := n.listeners[addr]; exists {
		return nil, fmt.Errorf("detsim/rt: address %s already in use", addr)
	}
	l := &Listener{net: n, addr: addr, incoming: NewChan[*Conn](n.sched, 16)}
	n.listeners[addr] = l
	return l, nil
}

// Accept blocks until an incoming connection arrives or the listener is closed.
func (l *Listener) Accept() (*Conn, error) {
	c, ok := l.incoming.RecvOK()
	if !ok {
		return nil, errors.New("detsim/rt: listener closed")
	}
	return c, nil
}

// Addr returns the listener's own address.
func (l *Listener) Addr() *Addr { return newAddr(l.addr) }

// Close stops the listener. Idempotent.
func (l *Listener) Close() error {
	if l.closed {
		return nil
	}
	l.closed = true
	l.incoming.Close()
	delete(l.net.listeners, l.addr)
	return nil
}

// Conn is a connected pair of endpoints, one side's Write delivering to the other's Read.
type Conn struct {
	net        *Network
	peer       *Conn
	localAddr  string
	remoteAddr string
	inbound    *Chan[[]byte]
	readBuf    []byte
	closed     bool
}

// Dial connects to addr with no particular local address.
func (n *Network) Dial(addr string) (*Conn, error) {
	return n.DialFrom("", addr)
}

// DialFrom connects to to from local address from, so partition checks are address-pair-aware.
func (n *Network) DialFrom(from, to string) (*Conn, error) {
	if n.blocked(from, to) {
		return nil, fmt.Errorf("detsim/rt: %s unreachable from %s", to, from)
	}
	l, ok := n.listeners[to]
	if !ok || l.closed {
		return nil, fmt.Errorf("detsim/rt: no listener at %s", to)
	}

	client := &Conn{net: n, localAddr: from, remoteAddr: to, inbound: NewChan[[]byte](n.sched, 64)}
	server := &Conn{net: n, localAddr: to, remoteAddr: from, inbound: NewChan[[]byte](n.sched, 64)}
	client.peer = server
	server.peer = client

	l.incoming.Send(server)
	return client, nil
}

// Write delivers p to the peer's Read after a randomized delay, or drops it per SetDropRate.
func (c *Conn) Write(p []byte) (int, error) {
	if c.closed {
		return 0, errors.New("detsim/rt: write on closed connection")
	}
	if c.net.blocked(c.localAddr, c.remoteAddr) {
		return len(p), nil
	}
	if c.net.dropRate > 0 && c.net.sched.Rand.Float64() < c.net.dropRate {
		return len(p), nil
	}

	data := append([]byte(nil), p...)
	delay := c.net.randomDelay()
	peer := c.peer
	localAddr, remoteAddr := c.localAddr, c.remoteAddr
	c.net.sched.Go(func() {
		c.net.sched.Sleep(delay)
		if peer.closed || c.net.blocked(localAddr, remoteAddr) {
			return
		}
		defer func() { recover() }()
		peer.inbound.Send(data)
	})
	return len(p), nil
}

// Read returns io.EOF once the connection is closed and drained.
func (c *Conn) Read(p []byte) (int, error) {
	if len(c.readBuf) == 0 {
		data, ok := c.inbound.RecvOK()
		if !ok {
			return 0, io.EOF
		}
		c.readBuf = data
	}
	n := copy(p, c.readBuf)
	c.readBuf = c.readBuf[n:]
	return n, nil
}

// LocalAddr returns the connection's local address.
func (c *Conn) LocalAddr() *Addr { return newAddr(c.localAddr) }

// RemoteAddr returns the connection's remote address.
func (c *Conn) RemoteAddr() *Addr { return newAddr(c.remoteAddr) }

// Close closes the connection. Idempotent.
func (c *Conn) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	c.inbound.Close()
	return nil
}
