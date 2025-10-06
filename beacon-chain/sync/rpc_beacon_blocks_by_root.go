package sync

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/execution"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/sync/verify"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
)

// sendBeaconBlocksRequest sends the `requests` beacon blocks by root requests to
// the peer with the given `id`. For each received block, it inserts the block into the
// pending queue. Then, for each received blocks, it checks if all corresponding sidecars
// are stored, and, if not, sends the corresponding sidecar requests and stores the received sidecars.
// For sidecars, only blob sidecars will be requested to the peer with the given `id`.
// For other types of sidecars, the request will be sent to the best peers.
func (s *Service) sendBeaconBlocksRequest(ctx context.Context, requests *types.BeaconBlockByRootsReq, id peer.ID) error {
	ctx, cancel := context.WithTimeout(ctx, respTimeout)
	defer cancel()

	requestedRoots := make(map[[fieldparams.RootLength]byte]bool)
	for _, root := range *requests {
		requestedRoots[root] = true
	}

	blks, err := SendBeaconBlocksByRootRequest(ctx, s.cfg.clock, s.cfg.p2p, id, requests, func(blk interfaces.ReadOnlySignedBeaconBlock) error {
		blkRoot, err := blk.Block().HashTreeRoot()
		if err != nil {
			return err
		}

		if ok := requestedRoots[blkRoot]; !ok {
			return fmt.Errorf("received unexpected block with root %x", blkRoot)
		}

		s.pendingQueueLock.Lock()
		defer s.pendingQueueLock.Unlock()

		if err := s.insertBlockToPendingQueue(blk.Block().Slot(), blk, blkRoot); err != nil {
			return errors.Wrapf(err, "insert block to pending queue for block with root %x", blkRoot)
		}

		return nil
	})

	// The following part deals with sidecars.
	postFuluBlocks := make([]blocks.ROBlock, 0, len(blks))
	for _, blk := range blks {
		blockVersion := blk.Version()

		if blockVersion >= version.Fulu {
			roBlock, err := blocks.NewROBlock(blk)
			if err != nil {
				return errors.Wrap(err, "new ro block")
			}

			postFuluBlocks = append(postFuluBlocks, roBlock)

			continue
		}

		if blockVersion >= version.Deneb {
			if err := s.requestAndSaveMissingBlobSidecars(blk, id); err != nil {
				return errors.Wrap(err, "request and save missing blob sidecars")
			}

			continue
		}
	}

	if err := s.requestAndSaveMissingDataColumnSidecars(postFuluBlocks); err != nil {
		return errors.Wrap(err, "request and save missing data columns")
	}

	return err
}

// requestAndSaveMissingDataColumns checks if the data columns are missing for the given block.
// If so, requests them and saves them to the storage.
func (s *Service) requestAndSaveMissingDataColumnSidecars(blks []blocks.ROBlock) error {
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
		Ctx:         s.ctx,
		Tor:         s.cfg.clock,
		P2P:         s.cfg.p2p,
		CtxMap:      s.ctxMap,
		Storage:     s.cfg.dataColumnStorage,
		NewVerifier: s.newColumnsVerifier,
	}

	sidecarsByRoot, missingIndicesByRoot, err := FetchDataColumnSidecars(params, blks, info.CustodyColumns)
	if err != nil {
		return errors.Wrap(err, "fetch data column sidecars")
	}

	if len(missingIndicesByRoot) > 0 {
		prettyMissingIndicesByRoot := make(map[string]string, len(missingIndicesByRoot))
		for root, indices := range missingIndicesByRoot {
			prettyMissingIndicesByRoot[fmt.Sprintf("%#x", root)] = helpers.SortedPrettySliceFromMap(indices)
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
func (s *Service) beaconBlocksRootRPCHandler(ctx context.Context, msg interface{}, stream libp2pcore.Stream) error {
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
