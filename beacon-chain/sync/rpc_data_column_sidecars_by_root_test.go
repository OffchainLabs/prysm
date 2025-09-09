package sync

import (
	"context"
	"io"
	"math"
	"sync"
	"testing"
	"time"

	chainMock "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/pkg/errors"
)

func TestDataColumnSidecarsByRootRPCHandler(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	beaconConfig := params.BeaconConfig()
	beaconConfig.FuluForkEpoch = 0
	params.OverrideBeaconConfig(beaconConfig)
	params.BeaconConfig().InitializeForkSchedule()
	ctxMap, err := ContextByteVersionsForValRoot(params.BeaconConfig().GenesisValidatorsRoot)
	require.NoError(t, err)
	ctx := context.Background()
	t.Run("wrong message type", func(t *testing.T) {
		service := &Service{}
		err := service.dataColumnSidecarByRootRPCHandler(t.Context(), nil, nil)
		require.ErrorIs(t, err, notDataColumnsByRootIdentifiersError)
	})

	t.Run("invalid request", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		beaconConfig := params.BeaconConfig()
		beaconConfig.MaxRequestDataColumnSidecars = 1
		params.OverrideBeaconConfig(beaconConfig)

		localP2P := p2ptest.NewTestP2P(t)
		service := &Service{cfg: &config{p2p: localP2P}}

		protocolID := protocol.ID(p2p.RPCDataColumnSidecarsByRootTopicV1)
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
		resetFlags := flags.Get()
		gFlags := new(flags.GlobalFlags)
		gFlags.DataColumnBatchLimit = 2
		flags.Init(gFlags)
		defer flags.Init(resetFlags)

		// Setting the ticker to 0 will cause the ticker to panic.
		// Setting it to the minimum value instead.
		refTickerDelay := tickerDelay
		tickerDelay = time.Nanosecond
		defer func() {
			tickerDelay = refTickerDelay
		}()

		params.SetupTestConfigCleanup(t)
		beaconConfig := params.BeaconConfig()
		beaconConfig.FuluForkEpoch = 1
		params.OverrideBeaconConfig(beaconConfig)

		localP2P := p2ptest.NewTestP2P(t)
		clock := startup.NewClock(time.Now(), [fieldparams.RootLength]byte{})

		params := []util.DataColumnParam{
			{Slot: 10, Index: 1}, {Slot: 10, Index: 2}, {Slot: 10, Index: 3},
			{Slot: 40, Index: 4}, {Slot: 40, Index: 6},
			{Slot: 45, Index: 7}, {Slot: 45, Index: 8}, {Slot: 45, Index: 9},
		}

		_, verifiedRODataColumns := util.CreateTestVerifiedRoDataColumnSidecars(t, params)

		storage := filesystem.NewEphemeralDataColumnStorage(t)
		err := storage.Save(verifiedRODataColumns)
		require.NoError(t, err)

		service := &Service{
			cfg: &config{
				p2p:               localP2P,
				clock:             clock,
				dataColumnStorage: storage,
				chain:             &chainMock.ChainService{},
			},
			rateLimiter: newRateLimiter(localP2P),
		}

		protocolID := protocol.ID(p2p.RPCDataColumnSidecarsByRootTopicV1)
		remoteP2P := p2ptest.NewTestP2P(t)

		var wg sync.WaitGroup
		wg.Add(1)

		root0 := verifiedRODataColumns[0].BlockRoot()
		root3 := verifiedRODataColumns[3].BlockRoot()
		root5 := verifiedRODataColumns[5].BlockRoot()

		remoteP2P.BHost.SetStreamHandler(protocolID, func(stream network.Stream) {
			defer wg.Done()

			sidecars := make([]*blocks.RODataColumn, 0, 5)

			for i := uint64(0); ; /* no stop condition */ i++ {
				sidecar, err := readChunkedDataColumnSidecar(stream, remoteP2P, ctxMap)
				if errors.Is(err, io.EOF) {
					// End of stream.
					break
				}

				require.NoError(t, err)
				sidecars = append(sidecars, sidecar)
			}

			require.Equal(t, 5, len(sidecars))
			require.Equal(t, root3, sidecars[0].BlockRoot())
			require.Equal(t, root3, sidecars[1].BlockRoot())
			require.Equal(t, root5, sidecars[2].BlockRoot())
			require.Equal(t, root5, sidecars[3].BlockRoot())
			require.Equal(t, root5, sidecars[4].BlockRoot())

			require.Equal(t, uint64(4), sidecars[0].Index)
			require.Equal(t, uint64(6), sidecars[1].Index)
			require.Equal(t, uint64(7), sidecars[2].Index)
			require.Equal(t, uint64(8), sidecars[3].Index)
			require.Equal(t, uint64(9), sidecars[4].Index)
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
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	maxCols := uint64(10) // Set a small value for testing
	config.MaxRequestDataColumnSidecars = maxCols
	params.OverrideBeaconConfig(config)

	tests := []struct {
		name        string
		colIdents   types.DataColumnsByRootIdentifiers
		expectedErr error
	}{
		{
			name: "Invalid request - multiple identifiers exceed max",
			colIdents: types.DataColumnsByRootIdentifiers{
				{
					BlockRoot: make([]byte, fieldparams.RootLength),
					Columns:   make([]uint64, maxCols/2+1),
				},
				{
					BlockRoot: make([]byte, fieldparams.RootLength),
					Columns:   make([]uint64, maxCols/2+1),
				},
			},
			expectedErr: types.ErrMaxDataColumnReqExceeded,
		},
		{
			name: "Valid request - less than max",
			colIdents: types.DataColumnsByRootIdentifiers{
				{
					BlockRoot: make([]byte, fieldparams.RootLength),
					Columns:   make([]uint64, maxCols-1),
				},
			},
			expectedErr: nil,
		},
		{
			name: "Valid request - multiple identifiers sum to max",
			colIdents: types.DataColumnsByRootIdentifiers{
				{
					BlockRoot: make([]byte, fieldparams.RootLength),
					Columns:   make([]uint64, maxCols/2),
				},
				{
					BlockRoot: make([]byte, fieldparams.RootLength),
					Columns:   make([]uint64, maxCols/2),
				},
			},
			expectedErr: nil,
		},
	}

	// Run tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDataColumnsByRootRequest(tt.colIdents)
			if tt.expectedErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.expectedErr)
			}
		})
	}
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
