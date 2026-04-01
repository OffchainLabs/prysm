package validator

import (
	"context"
	"fmt"
	"sync"

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

// bidSource indicates where the winning execution payload bid came from.
type bidSource int

const (
	bidSourceSelfBuild  bidSource = iota // Local self-build; caller must cache envelope.
	bidSourceP2P                         // Received via P2P gossip; builder broadcasts envelope independently.
	bidSourceBuilderAPI                  // Fetched via Builder API; caller must submit signed block to builder.
)

// setExecutionPayloadBid selects the best execution payload bid for the block.
// It considers three sources: the builder API bid, a cached P2P bid, and the
// local self-build. The best bid that exceeds the local value wins.
func (vs *Server) setExecutionPayloadBid(
	ctx context.Context,
	sBlk interfaces.SignedBeaconBlock,
	local *consensusblocks.GetPayloadResponse,
	selfBuildOnly bool,
	signedRequestAuths []*ethpb.SignedRequestAuth,
) (bidSource, error) {
	_, span := trace.StartSpan(ctx, "ProposerServer.setExecutionPayloadBid")
	defer span.End()

	if local == nil || local.ExecutionData == nil {
		return bidSourceSelfBuild, errors.New("local execution payload is nil")
	}

	if !selfBuildOnly {
		if best, src := vs.bestRemoteBid(ctx, sBlk, local, signedRequestAuths); best != nil {
			if err := sBlk.SetSignedExecutionPayloadBid(best); err != nil {
				return bidSourceSelfBuild, errors.Wrap(err, "could not set remote execution payload bid")
			}
			return src, nil
		}
	}

	// Fall back to self-build bid.
	bid, err := vs.createSelfBuildExecutionPayloadBid(local, sBlk.Block())
	if err != nil {
		return bidSourceSelfBuild, errors.Wrap(err, "could not create execution payload bid")
	}

	// Per spec, self-build bids must use G2 point-at-infinity as the signature.
	signedBid := &ethpb.SignedExecutionPayloadBid{
		Message:   bid,
		Signature: common.InfiniteSignature[:],
	}
	if err := sBlk.SetSignedExecutionPayloadBid(signedBid); err != nil {
		return bidSourceSelfBuild, errors.Wrap(err, "could not set signed execution payload bid")
	}

	return bidSourceSelfBuild, nil
}

// bestRemoteBid returns the best remote bid (builder API or P2P cache) that
// exceeds the local EL value, along with its source. Bids whose
// execution_payment exceeds the proposer's max_trusted_bid for the builder
// are rejected.
func (vs *Server) bestRemoteBid(
	ctx context.Context,
	sBlk interfaces.SignedBeaconBlock,
	local *consensusblocks.GetPayloadResponse,
	signedRequestAuths []*ethpb.SignedRequestAuth,
) (*ethpb.SignedExecutionPayloadBid, bidSource) {
	localValueGwei := primitives.WeiToGwei(local.Bid)

	// Fetch from builder API if configured.
	apiBid := vs.getBuilderAPIBid(ctx, sBlk, local, signedRequestAuths)

	// Look up cached P2P bid.
	p2pBid := vs.winningP2PBid(sBlk, local)

	// Pick the best remote bid that exceeds local value.
	// TODO: consider using Value + ExecutionPayment as the total bid value
	// for comparison, not just Value alone.
	var best *ethpb.SignedExecutionPayloadBid
	var src bidSource
	switch {
	case apiBid != nil && p2pBid != nil:
		if apiBid.Message.Value >= p2pBid.Message.Value {
			best, src = apiBid, bidSourceBuilderAPI
		} else {
			best, src = p2pBid, bidSourceP2P
		}
	case apiBid != nil:
		best, src = apiBid, bidSourceBuilderAPI
	case p2pBid != nil:
		best, src = p2pBid, bidSourceP2P
	}
	if best == nil || best.Message.Value <= localValueGwei {
		return nil, bidSourceSelfBuild
	}
	return best, src
}

// TODO: Add filterByMaxTrustedBid once per-builder preferences are tracked.
// Per builder-specs PR 138, the proposer should reject bids where:
//   bid.execution_payment > max_trusted_bid

// getBuilderAPIBid queries all builders for which we have a SignedRequestAuth
// and returns the highest-value bid. Returns nil if no builder is configured,
// the circuit breaker is active, no auths are provided, or all requests fail.
func (vs *Server) getBuilderAPIBid(
	ctx context.Context,
	sBlk interfaces.SignedBeaconBlock,
	local *consensusblocks.GetPayloadResponse,
	signedRequestAuths []*ethpb.SignedRequestAuth,
) *ethpb.SignedExecutionPayloadBid {
	if vs.BlockBuilder == nil || !vs.BlockBuilder.Configured() || len(signedRequestAuths) == 0 {
		return nil
	}

	slot := sBlk.Block().Slot()
	proposerIndex := sBlk.Block().ProposerIndex()

	// Check circuit breaker.
	activated, err := vs.circuitBreakBuilder(slot)
	if err != nil || activated {
		return nil
	}

	ed := local.ExecutionData
	parentHash := [32]byte(ed.ParentHash())
	parentRoot := sBlk.Block().ParentRoot()

	return vs.highestBidFromBuilders(ctx, slot, parentHash, parentRoot, proposerIndex, signedRequestAuths)
}

// highestBidFromBuilders queries each builder (one per SignedRequestAuth)
// concurrently and returns the bid with the highest value. Failures for
// individual builders are logged and skipped.
func (vs *Server) highestBidFromBuilders(
	ctx context.Context,
	slot primitives.Slot,
	parentHash [32]byte,
	parentRoot [32]byte,
	proposerIndex primitives.ValidatorIndex,
	auths []*ethpb.SignedRequestAuth,
) *ethpb.SignedExecutionPayloadBid {
	type result struct {
		bid *ethpb.SignedExecutionPayloadBid
		err error
		key []byte // builder pubkey for logging
	}

	results := make([]result, len(auths))
	var wg sync.WaitGroup
	for i, auth := range auths {
		wg.Add(1)
		go func(idx int, a *ethpb.SignedRequestAuth) {
			defer wg.Done()
			bid, err := vs.BlockBuilder.GetExecutionPayloadBid(ctx, slot, parentHash, parentRoot, proposerIndex, a)
			results[idx] = result{bid: bid, err: err, key: a.Message.BuilderPubkey}
		}(i, auth)
	}
	wg.Wait()

	var best *ethpb.SignedExecutionPayloadBid
	for _, r := range results {
		if r.err != nil {
			log.WithError(r.err).WithField("builderPubkey", fmt.Sprintf("%#x", r.key)).
				Debug("Could not get execution payload bid from builder")
			continue
		}
		if r.bid == nil || r.bid.Message == nil {
			continue
		}
		if best == nil || r.bid.Message.Value > best.Message.Value {
			best = r.bid
		}
	}
	if best != nil {
		log.WithFields(logrus.Fields{
			"slot":            slot,
			"builderIndex":    best.Message.BuilderIndex,
			"builderValue":    best.Message.Value,
			"buildersQueried": len(auths),
		}).Info("Selected highest execution payload bid from builder API")
	}
	return best
}

// winningP2PBid returns a cached P2P bid if one exists and exceeds the local EL value.
func (vs *Server) winningP2PBid(
	sBlk interfaces.SignedBeaconBlock,
	local *consensusblocks.GetPayloadResponse,
) *ethpb.SignedExecutionPayloadBid {
	if vs.HighestBidCache == nil {
		return nil
	}

	ed := local.ExecutionData
	parentHash := [32]byte(ed.ParentHash())
	cached, ok := vs.HighestBidCache.Get(sBlk.Block().Slot(), parentHash, sBlk.Block().ParentRoot())
	if !ok {
		return nil
	}

	builderValueGwei := cached.Message.Value
	localValueGwei := primitives.WeiToGwei(local.Bid)
	if builderValueGwei <= localValueGwei {
		log.WithFields(logrus.Fields{
			"slot":             sBlk.Block().Slot(),
			"builderValueGwei": builderValueGwei,
			"localValueGwei":   localValueGwei,
		}).Info("Local EL value exceeds P2P bid, using self-build")
		return nil
	}

	log.WithFields(logrus.Fields{
		"slot":             sBlk.Block().Slot(),
		"builderIndex":     cached.Message.BuilderIndex,
		"builderValueGwei": builderValueGwei,
		"localValueGwei":   localValueGwei,
	}).Info("Using P2P execution payload bid over self-build")
	return cached
}

// submitBlockToBuilder sends the signed beacon block to the builder via the
// Builder API after proposal. This allows the builder to construct and broadcast
// the corresponding execution payload envelope. Only called for Gloas blocks
// when the bid was obtained via the Builder API. Best-effort — failures are
// logged but do not affect the proposal.
func (vs *Server) submitBlockToBuilder(ctx context.Context, block interfaces.SignedBeaconBlock) {
	if vs.BlockBuilder == nil || !vs.BlockBuilder.Configured() {
		return
	}

	if err := vs.BlockBuilder.SubmitSignedBeaconBlock(ctx, block); err != nil {
		log.WithError(err).WithField("slot", block.Block().Slot()).
			Warn("Failed to submit signed beacon block to builder (best-effort)")
	} else {
		log.WithField("slot", block.Block().Slot()).
			Info("Submitted signed beacon block to builder")
	}
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
		BlobKzgCommitments: local.BlobsBundler.GetKzgCommitments(),
	}, nil
}
