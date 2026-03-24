package validator

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// setExecutionPayloadBid selects the best execution payload bid for the block.
// When selfBuildOnly is false it compares the highest P2P bid from the cache
// against the local EL block value and uses whichever is greater.
// Returns true when a self-build bid was selected (the caller must cache the
// execution payload envelope); false when a remote P2P bid was used.
func (vs *Server) setExecutionPayloadBid(
	ctx context.Context,
	sBlk interfaces.SignedBeaconBlock,
	local *consensusblocks.GetPayloadResponse,
	selfBuildOnly bool,
) (bool, error) {
	_, span := trace.StartSpan(ctx, "ProposerServer.setExecutionPayloadBid")
	defer span.End()

	if local == nil || local.ExecutionData == nil {
		return false, errors.New("local execution payload is nil")
	}

	// Check the highest-bid cache for a P2P bid that beats the local EL value.
	if !selfBuildOnly && vs.HighestBidCache != nil {
		ed := local.ExecutionData
		var parentHash [32]byte
		copy(parentHash[:], ed.ParentHash())
		parentRoot := sBlk.Block().ParentRoot()
		if cached, ok := vs.HighestBidCache.Get(sBlk.Block().Slot(), parentHash, parentRoot); ok {
			builderValueGwei := cached.Message.Value
			localValueGwei := primitives.WeiToGwei(local.Bid)
			if builderValueGwei > localValueGwei {
				log.WithFields(logrus.Fields{
					"slot":             sBlk.Block().Slot(),
					"builderIndex":     cached.Message.BuilderIndex,
					"builderValueGwei": builderValueGwei,
					"localValueGwei":   localValueGwei,
				}).Info("Using P2P execution payload bid over self-build")
				if err := sBlk.SetSignedExecutionPayloadBid(cached); err != nil {
					return false, errors.Wrap(err, "could not set cached P2P execution payload bid")
				}
				return false, nil
			}
			log.WithFields(logrus.Fields{
				"slot":             sBlk.Block().Slot(),
				"builderValueGwei": builderValueGwei,
				"localValueGwei":   localValueGwei,
			}).Info("Local EL value exceeds P2P bid, using self-build")
		}
	}

	// Fall back to self-build bid.
	bid, err := vs.createSelfBuildExecutionPayloadBid(local, sBlk.Block())
	if err != nil {
		return false, errors.Wrap(err, "could not create execution payload bid")
	}

	// Per spec, self-build bids must use G2 point-at-infinity as the signature.
	signedBid := &ethpb.SignedExecutionPayloadBid{
		Message:   bid,
		Signature: common.InfiniteSignature[:],
	}
	if err := sBlk.SetSignedExecutionPayloadBid(signedBid); err != nil {
		return false, errors.Wrap(err, "could not set signed execution payload bid")
	}

	return true, nil
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
