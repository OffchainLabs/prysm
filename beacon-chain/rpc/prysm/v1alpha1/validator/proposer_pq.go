package validator

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/pq"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations/pqpool"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Self-build only, without a payload envelope until that is ported.
func (vs *Server) buildBlockPQ(ctx context.Context, sBlk interfaces.SignedBeaconBlock, head state.BeaconState, parentFull bool) (*ethpb.GenericBeaconBlock, error) {
	if parentFull {
		return nil, status.Error(codes.Internal, "Payload envelopes are not supported on PQ yet")
	}

	eth1Data, err := vs.eth1DataMajorityVote(ctx, head)
	if err != nil {
		eth1Data = &ethpb.Eth1Data{DepositRoot: params.BeaconConfig().ZeroHash[:], BlockHash: params.BeaconConfig().ZeroHash[:]}
		log.WithError(err).Error("Could not get eth1data")
	}
	sBlk.SetEth1Data(eth1Data)
	sBlk.SetParentExecutionRequestsPQ(&enginev1.ExecutionRequestsPQ{})
	atts, proof, err := pqpool.Pack(ctx, head, vs.PQAttPool.All(), vs.PQAttPool.Aggregates())
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
	requestsRoot, err := (&enginev1.ExecutionRequestsPQ{}).HashTreeRoot()
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

	return vs.constructGenericBeaconBlock(sBlk, nil, primitives.ZeroWei())
}
