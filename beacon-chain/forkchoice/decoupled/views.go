package decoupled

import "github.com/OffchainLabs/prysm/v7/consensus-types/primitives"

// Spec: the (votes, support_votes) pair a duty reads. Participants set the denominator and the equivocation test, supported votes count for a block.
type view struct {
	participants []Vote
	supported    []Vote
}

func dedupe(votes []Vote) []Vote {
	seen := make(map[Vote]struct{}, len(votes))
	out := make([]Vote, 0, len(votes))
	for _, v := range votes {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// Spec: goldfish_vote's views for the slot-s vote. Block votes bypass the support cutoff, support comes only from the proposer's support list with the root resolving here.
func (st *Store) voterView(s primitives.Slot) view {
	if s == 0 {
		return view{}
	}
	prev := s - 1
	cutoff := st.supportCutoff(prev)
	var v view
	if b := st.pool[prev]; b != nil {
		for i, u := range b.votes {
			if b.at[i].Before(cutoff) {
				v.participants = append(v.participants, u)
			}
			if at, ok := st.resolutionTime(u); ok && at.Before(cutoff) {
				v.supported = append(v.supported, u)
			}
		}
	}
	for _, p := range st.bySlot[s] {
		for _, u := range p.BlockVotes {
			if u.Slot == prev {
				v.participants = append(v.participants, u)
			}
		}
		for _, u := range p.SupportVotes {
			if u.Slot == prev && st.resolve(u) != nil {
				v.supported = append(v.supported, u)
			}
		}
	}
	v.participants = dedupe(v.participants)
	v.supported = dedupe(v.supported)
	return v
}

// Spec: voter_block_tree. Blocks processed before the previous support cutoff plus every ancestor of a slot-s proposal.
func (st *Store) voterBlocks(s primitives.Slot) map[*Node]bool {
	in := make(map[*Node]bool, len(st.nodes))
	if s == 0 {
		return in
	}
	cutoff := st.supportCutoff(s - 1)
	for _, n := range st.nodes {
		if n.processed.Before(cutoff) {
			in[n] = true
		}
	}
	for _, p := range st.bySlot[s] {
		for n := p; n != nil && !in[n]; n = n.Parent {
			in[n] = true
		}
	}
	return in
}

// Spec: every held vote of a slot with the subset that resolves now, the view of ac_proposal, ac_to_fg and ac_head.
func (st *Store) heldView(s primitives.Slot) view {
	v := view{participants: st.votes(s)}
	for _, u := range v.participants {
		if st.resolve(u) != nil {
			v.supported = append(v.supported, u)
		}
	}
	return v
}
