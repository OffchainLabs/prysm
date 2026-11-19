package cache

import (
	"slices"
	"sync"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

type inclusionListKey struct {
	slot          primitives.Slot
	dependentRoot [32]byte
}

type inclusionListEntry struct {
	signed *ethpb.SignedInclusionList
	timely bool
}

// InclusionListStore holds the inclusion lists and the equivocators per (slot, dependent root).
type InclusionListStore struct {
	sync.RWMutex
	lists        map[inclusionListKey]map[primitives.ValidatorIndex]inclusionListEntry
	equivocators map[inclusionListKey]map[primitives.ValidatorIndex]struct{}
}

// NewInclusionListStore returns an empty inclusion list store.
func NewInclusionListStore() *InclusionListStore {
	return &InclusionListStore{
		lists:        make(map[inclusionListKey]map[primitives.ValidatorIndex]inclusionListEntry),
		equivocators: make(map[inclusionListKey]map[primitives.ValidatorIndex]struct{}),
	}
}

// Process stores the inclusion list and returns true only when it adds a new entry.
func (s *InclusionListStore) Process(signed *ethpb.SignedInclusionList, timely bool) bool {
	if signed == nil || signed.Message == nil {
		return false
	}
	msg := signed.Message
	key := inclusionListKey{slot: msg.Slot, dependentRoot: bytesutil.ToBytes32(msg.DependentRoot)}
	validatorIndex := msg.ValidatorIndex

	s.Lock()
	defer s.Unlock()

	byValidator, ok := s.lists[key]
	if !ok {
		byValidator = make(map[primitives.ValidatorIndex]inclusionListEntry)
		s.lists[key] = byValidator
	}

	// Mark as equivocating if the inclusion list differs from the stored one.
	if stored, ok := byValidator[validatorIndex]; ok {
		if equivocates(stored.signed.Message, msg) {
			if s.equivocators[key] == nil {
				s.equivocators[key] = make(map[primitives.ValidatorIndex]struct{})
			}
			s.equivocators[key][validatorIndex] = struct{}{}
		}
		return false
	}

	// Add the new inclusion list entry for this validator.
	byValidator[validatorIndex] = inclusionListEntry{signed: signed, timely: timely}
	return true
}

// equivocates reports whether both lists hash and their roots differ. An unhashable list is invalid and proves nothing.
func equivocates(a, b *ethpb.InclusionList) bool {
	ra, errA := a.HashTreeRoot()
	rb, errB := b.HashTreeRoot()
	return errA == nil && errB == nil && ra != rb
}

// Transactions returns the unique transactions of the non-equivocating lists.
func (s *InclusionListStore) Transactions(slot primitives.Slot, dependentRoot [32]byte, onlyTimely bool) [][]byte {
	s.RLock()
	defer s.RUnlock()

	key := inclusionListKey{slot: slot, dependentRoot: dependentRoot}
	seen := make(map[string]struct{})
	var txs [][]byte
	// Ascending validator index and list order give a deterministic result.
	for _, idx := range s.validIndices(key, onlyTimely) {
		for _, tx := range s.lists[key][idx].signed.Message.Transactions.Slice() {
			if _, ok := seen[string(tx)]; ok {
				continue
			}
			seen[string(tx)] = struct{}{}
			txs = append(txs, tx)
		}
	}
	return txs
}

// Bits returns one bit per committee position, set when that member has a valid list.
func (s *InclusionListStore) Bits(committee []primitives.ValidatorIndex, slot primitives.Slot, dependentRoot [32]byte, onlyTimely bool) bitfield.Bitvector16 {
	s.RLock()
	defer s.RUnlock()

	key := inclusionListKey{slot: slot, dependentRoot: dependentRoot}
	bits := bitfield.NewBitvector16()
	for i, idx := range committee {
		if s.isValid(key, idx, onlyTimely) {
			bits.SetBitAt(uint64(i), true)
		}
	}
	return bits
}

// IsBitsInclusive reports whether every locally set bit is also set in bits.
func (s *InclusionListStore) IsBitsInclusive(committee []primitives.ValidatorIndex, slot primitives.Slot, dependentRoot [32]byte, bits bitfield.Bitvector16, onlyTimely bool) bool {
	ok, err := bits.Contains(s.Bits(committee, slot, dependentRoot, onlyTimely))
	return err == nil && ok
}

// Get returns the stored inclusion list and its timeliness. The caller must not change the returned list.
func (s *InclusionListStore) Get(slot primitives.Slot, dependentRoot [32]byte, validatorIndex primitives.ValidatorIndex) (*ethpb.SignedInclusionList, bool, bool) {
	s.RLock()
	defer s.RUnlock()

	entry, ok := s.lists[inclusionListKey{slot: slot, dependentRoot: dependentRoot}][validatorIndex]
	return entry.signed, entry.timely, ok
}

// IsEquivocator reports whether the validator sent two different lists for the slot and dependent root.
func (s *InclusionListStore) IsEquivocator(slot primitives.Slot, dependentRoot [32]byte, validatorIndex primitives.ValidatorIndex) bool {
	s.RLock()
	defer s.RUnlock()

	_, ok := s.equivocators[inclusionListKey{slot: slot, dependentRoot: dependentRoot}][validatorIndex]
	return ok
}

// Prune removes the entries whose slot is more than MIN_SLOTS_FOR_INCLUSION_LISTS_REQUESTS slots before the current slot.
func (s *InclusionListStore) Prune(currentSlot primitives.Slot) {
	retain := primitives.Slot(params.BeaconConfig().MinSlotsForInclusionListsRequests)

	s.Lock()
	defer s.Unlock()

	for key := range s.lists {
		if key.slot+retain < currentSlot {
			delete(s.lists, key)
		}
	}
	for key := range s.equivocators {
		if key.slot+retain < currentSlot {
			delete(s.equivocators, key)
		}
	}
}

// validIndices returns the sorted validators with a non-equivocating list that passes the timeliness filter. The caller must hold the lock.
func (s *InclusionListStore) validIndices(key inclusionListKey, onlyTimely bool) []primitives.ValidatorIndex {
	indices := make([]primitives.ValidatorIndex, 0, len(s.lists[key]))
	for idx := range s.lists[key] {
		if s.isValid(key, idx, onlyTimely) {
			indices = append(indices, idx)
		}
	}
	slices.Sort(indices)
	return indices
}

// isValid reports whether the validator has a non-equivocating list that passes the timeliness filter. The caller must hold the lock.
func (s *InclusionListStore) isValid(key inclusionListKey, idx primitives.ValidatorIndex, onlyTimely bool) bool {
	entry, ok := s.lists[key][idx]
	if !ok {
		return false
	}
	if _, ok := s.equivocators[key][idx]; ok {
		return false
	}
	return !onlyTimely || entry.timely
}
