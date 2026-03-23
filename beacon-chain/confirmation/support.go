package confirmation

import (
	"context"

	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// SupportMap pre-aggregates vote support for the slot-range queries FCR needs.
// slotRootVoters backs get_block_support_between_slots (direct vote equality),
// totalSupport backs get_attestation_score (a vote credits every ancestor).
type SupportMap struct {
	slotRootVoters map[primitives.Slot]map[[32]byte][]primitives.ValidatorIndex
	balances       []uint64
	totalSupport   map[[32]byte]uint64
}

func NewSupportMap() *SupportMap {
	return &SupportMap{
		slotRootVoters: make(map[primitives.Slot]map[[32]byte][]primitives.ValidatorIndex),
		totalSupport:   make(map[[32]byte]uint64),
	}
}

func (s *SupportMap) Build(
	ctx context.Context,
	votes []forkchoicetypes.VoteData,
	balances []uint64,
	committees CommitteeAccessor,
	equivocating map[primitives.ValidatorIndex]bool,
	currentSlot primitives.Slot,
	fc ForkchoiceReader,
) error {
	if currentSlot == 0 {
		return nil
	}
	// is_confirmed_chain_safe reconfirmation can query empty-slot discounts down to current_epoch - 2.
	currentEpoch := slots.ToEpoch(currentSlot)
	startSlot := primitives.Slot(0)
	if currentEpoch > 2 {
		es, err := slots.EpochStart(currentEpoch - 2)
		if err == nil {
			startSlot = es
		}
	}
	endSlot := currentSlot - 1

	// Reuse the map buckets across the per-slot rebuilds.
	clear(s.slotRootVoters)
	s.balances = balances

	for slot := startSlot; slot <= endSlot; slot++ {
		members, err := committees.Committee(ctx, slot)
		if err != nil {
			return err
		}

		slotMap := make(map[[32]byte][]primitives.ValidatorIndex)
		for _, idx := range members {
			if equivocating[idx] {
				continue
			}

			i := uint64(idx)
			if i < uint64(len(balances)) && i < uint64(len(votes)) && balances[i] > 0 {
				root := votes[i].Root
				if root != [32]byte{} {
					slotMap[root] = append(slotMap[root], idx)
				}
			}
		}
		if len(slotMap) > 0 {
			s.slotRootVoters[slot] = slotMap
		}
	}

	clear(s.totalSupport)
	globalVotes := make(map[[32]byte]uint64)
	for i, vote := range votes {
		if vote.Root == ([32]byte{}) {
			continue
		}
		if i >= len(balances) || balances[i] == 0 {
			continue
		}
		if equivocating[primitives.ValidatorIndex(i)] {
			continue
		}
		globalVotes[vote.Root] += balances[i]
	}
	for root, bal := range globalVotes {
		r := root
		for {
			s.totalSupport[r] += bal
			parent, err := fc.ParentRoot(r)
			if err != nil || parent == ([32]byte{}) {
				break
			}
			r = parent
		}
	}

	return nil
}

// BlockSupportBetweenSlots implements get_block_support_between_slots.
// The spec sums over a participant set union, so a validator sitting in committees
// of several slots in the range counts once.
//
//	<spec fn="get_block_support_between_slots" fork="phase0">
//
//	participants = set()
//	for slot in range(start_slot, end_slot + 1):
//	    participants.update(get_slot_committee(store, slot))
//	return sum(balance[i] for i in participants
//	    if latest_messages[i].root == block_root
//	    and i not in equivocating_indices)
//	</spec>
func (s *SupportMap) BlockSupportBetweenSlots(root [32]byte, startSlot, endSlot primitives.Slot) uint64 {
	total := uint64(0)
	seen := make(map[primitives.ValidatorIndex]bool)
	for slot := startSlot; slot <= endSlot; slot++ {
		for _, idx := range s.slotRootVoters[slot][root] {
			if seen[idx] {
				continue
			}
			seen[idx] = true
			if uint64(idx) < uint64(len(s.balances)) {
				total += s.balances[idx]
			}
		}
	}
	return total
}

// AttestationScore implements get_attestation_score, votes for descendants count as support.
//
//	<spec fn="get_attestation_score" fork="phase0">
//
//	return sum(balance[i] for i in active_unslashed
//	    if i in latest_messages and i not in equivocating_indices
//	    and get_ancestor(store, latest_messages[i].root, store.blocks[root].slot) == root)
//	</spec>
func (s *SupportMap) AttestationScore(root [32]byte) uint64 {
	return s.totalSupport[root]
}

// EquivocationScoreByCommittee implements the spec's get_equivocation_score.
func EquivocationScoreByCommittee(
	ctx context.Context,
	committees CommitteeAccessor,
	equivocating map[primitives.ValidatorIndex]bool,
	balances []uint64,
	startSlot, endSlot primitives.Slot,
) uint64 {
	if len(equivocating) == 0 {
		return 0
	}
	total := uint64(0)
	seen := make(map[primitives.ValidatorIndex]bool)
	for slot := startSlot; slot <= endSlot; slot++ {
		members, err := committees.Committee(ctx, slot)
		if err != nil {
			continue
		}
		for _, idx := range members {
			if seen[idx] {
				continue
			}
			seen[idx] = true
			if equivocating[idx] && uint64(idx) < uint64(len(balances)) {
				total += balances[idx]
			}
		}
	}
	return total
}

// cachedCommittees memoizes per-slot committee lookups for the duration of one FCR run.
type cachedCommittees struct {
	inner CommitteeAccessor
	m     map[primitives.Slot][]primitives.ValidatorIndex
}

func newCachedCommittees(inner CommitteeAccessor) *cachedCommittees {
	return &cachedCommittees{inner: inner, m: make(map[primitives.Slot][]primitives.ValidatorIndex)}
}

func (c *cachedCommittees) Committee(ctx context.Context, slot primitives.Slot) ([]primitives.ValidatorIndex, error) {
	if v, ok := c.m[slot]; ok {
		return v, nil
	}
	v, err := c.inner.Committee(ctx, slot)
	if err != nil {
		return nil, err
	}
	c.m[slot] = v
	return v, nil
}
