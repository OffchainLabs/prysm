package decoupled

import "github.com/OffchainLabs/prysm/v7/consensus-types/primitives"

// Spec: the FG frame Φ. underRoot is nil when the root is F, since every node in the tree extends F.
type frame struct {
	root      *Node
	floor     primitives.Height
	underRoot map[*Node]bool
}

// Spec: get_fc_root, head_height_floor and fg_contest in one pass over the tree, h_max - 1 truncated at zero.
func (st *Store) frame() frame {
	floor := st.hMax
	if floor > 0 {
		floor--
	}
	if st.hMax != st.hJ+1 {
		return frame{root: st.finalized, floor: floor}
	}
	underJ := make(map[*Node]bool, len(st.nodes))
	contested := false
	var walk func(n *Node, under bool)
	walk = func(n *Node, under bool) {
		under = under || n == st.justified
		underJ[n] = under
		if !under && n.FG.Target.Height == st.hMax {
			contested = true
		}
		for _, c := range n.Children {
			walk(c, under)
		}
	}
	walk(st.root, false)
	if contested {
		return frame{root: st.finalized, floor: st.hMax}
	}
	return frame{root: st.justified, floor: floor, underRoot: underJ}
}

// Spec: Φ.viable_heads, extends the fork choice root and reaches the floor.
func (fr frame) viableHead(n *Node) bool {
	return n.FG.Target.Height >= fr.floor && (fr.underRoot == nil || fr.underRoot[n])
}

// Spec: has_viable_descendant for finality acceptance, the floor is the only test.
func (st *Store) hasViableDescendant(b *Node) bool {
	floor := st.frame().floor
	stack := []*Node{b}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n.FG.Target.Height >= floor {
			return true
		}
		stack = append(stack, n.Children...)
	}
	return false
}

// Spec: viable_tree over a view, blocks under the root with a viable head among their descendants inside the view. One postorder pass, views are ancestor closed so an excluded subtree holds no witness.
func (st *Store) viableTree(fr frame, in func(*Node) bool) func(*Node) bool {
	viable := make(map[*Node]bool, len(st.nodes))
	var walk func(n *Node) bool
	walk = func(n *Node) bool {
		if !in(n) {
			return false
		}
		v := fr.viableHead(n)
		for _, c := range n.Children {
			v = walk(c) || v
		}
		if fr.underRoot != nil && !fr.underRoot[n] {
			v = false
		}
		viable[n] = v
		return v
	}
	walk(st.root)
	return func(n *Node) bool { return viable[n] }
}

func everyNode(*Node) bool { return true }

// Spec: goldfish_eligible. The majority gate applies only once the walk stands on a viable head, and a current-slot block is always eligible. Pre-fork blocks had no votes to win, so they pass too.
func (fr frame) eligible(c *supportCounts, current, fork primitives.Slot) func(*Node) bool {
	return func(b *Node) bool {
		return b.Parent == nil || !fr.viableHead(b.Parent) || b.Slot == current || b.Slot < fork || c.hasMajority(b)
	}
}
