package gloas

import (
	"bytes"
	"context"
	gotime "time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	requests "github.com/OffchainLabs/prysm/v7/beacon-chain/core/requests"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/pkg/errors"
)

// ProcessParentExecutionPayload must run before process_block_header and
// process_execution_payload_bid, which overwrite the state fields it reads.
//
//	<spec fn="process_parent_execution_payload" fork="gloas" hash="defer_payload">
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

// ApplyParentExecutionPayload reads parent_bid from state.latest_execution_payload_bid
// and mutates st. Called by ProcessParentExecutionPayload and by the validator during
// block production before computing withdrawals.
//
//	<spec fn="apply_parent_execution_payload" fork="gloas" hash="defer_payload">
func ApplyParentExecutionPayload(
	ctx context.Context,
	st state.BeaconState,
	reqs *enginev1.ExecutionRequests,
) error {
	tBid := gotime.Now()
	parentBid, err := st.LatestExecutionPayloadBid()
	if err != nil {
		return errors.Wrap(err, "could not get latest execution payload bid")
	}
	parentSlot := parentBid.Slot()
	dBid := gotime.Since(tBid)

	tReqs := gotime.Now()
	if err := processExecutionRequests(ctx, st, reqs); err != nil {
		return errors.Wrap(err, "could not process parent execution requests")
	}
	dReqs := gotime.Since(tReqs)

	tQueue := gotime.Now()
	if err := st.QueueBuilderPaymentForSlot(parentSlot); err != nil {
		return errors.Wrap(err, "could not queue builder payment")
	}
	dQueue := gotime.Since(tQueue)

	tAvail := gotime.Now()
	if err := st.SetExecutionPayloadAvailability(parentSlot, true); err != nil {
		return errors.Wrap(err, "could not set parent execution payload availability")
	}
	dAvail := gotime.Since(tAvail)

	tHash := gotime.Now()
	blockHash := parentBid.BlockHash()
	if err := st.SetLatestBlockHash(blockHash); err != nil {
		return errors.Wrap(err, "could not set latest block hash")
	}
	dHash := gotime.Since(tHash)

	log.WithFields(map[string]any{
		"latestBid":       dBid,
		"processReqs":     dReqs,
		"queuePayment":    dQueue,
		"setAvailability": dAvail,
		"setLatestHash":   dHash,
	}).Info("ApplyParentExecutionPayload timings")

	return nil
}

func processExecutionRequests(ctx context.Context, st state.BeaconState, rqs *enginev1.ExecutionRequests) error {
	tHTR := gotime.Now()
	var prefetched []bool
	if rqs != nil && len(rqs.Deposits) > 0 {
		if root, err := rqs.HashTreeRoot(); err == nil {
			if v, ok := cache.DepositSig.Get(root); ok && len(v) == len(rqs.Deposits) {
				prefetched = v
			}
		}
	}
	dHTR := gotime.Since(tHTR)

	tDeps := gotime.Now()
	if err := processDepositRequests(ctx, st, rqs.Deposits, prefetched); err != nil {
		return errors.Wrap(err, "could not process deposit requests")
	}
	dDeps := gotime.Since(tDeps)

	tWds := gotime.Now()
	var err error
	st, err = requests.ProcessWithdrawalRequests(ctx, st, rqs.Withdrawals)
	if err != nil {
		return errors.Wrap(err, "could not process withdrawal requests")
	}
	dWds := gotime.Since(tWds)

	tCons := gotime.Now()
	err = requests.ProcessConsolidationRequests(ctx, st, rqs.Consolidations)
	dCons := gotime.Since(tCons)

	log.WithFields(map[string]any{
		"requestsHTR":      dHTR,
		"depositRequests":  dDeps,
		"withdrawalReqs":   dWds,
		"consolidationReq": dCons,
		"numDeposits":      len(rqs.Deposits),
		"numWithdrawals":   len(rqs.Withdrawals),
		"numConsolidation": len(rqs.Consolidations),
		"prefetched":       prefetched != nil,
	}).Info("processExecutionRequests timings")

	return err
}

// IsEmptyExecutionRequests returns true if the execution requests contain no entries.
func IsEmptyExecutionRequests(r *enginev1.ExecutionRequests) bool {
	if r == nil {
		return true
	}
	return len(r.Deposits) == 0 && len(r.Withdrawals) == 0 && len(r.Consolidations) == 0
}
