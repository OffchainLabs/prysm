package sync

import (
	"io"
	"math"
	"sync"
	"testing"
	"time"

	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	testDB "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/encoder"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/pkg/errors"
)

func TestDataColumnSidecarsByRootRPCHandler(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.FuluForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	params.BeaconConfig().InitializeForkSchedule()
	ctxMap, err := ContextByteVersionsForValRoot(params.BeaconConfig().GenesisValidatorsRoot)
	require.NoError(t, err)
	ctx := t.Context()

	protocolID := protocol.ID(p2p.RPCDataColumnSidecarsByRootTopicV1) + "/" + encoder.ProtocolSuffixSSZSnappy

	t.Run("wrong message type", func(t *testing.T) {
		service := &Service{}
		err := service.dataColumnSidecarByRootRPCHandler(t.Context(), nil, nil)
		require.ErrorIs(t, err, notDataColumnsByRootIdentifiersError)
	})

	t.Run("invalid request", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig()
		cfg.MaxRequestDataColumnSidecars = 1
		params.OverrideBeaconConfig(cfg)

		localP2P := p2ptest.NewTestP2P(t)
		service := &Service{cfg: &config{p2p: localP2P}, rateLimiter: newRateLimiter(localP2P)}
		remoteP2P := p2ptest.NewTestP2P(t)

		var wg sync.WaitGroup
		wg.Add(1)

		remoteP2P.BHost.SetStreamHandler(protocolID, func(stream network.Stream) {
			defer wg.Done()
			code, errMsg, err := readStatusCodeNoDeadline(stream, localP2P.Encoding())
			require.NoError(t, err)
			require.Equal(t, responseCodeInvalidRequest, code)
			require.Equal(t, types.ErrMaxDataColumnReqExceeded.Error(), errMsg)
		})

		localP2P.Connect(remoteP2P)
		stream, err := localP2P.BHost.NewStream(t.Context(), remoteP2P.BHost.ID(), protocolID)
		require.NoError(t, err)

		msg := types.DataColumnsByRootIdentifiers{{Columns: []uint64{1, 2, 3}}}
		require.Equal(t, true, localP2P.Peers().Scorers().BadResponsesScorer().Score(remoteP2P.PeerID()) >= 0)

		err = service.dataColumnSidecarByRootRPCHandler(t.Context(), msg, stream)
		require.NotNil(t, err)
		require.Equal(t, true, localP2P.Peers().Scorers().BadResponsesScorer().Score(remoteP2P.PeerID()) < 0)

		if util.WaitTimeout(&wg, 1*time.Second) {
			t.Fatal("Did not receive stream within 1 sec")
		}
	})

	t.Run("nominal", func(t *testing.T) {
		// Setting the ticker to 0 will cause the ticker to panic.
		// Setting it to the minimum value instead.
		refTickerDelay := tickerDelay
		tickerDelay = time.Nanosecond
		defer func() {
			tickerDelay = refTickerDelay
		}()

		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig()
		cfg.FuluForkEpoch = 1
		params.OverrideBeaconConfig(cfg)

		localP2P := p2ptest.NewTestP2P(t)
		clock := startup.NewClock(time.Now(), [fieldparams.RootLength]byte{})

		_, verifiedRODataColumns := util.CreateTestVerifiedRoDataColumnSidecars(
			t,
			[]util.DataColumnParam{
				{Slot: 10, Index: 1}, {Slot: 10, Index: 2}, {Slot: 10, Index: 3},
				{Slot: 40, Index: 4}, {Slot: 40, Index: 6},
				{Slot: 45, Index: 7}, {Slot: 45, Index: 8}, {Slot: 45, Index: 9},
				{Slot: 46, Index: 10}, // Corresponding block won't be saved in DB
			},
		)

		dataColumnStorage := filesystem.NewEphemeralDataColumnStorage(t)
		err := dataColumnStorage.Save(verifiedRODataColumns)
		require.NoError(t, err)

		beaconDB := testDB.SetupDB(t)
		indices := [...]int{0, 3, 5}

		roBlocks := make([]blocks.ROBlock, 0, len(indices))
		for _, i := range indices {
			blockPb := util.NewBeaconBlock()

			signedBeaconBlock, err := blocks.NewSignedBeaconBlock(blockPb)
			require.NoError(t, err)

			// Here the block root has to match the sidecar's block root.
			// (However, the block root does not match the actual root of the block, but we don't care for this test.)
			roBlock, err := blocks.NewROBlockWithRoot(signedBeaconBlock, verifiedRODataColumns[i].BlockRoot())
			require.NoError(t, err)

			roBlocks = append(roBlocks, roBlock)
		}

		err = beaconDB.SaveROBlocks(ctx, roBlocks, false /*cache*/)
		require.NoError(t, err)

		service := &Service{
			cfg: &config{
				p2p:               localP2P,
				beaconDB:          beaconDB,
				clock:             clock,
				dataColumnStorage: dataColumnStorage,
				chain:             &chainMock.ChainService{},
			},
			rateLimiter: newRateLimiter(localP2P),
		}

		remoteP2P := p2ptest.NewTestP2P(t)

		var wg sync.WaitGroup
		wg.Add(1)

		root0 := verifiedRODataColumns[0].BlockRoot()
		root3 := verifiedRODataColumns[3].BlockRoot()
		root5 := verifiedRODataColumns[5].BlockRoot()
		root8 := verifiedRODataColumns[8].BlockRoot()

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

			assert.Equal(t, 5, len(sidecars))
			assert.Equal(t, root3, sidecars[0].BlockRoot())
			assert.Equal(t, root3, sidecars[1].BlockRoot())
			assert.Equal(t, root5, sidecars[2].BlockRoot())
			assert.Equal(t, root5, sidecars[3].BlockRoot())
			assert.Equal(t, root5, sidecars[4].BlockRoot())

			assert.Equal(t, uint64(4), sidecars[0].Index)
			assert.Equal(t, uint64(6), sidecars[1].Index)
			assert.Equal(t, uint64(7), sidecars[2].Index)
			assert.Equal(t, uint64(8), sidecars[3].Index)
			assert.Equal(t, uint64(9), sidecars[4].Index)
		})

		localP2P.Connect(remoteP2P)
		stream, err := localP2P.BHost.NewStream(ctx, remoteP2P.BHost.ID(), protocolID)
		require.NoError(t, err)

		msg := types.DataColumnsByRootIdentifiers{
			{
				BlockRoot: root0[:],
				Columns:   []uint64{1, 2, 3},
			},
			{
				BlockRoot: root3[:],
				Columns:   []uint64{4, 5, 6},
			},
			{
				BlockRoot: root5[:],
				Columns:   []uint64{7, 8, 9},
			},
			{
				BlockRoot: root8[:],
				Columns:   []uint64{10},
			},
		}

		err = service.dataColumnSidecarByRootRPCHandler(ctx, msg, stream)
		require.NoError(t, err)
		require.Equal(t, true, localP2P.Peers().Scorers().BadResponsesScorer().Score(remoteP2P.PeerID()) >= 0)

		if util.WaitTimeout(&wg, 1*time.Minute) {
			t.Fatal("Did not receive stream within 1 sec")
		}
	})
}

func TestValidateDataColumnsByRootRequest(t *testing.T) {
	const max = 10

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.MaxRequestDataColumnSidecars = max
	params.OverrideBeaconConfig(cfg)

	t.Run("invalid", func(t *testing.T) {
		err := validateDataColumnsByRootRequest(max + 1)
		require.ErrorIs(t, err, types.ErrMaxDataColumnReqExceeded)
	})

	t.Run("valid", func(t *testing.T) {
		err := validateDataColumnsByRootRequest(max)
		require.NoError(t, err)
	})
}

func TestDataColumnsRPCMinValidSlot(t *testing.T) {
	type testCase struct {
		name          string
		fuluForkEpoch primitives.Epoch
		minReqEpochs  primitives.Epoch
		currentSlot   primitives.Slot
		expected      primitives.Slot
	}

	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	testCases := []testCase{
		{
			name:          "Fulu not enabled",
			fuluForkEpoch: math.MaxUint64, // Disable Fulu
			minReqEpochs:  5,
			currentSlot:   0,
			expected:      primitives.Slot(math.MaxUint64),
		},
		{
			name:          "Current epoch is before fulu fork epoch",
			fuluForkEpoch: 10,
			minReqEpochs:  5,
			currentSlot:   primitives.Slot(8 * slotsPerEpoch),
			expected:      primitives.Slot(10 * slotsPerEpoch),
		},
		{
			name:          "Current epoch is fulu fork epoch",
			fuluForkEpoch: 10,
			minReqEpochs:  5,
			currentSlot:   primitives.Slot(10 * slotsPerEpoch),
			expected:      primitives.Slot(10 * slotsPerEpoch),
		},
		{
			name:          "Current epoch between fulu fork epoch and minReqEpochs",
			fuluForkEpoch: 10,
			minReqEpochs:  20,
			currentSlot:   primitives.Slot(15 * slotsPerEpoch),
			expected:      primitives.Slot(10 * slotsPerEpoch),
		},
		{
			name:          "Current epoch after fulu fork epoch + minReqEpochs",
			fuluForkEpoch: 10,
			minReqEpochs:  5,
			currentSlot:   primitives.Slot(20 * slotsPerEpoch),
			expected:      primitives.Slot(15 * slotsPerEpoch),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			params.SetupTestConfigCleanup(t)
			config := params.BeaconConfig()
			config.FuluForkEpoch = tc.fuluForkEpoch
			config.MinEpochsForDataColumnSidecarsRequest = tc.minReqEpochs
			params.OverrideBeaconConfig(config)

			actual, err := dataColumnsRPCMinValidSlot(tc.currentSlot)
			require.NoError(t, err)
			require.Equal(t, tc.expected, actual)
		})
	}
}
