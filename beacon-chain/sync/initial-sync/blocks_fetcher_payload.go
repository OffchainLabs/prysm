package initialsync

import (
	"bytes"
	"context"
	"fmt"

	prysmsync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	p2ppb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// checkAllBlocksBuildOnEmpty verifies that all the passed blocks build on top of the empty block
// It ignores the first block in the slice
func checkAllBlocksBuildOnEmpty(blks []blocks.BlockWithROSidecars) error {
	b := blks[0].Block
	s, err := b.Block().Body().SignedExecutionPayloadBid()
	if err != nil {
		return errors.Wrap(err, "get execution payload bid from block")
	}
	firstBid := s.Message
	for i := 1; i < len(blks); i++ {
		next := blks[i].Block
		if next.ReadOnlySignedBeaconBlock == nil {
			return fmt.Errorf("nil block at index %d", i)
		}
		if next.Block().ParentRoot() != b.Root() {
			return fmt.Errorf("block with root %#x does not descend from %#x", next.Root(), b.Root())
		}
		nextSignedBid, err := next.Block().Body().SignedExecutionPayloadBid()
		if err != nil {
			return errors.Wrap(err, "get execution payload bid from block")
		}
		if !bytes.Equal(nextSignedBid.Message.ParentBlockHash, firstBid.ParentBlockHash) {
			return fmt.Errorf("block with root %#x does not build on top of the empty block", next.Root())
		}
		b = next
	}
	return nil
}

// validatePayloadBlockConsistency checks that the envelopes slice correponds to the blocks slice in
// the peers responses. If they were given by the same peer then we also penalize the peer if they are
// not consistent.
func (f *blocksFetcher) validatePayloadBlockConsistency(r *fetchRequestResponse) {
	if len(r.envelopes) == 0 {
		if err := checkAllBlocksBuildOnEmpty(r.bwb); err != nil {
			r.err = errors.Wrap(prysmsync.ErrInvalidFetchedData, err.Error())
			if r.blocksFrom == r.payloadsFrom {
				f.downscorePeer(r.blocksFrom, r.err)
			}
		}
		return
	}

	full, err := blocks.BlockBuiltOnEnvelope(r.envelopes[0], r.bwb[0].Block)
	if err != nil {
		r.err = errors.Wrap(prysmsync.ErrInvalidFetchedData, err.Error())
		return
	}
	pidx := 0
	if full {
		pidx = 1
	}
	log.WithFields(logrus.Fields{
		"blockSlot":      r.bwb[0].Block.Block().Slot(),
		"envelopeSlot":   envelopeSlotStr(r.envelopes[0]),
		"numBlocks":      len(r.bwb),
		"numEnvelopes":   len(r.envelopes),
		"firstBlockFull": full,
		"pidx":           pidx,
	}).Debug("Payload validation start")
	bh, err := r.bwb[0].Block.ParentHash()
	if err != nil {
		r.err = errors.Wrap(prysmsync.ErrInvalidFetchedData, err.Error())
		f.downscorePeer(r.blocksFrom, r.err)
		return
	}
	zeroHash := [32]byte{}
	for i, b := range r.bwb[1:] {
		nh, err := b.Block.ParentHash()
		if err != nil {
			r.err = errors.Wrap(prysmsync.ErrInvalidFetchedData, err.Error())
			f.downscorePeer(r.blocksFrom, r.err)
			return
		}
		if nh == bh {
			continue
		}
		// When the previous block is genesis (slot 0, parentHash all zeros), the
		// transition to a new parentHash at the next block is from the genesis
		// execution payload embedded in the state, not from an envelope.
		if bh == zeroHash && r.bwb[i].Block.Block().Slot() == 0 {
			bh = nh
			continue
		}
		if pidx >= len(r.envelopes) {
			log.Debug("Not enough envelopes corresponding to blocks, truncating the block batch")
			r.bwb = r.bwb[:i+1]
			return
		}
		env := r.envelopes[pidx]
		full, err := blocks.BlockBuiltOnEnvelope(env, b.Block)
		if err != nil || !full {
			log.WithFields(logrus.Fields{
				"blockIdx":        i + 1,
				"blockSlot":       b.Block.Block().Slot(),
				"blockParentHash": fmt.Sprintf("%#x", nh),
				"envelopeIdx":     pidx,
				"envelopeSlot":    envelopeSlotStr(env),
				"envelopeHash":    envelopeBlockHashStr(env),
				"prevParentHash":  fmt.Sprintf("%#x", bh),
				"matchErr":        err,
			}).Error("Envelope does not match block during initial sync")
			r.err = errors.Wrap(prysmsync.ErrInvalidFetchedData, "envelope does not match block")
			if r.blocksFrom == r.payloadsFrom {
				f.downscorePeer(r.blocksFrom, r.err)
			}
			return
		}
		bh = nh
		pidx++
	}
	if pidx < len(r.envelopes) {
		// Check if the next envelope belongs to the last block in the batch.
		lastBlock := r.bwb[len(r.bwb)-1]
		env, err := r.envelopes[pidx].Envelope()
		if err == nil && env.BeaconBlockRoot() == lastBlock.Block.Root() {
			r.envelopes = r.envelopes[:pidx+1]
		} else {
			log.Debug("Not enough blocks in batch, truncating envelopes")
			r.envelopes = r.envelopes[:pidx]
		}
	}
}

// fetchPayloads fetches execution payload envelopes correponding to blocks in
// `response.bwb`.
// `pid` is the initial peer to request payload from (usually the peer from which the block originated).
// `peers` is a list of peers to use for the request payloads if `pid` fails.
// `r.bwb` must be sorted by slot.
func (f *blocksFetcher) fetchPayloads(ctx context.Context, r *fetchRequestResponse, peers []peer.ID) {
	if len(r.bwb) == 0 {
		r.payloadsFrom = ""
		return
	}

	firstGloasIndex, err := findFirstForkIndex(r.bwb, version.Gloas)
	if err != nil {
		r.err = errors.Wrap(err, "find first Gloas index")
		r.payloadsFrom = ""
		return
	}
	if firstGloasIndex == len(r.bwb) {
		r.payloadsFrom = ""
		return
	}
	if firstGloasIndex > 0 {
		// We leave the first Gloas block so that the post-state is a post-CL state.
		log.Debug("Batch across the Fulu/Gloas fork, truncating it")
		r.bwb = r.bwb[:firstGloasIndex+1]
		r.payloadsFrom = ""
		return
	}

	// The whole block batch is gloas
	start := r.start
	if start > 0 {
		start--
	}
	envelopes, pid, err := f.fetchPayloadEnvelopesFromPeer(ctx, start, r.count, r.blocksFrom, peers)
	if err != nil {
		r.err = errors.Wrap(err, "fetch payload envelopes from peer")
		r.payloadsFrom = ""
		return
	}
	r.envelopes = envelopes
	r.payloadsFrom = pid
	log.WithFields(logrus.Fields{
		"reqStart":     start,
		"reqCount":     r.count,
		"rStart":       r.start,
		"numBlocks":    len(r.bwb),
		"numEnvelopes": len(envelopes),
		"firstBlockSlot": func() primitives.Slot {
			if len(r.bwb) > 0 {
				return r.bwb[0].Block.Block().Slot()
			}
			return 0
		}(),
		"firstEnvelopeSlot": func() string {
			if len(envelopes) > 0 {
				return envelopeSlotStr(envelopes[0])
			}
			return "none"
		}(),
	}).Debug("Fetched payloads for validation")
	f.validatePayloadBlockConsistency(r)
}

// fetchPayloadEnvelopesFromPeer fetches execution payload envelopes by range,
// trying pid first, then falling back to other peers.
func (f *blocksFetcher) fetchPayloadEnvelopesFromPeer(
	ctx context.Context,
	start primitives.Slot,
	count uint64,
	pid peer.ID,
	peers []peer.ID,
) ([]interfaces.ROSignedExecutionPayloadEnvelope, peer.ID, error) {
	ctx, span := trace.StartSpan(ctx, "initialsync.fetchPayloadEnvelopesFromPeer")
	defer span.End()

	req := &p2ppb.ExecutionPayloadEnvelopesByRangeRequest{
		StartSlot: start,
		Count:     count,
	}
	peers = f.filterPeers(ctx, peers, peersPercentagePerRequest)
	// Try the block provider first, then best bandwidth peers, then the rest.
	peers = append([]peer.ID{pid}, peers...)
	bestPeers := f.hasSufficientBandwidth(peers, req.Count)
	peers = append(bestPeers, peers...)
	peers = dedupPeers(peers)
	for _, p := range peers {
		envelopes, err := prysmsync.SendExecutionPayloadEnvelopesByRangeRequest(ctx, f.clock, f.p2p, p, f.ctxMap, req)
		if err != nil {
			log.WithFields(logrus.Fields{
				"peer":      p,
				"startSlot": req.StartSlot,
				"count":     req.Count,
			}).WithError(err).Debug("Could not request payload envelopes by range from peer")
			if errors.Is(err, prysmsync.ErrInvalidFetchedData) {
				f.downscorePeer(p, err)
			}
			continue
		}
		f.p2p.Peers().Scorers().BlockProviderScorer().Touch(p)
		roEnvelopes := make([]interfaces.ROSignedExecutionPayloadEnvelope, 0, len(envelopes))
		for _, env := range envelopes {
			wrapped, err := blocks.WrappedROSignedExecutionPayloadEnvelope(env)
			if err != nil {
				log.WithField("peer", p).WithError(err).Debug("Invalid payload envelope in response")
				continue
			}
			roEnvelopes = append(roEnvelopes, wrapped)
		}
		return roEnvelopes, p, nil
	}
	return nil, "", errNoPeersAvailable
}

func envelopeSlotStr(env interfaces.ROSignedExecutionPayloadEnvelope) string {
	msg, err := env.Envelope()
	if err != nil {
		return fmt.Sprintf("err:%v", err)
	}
	return fmt.Sprintf("%d", msg.Slot())
}

func envelopeBlockHashStr(env interfaces.ROSignedExecutionPayloadEnvelope) string {
	msg, err := env.Envelope()
	if err != nil {
		return fmt.Sprintf("err:%v", err)
	}
	ex, err := msg.Execution()
	if err != nil {
		return fmt.Sprintf("err:%v", err)
	}
	return fmt.Sprintf("%#x", ex.BlockHash())
}
