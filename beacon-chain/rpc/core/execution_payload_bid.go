package core

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	opfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// SubmitSignedExecutionPayloadBid verifies, records, and broadcasts a signed execution payload bid.
func (s *Service) SubmitSignedExecutionPayloadBid(ctx context.Context, req *ethpb.SignedExecutionPayloadBid) *RpcError {
	ctx, span := trace.StartSpan(ctx, "ValidatorServer.SubmitSignedExecutionPayloadBid")
	defer span.End()

	if req == nil || req.Message == nil {
		return &RpcError{Reason: BadRequest, Err: errors.New("signed execution payload bid is nil")}
	}
	if s.SyncChecker.Syncing() {
		return &RpcError{Reason: Unavailable, Err: errors.New("Syncing to latest head, not ready to respond")}
	}
	if slots.ToEpoch(req.Message.Slot) < params.BeaconConfig().GloasForkEpoch {
		return &RpcError{Reason: BadRequest, Err: errors.Errorf("execution payload bids are not supported before Gloas fork (slot %d)", req.Message.Slot)}
	}
	if rpcErr := s.validateSubmittedBid(ctx, req); rpcErr != nil {
		return rpcErr
	}

	// Self-published gossip is not looped back, so record the bid here for this node's own proposer.
	s.HighestBidCache.SetIfHigher(req)
	if err := s.P2P.Broadcast(ctx, req); err != nil {
		return &RpcError{Reason: Internal, Err: errors.Wrap(err, "could not broadcast signed execution payload bid")}
	}
	s.OperationNotifier.OperationFeed().Send(&feed.Event{
		Type: opfeed.ExecutionPayloadBidReceived,
		Data: &opfeed.ExecutionPayloadBidReceivedData{Bid: req},
	})
	return nil
}

func (s *Service) validateSubmittedBid(ctx context.Context, signed *ethpb.SignedExecutionPayloadBid) *RpcError {
	if s.NewExecutionPayloadBidVerifier == nil {
		return &RpcError{Reason: Internal, Err: errors.New("bid verifier not ready")}
	}
	ro, err := consensusblocks.WrappedROSignedExecutionPayloadBid(signed)
	if err != nil {
		return &RpcError{Reason: BadRequest, Err: errors.Wrap(err, "could not wrap signed bid")}
	}
	v := s.NewExecutionPayloadBidVerifier(ro, verification.GossipExecutionPayloadBidRequirements)
	bid, err := ro.Bid()
	if err != nil {
		return &RpcError{Reason: BadRequest, Err: errors.Wrap(err, "could not get bid")}
	}

	if err := v.VerifyCurrentOrNextSlot(); err != nil {
		return &RpcError{Reason: BadRequest, Err: err}
	}
	parentRoot := bid.ParentBlockRoot()
	st := transition.NextSlotStateReadOnly(parentRoot[:], bid.Slot())
	if st == nil || st.Slot() != bid.Slot() {
		return &RpcError{Reason: FailedPrecondition, Err: errors.New("state for bid parent block root is unavailable")}
	}
	dependentRoot, err := helpers.ProposerDependentRootOrGenesis(ctx, s.BeaconDB, st, bid.Slot())
	if err != nil {
		return &RpcError{Reason: Internal, Err: errors.Wrap(err, "could not get proposer dependent root")}
	}
	pref, ok := s.ProposerPreferencesCache.Get(dependentRoot, bid.Slot())
	if !ok {
		return &RpcError{Reason: FailedPrecondition, Err: errors.New("no proposer preferences seen for bid slot")}
	}

	if err := v.VerifyBuilderActive(st); err != nil {
		return &RpcError{Reason: BadRequest, Err: err}
	}
	if err := v.VerifyBuilderVersion(st); err != nil {
		return &RpcError{Reason: BadRequest, Err: err}
	}
	if err := v.VerifyExecutionPaymentZero(); err != nil {
		return &RpcError{Reason: BadRequest, Err: err}
	}
	if err := v.VerifyFeeRecipientMatches(pref.FeeRecipient[:]); err != nil {
		return &RpcError{Reason: BadRequest, Err: err}
	}
	if err := v.VerifyBlobKzgCommitmentsLimit(); err != nil {
		return &RpcError{Reason: BadRequest, Err: err}
	}
	if err := v.VerifyPrevRandao(st); err != nil {
		return &RpcError{Reason: BadRequest, Err: err}
	}
	if err := v.VerifyBuilderCanCoverBid(st); err != nil {
		return &RpcError{Reason: BadRequest, Err: err}
	}
	if err := v.VerifyParentBlockHash(s.ForkchoiceFetcher.HasPayloadBlockHash); err != nil {
		return &RpcError{Reason: BadRequest, Err: err}
	}
	parentGasLimit, err := s.ForkchoiceFetcher.GasLimit(parentRoot)
	if err != nil {
		return &RpcError{Reason: FailedPrecondition, Err: errors.Wrap(err, "could not get parent gas limit")}
	}
	if err := v.VerifyGasLimitTargetCompatible(parentGasLimit, pref.TargetGasLimit); err != nil {
		return &RpcError{Reason: BadRequest, Err: err}
	}
	parentSlot, err := s.ForkchoiceFetcher.RecentBlockSlot(parentRoot)
	if err != nil {
		return &RpcError{Reason: FailedPrecondition, Err: errors.Wrap(err, "could not get parent block slot")}
	}
	if err := v.VerifyBidSlotHigherThanParent(parentSlot); err != nil {
		return &RpcError{Reason: BadRequest, Err: err}
	}
	if err := v.VerifySignature(st); err != nil {
		return &RpcError{Reason: BadRequest, Err: err}
	}
	return nil
}
