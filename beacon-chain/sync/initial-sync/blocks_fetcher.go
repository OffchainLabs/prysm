package initialsync

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	coreTime "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/time"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db/filesystem"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	p2pTypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/types"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/startup"
	prysmsync "github.com/prysmaticlabs/prysm/v5/beacon-chain/sync"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/sync/verify"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	"github.com/prysmaticlabs/prysm/v5/cmd/beacon-chain/flags"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	leakybucket "github.com/prysmaticlabs/prysm/v5/container/leaky-bucket"
	"github.com/prysmaticlabs/prysm/v5/crypto/rand"
	mathPrysm "github.com/prysmaticlabs/prysm/v5/math"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	p2ppb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/sirupsen/logrus"
)

const (
	// maxPendingRequests limits how many concurrent fetch request one can initiate.
	maxPendingRequests = 64
	// peersPercentagePerRequest caps percentage of peers to be used in a request.
	peersPercentagePerRequest = 0.75
	// peersPercentagePerRequestDataColumns caps percentage of peers to be used in a data columns request.
	peersPercentagePerRequestDataColumns = 1.
	// handshakePollingInterval is a polling interval for checking the number of received handshakes.
	handshakePollingInterval = 5 * time.Second
	// peerLocksPollingInterval is a polling interval for checking if there are stale peer locks.
	peerLocksPollingInterval = 5 * time.Minute
	// peerLockMaxAge is maximum time before stale lock is purged.
	peerLockMaxAge = 60 * time.Minute
	// nonSkippedSlotsFullSearchEpochs how many epochs to check in full, before resorting to random
	// sampling of slots once per epoch
	nonSkippedSlotsFullSearchEpochs = 10
	// peerFilterCapacityWeight defines how peer's capacity affects peer's score. Provided as
	// percentage, i.e. 0.3 means capacity will determine 30% of peer's score.
	peerFilterCapacityWeight = 0.2
	// backtrackingMaxHops how many hops (during search for common ancestor in backtracking) to do
	// before giving up.
	backtrackingMaxHops = 128
)

var (
	errNoPeersAvailable      = errors.New("no peers available, waiting for reconnect")
	errFetcherCtxIsDone      = errors.New("fetcher's context is done, reinitialize")
	errSlotIsTooHigh         = errors.New("slot is higher than the finalized slot")
	errBlockAlreadyProcessed = errors.New("block is already processed")
	errParentDoesNotExist    = errors.New("beacon node doesn't have a parent in db with root")
	errNoPeersWithAltBlocks  = errors.New("no peers with alternative blocks found")
)

// Period to calculate expected limit for a single peer.
var blockLimiterPeriod = 30 * time.Second

// blocksFetcherConfig is a config to setup the block fetcher.
type blocksFetcherConfig struct {
	clock                    *startup.Clock
	ctxMap                   prysmsync.ContextByteVersions
	chain                    blockchainService
	p2p                      p2p.P2P
	db                       db.ReadOnlyDatabase
	peerFilterCapacityWeight float64
	mode                     syncMode
	bs                       filesystem.BlobStorageSummarizer
	bv                       verification.NewBlobVerifier
	cv                       verification.NewColumnVerifier
}

// blocksFetcher is a service to fetch chain data from peers.
// On an incoming requests, requested block range is evenly divided
// among available peers (for fair network load distribution).
type blocksFetcher struct {
	sync.Mutex
	ctx             context.Context
	cancel          context.CancelFunc
	rand            *rand.Rand
	chain           blockchainService
	clock           *startup.Clock
	ctxMap          prysmsync.ContextByteVersions
	p2p             p2p.P2P
	db              db.ReadOnlyDatabase
	bs              filesystem.BlobStorageSummarizer
	bv              verification.NewBlobVerifier
	cv              verification.NewColumnVerifier
	blocksPerPeriod uint64
	rateLimiter     *leakybucket.Collector
	peerLocks       map[peer.ID]*peerLock
	fetchRequests   chan *fetchRequestParams
	fetchResponses  chan *fetchRequestResponse
	capacityWeight  float64       // how remaining capacity affects peer selection
	mode            syncMode      // allows to use fetcher in different sync scenarios
	quit            chan struct{} // termination notifier
}

// peerLock restricts fetcher actions on per peer basis. Currently, used for rate limiting.
type peerLock struct {
	sync.Mutex
	accessed time.Time
}

// fetchRequestParams holds parameters necessary to schedule a fetch request.
type fetchRequestParams struct {
	ctx   context.Context // if provided, it is used instead of global fetcher's context
	start primitives.Slot // starting slot
	count uint64          // how many slots to receive (fetcher may return fewer slots)
}

// fetchRequestResponse is a combined type to hold results of both successful executions and errors.
// Valid usage pattern will be to check whether result's `err` is nil, before using `blocks`.
type fetchRequestResponse struct {
	pid   peer.ID
	start primitives.Slot
	count uint64
	bwb   []blocks.BlockWithROBlobs
	err   error
}

// newBlocksFetcher creates ready to use fetcher.
func newBlocksFetcher(ctx context.Context, cfg *blocksFetcherConfig) *blocksFetcher {
	blockBatchLimit := maxBatchLimit()
	blocksPerPeriod := blockBatchLimit
	allowedBlocksBurst := flags.Get().BlockBatchLimitBurstFactor * blockBatchLimit
	// Allow fetcher to go almost to the full burst capacity (less a single batch).
	rateLimiter := leakybucket.NewCollector(
		float64(blocksPerPeriod), int64(allowedBlocksBurst-blocksPerPeriod),
		blockLimiterPeriod, false /* deleteEmptyBuckets */)

	capacityWeight := cfg.peerFilterCapacityWeight
	if capacityWeight >= 1 {
		capacityWeight = peerFilterCapacityWeight
	}

	ctx, cancel := context.WithCancel(ctx)
	return &blocksFetcher{
		ctx:             ctx,
		cancel:          cancel,
		rand:            rand.NewGenerator(),
		chain:           cfg.chain,
		clock:           cfg.clock,
		ctxMap:          cfg.ctxMap,
		p2p:             cfg.p2p,
		db:              cfg.db,
		bs:              cfg.bs,
		bv:              cfg.bv,
		cv:              cfg.cv,
		blocksPerPeriod: uint64(blocksPerPeriod),
		rateLimiter:     rateLimiter,
		peerLocks:       make(map[peer.ID]*peerLock),
		fetchRequests:   make(chan *fetchRequestParams, maxPendingRequests),
		fetchResponses:  make(chan *fetchRequestResponse, maxPendingRequests),
		capacityWeight:  capacityWeight,
		mode:            cfg.mode,
		quit:            make(chan struct{}),
	}
}

// This specifies the block batch limit the initial sync fetcher will use. In the event the user has provided
// and excessive number, this is automatically lowered.
func maxBatchLimit() int {
	currLimit := flags.Get().BlockBatchLimit
	maxLimit := params.BeaconConfig().MaxRequestBlocks
	if params.DenebEnabled() {
		maxLimit = params.BeaconConfig().MaxRequestBlocksDeneb
	}
	castedMaxLimit, err := mathPrysm.Int(maxLimit)
	if err != nil {
		// Should be impossible to hit this case.
		log.WithError(err).Error("Unable to calculate the max batch limit")
		return currLimit
	}
	if currLimit > castedMaxLimit {
		log.Warnf("Specified batch size exceeds the block limit of the network, lowering from %d to %d", currLimit, maxLimit)
		currLimit = castedMaxLimit
	}
	return currLimit
}

// start boots up the fetcher, which starts listening for incoming fetch requests.
func (f *blocksFetcher) start() error {
	select {
	case <-f.ctx.Done():
		return errFetcherCtxIsDone
	default:
		go f.loop()
		return nil
	}
}

// stop terminates all fetcher operations.
func (f *blocksFetcher) stop() {
	defer func() {
		if f.rateLimiter != nil {
			f.rateLimiter.Free()
			f.rateLimiter = nil
		}
	}()
	f.cancel()
	<-f.quit // make sure that loop() is done
}

// requestResponses exposes a channel into which fetcher pushes generated request responses.
func (f *blocksFetcher) requestResponses() <-chan *fetchRequestResponse {
	return f.fetchResponses
}

// loop is a main fetcher loop, listens for incoming requests/cancellations, forwards outgoing responses.
func (f *blocksFetcher) loop() {
	defer close(f.quit)

	// Wait for all loop's goroutines to finish, and safely release resources.
	wg := &sync.WaitGroup{}
	defer func() {
		wg.Wait()
		close(f.fetchResponses)
	}()

	// Periodically remove stale peer locks.
	go func() {
		ticker := time.NewTicker(peerLocksPollingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				f.removeStalePeerLocks(peerLockMaxAge)
			case <-f.ctx.Done():
				return
			}
		}
	}()

	// Main loop.
	for {
		// Make sure there are available peers before processing requests.
		if _, err := f.waitForMinimumPeers(f.ctx); err != nil {
			log.Error(err)
		}

		select {
		case <-f.ctx.Done():
			log.Debug("Context closed, exiting goroutine (blocks fetcher)")
			return
		case req := <-f.fetchRequests:
			wg.Add(1)
			go func() {
				defer wg.Done()
				select {
				case <-f.ctx.Done():
				case f.fetchResponses <- f.handleRequest(req.ctx, req.start, req.count):
				}
			}()
		}
	}
}

// scheduleRequest adds request to incoming queue.
func (f *blocksFetcher) scheduleRequest(ctx context.Context, start primitives.Slot, count uint64) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	request := &fetchRequestParams{
		ctx:   ctx,
		start: start,
		count: count,
	}
	select {
	case <-f.ctx.Done():
		return errFetcherCtxIsDone
	case f.fetchRequests <- request:
	}
	return nil
}

// handleRequest parses fetch request and forwards it to response builder.
func (f *blocksFetcher) handleRequest(ctx context.Context, start primitives.Slot, count uint64) *fetchRequestResponse {
	ctx, span := trace.StartSpan(ctx, "initialsync.handleRequest")
	defer span.End()

	response := &fetchRequestResponse{
		start: start,
		count: count,
		bwb:   []blocks.BlockWithROBlobs{},
		err:   nil,
	}

	if ctx.Err() != nil {
		response.err = ctx.Err()
		return response
	}

	_, targetEpoch, peers := f.calculateHeadAndTargetEpochs()
	if len(peers) == 0 {
		response.err = errNoPeersAvailable
		return response
	}

	// Short circuit start far exceeding the highest finalized epoch in some infinite loop.
	if f.mode == modeStopOnFinalizedEpoch {
		highestFinalizedSlot := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(targetEpoch + 1))
		if start > highestFinalizedSlot {
			response.err = fmt.Errorf(
				"%w, slot: %d, highest finalized slot: %d",
				errSlotIsTooHigh, start, highestFinalizedSlot,
			)

			return response
		}
	}

	response.bwb, response.pid, response.err = f.fetchBlocksFromPeer(ctx, start, count, peers)

	if response.err != nil {
		return response
	}

	if coreTime.PeerDASIsActive(start) {
		response.err = f.fetchDataColumnsFromPeers(ctx, response.bwb, nil)
		return response
	}

	if err := f.fetchBlobsFromPeer(ctx, response.bwb, response.pid, peers); err != nil {
		response.err = err
	}

	return response
}

// fetchBlocksFromPeer fetches blocks from a single randomly selected peer.
func (f *blocksFetcher) fetchBlocksFromPeer(
	ctx context.Context,
	start primitives.Slot, count uint64,
	peers []peer.ID,
) ([]blocks.BlockWithROBlobs, peer.ID, error) {
	ctx, span := trace.StartSpan(ctx, "initialsync.fetchBlocksFromPeer")
	defer span.End()

	peers = f.filterPeers(ctx, peers, peersPercentagePerRequest)
	req := &p2ppb.BeaconBlocksByRangeRequest{
		StartSlot: start,
		Count:     count,
		Step:      1,
	}
	bestPeers := f.hasSufficientBandwidth(peers, req.Count)
	// We append the best peers to the front so that higher capacity
	// peers are dialed first.
	peers = append(bestPeers, peers...)
	peers = dedupPeers(peers)
	for i := 0; i < len(peers); i++ {
		p := peers[i]
		blocks, err := f.requestBlocks(ctx, req, p)
		if err != nil {
			log.WithField("peer", p).WithError(err).Debug("Could not request blocks by range from peer")
			continue
		}
		f.p2p.Peers().Scorers().BlockProviderScorer().Touch(p)
		robs, err := sortedBlockWithVerifiedBlobSlice(blocks)
		if err != nil {
			log.WithField("peer", p).WithError(err).Debug("invalid BeaconBlocksByRange response")
			continue
		}
		return robs, p, err
	}
	return nil, "", errNoPeersAvailable
}

func sortedBlockWithVerifiedBlobSlice(blks []interfaces.ReadOnlySignedBeaconBlock) ([]blocks.BlockWithROBlobs, error) {
	rb := make([]blocks.BlockWithROBlobs, len(blks))
	for i, b := range blks {
		ro, err := blocks.NewROBlock(b)
		if err != nil {
			return nil, err
		}
		rb[i] = blocks.BlockWithROBlobs{Block: ro}
	}
	sort.Sort(blocks.BlockWithROBlobsSlice(rb))
	return rb, nil
}

type commitmentCount struct {
	slot  primitives.Slot
	root  [32]byte
	count int
}

type commitmentCountList []commitmentCount

// countCommitments makes a list of all blocks that have commitments that need to be satisfied.
// This gives us a representation to finish building the request that is lightweight and readable for testing.
func countCommitments(bwb []blocks.BlockWithROBlobs, retentionStart primitives.Slot) commitmentCountList {
	if len(bwb) == 0 {
		return nil
	}
	// Short-circuit if the highest block is before the deneb start epoch or retention period start.
	// This assumes blocks are sorted by sortedBlockWithVerifiedBlobSlice.
	// bwb is sorted by slot, so if the last element is outside the retention window, no blobs are needed.
	if bwb[len(bwb)-1].Block.Block().Slot() < retentionStart {
		return nil
	}
	fc := make([]commitmentCount, 0, len(bwb))
	for i := range bwb {
		b := bwb[i]
		slot := b.Block.Block().Slot()
		if b.Block.Version() < version.Deneb {
			continue
		}
		if slot < retentionStart {
			continue
		}
		commits, err := b.Block.Block().Body().BlobKzgCommitments()
		if err != nil || len(commits) == 0 {
			continue
		}
		fc = append(fc, commitmentCount{slot: slot, root: b.Block.Root(), count: len(commits)})
	}
	return fc
}

// func slotRangeForCommitmentCounts(cc []commitmentCount, bs filesystem.BlobStorageSummarizer) *blobRange {
func (cc commitmentCountList) blobRange(bs filesystem.BlobStorageSummarizer) *blobRange {
	if len(cc) == 0 {
		return nil
	}
	// If we don't have a blob summarizer, can't check local blobs, request blobs over complete range.
	if bs == nil {
		return &blobRange{low: cc[0].slot, high: cc[len(cc)-1].slot}
	}
	for i := range cc {
		hci := cc[i]
		// This list is always ordered by increasing slot, per req/resp validation rules.
		// Skip through slots until we find one with missing blobs.
		if bs.Summary(hci.root).AllAvailable(hci.count) {
			continue
		}
		// The slow of the first missing blob is the lower bound.
		// If we don't find an upper bound, we'll have a 1 slot request (same low/high).
		needed := &blobRange{low: hci.slot, high: hci.slot}
		// Iterate backward through the list to find the highest missing slot above the lower bound.
		// Return the complete range as soon as we find it; if lower bound is already the last element,
		// or if we never find an upper bound, we'll fall through to the bounds being equal after this loop.
		for z := len(cc) - 1; z > i; z-- {
			hcz := cc[z]
			if bs.Summary(hcz.root).AllAvailable(hcz.count) {
				continue
			}
			needed.high = hcz.slot
			return needed
		}
		return needed
	}
	return nil
}

type blobRange struct {
	low  primitives.Slot
	high primitives.Slot
}

func (r *blobRange) Request() *p2ppb.BlobSidecarsByRangeRequest {
	if r == nil {
		return nil
	}
	return &p2ppb.BlobSidecarsByRangeRequest{
		StartSlot: r.low,
		Count:     uint64(r.high.FlooredSubSlot(r.low)) + 1,
	}
}

var errBlobVerification = errors.New("peer unable to serve aligned BlobSidecarsByRange and BeaconBlockSidecarsByRange responses")
var errMissingBlobsForBlockCommitments = errors.Wrap(errBlobVerification, "blobs unavailable for processing block with kzg commitments")

// verifyAndPopulateBlobs mutate the input `bwb` argument by adding verified blobs.
// This function mutates the input `bwb` argument.
func verifyAndPopulateBlobs(bwb []blocks.BlockWithROBlobs, blobs []blocks.ROBlob, req *p2ppb.BlobSidecarsByRangeRequest, bss filesystem.BlobStorageSummarizer) error {
	blobsByRoot := make(map[[32]byte][]blocks.ROBlob)
	for i := range blobs {
		if blobs[i].Slot() < req.StartSlot {
			continue
		}
		br := blobs[i].BlockRoot()
		blobsByRoot[br] = append(blobsByRoot[br], blobs[i])
	}
	for i := range bwb {
		err := populateBlock(&bwb[i], blobsByRoot[bwb[i].Block.Root()], req, bss)
		if err != nil {
			if errors.Is(err, errDidntPopulate) {
				continue
			}
			return err
		}
	}
	return nil
}

var errDidntPopulate = errors.New("skipping population of block")

// populateBlock verifies and populates blobs for a block.
// This function mutates the input `bw` argument.
func populateBlock(bw *blocks.BlockWithROBlobs, blobs []blocks.ROBlob, req *p2ppb.BlobSidecarsByRangeRequest, bss filesystem.BlobStorageSummarizer) error {
	blk := bw.Block
	if blk.Version() < version.Deneb || blk.Block().Slot() < req.StartSlot {
		return errDidntPopulate
	}

	commits, err := blk.Block().Body().BlobKzgCommitments()
	if err != nil {
		return errDidntPopulate
	}

	if len(commits) == 0 {
		return errDidntPopulate
	}

	// Drop blobs on the floor if we already have them.
	if bss != nil && bss.Summary(blk.Root()).AllAvailable(len(commits)) {
		return errDidntPopulate
	}

	if len(commits) != len(blobs) {
		return missingCommitError(blk.Root(), blk.Block().Slot(), commits)
	}

	for ci := range commits {
		if err := verify.BlobAlignsWithBlock(blobs[ci], blk); err != nil {
			return err
		}
	}

	bw.Blobs = blobs
	return nil
}

func missingCommitError(root [32]byte, slot primitives.Slot, missing [][]byte) error {
	missStr := make([]string, 0, len(missing))
	for k := range missing {
		missStr = append(missStr, fmt.Sprintf("%#x", k))
	}
	return errors.Wrapf(errMissingBlobsForBlockCommitments,
		"block root %#x at slot %d missing %d commitments %s", root, slot, len(missing), strings.Join(missStr, ","))
}

// fetchBlobsFromPeer fetches blocks from a single randomly selected peer.
// This function mutates the input `bwb` argument.
func (f *blocksFetcher) fetchBlobsFromPeer(ctx context.Context, bwb []blocks.BlockWithROBlobs, pid peer.ID, peers []peer.ID) error {
	ctx, span := trace.StartSpan(ctx, "initialsync.fetchBlobsFromPeer")
	defer span.End()
	if slots.ToEpoch(f.clock.CurrentSlot()) < params.BeaconConfig().DenebForkEpoch {
		return nil
	}
	blobWindowStart, err := prysmsync.BlobRPCMinValidSlot(f.clock.CurrentSlot())
	if err != nil {
		return err
	}
	// Construct request message based on observed interval of blocks in need of blobs.
	req := countCommitments(bwb, blobWindowStart).blobRange(f.bs).Request()
	if req == nil {
		return nil
	}
	peers = f.filterPeers(ctx, peers, peersPercentagePerRequest)
	// We dial the initial peer first to ensure that we get the desired set of blobs.
	peers = append([]peer.ID{pid}, peers...)
	peers = f.hasSufficientBandwidth(peers, req.Count)
	// We append the best peers to the front so that higher capacity
	// peers are dialed first. If all of them fail, we fallback to the
	// initial peer we wanted to request blobs from.
	peers = append(peers, pid)
	for i := 0; i < len(peers); i++ {
		p := peers[i]
		blobs, err := f.requestBlobs(ctx, req, p)
		if err != nil {
			log.WithField("peer", p).WithError(err).Debug("Could not request blobs by range from peer")
			continue
		}
		f.p2p.Peers().Scorers().BlockProviderScorer().Touch(p)
		if err := verifyAndPopulateBlobs(bwb, blobs, req, f.bs); err != nil {
			log.WithField("peer", p).WithError(err).Debug("Invalid BeaconBlobsByRange response")
			continue
		}
		return err
	}
	return errNoPeersAvailable
}

// sortedSliceFromMap returns a sorted slice of keys from a map.
func sortedSliceFromMap(m map[uint64]bool) []uint64 {
	result := make([]uint64, 0, len(m))
	for k := range m {
		result = append(result, k)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i] < result[j]
	})

	return result
}

type bwbSlice struct {
	start, end  int
	dataColumns map[uint64]bool
}

// buildBwbSlices builds slices of `bwb` that aims to optimize the count of
// by range requests needed to fetch missing data columns.
func buildBwbSlices(
	bwbs []blocks.BlockWithROBlobs,
	missingColumnsByRoot map[[fieldparams.RootLength]byte]map[uint64]bool,
) ([]bwbSlice, error) {
	// Return early if there are no blocks to process.
	if len(bwbs) == 0 {
		return []bwbSlice{}, nil
	}

	// It's safe to get the first item of the slice since we've already checked that it's not empty.
	firstROBlock := bwbs[0].Block
	firstBlockRoot := firstROBlock.Root()

	previousMissingDataColumns := map[uint64]bool{}

	if missing, ok := missingColumnsByRoot[firstBlockRoot]; ok {
		previousMissingDataColumns = missing
	}

	previousBlockSlot := firstROBlock.Block().Slot()
	previousStartIndex := 0

	const offset = 1

	result := make([]bwbSlice, 0, 1)
	for currentIndexWithoutOffest, bwb := range bwbs[offset:] {
		currentIndex := currentIndexWithoutOffest + offset
		// Extract the ROBlock from the blockWithROBlob.
		currentROBlock := bwb.Block

		// Extract the current block from the current ROBlock.
		currentBlock := currentROBlock.Block()

		// Extract the slot from the block.
		currentBlockSlot := currentBlock.Slot()

		if currentBlockSlot < previousBlockSlot {
			return nil, errors.New("blocks are not sorted by slot")
		}

		// Extract KZG commitments count from the current block body
		currentBlockkzgCommitments, err := currentBlock.Body().BlobKzgCommitments()
		if err != nil {
			return nil, errors.Wrap(err, "blob KZG commitments")
		}

		// Compute the count of KZG commitments.
		currentBlockKzgCommitmentCount := len(currentBlockkzgCommitments)

		// Skip blocks without commitments.
		if currentBlockKzgCommitmentCount == 0 {
			previousBlockSlot = currentBlockSlot
			continue
		}

		// Extract the current block root from the current ROBlock.
		currentBlockRoot := currentROBlock.Root()

		// Get the missing data columns for the current block.
		missingDataColumns := missingColumnsByRoot[currentBlockRoot]

		// Compute if the missing data columns differ.
		missingDataColumnsDiffer := uint64MapDiffer(previousMissingDataColumns, missingDataColumns)

		// Check if there is a gap or if the missing data columns differ.
		if missingDataColumnsDiffer {
			// Append the slice to the result.
			slice := bwbSlice{
				start:       previousStartIndex,
				end:         currentIndex - 1,
				dataColumns: previousMissingDataColumns,
			}

			result = append(result, slice)

			previousStartIndex = currentIndex
			previousMissingDataColumns = missingDataColumns
		}

		previousBlockSlot = currentBlockSlot
	}

	// Append the last slice to the result.
	lastSlice := bwbSlice{
		start:       previousStartIndex,
		end:         len(bwbs) - 1,
		dataColumns: previousMissingDataColumns,
	}

	result = append(result, lastSlice)

	return result, nil
}

// uint64MapDiffer returns true if the two maps differ.
func uint64MapDiffer(left, right map[uint64]bool) bool {
	if len(left) != len(right) {
		return true
	}

	for k := range left {
		if !right[k] {
			return true
		}
	}

	return false
}

// custodyColumns returns the columns we should custody.
func (f *blocksFetcher) custodyColumns() (map[uint64]bool, error) {
	// Retrieve our node ID.
	localNodeID := f.p2p.NodeID()

	// Retrieve the number of colums subnets we should custody.
	localCustodySubnetCount := peerdas.CustodySubnetCount()

	// Retrieve the columns we should custody.
	localCustodyColumns, err := peerdas.CustodyColumns(localNodeID, localCustodySubnetCount)
	if err != nil {
		return nil, errors.Wrap(err, "custody columns")
	}

	return localCustodyColumns, nil
}

// missingColumnsFromRoot computes the columns corresponding to blocks in `bwbs` that
// we should custody and that are not in our store.
// The result is indexed by root.
func (f *blocksFetcher) missingColumnsFromRoot(
	custodyColumns map[uint64]bool,
	minSlot primitives.Slot,
	bwbs []blocks.BlockWithROBlobs,
) (map[[fieldparams.RootLength]byte]map[uint64]bool, error) {
	missingColumnsByRoot := make(map[[fieldparams.RootLength]byte]map[uint64]bool)
	for _, bwb := range bwbs {
		// Extract the roblock from the roblock with RO blobs.
		roblock := bwb.Block

		// Extract the block from the roblock.
		block := roblock.Block()

		// Extract the slot of the block.
		blockSlot := block.Slot()

		// Skip if the block slot is lower than the column window start.
		if blockSlot < minSlot {
			continue
		}

		// Retrieve the blob KZG kzgCommitments.
		kzgCommitments, err := roblock.Block().Body().BlobKzgCommitments()
		if err != nil {
			return nil, errors.Wrap(err, "blob KZG commitments")
		}

		// Skip if there are no KZG commitments.
		if len(kzgCommitments) == 0 {
			continue
		}

		// Extract the block root.
		root := roblock.Root()

		// Retrieve the summary for the root.
		summary := f.bs.Summary(root)

		// Compute the set of missing columns.
		for column := range custodyColumns {
			if !summary.HasDataColumnIndex(column) {
				if _, ok := missingColumnsByRoot[root]; !ok {
					missingColumnsByRoot[root] = make(map[uint64]bool)
				}
				missingColumnsByRoot[root][column] = true
			}
		}
	}

	return missingColumnsByRoot, nil
}

// indicesFromRoot returns the indices indexed by root.
func indicesFromRoot(bwbs []blocks.BlockWithROBlobs) map[[fieldparams.RootLength]byte][]int {
	result := make(map[[fieldparams.RootLength]byte][]int, len(bwbs))
	for i := 0; i < len(bwbs); i++ {
		root := bwbs[i].Block.Root()
		result[root] = append(result[root], i)
	}

	return result
}

// blockFromRoot returns the block indexed by root.
func blockFromRoot(bwb []blocks.BlockWithROBlobs) map[[fieldparams.RootLength]byte]blocks.ROBlock {
	result := make(map[[fieldparams.RootLength]byte]blocks.ROBlock, len(bwb))
	for i := 0; i < len(bwb); i++ {
		root := bwb[i].Block.Root()
		result[root] = bwb[i].Block
	}

	return result
}

// fetchDataColumnsFromPeers looks at the blocks in `bwb` and retrieves all
// data columns for with the block has blob commitments, and for which our store is missing data columns
// we should custody.
// This function mutates `bwb` by adding the retrieved data columns.
// Prerequisite: bwb is sorted by slot.
func (f *blocksFetcher) fetchDataColumnsFromPeers(
	ctx context.Context,
	bwbs []blocks.BlockWithROBlobs,
	peers []peer.ID,
) error {
	// Time to wait if no peers are available.
	const (
		delay         = 5 * time.Second // Time to wait before retrying to fetch data columns.
		maxIdentifier = 1_000           // Max identifier for the request.
	)

	// Generate random identifier.
	identifier := f.rand.Intn(maxIdentifier)
	log := log.WithField("reqIdentifier", identifier)

	// Compute the columns we should custody.
	localCustodyColumns, err := f.custodyColumns()
	if err != nil {
		return errors.Wrap(err, "custody columns")
	}

	// Compute the current slot.
	currentSlot := f.clock.CurrentSlot()

	// Compute the minimum slot for which we should serve data columns.
	minimumSlot, err := prysmsync.DataColumnsRPCMinValidSlot(currentSlot)
	if err != nil {
		return errors.Wrap(err, "data columns RPC min valid slot")
	}

	// Compute all missing data columns indexed by root.
	missingColumnsByRoot, err := f.missingColumnsFromRoot(localCustodyColumns, minimumSlot, bwbs)
	if err != nil {
		return errors.Wrap(err, "missing columns from root")
	}

	// Return early if there are no missing data columns.
	if len(missingColumnsByRoot) == 0 {
		return nil
	}

	// Log the start of the process.
	start := time.Now()
	log.Debug("Fetch data columns from peers - start")

	for len(missingColumnsByRoot) > 0 {
		// Compute the optimal slices of `bwb` to minimize the number of by range returned columns.
		bwbsSlices, err := buildBwbSlices(bwbs, missingColumnsByRoot)
		if err != nil {
			return errors.Wrap(err, "build bwb slices")
		}

	outerLoop:
		for _, bwbsSlice := range bwbsSlices {
			lastSlot := bwbs[bwbsSlice.end].Block.Block().Slot()
			dataColumnsSlice := sortedSliceFromMap(bwbsSlice.dataColumns)
			dataColumnCount := uint64(len(dataColumnsSlice))

			// Filter out slices that are already complete.
			if dataColumnCount == 0 {
				continue
			}

			// If no peer is specified, get all connected peers.
			peersToFilter := peers
			if peersToFilter == nil {
				peersToFilter = f.p2p.Peers().Connected()
			}

			// Compute the block count of the request.
			startSlot := bwbs[bwbsSlice.start].Block.Block().Slot()
			endSlot := bwbs[bwbsSlice.end].Block.Block().Slot()
			blockCount := uint64(endSlot - startSlot + 1)

			filteredPeers, err := f.waitForPeersForDataColumns(ctx, peersToFilter, lastSlot, bwbsSlice.dataColumns, blockCount)
			if err != nil {
				return errors.Wrap(err, "wait for peers for data columns")
			}

			// Build the request.
			request := &p2ppb.DataColumnSidecarsByRangeRequest{
				StartSlot: startSlot,
				Count:     blockCount,
				Columns:   dataColumnsSlice,
			}

			// Get `bwbs` indices indexed by root.
			indicesByRoot := indicesFromRoot(bwbs)

			// Get blocks indexed by root.
			blocksByRoot := blockFromRoot(bwbs)

			// Prepare nice log fields.
			var columnsLog interface{} = "all"
			numberOfColuns := params.BeaconConfig().NumberOfColumns
			if dataColumnCount < numberOfColuns {
				columnsLog = dataColumnsSlice
			}

			log := log.WithFields(logrus.Fields{
				"start":   request.StartSlot,
				"count":   request.Count,
				"columns": columnsLog,
			})

			// Retrieve the missing data columns from the peers.
			for _, peer := range filteredPeers {
				success := f.fetchDataColumnFromPeer(ctx, bwbs, missingColumnsByRoot, blocksByRoot, indicesByRoot, peer, request)

				// If we have successfully retrieved some data columns, continue to the next slice.
				if success {
					continue outerLoop
				}
			}

			log.WithField("peers", filteredPeers).Warning("Fetch data columns from peers - no peers among this list returned any valid data columns")
		}

		if len(missingColumnsByRoot) > 0 {
			log.Debug("Fetch data columns from peers - continue")
		}
	}

	// Sort data columns by index.
	sortBwbsByColumnIndex(bwbs)

	log.WithField("duration", time.Since(start)).Debug("Fetch data columns from peers - success")
	return nil
}

// sortBwbsByColumnIndex sorts `bwbs` by column index.
func sortBwbsByColumnIndex(bwbs []blocks.BlockWithROBlobs) {
	for _, bwb := range bwbs {
		sort.Slice(bwb.Columns, func(i, j int) bool {
			return bwb.Columns[i].ColumnIndex < bwb.Columns[j].ColumnIndex
		})
	}
}

// waitForPeersForDataColumns filters `peers` to only include peers that are:
// - synced up to `lastSlot`,
// - custody all columns in `dataColumns`, and
// - have bandwidth to serve `blockCount` blocks.
// It waits until at least one peer is available.
func (f *blocksFetcher) waitForPeersForDataColumns(
	ctx context.Context,
	peers []peer.ID,
	lastSlot primitives.Slot,
	dataColumns map[uint64]bool,
	blockCount uint64,
) ([]peer.ID, error) {
	// Time to wait before retrying to find new peers.
	const delay = 5 * time.Second

	// Filter peers that custody all columns we need and that are synced to the epoch.
	filteredPeers, descriptions, err := f.peersWithSlotAndDataColumns(ctx, peers, lastSlot, dataColumns, blockCount)
	if err != nil {
		return nil, errors.Wrap(err, "peers with slot and data columns")
	}

	// Compute data columns count
	dataColumnCount := uint64(len(dataColumns))

	// Sort columns.
	columnsSlice := sortedSliceFromMap(dataColumns)

	// Build a nice log field.
	var columnsLog interface{} = "all"
	numberOfColuns := params.BeaconConfig().NumberOfColumns
	if dataColumnCount < numberOfColuns {
		columnsLog = columnsSlice
	}

	// Wait if no suitable peers are available.
	for len(filteredPeers) == 0 {
		log.
			WithFields(logrus.Fields{
				"peers":        peers,
				"waitDuration": delay,
				"targetSlot":   lastSlot,
				"columns":      columnsLog,
			}).
			Warning("Fetch data columns from peers - no peers available to retrieve missing data columns, retrying later")

		for _, description := range descriptions {
			log.Debug(description)
		}

		time.Sleep(delay)

		filteredPeers, descriptions, err = f.peersWithSlotAndDataColumns(ctx, peers, lastSlot, dataColumns, blockCount)
		if err != nil {
			return nil, errors.Wrap(err, "peers with slot and data columns")
		}
	}

	return filteredPeers, nil
}

// processDataColumn mutates `bwbs` argument by adding the data column,
// and mutates `missingColumnsByRoot` by removing the data column if the
// data column passes all the check.
func processDataColumn(
	bwbs []blocks.BlockWithROBlobs,
	missingColumnsByRoot map[[fieldparams.RootLength]byte]map[uint64]bool,
	columnVerifier verification.NewColumnVerifier,
	blocksByRoot map[[fieldparams.RootLength]byte]blocks.ROBlock,
	indicesByRoot map[[fieldparams.RootLength]byte][]int,
	dataColumn blocks.RODataColumn,
) bool {
	// Extract the block root from the data column.
	blockRoot := dataColumn.BlockRoot()

	// Find the position of the block in `bwbs` that corresponds to this block root.
	indices, ok := indicesByRoot[blockRoot]
	if !ok {
		// The peer returned a data column that we did not expect.
		// This is among others possible when the peer is not on the same fork.
		return false
	}

	// Extract the block from the block root.
	block, ok := blocksByRoot[blockRoot]
	if !ok {
		// This should never happen.
		log.WithField("blockRoot", fmt.Sprintf("%#x", blockRoot)).Error("Fetch data columns from peers - block not found")
		return false
	}

	// Verify the data column.
	if err := verify.ColumnAlignsWithBlock(dataColumn, block, columnVerifier); err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"root":   fmt.Sprintf("%#x", blockRoot),
			"slot":   block.Block().Slot(),
			"column": dataColumn.ColumnIndex,
		}).Warning("Fetch data columns from peers - fetched data column does not align with block")

		// TODO: Should we downscore the peer for that?
		return false
	}

	// Populate the corresponding items in `bwbs`.
	for _, index := range indices {
		bwbs[index].Columns = append(bwbs[index].Columns, dataColumn)
	}

	// Remove the column from the missing columns.
	delete(missingColumnsByRoot[blockRoot], dataColumn.ColumnIndex)
	if len(missingColumnsByRoot[blockRoot]) == 0 {
		delete(missingColumnsByRoot, blockRoot)
	}

	return true
}

// fetchDataColumnsFromPeer sends `request` to `peer`, then mutates:
// - `bwbs` by adding the fetched data columns,
// - `missingColumnsByRoot` by removing the fetched data columns.
func (f *blocksFetcher) fetchDataColumnFromPeer(
	ctx context.Context,
	bwbs []blocks.BlockWithROBlobs,
	missingColumnsByRoot map[[fieldparams.RootLength]byte]map[uint64]bool,
	blocksByRoot map[[fieldparams.RootLength]byte]blocks.ROBlock,
	indicesByRoot map[[fieldparams.RootLength]byte][]int,
	peer peer.ID,
	request *p2ppb.DataColumnSidecarsByRangeRequest,
) bool {
	// Define useful log field.
	log := log.WithField("peer", peer)

	// Wait for peer bandwidth if needed.
	if err := func() error {
		l := f.peerLock(peer)
		l.Lock()
		defer l.Unlock()

		remaining := uint64(f.rateLimiter.Remaining(peer.String()))

		// We're intentionally abusing the block rate limit here, treating data column requests as if they were block requests.
		// Since column requests take more bandwidth than blocks, we should improve how we account for the different kinds
		// of requests, more in proportion to the cost of serving them.
		if remaining < request.Count {
			log.Debug("Fetch data columns from peers - wait for bandwidth")
			if err := f.waitForBandwidth(peer, request.Count); err != nil {
				return errors.Wrap(err, "wait for bandwidth")
			}
		}

		f.rateLimiter.Add(peer.String(), int64(request.Count))

		return nil
	}(); err != nil {
		log.WithError(err).Warning("Fetch data columns from peers - could not wait for bandwidth")
		return false
	}

	// Send the request to the peer.
	requestStart := time.Now()
	roDataColumns, err := prysmsync.SendDataColumnsByRangeRequest(ctx, f.clock, f.p2p, peer, f.ctxMap, request)
	if err != nil {
		log.WithError(err).Warning("Fetch data columns from peers - could not send data columns by range request")
		return false
	}

	requestDuration := time.Since(requestStart)

	if len(roDataColumns) == 0 {
		log.Debug("Fetch data columns from peers - peer did not return any data columns")
		return false
	}

	globalSuccess := false

	for _, dataColumn := range roDataColumns {
		success := processDataColumn(bwbs, missingColumnsByRoot, f.cv, blocksByRoot, indicesByRoot, dataColumn)
		if success {
			globalSuccess = true
		}
	}

	if !globalSuccess {
		log.Debug("Fetch data columns from peers - peer did not return any valid data columns")
		return false
	}

	totalDuration := time.Since(requestStart)
	log.WithFields(logrus.Fields{
		"reqDuration":   requestDuration,
		"totalDuration": totalDuration,
	}).Debug("Fetch data columns from peers - got some columns")

	return true
}

// requestBlocks is a wrapper for handling BeaconBlocksByRangeRequest requests/streams.
func (f *blocksFetcher) requestBlocks(
	ctx context.Context,
	req *p2ppb.BeaconBlocksByRangeRequest,
	pid peer.ID,
) ([]interfaces.ReadOnlySignedBeaconBlock, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	l := f.peerLock(pid)
	l.Lock()
	log.WithFields(logrus.Fields{
		"peer":     pid,
		"start":    req.StartSlot,
		"count":    req.Count,
		"step":     req.Step,
		"capacity": f.rateLimiter.Remaining(pid.String()),
		"score":    f.p2p.Peers().Scorers().BlockProviderScorer().FormatScorePretty(pid),
	}).Debug("Requesting blocks")
	if f.rateLimiter.Remaining(pid.String()) < int64(req.Count) {
		if err := f.waitForBandwidth(pid, req.Count); err != nil {
			l.Unlock()
			return nil, err
		}
	}
	f.rateLimiter.Add(pid.String(), int64(req.Count))
	l.Unlock()
	return prysmsync.SendBeaconBlocksByRangeRequest(ctx, f.chain, f.p2p, pid, req, nil)
}

func (f *blocksFetcher) requestBlobs(ctx context.Context, req *p2ppb.BlobSidecarsByRangeRequest, pid peer.ID) ([]blocks.ROBlob, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	l := f.peerLock(pid)
	l.Lock()
	log.WithFields(logrus.Fields{
		"peer":     pid,
		"start":    req.StartSlot,
		"count":    req.Count,
		"capacity": f.rateLimiter.Remaining(pid.String()),
		"score":    f.p2p.Peers().Scorers().BlockProviderScorer().FormatScorePretty(pid),
	}).Debug("Requesting blobs")
	// We're intentionally abusing the block rate limit here, treating blob requests as if they were block requests.
	// Since blob requests take more bandwidth than blocks, we should improve how we account for the different kinds
	// of requests, more in proportion to the cost of serving them.
	if f.rateLimiter.Remaining(pid.String()) < int64(req.Count) {
		if err := f.waitForBandwidth(pid, req.Count); err != nil {
			l.Unlock()
			return nil, err
		}
	}
	f.rateLimiter.Add(pid.String(), int64(req.Count))
	l.Unlock()

	return prysmsync.SendBlobsByRangeRequest(ctx, f.clock, f.p2p, pid, f.ctxMap, req)
}

// requestBlocksByRoot is a wrapper for handling BeaconBlockByRootsReq requests/streams.
func (f *blocksFetcher) requestBlocksByRoot(
	ctx context.Context,
	req *p2pTypes.BeaconBlockByRootsReq,
	pid peer.ID,
) ([]interfaces.ReadOnlySignedBeaconBlock, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	l := f.peerLock(pid)
	l.Lock()
	log.WithFields(logrus.Fields{
		"peer":     pid,
		"numRoots": len(*req),
		"capacity": f.rateLimiter.Remaining(pid.String()),
		"score":    f.p2p.Peers().Scorers().BlockProviderScorer().FormatScorePretty(pid),
	}).Debug("Requesting blocks (by roots)")
	if f.rateLimiter.Remaining(pid.String()) < int64(len(*req)) {
		if err := f.waitForBandwidth(pid, uint64(len(*req))); err != nil {
			l.Unlock()
			return nil, err
		}
	}
	f.rateLimiter.Add(pid.String(), int64(len(*req)))
	l.Unlock()

	return prysmsync.SendBeaconBlocksByRootRequest(ctx, f.chain, f.p2p, pid, req, nil)
}

// waitForBandwidth blocks up until peer's bandwidth is restored.
func (f *blocksFetcher) waitForBandwidth(pid peer.ID, count uint64) error {
	log.WithField("peer", pid).Debug("Slowing down for rate limit")
	rem := f.rateLimiter.Remaining(pid.String())
	if uint64(rem) >= count {
		// Exit early if we have sufficient capacity
		return nil
	}
	intCount, err := mathPrysm.Int(count)
	if err != nil {
		return err
	}
	toWait := timeToWait(int64(intCount), rem, f.rateLimiter.Capacity(), f.rateLimiter.TillEmpty(pid.String()))
	timer := time.NewTimer(toWait)
	defer timer.Stop()
	select {
	case <-f.ctx.Done():
		return errFetcherCtxIsDone
	case <-timer.C:
		// Peer has gathered enough capacity to be polled again.
	}
	return nil
}

func (f *blocksFetcher) hasSufficientBandwidth(peers []peer.ID, count uint64) []peer.ID {
	var filteredPeers []peer.ID

	for _, p := range peers {
		if uint64(f.rateLimiter.Remaining(p.String())) < count {
			continue
		}
		copiedP := p
		filteredPeers = append(filteredPeers, copiedP)
	}
	return filteredPeers
}

// Determine how long it will take for us to have the required number of blocks allowed by our rate limiter.
// We do this by calculating the duration till the rate limiter can request these blocks without exceeding
// the provided bandwidth limits per peer.
func timeToWait(wanted, rem, capacity int64, timeTillEmpty time.Duration) time.Duration {
	// Defensive check if we have more than enough blocks
	// to request from the peer.
	if rem >= wanted {
		return 0
	}
	// Handle edge case where capacity is equal to the remaining amount
	// of blocks. This also handles the impossible case in where remaining blocks
	// exceed the limiter's capacity.
	if capacity <= rem {
		return 0
	}
	blocksNeeded := wanted - rem
	currentNumBlks := capacity - rem
	expectedTime := int64(timeTillEmpty) * blocksNeeded / currentNumBlks
	return time.Duration(expectedTime)
}

// deduplicates the provided peer list.
func dedupPeers(peers []peer.ID) []peer.ID {
	newPeerList := make([]peer.ID, 0, len(peers))
	peerExists := make(map[peer.ID]bool)

	for i := range peers {
		if peerExists[peers[i]] {
			continue
		}
		newPeerList = append(newPeerList, peers[i])
		peerExists[peers[i]] = true
	}
	return newPeerList
}
