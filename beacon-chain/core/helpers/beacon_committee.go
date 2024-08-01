// Package helpers contains helper functions outlined in the Ethereum Beacon Chain spec, such as
// computing committees, randao, rewards/penalties, and more.
package helpers

import (
	"context"
	"fmt"
	"sort"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/go-bitfield"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/cache"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/time"
	forkchoicetypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/forkchoice/types"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/container/slice"
	"github.com/prysmaticlabs/prysm/v5/crypto/hash"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/math"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

var (
	committeeCache       = cache.NewCommitteesCache()
	proposerIndicesCache = cache.NewProposerIndicesCache()
)

// SlotCommitteeCount returns the number of beacon committees of a slot. The
// active validator count is provided as an argument rather than an imported implementation
// from the spec definition. Having the active validator count as an argument allows for
// cheaper computation, instead of retrieving head state, one can retrieve the validator
// count.
//
// Spec pseudocode definition:
//
//	def get_committee_count_per_slot(state: BeaconState, epoch: Epoch) -> uint64:
//	 """
//	 Return the number of committees in each slot for the given ``epoch``.
//	 """
//	 return max(uint64(1), min(
//	     MAX_COMMITTEES_PER_SLOT,
//	     uint64(len(get_active_validator_indices(state, epoch))) // SLOTS_PER_EPOCH // TARGET_COMMITTEE_SIZE,
//	 ))
func SlotCommitteeCount(activeValidatorCount uint64) uint64 {
	var committeesPerSlot = activeValidatorCount / uint64(params.BeaconConfig().SlotsPerEpoch) / params.BeaconConfig().TargetCommitteeSize

	if committeesPerSlot > params.BeaconConfig().MaxCommitteesPerSlot {
		return params.BeaconConfig().MaxCommitteesPerSlot
	}
	if committeesPerSlot == 0 {
		return 1
	}

	return committeesPerSlot
}

// AttestationCommittees returns beacon state committees that reflect attestation's committee indices.
func AttestationCommittees(ctx context.Context, st state.ReadOnlyBeaconState, att ethpb.Att) ([][]primitives.ValidatorIndex, error) {
	var committees [][]primitives.ValidatorIndex
	if att.Version() >= version.Electra {
		committeeIndices := att.CommitteeBitsVal().BitIndices()
		committees = make([][]primitives.ValidatorIndex, len(committeeIndices))
		for i, ci := range committeeIndices {
			committee, err := BeaconCommitteeFromState(ctx, st, att.GetData().Slot, primitives.CommitteeIndex(ci))
			if err != nil {
				return nil, err
			}
			committees[i] = committee
		}
	} else {
		committee, err := BeaconCommitteeFromState(ctx, st, att.GetData().Slot, att.GetData().CommitteeIndex)
		if err != nil {
			return nil, err
		}
		committees = [][]primitives.ValidatorIndex{committee}
	}
	return committees, nil
}

// BeaconCommittees returns the list of all beacon committees for a given state at a given slot.
func BeaconCommittees(ctx context.Context, state state.ReadOnlyBeaconState, slot primitives.Slot) ([][]primitives.ValidatorIndex, error) {
	epoch := slots.ToEpoch(slot)
	activeCount, err := ActiveValidatorCount(ctx, state, epoch)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute active validator count")
	}
	committeesPerSlot := SlotCommitteeCount(activeCount)
	seed, err := Seed(state, epoch, params.BeaconConfig().DomainBeaconAttester)
	if err != nil {
		return nil, errors.Wrap(err, "could not get seed")
	}

	committees := make([][]primitives.ValidatorIndex, committeesPerSlot)
	var activeIndices []primitives.ValidatorIndex

	for idx := primitives.CommitteeIndex(0); idx < primitives.CommitteeIndex(len(committees)); idx++ {
		committee, err := committeeCache.Committee(ctx, slot, seed, idx)
		if err != nil {
			return nil, errors.Wrap(err, "could not interface with committee cache")
		}
		if committee != nil {
			committees[idx] = committee
			continue
		}

		if len(activeIndices) == 0 {
			activeIndices, err = ActiveValidatorIndices(ctx, state, epoch)
			if err != nil {
				return nil, errors.Wrap(err, "could not get active indices")
			}
		}
		committee, err = BeaconCommittee(ctx, activeIndices, seed, slot, idx)
		if err != nil {
			return nil, errors.Wrap(err, "could not compute beacon committee")
		}
		committees[idx] = committee
	}
	return committees, nil
}

// BeaconCommitteeFromState returns the crosslink committee of a given slot and committee index. This
// is a spec implementation where state is used as an argument. In case of state retrieval
// becomes expensive, consider using BeaconCommittee below.
//
// Spec pseudocode definition:
//
//	def get_beacon_committee(state: BeaconState, slot: Slot, index: CommitteeIndex) -> Sequence[ValidatorIndex]:
//	 """
//	 Return the beacon committee at ``slot`` for ``index``.
//	 """
//	 epoch = compute_epoch_at_slot(slot)
//	 committees_per_slot = get_committee_count_per_slot(state, epoch)
//	 return compute_committee(
//	     indices=get_active_validator_indices(state, epoch),
//	     seed=get_seed(state, epoch, DOMAIN_BEACON_ATTESTER),
//	     index=(slot % SLOTS_PER_EPOCH) * committees_per_slot + index,
//	     count=committees_per_slot * SLOTS_PER_EPOCH,
//	 )
func BeaconCommitteeFromState(ctx context.Context, state state.ReadOnlyBeaconState, slot primitives.Slot, committeeIndex primitives.CommitteeIndex) ([]primitives.ValidatorIndex, error) {
	epoch := slots.ToEpoch(slot)
	seed, err := Seed(state, epoch, params.BeaconConfig().DomainBeaconAttester)
	if err != nil {
		return nil, errors.Wrap(err, "could not get seed")
	}

	committee, err := committeeCache.Committee(ctx, slot, seed, committeeIndex)
	if err != nil {
		return nil, errors.Wrap(err, "could not interface with committee cache")
	}
	if committee != nil {
		return committee, nil
	}

	activeIndices, err := ActiveValidatorIndices(ctx, state, epoch)
	if err != nil {
		return nil, errors.Wrap(err, "could not get active indices")
	}

	return BeaconCommittee(ctx, activeIndices, seed, slot, committeeIndex)
}

// BeaconCommittee returns the beacon committee of a given slot and committee index. The
// validator indices and seed are provided as an argument rather than an imported implementation
// from the spec definition. Having them as an argument allows for cheaper computation run time.
//
// Spec pseudocode definition:
//
//	def get_beacon_committee(state: BeaconState, slot: Slot, index: CommitteeIndex) -> Sequence[ValidatorIndex]:
//	 """
//	 Return the beacon committee at ``slot`` for ``index``.
//	 """
//	 epoch = compute_epoch_at_slot(slot)
//	 committees_per_slot = get_committee_count_per_slot(state, epoch)
//	 return compute_committee(
//	     indices=get_active_validator_indices(state, epoch),
//	     seed=get_seed(state, epoch, DOMAIN_BEACON_ATTESTER),
//	     index=(slot % SLOTS_PER_EPOCH) * committees_per_slot + index,
//	     count=committees_per_slot * SLOTS_PER_EPOCH,
//	 )
func BeaconCommittee(
	ctx context.Context,
	validatorIndices []primitives.ValidatorIndex,
	seed [32]byte,
	slot primitives.Slot,
	committeeIndex primitives.CommitteeIndex,
) ([]primitives.ValidatorIndex, error) {
	committee, err := committeeCache.Committee(ctx, slot, seed, committeeIndex)
	if err != nil {
		return nil, errors.Wrap(err, "could not interface with committee cache")
	}
	if committee != nil {
		return committee, nil
	}

	committeesPerSlot := SlotCommitteeCount(uint64(len(validatorIndices)))

	indexOffset, err := math.Add64(uint64(committeeIndex), uint64(slot.ModSlot(params.BeaconConfig().SlotsPerEpoch).Mul(committeesPerSlot)))
	if err != nil {
		return nil, errors.Wrap(err, "could not add calculate index offset")
	}
	count := committeesPerSlot * uint64(params.BeaconConfig().SlotsPerEpoch)

	return ComputeCommittee(validatorIndices, seed, indexOffset, count)
}

// CommitteeAssignment represents committee list, committee index, and to be attested slot for a given epoch.
type CommitteeAssignment struct {
	Committee      []primitives.ValidatorIndex
	AttesterSlot   primitives.Slot
	CommitteeIndex primitives.CommitteeIndex
	PtcSlot        primitives.Slot
}

// verifyAssignmentEpoch verifies if the given epoch is valid for assignment based on the provided state.
// It checks if the epoch is not greater than the next epoch, and if the start slot of the epoch is greater
// than or equal to the minimum valid start slot calculated based on the state's current slot and historical roots.
func verifyAssignmentEpoch(epoch primitives.Epoch, state state.BeaconState) error {
	nextEpoch := time.NextEpoch(state)
	if epoch > nextEpoch {
		return fmt.Errorf("epoch %d can't be greater than next epoch %d", epoch, nextEpoch)
	}

	startSlot, err := slots.EpochStart(epoch)
	if err != nil {
		return err
	}
	minValidStartSlot := primitives.Slot(0)
	if stateSlot := state.Slot(); stateSlot >= params.BeaconConfig().SlotsPerHistoricalRoot {
		minValidStartSlot = stateSlot - params.BeaconConfig().SlotsPerHistoricalRoot
	}
	if startSlot < minValidStartSlot {
		return fmt.Errorf("start slot %d is smaller than the minimum valid start slot %d", startSlot, minValidStartSlot)
	}
	return nil
}

// ProposerAssignments calculates proposer assignments for each validator during the specified epoch.
// It verifies the validity of the epoch, then iterates through each slot in the epoch to determine the
// proposer for that slot and assigns them accordingly.
func ProposerAssignments(ctx context.Context, state state.BeaconState, epoch primitives.Epoch) (map[primitives.ValidatorIndex][]primitives.Slot, error) {
	// Verify if the epoch is valid for assignment based on the provided state.
	if err := verifyAssignmentEpoch(epoch, state); err != nil {
		return nil, err
	}
	startSlot, err := slots.EpochStart(epoch)
	if err != nil {
		return nil, err
	}

	proposerAssignments := make(map[primitives.ValidatorIndex][]primitives.Slot)

	originalStateSlot := state.Slot()

	for slot := startSlot; slot < startSlot+params.BeaconConfig().SlotsPerEpoch; slot++ {
		// Skip proposer assignment for genesis slot.
		if slot == 0 {
			continue
		}
		// Set the state's current slot.
		if err := state.SetSlot(slot); err != nil {
			return nil, err
		}

		// Determine the proposer index for the current slot.
		i, err := BeaconProposerIndex(ctx, state)
		if err != nil {
			return nil, errors.Wrapf(err, "could not check proposer at slot %d", state.Slot())
		}

		// Append the slot to the proposer's assignments.
		if _, ok := proposerAssignments[i]; !ok {
			proposerAssignments[i] = make([]primitives.Slot, 0)
		}
		proposerAssignments[i] = append(proposerAssignments[i], slot)
	}

	// Reset state back to its original slot.
	if err := state.SetSlot(originalStateSlot); err != nil {
		return nil, err
	}

	return proposerAssignments, nil
}

// CommitteeAssignments calculates committee assignments for each validator during the specified epoch.
// It retrieves active validator indices, determines the number of committees per slot, and computes
// assignments for each validator based on their presence in the provided validators slice.
func CommitteeAssignments(ctx context.Context, state state.BeaconState, epoch primitives.Epoch, validators []primitives.ValidatorIndex) (map[primitives.ValidatorIndex]*CommitteeAssignment, error) {
	// Verify if the epoch is valid for assignment based on the provided state.
	if err := verifyAssignmentEpoch(epoch, state); err != nil {
		return nil, err
	}
	slot, err := slots.EpochStart(epoch)
	if err != nil {
		return nil, err
	}
	vals := make(map[primitives.ValidatorIndex]struct{})
	for _, v := range validators {
		vals[v] = struct{}{}
	}
	assignments := make(map[primitives.ValidatorIndex]*CommitteeAssignment)

	committees, err := BeaconCommittees(ctx, state, slot)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute beacon committees")
	}
	ptcPerSlot, ptcMembersPerCommittee := PtcAllocation(len(committees))
	// Compute committee assignments for each slot in the epoch.
	endSlot := slot + params.BeaconConfig().SlotsPerEpoch
	for {
		for j, committee := range committees {
			for i, vIndex := range committee {
				if _, ok := vals[vIndex]; !ok { // Skip if the validator is not in the provided validators slice.
					continue
				}
				if _, ok := assignments[vIndex]; !ok {
					assignments[vIndex] = &CommitteeAssignment{}
				}
				assignments[vIndex].Committee = committee
				assignments[vIndex].AttesterSlot = slot
				assignments[vIndex].CommitteeIndex = primitives.CommitteeIndex(j)
				if uint64(j) < ptcPerSlot && uint64(i) < ptcMembersPerCommittee {
					assignments[vIndex].PtcSlot = slot
				}
			}
		}
		slot++
		if slot == endSlot {
			break
		}
		committees, err = BeaconCommittees(ctx, state, slot)
		if err != nil {
			return nil, errors.Wrap(err, "could not compute beacon committees")
		}
	}
	return assignments, nil
}

// VerifyBitfieldLength verifies that a bitfield length matches the given committee size.
func VerifyBitfieldLength(bf bitfield.Bitfield, committeeSize uint64) error {
	if bf.Len() != committeeSize {
		return fmt.Errorf(
			"wanted participants bitfield length %d, got: %d",
			committeeSize,
			bf.Len())
	}
	return nil
}

// ShuffledIndices uses input beacon state and returns the shuffled indices of the input epoch,
// the shuffled indices then can be used to break up into committees.
func ShuffledIndices(s state.ReadOnlyBeaconState, epoch primitives.Epoch) ([]primitives.ValidatorIndex, error) {
	seed, err := Seed(s, epoch, params.BeaconConfig().DomainBeaconAttester)
	if err != nil {
		return nil, errors.Wrapf(err, "could not get seed for epoch %d", epoch)
	}

	indices := make([]primitives.ValidatorIndex, 0, s.NumValidators())
	if err := s.ReadFromEveryValidator(func(idx int, val state.ReadOnlyValidator) error {
		if IsActiveValidatorUsingTrie(val, epoch) {
			indices = append(indices, primitives.ValidatorIndex(idx))
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// UnshuffleList is used as an optimized implementation for raw speed.
	return UnshuffleList(indices, seed)
}

// CommitteeIndices return beacon committee indices corresponding to bits that are set on the argument bitfield.
//
// Spec pseudocode definition:
//
//	def get_committee_indices(committee_bits: Bitvector) -> Sequence[CommitteeIndex]:
//	   return [CommitteeIndex(index) for index, bit in enumerate(committee_bits) if bit]
func CommitteeIndices(committeeBits bitfield.Bitfield) []primitives.CommitteeIndex {
	indices := committeeBits.BitIndices()
	committeeIndices := make([]primitives.CommitteeIndex, len(indices))
	for i, ix := range indices {
		committeeIndices[i] = primitives.CommitteeIndex(uint64(ix))
	}
	return committeeIndices
}

// UpdateCommitteeCache gets called at the beginning of every epoch to cache the committee shuffled indices
// list with committee index and epoch number. It caches the shuffled indices for the input epoch.
func UpdateCommitteeCache(ctx context.Context, state state.ReadOnlyBeaconState, e primitives.Epoch) error {
	seed, err := Seed(state, e, params.BeaconConfig().DomainBeaconAttester)
	if err != nil {
		return err
	}
	if committeeCache.HasEntry(string(seed[:])) {
		return nil
	}
	shuffledIndices, err := ShuffledIndices(state, e)
	if err != nil {
		return err
	}

	count := SlotCommitteeCount(uint64(len(shuffledIndices)))

	// Store the sorted indices as well as shuffled indices. In current spec,
	// sorted indices is required to retrieve proposer index. This is also
	// used for failing verify signature fallback.
	sortedIndices := make([]primitives.ValidatorIndex, len(shuffledIndices))
	copy(sortedIndices, shuffledIndices)
	sort.Slice(sortedIndices, func(i, j int) bool {
		return sortedIndices[i] < sortedIndices[j]
	})

	if err := committeeCache.AddCommitteeShuffledList(ctx, &cache.Committees{
		ShuffledIndices: shuffledIndices,
		CommitteeCount:  uint64(params.BeaconConfig().SlotsPerEpoch.Mul(count)),
		Seed:            seed,
		SortedIndices:   sortedIndices,
	}); err != nil {
		return err
	}
	return nil
}

// UpdateProposerIndicesInCache updates proposer indices entry of the committee cache.
// Input state is used to retrieve active validator indices.
// Input root is to use as key in the cache.
// Input epoch is the epoch to retrieve proposer indices for.
func UpdateProposerIndicesInCache(ctx context.Context, state state.ReadOnlyBeaconState, epoch primitives.Epoch) error {
	// The cache uses the state root at the end of (current epoch - 1) as key.
	// (e.g. for epoch 2, the key is root at slot 63)
	if epoch <= params.BeaconConfig().GenesisEpoch+params.BeaconConfig().MinSeedLookahead {
		return nil
	}
	slot, err := slots.EpochEnd(epoch - 1)
	if err != nil {
		return err
	}
	root, err := StateRootAtSlot(state, slot)
	if err != nil {
		return err
	}
	// Skip cache update if the key already exists
	_, ok := proposerIndicesCache.ProposerIndices(epoch, [32]byte(root))
	if ok {
		return nil
	}
	indices, err := ActiveValidatorIndices(ctx, state, epoch)
	if err != nil {
		return err
	}
	proposerIndices, err := PrecomputeProposerIndices(state, indices, epoch)
	if err != nil {
		return err
	}
	if len(proposerIndices) != int(params.BeaconConfig().SlotsPerEpoch) {
		return errors.New("invalid proposer length returned from state")
	}
	// This is here to deal with tests only
	var indicesArray [fieldparams.SlotsPerEpoch]primitives.ValidatorIndex
	copy(indicesArray[:], proposerIndices)
	proposerIndicesCache.Prune(epoch - 2)
	proposerIndicesCache.Set(epoch, [32]byte(root), indicesArray)
	return nil
}

// UpdateCachedCheckpointToStateRoot updates the map from checkpoints to state root in the proposer indices cache
func UpdateCachedCheckpointToStateRoot(state state.ReadOnlyBeaconState, cp *forkchoicetypes.Checkpoint) error {
	if cp.Epoch <= params.BeaconConfig().GenesisEpoch+params.BeaconConfig().MinSeedLookahead {
		return nil
	}
	slot, err := slots.EpochEnd(cp.Epoch)
	if err != nil {
		return err
	}
	root, err := state.StateRootAtIndex(uint64(slot % params.BeaconConfig().SlotsPerHistoricalRoot))
	if err != nil {
		return err
	}
	proposerIndicesCache.SetCheckpoint(*cp, [32]byte(root))
	return nil
}

// ExpandCommitteeCache resizes the cache to a higher limit.
func ExpandCommitteeCache() {
	committeeCache.ExpandCommitteeCache()
}

// CompressCommitteeCache resizes the cache to a lower limit.
func CompressCommitteeCache() {
	committeeCache.CompressCommitteeCache()
}

// ClearCache clears the beacon committee cache and sync committee cache.
func ClearCache() {
	committeeCache.Clear()
	proposerIndicesCache.Prune(0)
	syncCommitteeCache.Clear()
	balanceCache.Clear()
}

// ComputeCommittee returns the requested shuffled committee out of the total committees using
// validator indices and seed.
//
// Spec pseudocode definition:
//
//	def compute_committee(indices: Sequence[ValidatorIndex],
//	                    seed: Bytes32,
//	                    index: uint64,
//	                    count: uint64) -> Sequence[ValidatorIndex]:
//	  """
//	  Return the committee corresponding to ``indices``, ``seed``, ``index``, and committee ``count``.
//	  """
//	  start = (len(indices) * index) // count
//	  end = (len(indices) * uint64(index + 1)) // count
//	  return [indices[compute_shuffled_index(uint64(i), uint64(len(indices)), seed)] for i in range(start, end)]
func ComputeCommittee(
	indices []primitives.ValidatorIndex,
	seed [32]byte,
	index, count uint64,
) ([]primitives.ValidatorIndex, error) {
	validatorCount := uint64(len(indices))
	start := slice.SplitOffset(validatorCount, count, index)
	end := slice.SplitOffset(validatorCount, count, index+1)

	if start > validatorCount || end > validatorCount {
		return nil, errors.New("index out of range")
	}

	// Save the shuffled indices in cache, this is only needed once per epoch or once per new committee index.
	shuffledIndices := make([]primitives.ValidatorIndex, len(indices))
	copy(shuffledIndices, indices)
	// UnshuffleList is used here as it is an optimized implementation created
	// for fast computation of committees.
	// Reference implementation: https://github.com/protolambda/eth2-shuffle
	shuffledList, err := UnshuffleList(shuffledIndices, seed)
	if err != nil {
		return nil, err
	}

	return shuffledList[start:end], nil
}

// PrecomputeProposerIndices computes proposer indices of the current epoch and returns a list of proposer indices,
// the index of the list represents the slot number.
func PrecomputeProposerIndices(state state.ReadOnlyBeaconState, activeIndices []primitives.ValidatorIndex, e primitives.Epoch) ([]primitives.ValidatorIndex, error) {
	hashFunc := hash.CustomSHA256Hasher()
	proposerIndices := make([]primitives.ValidatorIndex, params.BeaconConfig().SlotsPerEpoch)

	seed, err := Seed(state, e, params.BeaconConfig().DomainBeaconProposer)
	if err != nil {
		return nil, errors.Wrap(err, "could not generate seed")
	}
	slot, err := slots.EpochStart(e)
	if err != nil {
		return nil, err
	}
	for i := uint64(0); i < uint64(params.BeaconConfig().SlotsPerEpoch); i++ {
		seedWithSlot := append(seed[:], bytesutil.Bytes8(uint64(slot)+i)...)
		seedWithSlotHash := hashFunc(seedWithSlot)
		index, err := ComputeProposerIndex(state, activeIndices, seedWithSlotHash)
		if err != nil {
			return nil, err
		}
		proposerIndices[i] = index
	}

	return proposerIndices, nil
}
