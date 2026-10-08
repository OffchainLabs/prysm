package decoupled

import (
	"slices"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// Spec: goldfish_fork_choice from get_anchor over the viable tree inside the given block view.
func (st *Store) selectHead(in func(*Node) bool, v view, voteSlot primitives.Slot, now time.Time) *Node {
	fr := st.frame()
	counts := st.countSupport(v, voteSlot)
	return ghost(st.anchor(fr, now), st.viableTree(fr, in), counts.score, fr.eligible(counts, st.slotAt(now)))
}

// Spec: ac_proposal. The parent and both vote lists come from one evaluation of the previous slot's held votes. A nil parent means no proposal.
func (st *Store) Proposal(slot primitives.Slot, now time.Time) (parent *Node, votes, support []Vote) {
	if !st.joined(slot) {
		return nil, nil, nil
	}
	v := st.heldView(slot - 1)
	return st.selectHead(everyNode, v, slot-1, now), slices.Clone(v.participants), v.supported
}

// Spec: goldfish_vote. Only the vote duty restricts the walk to the voter's block view. Nil means no vote.
func (st *Store) VoteHead(slot primitives.Slot, now time.Time) *Node {
	if !st.joined(slot) {
		return nil
	}
	in := st.voterBlocks(slot)
	return st.selectHead(func(n *Node) bool { return in[n] }, st.voterView(slot), slot-1, now)
}

// Spec: ac_head. Slot s-1 votes before the confirmation cutoff of slot s, slot s votes from then on.
func (st *Store) Head(now time.Time) *Node {
	s := st.slotAt(now)
	if s > 0 && now.Before(st.confirmationCutoff(s)) {
		s--
	}
	return st.selectHead(everyNode, st.heldView(s), s, now)
}

// Spec: ac_to_fg. fastConfirmed was filled walking up from the confirmed block, so the first entry still in the viable tree is the deepest.
func (st *Store) Source(now time.Time) (source, head *Node) {
	fr := st.frame()
	viable := st.viableTree(fr, everyNode)
	for _, n := range st.fastConfirmed {
		if st.inTree(n) && viable(n) {
			source = n
			break
		}
	}
	s := st.slotAt(now)
	return source, st.selectHead(everyNode, st.heldView(s), s, now)
}
