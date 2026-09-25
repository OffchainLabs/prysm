package decoupled

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// Spec: compute_balance_weighted_selection (gloas). Prysm's PTC walks beacon committees unshuffled, the Simplex committee samples the shuffled active set.
func computeBalanceWeightedSelection(st state.ReadOnlyBeaconState, indices []primitives.ValidatorIndex, seed [32]byte, size uint64, shuffle bool) ([]primitives.ValidatorIndex, error) {
	total := uint64(len(indices))
	if total == 0 {
		return nil, errors.New("no candidates for balance weighted selection")
	}
	effective := make([]uint64, total)
	for i, idx := range indices {
		eb, err := st.EffectiveBalanceAtIndex(idx)
		if err != nil {
			return nil, err
		}
		effective[i] = eb
	}
	maxBalance := params.BeaconConfig().MaxEffectiveBalanceElectra
	selected := make([]primitives.ValidatorIndex, 0, size)
	var randomBytes [32]byte
	for i := uint64(0); uint64(len(selected)) < size; i++ {
		offset := i % 16 * 2
		if offset == 0 {
			randomBytes = hash.Hash(append(seed[:], bytesutil.Bytes8(i/16)...))
		}
		next := i % total
		if shuffle {
			shuffled, err := helpers.ComputeShuffledIndex(primitives.ValidatorIndex(next), total, seed, true)
			if err != nil {
				return nil, err
			}
			next = uint64(shuffled)
		}
		randomValue := uint64(binary.LittleEndian.Uint16(randomBytes[offset : offset+2]))
		if effective[next]*fieldparams.MaxRandomValueElectra >= maxBalance*randomValue {
			selected = append(selected, indices[next])
		}
	}
	return selected, nil
}

// Spec: the seed in compute_available_committee, hash(get_seed(state, epoch, DOMAIN_AVAILABLE_ATTESTER) + uint_to_bytes(slot)).
func availableCommitteeSeed(st state.ReadOnlyBeaconState, epoch primitives.Epoch, slot primitives.Slot) ([32]byte, error) {
	seed, err := helpers.Seed(st, epoch, params.BeaconConfig().DomainAvailableAttester)
	if err != nil {
		return [32]byte{}, err
	}
	return hash.Hash(append(seed[:], bytesutil.Bytes8(uint64(slot))...)), nil
}

// Spec: compute_available_committee
func ComputeAvailableCommittee(ctx context.Context, st state.ReadOnlyBeaconState, slot primitives.Slot) ([]primitives.ValidatorIndex, error) {
	epoch := slots.ToEpoch(slot)
	seed, err := availableCommitteeSeed(st, epoch, slot)
	if err != nil {
		return nil, err
	}
	active, err := helpers.ActiveValidatorIndices(ctx, st, epoch)
	if err != nil {
		return nil, err
	}
	return computeBalanceWeightedSelection(st, active, seed, fieldparams.AvailableCommitteeSize, true)
}

// Spec: get_available_committee
func AvailableCommittee(st state.ReadOnlyBeaconState, slot primitives.Slot) ([]primitives.ValidatorIndex, error) {
	cfg := params.BeaconConfig()
	window, err := st.AvailableCommitteeWindow()
	if err != nil {
		return nil, err
	}
	epoch := slots.ToEpoch(slot)
	stateEpoch := time.CurrentEpoch(st)
	var offset uint64
	switch {
	case epoch < stateEpoch:
		if epoch+1 != stateEpoch {
			return nil, fmt.Errorf("slot %d is more than one epoch behind state epoch %d", slot, stateEpoch)
		}
	case epoch > stateEpoch+cfg.MinSeedLookahead:
		return nil, fmt.Errorf("slot %d is beyond the seed lookahead from state epoch %d", slot, stateEpoch)
	default:
		offset = uint64(epoch-stateEpoch+1) * uint64(cfg.SlotsPerEpoch)
	}
	index := offset + uint64(slot%cfg.SlotsPerEpoch)
	if index >= uint64(len(window)) {
		return nil, fmt.Errorf("available committee window index %d out of range %d", index, len(window))
	}
	return window[index].ValidatorIndices, nil
}

// Spec: initialize_available_committee_window
func InitializeAvailableCommitteeWindow(ctx context.Context, st state.ReadOnlyBeaconState) ([]*ethpb.AvailableCommittee, error) {
	cfg := params.BeaconConfig()
	slotsPerEpoch := cfg.SlotsPerEpoch
	window := make([]*ethpb.AvailableCommittee, 0, slotsPerEpoch.Mul(uint64(2+cfg.MinSeedLookahead)))
	for range slotsPerEpoch {
		window = append(window, &ethpb.AvailableCommittee{ValidatorIndices: make([]primitives.ValidatorIndex, fieldparams.AvailableCommitteeSize)})
	}
	start, err := slots.EpochStart(time.CurrentEpoch(st))
	if err != nil {
		return nil, err
	}
	for i := range slotsPerEpoch.Mul(uint64(1 + cfg.MinSeedLookahead)) {
		committee, err := ComputeAvailableCommittee(ctx, st, start+i)
		if err != nil {
			return nil, err
		}
		window = append(window, &ethpb.AvailableCommittee{ValidatorIndices: committee})
	}
	return window, nil
}

// Spec: process_available_committee_window
func ProcessAvailableCommitteeWindow(ctx context.Context, st state.BeaconState) error {
	cfg := params.BeaconConfig()
	window, err := st.AvailableCommitteeWindow()
	if err != nil {
		return err
	}
	slotsPerEpoch := uint64(cfg.SlotsPerEpoch)
	next := make([]*ethpb.AvailableCommittee, len(window))
	copy(next, window[slotsPerEpoch:])
	start, err := slots.EpochStart(time.CurrentEpoch(st) + cfg.MinSeedLookahead + 1)
	if err != nil {
		return err
	}
	tail := uint64(len(window)) - slotsPerEpoch
	for i := range slotsPerEpoch {
		committee, err := ComputeAvailableCommittee(ctx, st, start+primitives.Slot(i))
		if err != nil {
			return err
		}
		next[tail+i] = &ethpb.AvailableCommittee{ValidatorIndices: committee}
	}
	return st.SetAvailableCommitteeWindow(next)
}
