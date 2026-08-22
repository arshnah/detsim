package raft

import (
	"math/rand"

	"github.com/arshnah/detsim"
)

type Node struct {
	id    detsim.NodeID
	peers []detsim.NodeID
	net   *detsim.Network
	sim   *detsim.Sim
	rng   *rand.Rand

	currentTerm int
	votedFor    detsim.NodeID
	log         []LogEntry

	commitIndex int
	lastApplied int
	state       State
	electionGen int

	votesReceived map[detsim.NodeID]bool

	nextIndex  map[detsim.NodeID]int
	matchIndex map[detsim.NodeID]int
	leaderGen  int

	Committed []LogEntry
}

func NewNode(id detsim.NodeID, peers []detsim.NodeID, net *detsim.Network, sim *detsim.Sim, seed int64) *Node {
	n := &Node{
		id:    id,
		peers: peers,
		net:   net,
		sim:   sim,
		rng:   rand.New(rand.NewSource(seed)),
		state: Follower,
		log:   []LogEntry{{Term: 0, Index: 0}},
	}
	net.Register(id, n.handle)
	return n
}

func (n *Node) Start() {
	n.scheduleElectionCheck()
}

func (n *Node) State() (State, int) { return n.state, n.currentTerm }

func (n *Node) Submit(cmd string) (index int, isLeader bool) {
	if n.state != Leader {
		return 0, false
	}
	idx := n.log[len(n.log)-1].Index + 1
	n.log = append(n.log, LogEntry{Term: n.currentTerm, Index: idx, Command: cmd})
	return idx, true
}

func (n *Node) lastLogInfo() (index, term int) {
	last := n.log[len(n.log)-1]
	return last.Index, last.Term
}
