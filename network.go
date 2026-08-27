package detsim

// NodeID identifies a node registered on a Network.
type NodeID string

// Network is a fault-injectable network on top of a Sim.
type Network struct {
	sim        *Sim
	handlers   map[NodeID]func(from NodeID, msg any)
	partitions map[NodeID]map[NodeID]bool
	dropRate   float64
	minDelay   VirtualTime
	maxDelay   VirtualTime
}

// NewNetwork builds a Network driven by s's clock and Rand.
func NewNetwork(s *Sim) *Network {
	return &Network{
		sim:        s,
		handlers:   make(map[NodeID]func(from NodeID, msg any)),
		partitions: make(map[NodeID]map[NodeID]bool),
		minDelay:   1,
		maxDelay:   5,
	}
}

// Register sets the handler that receives messages addressed to id.
func (n *Network) Register(id NodeID, handler func(from NodeID, msg any)) {
	n.handlers[id] = handler
}

// SetDropRate sets the probability a send or delivery is dropped.
func (n *Network) SetDropRate(rate float64) { n.dropRate = rate }

// SetDelayRange sets the range delivery delay is drawn from.
func (n *Network) SetDelayRange(min, max VirtualTime) { n.minDelay, n.maxDelay = min, max }

// Partition blocks delivery between every node in groupA and every node in groupB.
func (n *Network) Partition(groupA, groupB []NodeID) {
	for _, a := range groupA {
		for _, b := range groupB {
			n.block(a, b)
			n.block(b, a)
		}
	}
}

func (n *Network) block(a, b NodeID) {
	if n.partitions[a] == nil {
		n.partitions[a] = make(map[NodeID]bool)
	}
	n.partitions[a][b] = true
}

// HealAll clears every partition set by Partition.
func (n *Network) HealAll() {
	n.partitions = make(map[NodeID]map[NodeID]bool)
}

// Heal clears the specific partition between a and b (both directions).
func (n *Network) Heal(a, b NodeID) {
	if m, ok := n.partitions[a]; ok {
		delete(m, b)
	}
	if m, ok := n.partitions[b]; ok {
		delete(m, a)
	}
}

func (n *Network) blocked(from, to NodeID) bool {
	if m, ok := n.partitions[from]; ok && m[to] {
		return true
	}
	return n.sim.Rand.Float64() < n.dropRate
}

// Send delivers msg from from to to after a random delay, subject to drops and partitions.
func (n *Network) Send(from, to NodeID, msg any) {
	if n.blocked(from, to) {
		return
	}
	delayRange := n.maxDelay - n.minDelay
	delay := n.minDelay
	if delayRange > 0 {
		delay += VirtualTime(n.sim.Rand.Int63n(int64(delayRange)))
	}
	n.sim.After(delay, func(s *Sim) {
		if n.blocked(from, to) {
			return
		}
		if handler, ok := n.handlers[to]; ok {
			handler(from, msg)
		}
	})
}
