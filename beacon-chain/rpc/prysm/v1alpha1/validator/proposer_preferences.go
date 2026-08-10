package validator

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// SubmitSignedProposerPreferences broadcasts signed proposer preferences and
// caches them locally for subsequent bid validation.
// Local submissions intentionally bypass full gossip verification (proposer
// lookahead, signature) because the validator client is trusted.
func (vs *Server) SubmitSignedProposerPreferences(
	ctx context.Context,
	req *ethpb.SubmitSignedProposerPreferencesRequest,
) (*emptypb.Empty, error) {
	if rpcErr := vs.CoreService.SubmitSignedProposerPreferences(ctx, req); rpcErr != nil {
		return nil, status.Error(core.ErrorReasonToGRPC(rpcErr.Reason), rpcErr.Err.Error())
	}
	return &emptypb.Empty{}, nil
}
