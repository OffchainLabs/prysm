package initialsync

import (
	"context"
	"fmt"
	"strings"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	prysmsync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/sync/verify"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	p2ppb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// fetchSidecars fetches sidecars corresponding to blocks in `response.bwb`.
// It mutates `Blobs` and `Columns` fields of `response.bwb` with fetched sidecars.
// `pid` is the initial peer to request blob from (usually the peer from which the block originated),
// `peers` is a list of peers to use for the request blobs if `pid` fails.
// `bwScs` must me sorted by slot.
// It sets r.blobsFrom to the peer ID from which blobs were fetched (if any).
func (f *blocksFetcher) fetchSidecars(ctx context.Context, r *fetchRequestResponse, peers []peer.ID) {
	samplesPerSlot := params.BeaconConfig().SamplesPerSlot

	if len(r.bwb) == 0 {
		r.blobsFrom = ""
		return
	}

	firstFuluIndex, err := findFirstForkIndex(r.bwb, version.Fulu)
	if err != nil {
		r.err = errors.Wrap(err, "find first Fulu index")
		r.blobsFrom = ""
		return
	}

	preFulu := r.bwb[:firstFuluIndex]
	postFulu := r.bwb[firstFuluIndex:]

	var blobsPid peer.ID

	if len(preFulu) > 0 {
		// Fetch blob sidecars. Try first with the same peer that served the blocks.
		blobsPid, err = f.fetchBlobsFromPeer(ctx, preFulu, r.blocksFrom, peers)
		if err != nil {
			r.blobsFrom = ""
			r.err = errors.Wrap(err, "fetch blobs from peer")
			return
		}
	}

	r.blobsFrom = blobsPid
	if len(postFulu) == 0 {
		return
	}

	// Compute the columns to request.
	custodyGroupCount, err := f.p2p.CustodyGroupCount(ctx)
	if err != nil {
		r.err = errors.Wrap(err, "custody group count")
		return
	}

	samplingSize := max(custodyGroupCount, samplesPerSlot)
	info, _, err := peerdas.Info(f.p2p.NodeID(), samplingSize)
	if err != nil {
		r.err = errors.Wrap(err, "custody info")
		return
	}

	currentSlot := f.clock.CurrentSlot()
	currentEpoch := slots.ToEpoch(currentSlot)

	roBlocks := make([]blocks.ROBlock, 0, len(postFulu))
	for _, blockWithSidecars := range postFulu {
		blockSlot := blockWithSidecars.Block.Block().Slot()
		blockEpoch := slots.ToEpoch(blockSlot)

		if params.WithinDAPeriod(blockEpoch, currentEpoch) {
			roBlocks = append(roBlocks, blockWithSidecars.Block)
		}
	}

	// Return early if there are no blocks that need data column sidecars.
	if len(roBlocks) == 0 {
		return
	}

	// Some blocks need data column sidecars, fetch them.
	params := prysmsync.DataColumnSidecarsParams{
		Ctx:         ctx,
		Tor:         f.clock,
		P2P:         f.p2p,
		RateLimiter: f.rateLimiter,
		CtxMap:      f.ctxMap,
		Storage:     f.dcs,
		NewVerifier: f.cv,
	}

	verifiedRoDataColumnsByRoot, missingIndicesByRoot, err := prysmsync.FetchDataColumnSidecars(params, roBlocks, info.CustodyColumns)
	if err != nil {
		r.blobsFrom = ""
		r.err = errors.Wrap(err, "fetch data column sidecars")
		return
	}

	if len(missingIndicesByRoot) > 0 {
		prettyMissingIndicesByRoot := make(map[string]string, len(missingIndicesByRoot))
		for root, indices := range missingIndicesByRoot {
			prettyMissingIndicesByRoot[fmt.Sprintf("%#x", root)] = helpers.SortedPrettySliceFromMap(indices)
		}
		r.blobsFrom = ""
		r.err = errors.Errorf("some sidecars are still missing after fetch: %v", prettyMissingIndicesByRoot)
		return
	}

	// Populate the response.
	for i := range r.bwb {
		bwSc := &r.bwb[i]
		root := bwSc.Block.Root()
		if columns, ok := verifiedRoDataColumnsByRoot[root]; ok {
			bwSc.Columns = columns
		}
	}
}

type commitmentCount struct {
	slot  primitives.Slot
	root  [32]byte
	count int
}

type commitmentCountList []commitmentCount

// countCommitments makes a list of all blocks that have commitments that need to be satisfied.
// This gives us a representation to finish building the request that is lightweight and readable for testing.
// `bwb` must be sorted by slot.
func countCommitments(bwb []blocks.BlockWithROSidecars, retentionStart primitives.Slot) commitmentCountList {
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
func verifyAndPopulateBlobs(bwb []blocks.BlockWithROSidecars, blobs []blocks.ROBlob, req *p2ppb.BlobSidecarsByRangeRequest, bss filesystem.BlobStorageSummarizer) error {
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
func populateBlock(bw *blocks.BlockWithROSidecars, blobs []blocks.ROBlob, req *p2ppb.BlobSidecarsByRangeRequest, bss filesystem.BlobStorageSummarizer) error {
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
	for _, k := range missing {
		missStr = append(missStr, fmt.Sprintf("%#x", k))
	}
	return errors.Wrapf(errMissingBlobsForBlockCommitments,
		"block root %#x at slot %d missing %d commitments %s", root, slot, len(missing), strings.Join(missStr, ","))
}

// fetchBlobsFromPeer fetches blocks from a single randomly selected peer.
// This function mutates the input `bwb` argument.
// `pid` is the initial peer to request blobs from (usually the peer from which the block originated),
// `peers` is a list of peers to use for the request if `pid` fails.
// `bwb` must be sorted by slot.
// It returns the peer ID from which blobs were fetched.
func (f *blocksFetcher) fetchBlobsFromPeer(ctx context.Context, bwb []blocks.BlockWithROSidecars, pid peer.ID, peers []peer.ID) (peer.ID, error) {
	if len(bwb) == 0 {
		return "", nil
	}

	ctx, span := trace.StartSpan(ctx, "initialsync.fetchBlobsFromPeer")
	defer span.End()
	if slots.ToEpoch(f.clock.CurrentSlot()) < params.BeaconConfig().DenebForkEpoch {
		return "", nil
	}
	blobWindowStart, err := prysmsync.BlobRPCMinValidSlot(f.clock.CurrentSlot())
	if err != nil {
		return "", err
	}
	// Construct request message based on observed interval of blocks in need of blobs.
	req := countCommitments(bwb, blobWindowStart).blobRange(f.bs).Request()
	if req == nil {
		return "", nil
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
		return p, err
	}
	return "", errNoPeersAvailable
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
