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

	chainMock "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	testDB "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	pb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

func TestDataColumnSidecarsByRangeRPCHandler(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	beaconConfig := params.BeaconConfig()
	beaconConfig.FuluForkEpoch = 0
	params.OverrideBeaconConfig(beaconConfig)
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

			assert.Equal(t, uint64(1), sidecars[0].Index)
			assert.Equal(t, uint64(2), sidecars[1].Index)
			assert.Equal(t, uint64(3), sidecars[2].Index)
			assert.Equal(t, uint64(4), sidecars[3].Index)
			assert.Equal(t, uint64(6), sidecars[4].Index)
			assert.Equal(t, uint64(7), sidecars[5].Index)
			assert.Equal(t, uint64(8), sidecars[6].Index)
			assert.Equal(t, uint64(9), sidecars[7].Index)
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
