package electra

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/altair"
	e "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/epoch"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/epoch/precompute"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
)

// Re-exports for methods that haven't changed in Electra.
var (
	InitializePrecomputeValidators       = altair.InitializePrecomputeValidators
	ProcessEpochParticipation            = altair.ProcessEpochParticipation
	ProcessInactivityScores              = altair.ProcessInactivityScores
	ProcessRewardsAndPenaltiesPrecompute = altair.ProcessRewardsAndPenaltiesPrecompute
	ProcessSlashings                     = e.ProcessSlashings
	ProcessEth1DataReset                 = e.ProcessEth1DataReset
	ProcessSlashingsReset                = e.ProcessSlashingsReset
	ProcessRandaoMixesReset              = e.ProcessRandaoMixesReset
	ProcessHistoricalDataUpdate          = e.ProcessHistoricalDataUpdate
	ProcessParticipationFlagUpdates      = altair.ProcessParticipationFlagUpdates
	ProcessSyncCommitteeUpdates          = altair.ProcessSyncCommitteeUpdates
	AttestationsDelta                    = altair.AttestationsDelta
)

// ProcessEpoch describes the per epoch operations that are performed on the beacon state.
// It's optimized by pre computing validator attested info and epoch total/attested balances upfront.
//
// Spec definition:
//
//	def process_epoch(state: BeaconState) -> None:
//	    process_justification_and_finalization(state)
//	    process_inactivity_updates(state)
//	    process_rewards_and_penalties(state)
//	    process_registry_updates(state)
//	    process_slashings(state)
//	    process_eth1_data_reset(state)
//	    process_pending_deposits(state)  # New in EIP7251
//	    process_pending_consolidations(state)  # New in EIP7251
//	    process_effective_balance_updates(state)
//	    process_slashings_reset(state)
//	    process_randao_mixes_reset(state)
func ProcessEpoch(ctx context.Context, state state.BeaconState) error {
	_, span := trace.StartSpan(ctx, "electra.ProcessEpoch")
	defer span.End()

	if state == nil || state.IsNil() {
		return errors.New("nil state")
	}
	vp, bp, err := InitializePrecomputeValidators(ctx, state)
	if err != nil {
		return err
	}
	vp, bp, err = ProcessEpochParticipation(ctx, state, bp, vp)
	if err != nil {
		return err
	}
	state, err = precompute.ProcessJustificationAndFinalizationPreCompute(state, bp)
	if err != nil {
		return errors.Wrap(err, "could not process justification")
	}
	state, vp, err = ProcessInactivityScores(ctx, state, vp)
	if err != nil {
		return errors.Wrap(err, "could not process inactivity updates")
	}
	state, err = ProcessRewardsAndPenaltiesPrecompute(state, bp, vp)
	if err != nil {
		return errors.Wrap(err, "could not process rewards and penalties")
	}

	if err := ProcessRegistryUpdates(ctx, state); err != nil {
		return errors.Wrap(err, "could not process registry updates")
	}

	proportionalSlashingMultiplier, err := state.ProportionalSlashingMultiplier()
	if err != nil {
		return err
	}
	state, err = ProcessSlashings(state, proportionalSlashingMultiplier)
	if err != nil {
		return err
	}
	state, err = ProcessEth1DataReset(state)
	if err != nil {
		return err
	}

	if err = ProcessPendingDeposits(ctx, state, primitives.Gwei(bp.ActiveCurrentEpoch)); err != nil {
		return err
	}
	if err = ProcessPendingConsolidations(ctx, state); err != nil {
		return err
	}
	if err = ProcessEffectiveBalanceUpdates(state); err != nil {
		return err
	}

	state, err = ProcessSlashingsReset(state)
	if err != nil {
		return err
	}
	state, err = ProcessRandaoMixesReset(state)
	if err != nil {
		return err
	}
	state, err = ProcessHistoricalDataUpdate(state)
	if err != nil {
		return err
	}

	state, err = ProcessParticipationFlagUpdates(state)
	if err != nil {
		return err
	}

	_, err = ProcessSyncCommitteeUpdates(ctx, state)
	if err != nil {
		return err
	}

	return nil
}

// VerifyBlockDepositLength
//
// Spec definition:
//
//	# [Modified in Electra:EIP6110]
//	  # Disable former deposit mechanism once all prior deposits are processed
//	  eth1_deposit_index_limit = min(state.eth1_data.deposit_count, state.deposit_requests_start_index)
//	  if state.eth1_deposit_index < eth1_deposit_index_limit:
//	      assert len(body.deposits) == min(MAX_DEPOSITS, eth1_deposit_index_limit - state.eth1_deposit_index)
//	  else:
//	      assert len(body.deposits) == 0
func VerifyBlockDepositLength(body interfaces.ReadOnlyBeaconBlockBody, state state.BeaconState) error {
	eth1Data := state.Eth1Data()
	requestsStartIndex, err := state.DepositRequestsStartIndex()
	if err != nil {
		return errors.Wrap(err, "failed to get requests start index")
	}
	eth1DepositIndexLimit := min(eth1Data.DepositCount, requestsStartIndex)
	if state.Eth1DepositIndex() < eth1DepositIndexLimit {
		if uint64(len(body.Deposits())) != min(params.BeaconConfig().MaxDeposits, eth1DepositIndexLimit-state.Eth1DepositIndex()) {
			return fmt.Errorf("incorrect outstanding deposits in block body, wanted: %d, got: %d", min(params.BeaconConfig().MaxDeposits, eth1DepositIndexLimit-state.Eth1DepositIndex()), len(body.Deposits()))
		}
	} else {
		if len(body.Deposits()) != 0 {
			return fmt.Errorf("incorrect outstanding deposits in block body, wanted: %d, got: %d", 0, len(body.Deposits()))
		}
	}
	return nil
}
