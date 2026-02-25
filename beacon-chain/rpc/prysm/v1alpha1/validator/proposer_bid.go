package validator

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

// setSelfBuildExecutionPayloadBid creates an execution payload bid from the local payload
// and sets it on the block body. The envelope is created and cached later by
// storeExecutionPayloadEnvelope once the block is fully built.
func (vs *Server) setSelfBuildExecutionPayloadBid(
	ctx context.Context,
	sBlk interfaces.SignedBeaconBlock,
	local *consensusblocks.GetPayloadResponse,
) error {
	_, span := trace.StartSpan(ctx, "ProposerServer.setSelfBuildExecutionPayloadBid")
	defer span.End()

	if local == nil || local.ExecutionData == nil {
		return errors.New("local execution payload is nil")
	}

	// Create execution payload bid from the local payload.
	bid, err := vs.createSelfBuildExecutionPayloadBid(local, sBlk.Block())
	if err != nil {
		return errors.Wrap(err, "could not create execution payload bid")
	}

	// Per spec, self-build bids must use G2 point-at-infinity as the signature.
	// Only the execution payload envelope requires a real signature from the proposer.
	signedBid := &ethpb.SignedExecutionPayloadBid{
		Message:   bid,
		Signature: common.InfiniteSignature[:],
	}
	if err := sBlk.SetSignedExecutionPayloadBid(signedBid); err != nil {
		return errors.Wrap(err, "could not set signed execution payload bid")
	}

	return nil
}

// createSelfBuildExecutionPayloadBid creates an ExecutionPayloadBid for self-building,
// where the proposer acts as its own builder. Per spec, the bid value must be zero
// and the builder index must be BUILDER_INDEX_SELF_BUILD.
func (vs *Server) createSelfBuildExecutionPayloadBid(
	local *consensusblocks.GetPayloadResponse,
	block interfaces.ReadOnlyBeaconBlock,
) (*ethpb.ExecutionPayloadBid, error) {
	ed := local.ExecutionData
	if ed == nil || ed.IsNil() {
		return nil, errors.New("execution data is nil")
	}

	parentBlockRoot := block.ParentRoot()
	return &ethpb.ExecutionPayloadBid{
		ParentBlockHash:    ed.ParentHash(),
		ParentBlockRoot:    bytesutil.SafeCopyBytes(parentBlockRoot[:]),
		BlockHash:          ed.BlockHash(),
		PrevRandao:         ed.PrevRandao(),
		FeeRecipient:       ed.FeeRecipient(),
		GasLimit:           ed.GasLimit(),
		BuilderIndex:       params.BeaconConfig().BuilderIndexSelfBuild,
		Slot:               block.Slot(),
		Value:              0,
		ExecutionPayment:   0,
		BlobKzgCommitments: [][]byte{}, // TODO: handle DA in gloas
	}, nil
}
