package raft

import "github.com/arshnah/detsim"

type pendingRead struct {
	term    int
	index   int
	seq     int
	acks    map[detsim.NodeID]bool
	done    func(ok bool)
	settled bool
}

// Read performs a linearizable read via ReadIndex. The leader confirms it still
// holds a quorum, waits for local apply to catch up, then calls done(true).
// Non-leaders and leaders that step down mid-flight call done(false).
func (n *Node) Read(done func(ok bool)) {
	if n.state != Leader {
		done(false)
		return
	}

	n.readSeq++
	pr := &pendingRead{
		term:  n.currentTerm,
		index: n.commitIndex,
		seq:   n.readSeq,
		acks:  map[detsim.NodeID]bool{n.id: true},
		done:  done,
	}
	n.pendingReads = append(n.pendingReads, pr)

	for _, p := range n.peers {
		n.net.Send(n.id, p, PingRequest{Term: n.currentTerm, Leader: n.id, ReadSeq: pr.seq})
	}

	n.tryResolveReads()
}

func (n *Node) onPingRequest(from detsim.NodeID, m PingRequest) {
	if m.Term > n.currentTerm {
		n.becomeFollower(m.Term)
	} else if m.Term == n.currentTerm && n.state != Leader {
		n.resetElectionTimer()
	}
	if m.Term < n.currentTerm {
		n.net.Send(n.id, from, PingReply{Term: n.currentTerm, From: n.id, ReadSeq: m.ReadSeq})
		return
	}
	n.net.Send(n.id, from, PingReply{Term: n.currentTerm, From: n.id, ReadSeq: m.ReadSeq})
}

func (n *Node) onPingReply(m PingReply) {
	if m.Term > n.currentTerm {
		n.becomeFollower(m.Term)
		return
	}
	if n.state != Leader || m.Term != n.currentTerm {
		return
	}
	for _, pr := range n.pendingReads {
		if pr.seq == m.ReadSeq && !pr.settled {
			pr.acks[m.From] = true
		}
	}
	n.tryResolveReads()
}

func (n *Node) tryResolveReads() {
	var remaining []*pendingRead
	for _, pr := range n.pendingReads {
		if pr.settled {
			continue
		}
		if pr.term != n.currentTerm || n.state != Leader {
			pr.settled = true
			pr.done(false)
			continue
		}
		if len(pr.acks)*2 > len(n.peers)+1 && n.lastApplied >= pr.index {
			pr.settled = true
			pr.done(true)
			continue
		}
		remaining = append(remaining, pr)
	}
	n.pendingReads = remaining
}

func (n *Node) failAllPendingReads() {
	for _, pr := range n.pendingReads {
		if !pr.settled {
			pr.settled = true
			pr.done(false)
		}
	}
	n.pendingReads = nil
}
