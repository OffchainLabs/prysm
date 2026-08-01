package gloas

import (
	"bytes"
	"context"

	requests "github.com/OffchainLabs/prysm/v7/beacon-chain/core/requests"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/pkg/errors"
)

// ProcessParentExecutionPayload must run before process_block_header and
// process_execution_payload_bid, which overwrite the state fields it reads.
//
//	<spec fn="process_parent_execution_payload" fork="gloas" hash="defer_payload">
func ProcessParentExecutionPayload(ctx context.Context, st state.BeaconState, blk interfaces.ReadOnlyBeaconBlock) error {
	_, span := trace.StartSpan(ctx, "gloas.ProcessParentExecutionPayload")
	defer span.End()

	body := blk.Body()
	signedBid, err := body.SignedExecutionPayloadBid()
	if err != nil {
		return errors.Wrap(err, "could not get signed execution payload bid")
	}
	bid := signedBid.Message

	parentBid, err := st.LatestExecutionPayloadBid()
	if err != nil {
		return errors.Wrap(err, "could not get parent execution payload bid")
	}

	parentExecutionRequests, err := body.ParentExecutionRequests()
	if err != nil {
		return errors.Wrap(err, "could not get parent execution requests")
	}

	parentBidBlockHash := parentBid.BlockHash()
	isParentFull := bytes.Equal(bid.ParentBlockHash, parentBidBlockHash[:])

	if !isParentFull {
		if !IsEmptyExecutionRequests(parentExecutionRequests) {
			return errors.New("parent was empty but parent_execution_requests is non-empty")
		}
		return nil
	}

	requestsRoot, err := parentExecutionRequests.HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "could not compute parent execution requests root")
	}
	parentBidRequestRoot := parentBid.ExecutionRequestsRoot()
	if requestsRoot != parentBidRequestRoot {
		return errors.Errorf("parent execution requests root mismatch: block=%#x, bid=%#x", requestsRoot, parentBidRequestRoot)
	}

	return ApplyParentExecutionPayload(ctx, st, parentExecutionRequests)
}

// ApplyParentExecutionPayload reads parent_bid from state.latest_execution_payload_bid
// and mutates st. Called by ProcessParentExecutionPayload and by the validator during
// block production before computing withdrawals.
//
//	<spec fn="apply_parent_execution_payload" fork="gloas" hash="bd6df5afe2">
//	    assert len(requests.withdrawals) <= MAX_WITHDRAWAL_REQUESTS_PER_PAYLOAD
//	    assert len(requests.consolidations) <= MAX_CONSOLIDATION_REQUESTS_PER_PAYLOAD
//	    assert len(requests.builder_deposits) <= MAX_BUILDER_DEPOSIT_REQUESTS_PER_PAYLOAD
//	    assert len(requests.builder_exits) <= MAX_BUILDER_EXIT_REQUESTS_PER_PAYLOAD
func ApplyParentExecutionPayload(
	ctx context.Context,
	st state.BeaconState,
	reqs *enginev1.ExecutionRequestsGloas,
) error {
	parentBid, err := st.LatestExecutionPayloadBid()
	if err != nil {
		return errors.Wrap(err, "could not get latest execution payload bid")
	}
	parentSlot := parentBid.Slot()

	if err := validateExecutionRequestLengths(reqs); err != nil {
		return err
	}

	if err := processExecutionRequests(ctx, st, reqs); err != nil {
		return errors.Wrap(err, "could not process parent execution requests")
	}

	if err := st.QueueBuilderPaymentForSlot(parentSlot); err != nil {
		return errors.Wrap(err, "could not queue builder payment")
	}

	if err := st.SetExecutionPayloadAvailability(parentSlot, true); err != nil {
		return errors.Wrap(err, "could not set parent execution payload availability")
	}

	blockHash := parentBid.BlockHash()
	if err := st.SetLatestBlockHash(blockHash); err != nil {
		return errors.Wrap(err, "could not set latest block hash")
	}

	return nil
}

// validateExecutionRequestLengths enforces the per-payload maxima that remain
// consensus-layer checks now that EIP-7688 uses ProgressiveList SSZ types.
func validateExecutionRequestLengths(reqs *enginev1.ExecutionRequestsGloas) error {
	cfg := params.BeaconConfig()
	if uint64(len(reqs.GetWithdrawals())) > cfg.MaxWithdrawalRequestsPerPayload {
		return errors.Errorf("too many withdrawal requests: %d > %d", len(reqs.GetWithdrawals()), cfg.MaxWithdrawalRequestsPerPayload)
	}
	if uint64(len(reqs.GetConsolidations())) > cfg.MaxConsolidationsRequestsPerPayload {
		return errors.Errorf("too many consolidation requests: %d > %d", len(reqs.GetConsolidations()), cfg.MaxConsolidationsRequestsPerPayload)
	}
	if uint64(len(reqs.GetBuilderDeposits())) > cfg.MaxBuilderDepositRequestsPerPayload {
		return errors.Errorf("too many builder deposit requests: %d > %d", len(reqs.GetBuilderDeposits()), cfg.MaxBuilderDepositRequestsPerPayload)
	}
	if uint64(len(reqs.GetBuilderExits())) > cfg.MaxBuilderExitRequestsPerPayload {
		return errors.Errorf("too many builder exit requests: %d > %d", len(reqs.GetBuilderExits()), cfg.MaxBuilderExitRequestsPerPayload)
	}
	return nil
}

func processExecutionRequests(ctx context.Context, st state.BeaconState, rqs *enginev1.ExecutionRequestsGloas) error {
	if err := ProcessDepositRequests(ctx, st, rqs.Deposits); err != nil {
		return errors.Wrap(err, "could not process deposit requests")
	}
	var err error
	st, err = requests.ProcessWithdrawalRequests(ctx, st, rqs.Withdrawals)
	if err != nil {
		return errors.Wrap(err, "could not process withdrawal requests")
	}
	if err := requests.ProcessConsolidationRequests(ctx, st, rqs.Consolidations); err != nil {
		return errors.Wrap(err, "could not process consolidation requests")
	}
	if err := ProcessBuilderDepositRequests(ctx, st, rqs.BuilderDeposits); err != nil {
		return errors.Wrap(err, "could not process builder deposit requests")
	}
	return ProcessBuilderExitRequests(ctx, st, rqs.BuilderExits)
}

// IsEmptyExecutionRequests returns true if the execution requests contain no entries.
func IsEmptyExecutionRequests(r *enginev1.ExecutionRequestsGloas) bool {
	if r == nil {
		return true
	}
	return len(r.Deposits) == 0 && len(r.Withdrawals) == 0 && len(r.Consolidations) == 0 &&
		len(r.BuilderDeposits) == 0 && len(r.BuilderExits) == 0
}
