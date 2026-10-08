package decoupled

import (
	"bytes"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// Spec: goldfish_score and voters_count, recomputed from one view. subtree[B] is the clean supporters of B's descendants including B.
type supportCounts struct {
	equivocators int
	participants int
	subtree      map[*Node]int
}

// Each clean supporter walks from its resolved block up to the root once, so subtree totals cost votes times depth, not a tree traversal per block.
func (st *Store) countSupport(v view, s primitives.Slot) *supportCounts {
	c := &supportCounts{subtree: make(map[*Node]int)}
	votesBy := make(map[primitives.ValidatorIndex]int)
	for _, u := range v.participants {
		votesBy[u.Validator]++
	}
	c.participants = len(votesBy)
	for _, n := range votesBy {
		if n >= 2 {
			c.equivocators++
		}
	}
	for _, u := range v.supported {
		if u.Slot != s || votesBy[u.Validator] >= 2 {
			continue
		}
		for n := st.resolve(u); n != nil; n = n.Parent {
			c.subtree[n]++
		}
	}
	return c
}

// Spec: an equivocator counts for every block, so it can neither create nor block a descent.
func (c *supportCounts) score(b *Node) int { return c.equivocators + c.subtree[b] }

// Spec: the relative-majority gate, voters_count < 2 * goldfish_score.
func (c *supportCounts) hasMajority(b *Node) bool { return c.participants < 2*c.score(b) }

func greaterRoot(a, b *Node) bool { return bytes.Compare(a.Root[:], b.Root[:]) > 0 }

// Spec: ghost. Membership is tested on the child only, so the anchor may sit outside the tree it walks.
func ghost(anchor *Node, in func(*Node) bool, score func(*Node) int, eligible func(*Node) bool) *Node {
	h := anchor
	for {
		var best *Node
		for _, c := range h.Children {
			if !in(c) || !eligible(c) {
				continue
			}
			if best == nil || score(c) > score(best) || (score(c) == score(best) && greaterRoot(c, best)) {
				best = c
			}
		}
		if best == nil {
			return h
		}
		h = best
	}
}
