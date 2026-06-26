package validator

import (
	"context"
	"sync"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// buildBlockGloas builds a Gloas (ePBS) block, whose body carries an execution payload bid
// rather than the payload itself. The payload is revealed separately via the envelope.
func (vs *Server) buildBlockGloas(ctx context.Context, sBlk interfaces.SignedBeaconBlock, head state.BeaconState, skipBuilder, parentFull, eagerPayloadStateRoot bool) (*ethpb.GenericBeaconBlock, error) {
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
	local, err := vs.getLocalPayload(ctx, sBlk.Block(), head, parentFull)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get local payload: %v", err)
	}
	selfBuilt, err := vs.setExecutionPayloadBid(ctx, sBlk, local, local.OverrideBuilder || skipBuilder)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not set execution payload bid: %v", err)
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

	blk, err := vs.constructGenericBeaconBlock(sBlk, nil, primitives.ZeroWei())
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
