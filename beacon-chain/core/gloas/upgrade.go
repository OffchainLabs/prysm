package gloas

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// UpgradeToGloas updates inputs a generic state to return the version Gloas state.
//
//	<spec fn="upgrade_to_gloas" fork="gloas" hash="8f67112c">
//	def upgrade_to_gloas(pre: fulu.BeaconState) -> BeaconState:
//	    epoch = fulu.get_current_epoch(pre)
//
//	    post = BeaconState(
//	        genesis_time=pre.genesis_time,
//	        genesis_validators_root=pre.genesis_validators_root,
//	        slot=pre.slot,
//	        fork=Fork(
//	            previous_version=pre.fork.current_version,
//	            # [Modified in Gloas:EIP7732]
//	            current_version=GLOAS_FORK_VERSION,
//	            epoch=epoch,
//	        ),
//	        latest_block_header=pre.latest_block_header,
//	        block_roots=pre.block_roots,
//	        state_roots=pre.state_roots,
//	        historical_roots=pre.historical_roots,
//	        eth1_data=pre.eth1_data,
//	        eth1_data_votes=pre.eth1_data_votes,
//	        eth1_deposit_index=pre.eth1_deposit_index,
//	        validators=pre.validators,
//	        balances=pre.balances,
//	        randao_mixes=pre.randao_mixes,
//	        slashings=pre.slashings,
//	        previous_epoch_participation=pre.previous_epoch_participation,
//	        current_epoch_participation=pre.current_epoch_participation,
//	        justification_bits=pre.justification_bits,
//	        previous_justified_checkpoint=pre.previous_justified_checkpoint,
//	        current_justified_checkpoint=pre.current_justified_checkpoint,
//	        finalized_checkpoint=pre.finalized_checkpoint,
//	        inactivity_scores=pre.inactivity_scores,
//	        current_sync_committee=pre.current_sync_committee,
//	        next_sync_committee=pre.next_sync_committee,
//	        # [Modified in Gloas:EIP7732]
//	        # Removed `latest_execution_payload_header`
//	        # [New in Gloas:EIP7732]
//	        latest_execution_payload_bid=ExecutionPayloadBid(
//	            block_hash=pre.latest_execution_payload_header.block_hash,
//	        ),
//	        next_withdrawal_index=pre.next_withdrawal_index,
//	        next_withdrawal_validator_index=pre.next_withdrawal_validator_index,
//	        historical_summaries=pre.historical_summaries,
//	        deposit_requests_start_index=pre.deposit_requests_start_index,
//	        deposit_balance_to_consume=pre.deposit_balance_to_consume,
//	        exit_balance_to_consume=pre.exit_balance_to_consume,
//	        earliest_exit_epoch=pre.earliest_exit_epoch,
//	        consolidation_balance_to_consume=pre.consolidation_balance_to_consume,
//	        earliest_consolidation_epoch=pre.earliest_consolidation_epoch,
//	        pending_deposits=pre.pending_deposits,
//	        pending_partial_withdrawals=pre.pending_partial_withdrawals,
//	        pending_consolidations=pre.pending_consolidations,
//	        proposer_lookahead=pre.proposer_lookahead,
//	        # [New in Gloas:EIP7732]
//	        builders=[],
//	        # [New in Gloas:EIP7732]
//	        next_withdrawal_builder_index=BuilderIndex(0),
//	        # [New in Gloas:EIP7732]
//	        execution_payload_availability=[0b1 for _ in range(SLOTS_PER_HISTORICAL_ROOT)],
//	        # [New in Gloas:EIP7732]
//	        builder_pending_payments=[BuilderPendingPayment() for _ in range(2 * SLOTS_PER_EPOCH)],
//	        # [New in Gloas:EIP7732]
//	        builder_pending_withdrawals=[],
//	        # [New in Gloas:EIP7732]
//	        latest_block_hash=pre.latest_execution_payload_header.block_hash,
//	        # [New in Gloas:EIP7732]
//	        payload_expected_withdrawals=[],
//	        # [New in Gloas:EIP7732]
//	        ptc_window=initialize_ptc_window(pre),
//	    )
//
//	    # [New in Gloas:EIP7732]
//	    onboard_builders_from_pending_deposits(post)
//
//	    return post
//	</spec>
//
//	<spec fn="process_execution_payload_bid" fork="gloas" hash="823c9f3a">
//	def process_execution_payload_bid(state: BeaconState, block: BeaconBlock) -> None:
//	    signed_bid = block.body.signed_execution_payload_bid
//	    bid = signed_bid.message
//	    builder_index = bid.builder_index
//	    amount = bid.value
//
//	    # For self-builds, amount must be zero regardless of withdrawal credential prefix
//	    if builder_index == BUILDER_INDEX_SELF_BUILD:
//	        assert amount == 0
//	        assert signed_bid.signature == bls.G2_POINT_AT_INFINITY
//	    else:
//	        # Verify that the builder is active
//	        assert is_active_builder(state, builder_index)
//	        # Verify that the builder has funds to cover the bid
//	        assert can_builder_cover_bid(state, builder_index, amount)
//	        # Verify that the bid signature is valid
//	        assert verify_execution_payload_bid_signature(state, signed_bid)
//
//	    # Verify commitments are under limit
//	    assert (
//	        len(bid.blob_kzg_commitments)
//	        <= get_blob_parameters(get_current_epoch(state)).max_blobs_per_block
//	    )
//
//	    # Verify that the bid is for the current slot
//	    assert bid.slot == block.slot
//	    # Verify that the bid is for the right parent block
//	    assert bid.parent_block_hash == state.latest_block_hash
//	    assert bid.parent_block_root == block.parent_root
//	    assert bid.prev_randao == get_randao_mix(state, get_current_epoch(state))
//
//	    # Record the pending payment if there is some payment
//	    if amount > 0:
//	        pending_payment = BuilderPendingPayment(
//	            weight=0,
//	            withdrawal=BuilderPendingWithdrawal(
//	                fee_recipient=bid.fee_recipient,
//	                amount=amount,
//	                builder_index=builder_index,
//	            ),
//	        )
//	        state.builder_pending_payments[SLOTS_PER_EPOCH + bid.slot % SLOTS_PER_EPOCH] = (
//	            pending_payment
//	        )
//
//	    # Cache the signed execution payload bid
//	    state.latest_execution_payload_bid = bid
//	</spec>
func UpgradeToGloas(beaconState state.BeaconState) (state.BeaconState, error) {
	s, err := upgradeToGloas(beaconState)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert to gloas")
	}
	ptcWindow, err := initializePTCWindow(context.Background(), s)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize ptc window")
	}
	if err := s.SetPTCWindow(ptcWindow); err != nil {
		return nil, errors.Wrap(err, "failed to set ptc window")
	}
	if err := s.OnboardBuildersFromPendingDeposits(); err != nil {
		return nil, errors.Wrap(err, "failed to onboard builders from pending deposits")
	}
	return s, nil
}

// initializePTCWindow builds the initial PTC window for the Gloas fork upgrade.
//
//	<spec fn="initialize_ptc_window" fork="gloas" hash="3764b7f5">
//	def initialize_ptc_window(
//	    state: BeaconState,
//	) -> Vector[Vector[ValidatorIndex, PTC_SIZE], (2 + MIN_SEED_LOOKAHEAD) * SLOTS_PER_EPOCH]:
//	    """
//	    Return the cached PTC window starting from the current epoch.
//	    Used to initialize the ``ptc_window`` field in the beacon state at genesis and after forks.
//	    """
//	    empty_previous_epoch = [
//	        Vector[ValidatorIndex, PTC_SIZE]([ValidatorIndex(0) for _ in range(PTC_SIZE)])
//	        for _ in range(SLOTS_PER_EPOCH)
//	    ]
//
//	    ptcs = []
//	    current_epoch = get_current_epoch(state)
//	    for e in range(1 + MIN_SEED_LOOKAHEAD):
//	        epoch = Epoch(current_epoch + e)
//	        start_slot = compute_start_slot_at_epoch(epoch)
//	        ptcs += [compute_ptc(state, Slot(start_slot + i)) for i in range(SLOTS_PER_EPOCH)]
//
//	    return empty_previous_epoch + ptcs
//	</spec>
func initializePTCWindow(ctx context.Context, st state.ReadOnlyBeaconState) ([]*ethpb.PTCs, error) {
	currentEpoch := slots.ToEpoch(st.Slot())
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	windowSize := slotsPerEpoch.Mul(uint64(2 + params.BeaconConfig().MinSeedLookahead))
	window := make([]*ethpb.PTCs, 0, windowSize)

	// Previous epoch has no cached data at fork time — fill with empty slots.
	for range slotsPerEpoch {
		window = append(window, &ethpb.PTCs{
			ValidatorIndices: make([]primitives.ValidatorIndex, fieldparams.PTCSize),
		})
	}

	// Compute PTC for current epoch through lookahead.
	startSlot, err := slots.EpochStart(currentEpoch)
	if err != nil {
		return nil, err
	}
	totalSlots := slotsPerEpoch.Mul(uint64(1 + params.BeaconConfig().MinSeedLookahead))
	for i := range totalSlots {
		ptc, err := computePTC(ctx, st, startSlot+i)
		if err != nil {
			return nil, err
		}
		window = append(window, ptcSlotFromValidatorIndices(ptc))
	}

	return window, nil
}

func upgradeToGloas(beaconState state.BeaconState) (state.BeaconState, error) {
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
	depositRequestsStartIndex, err := beaconState.DepositRequestsStartIndex()
	if err != nil {
		return nil, err
	}
	depositBalanceToConsume, err := beaconState.DepositBalanceToConsume()
	if err != nil {
		return nil, err
	}
	exitBalanceToConsume, err := beaconState.ExitBalanceToConsume()
	if err != nil {
		return nil, err
	}
	earliestExitEpoch, err := beaconState.EarliestExitEpoch()
	if err != nil {
		return nil, err
	}
	consolidationBalanceToConsume, err := beaconState.ConsolidationBalanceToConsume()
	if err != nil {
		return nil, err
	}
	earliestConsolidationEpoch, err := beaconState.EarliestConsolidationEpoch()
	if err != nil {
		return nil, err
	}
	pendingDeposits, err := beaconState.PendingDeposits()
	if err != nil {
		return nil, err
	}
	pendingPartialWithdrawals, err := beaconState.PendingPartialWithdrawals()
	if err != nil {
		return nil, err
	}
	pendingConsolidations, err := beaconState.PendingConsolidations()
	if err != nil {
		return nil, err
	}
	proposerLookahead, err := beaconState.ProposerLookahead()
	if err != nil {
		return nil, err
	}
	proposerLookaheadU64 := make([]uint64, len(proposerLookahead))
	for i, v := range proposerLookahead {
		proposerLookaheadU64[i] = uint64(v)
	}

	executionPayloadAvailability := make([]byte, int((params.BeaconConfig().SlotsPerHistoricalRoot+7)/8))
	for i := range executionPayloadAvailability {
		executionPayloadAvailability[i] = 0xff
	}

	builderPendingPayments := make([]*ethpb.BuilderPendingPayment, int(params.BeaconConfig().SlotsPerEpoch*2))
	for i := range builderPendingPayments {
		builderPendingPayments[i] = &ethpb.BuilderPendingPayment{
			Withdrawal: &ethpb.BuilderPendingWithdrawal{
				FeeRecipient: make([]byte, fieldparams.FeeRecipientLength),
			},
		}
	}

	s := &ethpb.BeaconStateGloas{
		GenesisTime:           uint64(beaconState.GenesisTime().Unix()),
		GenesisValidatorsRoot: beaconState.GenesisValidatorsRoot(),
		Slot:                  beaconState.Slot(),
		Fork: &ethpb.Fork{
			PreviousVersion: beaconState.Fork().CurrentVersion,
			CurrentVersion:  params.BeaconConfig().GloasForkVersion,
			Epoch:           time.CurrentEpoch(beaconState),
		},
		LatestBlockHeader:           beaconState.LatestBlockHeader(),
		BlockRoots:                  beaconState.BlockRoots(),
		StateRoots:                  beaconState.StateRoots(),
		HistoricalRoots:             beaconState.HistoricalRoots(),
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
		LatestExecutionPayloadBid: &ethpb.ExecutionPayloadBid{
			BlockHash:       payloadHeader.BlockHash(),
			FeeRecipient:    make([]byte, fieldparams.FeeRecipientLength),
			ParentBlockHash: make([]byte, fieldparams.RootLength),
			ParentBlockRoot: make([]byte, fieldparams.RootLength),
			PrevRandao:      make([]byte, fieldparams.RootLength),
		},
		NextWithdrawalIndex:           wi,
		NextWithdrawalValidatorIndex:  vi,
		HistoricalSummaries:           summaries,
		DepositRequestsStartIndex:     depositRequestsStartIndex,
		DepositBalanceToConsume:       depositBalanceToConsume,
		ExitBalanceToConsume:          exitBalanceToConsume,
		EarliestExitEpoch:             earliestExitEpoch,
		ConsolidationBalanceToConsume: consolidationBalanceToConsume,
		EarliestConsolidationEpoch:    earliestConsolidationEpoch,
		PendingDeposits:               pendingDeposits,
		PendingPartialWithdrawals:     pendingPartialWithdrawals,
		PendingConsolidations:         pendingConsolidations,
		ProposerLookahead:             proposerLookaheadU64,
		Builders:                      []*ethpb.Builder{},
		NextWithdrawalBuilderIndex:    primitives.BuilderIndex(0),
		ExecutionPayloadAvailability:  executionPayloadAvailability,
		BuilderPendingPayments:        builderPendingPayments,
		BuilderPendingWithdrawals:     []*ethpb.BuilderPendingWithdrawal{},
		LatestBlockHash:               payloadHeader.BlockHash(),
		PayloadExpectedWithdrawals:    []*enginev1.Withdrawal{},
	}
	return state_native.InitializeFromProtoUnsafeGloas(s)
}
