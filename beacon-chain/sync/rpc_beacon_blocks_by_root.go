package sync

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/execution"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/sync/verify"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/container/slice"
	"github.com/OffchainLabs/prysm/v7/crypto/rand"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
)

// parentSource tells where to request the unknown parents of the blocks received by root.
type parentSource uint8

const (
	fromServingPeer parentSource = iota //requests them only from the peer that served the blocks
	fromBestPeers                       //requests them from the best peers.
)

// sendBeaconBlocksRequest requests the blocks with the given roots from the peer `id`, and queues the received ones.
// It then fetches their missing sidecars: blobs from `id`, data columns from the best peers.
// The unknown parents of the newly queued blocks are requested in the background from the best peers
// if their child is from the proposer expected by the head state, or from `parentsFrom` otherwise.
// `slotBounds` optionally maps a requested root to the slot of its known child: the served block must have a lower slot.
// It returns the roots of the received blocks.
func (s *Service) sendBeaconBlocksRequest(
	ctx context.Context,
	requests *types.BeaconBlockByRootsReq,
	id peer.ID,
	parentsFrom parentSource,
	slotBounds map[[fieldparams.RootLength]byte]primitives.Slot,
) (map[[fieldparams.RootLength]byte]bool, error) {
	ctx, cancel := context.WithTimeout(ctx, respTimeout)
	defer cancel()

	requestedRoots := make(map[[fieldparams.RootLength]byte]bool)
	for _, root := range *requests {
		requestedRoots[root] = true
	}

	finalizedSlot, err := slots.EpochStart(s.cfg.chain.FinalizedCheckpt().Epoch)
	if err != nil {
		return nil, fmt.Errorf("finalized epoch start slot: %w", err)
	}

	received := make(map[[fieldparams.RootLength]byte]bool, len(*requests))

	// The parents of the newly queued blocks, with the slot of their child.
	parents := make(map[[fieldparams.RootLength]byte]primitives.Slot)

	// The parents to request from the best peers.
	trustedParents := make(map[[fieldparams.RootLength]byte]bool)

	blks, err := SendBeaconBlocksByRootRequest(ctx, s.cfg.clock, s.cfg.p2p, id, requests, func(blk interfaces.ReadOnlySignedBeaconBlock) error {
		blkRoot, err := blk.Block().HashTreeRoot()
		if err != nil {
			return err
		}

		if ok := requestedRoots[blkRoot]; !ok {
			return fmt.Errorf("received unexpected block with root %x", blkRoot)
		}

		// Each step back must lower the slot.
		slot := blk.Block().Slot()
		if bound, ok := slotBounds[blkRoot]; ok && slot >= bound {
			return fmt.Errorf("received block with root %x at slot %d, not before its child at slot %d", blkRoot, slot, bound)
		}

		if err := slots.VerifyTime(s.cfg.clock.GenesisTime(), slot, earlyBlockProcessingTolerance); err != nil {
			return fmt.Errorf("received block with root %x from the future: %w", blkRoot, err)
		}

		// Verify the signature before queueing, matching the gossip path, since a matched root does not cover it.
		if _, err := s.verifyPendingBlockSignature(ctx, id, blk, blkRoot); err != nil {
			return errors.Wrapf(err, "verify block signature for block with root %x", blkRoot)
		}

		// As on the gossip path, a block from the expected proposer is trusted, so is its parent.
		isTrusted := parentsFrom == fromBestPeers || s.isExpectedProposer(ctx, blk)

		s.pendingQueueLock.Lock()
		defer s.pendingQueueLock.Unlock()

		received[blkRoot] = true

		alreadyQueued := s.seenPendingBlocks[blkRoot]
		if err := s.insertBlockToPendingQueue(slot, blk, blkRoot); err != nil {
			return errors.Wrapf(err, "insert block to pending queue for block with root %x", blkRoot)
		}

		// Walk back only from newly queued blocks after the finalized slot.
		// Keep the lowest child slot.
		newlyQueued := !alreadyQueued && s.seenPendingBlocks[blkRoot]
		parentRoot := blk.Block().ParentRoot()
		if !newlyQueued || slot <= finalizedSlot {
			return nil
		}

		if bound, ok := parents[parentRoot]; !ok || slot < bound {
			parents[parentRoot] = slot
		}

		if isTrusted {
			trustedParents[parentRoot] = true
		}

		return nil
	})

	bestPeersParents := make([][fieldparams.RootLength]byte, 0, len(trustedParents))
	servingPeerParents := make([][fieldparams.RootLength]byte, 0, len(parents))
	for root := range parents {
		if s.cfg.chain.HasBlock(ctx, root) {
			continue
		}

		if trustedParents[root] {
			bestPeersParents = append(bestPeersParents, root)
			continue
		}

		servingPeerParents = append(servingPeerParents, root)
	}

	if len(bestPeersParents) > 0 {
		go func() {
			if err := s.sendBatchRootRequest(s.ctx, bestPeersParents, parents, rand.NewGenerator()); err != nil {
				log.WithError(err).WithField("count", len(bestPeersParents)).Debug("Could not request unknown parents from the best peers")
			}
		}()
	}

	if len(servingPeerParents) > 0 {
		go s.requestBlocksAccountable(s.ctx, id, servingPeerParents, parents)
	}

	// The following part deals with sidecars.
	postFuluBlocks := make([]blocks.ROBlock, 0, len(blks))
	for _, blk := range blks {
		blockVersion := blk.Version()

		if blockVersion >= version.Fulu {
			roBlock, err := blocks.NewROBlock(blk)
			if err != nil {
				return received, fmt.Errorf("new ro block: %w", err)
			}

			postFuluBlocks = append(postFuluBlocks, roBlock)

			continue
		}

		if blockVersion >= version.Deneb {
			if err := s.requestAndSaveMissingBlobSidecars(blk, id); err != nil {
				return received, fmt.Errorf("request and save missing blob sidecars: %w", err)
			}

			continue
		}
	}

	if err := s.requestAndSaveMissingDataColumnSidecars(postFuluBlocks); err != nil {
		return received, fmt.Errorf("request and save missing data columns: %w", err)
	}

	return received, err
}

// requestAndSaveMissingDataColumns checks if the data columns are missing for the given block.
// If so, requests them and saves them to the storage.
func (s *Service) requestAndSaveMissingDataColumnSidecars(blks []blocks.ROBlock) error {
	if len(blks) == 0 {
		return nil
	}

	// Process any gossip columns queued before the block arrived.
	for _, blk := range blks {
		s.processPendingGloasColumns(s.ctx, blk.Root(), blk)
	}

	preGloasBlocks := make([]blocks.ROBlock, 0, len(blks))
	for _, blk := range blks {
		if blk.Version() < version.Gloas {
			preGloasBlocks = append(preGloasBlocks, blk)
		}
	}

	return s.fetchAndSaveDataColumnSidecars(preGloasBlocks)
}

// fetchAndSaveDataColumnSidecars fetches the missing custody columns for the given blocks
// and saves them to the storage, failing if any remain missing.
func (s *Service) fetchAndSaveDataColumnSidecars(blks []blocks.ROBlock) error {
	if len(blks) == 0 {
		return nil
	}

	samplesPerSlot := params.BeaconConfig().SamplesPerSlot

	custodyGroupCount, err := s.cfg.p2p.CustodyGroupCount(s.ctx)
	if err != nil {
		return errors.Wrap(err, "custody group count")
	}

	samplingSize := max(custodyGroupCount, samplesPerSlot)
	info, _, err := peerdas.Info(s.cfg.p2p.NodeID(), samplingSize)
	if err != nil {
		return errors.Wrap(err, "custody info")
	}

	// Fetch missing data column sidecars.
	params := DataColumnSidecarsParams{
		Ctx:           s.ctx,
		Tor:           s.cfg.clock,
		P2P:           s.cfg.p2p,
		CtxMap:        s.ctxMap,
		Storage:       s.cfg.dataColumnStorage,
		NewVerifier:   s.newColumnsVerifier,
		RequestByRoot: true,
	}

	sidecarsByRoot, missingIndicesByRoot, err := FetchDataColumnSidecars(params, blks, info.CustodyColumns)
	if err != nil {
		return errors.Wrap(err, "fetch data column sidecars")
	}

	if len(missingIndicesByRoot) > 0 {
		prettyMissingIndicesByRoot := make(map[string]string, len(missingIndicesByRoot))
		for root, indices := range missingIndicesByRoot {
			prettyMissingIndicesByRoot[fmt.Sprintf("%#x", root)] = slice.SortedPrettySliceFromMap(indices)
		}
		return errors.Errorf("some sidecars are still missing after fetch: %v", prettyMissingIndicesByRoot)
	}

	// Save the sidecars to the storage.
	count := 0
	for _, sidecars := range sidecarsByRoot {
		count += len(sidecars)
	}

	sidecarsToSave := make([]blocks.VerifiedRODataColumn, 0, count)
	for _, sidecars := range sidecarsByRoot {
		sidecarsToSave = append(sidecarsToSave, sidecars...)
	}

	if err := s.cfg.dataColumnStorage.Save(sidecarsToSave); err != nil {
		return errors.Wrap(err, "save")
	}

	return nil
}

func (s *Service) requestAndSaveMissingBlobSidecars(block interfaces.ReadOnlySignedBeaconBlock, peerID peer.ID) error {
	blockRoot, err := block.Block().HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "hash tree root")
	}

	request, err := s.pendingBlobsRequestForBlock(blockRoot, block)
	if err != nil {
		return errors.Wrap(err, "pending blobs request for block")
	}

	if len(request) == 0 {
		return nil
	}

	if err := s.sendAndSaveBlobSidecars(s.ctx, request, peerID, block); err != nil {
		return errors.Wrap(err, "send and save blob sidecars")
	}

	return nil
}

// beaconBlocksRootRPCHandler looks up the request blocks from the database from the given block roots.
func (s *Service) beaconBlocksRootRPCHandler(ctx context.Context, msg any, stream libp2pcore.Stream) error {
	ctx, cancel := context.WithTimeout(ctx, ttfbTimeout)
	defer cancel()
	SetRPCStreamDeadlines(stream)
	log := log.WithField("handler", "beacon_blocks_by_root")

	rawMsg, ok := msg.(*types.BeaconBlockByRootsReq)
	if !ok {
		return errors.New("message is not type BeaconBlockByRootsReq")
	}
	blockRoots := *rawMsg
	if err := s.rateLimiter.validateRequest(stream, uint64(len(blockRoots))); err != nil {
		return err
	}
	if len(blockRoots) == 0 {
		// Add to rate limiter in the event no
		// roots are requested.
		s.rateLimiter.add(stream, 1)
		s.writeErrorResponseToStream(responseCodeInvalidRequest, "no block roots provided in request", stream)
		return errors.New("no block roots provided")
	}

	remotePeer := stream.Conn().RemotePeer()

	currentEpoch := slots.ToEpoch(s.cfg.clock.CurrentSlot())
	if uint64(len(blockRoots)) > params.MaxRequestBlock(currentEpoch) {
		s.downscorePeer(remotePeer, "beaconBlocksRootRPCHandlerTooManyRoots")
		s.writeErrorResponseToStream(responseCodeInvalidRequest, "requested more than the max block limit", stream)
		return errors.New("requested more than the max block limit")
	}
	s.rateLimiter.add(stream, int64(len(blockRoots)))

	for _, root := range blockRoots {
		blk, err := s.cfg.beaconDB.Block(ctx, root)
		if err != nil {
			log.WithError(err).Debug("Could not fetch block")
			s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
			return err
		}
		if err := blocks.BeaconBlockIsNil(blk); err != nil {
			continue
		}

		if blk.Block().IsBlinded() {
			blk, err = s.cfg.executionReconstructor.ReconstructFullBlock(ctx, blk)
			if err != nil {
				if errors.Is(err, execution.ErrEmptyBlockHash) {
					log.WithError(err).Warn("Could not reconstruct block from header with syncing execution client. Waiting to complete syncing")
				} else {
					log.WithError(err).Error("Could not get reconstruct full block from blinded body")
				}
				s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
				return err
			}
		}

		if err := s.chunkBlockWriter(stream, blk); err != nil {
			return err
		}
	}

	closeStream(stream, log)
	return nil
}

// sendAndSaveBlobSidecars sends the blob request and saves received sidecars.
func (s *Service) sendAndSaveBlobSidecars(ctx context.Context, request types.BlobSidecarsByRootReq, peerID peer.ID, block interfaces.ReadOnlySignedBeaconBlock) error {
	if len(request) == 0 {
		return nil
	}

	sidecars, err := SendBlobSidecarByRoot(ctx, s.cfg.clock, s.cfg.p2p, peerID, s.ctxMap, &request, block.Block().Slot())
	if err != nil {
		return err
	}

	RoBlock, err := blocks.NewROBlock(block)
	if err != nil {
		return err
	}
	if len(sidecars) != len(request) {
		return fmt.Errorf("received %d blob sidecars, expected %d for RPC", len(sidecars), len(request))
	}
	bv := verification.NewBlobBatchVerifier(s.newBlobVerifier, verification.PendingQueueBlobSidecarRequirements)
	for _, sidecar := range sidecars {
		if err := verify.BlobAlignsWithBlock(sidecar, RoBlock); err != nil {
			return err
		}
		log.WithFields(blobFields(sidecar)).Debug("Received blob sidecar RPC")
	}
	vscs, err := bv.VerifiedROBlobs(ctx, RoBlock, sidecars)
	if err != nil {
		return err
	}
	for i := range vscs {
		if err := s.cfg.blobStorage.Save(vscs[i]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) pendingBlobsRequestForBlock(root [32]byte, b interfaces.ReadOnlySignedBeaconBlock) (types.BlobSidecarsByRootReq, error) {
	if b.Version() < version.Deneb {
		return nil, nil // Block before deneb has no blob.
	}
	cc, err := b.Block().Body().BlobKzgCommitments()
	if err != nil {
		return nil, err
	}
	if len(cc) == 0 {
		return nil, nil
	}
	return s.constructPendingBlobsRequest(root, len(cc))
}

// constructPendingBlobsRequest creates a request for BlobSidecars by root, considering blobs already in DB.
func (s *Service) constructPendingBlobsRequest(root [32]byte, commitments int) (types.BlobSidecarsByRootReq, error) {
	if commitments == 0 {
		return nil, nil
	}
	summary := s.cfg.blobStorage.Summary(root)

	return requestsForMissingIndices(summary, commitments, root), nil
}

// requestsForMissingIndices constructs a slice of BlobIdentifiers that are missing from
// local storage, based on a mapping that represents which indices are locally stored,
// and the highest expected index.
func requestsForMissingIndices(stored filesystem.BlobStorageSummary, commitments int, root [32]byte) []*eth.BlobIdentifier {
	var ids []*eth.BlobIdentifier
	for i := uint64(0); i < uint64(commitments); i++ {
		if !stored.HasIndex(i) {
			ids = append(ids, &eth.BlobIdentifier{Index: i, BlockRoot: root[:]})
		}
	}
	return ids
}
