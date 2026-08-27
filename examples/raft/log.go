package raft

func (n *Node) baseIndex() int { return n.log[0].Index }
func (n *Node) lastIndex() int { return n.log[len(n.log)-1].Index }
func (n *Node) lastTerm() int  { return n.log[len(n.log)-1].Term }

func (n *Node) pos(index int) int { return index - n.baseIndex() }

func (n *Node) hasIndex(index int) bool {
	p := n.pos(index)
	return p >= 0 && p < len(n.log)
}

func (n *Node) entryAt(index int) LogEntry { return n.log[n.pos(index)] }

func (n *Node) termAt(index int) (int, bool) {
	if !n.hasIndex(index) {
		return 0, false
	}
	return n.entryAt(index).Term, true
}

func (n *Node) entriesFrom(index int) []LogEntry {
	p := n.pos(index)
	if p < 0 {
		p = 0
	}
	if p >= len(n.log) {
		return nil
	}
	return n.log[p:]
}

func (n *Node) truncateFrom(index int) {
	p := n.pos(index)
	if p < 0 {
		p = 0
	}
	if p < len(n.log) {
		n.log = n.log[:p]
	}
}
