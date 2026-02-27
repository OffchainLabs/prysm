package transition

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/electra"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/epoch/precompute"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/fulu"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	v "github.com/OffchainLabs/prysm/v7/beacon-chain/core/validators"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
)

// NSCPRocessSlotToValidate advances the given state to the slot of the given block using the NSC.
// This function assumes that the parent state is the latest state that has been processed before the given block.
// In particular, all that it is needed to get the blocks's prestate is to advance slots and possible epoch transitions.
func NSCProcessSlotToValidate(
	ctx context.Context,
	st state.BeaconState,
	b interfaces.ReadOnlyBeaconBlock) (state.BeaconState, error) {
	accessRoot := b.ParentRoot()
	if st.Version() < version.Gloas {
		return ProcessSlotsUsingNextSlotCache(ctx, st, accessRoot[:], b.Slot())
	}
	full, err := st.IsParentBlockFull()
	if err != nil {
		return nil, errors.Wrap(err, "could not determine if parent block is full")
	}
	if full {
		accessRoot, err = st.LatestBlockHash()
		if err != nil {
			return nil, errors.Wrap(err, "could not get latest block hash")
		}
	}
	return ProcessSlotsUsingNextSlotCache(ctx, st, accessRoot[:], b.Slot())
}

// ProcessOperations
//
// Spec definition:
//
//	<spec fn="process_operations" fork="gloas" hash="05a7a4ea">
//	def process_operations(state: BeaconState, body: BeaconBlockBody) -> None:
//	    # Disable former deposit mechanism once all prior deposits are processed
//	    eth1_deposit_index_limit = min(
//	        state.eth1_data.deposit_count, state.deposit_requests_start_index
//	    )
//	    if state.eth1_deposit_index < eth1_deposit_index_limit:
//	        assert len(body.deposits) == min(
//	            MAX_DEPOSITS, eth1_deposit_index_limit - state.eth1_deposit_index
//	        )
//	    else:
//	        assert len(body.deposits) == 0
//
//	    def for_ops(operations: Sequence[Any], fn: Callable[[BeaconState, Any], None]) -> None:
//	        for operation in operations:
//	            fn(state, operation)
//
//	    # [Modified in Gloas:EIP7732]
//	    for_ops(body.proposer_slashings, process_proposer_slashing)
//	    for_ops(body.attester_slashings, process_attester_slashing)
//	    # [Modified in Gloas:EIP7732]
//	    for_ops(body.attestations, process_attestation)
//	    for_ops(body.deposits, process_deposit)
//	    # [Modified in Gloas:EIP7732]
//	    for_ops(body.voluntary_exits, process_voluntary_exit)
//	    for_ops(body.bls_to_execution_changes, process_bls_to_execution_change)
//	    # [Modified in Gloas:EIP7732]
//	    # Removed `process_deposit_request`
//	    # [Modified in Gloas:EIP7732]
//	    # Removed `process_withdrawal_request`
//	    # [Modified in Gloas:EIP7732]
//	    # Removed `process_consolidation_request`
//	    # [New in Gloas:EIP7732]
//	    for_ops(body.payload_attestations, process_payload_attestation)
//	</spec>
func gloasOperations(ctx context.Context, st state.BeaconState, block interfaces.ReadOnlyBeaconBlock) (state.BeaconState, error) {
	var err error

	bb := block.Body()
	var exitInfo *v.ExitInfo
	hasSlashings := len(bb.ProposerSlashings()) > 0 || len(bb.AttesterSlashings()) > 0
	hasExits := len(bb.VoluntaryExits()) > 0
	if hasSlashings || hasExits {
		// ExitInformation is expensive to compute, only do it if we need it.
		exitInfo = v.ExitInformation(st)
		if err := helpers.UpdateTotalActiveBalanceCache(st, exitInfo.TotalActiveBalance); err != nil {
			return nil, errors.Wrap(err, "could not update total active balance cache")
		}
	}
	st, err = blocks.ProcessProposerSlashings(ctx, st, bb.ProposerSlashings(), exitInfo)
	if err != nil {
		return nil, errors.Wrap(ErrProcessProposerSlashingsFailed, err.Error())
	}
	st, err = blocks.ProcessAttesterSlashings(ctx, st, bb.AttesterSlashings(), exitInfo)
	if err != nil {
		return nil, errors.Wrap(ErrProcessAttesterSlashingsFailed, err.Error())
	}
	st, err = electra.ProcessAttestationsNoVerifySignature(ctx, st, block)
	if err != nil {
		return nil, errors.Wrap(ErrProcessAttestationsFailed, err.Error())
	}
	if _, err := electra.ProcessDeposits(ctx, st, bb.Deposits()); err != nil {
		return nil, errors.Wrap(ErrProcessDepositsFailed, err.Error())
	}
	st, err = blocks.ProcessVoluntaryExits(ctx, st, bb.VoluntaryExits(), exitInfo)
	if err != nil {
		return nil, errors.Wrap(ErrProcessVoluntaryExitsFailed, err.Error())
	}
	st, err = blocks.ProcessBLSToExecutionChanges(st, block)
	if err != nil {
		return nil, errors.Wrap(ErrProcessBLSChangesFailed, err.Error())
	}
	if err := gloas.ProcessPayloadAttestations(ctx, st, bb); err != nil {
		return nil, errors.Wrap(ErrProcessPayloadAttestationsFailed, err.Error())
	}

	return st, nil
}

// processEpochGloas describes the per epoch operations that are performed on the beacon state.
// It's optimized by pre computing validator attested info and epoch total/attested balances upfront.
//
// Spec definition:
//
//	<spec fn="process_epoch" fork="gloas" hash="393b69ef">
//	def process_epoch(state: BeaconState) -> None:
//	    process_justification_and_finalization(state)
//	    process_inactivity_updates(state)
//	    process_rewards_and_penalties(state)
//	    process_registry_updates(state)
//	    process_slashings(state)
//	    process_eth1_data_reset(state)
//	    process_pending_deposits(state)
//	    process_pending_consolidations(state)
//	    # [New in Gloas:EIP7732]
//	    process_builder_pending_payments(state)
//	    process_effective_balance_updates(state)
//	    process_slashings_reset(state)
//	    process_randao_mixes_reset(state)
//	    process_historical_summaries_update(state)
//	    process_participation_flag_updates(state)
//	    process_sync_committee_updates(state)
//	    process_proposer_lookahead(state)
//	</spec>
func processEpochGloas(ctx context.Context, state state.BeaconState) error {
	_, span := trace.StartSpan(ctx, "gloas.ProcessEpoch")
	defer span.End()

	if state == nil || state.IsNil() {
		return errors.New("nil state")
	}
	vp, bp, err := electra.InitializePrecomputeValidators(ctx, state)
	if err != nil {
		return err
	}
	vp, bp, err = electra.ProcessEpochParticipation(ctx, state, bp, vp)
	if err != nil {
		return err
	}
	state, err = precompute.ProcessJustificationAndFinalizationPreCompute(state, bp)
	if err != nil {
		return errors.Wrap(err, "could not process justification")
	}
	state, vp, err = electra.ProcessInactivityScores(ctx, state, vp)
	if err != nil {
		return errors.Wrap(err, "could not process inactivity updates")
	}
	state, err = electra.ProcessRewardsAndPenaltiesPrecompute(state, bp, vp)
	if err != nil {
		return errors.Wrap(err, "could not process rewards and penalties")
	}
	if err := electra.ProcessRegistryUpdates(ctx, state); err != nil {
		return errors.Wrap(err, "could not process registry updates")
	}
	if err := electra.ProcessSlashings(state); err != nil {
		return err
	}
	state, err = electra.ProcessEth1DataReset(state)
	if err != nil {
		return err
	}
	if err = electra.ProcessPendingDeposits(ctx, state, primitives.Gwei(bp.ActiveCurrentEpoch)); err != nil {
		return err
	}
	if err = electra.ProcessPendingConsolidations(ctx, state); err != nil {
		return err
	}
	if err = gloas.ProcessBuilderPendingPayments(state); err != nil {
		return err
	}
	if err = electra.ProcessEffectiveBalanceUpdates(state); err != nil {
		return err
	}
	state, err = electra.ProcessSlashingsReset(state)
	if err != nil {
		return err
	}
	state, err = electra.ProcessRandaoMixesReset(state)
	if err != nil {
		return err
	}
	state, err = electra.ProcessHistoricalDataUpdate(state)
	if err != nil {
		return err
	}
	state, err = electra.ProcessParticipationFlagUpdates(state)
	if err != nil {
		return err
	}
	_, err = electra.ProcessSyncCommitteeUpdates(ctx, state)
	if err != nil {
		return err
	}
	if err := fulu.ProcessProposerLookahead(ctx, state); err != nil {
		return err
	}
	return nil
}
