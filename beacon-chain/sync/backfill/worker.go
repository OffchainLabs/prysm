package backfill

import (
	"context"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/sync"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
)

var errInvalidBatchState = errors.New("invalid batch state")

type peerDownscorer func(peer.ID, string, error)

type workerCfg struct {
	clock        *startup.Clock
	verifier     *verifier
	ctxMap       sync.ContextByteVersions
	newVB        verification.NewBlobVerifier
	newVC        verification.NewDataColumnsVerifier
	blobStore    *filesystem.BlobStorage
	colStore     *filesystem.DataColumnStorage
	downscore    peerDownscorer
	currentNeeds func() das.CurrentNeeds
}

func initWorkerCfg(ctx context.Context, cfg *workerCfg, vw InitializerWaiter, store *Store) error {
	vi, err := vw.WaitForInitializer(ctx)
	if err != nil {
		return errors.Wrap(err, "WaitForInitializer")
	}
	cps, err := store.originState(ctx)
	if err != nil {
		return errors.Wrap(err, "originState")
	}
	keys, err := cps.PublicKeys()
	if err != nil {
		return errors.Wrap(err, "unable to retrieve public keys for all validators in the origin state")
	}
	vr := cps.GenesisValidatorsRoot()
	cm, err := sync.ContextByteVersionsForValRoot(bytesutil.ToBytes32(vr))
	if err != nil {
		return errors.Wrapf(err, "unable to initialize context version map using genesis validator root %#x", vr)
	}
	v, err := newBackfillVerifier(vr, keys)
	if err != nil {
		return errors.Wrapf(err, "newBackfillVerifier failed")
	}
	cfg.verifier = v
	cfg.ctxMap = cm
	cfg.newVB = newBlobVerifierFromInitializer(vi)
	cfg.newVC = newDataColumnVerifierFromInitializer(vi)
	return nil
}

type workerId int

type p2pWorker struct {
	id   workerId
	todo chan batch
	done chan batch
	p2p  p2p.P2P
	cfg  *workerCfg
}

func newP2pWorker(id workerId, p p2p.P2P, todo, done chan batch, cfg *workerCfg) *p2pWorker {
	return &p2pWorker{
		id:   id,
		todo: todo,
		done: done,
		p2p:  p,
		cfg:  cfg,
	}
}

func (w *p2pWorker) run(ctx context.Context) {
	for {
		select {
		case b := <-w.todo:
			if err := b.waitUntilReady(ctx); err != nil {
				log.WithField("batchId", b.id()).WithError(ctx.Err()).Info("Worker context canceled while waiting to retry")
				continue
			}
			log.WithFields(b.logFields()).WithField("backfillWorker", w.id).Trace("Worker received batch")
			switch b.state {
			case batchSequenced:
				b = w.handleBlocks(ctx, b)
			case batchSyncBlobs:
				b = w.handleBlobs(ctx, b)
			case batchSyncColumns:
				b = w.handleColumns(ctx, b)
			case batchImportable:
				// This state indicates the batch got all the way to be imported and failed,
				// so we need clear out the blocks to go all the way back to the start of the process.
				b.blocks = nil
				b = w.handleBlocks(ctx, b)
			default:
				// A batch in an unknown state represents an implementation error,
				// so we treat it as a fatal error meaning the worker pool should shut down.
				b = b.withFatalError(errors.Wrap(errInvalidBatchState, b.state.String()))
			}
			w.done <- b
		case <-ctx.Done():
			log.WithField("backfillWorker", w.id).Info("Worker exiting after context canceled")
			return
		}
	}
}

func (w *p2pWorker) handleBlocks(ctx context.Context, b batch) batch {
	current := w.cfg.clock.CurrentSlot()
	b.blockPeer = b.assignedPeer
	start := time.Now()
	results, err := sync.SendBeaconBlocksByRangeRequest(ctx, w.cfg.clock, w.p2p, b.blockPeer, b.blockRequest(), blockValidationMetrics)
	if err != nil {
		log.WithError(err).WithFields(b.logFields()).Debug("Failed to request SignedBeaconBlocks by range")
		return b.withRetryableError(err)
	}
	dlt := time.Now()
	blockDownloadMs.Observe(float64(dlt.Sub(start).Milliseconds()))

	toVerify, err := blocks.NewROBlockSlice(results)
	if err != nil {
		log.WithError(err).WithFields(b.logFields()).Debug("Failed to convert blocks to ROBlock slice")
		return b.withRetryableError(err)
	}
	verified, err := w.cfg.verifier.verify(toVerify)
	blockVerifyMs.Observe(float64(time.Since(dlt).Milliseconds()))
	if err != nil {
		if shouldDownscore(err) {
			w.cfg.downscore(b.blockPeer, "invalid SignedBeaconBlock batch rpc response", err)
		}
		log.WithError(err).WithFields(b.logFields()).Debug("Validation failed")
		return b.withRetryableError(err)
	}

	// This is a hack to get the rough size of the batch. This helps us approximate the amount of memory needed
	// to hold batches and relative sizes between batches, but will be inaccurate when it comes to measuring actual
	// bytes downloaded from peers, mainly because the p2p messages are snappy compressed.
	bdl := 0
	for i := range verified {
		bdl += verified[i].SizeSSZ()
	}
	blockDownloadBytesApprox.Add(float64(bdl))
	log.WithFields(b.logFields()).WithField("bytesDownloaded", bdl).Trace("Blocks downloaded")
	b.blocks = verified

	bscfg := &blobSyncConfig{currentNeeds: w.cfg.currentNeeds, nbv: w.cfg.newVB, store: w.cfg.blobStore}
	bs, err := newBlobSync(current, verified, bscfg)
	if err != nil {
		return b.withRetryableError(err)
	}
	cs, err := newColumnSync(ctx, b, verified, current, w.p2p, w.cfg)
	if err != nil {
		return b.withRetryableError(err)
	}
	b.blobs = bs
	b.columns = cs
	return b.transitionToNext()
}

func (w *p2pWorker) handleBlobs(ctx context.Context, b batch) batch {
	b.blobs.peer = b.assignedPeer
	start := time.Now()
	// we don't need to use the response for anything other than metrics, because blobResponseValidation
	// adds each of them to a batch AvailabilityStore once it is checked.
	blobs, err := sync.SendBlobsByRangeRequest(ctx, w.cfg.clock, w.p2p, b.blobs.peer, w.cfg.ctxMap, b.blobRequest(), b.blobs.validateNext, blobValidationMetrics)
	if err != nil {
		b.blobs = nil
		return b.withRetryableError(err)
	}
	dlt := time.Now()
	blobSidecarDownloadMs.Observe(float64(dlt.Sub(start).Milliseconds()))
	if len(blobs) > 0 {
		// All blobs are the same size, so we can compute 1 and use it for all in the batch.
		sz := blobs[0].SizeSSZ() * len(blobs)
		blobSidecarDownloadBytesApprox.Add(float64(sz))
		log.WithFields(b.logFields()).WithField("bytesDownloaded", sz).Debug("Blobs downloaded")
	}
	if b.blobs.needed() > 0 {
		// If we are missing blobs after processing the blob step, this is an error and we need to scrap the batch and start over.
		b.blobs = nil
		// Wipe retries periodically to avoid getting stuck on a bad block batch
		if b.retries%3 == 0 {
			b.blocks = []blocks.ROBlock{}
		}
		return b.withRetryableError(errors.New("missing blobs after blob download"))
	}
	return b.transitionToNext()
}

func (w *p2pWorker) handleColumns(ctx context.Context, b batch) batch {
	start := time.Now()
	b.columns.peer = b.assignedPeer

	// Bisector is used to keep track of the peer that provided each column, for scoring purposes.
	// When verification of a batch of columns fails, bisector is used to retry verification with batches
	// grouped by peer, to figure out if the failure is due to a specific peer.
	vr, err := b.validatingColumnRequest(b.columns.bisector)
	if err != nil {
		return b.withRetryableError(errors.Wrap(err, "creating validating column request"))
	}
	p := sync.DataColumnSidecarsParams{
		Ctx:    ctx,
		Tor:    w.cfg.clock,
		P2P:    w.p2p,
		CtxMap: w.cfg.ctxMap,
		// DownscorePeerOnRPCFault is very aggressive and is only used for fetching origin blobs during startup.
		DownscorePeerOnRPCFault: false,
		// SendDataColumnSidecarsByRangeRequest uses the DataColumnSidecarsParams param struct to cover
		// multiple different use cases. Some of them have different required fields. The following fields are
		// not used in the methods that backfill invokes. SendDataColumnSidecarsByRangeRequest should be refactored
		// to only require the minimum set of parameters.
		//RateLimiter *leakybucket.Collector
		//Storage:     w.cfg.cfs,
		//NewVerifier: vr.validate,
	}
	// The return is dropped because the validation code adds the columns
	// to the columnSync AvailabilityStore under the hood.
	_, err = sync.SendDataColumnSidecarsByRangeRequest(p, b.columns.peer, vr.req, vr.validate)
	if err != nil {
		if shouldDownscore(err) {
			w.cfg.downscore(b.columns.peer, "invalid DataColumnSidecar rpc response", err)
		}
		return b.withRetryableError(errors.Wrap(err, "failed to request data column sidecars"))
	}
	dataColumnSidecarDownloadMs.Observe(float64(time.Since(start).Milliseconds()))
	return b.transitionToNext()
}

func shouldDownscore(err error) bool {
	return errors.Is(err, errInvalidDataColumnResponse) ||
		errors.Is(err, sync.ErrInvalidFetchedData) ||
		errors.Is(err, errInvalidBlocks)
}
