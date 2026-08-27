package raft

import "github.com/arshnah/detsim"

const snapshotThreshold = 50

func (n *Node) maybeCompact() {
	if len(n.log) > snapshotThreshold {
		n.CompactLog(n.lastApplied)
	}
}

// CompactLog truncates the log up to and including upToIndex, replacing the
// removed entries with a single sentinel entry that records the compaction boundary.
func (n *Node) CompactLog(upToIndex int) {
	if upToIndex > n.commitIndex {
		upToIndex = n.commitIndex
	}
	if upToIndex <= n.baseIndex() || !n.hasIndex(upToIndex) {
		return
	}
	boundary := n.entryAt(upToIndex)
	rest := append([]LogEntry(nil), n.entriesFrom(upToIndex+1)...)
	n.log = append([]LogEntry{{Term: boundary.Term, Index: boundary.Index}}, rest...)
}

func (n *Node) sendInstallSnapshot(peer detsim.NodeID) {
	base := n.log[0]
	n.net.Send(n.id, peer, InstallSnapshot{
		Term:              n.currentTerm,
		LeaderID:          n.id,
		LastIncludedIndex: base.Index,
		LastIncludedTerm:  base.Term,
	})
}
