package testing

import (
	"github.com/ethereum/go-ethereum/p2p/enode"
)

// MockListener is a mock implementation of the Listener and ListenerRebooter interfaces
// that can be used in tests. It provides configurable behavior for all methods.
type MockListener struct {
	LocalNodeFunc   func() *enode.LocalNode
	SelfFunc        func() *enode.Node
	RandomNodesFunc func() enode.Iterator
	LookupFunc      func(enode.ID) []*enode.Node
	ResolveFunc     func(*enode.Node) *enode.Node
	PingFunc        func(*enode.Node) error
	RequestENRFunc  func(*enode.Node) (*enode.Node, error)
	RebootFunc      func() error
	CloseFunc       func()

	// Default implementations
	localNode *enode.LocalNode
	iterator  enode.Iterator
}

// NewMockListener creates a new MockListener with default implementations
func NewMockListener(localNode *enode.LocalNode, iterator enode.Iterator) *MockListener {
	return &MockListener{
		localNode: localNode,
		iterator:  iterator,
	}
}

func (m *MockListener) LocalNode() *enode.LocalNode {
	if m.LocalNodeFunc != nil {
		return m.LocalNodeFunc()
	}
	return m.localNode
}

func (m *MockListener) Self() *enode.Node {
	if m.SelfFunc != nil {
		return m.SelfFunc()
	}
	if m.localNode != nil {
		return m.localNode.Node()
	}
	return nil
}

func (m *MockListener) RandomNodes() enode.Iterator {
	if m.RandomNodesFunc != nil {
		return m.RandomNodesFunc()
	}
	return m.iterator
}

func (m *MockListener) Lookup(id enode.ID) []*enode.Node {
	if m.LookupFunc != nil {
		return m.LookupFunc(id)
	}
	return nil
}

func (m *MockListener) Resolve(node *enode.Node) *enode.Node {
	if m.ResolveFunc != nil {
		return m.ResolveFunc(node)
	}
	return nil
}

func (m *MockListener) Ping(node *enode.Node) error {
	if m.PingFunc != nil {
		return m.PingFunc(node)
	}
	return nil
}

func (m *MockListener) RequestENR(node *enode.Node) (*enode.Node, error) {
	if m.RequestENRFunc != nil {
		return m.RequestENRFunc(node)
	}
	return nil, nil
}

func (m *MockListener) RebootListener() error {
	if m.RebootFunc != nil {
		return m.RebootFunc()
	}
	return nil
}

func (m *MockListener) Close() {
	if m.CloseFunc != nil {
		m.CloseFunc()
	}
}

// MockIterator is a mock implementation of enode.Iterator for testing
type MockIterator struct {
	Nodes    []*enode.Node
	Position int
	Closed   bool
}

func NewMockIterator(nodes []*enode.Node) *MockIterator {
	return &MockIterator{
		Nodes: nodes,
	}
}

func (m *MockIterator) Next() bool {
	if m.Closed || m.Position >= len(m.Nodes) {
		return false
	}
	m.Position++
	return true
}

func (m *MockIterator) Node() *enode.Node {
	if m.Position == 0 || m.Position > len(m.Nodes) {
		return nil
	}
	return m.Nodes[m.Position-1]
}

func (m *MockIterator) Close() {
	m.Closed = true
}
