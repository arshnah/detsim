package raft

import "github.com/arshnah/detsim"

func (n *Node) handle(from detsim.NodeID, msg any) {
	switch m := msg.(type) {
	case RequestVote:
		n.onRequestVote(from, m)
	case RequestVoteReply:
		n.onRequestVoteReply(m)
	case AppendEntries:
		n.onAppendEntries(from, m)
	case AppendEntriesReply:
		n.onAppendEntriesReply(m)
	case InstallSnapshot:
		n.onInstallSnapshot(from, m)
	case InstallSnapshotReply:
		n.onInstallSnapshotReply(m)
	case PingRequest:
		n.onPingRequest(from, m)
	case PingReply:
		n.onPingReply(m)
	}
}

func (n *Node) onRequestVote(from detsim.NodeID, m RequestVote) {
	if m.Term > n.currentTerm {
		n.becomeFollower(m.Term)
	}
	reply := RequestVoteReply{Term: n.currentTerm, Voter: n.id}
	if m.Term < n.currentTerm {
		n.net.Send(n.id, from, reply)
		return
	}
	canVote := n.votedFor == "" || n.votedFor == m.CandidateID
	lastIdx, lastTerm := n.lastIndex(), n.lastTerm()
	logOK := m.LastLogTerm > lastTerm || (m.LastLogTerm == lastTerm && m.LastLogIndex >= lastIdx)
	if canVote && logOK {
		n.votedFor = m.CandidateID
		n.resetElectionTimer()
		reply.VoteGranted = true
	}
	n.net.Send(n.id, from, reply)
}

func (n *Node) onRequestVoteReply(m RequestVoteReply) {
	if m.Term > n.currentTerm {
		n.becomeFollower(m.Term)
		return
	}
	if n.state != Candidate || m.Term != n.currentTerm || !m.VoteGranted {
		return
	}
	n.votesReceived[m.Voter] = true
	if len(n.votesReceived)*2 > len(n.peers)+1 {
		n.becomeLeader()
	}
}

func (n *Node) onAppendEntries(from detsim.NodeID, m AppendEntries) {
	reply := AppendEntriesReply{Term: n.currentTerm, From: n.id}
	if m.Term < n.currentTerm {
		n.net.Send(n.id, from, reply)
		return
	}
	if m.Term > n.currentTerm || n.state != Follower {
		n.becomeFollower(m.Term)
	} else {
		n.resetElectionTimer()
	}
	reply.Term = n.currentTerm

	if m.PrevLogIndex < n.baseIndex() {
		reply.Success = true
		reply.MatchIndex = n.baseIndex()
		n.net.Send(n.id, from, reply)
		return
	}
	if m.PrevLogIndex > n.lastIndex() {
		reply.ConflictIndex = n.lastIndex() + 1
		reply.ConflictTerm = -1
		n.net.Send(n.id, from, reply)
		return
	}
	if t, _ := n.termAt(m.PrevLogIndex); t != m.PrevLogTerm {
		conflictTerm := t
		i := m.PrevLogIndex
		for i > n.baseIndex() {
			pt, ok := n.termAt(i - 1)
			if !ok || pt != conflictTerm {
				break
			}
			i--
		}
		reply.ConflictTerm = conflictTerm
		reply.ConflictIndex = i
		n.net.Send(n.id, from, reply)
		return
	}

	insertAt := m.PrevLogIndex + 1
	for i, e := range m.Entries {
		index := insertAt + i
		if n.hasIndex(index) {
			if t, _ := n.termAt(index); t != e.Term {
				n.truncateFrom(index)
				n.log = append(n.log, e)
				n.applyConfigChangeIfAny(e)
			}
			continue
		}
		n.log = append(n.log, e)
		n.applyConfigChangeIfAny(e)
	}

	if m.LeaderCommit > n.commitIndex {
		lastNew := n.lastIndex()
		if m.LeaderCommit < lastNew {
			n.commitIndex = m.LeaderCommit
		} else {
			n.commitIndex = lastNew
		}
		n.applyCommitted()
	}

	reply.Success = true
	reply.MatchIndex = n.lastIndex()
	n.net.Send(n.id, from, reply)
}

func (n *Node) onAppendEntriesReply(m AppendEntriesReply) {
	if m.Term > n.currentTerm {
		n.becomeFollower(m.Term)
		return
	}
	if n.state != Leader || m.Term != n.currentTerm {
		return
	}
	if m.Success {
		if m.MatchIndex > n.matchIndex[m.From] {
			n.matchIndex[m.From] = m.MatchIndex
			n.nextIndex[m.From] = m.MatchIndex + 1
		}
		n.advanceCommitIndex()
		return
	}
	if m.ConflictTerm == -1 {
		n.nextIndex[m.From] = m.ConflictIndex
		return
	}
	newNext := m.ConflictIndex
	for idx := n.lastIndex(); idx >= n.baseIndex(); idx-- {
		if t, ok := n.termAt(idx); ok && t == m.ConflictTerm {
			newNext = idx + 1
			break
		}
	}
	if newNext < 1 {
		newNext = 1
	}
	n.nextIndex[m.From] = newNext
}

func (n *Node) onInstallSnapshot(from detsim.NodeID, m InstallSnapshot) {
	reply := InstallSnapshotReply{Term: n.currentTerm, From: n.id}
	if m.Term < n.currentTerm {
		n.net.Send(n.id, from, reply)
		return
	}
	if m.Term > n.currentTerm || n.state != Follower {
		n.becomeFollower(m.Term)
	} else {
		n.resetElectionTimer()
	}
	reply.Term = n.currentTerm

	if m.LastIncludedIndex > n.baseIndex() {
		n.log = []LogEntry{{Term: m.LastIncludedTerm, Index: m.LastIncludedIndex}}
		if m.LastIncludedIndex > n.commitIndex {
			n.commitIndex = m.LastIncludedIndex
		}
		if m.LastIncludedIndex > n.lastApplied {
			n.lastApplied = m.LastIncludedIndex
		}
	}

	n.net.Send(n.id, from, reply)
}

func (n *Node) onInstallSnapshotReply(m InstallSnapshotReply) {
	if m.Term > n.currentTerm {
		n.becomeFollower(m.Term)
		return
	}
	if n.state != Leader {
		return
	}
	if n.baseIndex() > n.matchIndex[m.From] {
		n.matchIndex[m.From] = n.baseIndex()
	}
	n.nextIndex[m.From] = n.baseIndex() + 1
}
