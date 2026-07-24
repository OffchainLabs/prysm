package validator

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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// SubmitSignedExecutionPayloadBid verifies a signed execution payload bid against the gossip
// rules, records it in the local highest-bid cache, and broadcasts it to the P2P network.
func (vs *Server) SubmitSignedExecutionPayloadBid(
	ctx context.Context,
	req *ethpb.SignedExecutionPayloadBid,
) (*emptypb.Empty, error) {
	ctx, span := trace.StartSpan(ctx, "ValidatorServer.SubmitSignedExecutionPayloadBid")
	defer span.End()

	if req == nil || req.Message == nil {
		return nil, status.Errorf(codes.InvalidArgument, "signed execution payload bid is nil")
	}

	if vs.SyncChecker.Syncing() {
		return nil, status.Errorf(codes.Unavailable, "Syncing to latest head, not ready to respond")
	}

	if slots.ToEpoch(req.Message.Slot) < params.BeaconConfig().GloasForkEpoch {
		return nil, status.Errorf(codes.InvalidArgument,
			"execution payload bids are not supported before Gloas fork (slot %d)", req.Message.Slot)
	}

	if err := vs.validateSubmittedBid(ctx, req); err != nil {
		return nil, err
	}

	// Self-published gossip is not looped back, so record the bid here for this node's own proposer.
	vs.HighestBidCache.SetIfHigher(req)

	if err := vs.P2P.Broadcast(ctx, req); err != nil {
		return nil, status.Errorf(codes.Internal, "could not broadcast signed execution payload bid: %v", err)
	}

	vs.OperationNotifier.OperationFeed().Send(&feed.Event{
		Type: opfeed.ExecutionPayloadBidReceived,
		Data: &opfeed.ExecutionPayloadBidReceivedData{Bid: req},
	})

	return &emptypb.Empty{}, nil
}

// validateSubmittedBid mirrors the gossip bid checks so this node never relays a bid its peers
// would reject, and never caches a bid the proposer could not include.
func (vs *Server) validateSubmittedBid(ctx context.Context, signed *ethpb.SignedExecutionPayloadBid) error {
	if vs.NewExecutionPayloadBidVerifier == nil {
		return status.Error(codes.Internal, "bid verifier not ready")
	}
	ro, err := consensusblocks.WrappedROSignedExecutionPayloadBid(signed)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "could not wrap signed bid: %v", err)
	}
	v := vs.NewExecutionPayloadBidVerifier(ro, verification.GossipExecutionPayloadBidRequirements)
	bid, err := ro.Bid()
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "could not get bid: %v", err)
	}

	if err := v.VerifyCurrentOrNextSlot(); err != nil {
		return status.Errorf(codes.InvalidArgument, "%v", err)
	}
	parentRoot := bid.ParentBlockRoot()
	st := transition.NextSlotStateReadOnly(parentRoot[:], bid.Slot())
	if st == nil || st.Slot() != bid.Slot() {
		return status.Error(codes.FailedPrecondition, "state for bid parent block root is unavailable")
	}
	dependentRoot, err := helpers.ProposerDependentRootOrGenesis(ctx, vs.BeaconDB, st, bid.Slot())
	if err != nil {
		return status.Errorf(codes.Internal, "could not get proposer dependent root: %v", err)
	}
	pref, ok := vs.ProposerPreferencesCache.Get(dependentRoot, bid.Slot())
	if !ok {
		return status.Error(codes.FailedPrecondition, "no proposer preferences seen for bid slot")
	}

	if err := v.VerifyBuilderActive(st); err != nil {
		return status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if err := v.VerifyBuilderVersion(st); err != nil {
		return status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if err := v.VerifyExecutionPaymentZero(); err != nil {
		return status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if err := v.VerifyFeeRecipientMatches(pref.FeeRecipient[:]); err != nil {
		return status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if err := v.VerifyBlobKzgCommitmentsLimit(); err != nil {
		return status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if err := v.VerifyPrevRandao(st); err != nil {
		return status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if err := v.VerifyBuilderCanCoverBid(st); err != nil {
		return status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if err := v.VerifyParentBlockHash(vs.ForkchoiceFetcher.HasPayloadBlockHash); err != nil {
		return status.Errorf(codes.InvalidArgument, "%v", err)
	}
	parentGasLimit, err := vs.ForkchoiceFetcher.GasLimit(parentRoot)
	if err != nil {
		return status.Errorf(codes.FailedPrecondition, "could not get parent gas limit: %v", err)
	}
	if err := v.VerifyGasLimitTargetCompatible(parentGasLimit, pref.TargetGasLimit); err != nil {
		return status.Errorf(codes.InvalidArgument, "%v", err)
	}
	parentSlot, err := vs.ForkchoiceFetcher.RecentBlockSlot(parentRoot)
	if err != nil {
		return status.Errorf(codes.FailedPrecondition, "could not get parent block slot: %v", err)
	}
	if err := v.VerifyBidSlotHigherThanParent(parentSlot); err != nil {
		return status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if err := v.VerifySignature(st); err != nil {
		return status.Errorf(codes.InvalidArgument, "%v", err)
	}
	return nil
}
