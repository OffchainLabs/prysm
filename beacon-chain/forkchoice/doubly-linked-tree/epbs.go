package doublylinkedtree

func (n *Node) isParentFull() bool {
	// Finalized checkpoint is considered full
	if n.parent == nil || n.parent.parent == nil {
		return true
	}
	return n.parent.payloadHash != [32]byte{}
}
