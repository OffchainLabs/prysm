package validator

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// SubmitSignedExecutionPayloadBid verifies a signed execution payload bid against the gossip
// rules, records it in the local highest-bid cache, and broadcasts it to the P2P network.
func (vs *Server) SubmitSignedExecutionPayloadBid(
	ctx context.Context,
	req *ethpb.SignedExecutionPayloadBid,
) (*emptypb.Empty, error) {
	if rpcErr := vs.CoreService.SubmitSignedExecutionPayloadBid(ctx, req); rpcErr != nil {
		return nil, status.Error(core.ErrorReasonToGRPC(rpcErr.Reason), rpcErr.Err.Error())
	}
	return &emptypb.Empty{}, nil
}
