package confirmation

import (
	"context"
	"fmt"

	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// noAssignment marks a validator with no attestation duty in an epoch.
const noAssignment = uint8(0xFF)

// epochSlotTable maps every validator to its assigned attestation slot within one
// epoch. This makes membership queries O(1).
type epochSlotTable struct {
	start      primitives.Slot
	seed       [32]byte // attester shuffling seed
	slotOffset []uint8  // val index -> slot offset
}

// newEpochSlotTable builds the table for one epoch
func newEpochSlotTable(ctx context.Context, committees CommitteeAccessor, epoch primitives.Epoch, seed [32]byte, sizeHint int) (*epochSlotTable, error) {
	start, err := slots.EpochStart(epoch)
	if err != nil {
		return nil, fmt.Errorf("failed to get epoch start: %w", err)
	}

	offs := make([]uint8, sizeHint)
	for i := range offs {
		offs[i] = noAssignment
	}

	spe := primitives.Slot(params.BeaconConfig().SlotsPerEpoch)
	for slot := start; slot < start+spe; slot++ {
		members, err := committees.Committee(ctx, slot)
		if err != nil {
			return nil, fmt.Errorf("failed to get committee for slot %d: %w", slot, err)
		}

		for _, v := range members {
			if uint64(v) >= uint64(len(offs)) {
				grown := make([]uint8, v+1)
				copy(grown, offs)
				for i := len(offs); i < len(grown); i++ {
					grown[i] = noAssignment
				}
				offs = grown
			}

			offs[v] = uint8(slot - start)
		}
	}
	return &epochSlotTable{start: start, seed: seed, slotOffset: offs}, nil
}

// assignedSlot computes the attestation slot assigned to a validator in this epoch, if any.
// Caller should check the boolean return value before using the slot.
func (t *epochSlotTable) assignedSlot(v primitives.ValidatorIndex) (primitives.Slot, bool) {
	if uint64(v) >= uint64(len(t.slotOffset)) {
		return 0, false
	}

	if t.slotOffset[v] == noAssignment {
		return 0, false
	}

	return t.start + primitives.Slot(t.slotOffset[v]), true
}

// SupportMap pre-aggregates vote support for the slot-range queries FCR needs.
// totalSupport backs get_attestation_score (a vote credits every ancestor);
type SupportMap struct {
	votes        []forkchoicetypes.VoteData
	balances     []uint64
	equivocating map[primitives.ValidatorIndex]bool
	tables       []*epochSlotTable
	rootWeights  map[[32]byte]uint64
	totalSupport map[[32]byte]uint64
}

func NewSupportMap() *SupportMap {
	return &SupportMap{
		rootWeights:  make(map[[32]byte]uint64),
		totalSupport: make(map[[32]byte]uint64),
	}
}

// Build snapshots the query inputs and aggregates per-root vote weights, it reads no forkchoice state so it can run off the lock.
func (s *SupportMap) Build(
	votes []forkchoicetypes.VoteData,
	balances []uint64,
	tables []*epochSlotTable,
	equivocating map[primitives.ValidatorIndex]bool,
) {
	s.votes = votes
	s.balances = balances
	s.tables = tables
	s.equivocating = equivocating

	hasEquivocating := len(equivocating) > 0

	clear(s.rootWeights)
	for i, vote := range votes {
		if vote.Root == ([32]byte{}) {
			continue
		}
		if i >= len(balances) || balances[i] == 0 {
			continue
		}
		if hasEquivocating && equivocating[primitives.ValidatorIndex(i)] {
			continue
		}
		s.rootWeights[vote.Root] += balances[i]
	}
}

func (s *SupportMap) Accumulate(fc ForkchoiceReader) {
	clear(s.totalSupport)
	for root, bal := range s.rootWeights {
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
	if root == ([32]byte{}) {
		return 0
	}
	hasEquivocating := len(s.equivocating) > 0
	total := uint64(0)
	for i, vote := range s.votes {
		if vote.Root != root {
			continue
		}
		if i >= len(s.balances) || s.balances[i] == 0 {
			continue
		}
		if hasEquivocating && s.equivocating[primitives.ValidatorIndex(i)] {
			continue
		}
		for _, t := range s.tables {
			if sl, ok := t.assignedSlot(primitives.ValidatorIndex(i)); ok && sl >= startSlot && sl <= endSlot {
				total += s.balances[i]
				break
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

// EquivocationScore implements the spec's get_equivocation_score.
func (s *SupportMap) EquivocationScore(startSlot, endSlot primitives.Slot) uint64 {
	total := uint64(0)
	for idx, isEquivocating := range s.equivocating {
		if !isEquivocating || uint64(idx) >= uint64(len(s.balances)) {
			continue
		}
		for _, t := range s.tables {
			if sl, ok := t.assignedSlot(idx); ok && sl >= startSlot && sl <= endSlot {
				total += s.balances[idx]
				break
			}
		}
	}
	return total
}
