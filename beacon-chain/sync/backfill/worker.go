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
	"github.com/pkg/errors"
)

type workerCfg struct {
	c    *startup.Clock
	v    *verifier
	cm   sync.ContextByteVersions
	nbv  verification.NewBlobVerifier
	ndcv verification.NewDataColumnsVerifier
	bfs  *filesystem.BlobStorage
	cfs  *filesystem.DataColumnStorage
}

func initWorkerCfg(ctx context.Context, cfg *workerCfg, c *startup.Clock, vw InitializerWaiter, store *Store, bfs *filesystem.BlobStorage, cfs *filesystem.DataColumnStorage) (*workerCfg, error) {
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
	if cfg == nil {
		cfg = &workerCfg{}
	}
	cfg.c = c
	cfg.v = v
	cfg.cm = cm
	cfg.bfs = bfs
	cfg.cfs = cfs
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
				log.WithField("batch_id", b.id()).WithError(ctx.Err()).Info("worker context canceled while waiting to retry")
				continue
			}
			log.WithFields(b.logFields()).WithField("backfillWorker", w.id).Debug("Backfill worker received batch")
			if b.state == batchSyncBlobs {
				w.done <- w.handleSidecars(ctx, b)
				continue
			}
			if b.state == batchSyncColumns {
				w.done <- w.handleColumns(ctx, b)
				continue
			}

			w.done <- w.handleBlocks(ctx, b)
		case <-ctx.Done():
			log.WithField("backfillWorker", w.id).Info("Backfill worker exiting after context canceled")
			return
		}
	}
}

func (w *p2pWorker) handleBlocks(ctx context.Context, b batch) batch {
	current := w.cfg.c.CurrentSlot()
	blobRetentionStart, err := sync.BlobRPCMinValidSlot(current)
	if err != nil {
		return b.withRetryableError(errors.Wrap(err, "configuration issue, could not compute minimum blob retention slot"))
	}
	b.blockPid = b.busy
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
		return b.withFatalError(err)
	}
	return b.postBlockSync(vb, bs, cs)
}

func (w *p2pWorker) handleBlobs(ctx context.Context, b batch) batch {
	b.blobPid = b.busy
	start := time.Now()
	// we don't need to use the response for anything other than metrics, because blobResponseValidation
	// adds each of them to a batch AvailabilityStore once it is checked.
	blobs, err := sync.SendBlobsByRangeRequest(ctx, w.cfg.c, w.p2p, b.blobPid, w.cfg.cm, b.blobRequest(), b.blobResponseValidator(), blobValidationMetrics)
	if err != nil {
		b.bs = nil
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
	return b.postSidecarSync()
}

func (w *p2pWorker) handleColumns(ctx context.Context, b batch) batch {
	b.columnPid = b.busy
	start := time.Now()
	vr := b.validatingColumnRequest()
	// Response is dropped because the validation code adds the columns to the columnSync AvailabilityStore under the hood.
	_, err := sync.SendDataColumnSidecarsByRangeRequest(ctx, w.cfg.c, w.p2p, b.busy, w.cfg.cm, vr.req, vr.validate)
	if err != nil {
		return b.withRetryableError(errors.Wrap(err, "failed to request data column sidecars"))
	}
	dlt := time.Now()
	dataColumnSidecarDownloadMs.Observe(float64(dlt.Sub(start).Milliseconds()))
	return b.postSidecarSync()
}
