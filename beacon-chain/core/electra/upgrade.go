package electra

import (
	"sort"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/time"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

// UpgradeToElectra updates inputs a generic state to return the version Electra state.
//
// nolint:dupword
// Spec code:
// def upgrade_to_electra(pre: deneb.BeaconState) -> BeaconState:
//
//	epoch = deneb.get_current_epoch(pre)
//	latest_execution_payload_header = pre.latest_execution_payload_header
//
//	earliest_exit_epoch = compute_activation_exit_epoch(get_current_epoch(pre))
//	for validator in pre.validators:
//	    if validator.exit_epoch != FAR_FUTURE_EPOCH:
//	        if validator.exit_epoch > earliest_exit_epoch:
//	            earliest_exit_epoch = validator.exit_epoch
//	earliest_exit_epoch += Epoch(1)
//
//	post = BeaconState(
//	    # Versioning
//	    genesis_time=pre.genesis_time,
//	    genesis_validators_root=pre.genesis_validators_root,
//	    slot=pre.slot,
//	    fork=Fork(
//	        previous_version=pre.fork.current_version,
//	        current_version=ELECTRA_FORK_VERSION,  # [Modified in Electra:EIP6110]
//	        epoch=epoch,
//	    ),
//	    # History
//	    latest_block_header=pre.latest_block_header,
//	    block_roots=pre.block_roots,
//	    state_roots=pre.state_roots,
//	    historical_roots=pre.historical_roots,
//	    # Eth1
//	    eth1_data=pre.eth1_data,
//	    eth1_data_votes=pre.eth1_data_votes,
//	    eth1_deposit_index=pre.eth1_deposit_index,
//	    # Registry
//	    validators=pre.validators,
//	    balances=pre.balances,
//	    # Randomness
//	    randao_mixes=pre.randao_mixes,
//	    # Slashings
//	    slashings=pre.slashings,
//	    # Participation
//	    previous_epoch_participation=pre.previous_epoch_participation,
//	    current_epoch_participation=pre.current_epoch_participation,
//	    # Finality
//	    justification_bits=pre.justification_bits,
//	    previous_justified_checkpoint=pre.previous_justified_checkpoint,
//	    current_justified_checkpoint=pre.current_justified_checkpoint,
//	    finalized_checkpoint=pre.finalized_checkpoint,
//	    # Inactivity
//	    inactivity_scores=pre.inactivity_scores,
//	    # Sync
//	    current_sync_committee=pre.current_sync_committee,
//	    next_sync_committee=pre.next_sync_committee,
//	    # Execution-layer
//	    latest_execution_payload_header=latest_execution_payload_header,  # [Modified in Electra:EIP6110:EIP7002]
//	    # Withdrawals
//	    next_withdrawal_index=pre.next_withdrawal_index,
//	    next_withdrawal_validator_index=pre.next_withdrawal_validator_index,
//	    # Deep history valid from Capella onwards
//	    historical_summaries=pre.historical_summaries,
//	    # [New in Electra:EIP6110]
//	    deposit_requests_start_index=UNSET_DEPOSIT_REQUESTS_START_INDEX,
//	    # [New in Electra:EIP7251]
//	    deposit_balance_to_consume=0,
//	    exit_balance_to_consume=0,
//	    earliest_exit_epoch=earliest_exit_epoch,
//	    consolidation_balance_to_consume=0,
//	    earliest_consolidation_epoch=compute_activation_exit_epoch(get_current_epoch(pre)),
//	    pending_deposits=[],
//	    pending_partial_withdrawals=[],
//	    pending_consolidations=[],
//	)
//
//	post.exit_balance_to_consume = get_activation_exit_churn_limit(post)
//	post.consolidation_balance_to_consume = get_consolidation_churn_limit(post)
//
//	# [New in Electra:EIP7251]
//	# add validators that are not yet active to pending balance deposits
//	pre_activation = sorted([
//	    index for index, validator in enumerate(post.validators)
//	    if validator.activation_epoch == FAR_FUTURE_EPOCH
//	], key=lambda index: (
//	    post.validators[index].activation_eligibility_epoch,
//	    index
//	))
//
//	for index in pre_activation:
//	    balance = post.balances[index]
//	    post.balances[index] = 0
//	    validator = post.validators[index]
//	    validator.effective_balance = 0
//	    validator.activation_eligibility_epoch = FAR_FUTURE_EPOCH
//	    # Use bls.G2_POINT_AT_INFINITY as a signature field placeholder
//	    # and GENESIS_SLOT to distinguish from a pending deposit request
//	    post.pending_deposits.append(PendingDeposit(
//	        pubkey=validator.pubkey,
//	        withdrawal_credentials=validator.withdrawal_credentials,
//	        amount=balance,
//	        signature=bls.G2_POINT_AT_INFINITY,
//	        slot=GENESIS_SLOT,
//	    ))
//
//	# Ensure early adopters of compounding credentials go through the activation churn
//	for index, validator in enumerate(post.validators):
//	    if has_compounding_withdrawal_credential(validator):
//	        queue_excess_active_balance(post, ValidatorIndex(index))
//
//	return post
func UpgradeToElectra(beaconState state.BeaconState) (state.BeaconState, error) {
	currentSyncCommittee, err := beaconState.CurrentSyncCommittee()
	if err != nil {
		return nil, err
	}
	nextSyncCommittee, err := beaconState.NextSyncCommittee()
	if err != nil {
		return nil, err
	}
	prevEpochParticipation, err := beaconState.PreviousEpochParticipation()
	if err != nil {
		return nil, err
	}
	currentEpochParticipation, err := beaconState.CurrentEpochParticipation()
	if err != nil {
		return nil, err
	}
	inactivityScores, err := beaconState.InactivityScores()
	if err != nil {
		return nil, err
	}
	payloadHeader, err := beaconState.LatestExecutionPayloadHeader()
	if err != nil {
		return nil, err
	}
	txRoot, err := payloadHeader.TransactionsRoot()
	if err != nil {
		return nil, err
	}
	wdRoot, err := payloadHeader.WithdrawalsRoot()
	if err != nil {
		return nil, err
	}
	wi, err := beaconState.NextWithdrawalIndex()
	if err != nil {
		return nil, err
	}
	vi, err := beaconState.NextWithdrawalValidatorIndex()
	if err != nil {
		return nil, err
	}
	summaries, err := beaconState.HistoricalSummaries()
	if err != nil {
		return nil, err
	}
	historicalRoots, err := beaconState.HistoricalRoots()
	if err != nil {
		return nil, err
	}
	excessBlobGas, err := payloadHeader.ExcessBlobGas()
	if err != nil {
		return nil, err
	}
	blobGasUsed, err := payloadHeader.BlobGasUsed()
	if err != nil {
		return nil, err
	}

	// [New in Electra:EIP7251]
	earliestExitEpoch := helpers.ActivationExitEpoch(time.CurrentEpoch(beaconState))
	preActivationIndices := make([]primitives.ValidatorIndex, 0)
	compoundWithdrawalIndices := make([]primitives.ValidatorIndex, 0)
	if err = beaconState.ReadFromEveryValidator(func(index int, val state.ReadOnlyValidator) error {
		if val.ExitEpoch() != params.BeaconConfig().FarFutureEpoch && val.ExitEpoch() > earliestExitEpoch {
			earliestExitEpoch = val.ExitEpoch()
		}
		if val.ActivationEpoch() == params.BeaconConfig().FarFutureEpoch {
			preActivationIndices = append(preActivationIndices, primitives.ValidatorIndex(index))
		}
		if helpers.HasCompoundingWithdrawalCredential(val) {
			compoundWithdrawalIndices = append(compoundWithdrawalIndices, primitives.ValidatorIndex(index))
		}
		return nil
	}); err != nil {
		return nil, err
	}

	earliestExitEpoch++ // Increment to find the earliest possible exit epoch

	// note: should be the same in prestate and post beaconState.
	// we are deviating from the specs a bit as it calls for using the post beaconState
	tab, err := helpers.TotalActiveBalance(beaconState)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get total active balance")
	}

	s := &ethpb.BeaconStateElectra{
		GenesisTime:           beaconState.GenesisTime(),
		GenesisValidatorsRoot: beaconState.GenesisValidatorsRoot(),
		Slot:                  beaconState.Slot(),
		Fork: &ethpb.Fork{
			PreviousVersion: beaconState.Fork().CurrentVersion,
			CurrentVersion:  params.BeaconConfig().ElectraForkVersion,
			Epoch:           time.CurrentEpoch(beaconState),
		},
		LatestBlockHeader:           beaconState.LatestBlockHeader(),
		BlockRoots:                  beaconState.BlockRoots(),
		StateRoots:                  beaconState.StateRoots(),
		HistoricalRoots:             historicalRoots,
		Eth1Data:                    beaconState.Eth1Data(),
		Eth1DataVotes:               beaconState.Eth1DataVotes(),
		Eth1DepositIndex:            beaconState.Eth1DepositIndex(),
		Validators:                  beaconState.Validators(),
		Balances:                    beaconState.Balances(),
		RandaoMixes:                 beaconState.RandaoMixes(),
		Slashings:                   beaconState.Slashings(),
		PreviousEpochParticipation:  prevEpochParticipation,
		CurrentEpochParticipation:   currentEpochParticipation,
		JustificationBits:           beaconState.JustificationBits(),
		PreviousJustifiedCheckpoint: beaconState.PreviousJustifiedCheckpoint(),
		CurrentJustifiedCheckpoint:  beaconState.CurrentJustifiedCheckpoint(),
		FinalizedCheckpoint:         beaconState.FinalizedCheckpoint(),
		InactivityScores:            inactivityScores,
		CurrentSyncCommittee:        currentSyncCommittee,
		NextSyncCommittee:           nextSyncCommittee,
		LatestExecutionPayloadHeader: &enginev1.ExecutionPayloadHeaderElectra{
			ParentHash:       payloadHeader.ParentHash(),
			FeeRecipient:     payloadHeader.FeeRecipient(),
			StateRoot:        payloadHeader.StateRoot(),
			ReceiptsRoot:     payloadHeader.ReceiptsRoot(),
			LogsBloom:        payloadHeader.LogsBloom(),
			PrevRandao:       payloadHeader.PrevRandao(),
			BlockNumber:      payloadHeader.BlockNumber(),
			GasLimit:         payloadHeader.GasLimit(),
			GasUsed:          payloadHeader.GasUsed(),
			Timestamp:        payloadHeader.Timestamp(),
			ExtraData:        payloadHeader.ExtraData(),
			BaseFeePerGas:    payloadHeader.BaseFeePerGas(),
			BlockHash:        payloadHeader.BlockHash(),
			TransactionsRoot: txRoot,
			WithdrawalsRoot:  wdRoot,
			ExcessBlobGas:    excessBlobGas,
			BlobGasUsed:      blobGasUsed,
		},
		NextWithdrawalIndex:          wi,
		NextWithdrawalValidatorIndex: vi,
		HistoricalSummaries:          summaries,

		DepositRequestsStartIndex:     params.BeaconConfig().UnsetDepositRequestsStartIndex,
		DepositBalanceToConsume:       0,
		ExitBalanceToConsume:          helpers.ActivationExitChurnLimit(primitives.Gwei(tab)),
		EarliestExitEpoch:             earliestExitEpoch,
		ConsolidationBalanceToConsume: helpers.ConsolidationChurnLimit(primitives.Gwei(tab)),
		EarliestConsolidationEpoch:    helpers.ActivationExitEpoch(slots.ToEpoch(beaconState.Slot())),
		PendingDeposits:               make([]*ethpb.PendingDeposit, 0),
		PendingPartialWithdrawals:     make([]*ethpb.PendingPartialWithdrawal, 0),
		PendingConsolidations:         make([]*ethpb.PendingConsolidation, 0),
	}

	// Sorting preActivationIndices based on a custom criteria
	sort.Slice(preActivationIndices, func(i, j int) bool {
		// Comparing based on ActivationEligibilityEpoch and then by index if the epochs are the same
		if s.Validators[preActivationIndices[i]].ActivationEligibilityEpoch == s.Validators[preActivationIndices[j]].ActivationEligibilityEpoch {
			return preActivationIndices[i] < preActivationIndices[j]
		}
		return s.Validators[preActivationIndices[i]].ActivationEligibilityEpoch < s.Validators[preActivationIndices[j]].ActivationEligibilityEpoch
	})

	// need to cast the beaconState to use in helper functions
	post, err := state_native.InitializeFromProtoUnsafeElectra(s)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize post electra beaconState")
	}

	for _, index := range preActivationIndices {
		if err := QueueEntireBalanceAndResetValidator(post, index); err != nil {
			return nil, errors.Wrap(err, "failed to queue entire balance and reset validator")
		}
	}

	// Ensure early adopters of compounding credentials go through the activation churn
	for _, index := range compoundWithdrawalIndices {
		if err := QueueExcessActiveBalance(post, index); err != nil {
			return nil, errors.Wrap(err, "failed to queue excess active balance")
		}
	}

	return post, nil
}
