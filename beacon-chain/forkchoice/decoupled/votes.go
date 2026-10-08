package decoupled

import (
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

func (st *Store) votes(slot primitives.Slot) []Vote {
	if b := st.pool[slot]; b != nil {
		return b.votes
	}
	return nil
}

// Distinct votes per validator received before the instant, the pool as a scheduled action saw it.
func (b *bucket) votesBefore(instant time.Time) map[primitives.ValidatorIndex]int {
	seen := make(map[primitives.ValidatorIndex]int, len(b.votesBy))
	for i, u := range b.votes {
		if b.at[i].Before(instant) {
			seen[u.Validator]++
		}
	}
	return seen
}

func (st *Store) receivedAt(v Vote) (time.Time, bool) {
	b := st.pool[v.Slot]
	if b == nil {
		return time.Time{}, false
	}
	i, ok := b.index[v]
	if !ok {
		return time.Time{}, false
	}
	return b.at[i], true
}

// Votes are stamped on receipt, so the gossip validator must call this directly instead of parking votes in the attestation pool. Due ticks run first.
func (st *Store) ProcessVote(validators []primitives.ValidatorIndex, root [32]byte, slot primitives.Slot, now time.Time) {
	st.OnTick(now)
	for _, val := range validators {
		st.addVote(Vote{Validator: val, Slot: slot, Root: root}, now)
	}
}

// Spec: on_goldfish_vote. The slot window is [s-1, s], duplicates and a third distinct vote are dropped unstamped.
func (st *Store) addVote(v Vote, now time.Time) {
	current := st.slotAt(now)
	if v.Slot+1 < current || current < v.Slot {
		return
	}
	b := st.pool[v.Slot]
	if b == nil {
		b = &bucket{index: make(map[Vote]int), votesBy: make(map[primitives.ValidatorIndex]int)}
		st.pool[v.Slot] = b
	}
	if _, dup := b.index[v]; dup || b.votesBy[v.Validator] >= 2 {
		return
	}
	b.index[v] = len(b.votes)
	b.votesBy[v.Validator]++
	b.votes = append(b.votes, v)
	b.at = append(b.at, now)
}

// Spec: resolved. A root from a later slot than the vote never resolves, nor does one that left Φ.tree.
func (st *Store) resolve(v Vote) *Node {
	n := st.nodes[v.Root]
	if n == nil || n.Slot > v.Slot {
		return nil
	}
	return n
}

// Spec: resolution_time, the later of the vote and block receipts, unresolved otherwise. Genesis has a zero receipt.
func (st *Store) resolutionTime(v Vote) (time.Time, bool) {
	at, ok := st.receivedAt(v)
	if !ok {
		return time.Time{}, false
	}
	n := st.resolve(v)
	if n == nil {
		return time.Time{}, false
	}
	if n.processed.After(at) {
		return n.processed, true
	}
	return at, true
}

func countVotes(votes []Vote, val primitives.ValidatorIndex) int {
	n := 0
	for _, v := range votes {
		if v.Validator == val {
			n++
		}
	}
	return n
}

// Spec: equivocates, two distinct votes by one validator in the set.
func equivocates(votes []Vote, val primitives.ValidatorIndex) bool {
	return countVotes(votes, val) >= 2
}

func participates(votes []Vote, val primitives.ValidatorIndex) bool {
	return countVotes(votes, val) >= 1
}
