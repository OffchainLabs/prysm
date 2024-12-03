package doublylinkedtree

// GetAttesterHead returns the attester head root given inclusion list satisfaction.
func (f *ForkChoice) GetAttesterHead() [32]byte {
	head := f.store.headNode
	if head == nil {
		return [32]byte{}
	}

	parent := head.parent
	if parent == nil {
		return head.root
	}
	if head.notSatisfyingInclusionList {
		return parent.root
	}
	return head.root
}
