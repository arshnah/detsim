package raft

import "github.com/arshnah/detsim"

// ProposeAddNode appends a config-change entry to add id to the cluster.
// Returns the log index and true on success, or (0, false) if this node is not the leader.
func (n *Node) ProposeAddNode(id detsim.NodeID) (index int, ok bool) {
	return n.proposeConfigChange(ConfigChange{Add: true, NodeID: id})
}

// ProposeRemoveNode appends a config-change entry to remove id from the cluster.
// Returns the log index and true on success, or (0, false) if this node is not the leader.
func (n *Node) ProposeRemoveNode(id detsim.NodeID) (index int, ok bool) {
	return n.proposeConfigChange(ConfigChange{Add: false, NodeID: id})
}

func (n *Node) proposeConfigChange(cc ConfigChange) (index int, ok bool) {
	if n.state != Leader {
		return 0, false
	}
	idx := n.lastIndex() + 1
	entry := LogEntry{Term: n.currentTerm, Index: idx, ConfigChange: &cc}
	n.log = append(n.log, entry)
	n.applyConfigChangeIfAny(entry)
	return idx, true
}

// Peers returns a copy of the current peer list.
func (n *Node) Peers() []detsim.NodeID {
	out := make([]detsim.NodeID, len(n.peers))
	copy(out, n.peers)
	return out
}

func (n *Node) applyConfigChangeIfAny(e LogEntry) {
	if e.ConfigChange == nil {
		return
	}
	cc := e.ConfigChange
	if cc.NodeID == n.id {
		return
	}

	if cc.Add {
		for _, p := range n.peers {
			if p == cc.NodeID {
				return
			}
		}
		n.peers = append(n.peers, cc.NodeID)
		if n.state == Leader {
			if n.nextIndex == nil {
				n.nextIndex = make(map[detsim.NodeID]int)
			}
			if n.matchIndex == nil {
				n.matchIndex = make(map[detsim.NodeID]int)
			}
			n.nextIndex[cc.NodeID] = n.lastIndex() + 1
			n.matchIndex[cc.NodeID] = 0
		}
		return
	}

	remaining := n.peers[:0]
	for _, p := range n.peers {
		if p != cc.NodeID {
			remaining = append(remaining, p)
		}
	}
	n.peers = remaining
	delete(n.nextIndex, cc.NodeID)
	delete(n.matchIndex, cc.NodeID)
}
