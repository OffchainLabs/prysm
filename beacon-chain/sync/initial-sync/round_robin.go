package initialsync

import (
	"context"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/paulbellamy/ratecounter"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/transition"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/das"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db/filesystem"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/sync"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
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
func (s *Service) roundRobinSync(genesis time.Time) error {
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	transition.SkipSlotCache.Disable()
	defer transition.SkipSlotCache.Enable()

	s.counter = ratecounter.NewRateCounter(counterSeconds * time.Second)

	// Step 1 - Sync to end of finalized epoch.
	if err := s.syncToFinalizedEpoch(ctx, genesis); err != nil {
		return err
	}

	// Already at head, no need for 2nd phase.
	if s.cfg.Chain.HeadSlot() == slots.Since(genesis) {
		return nil
	}

	// Step 2 - sync to head from majority of peers (from no less than MinimumSyncPeers*2 peers)
	// having the same world view on non-finalized epoch.
	return s.syncToNonFinalizedEpoch(ctx, genesis)
}

func (s *Service) startBlocksQueue(ctx context.Context, highestSlot primitives.Slot, mode syncMode) (*blocksQueue, error) {
	vr := s.clock.GenesisValidatorsRoot()
	ctxMap, err := sync.ContextByteVersionsForValRoot(vr)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to initialize context version map using genesis validator root = %#x", vr)
	}

	summarizer, err := s.cfg.BlobStorage.WaitForSummarizer(ctx)
	if err != nil {
		// The summarizer is an optional optimization, we can continue without, only stop if there is a different error.
		if !errors.Is(err, filesystem.ErrBlobStorageSummarizerUnavailable) {
			return nil, err
		}
		summarizer = nil // This should already be nil, but we'll set it just to be safe.
	}
	cfg := &blocksQueueConfig{
		p2p:                 s.cfg.P2P,
		db:                  s.cfg.DB,
		chain:               s.cfg.Chain,
		clock:               s.clock,
		ctxMap:              ctxMap,
		highestExpectedSlot: highestSlot,
		mode:                mode,
		bs:                  summarizer,
		bv:                  s.newBlobVerifier,
		cv:                  s.newDataColumnsVerifier,
	}
	queue := newBlocksQueue(ctx, cfg)
	if err := queue.start(); err != nil {
		return nil, err
	}
	return queue, nil
}

// syncToFinalizedEpoch sync from head to the best known finalized epoch.
func (s *Service) syncToFinalizedEpoch(ctx context.Context, genesis time.Time) error {
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
		s.processFetchedData(ctx, genesis, s.cfg.Chain.HeadSlot(), data)
	}

	log.WithFields(logrus.Fields{
		"syncedSlot":  s.cfg.Chain.HeadSlot(),
		"currentSlot": slots.Since(genesis),
	}).Info("Synced to finalized epoch - now syncing blocks up to current head")
	if err := queue.stop(); err != nil {
		log.WithError(err).Debug("Error stopping queue")
	}

	return nil
}

// syncToNonFinalizedEpoch sync from head to best known non-finalized epoch supported by majority
// of peers (no less than MinimumSyncPeers*2 peers).
func (s *Service) syncToNonFinalizedEpoch(ctx context.Context, genesis time.Time) error {
	queue, err := s.startBlocksQueue(ctx, slots.Since(genesis), modeNonConstrained)
	if err != nil {
		return err
	}
	for data := range queue.fetchedData {
		s.processFetchedDataRegSync(ctx, genesis, s.cfg.Chain.HeadSlot(), data)
	}
	log.WithFields(logrus.Fields{
		"syncedSlot":  s.cfg.Chain.HeadSlot(),
		"currentSlot": slots.Since(genesis),
	}).Info("Synced to head of chain")
	if err := queue.stop(); err != nil {
		log.WithError(err).Debug("Error stopping queue")
	}

	return nil
}

// processFetchedData processes data received from queue.
func (s *Service) processFetchedData(
	ctx context.Context, genesis time.Time, startSlot primitives.Slot, data *blocksQueueFetchedData) {
	defer s.updatePeerScorerStats(data.pid, startSlot)

	// Use Batch Block Verify to process and verify batches directly.
	if err := s.processBatchedBlocks(ctx, genesis, data.bwb, s.cfg.Chain.ReceiveBlockBatch); err != nil {
		log.WithError(err).Warn("Skip processing batched blocks")
	}
}

// processFetchedDataRegSync processes data received from queue.
func (s *Service) processFetchedDataRegSync(
	ctx context.Context, genesis time.Time, startSlot primitives.Slot, data *blocksQueueFetchedData) {
	defer s.updatePeerScorerStats(data.pid, startSlot)

	bwb, err := validUnprocessed(ctx, data.bwb, s.cfg.Chain.HeadSlot(), s.isProcessedBlock)
	if err != nil {
		log.WithError(err).Debug("batch did not contain a valid sequence of unprocessed blocks")
		return
	}

	if len(bwb) == 0 {
		return
	}

	// Compute the first electra slot.
	firstElectraSlot, err := slots.EpochStart(params.BeaconConfig().ElectraForkEpoch)
	if err != nil {
		firstElectraSlot = math.MaxUint64
	}

	// Find the first block with a slot greater than or equal to the first electra slot.
	// (Blocks are sorted by slot)
	firstElectraIndex := sort.Search(len(bwb), func(i int) bool {
		return bwb[i].Block.Block().Slot() >= firstElectraSlot
	})

	preElectraBwbs := bwb[:firstElectraIndex]
	postElectraBwbs := bwb[firstElectraIndex:]

	blobBatchVerifier := verification.NewBlobBatchVerifier(s.newBlobVerifier, verification.InitsyncBlobSidecarRequirements)
	lazilyPersistentStore := das.NewLazilyPersistentStore(s.cfg.BlobStorage, blobBatchVerifier)

	log := log.WithField("firstSlot", data.bwb[0].Block.Block().Slot())

	logPre := log
	if len(preElectraBwbs) > 0 {
		logPre = logPre.WithField("firstUnprocessed", preElectraBwbs[0].Block.Block().Slot())
	}

	for _, b := range preElectraBwbs {
		log := logPre.WithFields(syncFields(b.Block))

		if err := lazilyPersistentStore.Persist(s.clock.CurrentSlot(), b.Blobs...); err != nil {
			log.WithError(err).Warning("Batch failure due to BlobSidecar issues")
			return
		}

		if err := s.processBlock(ctx, genesis, b, s.cfg.Chain.ReceiveBlock, lazilyPersistentStore); err != nil {
			switch {
			case errors.Is(err, errParentDoesNotExist):
				log.
					WithField("missingParent", fmt.Sprintf("%#x", b.Block.Block().ParentRoot())).
					Debug("Could not process batch blocks due to missing parent")
				return
			default:
				log.WithError(err).Warning("Block processing failure")
				return
			}
		}
	}

	logPost := log
	if len(postElectraBwbs) > 0 {
		logPost = log.WithField("firstUnprocessed", postElectraBwbs[0].Block.Block().Slot())
	}

	lazilyPersistentStoreColumn := das.NewLazilyPersistentStoreColumn(s.cfg.BlobStorage)

	for _, b := range postElectraBwbs {
		log := logPost.WithFields(syncFields(b.Block))

		if err := lazilyPersistentStoreColumn.PersistColumns(s.clock.CurrentSlot(), b.Columns...); err != nil {
			log.WithError(err).Warning("Batch failure due to DataColumnSidecar issues")
			return
		}

		if err := s.processBlock(ctx, genesis, b, s.cfg.Chain.ReceiveBlock, lazilyPersistentStoreColumn); err != nil {
			switch {
			case errors.Is(err, errParentDoesNotExist):
				log.
					WithField("missingParent", fmt.Sprintf("%#x", b.Block.Block().ParentRoot())).
					Debug("Could not process batch blocks due to missing parent")
				return
			default:
				log.WithError(err).Warning("Block processing failure")
				return
			}
		}
	}
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
		timeRemaining := time.Duration(float64(slots.Since(genesis)-blk.Slot())/rate) * time.Second
		log.WithFields(logrus.Fields{
			"peers":           len(s.cfg.P2P.Peers().Connected()),
			"blocksPerSecond": fmt.Sprintf("%.1f", rate),
		}).Infof(
			"Processing block %s %d/%d - estimated time remaining %s",
			fmt.Sprintf("0x%s...", hex.EncodeToString(blkRoot[:])[:8]),
			blk.Slot(), slots.Since(genesis), timeRemaining,
		)
	}
}

// logBatchSyncStatus and increments the block processing counter.
func (s *Service) logBatchSyncStatus(genesis time.Time, firstBlk blocks.ROBlock, nBlocks int) {
	s.counter.Incr(int64(nBlocks))
	rate := float64(s.counter.Rate()) / counterSeconds
	if rate == 0 {
		rate = 1
	}
	firstRoot := firstBlk.Root()
	timeRemaining := time.Duration(float64(slots.Since(genesis)-firstBlk.Block().Slot())/rate) * time.Second
	log.WithFields(logrus.Fields{
		"peers":                           len(s.cfg.P2P.Peers().Connected()),
		"blocksPerSecond":                 fmt.Sprintf("%.1f", rate),
		"batchSize":                       nBlocks,
		"startingFrom":                    fmt.Sprintf("0x%s...", hex.EncodeToString(firstRoot[:])[:8]),
		"latestProcessedSlot/currentSlot": fmt.Sprintf("%d/%d", firstBlk.Block().Slot(), slots.Since(genesis)),
		"estimatedTimeRemaining":          timeRemaining,
	}).Info("Processing blocks")
}

// processBlock performs basic checks on incoming block, and triggers receiver function.
func (s *Service) processBlock(
	ctx context.Context,
	genesis time.Time,
	bwb blocks.BlockWithROBlobs,
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

func validUnprocessed(ctx context.Context, bwb []blocks.BlockWithROBlobs, headSlot primitives.Slot, isProc processedChecker) ([]blocks.BlockWithROBlobs, error) {
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

func (s *Service) processPreElectraBatchedBlocks(
	ctx context.Context,
	bwbs []blocks.BlockWithROBlobs,
	bFunc batchBlockReceiverFn,
	genesis time.Time,
	firstBlock blocks.ROBlock,
) error {
	bwbCount := len(bwbs)
	if bwbCount == 0 {
		return nil
	}

	batchVerifier := verification.NewBlobBatchVerifier(s.newBlobVerifier, verification.InitsyncBlobSidecarRequirements)
	persistentStore := das.NewLazilyPersistentStore(s.cfg.BlobStorage, batchVerifier)
	s.logBatchSyncStatus(genesis, firstBlock, bwbCount)

	for _, bwb := range bwbs {
		if len(bwb.Blobs) == 0 {
			continue
		}

		if err := persistentStore.Persist(s.clock.CurrentSlot(), bwb.Blobs...); err != nil {
			return errors.Wrap(err, "persisting blobs")
		}
	}

	if err := bFunc(ctx, blocks.BlockWithROBlobsSlice(bwbs).ROBlocks(), persistentStore); err != nil {
		return errors.Wrap(err, "process pre-electra blocks")
	}

	return nil
}

func (s *Service) processPostElectraBatchedBlocks(
	ctx context.Context,
	bwbs []blocks.BlockWithROBlobs,
	bFunc batchBlockReceiverFn,
	genesis time.Time,
	firstBlock blocks.ROBlock,
) error {
	bwbCount := len(bwbs)

	if bwbCount == 0 {
		return nil
	}

	persistentStoreColumn := das.NewLazilyPersistentStoreColumn(s.cfg.BlobStorage)
	s.logBatchSyncStatus(genesis, firstBlock, bwbCount)
	for _, bwb := range bwbs {
		if len(bwb.Columns) == 0 {
			continue
		}

		if err := persistentStoreColumn.PersistColumns(s.clock.CurrentSlot(), bwb.Columns...); err != nil {
			return errors.Wrap(err, "persisting columns")
		}
	}

	if err := bFunc(ctx, blocks.BlockWithROBlobsSlice(bwbs).ROBlocks(), persistentStoreColumn); err != nil {
		return errors.Wrap(err, "process post-electra blocks")
	}

	return nil
}

func (s *Service) processBatchedBlocks(
	ctx context.Context,
	genesis time.Time,
	bwbs []blocks.BlockWithROBlobs,
	bFunc batchBlockReceiverFn,
) error {
	if len(bwbs) == 0 {
		return errors.New("0 blocks provided into method")
	}

	headSlot := s.cfg.Chain.HeadSlot()

	bwbs, err := validUnprocessed(ctx, bwbs, headSlot, s.isProcessedBlock)
	if err != nil {
		return errors.Wrap(err, "validating unprocessed blocks")
	}

	if len(bwbs) == 0 {
		return nil
	}

	firstBlock := bwbs[0].Block
	if !s.cfg.Chain.HasBlock(ctx, firstBlock.Block().ParentRoot()) {
		return fmt.Errorf("%w: %#x (in processBatchedBlocks, slot=%d)",
			errParentDoesNotExist, firstBlock.Block().ParentRoot(), firstBlock.Block().Slot())
	}

	// Compute the first electra slot.
	firstElectraSlot, err := slots.EpochStart(params.BeaconConfig().ElectraForkEpoch)
	if err != nil {
		firstElectraSlot = math.MaxUint64
	}

	// Find the first block with a slot greater than or equal to the first electra slot.
	// (Blocks are sorted by slot)
	firstElectraIndex := sort.Search(len(bwbs), func(i int) bool {
		return bwbs[i].Block.Block().Slot() >= firstElectraSlot
	})

	preElectraBwbs, postElectraBwbs := bwbs[:firstElectraIndex], bwbs[firstElectraIndex:]

	if err := s.processPreElectraBatchedBlocks(ctx, preElectraBwbs, bFunc, genesis, firstBlock); err != nil {
		return errors.Wrap(err, "process pre-electra blocks")
	}

	if err := s.processPostElectraBatchedBlocks(ctx, postElectraBwbs, bFunc, genesis, firstBlock); err != nil {
		return errors.Wrap(err, "process post-electra blocks")
	}

	return nil
}

// updatePeerScorerStats adjusts monitored metrics for a peer.
func (s *Service) updatePeerScorerStats(pid peer.ID, startSlot primitives.Slot) {
	if pid == "" {
		return
	}
	headSlot := s.cfg.Chain.HeadSlot()
	if startSlot >= headSlot {
		return
	}
	if diff := s.cfg.Chain.HeadSlot() - startSlot; diff > 0 {
		scorer := s.cfg.P2P.Peers().Scorers().BlockProviderScorer()
		scorer.IncrementProcessedBlocks(pid, uint64(diff))
	}
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
