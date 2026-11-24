package validator

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/pq"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Self-build only, builder bids are not ported.
func (vs *Server) buildBlockPQ(ctx context.Context, sBlk interfaces.SignedBeaconBlock, head state.BeaconState, parentFull bool) (*ethpb.GenericBeaconBlock, error) {
	eth1Data, err := vs.eth1DataMajorityVote(ctx, head)
	if err != nil {
		eth1Data = &ethpb.Eth1Data{DepositRoot: params.BeaconConfig().ZeroHash[:], BlockHash: params.BeaconConfig().ZeroHash[:]}
		log.WithError(err).Error("Could not get eth1data")
	}
	sBlk.SetEth1Data(eth1Data)
	parentRequests, err := vs.parentExecutionRequestsPQ(ctx, sBlk.Block().ParentRoot(), parentFull)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get parent execution requests: %v", err)
	}
	sBlk.SetParentExecutionRequestsPQ(parentRequests)
	atts, proof, err := vs.packPQ(ctx, sBlk.Block().Slot(), head)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not pack attestations: %v", err)
	}
	if err := sBlk.SetAttestations(atts); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not set attestations: %v", err)
	}
	sBlk.SetAttestationProof(proof)

	local, err := vs.getLocalPayload(ctx, sBlk.Block(), head, parentFull)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get local payload: %v", err)
	}
	bid, err := vs.createSelfBuildExecutionPayloadBid(local, sBlk.Block())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not create execution payload bid: %v", err)
	}
	requestsRoot, err := executionRequestsPQ(local).HashTreeRoot()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not hash execution requests: %v", err)
	}
	bid.ExecutionRequestsRoot = requestsRoot[:]
	sBlk.SetSignedExecutionPayloadBidPQ(&ethpb.SignedExecutionPayloadBidPQ{
		Message:   bid,
		Signature: make([]byte, pq.XMSSSignatureLength),
	})

	sr, _, err := vs.computePostBlockStateAndRoot(ctx, sBlk)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not compute state root: %v", err)
	}
	sBlk.SetStateRoot(sr)
	if err := vs.storeExecutionPayloadEnvelopePQ(sBlk, local); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not build execution payload envelope: %v", err)
	}

	return vs.constructGenericBeaconBlock(sBlk, nil, primitives.ZeroWei())
}
