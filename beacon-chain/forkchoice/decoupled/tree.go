package decoupled

import (
	"errors"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// Spec: Store.init and ac_restart. Genesis carries the initial chain state, so h_max starts at 1. now is the restart time the joined rule is measured from.
func New(cfg Config, genesis time.Time, genesisRoot [32]byte, now time.Time) (*Store, error) {
	if cfg.Delta <= 0 {
		return nil, errors.New("delta must be positive")
	}
	if cfg.LockStreak < 2 {
		return nil, errors.New("lock streak must be at least 2")
	}
	if cfg.CommitteeSize == 0 {
		return nil, errors.New("committee size must be positive")
	}
	g := &Node{
		Root: genesisRoot,
		FG: FGSummary{
			Target:    Pair{Height: 1, Root: genesisRoot},
			Justified: Pair{Root: genesisRoot},
			Finalized: Pair{Root: genesisRoot},
		},
	}
	return &Store{
		cfg:             cfg,
		genesis:         genesis,
		genesisRoot:     genesisRoot,
		restartTime:     now,
		root:            g,
		nodes:           map[[32]byte]*Node{genesisRoot: g},
		bySlot:          map[primitives.Slot][]*Node{0: {g}},
		finalized:       g,
		justified:       g,
		hMax:            1,
		pool:            make(map[primitives.Slot]*bucket),
		saved:           make(map[primitives.Slot]*Node),
		latestConfirmed: g,
	}, nil
}

func (st *Store) Node(root [32]byte) *Node { return st.nodes[root] }

func (st *Store) HasNode(root [32]byte) bool { return st.nodes[root] != nil }

func (st *Store) addChild(root [32]byte, parent *Node, slot primitives.Slot) *Node {
	n := &Node{Root: root, Slot: slot, Parent: parent}
	parent.Children = append(parent.Children, n)
	st.nodes[root] = n
	st.bySlot[slot] = append(st.bySlot[slot], n)
	return n
}

// Nodes are removed from the maps at finalization, so map membership is membership in Φ.tree.
func (st *Store) inTree(n *Node) bool { return n != nil && st.nodes[n.Root] == n }

// Spec: Block.preceq, a ⪯ b. Parents have strictly lower slots, so the walk stops at a's slot.
func isAncestor(a, b *Node) bool {
	for b != nil && b.Slot > a.Slot {
		b = b.Parent
	}
	return a == b
}

func (st *Store) IsAncestor(ancestor, descendant [32]byte) bool {
	a, b := st.nodes[ancestor], st.nodes[descendant]
	return a != nil && b != nil && isAncestor(a, b)
}

func commonAncestor(a, b *Node) *Node {
	for a != b {
		if a.Slot > b.Slot {
			a = a.Parent
		} else {
			b = b.Parent
		}
	}
	return a
}

// Spec: deepest, the most parent edges from genesis, ties by the greatest root.
func deeper(a, b *Node) bool {
	da, db := depth(a), depth(b)
	return da > db || (da == db && greaterRoot(a, b))
}

func depth(n *Node) int {
	d := 0
	for ; n.Parent != nil; n = n.Parent {
		d++
	}
	return d
}

// Spec: Φ.tree shrinks to the descendants of F, nothing below F is read again.
func (st *Store) prune() {
	f := st.finalized
	f.Parent = nil
	st.root = f
	kept := make(map[[32]byte]*Node, len(st.nodes))
	bySlot := make(map[primitives.Slot][]*Node)
	stack := []*Node{f}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		kept[n.Root] = n
		bySlot[n.Slot] = append(bySlot[n.Slot], n)
		stack = append(stack, n.Children...)
	}
	st.nodes, st.bySlot = kept, bySlot
}
