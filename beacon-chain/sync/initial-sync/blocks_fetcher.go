package initialsync

import (
	"context"
	"fmt"
	"math"
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
			response.err = fmt.Errorf("%w, slot: %d, highest finalized slot: %d",
				errSlotIsTooHigh, start, highestFinalizedSlot)
			return response
		}
	}

	response.bwb, response.pid, response.err = f.fetchBlocksFromPeer(ctx, start, count, peers)

	if response.err != nil {
		return response
	}

	if coreTime.PeerDASIsActive(start) {
		response.err = f.fetchDataColumnsFromPeers(ctx, response.bwb, peers)
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

func (r *blobRange) RequestDataColumns() *p2ppb.DataColumnSidecarsByRangeRequest {
	if r == nil {
		return nil
	}
	return &p2ppb.DataColumnSidecarsByRangeRequest{
		StartSlot: r.low,
		Count:     uint64(r.high.SubSlot(r.low)) + 1,
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

// blocksWithMissingDataColumnsBoundaries finds the first and last block in `bwb` that:
// - are in the blob retention period,
// - contain at least one blob, and
// - have at least one missing data column.
func (f *blocksFetcher) blocksWithMissingDataColumnsBoundaries(
	bwb []blocks.BlockWithROBlobs,
	currentSlot primitives.Slot,
	localCustodyColumns map[uint64]bool,
) (bool, int, int, error) {
	// Get, regarding the current slot, the minimum slot for which we should serve data columns.
	columnWindowStart, err := prysmsync.DataColumnsRPCMinValidSlot(currentSlot)
	if err != nil {
		return false, 0, 0, errors.Wrap(err, "data columns RPC min valid slot")
	}

	// Find the first block with a slot higher than or equal to columnWindowStart,
	firstWindowIndex := -1
	for i := range bwb {
		if bwb[i].Block.Block().Slot() >= columnWindowStart {
			firstWindowIndex = i
			break
		}
	}

	if firstWindowIndex == -1 {
		// There is no block with slot greater than or equal to columnWindowStart.
		return false, 0, 0, nil
	}

	// Find the first block which contains blob commitments and for which some data columns are missing.
	firstIndex := -1
	for i := firstWindowIndex; i < len(bwb); i++ {
		// Is there any blob commitment in this block?
		commits, err := bwb[i].Block.Block().Body().BlobKzgCommitments()
		if err != nil {
			return false, 0, 0, errors.Wrap(err, "blob KZG commitments")
		}

		if len(commits) == 0 {
			continue
		}

		// Is there at least one column we should custody that is not in our store?
		root := bwb[i].Block.Root()
		allColumnsAreAvailable := f.bs.Summary(root).AllDataColumnsAvailable(localCustodyColumns)

		if !allColumnsAreAvailable {
			firstIndex = i
			break
		}
	}

	if firstIndex == -1 {
		// There is no block with at least one missing data column.
		return false, 0, 0, nil
	}

	// Find the last block which contains blob commitments and for which some data columns are missing.
	lastIndex := len(bwb) - 1
	for i := lastIndex; i >= firstIndex; i-- {
		// Is there any blob commitment in this block?
		commits, err := bwb[i].Block.Block().Body().BlobKzgCommitments()
		if err != nil {
			return false, 0, 0, errors.Wrap(err, "blob KZG commitments")
		}

		if len(commits) == 0 {
			continue
		}

		// Is there at least one column we should custody that is not in our store?
		root := bwb[i].Block.Root()
		allColumnsAreAvailable := f.bs.Summary(root).AllDataColumnsAvailable(localCustodyColumns)

		if !allColumnsAreAvailable {
			lastIndex = i
			break
		}
	}

	return true, firstIndex, lastIndex, nil
}

// custodyAllNeededColumns filter `inputPeers` that custody all columns in `columns`.
func (f *blocksFetcher) custodyAllNeededColumns(inputPeers []peer.ID, columns map[uint64]bool) ([]peer.ID, error) {
	outputPeers := make([]peer.ID, 0, len(inputPeers))

loop:
	for _, peer := range inputPeers {
		// Get the node ID from the peer ID.
		nodeID, err := p2p.ConvertPeerIDToNodeID(peer)
		if err != nil {
			return nil, errors.Wrap(err, "convert peer ID to node ID")
		}

		// Get the custody columns count from the peer.
		custodyCount := f.p2p.DataColumnsCustodyCountFromRemotePeer(peer)

		// Get the custody columns from the peer.
		remoteCustodyColumns, err := peerdas.CustodyColumns(nodeID, custodyCount)
		if err != nil {
			return nil, errors.Wrap(err, "custody columns")
		}

		for column := range columns {
			if !remoteCustodyColumns[column] {
				continue loop
			}
		}

		outputPeers = append(outputPeers, peer)
	}

	return outputPeers, nil
}

// filterPeersForDataColumns filters peers able to serve us `dataColumns`.
func (f *blocksFetcher) filterPeersForDataColumns(
	ctx context.Context,
	blocksCount uint64,
	dataColumns map[uint64]bool,
	peers []peer.ID,
) ([]peer.ID, error) {
	// TODO: Uncomment when we are not in devnet any more.
	// TODO: Find a way to have this uncommented without being in devnet.
	// // Filter peers based on the percentage of peers to be used in a request.
	// peers = f.filterPeers(ctx, peers, peersPercentagePerRequest)

	// // Filter peers on bandwidth.
	// peers = f.hasSufficientBandwidth(peers, blocksCount)

	// Select peers which custody ALL wanted columns.
	// Basically, it is very unlikely that a non-supernode peer will have custody of all columns.
	// TODO: Modify to retrieve data columns from all possible peers.
	// TODO: If a peer does respond some of the request columns, do not re-request responded columns.
	peers, err := f.custodyAllNeededColumns(peers, dataColumns)
	if err != nil {
		return nil, errors.Wrap(err, "custody all needed columns")
	}

	// Randomize the order of the peers.
	randGen := rand.NewGenerator()
	randGen.Shuffle(len(peers), func(i, j int) {
		peers[i], peers[j] = peers[j], peers[i]
	})

	return peers, nil
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

// missingColumnsFromRoot returns the missing columns indexed by root.
func (f *blocksFetcher) missingColumnsFromRoot(
	custodyColumns map[uint64]bool,
	bwb []blocks.BlockWithROBlobs,
) (map[[fieldparams.RootLength]byte]map[uint64]bool, error) {
	result := make(map[[fieldparams.RootLength]byte]map[uint64]bool)
	for i := 0; i < len(bwb); i++ {
		block := bwb[i].Block

		// Retrieve the blob KZG commitments.
		commitments, err := block.Block().Body().BlobKzgCommitments()
		if err != nil {
			return nil, errors.Wrap(err, "blob KZG commitments")
		}

		// Skip if there are no commitments.
		if len(commitments) == 0 {
			continue
		}

		// Retrieve the root.
		root := block.Root()

		for column := range custodyColumns {
			// If there is at least one commitment for this block and if a column we should custody
			// is not in our store, then we should retrieve it.
			if !f.bs.Summary(root).HasDataColumnIndex(column) {
				if _, ok := result[root]; !ok {
					result[root] = make(map[uint64]bool)
				}
				result[root][column] = true
			}
		}
	}

	return result, nil
}

// indicesFromRoot returns the indices indexed by root.
func indicesFromRoot(bwb []blocks.BlockWithROBlobs) map[[fieldparams.RootLength]byte][]int {
	result := make(map[[fieldparams.RootLength]byte][]int, len(bwb))
	for i := 0; i < len(bwb); i++ {
		root := bwb[i].Block.Root()
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

// minInt returns the minimum integer in a slice.
func minInt(slice []int) int {
	min := math.MaxInt
	for _, item := range slice {
		if item < min {
			min = item
		}
	}

	return min
}

// maxInt returns the maximum integer in a slice.
func maxInt(slice []int) int {
	max := math.MinInt
	for _, item := range slice {
		if item > max {
			max = item
		}
	}

	return max
}

// requestDataColumnsFromPeers send `request` to each peer in `peers` until a peer returns at least one data column.
func (f *blocksFetcher) requestDataColumnsFromPeers(
	ctx context.Context,
	request *p2ppb.DataColumnSidecarsByRangeRequest,
	peers []peer.ID,
) ([]blocks.RODataColumn, peer.ID, error) {
	for _, peer := range peers {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}

		err := func() error {
			l := f.peerLock(peer)
			l.Lock()
			defer l.Unlock()

			log.WithFields(logrus.Fields{
				"peer":     peer,
				"start":    request.StartSlot,
				"count":    request.Count,
				"capacity": f.rateLimiter.Remaining(peer.String()),
				"score":    f.p2p.Peers().Scorers().BlockProviderScorer().FormatScorePretty(peer),
			}).Debug("Requesting data columns")

			// We're intentionally abusing the block rate limit here, treating data column requests as if they were block requests.
			// Since column requests take more bandwidth than blocks, we should improve how we account for the different kinds
			// of requests, more in proportion to the cost of serving them.
			if f.rateLimiter.Remaining(peer.String()) < int64(request.Count) {
				if err := f.waitForBandwidth(peer, request.Count); err != nil {
					return errors.Wrap(err, "wait for bandwidth")
				}
			}

			f.rateLimiter.Add(peer.String(), int64(request.Count))

			return nil
		}()

		if err != nil {
			return nil, "", err
		}

		roDataColumns, err := prysmsync.SendDataColumnsByRangeRequest(ctx, f.clock, f.p2p, peer, f.ctxMap, request)
		if err != nil {
			log.WithField("peer", peer).WithError(err).Warning("Could not request data columns by range from peer")
			continue
		}

		// If the peer did not return any data columns, go to the next peer.
		if len(roDataColumns) == 0 {
			continue
		}

		// We have received at least one data columns from the peer.
		return roDataColumns, peer, nil
	}

	// No peer returned any data columns.
	return nil, "", nil
}

// firstLastIndices returns the first and last indices where we have missing columns.
func firstLastIndices(
	missingColumnsFromRoot map[[fieldparams.RootLength]byte]map[uint64]bool,
	indicesFromRoot map[[fieldparams.RootLength]byte][]int,
) (int, int) {
	firstIndex, lastIndex := math.MaxInt, -1
	for root := range missingColumnsFromRoot {
		indices := indicesFromRoot[root]

		index := minInt(indices)
		if index < firstIndex {
			firstIndex = index
		}

		index = maxInt(indices)
		if index > lastIndex {
			lastIndex = index
		}
	}

	return firstIndex, lastIndex
}

// processRetrievedDataColumns processes the retrieved data columns.
// This function:
// - Mutate `bwb` by adding the retrieved data columns.
// - Mutate `missingColumnsFromRoot` by removing the columns that have been retrieved.
func processRetrievedDataColumns(
	roDataColumns []blocks.RODataColumn,
	blockFromRoot map[[fieldparams.RootLength]byte]blocks.ROBlock,
	indicesFromRoot map[[fieldparams.RootLength]byte][]int,
	missingColumnsFromRoot map[[fieldparams.RootLength]byte]map[uint64]bool,
	bwb []blocks.BlockWithROBlobs,
	colVerifier verification.NewColumnVerifier,
) {
	retrievedColumnsFromRoot := make(map[[fieldparams.RootLength]byte]map[uint64]bool)

	// Verify and populate columns
	for i := range roDataColumns {
		dataColumn := roDataColumns[i]

		root := dataColumn.BlockRoot()
		columnIndex := dataColumn.ColumnIndex

		missingColumns, ok := missingColumnsFromRoot[root]
		if !ok {
			continue
		}

		if !missingColumns[columnIndex] {
			continue
		}

		// Verify the data column.
		if err := verify.ColumnAlignsWithBlock(dataColumn, blockFromRoot[root], colVerifier); err != nil {
			// TODO: Should we downscore the peer for that?
			continue
		}

		// Populate the block with the data column.
		for _, index := range indicesFromRoot[root] {
			if bwb[index].Columns == nil {
				bwb[index].Columns = make([]blocks.RODataColumn, 0)
			}

			bwb[index].Columns = append(bwb[index].Columns, dataColumn)
		}

		// Populate the retrieved columns.
		if _, ok := retrievedColumnsFromRoot[root]; !ok {
			retrievedColumnsFromRoot[root] = make(map[uint64]bool)
		}

		retrievedColumnsFromRoot[root][columnIndex] = true

		// Remove the column from the missing columns.
		delete(missingColumnsFromRoot[root], columnIndex)
		if len(missingColumnsFromRoot[root]) == 0 {
			delete(missingColumnsFromRoot, root)
		}
	}
}

// retrieveMissingDataColumnsFromPeers retrieves the missing data columns from the peers.
// This function:
// - Mutate `bwb` by adding the retrieved data columns.
// - Mutate `missingColumnsFromRoot` by removing the columns that have been retrieved.
// This function returns when all the missing data columns have been retrieved,
// or when the context is canceled.
func (f *blocksFetcher) retrieveMissingDataColumnsFromPeers(
	ctx context.Context,
	bwb []blocks.BlockWithROBlobs,
	missingColumnsFromRoot map[[fieldparams.RootLength]byte]map[uint64]bool,
	indicesFromRoot map[[fieldparams.RootLength]byte][]int,
	peers []peer.ID,
) error {
	for len(missingColumnsFromRoot) > 0 {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Get the first and last indices where we have missing columns.
		firstIndex, lastIndex := firstLastIndices(missingColumnsFromRoot, indicesFromRoot)

		// Get the first and the last slot.
		firstSlot := bwb[firstIndex].Block.Block().Slot()
		lastSlot := bwb[lastIndex].Block.Block().Slot()

		// Get the number of blocks to retrieve.
		blocksCount := uint64(lastSlot - firstSlot + 1)

		// Get the missing data columns.
		missingDataColumns := make(map[uint64]bool)
		for _, columns := range missingColumnsFromRoot {
			for column := range columns {
				missingDataColumns[column] = true
			}
		}

		// Filter peers.
		filteredPeers, err := f.filterPeersForDataColumns(ctx, blocksCount, missingDataColumns, peers)
		if err != nil {
			return errors.Wrap(err, "filter peers for data columns")
		}

		if len(filteredPeers) == 0 {
			log.
				WithFields(logrus.Fields{
					"nonFilteredPeersCount": len(peers),
					"filteredPeersCount":    len(filteredPeers),
				}).
				Debug("No peers available to retrieve missing data columns, retrying in 5 seconds")

			time.Sleep(5 * time.Second)
			continue
		}

		// Get the first slot for which we should retrieve data columns.
		startSlot := bwb[firstIndex].Block.Block().Slot()

		// Build the request.
		request := &p2ppb.DataColumnSidecarsByRangeRequest{
			StartSlot: startSlot,
			Count:     blocksCount,
			Columns:   sortedSliceFromMap(missingDataColumns),
		}

		// Get all the blocks and data columns we should retrieve.
		blockFromRoot := blockFromRoot(bwb[firstIndex : lastIndex+1])

		// Iterate requests over all peers, and exits as soon as at least one data column is retrieved.
		roDataColumns, peer, err := f.requestDataColumnsFromPeers(ctx, request, filteredPeers)
		if err != nil {
			return errors.Wrap(err, "request data columns from peers")
		}

		if len(roDataColumns) == 0 {
			log.Debug("No data columns returned from any peer, retrying in 5 seconds")
			time.Sleep(5 * time.Second)
			continue
		}

		// Process the retrieved data columns.
		processRetrievedDataColumns(roDataColumns, blockFromRoot, indicesFromRoot, missingColumnsFromRoot, bwb, f.cv)

		if len(missingColumnsFromRoot) > 0 {
			numberOfColumns := params.BeaconConfig().NumberOfColumns

			for root, missingColumns := range missingColumnsFromRoot {
				missingColumnsCount := uint64(len(missingColumns))
				var missingColumnsLog interface{} = "all"

				if missingColumnsCount < numberOfColumns {
					missingColumnsLog = sortedSliceFromMap(missingColumns)
				}

				slot := blockFromRoot[root].Block().Slot()
				log.WithFields(logrus.Fields{
					"peer":           peer,
					"root":           fmt.Sprintf("%#x", root),
					"slot":           slot,
					"missingColumns": missingColumnsLog,
				}).Debug("Peer did not correctly return data columns")
			}
		}
	}

	return nil
}

// fetchDataColumnsFromPeers looks at the blocks in `bwb` and retrieves all
// data columns for with the block has blob commitments, and for which our store is missing data columns
// we should custody.
// This function mutates `bwb` by adding the retrieved data columns.
// Preqrequisite: bwb is sorted by slot.
func (f *blocksFetcher) fetchDataColumnsFromPeers(
	ctx context.Context,
	bwb []blocks.BlockWithROBlobs,
	peers []peer.ID,
) error {
	ctx, span := trace.StartSpan(ctx, "initialsync.fetchColumnsFromPeer")
	defer span.End()

	// Get the current slot.
	currentSlot := f.clock.CurrentSlot()

	// If there is no data columns before deneb. Early return.
	if slots.ToEpoch(currentSlot) < params.BeaconConfig().DenebForkEpoch {
		return nil
	}

	// Get the columns we custody.
	localCustodyColumns, err := f.custodyColumns()
	if err != nil {
		return errors.Wrap(err, "custody columns")
	}

	// Find the first and last block in `bwb` that:
	// - are in the blob retention period,
	// - contain at least one blob, and
	// - have at least one missing data column.
	someColumnsAreMissing, firstIndex, lastIndex, err := f.blocksWithMissingDataColumnsBoundaries(bwb, currentSlot, localCustodyColumns)
	if err != nil {
		return errors.Wrap(err, "blocks with missing data columns boundaries")
	}

	// If there is no block with missing data columns, early return.
	if !someColumnsAreMissing {
		return nil
	}

	// Get all missing columns indexed by root.
	missingColumnsFromRoot, err := f.missingColumnsFromRoot(localCustodyColumns, bwb[firstIndex:lastIndex+1])
	if err != nil {
		return errors.Wrap(err, "missing columns from root")
	}

	// Get all indices indexed by root.
	indicesFromRoot := indicesFromRoot(bwb)

	// Retrieve the missing data columns from the peers.
	if err := f.retrieveMissingDataColumnsFromPeers(ctx, bwb, missingColumnsFromRoot, indicesFromRoot, peers); err != nil {
		return errors.Wrap(err, "retrieve missing data columns from peers")
	}

	log.Debug("Successfully retrieved all data columns")

	return nil
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
