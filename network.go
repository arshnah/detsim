package detsim

type NodeID string

type Network struct {
	sim        *Sim
	handlers   map[NodeID]func(from NodeID, msg any)
	partitions map[NodeID]map[NodeID]bool
	dropRate   float64
	minDelay   VirtualTime
	maxDelay   VirtualTime
}

func NewNetwork(s *Sim) *Network {
	return &Network{
		sim:        s,
		handlers:   make(map[NodeID]func(from NodeID, msg any)),
		partitions: make(map[NodeID]map[NodeID]bool),
		minDelay:   1,
		maxDelay:   5,
	}
}

func (n *Network) Register(id NodeID, handler func(from NodeID, msg any)) {
	n.handlers[id] = handler
}

func (n *Network) SetDropRate(rate float64)           { n.dropRate = rate }
func (n *Network) SetDelayRange(min, max VirtualTime) { n.minDelay, n.maxDelay = min, max }

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

func (n *Network) HealAll() {
	n.partitions = make(map[NodeID]map[NodeID]bool)
}

func (n *Network) blocked(from, to NodeID) bool {
	if m, ok := n.partitions[from]; ok && m[to] {
		return true
	}
	return n.sim.Rand.Float64() < n.dropRate
}

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
