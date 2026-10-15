package cache

import (
	"slices"
	"sync"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

type inclusionListKey struct {
	slot          primitives.Slot
	dependentRoot [32]byte
}

type inclusionListEntry struct {
	signed *ethpb.SignedInclusionList
	timely bool
}

// InclusionListStore holds the inclusion lists and the equivocators per (slot, dependent root), as in the EIP-7805 InclusionListStore.
type InclusionListStore struct {
	lock         sync.RWMutex
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

// Process stores the inclusion list as in process_inclusion_list and returns true only when it adds a new entry. The caller must not change the list after Process.
func (s *InclusionListStore) Process(signed *ethpb.SignedInclusionList, timely bool) bool {
	if signed == nil || signed.Message == nil {
		return false
	}
	msg := signed.Message
	key := inclusionListKey{slot: msg.Slot, dependentRoot: bytesutil.ToBytes32(msg.DependentRoot)}

	s.lock.Lock()
	defer s.lock.Unlock()

	byValidator, ok := s.lists[key]
	if !ok {
		byValidator = make(map[primitives.ValidatorIndex]inclusionListEntry)
		s.lists[key] = byValidator
	}
	if stored, ok := byValidator[msg.ValidatorIndex]; ok {
		if !sameInclusionList(stored.signed.Message, msg) {
			if s.equivocators[key] == nil {
				s.equivocators[key] = make(map[primitives.ValidatorIndex]struct{})
			}
			s.equivocators[key][msg.ValidatorIndex] = struct{}{}
		}
		return false
	}
	byValidator[msg.ValidatorIndex] = inclusionListEntry{signed: signed, timely: timely}
	return true
}

// sameInclusionList compares by hash tree root, which is SSZ value equality, and falls back to proto.Equal if a root fails.
func sameInclusionList(a, b *ethpb.InclusionList) bool {
	ra, errA := a.HashTreeRoot()
	rb, errB := b.HashTreeRoot()
	if errA != nil || errB != nil {
		return proto.Equal(a, b)
	}
	return ra == rb
}

// Transactions returns the unique transactions of the non-equivocating lists, as in get_inclusion_list_transactions. The caller must not change the returned transactions.
func (s *InclusionListStore) Transactions(slot primitives.Slot, dependentRoot [32]byte, onlyTimely bool) [][]byte {
	s.lock.RLock()
	defer s.lock.RUnlock()

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

// Bits returns one bit per committee position, set when that member has a valid list, as in get_inclusion_list_bits.
func (s *InclusionListStore) Bits(committee []primitives.ValidatorIndex, slot primitives.Slot, dependentRoot [32]byte, onlyTimely bool) bitfield.Bitvector16 {
	s.lock.RLock()
	defer s.lock.RUnlock()

	valid := s.validIndices(inclusionListKey{slot: slot, dependentRoot: dependentRoot}, onlyTimely)
	bits := bitfield.NewBitvector16()
	for i, idx := range committee {
		if _, ok := slices.BinarySearch(valid, idx); ok {
			bits.SetBitAt(uint64(i), true)
		}
	}
	return bits
}

// IsBitsInclusive reports whether every locally set bit is also set in bits, as in is_inclusion_list_bits_inclusive.
func (s *InclusionListStore) IsBitsInclusive(committee []primitives.ValidatorIndex, slot primitives.Slot, dependentRoot [32]byte, bits bitfield.Bitvector16, onlyTimely bool) bool {
	local := s.Bits(committee, slot, dependentRoot, onlyTimely)
	for i := range local.Len() {
		if local.BitAt(i) && !bits.BitAt(i) {
			return false
		}
	}
	return true
}

// Get returns the stored inclusion list and its timeliness, also for an equivocator; call IsEquivocator. The caller must not change the returned list.
func (s *InclusionListStore) Get(slot primitives.Slot, dependentRoot [32]byte, validatorIndex primitives.ValidatorIndex) (*ethpb.SignedInclusionList, bool, bool) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	entry, ok := s.lists[inclusionListKey{slot: slot, dependentRoot: dependentRoot}][validatorIndex]
	return entry.signed, entry.timely, ok
}

// IsEquivocator reports whether the validator sent two different lists for the slot and dependent root.
func (s *InclusionListStore) IsEquivocator(slot primitives.Slot, dependentRoot [32]byte, validatorIndex primitives.ValidatorIndex) bool {
	s.lock.RLock()
	defer s.lock.RUnlock()

	_, ok := s.equivocators[inclusionListKey{slot: slot, dependentRoot: dependentRoot}][validatorIndex]
	return ok
}

// Prune removes the entries whose slot is more than MIN_SLOTS_FOR_INCLUSION_LISTS_REQUESTS slots before the current slot.
func (s *InclusionListStore) Prune(currentSlot primitives.Slot) {
	retain := primitives.Slot(params.BeaconConfig().MinSlotsForInclusionListsRequests)

	s.lock.Lock()
	defer s.lock.Unlock()

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
	var indices []primitives.ValidatorIndex
	for idx, entry := range s.lists[key] {
		if _, ok := s.equivocators[key][idx]; ok {
			continue
		}
		if onlyTimely && !entry.timely {
			continue
		}
		indices = append(indices, idx)
	}
	slices.Sort(indices)
	return indices
}
