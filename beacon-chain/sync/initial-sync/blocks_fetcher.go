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
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db/filesystem"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	p2pTypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/types"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/startup"
	prysmsync "github.com/prysmaticlabs/prysm/v5/beacon-chain/sync"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/sync/verify"
	"github.com/prysmaticlabs/prysm/v5/cmd/beacon-chain/flags"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	blocks2 "github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	leakybucket "github.com/prysmaticlabs/prysm/v5/container/leaky-bucket"
	"github.com/prysmaticlabs/prysm/v5/crypto/rand"
	"github.com/prysmaticlabs/prysm/v5/math"
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
	bwb   []blocks2.BlockWithROBlobs
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
	castedMaxLimit, err := math.Int(maxLimit)
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
		bwb:   []blocks2.BlockWithROBlobs{},
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
	if response.err == nil {
		if features.Get().EnablePeerDAS {
			bwb, err := f.fetchColumnsFromPeer(ctx, response.bwb, response.pid, peers)
			if err != nil {
				response.err = err
			}
			response.bwb = bwb
		} else {
			bwb, err := f.fetchBlobsFromPeer(ctx, response.bwb, response.pid, peers)
			if err != nil {
				response.err = err
			}
			response.bwb = bwb
		}
	}
	return response
}

// fetchBlocksFromPeer fetches blocks from a single randomly selected peer.
func (f *blocksFetcher) fetchBlocksFromPeer(
	ctx context.Context,
	start primitives.Slot, count uint64,
	peers []peer.ID,
) ([]blocks2.BlockWithROBlobs, peer.ID, error) {
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

func sortedBlockWithVerifiedBlobSlice(blocks []interfaces.ReadOnlySignedBeaconBlock) ([]blocks2.BlockWithROBlobs, error) {
	rb := make([]blocks2.BlockWithROBlobs, len(blocks))
	for i, b := range blocks {
		ro, err := blocks2.NewROBlock(b)
		if err != nil {
			return nil, err
		}
		rb[i] = blocks2.BlockWithROBlobs{Block: ro}
	}
	sort.Sort(blocks2.BlockWithROBlobsSlice(rb))
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
func countCommitments(bwb []blocks2.BlockWithROBlobs, retentionStart primitives.Slot) commitmentCountList {
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

func verifyAndPopulateBlobs(bwb []blocks2.BlockWithROBlobs, blobs []blocks.ROBlob, req *p2ppb.BlobSidecarsByRangeRequest, bss filesystem.BlobStorageSummarizer) ([]blocks2.BlockWithROBlobs, error) {
	blobsByRoot := make(map[[32]byte][]blocks.ROBlob)
	for i := range blobs {
		if blobs[i].Slot() < req.StartSlot {
			continue
		}
		br := blobs[i].BlockRoot()
		blobsByRoot[br] = append(blobsByRoot[br], blobs[i])
	}
	for i := range bwb {
		bwi, err := populateBlock(bwb[i], blobsByRoot[bwb[i].Block.Root()], req, bss)
		if err != nil {
			if errors.Is(err, errDidntPopulate) {
				continue
			}
			return bwb, err
		}
		bwb[i] = bwi
	}
	return bwb, nil
}

func verifyAndPopulateColumns(bwb []blocks2.BlockWithROBlobs, columns []blocks.RODataColumn, req *p2ppb.DataColumnSidecarsByRangeRequest, bss filesystem.BlobStorageSummarizer) ([]blocks2.BlockWithROBlobs, error) {
	columnsByRoot := make(map[[32]byte][]blocks.RODataColumn)
	for i := range columns {
		if columns[i].Slot() < req.StartSlot {
			continue
		}
		br := columns[i].BlockRoot()
		columnsByRoot[br] = append(columnsByRoot[br], columns[i])
	}
	for i := range bwb {
		bwi, err := populateBlockWithColumns(bwb[i], columnsByRoot[bwb[i].Block.Root()], req, bss)
		if err != nil {
			if errors.Is(err, errDidntPopulate) {
				continue
			}
			return bwb, err
		}
		bwb[i] = bwi
	}
	return bwb, nil
}

var errDidntPopulate = errors.New("skipping population of block")

func populateBlock(bw blocks2.BlockWithROBlobs, blobs []blocks.ROBlob, req *p2ppb.BlobSidecarsByRangeRequest, bss filesystem.BlobStorageSummarizer) (blocks2.BlockWithROBlobs, error) {
	blk := bw.Block
	if blk.Version() < version.Deneb || blk.Block().Slot() < req.StartSlot {
		return bw, errDidntPopulate
	}
	commits, err := blk.Block().Body().BlobKzgCommitments()
	if err != nil {
		return bw, errDidntPopulate
	}
	if len(commits) == 0 {
		return bw, errDidntPopulate
	}
	// Drop blobs on the floor if we already have them.
	if bss != nil && bss.Summary(blk.Root()).AllAvailable(len(commits)) {
		return bw, errDidntPopulate
	}
	if len(commits) != len(blobs) {
		return bw, missingCommitError(blk.Root(), blk.Block().Slot(), commits)
	}
	for ci := range commits {
		if err := verify.BlobAlignsWithBlock(blobs[ci], blk); err != nil {
			return bw, err
		}
	}
	bw.Blobs = blobs
	return bw, nil
}

func populateBlockWithColumns(bw blocks2.BlockWithROBlobs, columns []blocks.RODataColumn, req *p2ppb.DataColumnSidecarsByRangeRequest, bss filesystem.BlobStorageSummarizer) (blocks2.BlockWithROBlobs, error) {
	blk := bw.Block
	if blk.Version() < version.Deneb || blk.Block().Slot() < req.StartSlot {
		return bw, errDidntPopulate
	}
	commits, err := blk.Block().Body().BlobKzgCommitments()
	if err != nil {
		return bw, errDidntPopulate
	}
	if len(commits) == 0 {
		return bw, errDidntPopulate
	}
	colsPersub := params.BeaconConfig().NumberOfColumns / params.BeaconConfig().DataColumnSidecarSubnetCount
	subnetCount := peerdas.CustodySubnetCount()
	if len(columns) != int(subnetCount*colsPersub) {
		return bw, errors.Errorf("unequal custodied columns provided, got %d instead of %d", len(columns), subnetCount)
	}
	for ci := range columns {
		if err := verify.ColumnAlignsWithBlock(columns[ci], blk); err != nil {
			return bw, err
		}
	}
	bw.Columns = columns
	return bw, nil
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
func (f *blocksFetcher) fetchBlobsFromPeer(ctx context.Context, bwb []blocks2.BlockWithROBlobs, pid peer.ID, peers []peer.ID) ([]blocks2.BlockWithROBlobs, error) {
	ctx, span := trace.StartSpan(ctx, "initialsync.fetchBlobsFromPeer")
	defer span.End()
	if slots.ToEpoch(f.clock.CurrentSlot()) < params.BeaconConfig().DenebForkEpoch {
		return bwb, nil
	}
	blobWindowStart, err := prysmsync.BlobRPCMinValidSlot(f.clock.CurrentSlot())
	if err != nil {
		return nil, err
	}
	// Construct request message based on observed interval of blocks in need of blobs.
	req := countCommitments(bwb, blobWindowStart).blobRange(f.bs).Request()
	if req == nil {
		return bwb, nil
	}
	peers = f.filterPeers(ctx, peers, peersPercentagePerRequest)
	// We dial the initial peer first to ensure that we get the desired set of blobs.
	wantedPeers := append([]peer.ID{pid}, peers...)
	bestPeers := f.hasSufficientBandwidth(wantedPeers, req.Count)
	// We append the best peers to the front so that higher capacity
	// peers are dialed first. If all of them fail, we fallback to the
	// initial peer we wanted to request blobs from.
	peers = append(bestPeers, pid)
	for i := 0; i < len(peers); i++ {
		p := peers[i]
		blobs, err := f.requestBlobs(ctx, req, p)
		if err != nil {
			log.WithField("peer", p).WithError(err).Debug("Could not request blobs by range from peer")
			continue
		}
		f.p2p.Peers().Scorers().BlockProviderScorer().Touch(p)
		robs, err := verifyAndPopulateBlobs(bwb, blobs, req, f.bs)
		if err != nil {
			log.WithField("peer", p).WithError(err).Debug("Invalid BeaconBlobsByRange response")
			continue
		}
		return robs, err
	}
	return nil, errNoPeersAvailable
}

// fetchColumnsFromPeer fetches blocks from a single randomly selected peer.
func (f *blocksFetcher) fetchColumnsFromPeer(ctx context.Context, bwb []blocks2.BlockWithROBlobs, pid peer.ID, peers []peer.ID) ([]blocks2.BlockWithROBlobs, error) {
	ctx, span := trace.StartSpan(ctx, "initialsync.fetchColumnsFromPeer")
	defer span.End()
	if slots.ToEpoch(f.clock.CurrentSlot()) < params.BeaconConfig().DenebForkEpoch {
		return bwb, nil
	}
	columnWindowStart, err := prysmsync.DataColumnsRPCMinValidSlot(f.clock.CurrentSlot())
	if err != nil {
		return nil, err
	}
	// Construct request message based on observed interval of blocks in need of columns.
	req := countCommitments(bwb, columnWindowStart).blobRange(f.bs).RequestDataColumns()
	if req == nil {
		return bwb, nil
	}
	// Construct request message based on required custodied columns.
	custodyCols, err := peerdas.CustodyColumns(f.p2p.NodeID(), peerdas.CustodySubnetCount())
	if err != nil {
		return nil, err
	}

	colIdxs := make([]uint64, 0, len(custodyCols))
	for c := range custodyCols {
		colIdxs = append(colIdxs, c)
	}
	req.Columns = colIdxs
	peers = f.filterPeers(ctx, peers, peersPercentagePerRequest)
	// We dial the initial peer first to ensure that we get the desired set of columns.
	wantedPeers := append([]peer.ID{pid}, peers...)
	bestPeers := f.hasSufficientBandwidth(wantedPeers, req.Count)
	// We append the best peers to the front so that higher capacity
	// peers are dialed first. If all of them fail, we fallback to the
	// initial peer we wanted to request blobs from.
	peers = append(bestPeers, pid)
	for i := 0; i < len(peers); i++ {
		p := peers[i]
		nid, err := p2p.ConvertPeerIDToNodeID(pid)
		if err != nil {
			return nil, err
		}

		remoteCustody, err := peerdas.CustodyColumns(nid, f.p2p.CustodyCountFromRemotePeer(p))
		if err != nil {
			return nil, err
		}
		if !remotePeerHasCustody(req.Columns, remoteCustody) {
			// TODO: For easier interop we do not skip for now
			log.Warnf("Remote peer %s does not have wanted columns", p.String())
		}
		columns, err := f.requestColumns(ctx, req, p)
		if err != nil {
			log.WithField("peer", p).WithError(err).Debug("Could not request data columns by range from peer")
			continue
		}
		f.p2p.Peers().Scorers().BlockProviderScorer().Touch(p)
		robs, err := verifyAndPopulateColumns(bwb, columns, req, f.bs)
		if err != nil {
			log.WithField("peer", p).WithError(err).Debug("Invalid DataColumnByRange response")
			continue
		}
		return robs, err
	}
	return nil, errNoPeersAvailable
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

func (f *blocksFetcher) requestColumns(ctx context.Context, req *p2ppb.DataColumnSidecarsByRangeRequest, pid peer.ID) ([]blocks.RODataColumn, error) {
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
	}).Debug("Requesting Columns")
	// We're intentionally abusing the block rate limit here, treating data column requests as if they were block requests.
	// Since column requests take more bandwidth than blocks, we should improve how we account for the different kinds
	// of requests, more in proportion to the cost of serving them.
	if f.rateLimiter.Remaining(pid.String()) < int64(req.Count) {
		if err := f.waitForBandwidth(pid, req.Count); err != nil {
			l.Unlock()
			return nil, err
		}
	}
	f.rateLimiter.Add(pid.String(), int64(req.Count))
	l.Unlock()

	return prysmsync.SendDataColumnsByRangeRequest(ctx, f.clock, f.p2p, pid, f.ctxMap, req)
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
	intCount, err := math.Int(count)
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
	filteredPeers := []peer.ID{}
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

func remotePeerHasCustody(wantedIdxs []uint64, remoteCustMap map[uint64]bool) bool {
	for _, wIdx := range wantedIdxs {
		if !remoteCustMap[wIdx] {
			return false
		}
	}
	return true
}
