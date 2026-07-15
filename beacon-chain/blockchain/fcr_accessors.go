package blockchain

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/confirmation"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	coreTime "github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
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

func (a *fcrCommitteeAccessor) Seed(_ context.Context, epoch primitives.Epoch) ([32]byte, error) {
	a.s.headLock.RLock()
	headState := a.s.head.state
	a.s.headLock.RUnlock()
	if headState == nil || headState.IsNil() {
		return [32]byte{}, errors.New("head state not available")
	}
	return helpers.Seed(headState, epoch, params.BeaconConfig().DomainBeaconAttester)
}

type fcrBalanceAccessor struct {
	s *Service
	// Checkpoint states are immutable, cached entries never go stale.
	byRoot map[[32]byte]*confirmation.FFGStateInfo
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

func (a *fcrBalanceAccessor) BalanceInfoByCheckpoint(ctx context.Context, root [32]byte) ([]uint64, uint64, error) {
	if cached, ok := a.byRoot[root]; ok {
		return cached.Balances, cached.TotalActiveBalance, nil
	}
	st, err := a.s.cfg.StateGen.StateByRoot(ctx, root)
	if err != nil {
		return nil, 0, errors.Wrap(err, "could not get state for checkpoint root")
	}
	if st == nil || st.IsNil() {
		return nil, 0, errors.New("nil state for checkpoint root")
	}
	info, err := extractBalanceInfo(ctx, st)
	if err != nil {
		return nil, 0, err
	}
	if a.byRoot == nil {
		a.byRoot = make(map[[32]byte]*confirmation.FFGStateInfo)
	} else if len(a.byRoot) > 3 {
		for k := range a.byRoot {
			delete(a.byRoot, k)
			break
		}
	}
	a.byRoot[root] = info
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
