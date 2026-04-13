package gloas

import (
	"bytes"
	"context"

	requests "github.com/OffchainLabs/prysm/v7/beacon-chain/core/requests"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/pkg/errors"
)

// ProcessParentExecutionPayload processes deferred effects from the parent's
// execution payload. Must run first in process_block, before process_block_header,
// because it reads state.latest_block_header.slot and state.latest_execution_payload_bid
// which are overwritten by process_block_header and process_execution_payload_bid.
//
//	<spec fn="process_parent_execution_payload" fork="gloas" hash="defer_payload">
//	def process_parent_execution_payload(state: BeaconState, block: BeaconBlock) -> None:
//	    bid = block.body.signed_execution_payload_bid.message
//	    parent_bid = state.latest_execution_payload_bid
//	    is_parent_full = bid.parent_block_hash == parent_bid.block_hash
//	    if not is_parent_full:
//	        assert block.body.parent_execution_requests == ExecutionRequests()
//	        return
//	    parent_slot = state.latest_block_header.slot
//	    parent_epoch = compute_epoch_at_slot(parent_slot)
//	    state.execution_payload_availability[parent_slot % SLOTS_PER_HISTORICAL_ROOT] = 0b1
//	    requests = block.body.parent_execution_requests
//	    assert hash_tree_root(requests) == parent_bid.execution_requests_root
//	    for_ops(requests.deposits, process_deposit_request)
//	    for_ops(requests.withdrawals, process_withdrawal_request)
//	    for_ops(requests.consolidations, process_consolidation_request)
//	    if parent_epoch == get_current_epoch(state):
//	        payment_index = SLOTS_PER_EPOCH + parent_slot % SLOTS_PER_EPOCH
//	    elif parent_epoch == get_previous_epoch(state):
//	        payment_index = parent_slot % SLOTS_PER_EPOCH
//	    else:
//	        payment_index = None
//	    if payment_index is not None:
//	        payment = state.builder_pending_payments[payment_index]
//	        if payment.withdrawal.amount > 0:
//	            state.builder_pending_withdrawals.append(payment.withdrawal)
//	        state.builder_pending_payments[payment_index] = BuilderPendingPayment()
//	    state.latest_block_hash = bid.parent_block_hash
//	</spec>
func ProcessParentExecutionPayload(ctx context.Context, st state.BeaconState, blk interfaces.ReadOnlyBeaconBlock) error {
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

// ApplyParentExecutionPayload applies the parent's execution requests,
// queues the builder payment, updates payload availability and latest_block_hash.
// It reads the parent bid from state.latest_execution_payload_bid.
// Called by ProcessParentExecutionPayload during block processing and by the
// validator during block production before computing withdrawals.
//
//	<spec fn="apply_parent_execution_payload" fork="gloas" hash="defer_payload">
func ApplyParentExecutionPayload(
	ctx context.Context,
	st state.BeaconState,
	reqs *enginev1.ExecutionRequests,
) error {
	parentBid, err := st.LatestExecutionPayloadBid()
	if err != nil {
		return errors.Wrap(err, "could not get latest execution payload bid")
	}
	parentSlot := parentBid.Slot()

	if err := ProcessExecutionRequests(ctx, st, reqs); err != nil {
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

// ProcessExecutionRequests processes deposits, withdrawals, and consolidations from execution requests.
// Called by ApplyParentExecutionPayload during block processing and by the
// validator during block production before computing withdrawals.
func ProcessExecutionRequests(ctx context.Context, st state.BeaconState, rqs *enginev1.ExecutionRequests) error {
	if err := processDepositRequests(ctx, st, rqs.Deposits); err != nil {
		return errors.Wrap(err, "could not process deposit requests")
	}
	var err error
	st, err = requests.ProcessWithdrawalRequests(ctx, st, rqs.Withdrawals)
	if err != nil {
		return errors.Wrap(err, "could not process withdrawal requests")
	}
	return requests.ProcessConsolidationRequests(ctx, st, rqs.Consolidations)
}

// IsEmptyExecutionRequests returns true if the execution requests contain no entries.
func IsEmptyExecutionRequests(r *enginev1.ExecutionRequests) bool {
	if r == nil {
		return true
	}
	return len(r.Deposits) == 0 && len(r.Withdrawals) == 0 && len(r.Consolidations) == 0
}
