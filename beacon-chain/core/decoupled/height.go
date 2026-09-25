package decoupled

import (
	"context"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// Spec: σ.nj ← (K | σ.h) ∧ (σ.h − σ.h_F > D), PDF §4.
func IsNonjustifiableHeight(height, finalizedHeight primitives.Height) bool {
	cfg := params.BeaconConfig()
	return uint64(height)%cfg.KNonjustifiable == 0 && uint64(height) > uint64(finalizedHeight)+cfg.FinalityDebtThreshold
}

// Spec: the empty height pair (⊥, ⊥, ⊥), PDF §4. Every chain starts at height 1, so height 0 stands for ⊥.
func IsEmptyHeightPair(data *ethpb.AttestationDataDecoupled) bool {
	return data.Height == 0
}

// Spec: w(Q) ≥ q with q = ⌈2W/3⌉, PDF §4. Ethereum weighs by effective balance, and slashed stake stays in W without supporting.
func hasQuorum(ctx context.Context, st state.ReadOnlyBeaconState, bits bitfield.Bitlist) (bool, error) {
	total, err := helpers.TotalActiveBalance(ctx, st)
	if err != nil {
		return false, err
	}
	var weight uint64
	epoch := time.CurrentEpoch(st)
	for idx, val := range st.ValidatorsReadOnlySeq() {
		if !bitSet(bits, idx) {
			continue
		}
		if val.Slashed() || !helpers.IsActiveValidatorUsingTrie(val, epoch) {
			continue
		}
		weight += val.EffectiveBalance()
	}
	cfg := params.BeaconConfig()
	return weight*cfg.FinalityQuorumDenominator >= total*cfg.FinalityQuorumNumerator, nil
}

func resetBitlist(st state.ReadOnlyBeaconState) bitfield.Bitlist {
	return bitfield.NewBitlist(uint64(st.NumValidators()))
}

func bitSet(b bitfield.Bitlist, i primitives.ValidatorIndex) bool {
	return uint64(i) < b.Len() && b.BitAt(uint64(i))
}

// Spec: process_height_events(σ), PDF §4.
func ProcessHeightEvents(ctx context.Context, st state.BeaconState) error {
	justifiedHeight, err := st.JustifiedHeight()
	if err != nil {
		return err
	}
	finalizedHeight, err := st.FinalizedHeight()
	if err != nil {
		return err
	}
	if justifiedHeight > finalizedHeight {
		finalize, err := st.FinalityParticipation()
		if err != nil {
			return err
		}
		ok, err := hasQuorum(ctx, st, finalize)
		if err != nil {
			return err
		}
		if ok {
			justified, err := st.JustifiedCheckpointDecoupled()
			if err != nil {
				return err
			}
			if err := st.SetFinalizedCheckpointDecoupled(justified.Copy()); err != nil {
				return err
			}
			if err := st.SetFinalizedHeight(justifiedHeight); err != nil {
				return err
			}
		}
	}
	target, err := st.TargetParticipation()
	if err != nil {
		return err
	}
	ok, err := hasQuorum(ctx, st, target)
	if err != nil {
		return err
	}
	if ok {
		nonjustifiable, err := st.CurrentHeightNonjustifiable()
		if err != nil {
			return err
		}
		if !nonjustifiable {
			if err := justifyEntry(st); err != nil {
				return err
			}
		}
		return advanceHeight(st)
	}
	entry, err := st.CurrentHeightTarget()
	if err != nil {
		return err
	}
	delay := primitives.Slot(params.BeaconConfig().TimeoutDelayRounds * slots.SlotsPerRoundAt(st.Slot()))
	if st.Slot() < entry.Slot+delay {
		return nil
	}
	progress, err := st.Progress()
	if err != nil {
		return err
	}
	ok, err = hasQuorum(ctx, st, progress)
	if err != nil || !ok {
		return err
	}
	return advanceHeight(st)
}

// Spec: (σ.J, σ.h_j) ← (σ.T_h, σ.h) and σ.finalize ← false^V, PDF §4.
func justifyEntry(st state.BeaconState) error {
	entry, err := st.CurrentHeightTarget()
	if err != nil {
		return err
	}
	height, err := st.CurrentHeight()
	if err != nil {
		return err
	}
	if err := st.SetJustifiedCheckpointDecoupled(entry.Copy()); err != nil {
		return err
	}
	if err := st.SetJustifiedHeight(height); err != nil {
		return err
	}
	return st.SetFinalityParticipation(resetBitlist(st))
}

// Spec: advance_height(σ), PDF §4. T_h ← σ.L keeps an empty root until FillHeightTargetRoot, since a post-state cannot hold its own block's root.
func advanceHeight(st state.BeaconState) error {
	height, err := st.CurrentHeight()
	if err != nil {
		return err
	}
	next := height + 1
	if err := st.SetCurrentHeight(next); err != nil {
		return err
	}
	if err := st.SetCurrentHeightTarget(&ethpb.CheckpointDecoupled{Slot: st.Slot(), Root: make([]byte, fieldparams.RootLength)}); err != nil {
		return err
	}
	finalizedHeight, err := st.FinalizedHeight()
	if err != nil {
		return err
	}
	if err := st.SetCurrentHeightNonjustifiable(IsNonjustifiableHeight(next, finalizedHeight)); err != nil {
		return err
	}
	if err := st.SetTargetParticipation(resetBitlist(st)); err != nil {
		return err
	}
	return st.SetProgress(resetBitlist(st))
}

// Spec: T_h ← σ.L from advance_height, PDF §4. The root is filled one slot later, once the block's header holds its state root.
func FillHeightTargetRoot(st state.BeaconState, previousBlockRoot [32]byte) error {
	entry, err := st.CurrentHeightTarget()
	if err != nil {
		return err
	}
	if !isEmptyRoot(entry.Root) || entry.Slot != st.LatestBlockHeader().Slot {
		return nil
	}
	return st.SetCurrentHeightTarget(&ethpb.CheckpointDecoupled{Slot: entry.Slot, Root: previousBlockRoot[:]})
}
