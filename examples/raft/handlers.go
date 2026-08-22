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
	lastIdx, lastTerm := n.lastLogInfo()
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

	if m.PrevLogIndex >= len(n.log) {
		reply.ConflictIndex = n.log[len(n.log)-1].Index + 1
		reply.ConflictTerm = -1
		n.net.Send(n.id, from, reply)
		return
	}
	if m.PrevLogIndex >= 0 && n.log[m.PrevLogIndex].Term != m.PrevLogTerm {
		conflictTerm := n.log[m.PrevLogIndex].Term
		i := m.PrevLogIndex
		for i > 0 && n.log[i-1].Term == conflictTerm {
			i--
		}
		reply.ConflictTerm = conflictTerm
		reply.ConflictIndex = n.log[i].Index
		n.net.Send(n.id, from, reply)
		return
	}

	insertAt := m.PrevLogIndex + 1
	for i, e := range m.Entries {
		pos := insertAt + i
		if pos < len(n.log) {
			if n.log[pos].Term != e.Term {
				n.log = append(n.log[:pos], e)
			}
			continue
		}
		n.log = append(n.log, e)
	}

	if m.LeaderCommit > n.commitIndex {
		lastNew := n.log[len(n.log)-1].Index
		if m.LeaderCommit < lastNew {
			n.commitIndex = m.LeaderCommit
		} else {
			n.commitIndex = lastNew
		}
		n.applyCommitted()
	}

	reply.Success = true
	reply.MatchIndex = n.log[len(n.log)-1].Index
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
	for i := len(n.log) - 1; i >= 0; i-- {
		if n.log[i].Term == m.ConflictTerm {
			newNext = n.log[i].Index + 1
			break
		}
	}
	if newNext < 1 {
		newNext = 1
	}
	n.nextIndex[m.From] = newNext
}
