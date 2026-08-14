package validator

import (
	"context"
	"sync"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/sirupsen/logrus"
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
		log.WithError(err).Warn("Could not get local payload, falling back to P2P bid")
		if fbErr := vs.setP2PBidFallback(ctx, sBlk, head, parentFull); fbErr != nil {
			return nil, status.Errorf(codes.Internal, "Could not get local payload and no P2P bid fallback: %v", fbErr)
		}
	} else {
		// Must run before setExecutionPayloadBid, which commits to execution_requests_root.
		vs.injectMockSweepThresholdRequests(local, sBlk.Block().Slot())

		selfBuildOnly := local.OverrideBuilder || skipBuilder
		var builderBid *ethpb.SignedExecutionPayloadBid
		var builderURL string
		var maxExecutionPayment uint64
		if !selfBuildOnly && len(builderRequestAuths) > 0 {
			val, valErr := head.ValidatorAtIndexReadOnly(sBlk.Block().ProposerIndex())
			parentGasLimit, glErr := vs.ForkchoiceFetcher.GasLimit(sBlk.Block().ParentRoot())
			switch {
			case valErr != nil:
				log.WithError(valErr).Error("Could not get proposer for builder bid request")
			case glErr != nil:
				log.WithError(glErr).Error("Could not get parent gas limit for builder bid request")
			default:
				pubkey := val.PublicKey()
				if v, ok := vs.maxExecutionPayments.Load(pubkey); ok {
					maxExecutionPayment, _ = v.(uint64)
				}
				pref := vs.proposerPreferenceForProposal(ctx, head, sBlk.Block().Slot(), sBlk.Block().ProposerIndex())
				feeRecipient := pref.FeeRecipientOrDefault()
				builderBid, builderURL = vs.getBuilderExecutionPayloadBid(ctx, head, &builderBidQuery{
					slot:           sBlk.Block().Slot(),
					parentRoot:     sBlk.Block().ParentRoot(),
					parentHash:     bytesutil.ToBytes32(local.ExecutionData.ParentHash()),
					pubkey:         pubkey,
					maxPayment:     maxExecutionPayment,
					feeRecipient:   feeRecipient[:],
					parentGasLimit: parentGasLimit,
					targetGasLimit: pref.GasLimitOr(parentGasLimit),
					auths:          builderRequestAuths,
				})
			}
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

// injectMockSweepThresholdRequests appends devnet-only EIP-8148 set sweep threshold requests
// to the payload the execution client just returned, so the request type can be exercised
// before any execution client implements EIP-7685 request type 0x05. Requests are staged
// via POST /prysm/v1/debug/beacon/sweep_threshold_requests and drained here exactly once.
func (vs *Server) injectMockSweepThresholdRequests(local *consensusblocks.GetPayloadResponse, slot primitives.Slot) {
	if local == nil || local.ExecutionRequestsGloas == nil {
		return
	}

	mocked := vs.MockSweepThresholdPool.Drain()
	if len(mocked) == 0 {
		return
	}

	requests := local.ExecutionRequestsGloas
	room := int(params.BeaconConfig().MaxSetSweepThresholdRequestsPerPayload) - len(requests.SweepThresholds)
	if room <= 0 {
		log.WithField("slot", slot).Warning("Dropping mocked set sweep threshold requests, payload is already at the per-payload limit")
		return
	}

	if len(mocked) > room {
		log.WithFields(logrus.Fields{
			"slot":    slot,
			"dropped": len(mocked) - room,
		}).Warning("Dropping mocked set sweep threshold requests over the per-payload limit")
		mocked = mocked[:room]
	}

	requests.SweepThresholds = append(requests.SweepThresholds, mocked...)
	log.WithFields(logrus.Fields{
		"slot":  slot,
		"count": len(mocked),
	}).Warning("DEVNET ONLY: injected mocked EIP-8148 set sweep threshold requests into the local payload")
}
