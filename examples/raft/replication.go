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
	if next <= n.baseIndex() {
		n.sendInstallSnapshot(peer)
		return
	}
	prevIdx := next - 1
	prevTerm, _ := n.termAt(prevIdx)
	entries := append([]LogEntry(nil), n.entriesFrom(next)...)
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
	for idx := n.lastIndex(); idx > n.commitIndex && idx > n.baseIndex(); idx-- {
		term, ok := n.termAt(idx)
		if !ok || term != n.currentTerm {
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
		if n.hasIndex(n.lastApplied) {
			n.Committed = append(n.Committed, n.entryAt(n.lastApplied))
		}
	}
	n.maybeCompact()
	n.tryResolveReads()
}
