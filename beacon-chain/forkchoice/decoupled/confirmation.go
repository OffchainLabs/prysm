package decoupled

import (
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// Spec: confirmation_score and voters_count as of the evaluation time. Stamps are exact, so a late evaluation reads the same view an on-time one would.
func (st *Store) confirmationCounts(slot primitives.Slot) *supportCounts {
	c := &supportCounts{subtree: make(map[*Node]int)}
	b := st.pool[slot]
	if b == nil {
		return c
	}
	votesBy := b.votesBefore(st.evaluationTime(slot))
	c.participants = len(votesBy)
	cutoff := st.confirmationCutoff(slot)
	for _, u := range b.votes {
		if votesBy[u.Validator] != 1 {
			continue
		}
		if at, ok := st.resolutionTime(u); !ok || !at.Before(cutoff) {
			continue
		}
		for n := st.resolve(u); n != nil; n = n.Parent {
			c.subtree[n]++
		}
	}
	return c
}

// Spec: get_confirmation, a walk from F over the whole tree gated by a strict majority, nil when even the result lacks one.
func (st *Store) confirm(slot primitives.Slot) *Node {
	c := st.confirmationCounts(slot)
	h := ghost(st.finalized, everyNode, c.score, c.hasMajority)
	if !c.hasMajority(h) {
		return nil
	}
	return h
}

// Spec: advance. An ancestor candidate leaves the record, an extension or a conflict replaces it.
func (st *Store) advance(candidate *Node) {
	if st.inTree(st.latestConfirmed) && isAncestor(candidate, st.latestConfirmed) {
		return
	}
	st.latestConfirmed = candidate
}

// Spec: update_confirmation, run at the evaluation time of slot. Saves the result before the lock veto, then locks the deepest block on the result's chain that passes the fast or slow test. Unjoined for the slot, it saves no result, so the fast set stays empty and the streak breaks.
func (st *Store) updateConfirmation(slot primitives.Slot, now time.Time) {
	st.fastConfirmed = st.fastConfirmed[:0]
	if st.restartTime.After(st.slotStart(slot)) {
		st.saved[slot] = nil
		return
	}
	candidate := st.confirm(slot)
	st.saved[slot] = candidate
	if candidate == nil || st.vetoed(candidate, now) {
		return
	}
	st.advance(candidate)
	c := st.confirmationCounts(slot)
	streak := st.streakAncestor(candidate, slot)
	slow := false
	var locked *Node
	for n := candidate; n != nil && n.Root != st.genesisRoot; n = n.Parent {
		fast := 2*uint64(c.score(n)) >= st.cfg.CommitteeSize
		if fast {
			st.fastConfirmed = append(st.fastConfirmed, n)
		}
		slow = slow || n == streak
		if locked == nil && (fast || slow) {
			locked = n
		}
	}
	if locked != nil {
		st.locks = append(st.locks, lock{block: locked, deadline: st.unlockTime(slot + primitives.Slot(st.cfg.LockStreak))})
	}
}

// Spec: the slow test. The deepest block every saved confirmation of the previous k-1 slots extends is their common ancestor with the candidate, so the test holds from that block up the chain. Nil when a slot in the streak has no result in the tree.
func (st *Store) streakAncestor(candidate *Node, slot primitives.Slot) *Node {
	streak := candidate
	for i := primitives.Slot(1); i < primitives.Slot(st.cfg.LockStreak); i++ {
		if i > slot {
			return nil
		}
		c := st.saved[slot-i]
		if !st.inTree(c) {
			return nil
		}
		streak = commonAncestor(streak, c)
	}
	return streak
}

// Spec: latest_confirmed, the record no voting rule reads.
func (st *Store) LatestConfirmed() *Node { return st.latestConfirmed }

// Spec: ac_confirmed, the record clamped to the head's chain so the confirmed tip never leads the head.
func (st *Store) Confirmed(now time.Time) *Node {
	c := st.latestConfirmed
	if !st.inTree(c) {
		c = st.finalized
	}
	return commonAncestor(c, st.Head(now))
}
