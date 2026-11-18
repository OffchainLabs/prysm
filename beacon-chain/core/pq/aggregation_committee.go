package pq

import (
	"context"
	"encoding/binary"
	"slices"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// Spec: compute_balance_weighted_selection with size=1 and shuffle_indices=False
func selectByBalance(st state.ReadOnlyBeaconState, candidates []primitives.ValidatorIndex, seed [32]byte) (primitives.ValidatorIndex, error) {
	if len(candidates) == 0 {
		return 0, errors.New("no candidates")
	}
	maxBalance := params.BeaconConfig().MaxEffectiveBalanceElectra
	total := uint64(len(candidates))
	buf := make([]byte, 40)
	copy(buf, seed[:])
	var randomBytes [32]byte
	for i := uint64(0); ; i++ {
		offset := i % 16 * 2
		if offset == 0 {
			binary.LittleEndian.PutUint64(buf[32:], i/16)
			randomBytes = hash.Hash(buf)
		}
		candidate := candidates[i%total]
		val, err := st.ValidatorAtIndexReadOnly(candidate)
		if err != nil {
			return 0, err
		}
		randomValue := uint64(binary.LittleEndian.Uint16(randomBytes[offset : offset+2]))
		if val.EffectiveBalance()*fieldparams.MaxRandomValueElectra >= maxBalance*randomValue {
			return candidate, nil
		}
	}
}

// Spec: compute_aggregation_committee
func computeAggregationCommittee(ctx context.Context, st state.ReadOnlyBeaconState, slot primitives.Slot, index primitives.CommitteeIndex) ([]primitives.ValidatorIndex, error) {
	epochSeed, err := helpers.Seed(st, slots.ToEpoch(slot), params.BeaconConfig().DomainSelectionProof)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 48)
	copy(buf, epochSeed[:])
	binary.LittleEndian.PutUint64(buf[32:], uint64(slot))
	binary.LittleEndian.PutUint64(buf[40:], uint64(index))
	seed := hash.Hash(buf)

	committee, err := helpers.BeaconCommitteeFromState(ctx, st, slot, index)
	if err != nil {
		return nil, err
	}
	candidates := slices.Clone(committee)
	aggregators := make([]primitives.ValidatorIndex, 0, params.BeaconConfig().AggregatorsPerCommittee)
	roundBuf := make([]byte, 40)
	copy(roundBuf, seed[:])
	for uint64(len(aggregators)) < params.BeaconConfig().AggregatorsPerCommittee && len(candidates) > 0 {
		binary.LittleEndian.PutUint64(roundBuf[32:], uint64(len(aggregators)))
		aggregator, err := selectByBalance(st, candidates, hash.Hash(roundBuf))
		if err != nil {
			return nil, err
		}
		aggregators = append(aggregators, aggregator)
		candidates = slices.DeleteFunc(candidates, func(c primitives.ValidatorIndex) bool { return c == aggregator })
	}
	return aggregators, nil
}

// Spec: compute_aggregation_committees
func computeAggregationCommittees(ctx context.Context, st state.ReadOnlyBeaconState, slot primitives.Slot) (*ethpb.AggregationCommittees, error) {
	epoch := slots.ToEpoch(slot)
	activeCount, err := helpers.ActiveValidatorCount(ctx, st, epoch)
	if err != nil {
		return nil, err
	}
	committeesPerSlot := helpers.SlotCommitteeCountAtEpoch(activeCount, epoch)
	committees := make([][]primitives.ValidatorIndex, committeesPerSlot)
	for i := range committees {
		committees[i], err = computeAggregationCommittee(ctx, st, slot, primitives.CommitteeIndex(i))
		if err != nil {
			return nil, errors.Wrapf(err, "aggregation committee %d at slot %d", i, slot)
		}
	}
	return &ethpb.AggregationCommittees{Committees: committees}, nil
}

func computeEpochAggregationCommittees(ctx context.Context, st state.ReadOnlyBeaconState, epoch primitives.Epoch) ([]*ethpb.AggregationCommittees, error) {
	startSlot, err := slots.EpochStart(epoch)
	if err != nil {
		return nil, err
	}
	out := make([]*ethpb.AggregationCommittees, params.BeaconConfig().SlotsPerEpoch)
	for i := range out {
		out[i], err = computeAggregationCommittees(ctx, st, startSlot+primitives.Slot(i))
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Spec: initialize_aggregation_committee_window
func InitializeAggregationCommitteeWindow(ctx context.Context, st state.ReadOnlyBeaconState) ([]*ethpb.AggregationCommittees, error) {
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	window := make([]*ethpb.AggregationCommittees, 0, slotsPerEpoch.Mul(uint64(2+params.BeaconConfig().MinSeedLookahead)))
	for range slotsPerEpoch {
		window = append(window, &ethpb.AggregationCommittees{Committees: [][]primitives.ValidatorIndex{}})
	}
	currentEpoch := slots.ToEpoch(st.Slot())
	for epoch := currentEpoch; epoch <= currentEpoch+params.BeaconConfig().MinSeedLookahead; epoch++ {
		committees, err := computeEpochAggregationCommittees(ctx, st, epoch)
		if err != nil {
			return nil, err
		}
		window = append(window, committees...)
	}
	return window, nil
}

// Spec: process_aggregation_committee_window
func ProcessAggregationCommitteeWindow(ctx context.Context, st state.BeaconState) error {
	nextEpoch := slots.ToEpoch(st.Slot()) + params.BeaconConfig().MinSeedLookahead + 1
	committees, err := computeEpochAggregationCommittees(ctx, st, nextEpoch)
	if err != nil {
		return err
	}
	return st.RotateAggregationCommitteeWindow(committees)
}
