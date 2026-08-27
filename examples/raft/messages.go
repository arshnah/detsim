package raft

import "github.com/arshnah/detsim"

// State represents the three possible roles of a Raft node.
type State int

const (
	Follower  State = iota // Follower replicates log entries from the leader.
	Candidate              // Candidate is actively soliciting votes to become leader.
	Leader                 // Leader accepts client commands and replicates them to followers.
)

const (
	minElectionTimeout = 150
	maxElectionTimeout = 300
	heartbeatInterval  = 50
)

// LogEntry is one entry in a Raft node's replicated log.
type LogEntry struct {
	Term    int
	Index   int
	Command string

	ConfigChange *ConfigChange
}

// ConfigChange is a cluster membership mutation carried inside a LogEntry.
type ConfigChange struct {
	Add    bool
	NodeID detsim.NodeID
}

// RequestVote is sent by a Candidate to solicit votes from followers.
type RequestVote struct {
	Term         int
	CandidateID  detsim.NodeID
	LastLogIndex int
	LastLogTerm  int
}

// RequestVoteReply is the response to a RequestVote RPC.
type RequestVoteReply struct {
	Term        int
	VoteGranted bool
	Voter       detsim.NodeID
}

// AppendEntries is the heartbeat and log-replication RPC sent by the leader.
type AppendEntries struct {
	Term         int
	LeaderID     detsim.NodeID
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry
	LeaderCommit int
}

// AppendEntriesReply is the response to an AppendEntries RPC.
type AppendEntriesReply struct {
	Term          int
	Success       bool
	ConflictIndex int
	ConflictTerm  int
	From          detsim.NodeID
	MatchIndex    int
}

// InstallSnapshot is sent by the leader to bring a lagging follower up to date
// when the follower's next index has fallen behind the leader's compacted base.
type InstallSnapshot struct {
	Term              int
	LeaderID          detsim.NodeID
	LastIncludedIndex int
	LastIncludedTerm  int
}

// InstallSnapshotReply is the response to an InstallSnapshot RPC.
type InstallSnapshotReply struct {
	Term int
	From detsim.NodeID
}

// PingRequest is sent by the leader during a linearizable read to confirm it
// still has majority support.
type PingRequest struct {
	Term    int
	Leader  detsim.NodeID
	ReadSeq int
}

// PingReply is the response to a PingRequest during a linearizable read.
type PingReply struct {
	Term    int
	From    detsim.NodeID
	ReadSeq int
}
