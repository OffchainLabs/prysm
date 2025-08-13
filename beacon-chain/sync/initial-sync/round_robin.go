package initialsync

import (
	"context"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/sync"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/paulbellamy/ratecounter"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	// counterSeconds is an interval over which an average rate will be calculated.
	counterSeconds = 20
)

// blockReceiverFn defines block receiving function.
type blockReceiverFn func(ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock, blockRoot [32]byte, avs das.AvailabilityStore) error

// batchBlockReceiverFn defines batch receiving function.
type batchBlockReceiverFn func(ctx context.Context, blks []blocks.ROBlock, avs das.AvailabilityStore) error

// Round Robin sync looks at the latest peer statuses and syncs up to the highest known epoch.
//
// Step 1 - Sync to finalized epoch.
// Sync with peers having the majority on best finalized epoch greater than node's head state.
//
// Step 2 - Sync to head from finalized epoch.
// Using enough peers (at least, MinimumSyncPeers*2, for example) obtain best non-finalized epoch,
// known to majority of the peers, and keep fetching blocks, up until that epoch is reached.
func (s *Service) roundRobinSync() error {
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	transition.SkipSlotCache.Disable()
	defer transition.SkipSlotCache.Enable()

	s.counter = ratecounter.NewRateCounter(counterSeconds * time.Second)

	// Step 1 - Sync to end of finalized epoch.
	if err := s.syncToFinalizedEpoch(ctx); err != nil {
		return err
	}

	// Already at head, no need for 2nd phase.
	if s.cfg.Chain.HeadSlot() == slots.CurrentSlot(s.genesisTime) {
		return nil
	}

	// Step 2 - sync to head from majority of peers (from no less than MinimumSyncPeers*2 peers)
	// having the same world view on non-finalized epoch.
	return s.syncToNonFinalizedEpoch(ctx)
}

func (s *Service) startBlocksQueue(ctx context.Context, highestSlot primitives.Slot, mode syncMode) (*blocksQueue, error) {
	vr := s.clock.GenesisValidatorsRoot()
	ctxMap, err := sync.ContextByteVersionsForValRoot(vr)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to initialize context version map using genesis validator root = %#x", vr)
	}

	cfg := &blocksQueueConfig{
		p2p:                 s.cfg.P2P,
		db:                  s.cfg.DB,
		chain:               s.cfg.Chain,
		clock:               s.clock,
		ctxMap:              ctxMap,
		highestExpectedSlot: highestSlot,
		mode:                mode,
		bs:                  s.cfg.BlobStorage,
		dcs:                 s.cfg.DataColumnStorage,
		cv:                  s.newDataColumnsVerifier,
	}
	queue := newBlocksQueue(ctx, cfg)
	if err := queue.start(); err != nil {
		return nil, err
	}
	return queue, nil
}

// syncToFinalizedEpoch sync from head to the best known finalized epoch.
func (s *Service) syncToFinalizedEpoch(ctx context.Context) error {
	highestFinalizedSlot, err := slots.EpochStart(s.highestFinalizedEpoch())
	if err != nil {
		return err
	}
	if s.cfg.Chain.HeadSlot() >= highestFinalizedSlot {
		// No need to sync, already synced to the finalized slot.
		log.Debug("Already synced to finalized epoch")
		return nil
	}

	queue, err := s.startBlocksQueue(ctx, highestFinalizedSlot, modeStopOnFinalizedEpoch)
	if err != nil {
		return err
	}

	for data := range queue.fetchedData {
		s.processFetchedData(ctx, data)
	}

	log.WithFields(logrus.Fields{
		"syncedSlot":  s.cfg.Chain.HeadSlot(),
		"currentSlot": slots.CurrentSlot(s.genesisTime),
	}).Info("Synced to finalized epoch - now syncing blocks up to current head")
	if err := queue.stop(); err != nil {
		log.WithError(err).Debug("Error stopping queue")
	}

	return nil
}

// syncToNonFinalizedEpoch sync from head to best known non-finalized epoch supported by majority
// of peers (no less than MinimumSyncPeers*2 peers).
func (s *Service) syncToNonFinalizedEpoch(ctx context.Context) error {
	queue, err := s.startBlocksQueue(ctx, slots.CurrentSlot(s.genesisTime), modeNonConstrained)
	if err != nil {
		return err
	}
	for data := range queue.fetchedData {
		count, err := s.processFetchedDataRegSync(ctx, data)
		s.updatePeerScorerStats(data, count, err)
	}
	log.WithFields(logrus.Fields{
		"syncedSlot":  s.cfg.Chain.HeadSlot(),
		"currentSlot": slots.CurrentSlot(s.genesisTime),
	}).Info("Synced to head of chain")
	if err := queue.stop(); err != nil {
		log.WithError(err).Debug("Error stopping queue")
	}

	return nil
}

// processFetchedData processes data received from queue.
func (s *Service) processFetchedData(ctx context.Context, data *blocksQueueFetchedData) {
	// Use Batch Block Verify to process and verify batches directly.
	count, err := s.processBatchedBlocks(ctx, data.bwb, s.cfg.Chain.ReceiveBlockBatch)
	if err != nil {
		log.WithError(err).Warn("Skip processing batched blocks")
	}
	s.updatePeerScorerStats(data, count, err)
}

// processFetchedDataRegSync processes data received from queue.
func (s *Service) processFetchedDataRegSync(ctx context.Context, data *blocksQueueFetchedData) (uint64, error) {
	bwb, err := validUnprocessed(ctx, data.bwb, s.cfg.Chain.HeadSlot(), s.isProcessedBlock)
	if err != nil {
		log.WithError(err).Debug("Batch did not contain a valid sequence of unprocessed blocks")
		return 0, err
	}

	if len(bwb) == 0 {
		return 0, nil
	}

	// Separate blocks with blobs from blocks with data columns.
	fistDataColumnIndex := sort.Search(len(bwb), func(i int) bool {
		return bwb[i].Block.Version() >= version.Fulu
	})

	blocksWithBlobs := bwb[:fistDataColumnIndex]
	blocksWithDataColumns := bwb[fistDataColumnIndex:]

	blobBatchVerifier := verification.NewBlobBatchVerifier(s.newBlobVerifier, verification.InitsyncBlobSidecarRequirements)
	lazilyPersistentStoreBlobs := das.NewLazilyPersistentStore(s.cfg.BlobStorage, blobBatchVerifier)

	log := log.WithField("firstSlot", data.bwb[0].Block.Block().Slot())
	logBlobs, logDataColumns := log, log

	if len(blocksWithBlobs) > 0 {
		logBlobs = logBlobs.WithField("firstUnprocessed", blocksWithBlobs[0].Block.Block().Slot())
	}

	for i, b := range blocksWithBlobs {
		if err := lazilyPersistentStoreBlobs.Persist(s.clock.CurrentSlot(), b.Blobs...); err != nil {
			logBlobs.WithError(err).WithFields(syncFields(b.Block)).Warning("Batch failure due to BlobSidecar issues")
			return uint64(i), err
		}

		if err := s.processBlock(ctx, s.genesisTime, b, s.cfg.Chain.ReceiveBlock, lazilyPersistentStoreBlobs); err != nil {
			if errors.Is(err, errParentDoesNotExist) {
				logBlobs.WithField("missingParent", fmt.Sprintf("%#x", b.Block.Block().ParentRoot())).
					WithFields(syncFields(b.Block)).Debug("Could not process batch blocks due to missing parent")
			} else {
				logBlobs.WithError(err).WithFields(syncFields(b.Block)).Warn("Block processing failure")
			}

			return uint64(i), err
		}
	}

	if len(blocksWithDataColumns) == 0 {
		return uint64(len(bwb)), nil
	}

	// Save data column sidecars.
	count := 0
	for _, b := range blocksWithDataColumns {
		count += len(b.Columns)
	}

	sidecarsToSave := make([]blocks.VerifiedRODataColumn, 0, count)
	for _, blockWithDataColumns := range blocksWithDataColumns {
		sidecarsToSave = append(sidecarsToSave, blockWithDataColumns.Columns...)
	}

	if err := s.cfg.DataColumnStorage.Save(sidecarsToSave); err != nil {
		return 0, errors.Wrap(err, "save data column sidecars")
	}

	for i, b := range blocksWithDataColumns {
		logDataColumns := logDataColumns.WithFields(syncFields(b.Block))

		if err := s.processBlock(ctx, s.genesisTime, b, s.cfg.Chain.ReceiveBlock, nil); err != nil {
			switch {
			case errors.Is(err, errParentDoesNotExist):
				logDataColumns.
					WithField("missingParent", fmt.Sprintf("%#x", b.Block.Block().ParentRoot())).
					Debug("Could not process batch blocks due to missing parent")
				return uint64(i), err
			default:
				logDataColumns.WithError(err).Warning("Block processing failure")
				return uint64(i), err
			}
		}
	}
	return uint64(len(bwb)), nil
}

func syncFields(b blocks.ROBlock) logrus.Fields {
	return logrus.Fields{
		"root":     fmt.Sprintf("%#x", b.Root()),
		"lastSlot": b.Block().Slot(),
	}
}

// highestFinalizedEpoch returns the absolute highest finalized epoch of all connected peers.
// It returns `0` if no peers are connected.
// Note this can be lower than our finalized epoch if our connected peers are all behind us.
func (s *Service) highestFinalizedEpoch() primitives.Epoch {
	highest := primitives.Epoch(0)
	for _, pid := range s.cfg.P2P.Peers().Connected() {
		peerChainState, err := s.cfg.P2P.Peers().ChainState(pid)

		if err != nil || peerChainState == nil {
			continue
		}

		if peerChainState.FinalizedEpoch > highest {
			highest = peerChainState.FinalizedEpoch
		}
	}

	return highest
}

// logSyncStatus and increment block processing counter.
func (s *Service) logSyncStatus(genesis time.Time, blk interfaces.ReadOnlyBeaconBlock, blkRoot [32]byte) {
	s.counter.Incr(1)
	rate := float64(s.counter.Rate()) / counterSeconds
	if rate == 0 {
		rate = 1
	}
	if slots.IsEpochStart(blk.Slot()) {
		timeRemaining := time.Duration(float64(slots.CurrentSlot(genesis)-blk.Slot())/rate) * time.Second
		log.WithFields(logrus.Fields{
			"peers":           len(s.cfg.P2P.Peers().Connected()),
			"blocksPerSecond": fmt.Sprintf("%.1f", rate),
		}).Infof(
			"Processing block %s %d/%d - estimated time remaining %s",
			fmt.Sprintf("0x%s...", hex.EncodeToString(blkRoot[:])[:8]),
			blk.Slot(), slots.CurrentSlot(genesis), timeRemaining,
		)
	}
}

// logBatchSyncStatus and increments the block processing counter.
func (s *Service) logBatchSyncStatus(firstBlk blocks.ROBlock, nBlocks int) {
	genesis := s.genesisTime
	s.counter.Incr(int64(nBlocks))
	rate := float64(s.counter.Rate()) / counterSeconds
	if rate == 0 {
		rate = 1
	}
	firstRoot := firstBlk.Root()
	timeRemaining := time.Duration(float64(slots.CurrentSlot(genesis)-firstBlk.Block().Slot())/rate) * time.Second
	log.WithFields(logrus.Fields{
		"peers":                           len(s.cfg.P2P.Peers().Connected()),
		"blocksPerSecond":                 fmt.Sprintf("%.1f", rate),
		"batchSize":                       nBlocks,
		"startingFrom":                    fmt.Sprintf("0x%s...", hex.EncodeToString(firstRoot[:])[:8]),
		"latestProcessedSlot/currentSlot": fmt.Sprintf("%d/%d", firstBlk.Block().Slot(), slots.CurrentSlot(genesis)),
		"estimatedTimeRemaining":          timeRemaining,
	}).Info("Processing blocks")
}

// processBlock performs basic checks on incoming block, and triggers receiver function.
func (s *Service) processBlock(
	ctx context.Context,
	genesis time.Time,
	bwb blocks.BlockWithROSidecars,
	blockReceiver blockReceiverFn,
	avs das.AvailabilityStore,
) error {
	blk := bwb.Block
	blkRoot := blk.Root()
	if s.isProcessedBlock(ctx, blk) {
		return fmt.Errorf("slot: %d , root %#x: %w", blk.Block().Slot(), blkRoot, errBlockAlreadyProcessed)
	}

	s.logSyncStatus(genesis, blk.Block(), blkRoot)
	if !s.cfg.Chain.HasBlock(ctx, blk.Block().ParentRoot()) {
		return fmt.Errorf("%w: (in processBlock, slot=%d) %#x", errParentDoesNotExist, blk.Block().Slot(), blk.Block().ParentRoot())
	}
	return blockReceiver(ctx, blk, blkRoot, avs)
}

type processedChecker func(context.Context, blocks.ROBlock) bool

func validUnprocessed(ctx context.Context, bwb []blocks.BlockWithROSidecars, headSlot primitives.Slot, isProc processedChecker) ([]blocks.BlockWithROSidecars, error) {
	// use a pointer to avoid confusing the zero-value with the case where the first element is processed.
	var processed *int
	for i := range bwb {
		b := bwb[i].Block
		if headSlot >= b.Block().Slot() && isProc(ctx, b) {
			val := i
			processed = &val
			continue
		}
		if i > 0 {
			parent := bwb[i-1].Block
			if parent.Root() != b.Block().ParentRoot() {
				return nil, fmt.Errorf("expected linear block list with parent root of %#x (slot %d) but received %#x (slot %d)",
					parent.Root(), parent.Block().Slot(), b.Block().ParentRoot(), b.Block().Slot())
			}
		}
	}
	if processed == nil {
		return bwb, nil
	}
	if *processed+1 == len(bwb) {
		maxIncoming := bwb[len(bwb)-1].Block
		maxRoot := maxIncoming.Root()
		return nil, fmt.Errorf("%w: headSlot=%d, blockSlot=%d, root=%#x", errBlockAlreadyProcessed, headSlot, maxIncoming.Block().Slot(), maxRoot)
	}
	nonProcessedIdx := *processed + 1
	return bwb[nonProcessedIdx:], nil
}

func (s *Service) processBatchedBlocks(ctx context.Context, bwb []blocks.BlockWithROSidecars, bFunc batchBlockReceiverFn) (uint64, error) {
	bwbCount := uint64(len(bwb))
	if bwbCount == 0 {
		return 0, errors.New("0 blocks provided into method")
	}

	headSlot := s.cfg.Chain.HeadSlot()
	bwb, err := validUnprocessed(ctx, bwb, headSlot, s.isProcessedBlock)
	if err != nil {
		return 0, err
	}

	firstBlock := bwb[0].Block
	if !s.cfg.Chain.HasBlock(ctx, firstBlock.Block().ParentRoot()) {
		return 0, fmt.Errorf("%w: %#x (in processBatchedBlocks, slot=%d)",
			errParentDoesNotExist, firstBlock.Block().ParentRoot(), firstBlock.Block().Slot())
	}

	firstFuluIndex, err := findFirstFuluIndex(bwb)
	if err != nil {
		return 0, errors.Wrap(err, "finding first Fulu index")
	}

	blocksWithBlobs := bwb[:firstFuluIndex]
	blocksWithDataColumns := bwb[firstFuluIndex:]

	if err := s.processBlocksWithBlobs(ctx, blocksWithBlobs, bFunc, firstBlock); err != nil {
		return 0, errors.Wrap(err, "processing blocks with blobs")
	}

	if err := s.processBlocksWithDataColumns(ctx, blocksWithDataColumns, bFunc, firstBlock); err != nil {
		return 0, errors.Wrap(err, "processing blocks with data columns")
	}

	return bwbCount, nil
}

func (s *Service) processBlocksWithBlobs(ctx context.Context, bwbs []blocks.BlockWithROSidecars, bFunc batchBlockReceiverFn, firstBlock blocks.ROBlock) error {
	bwbCount := len(bwbs)
	if bwbCount == 0 {
		return nil
	}

	batchVerifier := verification.NewBlobBatchVerifier(s.newBlobVerifier, verification.InitsyncBlobSidecarRequirements)
	persistentStore := das.NewLazilyPersistentStore(s.cfg.BlobStorage, batchVerifier)
	s.logBatchSyncStatus(firstBlock, bwbCount)

	for _, bwb := range bwbs {
		if len(bwb.Blobs) == 0 {
			continue
		}

		if err := persistentStore.Persist(s.clock.CurrentSlot(), bwb.Blobs...); err != nil {
			return errors.Wrap(err, "persisting blobs")
		}
	}

	robs := blocks.BlockWithROBlobsSlice(bwbs).ROBlocks()
	if err := bFunc(ctx, robs, persistentStore); err != nil {
		return errors.Wrap(err, "processing blocks with blobs")
	}

	return nil
}

func (s *Service) processBlocksWithDataColumns(ctx context.Context, bwbs []blocks.BlockWithROSidecars, bFunc batchBlockReceiverFn, firstBlock blocks.ROBlock) error {
	bwbCount := len(bwbs)
	if bwbCount == 0 {
		return nil
	}

	s.logBatchSyncStatus(firstBlock, bwbCount)

	// Save data column sidecars.
	count := 0
	for _, bwb := range bwbs {
		count += len(bwb.Columns)
	}

	sidecarsToSave := make([]blocks.VerifiedRODataColumn, 0, count)
	for _, blockWithDataColumns := range bwbs {
		sidecarsToSave = append(sidecarsToSave, blockWithDataColumns.Columns...)
	}

	if err := s.cfg.DataColumnStorage.Save(sidecarsToSave); err != nil {
		return errors.Wrap(err, "save data column sidecars")
	}

	robs := blocks.BlockWithROBlobsSlice(bwbs).ROBlocks()
	if err := bFunc(ctx, robs, nil); err != nil {
		return errors.Wrap(err, "process post-Fulu blocks")
	}

	return nil
}

func isPunishableError(err error) bool {
	return errors.Is(err, verification.ErrInvalid)
}

// updatePeerScorerStats adjusts monitored metrics for a peer.
func (s *Service) updatePeerScorerStats(data *blocksQueueFetchedData, count uint64, err error) {
	if isPunishableError(err) {
		if verification.IsBlobValidationFailure(err) {
			s.downscorePeer(data.blobsFrom, "invalidBlobs")
		} else {
			s.downscorePeer(data.blocksFrom, "invalidBlocks")
		}

		// If the error is punishable, exit here so that we don't give them credit for providing bad blocks.
		return
	}
	s.cfg.P2P.Peers().Scorers().BlockProviderScorer().IncrementProcessedBlocks(data.blocksFrom, count)
}

// isProcessedBlock checks DB and local cache for presence of a given block, to avoid duplicates.
func (s *Service) isProcessedBlock(ctx context.Context, blk blocks.ROBlock) bool {
	cp := s.cfg.Chain.FinalizedCheckpt()
	finalizedSlot, err := slots.EpochStart(cp.Epoch)
	if err != nil {
		return false
	}
	// If block is before our finalized checkpoint
	// we do not process it.
	if blk.Block().Slot() <= finalizedSlot {
		return true
	}
	// If block exists in our db and is before or equal to our current head
	// we ignore it.
	if s.cfg.Chain.HeadSlot() >= blk.Block().Slot() && s.cfg.Chain.HasBlock(ctx, blk.Root()) {
		return true
	}
	return false
}

func (s *Service) downscorePeer(peerID peer.ID, reason string) {
	newScore := s.cfg.P2P.Peers().Scorers().BadResponsesScorer().Increment(peerID)
	log.WithFields(logrus.Fields{"peerID": peerID, "reason": reason, "newScore": newScore}).Debug("Downscore peer")
}
