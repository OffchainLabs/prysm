package decoupled

import (
	"errors"
	"slices"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// Spec: Store.init and ac_restart. The root is F with its own slot and target, its Finalized pair is the accepted finalization and seeds J, the genesis σ has target height 1 and F at height 0. now is the restart time the joined rule is measured from.
func New(cfg Config, genesis time.Time, root Block, now time.Time) (*Store, error) {
	if cfg.Delta <= 0 {
		return nil, errors.New("delta must be positive")
	}
	if cfg.LockStreak < 2 {
		return nil, errors.New("lock streak must be at least 2")
	}
	if cfg.CommitteeSize == 0 {
		return nil, errors.New("committee size must be positive")
	}
	g := &Node{Root: root.Root, Slot: root.Slot, FG: root.FG}
	return &Store{
		cfg:             cfg,
		genesis:         genesis,
		genesisRoot:     root.Root,
		restartTime:     now,
		root:            g,
		nodes:           map[[32]byte]*Node{root.Root: g},
		bySlot:          map[primitives.Slot][]*Node{root.Slot: {g}},
		finalized:       g,
		justified:       g,
		hJ:              root.FG.Finalized.Height,
		hF:              root.FG.Finalized.Height,
		hMax:            root.FG.Target.Height,
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

// Blocks the execution layer invalidated leave the tree with their subtrees. F cannot leave. h_max, J and h_j are rebuilt from the survivors with J seeded at F, since F's own state names an older justification and the removed blocks may have carried the evidence.
func (st *Store) Remove(roots [][32]byte) error {
	for _, r := range roots {
		n := st.nodes[r]
		if n == nil {
			continue
		}
		if n == st.finalized {
			return errors.New("cannot remove the finalized block")
		}
		if p := n.Parent; p != nil {
			p.Children = slices.DeleteFunc(p.Children, func(c *Node) bool { return c == n })
		}
		st.drop(n)
	}
	st.justified, st.hJ, st.hMax = st.finalized, st.hF, st.finalized.FG.Target.Height
	for _, n := range st.nodes {
		st.hMax = max(st.hMax, n.FG.Target.Height)
		if j := st.nodes[n.FG.Justified.Root]; j != nil && (Pair{Height: st.hJ, Root: st.justified.Root}).less(n.FG.Justified) {
			st.justified, st.hJ = j, n.FG.Justified.Height
		}
	}
	return nil
}

func (st *Store) drop(n *Node) {
	delete(st.nodes, n.Root)
	st.bySlot[n.Slot] = slices.DeleteFunc(st.bySlot[n.Slot], func(c *Node) bool { return c == n })
	for _, c := range n.Children {
		st.drop(c)
	}
}

// The block of the chain through root at the greatest slot not above slot, false when root is unknown or the chain leaves the tree above slot.
func (st *Store) AncestorAtSlot(root [32]byte, slot primitives.Slot) ([32]byte, bool) {
	n := st.nodes[root]
	for n != nil && n.Slot > slot {
		n = n.Parent
	}
	if n == nil {
		return [32]byte{}, false
	}
	return n.Root, true
}
