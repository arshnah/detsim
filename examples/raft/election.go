package raft

import "github.com/arshnah/detsim"

func (n *Node) electionTimeout() detsim.VirtualTime {
	span := maxElectionTimeout - minElectionTimeout
	return detsim.VirtualTime(minElectionTimeout+n.rng.Intn(span+1)) * 1_000_000
}

func (n *Node) scheduleElectionCheck() {
	gen := n.electionGen
	n.sim.After(n.electionTimeout(), func(s *detsim.Sim) {
		if gen != n.electionGen {
			return
		}
		if n.state != Leader {
			n.startElection()
		}
		n.scheduleElectionCheck()
	})
}

func (n *Node) resetElectionTimer() {
	n.electionGen++
	n.scheduleElectionCheck()
}

func (n *Node) startElection() {
	n.electionGen++
	n.state = Candidate
	n.currentTerm++
	n.votedFor = n.id
	n.votesReceived = map[detsim.NodeID]bool{n.id: true}
	n.scheduleElectionCheck()

	lastIdx, lastTerm := n.lastIndex(), n.lastTerm()
	for _, p := range n.peers {
		n.net.Send(n.id, p, RequestVote{
			Term:         n.currentTerm,
			CandidateID:  n.id,
			LastLogIndex: lastIdx,
			LastLogTerm:  lastTerm,
		})
	}
}

func (n *Node) becomeFollower(term int) {
	n.state = Follower
	n.currentTerm = term
	n.votedFor = ""
	n.resetElectionTimer()
	n.failAllPendingReads()
}

func (n *Node) becomeLeader() {
	n.state = Leader
	n.leaderGen++
	n.nextIndex = make(map[detsim.NodeID]int)
	n.matchIndex = make(map[detsim.NodeID]int)
	lastIdx := n.lastIndex()
	for _, p := range n.peers {
		n.nextIndex[p] = lastIdx + 1
		n.matchIndex[p] = 0
	}
	n.sendHeartbeats(n.currentTerm, n.leaderGen)
}
