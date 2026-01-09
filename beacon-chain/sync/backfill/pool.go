package backfill

import (
	"context"
	"maps"
	"math"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/sync"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
)

type batchWorkerPool interface {
	spawn(ctx context.Context, n int, a PeerAssigner, cfg *workerCfg)
	todo(b batch)
	complete() (batch, error)
}

type worker interface {
	run(context.Context)
}

type newWorker func(id workerId, in, out chan batch, cfg *workerCfg) worker

func defaultNewWorker(p p2p.P2P) newWorker {
	return func(id workerId, in, out chan batch, cfg *workerCfg) worker {
		return newP2pWorker(id, p, in, out, cfg)
	}
}

// minRequestInterval is the minimum amount of time between requests.
// ie a value of 1s means we'll make ~1 req/sec per peer.
const minReqInterval = time.Second

type p2pBatchWorkerPool struct {
	maxBatches     int
	newWorker      newWorker
	toWorkers      chan batch
	fromWorkers    chan batch
	toRouter       chan batch
	fromRouter     chan batch
	shutdownErr    chan error
	endSeq         []batch
	ctx            context.Context
	cancel         func()
	earliest       primitives.Slot // earliest is the earliest slot a worker is processing
	peerCache      *sync.DASPeerCache
	p2p            p2p.P2P
	peerFailLogger *intervalLogger
	needs          func() das.CurrentNeeds
}

var _ batchWorkerPool = &p2pBatchWorkerPool{}

func newP2PBatchWorkerPool(p p2p.P2P, maxBatches int, needs func() das.CurrentNeeds) *p2pBatchWorkerPool {
	nw := defaultNewWorker(p)
	return &p2pBatchWorkerPool{
		newWorker:      nw,
		toRouter:       make(chan batch, maxBatches),
		fromRouter:     make(chan batch, maxBatches),
		toWorkers:      make(chan batch),
		fromWorkers:    make(chan batch),
		maxBatches:     maxBatches,
		shutdownErr:    make(chan error),
		peerCache:      sync.NewDASPeerCache(p),
		p2p:            p,
		peerFailLogger: newIntervalLogger(log, 5),
		earliest:       primitives.Slot(math.MaxUint64),
		needs:          needs,
	}
}

func (p *p2pBatchWorkerPool) spawn(ctx context.Context, n int, a PeerAssigner, cfg *workerCfg) {
	p.ctx, p.cancel = context.WithCancel(ctx)
	go p.batchRouter(a)
	for i := range n {
		go p.newWorker(workerId(i), p.toWorkers, p.fromWorkers, cfg).run(p.ctx)
	}
}

func (p *p2pBatchWorkerPool) todo(b batch) {
	// Intercept batchEndSequence batches so workers can remain unaware of this state.
	// Workers don't know what to do with batchEndSequence batches. They are a signal to the pool that the batcher
	// has stopped producing things for the workers to do and the pool is close to winding down. See complete()
	// to understand how the pool manages the state where all workers are idle
	// and all incoming batches signal end of sequence.
	if b.state == batchEndSequence {
		p.endSeq = append(p.endSeq, b)
		return
	}
	p.toRouter <- b
}

func (p *p2pBatchWorkerPool) complete() (batch, error) {
	if len(p.endSeq) == p.maxBatches {
		return p.endSeq[0], errEndSequence
	}

	select {
	case b := <-p.fromRouter:
		return b, nil
	case err := <-p.shutdownErr:
		return batch{}, errors.Wrap(err, "fatal error from backfill worker pool")
	case <-p.ctx.Done():
		log.WithError(p.ctx.Err()).Info("p2pBatchWorkerPool context canceled, shutting down")
		return batch{}, p.ctx.Err()
	}
}

func (p *p2pBatchWorkerPool) batchRouter(pa PeerAssigner) {
	busy := make(map[peer.ID]bool)
	todo := make([]batch, 0)
	rt := time.NewTicker(time.Second)
	for {
		select {
		case b := <-p.toRouter:
			todo = append(todo, b)
			// sort batches in descending order so that we'll always process the dependent batches first
			sortBatchDesc(todo)
		case <-rt.C:
			// Worker assignments can fail if assignBatch can't find a suitable peer.
			// This ticker exists to periodically break out of the channel select
			// to retry failed assignments.
		case b := <-p.fromWorkers:
			if b.state == batchErrFatal {
				p.shutdown(b.err)
			}
			pid := b.assignedPeer
			delete(busy, pid)
			if b.workComplete() {
				p.fromRouter <- b
				break
			}
			todo = append(todo, b)
			sortBatchDesc(todo)
		case <-p.ctx.Done():
			log.WithError(p.ctx.Err()).Info("p2pBatchWorkerPool context canceled, shutting down")
			p.shutdown(p.ctx.Err())
			return
		}
		var err error
		todo, err = p.processTodo(todo, pa, busy)
		if err != nil {
			p.shutdown(err)
		}
	}
}

func (p *p2pBatchWorkerPool) processTodo(todo []batch, pa PeerAssigner, busy map[peer.ID]bool) ([]batch, error) {
	if len(todo) == 0 {
		return todo, nil
	}
	notBusy, err := pa.Assign(peers.NotBusy(busy))
	if err != nil {
		if errors.Is(err, peers.ErrInsufficientSuitable) {
			// Transient error resulting from insufficient number of connected peers. Leave batches in
			// queue and get to them whenever the peer situation is resolved.
			return todo, nil
		}
		return nil, err
	}
	if len(notBusy) == 0 {
		log.Debug("No suitable peers available for batch assignment")
		return todo, nil
	}

	custodied := peerdas.NewColumnIndices()
	if highestEpoch(todo) >= params.BeaconConfig().FuluForkEpoch {
		custodied, err = currentCustodiedColumns(p.ctx, p.p2p)
		if err != nil {
			return nil, errors.Wrap(err, "current custodied columns")
		}
	}
	picker, err := p.peerCache.NewPicker(notBusy, custodied, minReqInterval)
	if err != nil {
		log.WithError(err).Error("Failed to compute column-weighted peer scores")
		return todo, nil
	}

	for i, b := range todo {
		needs := p.needs()
		if b.expired(needs) {
			p.endSeq = append(p.endSeq, b.withState(batchEndSequence))
			continue
		}
		excludePeers := busy
		if b.state == batchErrFatal {
			// Fatal error detected in batch, shut down the pool.
			return nil, b.err
		}

		if b.state == batchErrRetryable {
			// Columns can fail in a partial fashion, so we nee to reset
			// components that track peer interactions for multiple columns
			// to enable partial retries.
			b = resetToRetryColumns(b, needs)
			if b.state == batchSequenced {
				// Transitioning to batchSequenced means we need to download a new block batch because there was
				// a problem making or verifying the last block request, so we should try to pick a different peer this time.
				excludePeers = busyCopy(busy)
				excludePeers[b.blockPeer] = true
				b.blockPeer = "" // reset block peer so we can fail back to it next time if there is an issue with assignment.
			}
		}

		pid, cols, err := b.selectPeer(picker, excludePeers)
		if err != nil {
			p.peerFailLogger.WithField("notBusy", len(notBusy)).WithError(err).WithFields(b.logFields()).Debug("Failed to select peer for batch")
			// Return the remaining todo items and allow the outer loop to control when we try again.
			return todo[i:], nil
		}
		busy[pid] = true
		b.assignedPeer = pid
		b.nextReqCols = cols

		backfillBatchTimeWaiting.Observe(float64(time.Since(b.scheduled).Milliseconds()))
		p.toWorkers <- b
		p.updateEarliest(b.begin)
	}
	return []batch{}, nil
}

func busyCopy(busy map[peer.ID]bool) map[peer.ID]bool {
	busyCp := make(map[peer.ID]bool, len(busy))
	maps.Copy(busyCp, busy)
	return busyCp
}

func highestEpoch(batches []batch) primitives.Epoch {
	highest := primitives.Epoch(0)
	for _, b := range batches {
		epoch := slots.ToEpoch(b.end - 1)
		if epoch > highest {
			highest = epoch
		}
	}
	return highest
}

func (p *p2pBatchWorkerPool) updateEarliest(current primitives.Slot) {
	if current >= p.earliest {
		return
	}
	p.earliest = current
	oldestBatch.Set(float64(p.earliest))
}

func (p *p2pBatchWorkerPool) shutdown(err error) {
	p.cancel()
	p.shutdownErr <- err
}
