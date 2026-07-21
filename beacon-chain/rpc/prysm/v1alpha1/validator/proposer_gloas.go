package validator

import (
	"context"
	"sync"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// buildBlockGloas builds a Gloas (ePBS) block, whose body carries an execution payload bid
// rather than the payload itself. The payload is revealed separately via the envelope.
func (vs *Server) buildBlockGloas(ctx context.Context, sBlk interfaces.SignedBeaconBlock, head state.BeaconState, skipBuilder, parentFull, eagerPayloadStateRoot bool, builderPreferences []*ethpb.BuilderPreferenceV1) (*ethpb.GenericBeaconBlock, error) {
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
	var winningBuilderURL string
	local, err := vs.getLocalPayload(ctx, sBlk.Block(), head, parentFull)
	if err != nil {
		log.WithError(err).Warn("Could not get local payload, falling back to P2P bid")
		if fbErr := vs.setP2PBidFallback(ctx, sBlk, head, parentFull); fbErr != nil {
			return nil, status.Errorf(codes.Internal, "Could not get local payload and no P2P bid fallback: %v", fbErr)
		}
	} else {
		selfBuildOnly := local.OverrideBuilder || skipBuilder
		var builderBid *ethpb.SignedExecutionPayloadBid
		var builderURL string
		prefs := bidPreferences{boostFactor: neutralBuilderBoostFactor}
		if !selfBuildOnly && len(builderPreferences) > 0 {
			val, valErr := head.ValidatorAtIndexReadOnly(sBlk.Block().ProposerIndex())
			parentGasLimit, glErr := vs.ForkchoiceFetcher.GasLimit(sBlk.Block().ParentRoot())
			switch {
			case valErr != nil:
				log.WithError(valErr).Error("Could not get proposer for builder bid request")
			case glErr != nil:
				log.WithError(glErr).Error("Could not get parent gas limit for builder bid request")
			default:
				pubkey := val.PublicKey()
				var authsByURL map[string]*ethpb.SignedRequestAuthV1
				authsByURL, prefs = builderAuthsAndPrefs(builderPreferences)
				pref := vs.proposerPreferenceForProposal(ctx, head, sBlk.Block().Slot(), sBlk.Block().ProposerIndex())
				feeRecipient := pref.FeeRecipientOrDefault()
				builderBid, builderURL = vs.getBuilderExecutionPayloadBid(ctx, head, &builderBidQuery{
					slot:           sBlk.Block().Slot(),
					parentRoot:     sBlk.Block().ParentRoot(),
					parentHash:     bytesutil.ToBytes32(local.ExecutionData.ParentHash()),
					pubkey:         pubkey,
					maxPayment:     prefs.maxPayment,
					feeRecipient:   feeRecipient[:],
					parentGasLimit: parentGasLimit,
					targetGasLimit: pref.GasLimitOr(parentGasLimit),
					authsByURL:     authsByURL,
				})
			}
		}
		src, bidErr := vs.setExecutionPayloadBid(ctx, sBlk, local, builderBid, prefs, selfBuildOnly)
		if bidErr != nil {
			return nil, status.Errorf(codes.Internal, "Could not set execution payload bid: %v", bidErr)
		}
		if src == bidSourceBuilderAPI {
			winningBuilderURL = builderURL
		}
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
	blk.BuilderUrl = winningBuilderURL

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

// builderAuthsAndPrefs extracts the signed request auths and the effective
// proposer-local bid preferences from the inline builder preferences carried on
// the block request. The effective preferences use the first valid entry as the
// proposal baseline. The beacon node enforces the caps locally during bid
// selection; communicating preferences to the builder is left to the bid request.
func builderAuthsAndPrefs(prefs []*ethpb.BuilderPreferenceV1) (map[string]*ethpb.SignedRequestAuthV1, bidPreferences) {
	authsByURL := make(map[string]*ethpb.SignedRequestAuthV1, len(prefs))
	bp := bidPreferences{boostFactor: neutralBuilderBoostFactor}
	baselineSet := false
	for _, p := range prefs {
		if p == nil || p.Request == nil || p.Request.Auth == nil || p.Url == "" {
			continue
		}
		if _, ok := authsByURL[p.Url]; !ok {
			authsByURL[p.Url] = p.Request.Auth
		}
		if !baselineSet {
			bp.maxPayment = uint64(p.Request.Preferences.GetMaxExecutionPayment())
			if p.MinBid != nil {
				bp.minBid = uint64(p.GetMinBid())
			}
			if p.BuilderBoostFactor != nil {
				bp.boostFactor = p.GetBuilderBoostFactor()
			}
			baselineSet = true
		}
	}
	return authsByURL, bp
}
