package sync

import (
	"context"
	"fmt"
	"io"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	testDB "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestDataColumnSidecarsByRangeRPCHandler(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.FuluForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	params.BeaconConfig().InitializeForkSchedule()
	ctx := context.Background()
	t.Run("wrong message type", func(t *testing.T) {
		service := &Service{}
		err := service.dataColumnSidecarsByRangeRPCHandler(ctx, nil, nil)
		require.ErrorIs(t, err, notDataColumnsByRangeIdentifiersError)
	})
	mockNower := &startup.MockNower{}
	clock := startup.NewClock(time.Now(), params.BeaconConfig().GenesisValidatorsRoot, startup.WithNower(mockNower.Now))

	ctxMap, err := ContextByteVersionsForValRoot(params.BeaconConfig().GenesisValidatorsRoot)
	require.NoError(t, err)

	t.Run("invalid request", func(t *testing.T) {
		slot := primitives.Slot(400)
		mockNower.SetSlot(t, clock, slot)

		localP2P, remoteP2P := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)

		service := &Service{
			cfg: &config{
				p2p: localP2P,
				chain: &chainMock.ChainService{
					Slot: &slot,
				},
				clock: clock,
			},
			rateLimiter: newRateLimiter(localP2P),
		}

		protocolID := protocol.ID(fmt.Sprintf("%s/ssz_snappy", p2p.RPCDataColumnSidecarsByRangeTopicV1))

		var wg sync.WaitGroup
		wg.Add(1)

		remoteP2P.BHost.SetStreamHandler(protocolID, func(stream network.Stream) {
			defer wg.Done()
			code, _, err := readStatusCodeNoDeadline(stream, localP2P.Encoding())
			assert.NoError(t, err)
			assert.Equal(t, responseCodeInvalidRequest, code)
		})

		localP2P.Connect(remoteP2P)
		stream, err := localP2P.BHost.NewStream(ctx, remoteP2P.BHost.ID(), protocolID)
		require.NoError(t, err)

		msg := &pb.DataColumnSidecarsByRangeRequest{
			Count: 0, // Invalid count
		}
		require.Equal(t, true, localP2P.Peers().Scorers().BadResponsesScorer().Score(remoteP2P.PeerID()) >= 0)

		err = service.dataColumnSidecarsByRangeRPCHandler(ctx, msg, stream)
		require.NotNil(t, err)
		require.Equal(t, true, localP2P.Peers().Scorers().BadResponsesScorer().Score(remoteP2P.PeerID()) < 0)

		if util.WaitTimeout(&wg, 1*time.Second) {
			t.Fatal("Did not receive stream within 1 sec")
		}
	})

	t.Run("in the future", func(t *testing.T) {
		slot := primitives.Slot(400)
		mockNower.SetSlot(t, clock, slot)

		localP2P, remoteP2P := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
		protocolID := protocol.ID(fmt.Sprintf("%s/ssz_snappy", p2p.RPCDataColumnSidecarsByRangeTopicV1))

		service := &Service{
			cfg: &config{
				p2p: localP2P,
				chain: &chainMock.ChainService{
					Slot: &slot,
				},
				clock: clock,
			},
			rateLimiter: newRateLimiter(localP2P),
		}

		var wg sync.WaitGroup
		wg.Add(1)

		remoteP2P.BHost.SetStreamHandler(protocolID, func(stream network.Stream) {
			defer wg.Done()

			_, err := readChunkedDataColumnSidecar(stream, remoteP2P, ctxMap)
			assert.Equal(t, true, errors.Is(err, io.EOF))
		})

		localP2P.Connect(remoteP2P)
		stream, err := localP2P.BHost.NewStream(ctx, remoteP2P.BHost.ID(), protocolID)
		require.NoError(t, err)

		msg := &pb.DataColumnSidecarsByRangeRequest{
			StartSlot: slot + 1,
			Count:     50,
			Columns:   []uint64{1, 2, 3, 4, 6, 7, 8, 9, 10},
		}

		err = service.dataColumnSidecarsByRangeRPCHandler(ctx, msg, stream)
		require.NoError(t, err)
	})

	t.Run("nominal", func(t *testing.T) {
		slot := primitives.Slot(400)

		params := []util.DataColumnParam{
			{Slot: 10, Index: 1}, {Slot: 10, Index: 2}, {Slot: 10, Index: 3},
			{Slot: 40, Index: 4}, {Slot: 40, Index: 6},
			{Slot: 45, Index: 7}, {Slot: 45, Index: 8}, {Slot: 45, Index: 9},
		}

		_, verifiedRODataColumns := util.CreateTestVerifiedRoDataColumnSidecars(t, params)

		storage := filesystem.NewEphemeralDataColumnStorage(t)
		err = storage.Save(verifiedRODataColumns)
		require.NoError(t, err)

		localP2P, remoteP2P := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
		protocolID := protocol.ID(fmt.Sprintf("%s/ssz_snappy", p2p.RPCDataColumnSidecarsByRangeTopicV1))

		roots := [][fieldparams.RootLength]byte{
			verifiedRODataColumns[0].BlockRoot(),
			verifiedRODataColumns[3].BlockRoot(),
			verifiedRODataColumns[5].BlockRoot(),
		}

		slots := []primitives.Slot{
			verifiedRODataColumns[0].Slot(),
			verifiedRODataColumns[3].Slot(),
			verifiedRODataColumns[5].Slot(),
		}

		beaconDB := testDB.SetupDB(t)
		roBlocks := make([]blocks.ROBlock, 0, len(roots))
		for i := range 3 {
			signedBeaconBlockPb := util.NewBeaconBlock()
			signedBeaconBlockPb.Block.Slot = slots[i]
			if i != 0 {
				signedBeaconBlockPb.Block.ParentRoot = roots[i-1][:]
			}

			signedBeaconBlock, err := blocks.NewSignedBeaconBlock(signedBeaconBlockPb)
			require.NoError(t, err)

			// There is a discrepancy between the root of the beacon block and the rodata column root,
			// but for the sake of this test, we actually don't care.
			roblock, err := blocks.NewROBlockWithRoot(signedBeaconBlock, roots[i])
			require.NoError(t, err)

			roBlocks = append(roBlocks, roblock)
		}

		err = beaconDB.SaveROBlocks(ctx, roBlocks, false /*cache*/)
		require.NoError(t, err)

		mockNower.SetSlot(t, clock, slot)
		service := &Service{
			cfg: &config{
				p2p:               localP2P,
				beaconDB:          beaconDB,
				chain:             &chainMock.ChainService{},
				dataColumnStorage: storage,
				clock:             clock,
			},
			rateLimiter: newRateLimiter(localP2P),
		}

		root0 := verifiedRODataColumns[0].BlockRoot()
		root3 := verifiedRODataColumns[3].BlockRoot()
		root5 := verifiedRODataColumns[5].BlockRoot()

		var wg sync.WaitGroup
		wg.Add(1)

		remoteP2P.BHost.SetStreamHandler(protocolID, func(stream network.Stream) {
			defer wg.Done()

			sidecars := make([]*blocks.RODataColumn, 0, 5)

			for i := uint64(0); ; /* no stop condition */ i++ {
				sidecar, err := readChunkedDataColumnSidecar(stream, remoteP2P, ctxMap)
				if errors.Is(err, io.EOF) {
					// End of stream.
					break
				}

				assert.NoError(t, err)
				sidecars = append(sidecars, sidecar)
			}

			assert.Equal(t, 8, len(sidecars))
			assert.Equal(t, root0, sidecars[0].BlockRoot())
			assert.Equal(t, root0, sidecars[1].BlockRoot())
			assert.Equal(t, root0, sidecars[2].BlockRoot())
			assert.Equal(t, root3, sidecars[3].BlockRoot())
			assert.Equal(t, root3, sidecars[4].BlockRoot())
			assert.Equal(t, root5, sidecars[5].BlockRoot())
			assert.Equal(t, root5, sidecars[6].BlockRoot())
			assert.Equal(t, root5, sidecars[7].BlockRoot())

			assert.Equal(t, uint64(1), sidecars[0].Index())
			assert.Equal(t, uint64(2), sidecars[1].Index())
			assert.Equal(t, uint64(3), sidecars[2].Index())
			assert.Equal(t, uint64(4), sidecars[3].Index())
			assert.Equal(t, uint64(6), sidecars[4].Index())
			assert.Equal(t, uint64(7), sidecars[5].Index())
			assert.Equal(t, uint64(8), sidecars[6].Index())
			assert.Equal(t, uint64(9), sidecars[7].Index())
		})

		localP2P.Connect(remoteP2P)
		stream, err := localP2P.BHost.NewStream(ctx, remoteP2P.BHost.ID(), protocolID)
		require.NoError(t, err)

		msg := &pb.DataColumnSidecarsByRangeRequest{
			StartSlot: 5,
			Count:     50,
			Columns:   []uint64{1, 2, 3, 4, 6, 7, 8, 9, 10},
		}

		err = service.dataColumnSidecarsByRangeRPCHandler(ctx, msg, stream)
		require.NoError(t, err)
	})

	t.Run("gloas skips columns of empty slots", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		gloasCfg := params.BeaconConfig().Copy()
		gloasCfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(gloasCfg)
		params.BeaconConfig().InitializeForkSchedule()

		gloasCtxMap, err := ContextByteVersionsForValRoot(params.BeaconConfig().GenesisValidatorsRoot)
		require.NoError(t, err)

		beaconDB := testDB.SetupDB(t)
		storage := filesystem.NewEphemeralDataColumnStorage(t)

		// Block at slot 30 builds on slot 10's payload, so slot 20 is an empty slot.
		blockSlots := []primitives.Slot{10, 20, 30, 40}
		bidParentHashes := [][32]byte{{0}, {10}, {10}, {30}}
		roots := make([][fieldparams.RootLength]byte, len(blockSlots))
		var prevRoot [32]byte

		for i, sl := range blockSlots {
			blk := util.NewBeaconBlockGloas()
			blk.Block.Slot = sl
			copy(blk.Block.ParentRoot, prevRoot[:])
			bidHash := [32]byte{byte(sl)}
			copy(blk.Block.Body.SignedExecutionPayloadBid.Message.BlockHash, bidHash[:])
			copy(blk.Block.Body.SignedExecutionPayloadBid.Message.ParentBlockHash, bidParentHashes[i][:])
			wsb := util.SaveBlock(t, ctx, beaconDB, blk)
			htr, err := wsb.Block().HashTreeRoot()
			require.NoError(t, err)
			roots[i] = htr
			prevRoot = htr
		}
		// Production always has a saved head root, which LowestRootsAtOrAboveSlot falls back to above the tip.
		require.NoError(t, beaconDB.SaveStateSummary(ctx, &pb.StateSummary{Slot: 40, Root: roots[3][:]}))
		require.NoError(t, beaconDB.SaveHeadBlockRoot(ctx, roots[3]))

		verifiedSidecars := make([]blocks.VerifiedRODataColumn, 0, len(blockSlots))
		for i, sl := range blockSlots {
			sidecar := &pb.DataColumnSidecarGloas{
				Index:           1,
				Slot:            sl,
				BeaconBlockRoot: roots[i][:],
			}
			roSidecar, err := blocks.NewRODataColumnGloas(sidecar)
			require.NoError(t, err)
			verifiedSidecars = append(verifiedSidecars, blocks.NewVerifiedRODataColumn(roSidecar))
		}
		require.NoError(t, storage.Save(verifiedSidecars))

		localP2P, remoteP2P := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
		protocolID := protocol.ID(fmt.Sprintf("%s/ssz_snappy", p2p.RPCDataColumnSidecarsByRangeTopicV1))

		slot := primitives.Slot(400)
		mockNower.SetSlot(t, clock, slot)
		service := &Service{
			cfg: &config{
				p2p:               localP2P,
				beaconDB:          beaconDB,
				chain:             &chainMock.ChainService{},
				dataColumnStorage: storage,
				clock:             clock,
			},
			rateLimiter: newRateLimiter(localP2P),
		}

		localP2P.Connect(remoteP2P)
		msg := &pb.DataColumnSidecarsByRangeRequest{
			StartSlot: 5,
			Count:     50,
			Columns:   []uint64{1},
		}

		requestAndCollect := func(expectedRoots [][fieldparams.RootLength]byte) {
			var wg sync.WaitGroup
			wg.Add(1)

			remoteP2P.BHost.SetStreamHandler(protocolID, func(stream network.Stream) {
				defer wg.Done()

				sidecars := make([]*blocks.RODataColumn, 0, len(expectedRoots))
				for {
					sidecar, err := readChunkedDataColumnSidecar(stream, remoteP2P, gloasCtxMap)
					if errors.Is(err, io.EOF) {
						break
					}
					assert.NoError(t, err)
					sidecars = append(sidecars, sidecar)
				}

				assert.Equal(t, len(expectedRoots), len(sidecars))
				if len(sidecars) == len(expectedRoots) {
					for i, root := range expectedRoots {
						assert.Equal(t, root, sidecars[i].BlockRoot())
					}
				}
			})

			stream, err := localP2P.BHost.NewStream(ctx, remoteP2P.BHost.ID(), protocolID)
			require.NoError(t, err)
			require.NoError(t, service.dataColumnSidecarsByRangeRPCHandler(ctx, msg, stream))

			if util.WaitTimeout(&wg, 2*time.Second) {
				t.Fatal("timed out waiting for remote stream handler")
			}
		}

		// Slot 20 is empty and slot 40 is the tip without a processed envelope, both are withheld.
		requestAndCollect([][fieldparams.RootLength]byte{roots[0], roots[2]})

		// Once the tip envelope is processed, its columns are served.
		env := testSignedEnvelope(40, roots[3][:])
		require.NoError(t, beaconDB.SaveExecutionPayloadEnvelope(ctx, env))
		requestAndCollect([][fieldparams.RootLength]byte{roots[0], roots[2], roots[3]})
	})
}

func TestValidateDataColumnsByRange(t *testing.T) {
	maxUint := primitives.Slot(math.MaxUint64)

	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.FuluForkEpoch = 10
	config.MinEpochsForDataColumnSidecarsRequest = 4096
	params.OverrideBeaconConfig(config)

	tests := []struct {
		name        string
		startSlot   primitives.Slot
		count       uint64
		currentSlot primitives.Slot
		expected    *rangeParams
		expectErr   bool
		errContains string
	}{
		{
			name:        "zero count returns error",
			count:       0,
			expectErr:   true,
			errContains: "invalid request count parameter",
		},
		{
			name:        "overflow in addition returns error",
			startSlot:   maxUint - 5,
			count:       10,
			currentSlot: maxUint,
			expectErr:   true,
			errContains: "overflow start + count -1",
		},
		{
			name:        "start greater than current returns nil",
			startSlot:   150,
			count:       10,
			currentSlot: 100,
			expected:    nil,
			expectErr:   false,
		},
		{
			name:        "end slot greater than min start slot returns nil",
			startSlot:   150,
			count:       100,
			currentSlot: 300,
			expected:    nil,
			expectErr:   false,
		},
		{
			name:        "range within limits",
			startSlot:   350,
			count:       10,
			currentSlot: 400,
			expected:    &rangeParams{start: 350, end: 359, size: 10},
			expectErr:   false,
		},
		{
			name:        "range exceeds limits",
			startSlot:   0,
			count:       10_000,
			currentSlot: 400,
			expected:    &rangeParams{start: 320, end: 400, size: 81},
			expectErr:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := &pb.DataColumnSidecarsByRangeRequest{
				StartSlot: tc.startSlot,
				Count:     tc.count,
			}

			rangeParameters, err := validateDataColumnsByRange(request, tc.currentSlot)
			if tc.expectErr {
				require.ErrorContains(t, err, tc.errContains)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expected, rangeParameters)
		})
	}
}
