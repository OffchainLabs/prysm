package validator

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	opfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Deprecated: The gRPC API will remain the default and fully supported through v8 (expected in 2026) but will be eventually removed in favor of REST API.
//
// ProposeExit proposes an exit for a validator.
func (vs *Server) ProposeExit(ctx context.Context, req *ethpb.SignedVoluntaryExit) (*ethpb.ProposeExitResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "nil request")
	}
	s, err := vs.HeadFetcher.HeadStateReadOnly(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get head state: %v", err)
	}
	if req.Exit == nil {
		return nil, status.Error(codes.InvalidArgument, "voluntary exit does not exist")
	}
	if req.Signature == nil || len(req.Signature) != fieldparams.BLSSignatureLength {
		return nil, status.Error(codes.InvalidArgument, "invalid signature provided")
	}

	// Builder exits are only valid from Gloas onwards.
	if req.Exit.ValidatorIndex.IsBuilderIndex() {
		if s.Version() < version.Gloas {
			return nil, status.Error(codes.InvalidArgument, "builder exits not supported before Gloas")
		}
	}
	// Confirm the validator/builder is eligible to exit with the parameters provided.
	var val stateutil.CompactValidator
	if !req.Exit.ValidatorIndex.IsBuilderIndex() {
		val, err = s.ValidatorAtIndexReadOnly(req.Exit.ValidatorIndex)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "validator index exceeds validator set length")
		}
	}

	if err := blocks.VerifyExitAndSignature(&val, s, req); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	vs.OperationNotifier.OperationFeed().Send(&feed.Event{
		Type: opfeed.ExitReceived,
		Data: &opfeed.ExitReceivedData{
			Exit: req,
		},
	})

	vs.ExitPool.InsertVoluntaryExit(req)

	r, err := req.Exit.HashTreeRoot()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get tree hash of exit: %v", err)
	}

	return &ethpb.ProposeExitResponse{
		ExitRoot: r[:],
	}, vs.P2P.Broadcast(ctx, req)
}
