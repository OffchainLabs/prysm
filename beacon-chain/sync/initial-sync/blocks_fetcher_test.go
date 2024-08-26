package initialsync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	GoKZG "github.com/crate-crypto/go-kzg-4844"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/kzg"
	mock "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db/filesystem"
	dbtest "github.com/prysmaticlabs/prysm/v5/beacon-chain/db/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/peers"
	p2ptest "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/startup"
	beaconsync "github.com/prysmaticlabs/prysm/v5/beacon-chain/sync"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	"github.com/prysmaticlabs/prysm/v5/cmd/beacon-chain/flags"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	leakybucket "github.com/prysmaticlabs/prysm/v5/container/leaky-bucket"
	"github.com/prysmaticlabs/prysm/v5/container/slice"
	ecdsaprysm "github.com/prysmaticlabs/prysm/v5/crypto/ecdsa"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/network/forks"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/sirupsen/logrus"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func TestBlocksFetcher_InitStartStop(t *testing.T) {
	mc, p2p, _ := initializeTestServices(t, []primitives.Slot{}, []*peerData{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fetcher := newBlocksFetcher(
		ctx,
		&blocksFetcherConfig{
			chain: mc,
			p2p:   p2p,
		},
	)

	t.Run("check for leaked goroutines", func(t *testing.T) {
		err := fetcher.start()
		require.NoError(t, err)
		fetcher.stop() // should block up until all resources are reclaimed
		select {
		case <-fetcher.requestResponses():
		default:
			t.Error("fetchResponses channel is leaked")
		}
	})

	t.Run("re-starting of stopped fetcher", func(t *testing.T) {
		assert.ErrorContains(t, errFetcherCtxIsDone.Error(), fetcher.start())
	})

	t.Run("multiple stopping attempts", func(t *testing.T) {
		fetcher := newBlocksFetcher(
			context.Background(),
			&blocksFetcherConfig{
				chain: mc,
				p2p:   p2p,
			})
		require.NoError(t, fetcher.start())
		fetcher.stop()
		fetcher.stop()
	})

	t.Run("cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		fetcher := newBlocksFetcher(
			ctx,
			&blocksFetcherConfig{
				chain: mc,
				p2p:   p2p,
			})
		require.NoError(t, fetcher.start())
		cancel()
		fetcher.stop()
	})

	t.Run("peer filter capacity weight", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		fetcher := newBlocksFetcher(
			ctx,
			&blocksFetcherConfig{
				chain:                    mc,
				p2p:                      p2p,
				peerFilterCapacityWeight: 2,
			})
		require.NoError(t, fetcher.start())
		assert.Equal(t, peerFilterCapacityWeight, fetcher.capacityWeight)
	})
}

func TestBlocksFetcher_RoundRobin(t *testing.T) {
	slotsInBatch := primitives.Slot(flags.Get().BlockBatchLimit)
	requestsGenerator := func(start, end, batchSize primitives.Slot) []*fetchRequestParams {
		var requests []*fetchRequestParams
		for i := start; i <= end; i += batchSize {
			requests = append(requests, &fetchRequestParams{
				start: i,
				count: uint64(batchSize),
			})
		}
		return requests
	}
	tests := []struct {
		name               string
		expectedBlockSlots []primitives.Slot
		peers              []*peerData
		requests           []*fetchRequestParams
	}{
		{
			name:               "Single peer with all blocks",
			expectedBlockSlots: makeSequence(1, 3*slotsInBatch),
			peers: []*peerData{
				{
					blocks:         makeSequence(1, 3*slotsInBatch),
					finalizedEpoch: slots.ToEpoch(3 * slotsInBatch),
					headSlot:       3 * slotsInBatch,
				},
			},
			requests: requestsGenerator(1, 3*slotsInBatch, slotsInBatch),
		},
		{
			name:               "Single peer with all blocks (many small requests)",
			expectedBlockSlots: makeSequence(1, 3*slotsInBatch),
			peers: []*peerData{
				{
					blocks:         makeSequence(1, 3*slotsInBatch),
					finalizedEpoch: slots.ToEpoch(3 * slotsInBatch),
					headSlot:       3 * slotsInBatch,
				},
			},
			requests: requestsGenerator(1, 3*slotsInBatch, slotsInBatch/4),
		},
		{
			name:               "Multiple peers with all blocks",
			expectedBlockSlots: makeSequence(1, 3*slotsInBatch),
			peers: []*peerData{
				{
					blocks:         makeSequence(1, 3*slotsInBatch),
					finalizedEpoch: slots.ToEpoch(3 * slotsInBatch),
					headSlot:       3 * slotsInBatch,
				},
				{
					blocks:         makeSequence(1, 3*slotsInBatch),
					finalizedEpoch: slots.ToEpoch(3 * slotsInBatch),
					headSlot:       3 * slotsInBatch,
				},
				{
					blocks:         makeSequence(1, 3*slotsInBatch),
					finalizedEpoch: slots.ToEpoch(3 * slotsInBatch),
					headSlot:       3 * slotsInBatch,
				},
			},
			requests: requestsGenerator(1, 3*slotsInBatch, slotsInBatch),
		},
		{
			name:               "Multiple peers with skipped slots",
			expectedBlockSlots: append(makeSequence(1, 64), makeSequence(500, 640)...), // up to 18th epoch
			peers: []*peerData{
				{
					blocks:         append(makeSequence(1, 64), makeSequence(500, 640)...),
					finalizedEpoch: 18,
					headSlot:       640,
				},
				{
					blocks:         append(makeSequence(1, 64), makeSequence(500, 640)...),
					finalizedEpoch: 18,
					headSlot:       640,
				},
				{
					blocks:         append(makeSequence(1, 64), makeSequence(500, 640)...),
					finalizedEpoch: 18,
					headSlot:       640,
				},
				{
					blocks:         append(makeSequence(1, 64), makeSequence(500, 640)...),
					finalizedEpoch: 18,
					headSlot:       640,
				},
				{
					blocks:         append(makeSequence(1, 64), makeSequence(500, 640)...),
					finalizedEpoch: 18,
					headSlot:       640,
				},
			},
			requests: []*fetchRequestParams{
				{
					start: 1,
					count: uint64(slotsInBatch),
				},
				{
					start: slotsInBatch + 1,
					count: uint64(slotsInBatch),
				},
				{
					start: 2*slotsInBatch + 1,
					count: uint64(slotsInBatch),
				},
				{
					start: 500,
					count: 53,
				},
				{
					start: 553,
					count: 200,
				},
			},
		},
		{
			name:               "Multiple peers with failures",
			expectedBlockSlots: makeSequence(1, 2*slotsInBatch),
			peers: []*peerData{
				{
					blocks:         makeSequence(1, 320),
					finalizedEpoch: 8,
					headSlot:       320,
				},
				{
					blocks:         makeSequence(1, 320),
					finalizedEpoch: 8,
					headSlot:       320,
					failureSlots:   makeSequence(1, 32), // first epoch
				},
				{
					blocks:         makeSequence(1, 320),
					finalizedEpoch: 8,
					headSlot:       320,
				},
				{
					blocks:         makeSequence(1, 320),
					finalizedEpoch: 8,
					headSlot:       320,
				},
			},
			requests: []*fetchRequestParams{
				{
					start: 1,
					count: uint64(slotsInBatch),
				},
				{
					start: slotsInBatch + 1,
					count: uint64(slotsInBatch),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache.initializeRootCache(tt.expectedBlockSlots, t)

			beaconDB := dbtest.SetupDB(t)

			p := p2ptest.NewTestP2P(t)
			connectPeers(t, p, tt.peers, p.Peers())
			cache.RLock()
			genesisRoot := cache.rootCache[0]
			cache.RUnlock()

			util.SaveBlock(t, context.Background(), beaconDB, util.NewBeaconBlock())

			st, err := util.NewBeaconState()
			require.NoError(t, err)

			gt := time.Now()
			vr := [32]byte{}
			clock := startup.NewClock(gt, vr)
			mc := &mock.ChainService{
				State: st,
				Root:  genesisRoot[:],
				DB:    beaconDB,
				FinalizedCheckPoint: &ethpb.Checkpoint{
					Epoch: 0,
				},
				Genesis:        time.Now(),
				ValidatorsRoot: [32]byte{},
			}

			ctx, cancel := context.WithCancel(context.Background())
			fetcher := newBlocksFetcher(ctx, &blocksFetcherConfig{
				chain: mc,
				p2p:   p,
				clock: clock,
			})
			require.NoError(t, fetcher.start())

			var wg sync.WaitGroup
			wg.Add(len(tt.requests)) // how many block requests we are going to make
			go func() {
				wg.Wait()
				log.Debug("Stopping fetcher")
				fetcher.stop()
			}()

			processFetchedBlocks := func() ([]blocks.BlockWithROBlobs, error) {
				defer cancel()
				var unionRespBlocks []blocks.BlockWithROBlobs

				for {
					select {
					case resp, ok := <-fetcher.requestResponses():
						if !ok { // channel closed, aggregate
							return unionRespBlocks, nil
						}

						if resp.err != nil {
							log.WithError(resp.err).Debug("Block fetcher returned error")
						} else {
							unionRespBlocks = append(unionRespBlocks, resp.bwb...)
							if len(resp.bwb) == 0 {
								log.WithFields(logrus.Fields{
									"start": resp.start,
									"count": resp.count,
								}).Debug("Received empty slot")
							}
						}

						wg.Done()
					case <-ctx.Done():
						log.Debug("Context closed, exiting goroutine")
						return unionRespBlocks, nil
					}
				}
			}

			maxExpectedBlocks := uint64(0)
			for _, requestParams := range tt.requests {
				err = fetcher.scheduleRequest(context.Background(), requestParams.start, requestParams.count)
				assert.NoError(t, err)
				maxExpectedBlocks += requestParams.count
			}

			bwb, err := processFetchedBlocks()
			assert.NoError(t, err)

			sort.Sort(blocks.BlockWithROBlobsSlice(bwb))
			ss := make([]primitives.Slot, len(bwb))
			for i, b := range bwb {
				ss[i] = b.Block.Block().Slot()
			}

			log.WithFields(logrus.Fields{
				"blocksLen": len(bwb),
				"slots":     ss,
			}).Debug("Finished block fetching")

			if len(bwb) > int(maxExpectedBlocks) {
				t.Errorf("Too many blocks returned. Wanted %d got %d", maxExpectedBlocks, len(bwb))
			}
			assert.Equal(t, len(tt.expectedBlockSlots), len(bwb), "Processes wrong number of blocks")
			var receivedBlockSlots []primitives.Slot
			for _, b := range bwb {
				receivedBlockSlots = append(receivedBlockSlots, b.Block.Block().Slot())
			}
			missing := slice.NotSlot(slice.IntersectionSlot(tt.expectedBlockSlots, receivedBlockSlots), tt.expectedBlockSlots)
			if len(missing) > 0 {
				t.Errorf("Missing blocks at slots %v", missing)
			}
		})
	}
}

func TestBlocksFetcher_scheduleRequest(t *testing.T) {
	blockBatchLimit := uint64(flags.Get().BlockBatchLimit)
	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		fetcher := newBlocksFetcher(ctx, &blocksFetcherConfig{})
		cancel()
		assert.ErrorContains(t, "context canceled", fetcher.scheduleRequest(ctx, 1, blockBatchLimit))
	})

	t.Run("unblock on context cancellation", func(t *testing.T) {
		fetcher := newBlocksFetcher(context.Background(), &blocksFetcherConfig{})
		for i := 0; i < maxPendingRequests; i++ {
			assert.NoError(t, fetcher.scheduleRequest(context.Background(), 1, blockBatchLimit))
		}

		// Will block on next request (and wait until requests are either processed or context is closed).
		go func() {
			fetcher.cancel()
		}()
		assert.ErrorContains(t, errFetcherCtxIsDone.Error(),
			fetcher.scheduleRequest(context.Background(), 1, blockBatchLimit))
	})
}
func TestBlocksFetcher_handleRequest(t *testing.T) {
	blockBatchLimit := flags.Get().BlockBatchLimit
	chainConfig := struct {
		expectedBlockSlots []primitives.Slot
		peers              []*peerData
	}{
		expectedBlockSlots: makeSequence(1, primitives.Slot(blockBatchLimit)),
		peers: []*peerData{
			{
				blocks:         makeSequence(1, 320),
				finalizedEpoch: 8,
				headSlot:       320,
			},
			{
				blocks:         makeSequence(1, 320),
				finalizedEpoch: 8,
				headSlot:       320,
			},
		},
	}

	mc, p2p, _ := initializeTestServices(t, chainConfig.expectedBlockSlots, chainConfig.peers)
	mc.ValidatorsRoot = [32]byte{}
	mc.Genesis = time.Now()

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		fetcher := newBlocksFetcher(ctx, &blocksFetcherConfig{
			chain: mc,
			p2p:   p2p,
			clock: startup.NewClock(mc.Genesis, mc.ValidatorsRoot),
		})

		cancel()
		response := fetcher.handleRequest(ctx, 1, uint64(blockBatchLimit))
		assert.ErrorContains(t, "context canceled", response.err)
	})

	t.Run("receive blocks", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		fetcher := newBlocksFetcher(ctx, &blocksFetcherConfig{
			chain: mc,
			p2p:   p2p,
			clock: startup.NewClock(mc.Genesis, mc.ValidatorsRoot),
		})

		requestCtx, reqCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer reqCancel()
		go func() {
			response := fetcher.handleRequest(requestCtx, 1 /* start */, uint64(blockBatchLimit) /* count */)
			select {
			case <-ctx.Done():
			case fetcher.fetchResponses <- response:
			}
		}()

		var bwb []blocks.BlockWithROBlobs
		select {
		case <-ctx.Done():
			t.Error(ctx.Err())
		case resp := <-fetcher.requestResponses():
			if resp.err != nil {
				t.Error(resp.err)
			} else {
				bwb = resp.bwb
			}
		}
		if uint64(len(bwb)) != uint64(blockBatchLimit) {
			t.Errorf("incorrect number of blocks returned, expected: %v, got: %v", blockBatchLimit, len(bwb))
		}

		var receivedBlockSlots []primitives.Slot
		for _, b := range bwb {
			receivedBlockSlots = append(receivedBlockSlots, b.Block.Block().Slot())
		}
		missing := slice.NotSlot(slice.IntersectionSlot(chainConfig.expectedBlockSlots, receivedBlockSlots), chainConfig.expectedBlockSlots)
		if len(missing) > 0 {
			t.Errorf("Missing blocks at slots %v", missing)
		}
	})
}

func TestBlocksFetcher_requestBeaconBlocksByRange(t *testing.T) {
	blockBatchLimit := flags.Get().BlockBatchLimit
	chainConfig := struct {
		expectedBlockSlots []primitives.Slot
		peers              []*peerData
	}{
		expectedBlockSlots: makeSequence(1, 320),
		peers: []*peerData{
			{
				blocks:         makeSequence(1, 320),
				finalizedEpoch: 8,
				headSlot:       320,
			},
			{
				blocks:         makeSequence(1, 320),
				finalizedEpoch: 8,
				headSlot:       320,
			},
		},
	}

	mc, p2p, _ := initializeTestServices(t, chainConfig.expectedBlockSlots, chainConfig.peers)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fetcher := newBlocksFetcher(
		ctx,
		&blocksFetcherConfig{
			chain: mc,
			p2p:   p2p,
		})

	_, peerIDs := p2p.Peers().BestFinalized(params.BeaconConfig().MaxPeersToSync, slots.ToEpoch(mc.HeadSlot()))
	req := &ethpb.BeaconBlocksByRangeRequest{
		StartSlot: 1,
		Step:      1,
		Count:     uint64(blockBatchLimit),
	}
	blocks, err := fetcher.requestBlocks(ctx, req, peerIDs[0])
	assert.NoError(t, err)
	assert.Equal(t, uint64(blockBatchLimit), uint64(len(blocks)), "Incorrect number of blocks returned")

	// Test context cancellation.
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	_, err = fetcher.requestBlocks(ctx, req, peerIDs[0])
	assert.ErrorContains(t, "context canceled", err)
}

func TestBlocksFetcher_RequestBlocksRateLimitingLocks(t *testing.T) {
	p1 := p2ptest.NewTestP2P(t)
	p2 := p2ptest.NewTestP2P(t)
	p3 := p2ptest.NewTestP2P(t)
	p1.Connect(p2)
	p1.Connect(p3)
	require.Equal(t, 2, len(p1.BHost.Network().Peers()), "Expected peers to be connected")
	req := &ethpb.BeaconBlocksByRangeRequest{
		StartSlot: 100,
		Step:      1,
		Count:     64,
	}

	topic := p2p.RPCBlocksByRangeTopicV1
	protocol := libp2pcore.ProtocolID(topic + p2.Encoding().ProtocolSuffix())
	streamHandlerFn := func(stream network.Stream) {
		assert.NoError(t, stream.Close())
	}
	p2.BHost.SetStreamHandler(protocol, streamHandlerFn)
	p3.BHost.SetStreamHandler(protocol, streamHandlerFn)

	burstFactor := uint64(flags.Get().BlockBatchLimitBurstFactor)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fetcher := newBlocksFetcher(ctx, &blocksFetcherConfig{p2p: p1})
	fetcher.rateLimiter = leakybucket.NewCollector(float64(req.Count), int64(req.Count*burstFactor), 1*time.Second, false)
	gt := time.Now()
	vr := [32]byte{}
	fetcher.chain = &mock.ChainService{Genesis: gt, ValidatorsRoot: vr}
	hook := logTest.NewGlobal()
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		// Exhaust available rate for p2, so that rate limiting is triggered.
		for i := uint64(0); i <= burstFactor; i++ {
			if i == burstFactor {
				// The next request will trigger rate limiting for p2. Now, allow concurrent
				// p3 data request (p3 shouldn't be rate limited).
				time.AfterFunc(1*time.Second, func() {
					wg.Done()
				})
			}
			_, err := fetcher.requestBlocks(ctx, req, p2.PeerID())
			if err != nil {
				assert.ErrorContains(t, errFetcherCtxIsDone.Error(), err)
			}
		}
	}()

	// Wait until p2 exhausts its rate and is spinning on rate limiting timer.
	wg.Wait()

	// The next request should NOT trigger rate limiting as rate is exhausted for p2, not p3.
	ch := make(chan struct{}, 1)
	go func() {
		_, err := fetcher.requestBlocks(ctx, req, p3.PeerID())
		assert.NoError(t, err)
		ch <- struct{}{}
	}()
	timer := time.NewTimer(2 * time.Second)
	select {
	case <-timer.C:
		t.Error("p3 takes too long to respond: lock contention")
	case <-ch:
		// p3 responded w/o waiting for rate limiter's lock (on which p2 spins).
	}
	// Make sure that p2 has been rate limited.
	require.LogsContain(t, hook, fmt.Sprintf("msg=\"Slowing down for rate limit\" peer=%s", p2.PeerID()))
}

func TestBlocksFetcher_WaitForBandwidth(t *testing.T) {
	p1 := p2ptest.NewTestP2P(t)
	p2 := p2ptest.NewTestP2P(t)
	p1.Connect(p2)
	require.Equal(t, 1, len(p1.BHost.Network().Peers()), "Expected peers to be connected")
	req := &ethpb.BeaconBlocksByRangeRequest{
		Count: 64,
	}

	topic := p2p.RPCBlocksByRangeTopicV1
	protocol := libp2pcore.ProtocolID(topic + p2.Encoding().ProtocolSuffix())
	streamHandlerFn := func(stream network.Stream) {
		assert.NoError(t, stream.Close())
	}
	p2.BHost.SetStreamHandler(protocol, streamHandlerFn)

	burstFactor := uint64(flags.Get().BlockBatchLimitBurstFactor)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fetcher := newBlocksFetcher(ctx, &blocksFetcherConfig{p2p: p1})
	fetcher.rateLimiter = leakybucket.NewCollector(float64(req.Count), int64(req.Count*burstFactor), 5*time.Second, false)
	gt := time.Now()
	vr := [32]byte{}
	fetcher.chain = &mock.ChainService{Genesis: gt, ValidatorsRoot: vr}
	start := time.Now()
	assert.NoError(t, fetcher.waitForBandwidth(p2.PeerID(), 10))
	dur := time.Since(start)
	assert.Equal(t, true, dur < time.Millisecond, "waited excessively for bandwidth")
	fetcher.rateLimiter.Add(p2.PeerID().String(), int64(req.Count*burstFactor))
	start = time.Now()
	assert.NoError(t, fetcher.waitForBandwidth(p2.PeerID(), req.Count))
	dur = time.Since(start)
	assert.Equal(t, float64(5), dur.Truncate(1*time.Second).Seconds(), "waited excessively for bandwidth")
}

func TestBlocksFetcher_requestBlocksFromPeerReturningInvalidBlocks(t *testing.T) {
	p1 := p2ptest.NewTestP2P(t)
	tests := []struct {
		name         string
		req          *ethpb.BeaconBlocksByRangeRequest
		handlerGenFn func(req *ethpb.BeaconBlocksByRangeRequest) func(stream network.Stream)
		wantedErr    string
		validate     func(req *ethpb.BeaconBlocksByRangeRequest, blocks []interfaces.ReadOnlySignedBeaconBlock)
	}{
		{
			name: "no error",
			req: &ethpb.BeaconBlocksByRangeRequest{
				StartSlot: 100,
				Step:      4,
				Count:     64,
			},
			handlerGenFn: func(req *ethpb.BeaconBlocksByRangeRequest) func(stream network.Stream) {
				return func(stream network.Stream) {
					for i := req.StartSlot; i < req.StartSlot.Add(req.Count*req.Step); i += primitives.Slot(req.Step) {
						blk := util.NewBeaconBlock()
						blk.Block.Slot = i
						tor := startup.NewClock(time.Now(), [32]byte{})
						wsb, err := blocks.NewSignedBeaconBlock(blk)
						require.NoError(t, err)
						assert.NoError(t, beaconsync.WriteBlockChunk(stream, tor, p1.Encoding(), wsb))
					}
					assert.NoError(t, stream.Close())
				}
			},
			validate: func(req *ethpb.BeaconBlocksByRangeRequest, blocks []interfaces.ReadOnlySignedBeaconBlock) {
				assert.Equal(t, req.Count, uint64(len(blocks)))
			},
		},
		{
			name: "too many blocks",
			req: &ethpb.BeaconBlocksByRangeRequest{
				StartSlot: 100,
				Step:      1,
				Count:     64,
			},
			handlerGenFn: func(req *ethpb.BeaconBlocksByRangeRequest) func(stream network.Stream) {
				return func(stream network.Stream) {
					for i := req.StartSlot; i < req.StartSlot.Add(req.Count*req.Step+1); i += primitives.Slot(req.Step) {
						blk := util.NewBeaconBlock()
						blk.Block.Slot = i
						tor := startup.NewClock(time.Now(), [32]byte{})
						wsb, err := blocks.NewSignedBeaconBlock(blk)
						require.NoError(t, err)
						assert.NoError(t, beaconsync.WriteBlockChunk(stream, tor, p1.Encoding(), wsb))
					}
					assert.NoError(t, stream.Close())
				}
			},
			validate: func(req *ethpb.BeaconBlocksByRangeRequest, blocks []interfaces.ReadOnlySignedBeaconBlock) {
				assert.Equal(t, 0, len(blocks))
			},
			wantedErr: beaconsync.ErrInvalidFetchedData.Error(),
		},
		{
			name: "not in a consecutive order",
			req: &ethpb.BeaconBlocksByRangeRequest{
				StartSlot: 100,
				Step:      1,
				Count:     64,
			},
			handlerGenFn: func(req *ethpb.BeaconBlocksByRangeRequest) func(stream network.Stream) {
				return func(stream network.Stream) {
					blk := util.NewBeaconBlock()
					blk.Block.Slot = 163
					tor := startup.NewClock(time.Now(), [32]byte{})
					wsb, err := blocks.NewSignedBeaconBlock(blk)
					require.NoError(t, err)
					assert.NoError(t, beaconsync.WriteBlockChunk(stream, tor, p1.Encoding(), wsb))

					blk = util.NewBeaconBlock()
					blk.Block.Slot = 162
					wsb, err = blocks.NewSignedBeaconBlock(blk)
					require.NoError(t, err)
					assert.NoError(t, beaconsync.WriteBlockChunk(stream, tor, p1.Encoding(), wsb))
					assert.NoError(t, stream.Close())
				}
			},
			validate: func(req *ethpb.BeaconBlocksByRangeRequest, blocks []interfaces.ReadOnlySignedBeaconBlock) {
				assert.Equal(t, 0, len(blocks))
			},
			wantedErr: beaconsync.ErrInvalidFetchedData.Error(),
		},
		{
			name: "same slot number",
			req: &ethpb.BeaconBlocksByRangeRequest{
				StartSlot: 100,
				Step:      1,
				Count:     64,
			},
			handlerGenFn: func(req *ethpb.BeaconBlocksByRangeRequest) func(stream network.Stream) {
				return func(stream network.Stream) {
					blk := util.NewBeaconBlock()
					blk.Block.Slot = 160
					tor := startup.NewClock(time.Now(), [32]byte{})

					wsb, err := blocks.NewSignedBeaconBlock(blk)
					require.NoError(t, err)
					assert.NoError(t, beaconsync.WriteBlockChunk(stream, tor, p1.Encoding(), wsb))

					blk = util.NewBeaconBlock()
					blk.Block.Slot = 160
					wsb, err = blocks.NewSignedBeaconBlock(blk)
					require.NoError(t, err)
					assert.NoError(t, beaconsync.WriteBlockChunk(stream, tor, p1.Encoding(), wsb))
					assert.NoError(t, stream.Close())
				}
			},
			validate: func(req *ethpb.BeaconBlocksByRangeRequest, blocks []interfaces.ReadOnlySignedBeaconBlock) {
				assert.Equal(t, 0, len(blocks))
			},
			wantedErr: beaconsync.ErrInvalidFetchedData.Error(),
		},
		{
			name: "slot is too low",
			req: &ethpb.BeaconBlocksByRangeRequest{
				StartSlot: 100,
				Step:      1,
				Count:     64,
			},
			handlerGenFn: func(req *ethpb.BeaconBlocksByRangeRequest) func(stream network.Stream) {
				return func(stream network.Stream) {
					defer func() {
						assert.NoError(t, stream.Close())
					}()
					for i := req.StartSlot; i < req.StartSlot.Add(req.Count*req.Step); i += primitives.Slot(req.Step) {
						blk := util.NewBeaconBlock()
						tor := startup.NewClock(time.Now(), [32]byte{})
						// Patch mid block, with invalid slot number.
						if i == req.StartSlot.Add(req.Count*req.Step/2) {
							blk.Block.Slot = req.StartSlot - 1
							wsb, err := blocks.NewSignedBeaconBlock(blk)
							require.NoError(t, err)
							assert.NoError(t, beaconsync.WriteBlockChunk(stream, tor, p1.Encoding(), wsb))
							break
						}
						blk.Block.Slot = i
						wsb, err := blocks.NewSignedBeaconBlock(blk)
						require.NoError(t, err)
						assert.NoError(t, beaconsync.WriteBlockChunk(stream, tor, p1.Encoding(), wsb))
					}
				}
			},
			wantedErr: beaconsync.ErrInvalidFetchedData.Error(),
			validate: func(req *ethpb.BeaconBlocksByRangeRequest, blocks []interfaces.ReadOnlySignedBeaconBlock) {
				assert.Equal(t, 0, len(blocks))
			},
		},
		{
			name: "slot is too high",
			req: &ethpb.BeaconBlocksByRangeRequest{
				StartSlot: 100,
				Step:      1,
				Count:     64,
			},
			handlerGenFn: func(req *ethpb.BeaconBlocksByRangeRequest) func(stream network.Stream) {
				return func(stream network.Stream) {
					defer func() {
						assert.NoError(t, stream.Close())
					}()
					for i := req.StartSlot; i < req.StartSlot.Add(req.Count*req.Step); i += primitives.Slot(req.Step) {
						blk := util.NewBeaconBlock()
						tor := startup.NewClock(time.Now(), [32]byte{})
						// Patch mid block, with invalid slot number.
						if i == req.StartSlot.Add(req.Count*req.Step/2) {
							blk.Block.Slot = req.StartSlot.Add(req.Count * req.Step)
							wsb, err := blocks.NewSignedBeaconBlock(blk)
							require.NoError(t, err)
							assert.NoError(t, beaconsync.WriteBlockChunk(stream, tor, p1.Encoding(), wsb))
							break
						}
						blk.Block.Slot = i
						wsb, err := blocks.NewSignedBeaconBlock(blk)
						require.NoError(t, err)
						assert.NoError(t, beaconsync.WriteBlockChunk(stream, tor, p1.Encoding(), wsb))
					}
				}
			},
			wantedErr: beaconsync.ErrInvalidFetchedData.Error(),
			validate: func(req *ethpb.BeaconBlocksByRangeRequest, blocks []interfaces.ReadOnlySignedBeaconBlock) {
				assert.Equal(t, 0, len(blocks))
			},
		},
		{
			name: "valid step increment",
			req: &ethpb.BeaconBlocksByRangeRequest{
				StartSlot: 100,
				Step:      5,
				Count:     64,
			},
			handlerGenFn: func(req *ethpb.BeaconBlocksByRangeRequest) func(stream network.Stream) {
				return func(stream network.Stream) {
					blk := util.NewBeaconBlock()
					blk.Block.Slot = 100
					tor := startup.NewClock(time.Now(), [32]byte{})
					wsb, err := blocks.NewSignedBeaconBlock(blk)
					require.NoError(t, err)
					assert.NoError(t, beaconsync.WriteBlockChunk(stream, tor, p1.Encoding(), wsb))

					blk = util.NewBeaconBlock()
					blk.Block.Slot = 105
					wsb, err = blocks.NewSignedBeaconBlock(blk)
					require.NoError(t, err)
					assert.NoError(t, beaconsync.WriteBlockChunk(stream, tor, p1.Encoding(), wsb))
					assert.NoError(t, stream.Close())
				}
			},
			validate: func(req *ethpb.BeaconBlocksByRangeRequest, blocks []interfaces.ReadOnlySignedBeaconBlock) {
				assert.Equal(t, 2, len(blocks))
			},
		},
		{
			name: "invalid step increment",
			req: &ethpb.BeaconBlocksByRangeRequest{
				StartSlot: 100,
				Step:      5,
				Count:     64,
			},
			handlerGenFn: func(req *ethpb.BeaconBlocksByRangeRequest) func(stream network.Stream) {
				return func(stream network.Stream) {
					blk := util.NewBeaconBlock()
					blk.Block.Slot = 100
					tor := startup.NewClock(time.Now(), [32]byte{})
					wsb, err := blocks.NewSignedBeaconBlock(blk)
					require.NoError(t, err)
					assert.NoError(t, beaconsync.WriteBlockChunk(stream, tor, p1.Encoding(), wsb))

					blk = util.NewBeaconBlock()
					blk.Block.Slot = 103
					wsb, err = blocks.NewSignedBeaconBlock(blk)
					require.NoError(t, err)
					assert.NoError(t, beaconsync.WriteBlockChunk(stream, tor, p1.Encoding(), wsb))
					assert.NoError(t, stream.Close())
				}
			},
			validate: func(req *ethpb.BeaconBlocksByRangeRequest, blocks []interfaces.ReadOnlySignedBeaconBlock) {
				assert.Equal(t, 0, len(blocks))
			},
			wantedErr: beaconsync.ErrInvalidFetchedData.Error(),
		},
	}

	topic := p2p.RPCBlocksByRangeTopicV1
	protocol := libp2pcore.ProtocolID(topic + p1.Encoding().ProtocolSuffix())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fetcher := newBlocksFetcher(ctx, &blocksFetcherConfig{p2p: p1, chain: &mock.ChainService{Genesis: time.Now(), ValidatorsRoot: [32]byte{}}})
	fetcher.rateLimiter = leakybucket.NewCollector(0.000001, 640, 1*time.Second, false)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p2 := p2ptest.NewTestP2P(t)
			p1.Connect(p2)

			p2.BHost.SetStreamHandler(protocol, tt.handlerGenFn(tt.req))
			blocks, err := fetcher.requestBlocks(ctx, tt.req, p2.PeerID())
			if tt.wantedErr != "" {
				assert.ErrorContains(t, tt.wantedErr, err)
			} else {
				assert.NoError(t, err)
				tt.validate(tt.req, blocks)
			}
		})
	}
}

func TestTimeToWait(t *testing.T) {
	tests := []struct {
		name          string
		wanted        int64
		rem           int64
		capacity      int64
		timeTillEmpty time.Duration
		want          time.Duration
	}{
		{
			name:          "Limiter has sufficient blocks",
			wanted:        64,
			rem:           64,
			capacity:      320,
			timeTillEmpty: 200 * time.Second,
			want:          0 * time.Second,
		},
		{
			name:          "Limiter has full capacity remaining",
			wanted:        350,
			rem:           320,
			capacity:      320,
			timeTillEmpty: 0 * time.Second,
			want:          0 * time.Second,
		},
		{
			name:          "Limiter has reached full capacity",
			wanted:        64,
			rem:           0,
			capacity:      640,
			timeTillEmpty: 60 * time.Second,
			want:          6 * time.Second,
		},
		{
			name:          "Requesting full capacity from peer",
			wanted:        640,
			rem:           0,
			capacity:      640,
			timeTillEmpty: 60 * time.Second,
			want:          60 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := timeToWait(tt.wanted, tt.rem, tt.capacity, tt.timeTillEmpty); got != tt.want {
				t.Errorf("timeToWait() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBlobRangeForBlocks(t *testing.T) {
	blks, _ := util.ExtendBlocksPlusBlobs(t, []blocks.ROBlock{}, 10)
	sbbs := make([]interfaces.ReadOnlySignedBeaconBlock, len(blks))
	for i := range blks {
		sbbs[i] = blks[i]
	}
	retentionStart := primitives.Slot(5)
	bwb, err := sortedBlockWithVerifiedBlobSlice(sbbs)
	require.NoError(t, err)
	bounds := countCommitments(bwb, retentionStart).blobRange(nil)
	require.Equal(t, retentionStart, bounds.low)
	higher := primitives.Slot(len(blks) + 1)
	bounds = countCommitments(bwb, higher).blobRange(nil)
	var nilBounds *blobRange
	require.Equal(t, nilBounds, bounds)

	blks, _ = util.ExtendBlocksPlusBlobs(t, []blocks.ROBlock{}, 10)
	sbbs = make([]interfaces.ReadOnlySignedBeaconBlock, len(blks))
	for i := range blks {
		sbbs[i] = blks[i]
	}
	bwb, err = sortedBlockWithVerifiedBlobSlice(sbbs)
	require.NoError(t, err)
	retentionStart = bwb[5].Block.Block().Slot()
	next := bwb[6].Block.Block().Slot()
	skip := bwb[5].Block.Block()
	bwb[5].Block, _ = util.GenerateTestDenebBlockWithSidecar(t, skip.ParentRoot(), skip.Slot(), 0)
	bounds = countCommitments(bwb, retentionStart).blobRange(nil)
	require.Equal(t, next, bounds.low)
}

func TestBlobRequest(t *testing.T) {
	var nilReq *ethpb.BlobSidecarsByRangeRequest
	// no blocks
	req := countCommitments([]blocks.BlockWithROBlobs{}, 0).blobRange(nil).Request()
	require.Equal(t, nilReq, req)
	blks, _ := util.ExtendBlocksPlusBlobs(t, []blocks.ROBlock{}, 10)
	sbbs := make([]interfaces.ReadOnlySignedBeaconBlock, len(blks))
	for i := range blks {
		sbbs[i] = blks[i]
	}
	bwb, err := sortedBlockWithVerifiedBlobSlice(sbbs)
	require.NoError(t, err)
	maxBlkSlot := primitives.Slot(len(blks) - 1)

	tooHigh := primitives.Slot(len(blks) + 1)
	req = countCommitments(bwb, tooHigh).blobRange(nil).Request()
	require.Equal(t, nilReq, req)

	req = countCommitments(bwb, maxBlkSlot).blobRange(nil).Request()
	require.Equal(t, uint64(1), req.Count)
	require.Equal(t, maxBlkSlot, req.StartSlot)

	halfway := primitives.Slot(5)
	req = countCommitments(bwb, halfway).blobRange(nil).Request()
	require.Equal(t, halfway, req.StartSlot)
	// adding 1 to include the halfway slot itself
	require.Equal(t, uint64(1+maxBlkSlot-halfway), req.Count)

	before := bwb[0].Block.Block().Slot()
	allAfter := bwb[1:]
	req = countCommitments(allAfter, before).blobRange(nil).Request()
	require.Equal(t, allAfter[0].Block.Block().Slot(), req.StartSlot)
	require.Equal(t, len(allAfter), int(req.Count))
}

func TestCountCommitments(t *testing.T) {
	type testcase struct {
		name     string
		bwb      func(t *testing.T, c testcase) []blocks.BlockWithROBlobs
		retStart primitives.Slot
		resCount int
	}
	cases := []testcase{
		{
			name: "nil blocks is safe",
			bwb: func(t *testing.T, c testcase) []blocks.BlockWithROBlobs {
				return nil
			},
			retStart: 0,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bwb := c.bwb(t, c)
			cc := countCommitments(bwb, c.retStart)
			require.Equal(t, c.resCount, len(cc))
		})
	}
}

func TestCommitmentCountList(t *testing.T) {
	cases := []struct {
		name     string
		cc       commitmentCountList
		bss      func(*testing.T) filesystem.BlobStorageSummarizer
		expected *blobRange
		request  *ethpb.BlobSidecarsByRangeRequest
	}{
		{
			name:     "nil commitmentCount is safe",
			cc:       nil,
			expected: nil,
			request:  nil,
		},
		{
			name: "nil bss, single slot",
			cc: []commitmentCount{
				{slot: 11235, count: 1},
			},
			expected: &blobRange{low: 11235, high: 11235},
			request:  &ethpb.BlobSidecarsByRangeRequest{StartSlot: 11235, Count: 1},
		},
		{
			name: "nil bss, sparse slots",
			cc: []commitmentCount{
				{slot: 11235, count: 1},
				{slot: 11240, count: fieldparams.MaxBlobsPerBlock},
				{slot: 11250, count: 3},
			},
			expected: &blobRange{low: 11235, high: 11250},
			request:  &ethpb.BlobSidecarsByRangeRequest{StartSlot: 11235, Count: 16},
		},
		{
			name: "AllAvailable in middle, some avail low, none high",
			bss: func(t *testing.T) filesystem.BlobStorageSummarizer {
				onDisk := map[[32]byte][]int{
					bytesutil.ToBytes32([]byte("0")): {0, 1},
					bytesutil.ToBytes32([]byte("1")): {0, 1, 2, 3, 4, 5},
				}
				return filesystem.NewMockBlobStorageSummarizer(t, onDisk)
			},
			cc: []commitmentCount{
				{slot: 0, count: 3, root: bytesutil.ToBytes32([]byte("0"))},
				{slot: 5, count: fieldparams.MaxBlobsPerBlock, root: bytesutil.ToBytes32([]byte("1"))},
				{slot: 15, count: 3},
			},
			expected: &blobRange{low: 0, high: 15},
			request:  &ethpb.BlobSidecarsByRangeRequest{StartSlot: 0, Count: 16},
		},
		{
			name: "AllAvailable at high and low",
			bss: func(t *testing.T) filesystem.BlobStorageSummarizer {
				onDisk := map[[32]byte][]int{
					bytesutil.ToBytes32([]byte("0")): {0, 1},
					bytesutil.ToBytes32([]byte("2")): {0, 1, 2, 3, 4, 5},
				}
				return filesystem.NewMockBlobStorageSummarizer(t, onDisk)
			},
			cc: []commitmentCount{
				{slot: 0, count: 2, root: bytesutil.ToBytes32([]byte("0"))},
				{slot: 5, count: 3},
				{slot: 15, count: fieldparams.MaxBlobsPerBlock, root: bytesutil.ToBytes32([]byte("2"))},
			},
			expected: &blobRange{low: 5, high: 5},
			request:  &ethpb.BlobSidecarsByRangeRequest{StartSlot: 5, Count: 1},
		},
		{
			name: "AllAvailable at high and low, adjacent range in middle",
			bss: func(t *testing.T) filesystem.BlobStorageSummarizer {
				onDisk := map[[32]byte][]int{
					bytesutil.ToBytes32([]byte("0")): {0, 1},
					bytesutil.ToBytes32([]byte("2")): {0, 1, 2, 3, 4, 5},
				}
				return filesystem.NewMockBlobStorageSummarizer(t, onDisk)
			},
			cc: []commitmentCount{
				{slot: 0, count: 2, root: bytesutil.ToBytes32([]byte("0"))},
				{slot: 5, count: 3},
				{slot: 6, count: 3},
				{slot: 15, count: fieldparams.MaxBlobsPerBlock, root: bytesutil.ToBytes32([]byte("2"))},
			},
			expected: &blobRange{low: 5, high: 6},
			request:  &ethpb.BlobSidecarsByRangeRequest{StartSlot: 5, Count: 2},
		},
		{
			name: "AllAvailable at high and low, range in middle",
			bss: func(t *testing.T) filesystem.BlobStorageSummarizer {
				onDisk := map[[32]byte][]int{
					bytesutil.ToBytes32([]byte("0")): {0, 1},
					bytesutil.ToBytes32([]byte("1")): {0, 1},
					bytesutil.ToBytes32([]byte("2")): {0, 1, 2, 3, 4, 5},
				}
				return filesystem.NewMockBlobStorageSummarizer(t, onDisk)
			},
			cc: []commitmentCount{
				{slot: 0, count: 2, root: bytesutil.ToBytes32([]byte("0"))},
				{slot: 5, count: 3, root: bytesutil.ToBytes32([]byte("1"))},
				{slot: 10, count: 3},
				{slot: 15, count: fieldparams.MaxBlobsPerBlock, root: bytesutil.ToBytes32([]byte("2"))},
			},
			expected: &blobRange{low: 5, high: 10},
			request:  &ethpb.BlobSidecarsByRangeRequest{StartSlot: 5, Count: 6},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var bss filesystem.BlobStorageSummarizer
			if c.bss != nil {
				bss = c.bss(t)
			}
			br := c.cc.blobRange(bss)
			require.DeepEqual(t, c.expected, br)
			if c.request == nil {
				require.IsNil(t, br.Request())
			} else {
				req := br.Request()
				require.DeepEqual(t, req.StartSlot, c.request.StartSlot)
				require.DeepEqual(t, req.Count, c.request.Count)
			}
		})
	}
}

func testSequenceBlockWithBlob(t *testing.T, nblocks int) ([]blocks.BlockWithROBlobs, []blocks.ROBlob) {
	blks, blobs := util.ExtendBlocksPlusBlobs(t, []blocks.ROBlock{}, nblocks)
	sbbs := make([]interfaces.ReadOnlySignedBeaconBlock, len(blks))
	for i := range blks {
		sbbs[i] = blks[i]
	}
	bwb, err := sortedBlockWithVerifiedBlobSlice(sbbs)
	require.NoError(t, err)
	return bwb, blobs
}

func testReqFromResp(bwb []blocks.BlockWithROBlobs) *ethpb.BlobSidecarsByRangeRequest {
	return &ethpb.BlobSidecarsByRangeRequest{
		StartSlot: bwb[0].Block.Block().Slot(),
		Count:     uint64(bwb[len(bwb)-1].Block.Block().Slot()-bwb[0].Block.Block().Slot()) + 1,
	}
}

func TestVerifyAndPopulateBlobs(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		bwb, blobs := testSequenceBlockWithBlob(t, 10)

		expectedCommits := make(map[[48]byte]bool)
		for _, bl := range blobs {
			expectedCommits[bytesutil.ToBytes48(bl.KzgCommitment)] = true
		}
		require.Equal(t, len(blobs), len(expectedCommits))

		err := verifyAndPopulateBlobs(bwb, blobs, testReqFromResp(bwb), nil)
		require.NoError(t, err)
		for _, bw := range bwb {
			commits, err := bw.Block.Block().Body().BlobKzgCommitments()
			require.NoError(t, err)
			require.Equal(t, len(commits), len(bw.Blobs))
			for i := range commits {
				bc := bytesutil.ToBytes48(commits[i])
				require.Equal(t, bc, bytesutil.ToBytes48(bw.Blobs[i].KzgCommitment))
				// Since we delete entries we've seen, duplicates will cause an error here.
				_, ok := expectedCommits[bc]
				// Make sure this was an expected delete, then delete it from the map so we can make sure we saw all of them.
				require.Equal(t, true, ok)
				delete(expectedCommits, bc)
			}
		}
		// We delete each entry we've seen, so if we see all expected commits, the map should be empty at the end.
		require.Equal(t, 0, len(expectedCommits))
	})
	t.Run("missing blobs", func(t *testing.T) {
		bwb, blobs := testSequenceBlockWithBlob(t, 10)
		err := verifyAndPopulateBlobs(bwb, blobs[1:], testReqFromResp(bwb), nil)
		require.ErrorIs(t, err, errMissingBlobsForBlockCommitments)
	})
	t.Run("no blobs for last block", func(t *testing.T) {
		bwb, blobs := testSequenceBlockWithBlob(t, 10)
		lastIdx := len(bwb) - 1
		lastBlk := bwb[lastIdx].Block
		cmts, err := lastBlk.Block().Body().BlobKzgCommitments()
		require.NoError(t, err)
		blobs = blobs[0 : len(blobs)-len(cmts)]
		lastBlk, _ = util.GenerateTestDenebBlockWithSidecar(t, lastBlk.Block().ParentRoot(), lastBlk.Block().Slot(), 0)
		bwb[lastIdx].Block = lastBlk
		err = verifyAndPopulateBlobs(bwb, blobs, testReqFromResp(bwb), nil)
		require.NoError(t, err)
	})
	t.Run("blobs not copied if all locally available", func(t *testing.T) {
		bwb, blobs := testSequenceBlockWithBlob(t, 10)
		// r1 only has some blobs locally available, so we'll still copy them all.
		// r7 has all blobs locally available, so we shouldn't copy them.
		i1, i7 := 1, 7
		r1, r7 := bwb[i1].Block.Root(), bwb[i7].Block.Root()
		onDisk := map[[32]byte][]int{
			r1: {0, 1},
			r7: {0, 1, 2, 3, 4, 5},
		}
		bss := filesystem.NewMockBlobStorageSummarizer(t, onDisk)
		err := verifyAndPopulateBlobs(bwb, blobs, testReqFromResp(bwb), bss)
		require.NoError(t, err)
		require.Equal(t, 6, len(bwb[i1].Blobs))
		require.Equal(t, 0, len(bwb[i7].Blobs))
	})
}

func TestBatchLimit(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	testCfg := params.BeaconConfig().Copy()
	testCfg.DenebForkEpoch = math.MaxUint64
	params.OverrideBeaconConfig(testCfg)

	resetFlags := flags.Get()
	flags.Init(&flags.GlobalFlags{
		BlockBatchLimit:            640,
		BlockBatchLimitBurstFactor: 10,
	})
	defer func() {
		flags.Init(resetFlags)
	}()

	assert.Equal(t, 640, maxBatchLimit())

	testCfg.DenebForkEpoch = 100000
	params.OverrideBeaconConfig(testCfg)

	assert.Equal(t, params.BeaconConfig().MaxRequestBlocksDeneb, uint64(maxBatchLimit()))
}

func TestBlockFetcher_HasSufficientBandwidth(t *testing.T) {
	bf := newBlocksFetcher(context.Background(), &blocksFetcherConfig{})
	currCap := bf.rateLimiter.Capacity()
	wantedAmt := currCap - 100
	bf.rateLimiter.Add(peer.ID("a").String(), wantedAmt)
	bf.rateLimiter.Add(peer.ID("c").String(), wantedAmt)
	bf.rateLimiter.Add(peer.ID("f").String(), wantedAmt)
	bf.rateLimiter.Add(peer.ID("d").String(), wantedAmt)

	receivedPeers := bf.hasSufficientBandwidth([]peer.ID{"a", "b", "c", "d", "e", "f"}, 110)
	for _, p := range receivedPeers {
		switch p {
		case "a", "c", "f", "d":
			t.Errorf("peer has exceeded capacity: %s", p)
		}
	}
	assert.Equal(t, 2, len(receivedPeers))
}

func TestSortedSliceFromMap(t *testing.T) {
	m := map[uint64]bool{1: true, 3: true, 2: true, 4: true}
	expected := []uint64{1, 2, 3, 4}

	actual := sortedSliceFromMap(m)
	require.DeepSSZEqual(t, expected, actual)
}

type blockParams struct {
	slot     primitives.Slot
	hasBlobs bool
}

func rootFromUint64(u uint64) [fieldparams.RootLength]byte {
	var root [fieldparams.RootLength]byte
	binary.LittleEndian.PutUint64(root[:], u)
	return root
}

func createPeer(t *testing.T, privateKeyOffset int, custodyCount uint64) (*enr.Record, peer.ID) {
	privateKeyBytes := make([]byte, 32)
	for i := 0; i < 32; i++ {
		privateKeyBytes[i] = byte(privateKeyOffset + i)
	}

	unmarshalledPrivateKey, err := crypto.UnmarshalSecp256k1PrivateKey(privateKeyBytes)
	require.NoError(t, err)

	privateKey, err := ecdsaprysm.ConvertFromInterfacePrivKey(unmarshalledPrivateKey)
	require.NoError(t, err)

	peerID, err := peer.IDFromPrivateKey(unmarshalledPrivateKey)
	require.NoError(t, err)

	record := &enr.Record{}
	record.Set(peerdas.Csc(custodyCount))
	record.Set(enode.Secp256k1(privateKey.PublicKey))

	return record, peerID
}

func TestCustodyAllNeededColumns(t *testing.T) {
	const dataColumnsCount = 31

	p2p := p2ptest.NewTestP2P(t)

	dataColumns := make(map[uint64]bool, dataColumnsCount)
	for i := range dataColumnsCount {
		dataColumns[uint64(i)] = true
	}

	custodyCounts := [...]uint64{
		4 * params.BeaconConfig().CustodyRequirement,
		32 * params.BeaconConfig().CustodyRequirement,
		4 * params.BeaconConfig().CustodyRequirement,
		32 * params.BeaconConfig().CustodyRequirement}

	peersID := make([]peer.ID, 0, len(custodyCounts))
	for _, custodyCount := range custodyCounts {
		peerRecord, peerID := createPeer(t, len(peersID), custodyCount)
		peersID = append(peersID, peerID)
		p2p.Peers().Add(peerRecord, peerID, nil, network.DirOutbound)
	}

	expected := []peer.ID{peersID[1], peersID[3]}

	blocksFetcher := newBlocksFetcher(context.Background(), &blocksFetcherConfig{
		p2p: p2p,
	})

	actual, err := blocksFetcher.custodyAllNeededColumns(peersID, dataColumns)
	require.NoError(t, err)

	require.DeepSSZEqual(t, expected, actual)
}

func TestCustodyColumns(t *testing.T) {
	blocksFetcher := newBlocksFetcher(context.Background(), &blocksFetcherConfig{
		p2p: p2ptest.NewTestP2P(t),
	})

	expected := params.BeaconConfig().CustodyRequirement

	actual, err := blocksFetcher.custodyColumns()
	require.NoError(t, err)

	require.Equal(t, int(expected), len(actual))
}

func TestMinInt(t *testing.T) {
	input := []int{1, 2, 3, 4, 5, 5, 4, 3, 2, 1}
	const expected = 1

	actual := minInt(input)

	require.Equal(t, expected, actual)
}

func TestMaxInt(t *testing.T) {
	input := []int{1, 2, 3, 4, 5, 5, 4, 3, 2, 1}
	const expected = 5

	actual := maxInt(input)

	require.Equal(t, expected, actual)
}

// deterministicRandomness returns a random bytes array based on the seed
func deterministicRandomness(t *testing.T, seed int64) [32]byte {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, seed)
	require.NoError(t, err)
	bytes := buf.Bytes()

	return sha256.Sum256(bytes)
}

// getRandFieldElement returns a serialized random field element in big-endian
func getRandFieldElement(t *testing.T, seed int64) [32]byte {
	bytes := deterministicRandomness(t, seed)
	var r fr.Element
	r.SetBytes(bytes[:])

	return GoKZG.SerializeScalar(r)
}

// getRandBlob returns a random blob using the passed seed as entropy
func getRandBlob(t *testing.T, seed int64) kzg.Blob {
	var blob kzg.Blob
	for i := 0; i < len(blob); i += 32 {
		fieldElementBytes := getRandFieldElement(t, seed+int64(i))
		copy(blob[i:i+32], fieldElementBytes[:])
	}
	return blob
}

type (
	responseParams struct {
		slot        primitives.Slot
		columnIndex uint64
		alterate    bool
	}

	peerParams struct {
		// Custody subnet count
		csc uint64

		// key: RPCDataColumnSidecarsByRangeTopicV1 stringified
		// value: The list of all slotxindex to respond by request number
		toRespond map[string][][]responseParams
	}
)

// createAndConnectPeer creates a peer and connects it to the p2p service.
// The peer will respond to the `RPCDataColumnSidecarsByRangeTopicV1` topic.
func createAndConnectPeer(
	t *testing.T,
	p2pService *p2ptest.TestP2P,
	chainService *mock.ChainService,
	dataColumnsSidecarFromSlot map[primitives.Slot][]*ethpb.DataColumnSidecar,
	peerParams peerParams,
	offset int,
) *p2ptest.TestP2P {
	// Create the private key, depending on the offset.
	privateKeyBytes := make([]byte, 32)
	for i := 0; i < 32; i++ {
		privateKeyBytes[i] = byte(offset + i)
	}

	privateKey, err := crypto.UnmarshalSecp256k1PrivateKey(privateKeyBytes)
	require.NoError(t, err)

	// Create the peer.
	peer := p2ptest.NewTestP2P(t, libp2p.Identity(privateKey))

	// Create a call counter.
	countFromRequest := make(map[string]int, len(peerParams.toRespond))

	peer.SetStreamHandler(p2p.RPCDataColumnSidecarsByRangeTopicV1+"/ssz_snappy", func(stream network.Stream) {
		// Decode the request.
		req := new(ethpb.DataColumnSidecarsByRangeRequest)

		err := peer.Encoding().DecodeWithMaxLength(stream, req)
		require.NoError(t, err)

		// Convert the request to a string.
		reqString := req.String()

		// Get the response to send.
		items, ok := peerParams.toRespond[reqString]
		require.Equal(t, true, ok)

		for _, responseParams := range items[countFromRequest[reqString]] {
			// Get data columns sidecars for this slot.
			dataColumnsSidecar, ok := dataColumnsSidecarFromSlot[responseParams.slot]
			require.Equal(t, true, ok)

			// Get the data column sidecar.
			dataColumn := dataColumnsSidecar[responseParams.columnIndex]

			// Alter the data column if needed.
			initialValue0, initialValue1 := dataColumn.DataColumn[0][0], dataColumn.DataColumn[0][1]

			if responseParams.alterate {
				dataColumn.DataColumn[0][0] = 0
				dataColumn.DataColumn[0][1] = 0
			}

			// Send the response.
			err := beaconsync.WriteDataColumnSidecarChunk(stream, chainService, p2pService.Encoding(), dataColumn)
			require.NoError(t, err)

			if responseParams.alterate {
				// Restore the data column.
				dataColumn.DataColumn[0][0] = initialValue0
				dataColumn.DataColumn[0][1] = initialValue1
			}
		}

		// Close the stream.
		err = stream.Close()
		require.NoError(t, err)

		// Increment the call counter.
		countFromRequest[reqString]++
	})

	// Create the record and set the custody count.
	enr := &enr.Record{}
	enr.Set(peerdas.Csc(peerParams.csc))

	// Add the peer and connect it.
	p2pService.Peers().Add(enr, peer.PeerID(), nil, network.DirOutbound)
	p2pService.Peers().SetConnectionState(peer.PeerID(), peers.PeerConnected)
	p2pService.Connect(peer)

	return peer
}

func defaultMockChain(t *testing.T, currentSlot uint64) (*mock.ChainService, *startup.Clock) {
	de := params.BeaconConfig().DenebForkEpoch
	df, err := forks.Fork(de)
	require.NoError(t, err)
	denebBuffer := params.BeaconConfig().MinEpochsForBlobsSidecarsRequest + 1000
	ce := de + denebBuffer
	fe := ce - 2
	cs, err := slots.EpochStart(ce)
	require.NoError(t, err)
	now := time.Now()
	genOffset := primitives.Slot(params.BeaconConfig().SecondsPerSlot) * cs
	genesisTime := now.Add(-1 * time.Second * time.Duration(int64(genOffset)))

	clock := startup.NewClock(genesisTime, [32]byte{}, startup.WithNower(
		func() time.Time {
			return genesisTime.Add(time.Duration(currentSlot*params.BeaconConfig().SecondsPerSlot) * time.Second)
		},
	))

	chain := &mock.ChainService{
		FinalizedCheckPoint: &ethpb.Checkpoint{Epoch: fe},
		Fork:                df,
	}

	return chain, clock
}

func TestFirstLastIndices(t *testing.T) {
	missingColumnsFromRoot := map[[fieldparams.RootLength]byte]map[uint64]bool{
		rootFromUint64(42): {1: true, 3: true, 5: true},
		rootFromUint64(43): {2: true, 4: true, 6: true},
		rootFromUint64(44): {7: true, 8: true, 9: true},
	}

	indicesFromRoot := map[[fieldparams.RootLength]byte][]int{
		rootFromUint64(42): {5, 6, 7},
		rootFromUint64(43): {8, 9},
		rootFromUint64(44): {3, 2, 1},
	}

	const (
		expectedFirst = 1
		expectedLast  = 9
	)

	actualFirst, actualLast := firstLastIndices(missingColumnsFromRoot, indicesFromRoot)

	require.Equal(t, expectedFirst, actualFirst)
	require.Equal(t, expectedLast, actualLast)
}

func TestFetchDataColumnsFromPeers(t *testing.T) {
	const blobsCount = 6

	testCases := []struct {
		// Name of the test case.
		name string

		// INPUTS
		// ------

		// Fork epochs.
		denebForkEpoch   primitives.Epoch
		eip7954ForkEpoch primitives.Epoch

		// Current slot.
		currentSlot uint64

		// Blocks with blobs parameters.
		blocksParams []blockParams

		// - Position in the slice: Stored data columns in the store for the
		//   nth position in the input bwb.
		// - Key                  : Column index
		// - Value                : Always true
		storedDataColumns []map[int]bool

		peersParams []peerParams

		// OUTPUTS
		// -------

		// Data columns that should be added to `bwb`.
		addedRODataColumns [][]int
	}{
		{
			name:           "Deneb fork epoch not reached",
			denebForkEpoch: primitives.Epoch(math.MaxUint64),
			blocksParams: []blockParams{
				{slot: 1, hasBlobs: true},
				{slot: 2, hasBlobs: true},
				{slot: 3, hasBlobs: true},
			},
			addedRODataColumns: [][]int{nil, nil, nil},
		},
		{
			name:             "All blocks are before EIP-7954 fork epoch",
			denebForkEpoch:   0,
			eip7954ForkEpoch: 1,
			currentSlot:      40,
			blocksParams: []blockParams{
				{slot: 25, hasBlobs: false},
				{slot: 26, hasBlobs: false},
				{slot: 27, hasBlobs: false},
				{slot: 28, hasBlobs: false},
			},
			addedRODataColumns: [][]int{nil, nil, nil, nil},
		},
		{
			name:             "All blocks with commitments before are EIP-7954 fork epoch",
			denebForkEpoch:   0,
			eip7954ForkEpoch: 1,
			currentSlot:      40,
			blocksParams: []blockParams{
				{slot: 25, hasBlobs: false},
				{slot: 26, hasBlobs: true},
				{slot: 27, hasBlobs: true},
				{slot: 32, hasBlobs: false},
				{slot: 33, hasBlobs: false},
			},
			addedRODataColumns: [][]int{nil, nil, nil, nil, nil},
		},
		{
			name:             "Some blocks with blobs but without any missing data columns",
			denebForkEpoch:   0,
			eip7954ForkEpoch: 1,
			currentSlot:      40,
			blocksParams: []blockParams{
				{slot: 25, hasBlobs: false},
				{slot: 26, hasBlobs: true},
				{slot: 27, hasBlobs: true},
				{slot: 32, hasBlobs: false},
				{slot: 33, hasBlobs: true},
			},
			storedDataColumns: []map[int]bool{
				nil,
				nil,
				nil,
				nil,
				{6: true, 38: true, 70: true, 102: true},
			},
			addedRODataColumns: [][]int{nil, nil, nil, nil, nil},
		},
		{
			name:             "Some blocks with blobs with missing data columns - one round needed",
			denebForkEpoch:   0,
			eip7954ForkEpoch: 1,
			currentSlot:      40,
			blocksParams: []blockParams{
				{slot: 25, hasBlobs: false},
				{slot: 27, hasBlobs: true},
				{slot: 32, hasBlobs: false},
				{slot: 33, hasBlobs: true},
				{slot: 34, hasBlobs: true},
				{slot: 35, hasBlobs: false},
				{slot: 36, hasBlobs: true},
				{slot: 37, hasBlobs: true},
				{slot: 38, hasBlobs: true},
				{slot: 39, hasBlobs: false},
			},
			storedDataColumns: []map[int]bool{
				nil,
				nil,
				nil,
				{6: true, 38: true, 70: true, 102: true},
				{6: true, 70: true},
				nil,
				{6: true, 38: true, 70: true, 102: true},
				{38: true, 102: true},
				{6: true, 38: true, 70: true, 102: true},
				nil,
			},
			peersParams: []peerParams{
				{
					csc: 128,
					toRespond: map[string][][]responseParams{
						(&ethpb.DataColumnSidecarsByRangeRequest{
							StartSlot: 34,
							Count:     4,
							Columns:   []uint64{6, 38, 70, 102},
						}).String(): {
							{
								{slot: 34, columnIndex: 6},
								{slot: 34, columnIndex: 38},
								{slot: 34, columnIndex: 70},
								{slot: 34, columnIndex: 102},
								{slot: 36, columnIndex: 6},
								{slot: 36, columnIndex: 38},
								{slot: 36, columnIndex: 70},
								{slot: 36, columnIndex: 102},
								{slot: 37, columnIndex: 6},
								{slot: 37, columnIndex: 38},
								{slot: 37, columnIndex: 70},
								{slot: 37, columnIndex: 102},
							},
						},
					},
				},
				{
					csc: 128,
					toRespond: map[string][][]responseParams{
						(&ethpb.DataColumnSidecarsByRangeRequest{
							StartSlot: 34,
							Count:     4,
							Columns:   []uint64{6, 38, 70, 102},
						}).String(): {
							{
								{slot: 34, columnIndex: 6},
								{slot: 34, columnIndex: 38},
								{slot: 34, columnIndex: 70},
								{slot: 34, columnIndex: 102},
								{slot: 36, columnIndex: 6},
								{slot: 36, columnIndex: 38},
								{slot: 36, columnIndex: 70},
								{slot: 36, columnIndex: 102},
								{slot: 37, columnIndex: 6},
								{slot: 37, columnIndex: 38},
								{slot: 37, columnIndex: 70},
								{slot: 37, columnIndex: 102},
							},
						},
					},
				},
				{
					csc: 128,
					toRespond: map[string][][]responseParams{
						(&ethpb.DataColumnSidecarsByRangeRequest{
							StartSlot: 34,
							Count:     4,
							Columns:   []uint64{6, 38, 70, 102},
						}).String(): {
							{},
						},
					},
				},
				{
					csc: 128,
					toRespond: map[string][][]responseParams{
						(&ethpb.DataColumnSidecarsByRangeRequest{
							StartSlot: 34,
							Count:     4,
							Columns:   []uint64{6, 38, 70, 102},
						}).String(): {
							{},
						},
					},
				},
			},
			addedRODataColumns: [][]int{
				nil,
				nil,
				nil,
				nil,
				{38, 102},
				nil,
				nil,
				{6, 70},
				nil,
				nil,
			},
		},
		{
			name:             "Some blocks with blobs with missing data columns - several rounds needed",
			denebForkEpoch:   0,
			eip7954ForkEpoch: 1,
			currentSlot:      40,
			blocksParams: []blockParams{
				{slot: 25, hasBlobs: false},
				{slot: 27, hasBlobs: true},
				{slot: 32, hasBlobs: false},
				{slot: 33, hasBlobs: true},
				{slot: 34, hasBlobs: true},
				{slot: 35, hasBlobs: false},
				{slot: 37, hasBlobs: true},
				{slot: 38, hasBlobs: true},
				{slot: 39, hasBlobs: false},
			},
			storedDataColumns: []map[int]bool{
				nil,
				nil,
				nil,
				{6: true, 38: true, 70: true, 102: true},
				{6: true, 70: true},
				nil,
				{38: true, 102: true},
				{6: true, 38: true, 70: true, 102: true},
				nil,
			},
			peersParams: []peerParams{
				{
					csc: 128,
					toRespond: map[string][][]responseParams{
						(&ethpb.DataColumnSidecarsByRangeRequest{
							StartSlot: 34,
							Count:     4,
							Columns:   []uint64{6, 38, 70, 102},
						}).String(): {
							{
								{slot: 34, columnIndex: 38},
							},
						},
						(&ethpb.DataColumnSidecarsByRangeRequest{
							StartSlot: 34,
							Count:     4,
							Columns:   []uint64{6, 70, 102},
						}).String(): {
							{
								{slot: 34, columnIndex: 102},
							},
						},
						(&ethpb.DataColumnSidecarsByRangeRequest{
							StartSlot: 37,
							Count:     1,
							Columns:   []uint64{6, 70},
						}).String(): {
							{
								{slot: 37, columnIndex: 6},
								{slot: 37, columnIndex: 70},
							},
						},
					},
				},
				{csc: 0},
				{csc: 0},
			},
			addedRODataColumns: [][]int{
				nil,
				nil,
				nil,
				nil,
				{38, 102},
				nil,
				{6, 70},
				nil,
				nil,
			},
		},
		{
			name:             "Some blocks with blobs with missing data columns - no peers response at first",
			denebForkEpoch:   0,
			eip7954ForkEpoch: 1,
			currentSlot:      40,
			blocksParams: []blockParams{
				{slot: 38, hasBlobs: true},
			},
			storedDataColumns: []map[int]bool{
				{38: true, 102: true},
			},
			peersParams: []peerParams{
				{
					csc: 128,
					toRespond: map[string][][]responseParams{
						(&ethpb.DataColumnSidecarsByRangeRequest{
							StartSlot: 38,
							Count:     1,
							Columns:   []uint64{6, 70},
						}).String(): {
							nil,
							{
								{slot: 38, columnIndex: 6},
								{slot: 38, columnIndex: 70},
							},
						},
					},
				},
				{
					csc: 128,
					toRespond: map[string][][]responseParams{
						(&ethpb.DataColumnSidecarsByRangeRequest{
							StartSlot: 38,
							Count:     1,
							Columns:   []uint64{6, 70},
						}).String(): {
							nil,
							{
								{slot: 38, columnIndex: 6},
								{slot: 38, columnIndex: 70},
							},
						},
					},
				},
			},
			addedRODataColumns: [][]int{
				{6, 70},
			},
		},
		{
			name:             "Some blocks with blobs with missing data columns - first response is invalid",
			denebForkEpoch:   0,
			eip7954ForkEpoch: 1,
			currentSlot:      40,
			blocksParams: []blockParams{
				{slot: 38, hasBlobs: true},
			},
			storedDataColumns: []map[int]bool{
				{38: true, 102: true},
			},
			peersParams: []peerParams{
				{
					csc: 128,
					toRespond: map[string][][]responseParams{
						(&ethpb.DataColumnSidecarsByRangeRequest{
							StartSlot: 38,
							Count:     1,
							Columns:   []uint64{6, 70},
						}).String(): {
							{
								{slot: 38, columnIndex: 6, alterate: true},
								{slot: 38, columnIndex: 70},
							},
						},
						(&ethpb.DataColumnSidecarsByRangeRequest{
							StartSlot: 38,
							Count:     1,
							Columns:   []uint64{6},
						}).String(): {
							{
								{slot: 38, columnIndex: 6},
							},
						},
					},
				},
			},
			addedRODataColumns: [][]int{
				{70, 6},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Consistency checks.
			require.Equal(t, len(tc.blocksParams), len(tc.addedRODataColumns))

			// Create a context.
			ctx := context.Background()

			// Initialize the trusted setup.
			err := kzg.Start()
			require.NoError(t, err)

			// Create blocks, RO data columns and data columns sidecar from slot.
			roBlocks := make([]blocks.ROBlock, len(tc.blocksParams))
			roDatasColumns := make([][]blocks.RODataColumn, len(tc.blocksParams))
			dataColumnsSidecarFromSlot := make(map[primitives.Slot][]*ethpb.DataColumnSidecar, len(tc.blocksParams))

			for i, blockParams := range tc.blocksParams {
				pbSignedBeaconBlock := util.NewBeaconBlockDeneb()
				pbSignedBeaconBlock.Block.Slot = blockParams.slot

				if blockParams.hasBlobs {
					blobs := make([]kzg.Blob, blobsCount)
					blobKzgCommitments := make([][]byte, blobsCount)

					for j := range blobsCount {
						blob := getRandBlob(t, int64(i+j))
						blobs[j] = blob

						blobKzgCommitment, err := kzg.BlobToKZGCommitment(&blob)
						require.NoError(t, err)

						blobKzgCommitments[j] = blobKzgCommitment[:]
					}

					pbSignedBeaconBlock.Block.Body.BlobKzgCommitments = blobKzgCommitments
					signedBeaconBlock, err := blocks.NewSignedBeaconBlock(pbSignedBeaconBlock)
					require.NoError(t, err)

					pbDataColumnsSidecar, err := peerdas.DataColumnSidecars(signedBeaconBlock, blobs)
					require.NoError(t, err)

					dataColumnsSidecarFromSlot[blockParams.slot] = pbDataColumnsSidecar

					roDataColumns := make([]blocks.RODataColumn, 0, len(pbDataColumnsSidecar))
					for _, pbDataColumnSidecar := range pbDataColumnsSidecar {
						roDataColumn, err := blocks.NewRODataColumn(pbDataColumnSidecar)
						require.NoError(t, err)

						roDataColumns = append(roDataColumns, roDataColumn)
					}

					roDatasColumns[i] = roDataColumns
				}

				signedBeaconBlock, err := blocks.NewSignedBeaconBlock(pbSignedBeaconBlock)
				require.NoError(t, err)

				roBlock, err := blocks.NewROBlock(signedBeaconBlock)
				require.NoError(t, err)

				roBlocks[i] = roBlock
			}

			// Set the Deneb fork epoch.
			params.BeaconConfig().DenebForkEpoch = tc.denebForkEpoch

			// Set the EIP-7594 fork epoch.
			params.BeaconConfig().Eip7594ForkEpoch = tc.eip7954ForkEpoch

			// Save the blocks in the store.
			storage := make(map[[fieldparams.RootLength]byte][]int)
			for index, columns := range tc.storedDataColumns {
				root := roBlocks[index].Root()

				columnsSlice := make([]int, 0, len(columns))
				for column := range columns {
					columnsSlice = append(columnsSlice, column)
				}

				storage[root] = columnsSlice
			}

			blobStorageSummarizer := filesystem.NewMockBlobStorageSummarizer(t, storage)

			// Create a chain and a clock.
			chain, clock := defaultMockChain(t, tc.currentSlot)

			// Create the P2P service.
			p2pSvc := p2ptest.NewTestP2P(t, libp2p.Identity(genFixedCustodyPeer(t)))
			nodeID, err := p2p.ConvertPeerIDToNodeID(p2pSvc.PeerID())
			require.NoError(t, err)
			p2pSvc.EnodeID = nodeID

			// Connect the peers.
			peers := make([]*p2ptest.TestP2P, 0, len(tc.peersParams))
			for i, peerParams := range tc.peersParams {
				peer := createAndConnectPeer(t, p2pSvc, chain, dataColumnsSidecarFromSlot, peerParams, i)
				peers = append(peers, peer)
			}

			peersID := make([]peer.ID, 0, len(peers))
			for _, peer := range peers {
				peerID := peer.PeerID()
				peersID = append(peersID, peerID)
			}

			// Create `bwb`.
			bwb := make([]blocks.BlockWithROBlobs, 0, len(tc.blocksParams))
			for _, roBlock := range roBlocks {
				bwb = append(bwb, blocks.BlockWithROBlobs{Block: roBlock})
			}
			clockSync := startup.NewClockSynchronizer()
			require.NoError(t, clockSync.SetClock(clock))
			iniWaiter := verification.NewInitializerWaiter(clockSync, nil, nil)
			ini, err := iniWaiter.WaitForInitializer(ctx)
			require.NoError(t, err)

			// Create the block fetcher.
			blocksFetcher := newBlocksFetcher(ctx, &blocksFetcherConfig{
				clock:  clock,
				ctxMap: map[[4]byte]int{{245, 165, 253, 66}: version.Deneb},
				p2p:    p2pSvc,
				bs:     blobStorageSummarizer,
				cv:     newColumnVerifierFromInitializer(ini),
			})

			// Fetch the data columns from the peers.
			err = blocksFetcher.fetchDataColumnsFromPeers(ctx, bwb, peersID)
			require.NoError(t, err)

			// Check the added RO data columns.
			for i := range bwb {
				blockWithROBlobs := bwb[i]
				addedRODataColumns := tc.addedRODataColumns[i]

				if addedRODataColumns == nil {
					require.Equal(t, 0, len(blockWithROBlobs.Columns))
					continue
				}

				expectedRODataColumns := make([]blocks.RODataColumn, 0, len(tc.addedRODataColumns[i]))
				for _, column := range addedRODataColumns {
					roDataColumn := roDatasColumns[i][column]
					expectedRODataColumns = append(expectedRODataColumns, roDataColumn)
				}

				actualRODataColumns := blockWithROBlobs.Columns
				require.DeepSSZEqual(t, expectedRODataColumns, actualRODataColumns)
			}
		})
	}
}

// This generates a peer which custodies the columns of 6,38,70 and 102.
func genFixedCustodyPeer(t *testing.T) crypto.PrivKey {
	rawObj, err := hex.DecodeString("58f40e5010e67d07e5fb37c62d6027964de2bef532acf06cf4f1766f5273ae95")
	if err != nil {
		t.Fatal(err)
	}
	pkey, err := crypto.UnmarshalSecp256k1PrivateKey(rawObj)
	require.NoError(t, err)
	return pkey
}
