package backfill

import (
	"context"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/sync"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
)

type peerDownscorer func(peer.ID, string, error)

type workerCfg struct {
	c         *startup.Clock
	v         *verifier
	cm        sync.ContextByteVersions
	nbv       verification.NewBlobVerifier
	ndcv      verification.NewDataColumnsVerifier
	bfs       *filesystem.BlobStorage
	cfs       *filesystem.DataColumnStorage
	downscore peerDownscorer
}

func initWorkerCfg(ctx context.Context, cfg *workerCfg, vw InitializerWaiter, store *Store) (*workerCfg, error) {
	vi, err := vw.WaitForInitializer(ctx)
	if err != nil {
		return nil, err
	}
	cps, err := store.originState(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := cps.PublicKeys()
	if err != nil {
		return nil, errors.Wrap(err, "unable to retrieve public keys for all validators in the origin state")
	}
	vr := cps.GenesisValidatorsRoot()
	cm, err := sync.ContextByteVersionsForValRoot(bytesutil.ToBytes32(vr))
	if err != nil {
		return nil, errors.Wrapf(err, "unable to initialize context version map using genesis validator root %#x", vr)
	}
	v, err := newBackfillVerifier(vr, keys)
	if err != nil {
		return nil, errors.Wrapf(err, "newBackfillVerifier failed")
	}
	cfg.v = v
	cfg.cm = cm
	cfg.nbv = newBlobVerifierFromInitializer(vi)
	cfg.ndcv = newDataColumnVerifierFromInitializer(vi)
	return cfg, nil
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
			log.WithFields(b.logFields()).WithField("backfillWorker", w.id).Trace("Backfill worker received batch")
			switch b.state {
			case batchSyncBlobs:
				b = w.handleBlobs(ctx, b)
			case batchSyncColumns:
				b = w.handleColumns(ctx, b)
			case batchSequenced:
				b = w.handleBlocks(ctx, b)
			default:
				log.WithFields(b.logFields()).WithField("backfillWorker", w.id).Debug("batch in unhandled state")
				panic("unhandled batch state") // lint:nopanic -- TODO: this panic is temporary / for debugging.
			}
			w.done <- b
		case <-ctx.Done():
			log.WithField("backfillWorker", w.id).Info("Backfill worker exiting after context canceled")
			return
		}
	}
}

func resetRetryableColumns(b batch) batch {
	// return the given batch as-is if it isn't in a state that this func should handle.
	if b.columns == nil || b.columns.bisector == nil || len(b.columns.bisector.errs) == 0 {
		return b
	}
	bisector := b.columns.bisector
	roots := bisector.failingRoots()
	if len(roots) == 0 {
		return b
	}
	// Add all the failed columns back to the toDownload structure.
	for _, root := range roots {
		bc := b.columns.toDownload[root]
		bc.remaining.Union(bisector.failuresFor(root))
	}
	b.columns.bisector.reset()
	return b
}

func (w *p2pWorker) handleBlocks(ctx context.Context, b batch) batch {
	current := w.cfg.c.CurrentSlot()
	// TODO: refactor all the blob and column setup stuff.
	// we know the slot when we first set up the batch, so we should be able to determine if we need the blob setup bits at all
	// before we fetch the blocks. Same goes for the column dependencies.
	blobRetentionStart, err := sync.BlobRPCMinValidSlot(current)
	if err != nil {
		return b.withRetryableError(errors.Wrap(err, "configuration issue, could not compute minimum blob retention slot"))
	}
	b.blockPid = b.peer
	start := time.Now()
	results, err := sync.SendBeaconBlocksByRangeRequest(ctx, w.cfg.c, w.p2p, b.blockPid, b.blockRequest(), blockValidationMetrics)
	if err != nil {
		log.WithError(err).WithFields(b.logFields()).Debug("Batch requesting failed")
		return b.withRetryableError(err)
	}
	dlt := time.Now()
	blockDownloadMs.Observe(float64(dlt.Sub(start).Milliseconds()))
	toVerify, err := blocks.NewROBlockSlice(results)
	if err != nil {
		log.WithError(err).WithFields(b.logFields()).Debug("Batch conversion to ROBlock failed")
		return b.withRetryableError(err)
	}

	vb, err := w.cfg.v.verify(toVerify)
	blockVerifyMs.Observe(float64(time.Since(dlt).Milliseconds()))
	if err != nil {
		log.WithError(err).WithFields(b.logFields()).Debug("Batch validation failed")
		return b.withRetryableError(err)
	}
	// This is a hack to get the rough size of the batch. This helps us approximate the amount of memory needed
	// to hold batches and relative sizes between batches, but will be inaccurate when it comes to measuring actual
	// bytes downloaded from peers, mainly because the p2p messages are snappy compressed.
	bdl := 0
	for i := range vb {
		bdl += vb[i].SizeSSZ()
	}
	blockDownloadBytesApprox.Add(float64(bdl))
	log.WithFields(b.logFields()).WithField("dlbytes", bdl).Debug("Backfill batch block bytes downloaded")
	bscfg := &blobSyncConfig{retentionStart: blobRetentionStart, nbv: w.cfg.nbv, store: w.cfg.bfs}
	bs, err := newBlobSync(current, vb, bscfg)
	if err != nil {
		return b.withRetryableError(err)
	}
	cs, err := newColumnSync(b, vb, current, w.p2p, vb, w.cfg)
	if err != nil {
		return b.withRetryableError(err)
	}
	b.blocks = vb
	b.blobs = bs
	b.columns = cs
	return b.transitionToNext()
}

func (w *p2pWorker) handleBlobs(ctx context.Context, b batch) batch {
	b.blobs.pid = b.peer
	start := time.Now()
	// we don't need to use the response for anything other than metrics, because blobResponseValidation
	// adds each of them to a batch AvailabilityStore once it is checked.
	blobs, err := sync.SendBlobsByRangeRequest(ctx, w.cfg.c, w.p2p, b.blobs.pid, w.cfg.cm, b.blobRequest(), b.blobs.validateNext, blobValidationMetrics)
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
		log.WithFields(b.logFields()).WithField("dlbytes", sz).Debug("Backfill batch blob bytes downloaded")
	}
	if b.blobs.needed() > 0 {
		// If we are missing blobs after processing the blob step, this is an error and we need to scrap the batch and start over.
		b.blobs = nil
		b.blocks = []blocks.ROBlock{}
		return b.withRetryableError(errors.New("missing blobs after blob download"))
	}
	return b.transitionToNext()
}

func (w *p2pWorker) handleColumns(ctx context.Context, b batch) batch {
	start := time.Now()
	b.columns.peer = b.peer

	// Bisector is used to keep track of the peer that provided each column, for scoring purposes.
	// When verification of a batch of columns fails, bisector is used to retry verification with batches
	// grouped by peer, to figure out if the failure is due to a specific peer.
	vr := b.validatingColumnRequest(b.columns.bisector)
	// TODO: the upstream definition of SendDataColumnSidecarsByRangeRequest requires this params type
	// which has several ambiguously optional fields. The sidecar request functions should be refactored
	// to use a more explicit set of parameters. RateLimiter, Storage and NewVerifier are not used inside
	// SendDataColumnSidecarsByRangeRequest.
	p := sync.DataColumnSidecarsParams{
		Ctx: ctx,
		Tor: w.cfg.c,
		P2P: w.p2p,
		//RateLimiter *leakybucket.Collector
		CtxMap: w.cfg.cm,
		//Storage:     w.cfg.cfs,
		//NewVerifier: vr.validate,
	}
	// The return is dropped because the validation code adds the columns
	// to the columnSync AvailabilityStore under the hood.
	_, err := sync.SendDataColumnSidecarsByRangeRequest(p, b.columns.peer, vr.req, vr.validate)
	if err != nil {
		if shouldDownscore(err) {
			w.cfg.downscore(b.columns.peer, "bad SendDataColumnSidecarsByRangeRequest response", err)
		}
		return b.withRetryableError(errors.Wrap(err, "failed to request data column sidecars"))
	}
	dataColumnSidecarDownloadMs.Observe(float64(time.Now().Sub(start).Milliseconds()))
	return b.transitionToNext()
}

func shouldDownscore(err error) bool {
	return errors.Is(err, errInvalidDataColumnResponse) ||
		errors.Is(err, sync.ErrInvalidFetchedData)
}
