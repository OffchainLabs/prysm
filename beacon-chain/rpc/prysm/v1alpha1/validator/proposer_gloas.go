package validator

import (
	"context"
	"math/big"
	"sync"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// buildBlockGloas builds a Gloas (ePBS) block, whose body carries an execution payload bid
// rather than the payload itself. The payload is revealed separately via the envelope.
func (vs *Server) buildBlockGloas(ctx context.Context, sBlk interfaces.SignedBeaconBlock, head state.BeaconState, skipBuilder, parentFull, eagerPayloadStateRoot bool, builderRequestAuths []*ethpb.SignedRequestAuthV1) (*ethpb.GenericBeaconBlock, error) {
	if parentFull {
		if err := vs.applyParentExecutionPayloadToHead(ctx, head, sBlk.Block().ParentRoot()); err != nil {
			return nil, status.Errorf(codes.Internal, "Could not apply parent execution payload: %v", err)
		}
	}

	var wg sync.WaitGroup
	wg.Go(func() {
		vs.setPreGloasConsensusFields(ctx, sBlk, head)
		if err := sBlk.SetPayloadAttestations(vs.getPayloadAttestations(ctx, head, sBlk.Block().ParentRoot())); err != nil {
			log.WithError(err).Error("Could not set payload attestations")
		}
		if err := vs.setParentExecutionRequests(ctx, sBlk, head, parentFull); err != nil {
			log.WithError(err).Error("Could not set parent execution requests")
		}
	})

	// local is our self-build candidate and the baseline for comparing incoming bids.
	var selfBuilt bool
	local, err := vs.getLocalPayload(ctx, sBlk.Block(), head, parentFull)
	if err != nil {
		log.WithError(err).Warn("Could not get local payload, falling back to remote bids")
		src, builderURL, fbErr := vs.setRemoteBidFallback(ctx, sBlk, head, parentFull, skipBuilder, builderRequestAuths)
		if fbErr != nil {
			return nil, status.Errorf(codes.Internal, "Could not get local payload and no remote bid fallback: %v", fbErr)
		}
		vs.recordBidSource(sBlk.Block().Slot(), src, builderURL)
	} else {
		selfBuildOnly := local.OverrideBuilder || skipBuilder
		var builderBid *ethpb.SignedExecutionPayloadBid
		var builderURL string
		var maxExecutionPayment uint64
		if !selfBuildOnly {
			builderBid, builderURL, maxExecutionPayment = vs.builderBidForProposal(ctx, sBlk, head, bytesutil.ToBytes32(local.ExecutionData.ParentHash()), builderRequestAuths)
		}
		src, bidErr := vs.setExecutionPayloadBid(ctx, sBlk, local, builderBid, maxExecutionPayment, selfBuildOnly)
		if bidErr != nil {
			return nil, status.Errorf(codes.Internal, "Could not set execution payload bid: %v", bidErr)
		}
		vs.recordBidSource(sBlk.Block().Slot(), src, builderURL)
		selfBuilt = src == bidSourceSelfBuild
	}

	wg.Wait()

	sr, _, err := vs.computePostBlockStateAndRoot(ctx, sBlk)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not compute state root: %v", err)
	}
	sBlk.SetStateRoot(sr)

	var envelope *ethpb.ExecutionPayloadEnvelope
	if selfBuilt { // self-build reveals its own payload later, so cache the envelope now
		envelope, err = vs.storeExecutionPayloadEnvelope(sBlk, local)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not build execution payload envelope: %v", err)
		}
	}

	blk, err := vs.constructGenericBeaconBlock(sBlk, nil, vs.gloasPayloadValue(sBlk, local, selfBuilt))
	if err != nil {
		return nil, err
	}

	// Eager (stateless) self-build: bundle envelope + blobs inline; stateful publishes from the cache.
	if eagerPayloadStateRoot && envelope != nil {
		var blobs, kzgProofs [][]byte
		if local.BlobsBundler != nil {
			blobs = local.BlobsBundler.GetBlobs()
			kzgProofs = local.BlobsBundler.GetProofs()
		}
		blk.Block = &ethpb.GenericBeaconBlock_GloasContents{GloasContents: &ethpb.BeaconBlockContentsGloas{
			Block:                    blk.GetGloas(),
			ExecutionPayloadEnvelope: envelope,
			KzgProofs:                kzgProofs,
			Blobs:                    blobs,
		}}
	}
	return blk, nil
}

// gloasPayloadValue is the local payload value when self-building, or the bid
// value when committing to an external bid.
func (vs *Server) gloasPayloadValue(sBlk interfaces.SignedBeaconBlock, local *consensusblocks.GetPayloadResponse, selfBuilt bool) primitives.Wei {
	if selfBuilt {
		if local == nil || local.Bid == nil {
			return primitives.ZeroWei()
		}
		return local.Bid
	}
	bid, err := sBlk.Block().Body().SignedExecutionPayloadBid()
	if err != nil || bid == nil || bid.Message == nil {
		return primitives.ZeroWei()
	}
	value := new(big.Int).SetUint64(uint64(bid.Message.Value))
	return value.Mul(value, big.NewInt(1e9))
}
