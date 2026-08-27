package raft

import (
	"math/rand"

	"github.com/arshnah/detsim"
)

// Node is a single Raft node participating in leader election, log replication,
// snapshots, linearizable reads, and cluster membership changes.
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

	readSeq      int
	pendingReads []*pendingRead

	Committed []LogEntry
}

// NewNode creates a Raft node and registers it on the network. Call Start to begin
// participating in elections.
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

// Start kicks off the election timer so this node begins participating in the cluster.
func (n *Node) Start() {
	n.scheduleElectionCheck()
}

// State returns the node's current role and term.
func (n *Node) State() (State, int) { return n.state, n.currentTerm }

// ID returns the node's unique identifier.
func (n *Node) ID() detsim.NodeID { return n.id }

// Submit appends cmd to the leader's log. Returns the log index and true on success,
// or (0, false) if this node is not the current leader.
func (n *Node) Submit(cmd string) (index int, isLeader bool) {
	if n.state != Leader {
		return 0, false
	}
	idx := n.lastIndex() + 1
	n.log = append(n.log, LogEntry{Term: n.currentTerm, Index: idx, Command: cmd})
	return idx, true
}
