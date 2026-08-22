package raft

import "github.com/arshnah/detsim"

type State int

const (
	Follower State = iota
	Candidate
	Leader
)

const (
	minElectionTimeout = 150
	maxElectionTimeout = 300
	heartbeatInterval  = 50
)

type LogEntry struct {
	Term    int
	Index   int
	Command string
}

type RequestVote struct {
	Term         int
	CandidateID  detsim.NodeID
	LastLogIndex int
	LastLogTerm  int
}

type RequestVoteReply struct {
	Term        int
	VoteGranted bool
	Voter       detsim.NodeID
}

type AppendEntries struct {
	Term         int
	LeaderID     detsim.NodeID
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry
	LeaderCommit int
}

type AppendEntriesReply struct {
	Term          int
	Success       bool
	ConflictIndex int
	ConflictTerm  int
	From          detsim.NodeID
	MatchIndex    int
}
