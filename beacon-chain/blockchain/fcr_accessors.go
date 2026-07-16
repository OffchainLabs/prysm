package blockchain

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/confirmation"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	coreTime "github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

type fcrCommitteeAccessor struct {
	s *Service
}

func (a *fcrCommitteeAccessor) Committee(ctx context.Context, slot primitives.Slot) ([]primitives.ValidatorIndex, error) {
	a.s.headLock.RLock()
	headState := a.s.head.state
	a.s.headLock.RUnlock()
	if headState == nil || headState.IsNil() {
		return nil, errors.New("head state not available")
	}
	// Shuffling past the next epoch is not final, computing it would silently use a stale randao mix.
	if slots.ToEpoch(slot) > coreTime.CurrentEpoch(headState)+1 {
		return nil, errors.Errorf("slot %d is beyond the head state's next epoch", slot)
	}
	committees, err := helpers.BeaconCommittees(ctx, headState, slot)
	if err != nil {
		return nil, err
	}
	n := 0
	for _, committee := range committees {
		n += len(committee)
	}
	result := make([]primitives.ValidatorIndex, 0, n)
	for _, committee := range committees {
		result = append(result, committee...)
	}
	return result, nil
}

type fcrBalanceAccessor struct {
	s *Service
	// Checkpoint states are immutable, cached entries never go stale.
	byCheckpoint map[forkchoicetypes.Checkpoint]*confirmation.FFGStateInfo
}

func extractBalanceInfo(ctx context.Context, st state.ReadOnlyBeaconState) (*confirmation.FFGStateInfo, error) {
	total, err := helpers.TotalActiveBalance(ctx, st)
	if err != nil {
		return nil, err
	}
	balances := make([]uint64, st.NumValidators())
	epoch := coreTime.CurrentEpoch(st)
	for idx, val := range st.ValidatorsReadOnlySeq() {
		if helpers.IsActiveValidatorUsingTrie(val, epoch) && !val.Slashed() {
			balances[idx] = val.EffectiveBalance()
		}
	}
	return &confirmation.FFGStateInfo{TotalActiveBalance: total, Balances: balances}, nil
}

func (a *fcrBalanceAccessor) BalanceInfoByCheckpoint(ctx context.Context, cp forkchoicetypes.Checkpoint) ([]uint64, uint64, error) {
	if cached, ok := a.byCheckpoint[cp]; ok {
		return cached.Balances, cached.TotalActiveBalance, nil
	}
	epochStart, err := slots.EpochStart(cp.Epoch)
	if err != nil {
		return nil, 0, err
	}
	var st state.ReadOnlyBeaconState
	// Attestation processing keeps these states in the checkpoint state cache, this is almost always a hit.
	cached, err := a.s.checkpointStateCache.StateByCheckpoint(&ethpb.Checkpoint{Epoch: cp.Epoch, Root: cp.Root[:]})
	if err == nil && cached != nil && !cached.IsNil() {
		st = cached
	} else {
		base, err := a.s.cfg.StateGen.StateByRoot(ctx, cp.Root)
		if err != nil {
			return nil, 0, errors.Wrap(err, "could not get state for checkpoint root")
		}
		if base == nil || base.IsNil() {
			return nil, 0, errors.New("nil state for checkpoint root")
		}
		// A skipped boundary slot leaves the root's state in the prior epoch, the spec's checkpoint state is advanced to the epoch start.
		advanced, err := transition.ProcessSlotsIfPossible(ctx, base, epochStart)
		if err != nil {
			return nil, 0, errors.Wrap(err, "could not advance state to checkpoint epoch")
		}
		st = advanced
	}
	info, err := extractBalanceInfo(ctx, st)
	if err != nil {
		return nil, 0, err
	}
	if a.byCheckpoint == nil {
		a.byCheckpoint = make(map[forkchoicetypes.Checkpoint]*confirmation.FFGStateInfo)
	} else if len(a.byCheckpoint) > 3 {
		for k := range a.byCheckpoint {
			delete(a.byCheckpoint, k)
			break
		}
	}
	a.byCheckpoint[cp] = info
	return info.Balances, info.TotalActiveBalance, nil
}

func (a *fcrBalanceAccessor) PulledUpHeadState(ctx context.Context, headRoot [32]byte) (*confirmation.FFGStateInfo, error) {
	a.s.headLock.RLock()
	var headState state.BeaconState
	if a.s.headRoot() == headRoot {
		headState = a.s.head.state
	}
	a.s.headLock.RUnlock()

	// The service head can move mid run, fall back to loading the snapshotted root.
	if headState == nil || headState.IsNil() {
		st, err := a.s.cfg.StateGen.StateByRoot(ctx, headRoot)
		if err != nil {
			return nil, errors.Wrap(err, "could not get state for head root")
		}
		if st == nil || st.IsNil() {
			return nil, errors.New("head state not available")
		}
		headState = st
	}

	currentEpoch := slots.EpochsSinceGenesis(a.s.genesisTime)
	stateEpoch := coreTime.CurrentEpoch(headState)

	var st state.ReadOnlyBeaconState
	if stateEpoch < currentEpoch {
		epochStart, err := slots.EpochStart(currentEpoch)
		if err != nil {
			return nil, err
		}
		// Prefer the nextSlotCache to avoid a state copy plus ProcessSlots.
		cached := transition.NextSlotState(headRoot[:], epochStart)
		if cached != nil && !cached.IsNil() {
			if cached.Slot() < epochStart {
				advanced, err := transition.ProcessSlots(ctx, cached, epochStart)
				if err != nil {
					return nil, errors.Wrap(err, "could not advance cached state")
				}
				st = advanced
			} else {
				st = cached
			}
		} else {
			copied := headState.Copy()
			advanced, err := transition.ProcessSlots(ctx, copied, epochStart)
			if err != nil {
				return nil, errors.Wrap(err, "could not advance head state")
			}
			st = advanced
		}
	} else {
		st = headState
	}

	return extractBalanceInfo(ctx, st)
}
