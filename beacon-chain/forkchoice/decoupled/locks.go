package decoupled

import (
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// Spec: a live lock has not expired and its block is in Φ.tree.
func (st *Store) live(l lock, now time.Time) bool {
	return l.deadline.After(now) && st.inTree(l.block)
}

// Spec: a live lock vetoes a result that conflicts with its block.
func (st *Store) vetoed(b *Node, now time.Time) bool {
	for _, l := range st.locks {
		if st.live(l, now) && !isAncestor(l.block, b) && !isAncestor(b, l.block) {
			return true
		}
	}
	return false
}

// Spec: get_anchor, the deepest locked block of the viable tree, else the fork choice root. A block is locked when a live lock's block extends it.
func (st *Store) anchor(fr frame, now time.Time) *Node {
	viable := st.viableTree(fr, everyNode)
	var anchor *Node
	for _, l := range st.locks {
		if !st.live(l, now) {
			continue
		}
		for n := l.block; n != nil; n = n.Parent {
			if viable(n) {
				if anchor == nil || deeper(n, anchor) {
					anchor = n
				}
				break
			}
		}
	}
	if anchor == nil {
		return fr.root
	}
	return anchor
}

// Spec: unlock against the slot's votes as they stood at the unlock time, so a late tick gives the scheduled result. Each live lock moves back to the deepest ancestor fewer than half the committee oppose, keeping its deadline. Expired locks are dropped.
func (st *Store) unlock(slot primitives.Slot, now time.Time) {
	kept := st.locks[:0]
	for _, l := range st.locks {
		if st.live(l, now) {
			kept = append(kept, lock{block: st.retreat(l.block, st.pool[slot], st.unlockTime(slot)), deadline: l.deadline})
		}
	}
	st.locks = kept
}

// A clean vote opposes exactly the ancestors of the locked block strictly below its fork point with that chain, so counting votes at their fork point once gives every ancestor's opponents in one walk up.
func (st *Store) retreat(locked *Node, b *bucket, asOf time.Time) *Node {
	total := uint64(0)
	forkAt := make(map[*Node]uint64)
	if b != nil {
		votesBy := b.votesBefore(asOf)
		for _, u := range b.votes {
			if votesBy[u.Validator] != 1 {
				continue
			}
			at, ok := st.resolutionTime(u)
			if !ok || !at.Before(asOf) {
				continue
			}
			if r := st.resolve(u); commonAncestor(locked, r) != r {
				forkAt[commonAncestor(locked, r)]++
				total++
			}
		}
	}
	above := total
	for n := locked; ; n = n.Parent {
		above -= forkAt[n]
		if 2*above < st.cfg.CommitteeSize || n.Parent == nil {
			return n
		}
	}
}
