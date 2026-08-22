package raft

import "github.com/arshnah/detsim"

func (n *Node) sendHeartbeats(term, gen int) {
	if n.state != Leader || n.currentTerm != term || n.leaderGen != gen {
		return
	}
	for _, p := range n.peers {
		n.replicateTo(p)
	}
	n.sim.After(heartbeatInterval*1_000_000, func(s *detsim.Sim) {
		n.sendHeartbeats(term, gen)
	})
}

func (n *Node) replicateTo(peer detsim.NodeID) {
	next := n.nextIndex[peer]
	prevIdx := next - 1
	prevTerm := 0
	if prevIdx >= 0 && prevIdx < len(n.log) {
		prevTerm = n.log[prevIdx].Term
	}
	var entries []LogEntry
	if next >= 0 && next < len(n.log) {
		entries = append(entries, n.log[next:]...)
	}
	n.net.Send(n.id, peer, AppendEntries{
		Term:         n.currentTerm,
		LeaderID:     n.id,
		PrevLogIndex: prevIdx,
		PrevLogTerm:  prevTerm,
		Entries:      entries,
		LeaderCommit: n.commitIndex,
	})
}

func (n *Node) advanceCommitIndex() {
	for idx := n.log[len(n.log)-1].Index; idx > n.commitIndex; idx-- {
		if idx < 0 || idx >= len(n.log) || n.log[idx].Term != n.currentTerm {
			continue
		}
		count := 1
		for _, p := range n.peers {
			if n.matchIndex[p] >= idx {
				count++
			}
		}
		if count*2 > len(n.peers)+1 {
			n.commitIndex = idx
			n.applyCommitted()
			return
		}
	}
}

func (n *Node) applyCommitted() {
	for n.lastApplied < n.commitIndex {
		n.lastApplied++
		n.Committed = append(n.Committed, n.log[n.lastApplied])
	}
}
