package sync

import (
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/execution"
	mockExecution "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations"
	mockp2p "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

func TestService_beaconBlockSubscriber(t *testing.T) {
	pooledAttestations := []*ethpb.Attestation{
		// Aggregated.
		util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b00011111}}),
		// Unaggregated.
		util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b00010001}}),
	}

	type args struct {
		msg proto.Message
	}
	tests := []struct {
		name      string
		args      args
		wantedErr string
		check     func(*testing.T, *Service)
	}{
		{
			name: "invalid block does not remove attestations",
			args: args{
				msg: func() *ethpb.SignedBeaconBlock {
					b := util.NewBeaconBlock()
					b.Block.Body.Attestations = pooledAttestations
					return b
				}(),
			},
			wantedErr: chainMock.ErrNilState.Error(),
			check: func(t *testing.T, s *Service) {
				if s.cfg.attPool.AggregatedAttestationCount() == 0 {
					t.Error("Expected at least 1 aggregated attestation in the pool")
				}
				if s.cfg.attPool.UnaggregatedAttestationCount() == 0 {
					t.Error("Expected at least 1 unaggregated attestation in the pool")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := dbtest.SetupDB(t)
			s := &Service{
				cfg: &config{
					chain: &chainMock.ChainService{
						DB:   db,
						Root: make([]byte, 32),
					},
					attPool:                attestations.NewPool(),
					blobStorage:            filesystem.NewEphemeralBlobStorage(t),
					executionReconstructor: &mockExecution.EngineClient{},
				},
			}
			s.initCaches()
			// Set up attestation pool.
			for _, att := range pooledAttestations {
				if att.IsAggregated() {
					assert.NoError(t, s.cfg.attPool.SaveAggregatedAttestation(att))
				} else {
					assert.NoError(t, s.cfg.attPool.SaveUnaggregatedAttestation(att))
				}
			}
			// Perform method under test call.
			err := s.beaconBlockSubscriber(t.Context(), tt.args.msg)
			if tt.wantedErr != "" {
				assert.ErrorContains(t, tt.wantedErr, err)
			} else {
				assert.NoError(t, err)
			}
			if tt.check != nil {
				tt.check(t, s)
			}
		})
	}
}

func TestService_BeaconBlockSubscribe_ExecutionEngineTimesOut(t *testing.T) {
	s := &Service{
		cfg: &config{
			chain: &chainMock.ChainService{
				ReceiveBlockMockErr: execution.ErrHTTPTimeout,
			},
		},
		seenBlockCache: lruwrpr.New(10),
		badBlockCache:  lruwrpr.New(10),
	}
	require.ErrorIs(t, execution.ErrHTTPTimeout, s.beaconBlockSubscriber(t.Context(), util.NewBeaconBlock()))
	require.Equal(t, 0, len(s.badBlockCache.Keys()))
	require.Equal(t, 1, len(s.seenBlockCache.Keys()))
}

func TestService_BeaconBlockSubscribe_UndefinedEeError(t *testing.T) {
	msg := "timeout"
	err := errors.WithMessage(blockchain.ErrUndefinedExecutionEngineError, msg)

	s := &Service{
		cfg: &config{
			chain: &chainMock.ChainService{
				ReceiveBlockMockErr: err,
			},
		},
		seenBlockCache: lruwrpr.New(10),
		badBlockCache:  lruwrpr.New(10),
	}
	require.ErrorIs(t, s.beaconBlockSubscriber(t.Context(), util.NewBeaconBlock()), blockchain.ErrUndefinedExecutionEngineError)
	require.Equal(t, 0, len(s.badBlockCache.Keys()))
	require.Equal(t, 1, len(s.seenBlockCache.Keys()))
}

func TestProcessSidecarsFromExecutionFromBlock(t *testing.T) {
	t.Run("blobs", func(t *testing.T) {
		rob, err := blocks.NewROBlob(
			&ethpb.BlobSidecar{
				SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
					Header: &ethpb.BeaconBlockHeader{
						ParentRoot: make([]byte, 32),
						BodyRoot:   make([]byte, 32),
						StateRoot:  make([]byte, 32),
					},
					Signature: []byte("signature"),
				},
			})
		require.NoError(t, err)

		chainService := &chainMock.ChainService{
			Genesis: time.Now(),
		}

		b := util.NewBeaconBlockDeneb()
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)

		roBlock, err := blocks.NewROBlock(sb)
		require.NoError(t, err)

		tests := []struct {
			name              string
			blobSidecars      []blocks.VerifiedROBlob
			expectedBlobCount int
		}{
			{
				name:              "Constructed 0 blobs",
				blobSidecars:      nil,
				expectedBlobCount: 0,
			},
			{
				name: "Constructed 6 blobs",
				blobSidecars: []blocks.VerifiedROBlob{
					{ROBlob: rob}, {ROBlob: rob}, {ROBlob: rob}, {ROBlob: rob}, {ROBlob: rob}, {ROBlob: rob},
				},
				expectedBlobCount: 6,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				s := Service{
					cfg: &config{
						p2p:         mockp2p.NewTestP2P(t),
						chain:       chainService,
						clock:       startup.NewClock(time.Now(), [32]byte{}),
						blobStorage: filesystem.NewEphemeralBlobStorage(t),
						executionReconstructor: &mockExecution.EngineClient{
							BlobSidecars: tt.blobSidecars,
						},
						operationNotifier: &chainMock.MockOperationNotifier{},
					},
					seenBlobCache: lruwrpr.New(1),
				}
				err := s.processSidecarsFromExecutionFromBlock(t.Context(), roBlock)
				require.NoError(t, err)
				require.Equal(t, tt.expectedBlobCount, len(chainService.Blobs))
			})
		}
	})

	t.Run("data columns", func(t *testing.T) {
		custodyRequirement := params.BeaconConfig().CustodyRequirement

		// load trusted setup
		err := kzg.Start()
		require.NoError(t, err)

		// Setup right fork epoch
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.CapellaForkEpoch = 0
		cfg.DenebForkEpoch = 0
		cfg.ElectraForkEpoch = 0
		cfg.FuluForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		chainService := &chainMock.ChainService{
			Genesis: time.Now(),
		}

		allColumns := make([]blocks.VerifiedRODataColumn, 128)
		for i := range allColumns {
			rod, err := blocks.NewRODataColumn(
				&ethpb.DataColumnSidecar{
					SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
						Header: &ethpb.BeaconBlockHeader{
							ParentRoot:    make([]byte, 32),
							BodyRoot:      make([]byte, 32),
							StateRoot:     make([]byte, 32),
							ProposerIndex: primitives.ValidatorIndex(123),
							Slot:          primitives.Slot(123),
						},
						Signature: []byte("signature"),
					},
					Index: uint64(i),
				})
			require.NoError(t, err)
			allColumns[i] = blocks.VerifiedRODataColumn{RODataColumn: rod}
		}
		tests := []struct {
			name                    string
			dataColumnSidecars      []blocks.VerifiedRODataColumn
			blobCount               int
			expectedDataColumnCount int
		}{
			{
				name:                    "Constructed 0 data columns with no blobs",
				blobCount:               0,
				dataColumnSidecars:      nil,
				expectedDataColumnCount: 0,
			},
			{
				name:                    "Constructed 128 data columns with all blobs",
				blobCount:               1,
				dataColumnSidecars:      allColumns,
				expectedDataColumnCount: 8,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				s := Service{
					cfg: &config{
						p2p:               mockp2p.NewTestP2P(t),
						chain:             chainService,
						clock:             startup.NewClock(time.Now(), [32]byte{}),
						dataColumnStorage: filesystem.NewEphemeralDataColumnStorage(t),
						executionReconstructor: &mockExecution.EngineClient{
							DataColumnSidecars: tt.dataColumnSidecars,
						},
						operationNotifier: &chainMock.MockOperationNotifier{},
					},
					seenDataColumnCache: newSlotAwareCache(1),
				}

				_, _, err := s.cfg.p2p.UpdateCustodyInfo(0, custodyRequirement)
				require.NoError(t, err)

				kzgCommitments := make([][]byte, 0, tt.blobCount)
				for range tt.blobCount {
					kzgCommitment := make([]byte, 48)
					kzgCommitments = append(kzgCommitments, kzgCommitment)
				}

				b := util.NewBeaconBlockFulu()
				b.Block.Body.BlobKzgCommitments = kzgCommitments

				sb, err := blocks.NewSignedBeaconBlock(b)
				require.NoError(t, err)

				roBlock, err := blocks.NewROBlock(sb)
				require.NoError(t, err)

				err = s.processSidecarsFromExecutionFromBlock(t.Context(), roBlock)
				require.NoError(t, err)
				require.Equal(t, tt.expectedDataColumnCount, len(chainService.DataColumns))
			})
		}
	})

	t.Run("gloas data columns from bid", func(t *testing.T) {
		custodyRequirement := params.BeaconConfig().CustodyRequirement

		err := kzg.Start()
		require.NoError(t, err)

		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.CapellaForkEpoch = 0
		cfg.DenebForkEpoch = 0
		cfg.ElectraForkEpoch = 0
		cfg.FuluForkEpoch = 0
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		chainService := &chainMock.ChainService{
			Genesis: time.Now(),
		}

		allColumns := make([]blocks.VerifiedRODataColumn, 128)
		for i := range allColumns {
			gdc, err := blocks.NewRODataColumnGloas(
				&ethpb.DataColumnSidecarGloas{
					Index:           uint64(i),
					Slot:            primitives.Slot(1),
					BeaconBlockRoot: make([]byte, 32),
				})
			require.NoError(t, err)
			allColumns[i] = blocks.VerifiedRODataColumn{RODataColumn: gdc}
		}

		tests := []struct {
			name                    string
			dataColumnSidecars      []blocks.VerifiedRODataColumn
			blobCount               int
			expectedDataColumnCount int
		}{
			{
				name:                    "Constructed 0 data columns with no blobs",
				blobCount:               0,
				dataColumnSidecars:      nil,
				expectedDataColumnCount: 0,
			},
			{
				name:                    "Constructed 128 data columns with blobs",
				blobCount:               1,
				dataColumnSidecars:      allColumns,
				expectedDataColumnCount: 8,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				s := Service{
					cfg: &config{
						p2p:               mockp2p.NewTestP2P(t),
						chain:             chainService,
						clock:             startup.NewClock(time.Now(), [32]byte{}),
						dataColumnStorage: filesystem.NewEphemeralDataColumnStorage(t),
						executionReconstructor: &mockExecution.EngineClient{
							DataColumnSidecars: tt.dataColumnSidecars,
						},
						operationNotifier: &chainMock.MockOperationNotifier{},
					},
					seenDataColumnCache: newSlotAwareCache(1),
				}

				_, _, err := s.cfg.p2p.UpdateCustodyInfo(0, custodyRequirement)
				require.NoError(t, err)

				kzgCommitments := make([][]byte, 0, tt.blobCount)
				for range tt.blobCount {
					kzgCommitments = append(kzgCommitments, make([]byte, 48))
				}

				b := util.NewBeaconBlockGloas()
				b.Block.Body.SignedExecutionPayloadBid.Message.BlobKzgCommitments = kzgCommitments
				b.Block.Slot = 1

				sb, err := blocks.NewSignedBeaconBlock(b)
				require.NoError(t, err)

				roBlock, err := blocks.NewROBlock(sb)
				require.NoError(t, err)

				err = s.processSidecarsFromExecutionFromBlock(t.Context(), roBlock)
				require.NoError(t, err)
				require.Equal(t, tt.expectedDataColumnCount, len(chainService.DataColumns))
			})
		}
	})
}

func TestHaveAllSidecarsBeenSeen(t *testing.T) {
	const (
		slot          = 42
		proposerIndex = 1664
	)
	gloasRoot := [fieldparams.RootLength]byte{0xaa}
	service := NewService(t.Context(), WithP2P(mockp2p.NewTestP2P(t)))
	service.initCaches()

	// Pre-Gloas sidecars are keyed by (slot, proposer index, index).
	service.setSeenDataColumnIndex(slot, proposerIndex, 1)
	service.setSeenDataColumnIndex(slot, proposerIndex, 3)

	// Gloas sidecars are keyed by (block root, index).
	service.setSeenDataColumnRootIndex(gloasRoot, 1, slot)
	service.setSeenDataColumnRootIndex(gloasRoot, 3, slot)

	testCases := []struct {
		expected bool
		isGloas  bool
		root     [fieldparams.RootLength]byte
		toSample map[uint64]bool
		name     string
	}{
		{
			name:     "fulu all sidecars seen",
			toSample: map[uint64]bool{1: true, 3: true},
			expected: true,
		},
		{
			name:     "fulu not all sidecars seen",
			toSample: map[uint64]bool{1: true, 2: true, 3: true},
			expected: false,
		},
		{
			name:     "gloas all sidecars seen",
			isGloas:  true,
			root:     gloasRoot,
			toSample: map[uint64]bool{1: true, 3: true},
			expected: true,
		},
		{
			name:     "gloas not all sidecars seen",
			isGloas:  true,
			root:     gloasRoot,
			toSample: map[uint64]bool{1: true, 2: true, 3: true},
			expected: false,
		},
		{
			// Regression: Gloas columns must be read via the (root, index) key. Columns marked
			// seen only under the Fulu (slot, proposer, index) key must not satisfy a Gloas check.
			name:     "gloas does not read fulu key",
			isGloas:  true,
			root:     [fieldparams.RootLength]byte{0xbb},
			toSample: map[uint64]bool{1: true, 3: true},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := service.haveAllSidecarsBeenSeen(tc.isGloas, tc.root, slot, proposerIndex, tc.toSample)
			require.Equal(t, tc.expected, actual)
		})
	}
}

// TestBroadcastAndReceiveUnseenDataColumnSidecars_GloasUsesRootKey verifies the unseen filter keys
// Gloas sidecars by (block root, index): a column already seen under that key is skipped, the rest pass.
func TestBroadcastAndReceiveUnseenDataColumnSidecars_GloasUsesRootKey(t *testing.T) {
	chainService := &chainMock.ChainService{}
	s := Service{
		cfg: &config{
			p2p:               mockp2p.NewTestP2P(t),
			chain:             chainService,
			clock:             startup.NewClock(time.Now(), [32]byte{}),
			dataColumnStorage: filesystem.NewEphemeralDataColumnStorage(t),
			operationNotifier: &chainMock.MockOperationNotifier{},
		},
		seenDataColumnCache: newSlotAwareCache(seenDataColumnSize),
	}

	slot := primitives.Slot(7)
	root := [fieldparams.RootLength]byte{0xab}
	needed := map[uint64]bool{0: true, 1: true, 2: true}

	sidecars := make([]blocks.VerifiedRODataColumn, 0, 3)
	for i := range uint64(3) {
		gdc, err := blocks.NewRODataColumnGloasWithRoot(&ethpb.DataColumnSidecarGloas{
			Index:           i,
			Slot:            slot,
			BeaconBlockRoot: root[:],
		}, root)
		require.NoError(t, err)
		sidecars = append(sidecars, blocks.VerifiedRODataColumn{RODataColumn: gdc})
	}

	// Mark index 1 already seen via the Gloas (root, index) key.
	s.setSeenDataColumnRootIndex(root, 1, slot)

	unseen, err := s.broadcastAndReceiveUnseenDataColumnSidecars(t.Context(), slot, 0, needed, sidecars, false)
	require.NoError(t, err)

	require.Equal(t, 2, len(unseen))
	require.Equal(t, true, unseen[0])
	_, seen1 := unseen[1]
	require.Equal(t, false, seen1)
	require.Equal(t, true, unseen[2])
}

// TestProcessDataColumnSidecarsFromExecution_GloasEarlyExitWhenAllSeen verifies that when every
// sampled Gloas column is already marked seen (e.g. delivered via gossip), the EL reconstruction
// path exits early instead of reconstructing and re-broadcasting. Gloas columns are tracked under
// the (block root, index) key, so the early-exit must consult that key, not the Fulu one.
func TestProcessDataColumnSidecarsFromExecution_GloasEarlyExitWhenAllSeen(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.CapellaForkEpoch = 0
	cfg.DenebForkEpoch = 0
	cfg.ElectraForkEpoch = 0
	cfg.FuluForkEpoch = 0
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	chainService := &chainMock.ChainService{Genesis: time.Now()}

	// The EL would return a full set of columns if it were ever consulted.
	elColumns := make([]blocks.VerifiedRODataColumn, fieldparams.NumberOfColumns)
	for i := range elColumns {
		gdc, err := blocks.NewRODataColumnGloas(&ethpb.DataColumnSidecarGloas{
			Index:           uint64(i),
			Slot:            primitives.Slot(1),
			BeaconBlockRoot: make([]byte, 32),
		})
		require.NoError(t, err)
		elColumns[i] = blocks.VerifiedRODataColumn{RODataColumn: gdc}
	}

	s := Service{
		cfg: &config{
			p2p:               mockp2p.NewTestP2P(t),
			chain:             chainService,
			clock:             startup.NewClock(time.Now(), [32]byte{}),
			dataColumnStorage: filesystem.NewEphemeralDataColumnStorage(t),
			executionReconstructor: &mockExecution.EngineClient{
				DataColumnSidecars: elColumns,
			},
			operationNotifier: &chainMock.MockOperationNotifier{},
		},
		seenDataColumnCache: newSlotAwareCache(seenDataColumnSize),
	}

	_, _, err := s.cfg.p2p.UpdateCustodyInfo(0, params.BeaconConfig().CustodyRequirement)
	require.NoError(t, err)

	b := util.NewBeaconBlockGloas()
	b.Block.Body.SignedExecutionPayloadBid.Message.BlobKzgCommitments = [][]byte{make([]byte, 48)}
	b.Block.Slot = 1

	sb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	roBlock, err := blocks.NewROBlock(sb)
	require.NoError(t, err)

	// Simulate every column already received via gossip: mark seen under the Gloas (root, index) key.
	root := roBlock.Root()
	for i := range uint64(fieldparams.NumberOfColumns) {
		s.setSeenDataColumnRootIndex(root, i, primitives.Slot(1))
	}

	require.NoError(t, s.processSidecarsFromExecutionFromBlock(t.Context(), roBlock))

	// Early-exit must skip EL reconstruction entirely, so nothing is reconstructed/received.
	require.Equal(t, 0, len(chainService.DataColumns))
}

func TestColumnIndicesToSample(t *testing.T) {
	const earliestAvailableSlot = 0
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.SamplesPerSlot = 4
	params.OverrideBeaconConfig(cfg)

	testCases := []struct {
		name              string
		custodyGroupCount uint64
		expected          map[uint64]bool
	}{
		{
			name:              "custody group count lower than samples per slot",
			custodyGroupCount: 3,
			expected:          map[uint64]bool{1: true, 17: true, 87: true, 102: true},
		},
		{
			name:              "custody group count higher than samples per slot",
			custodyGroupCount: 5,
			expected:          map[uint64]bool{1: true, 17: true, 75: true, 87: true, 102: true},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p2p := mockp2p.NewTestP2P(t)
			_, _, err := p2p.UpdateCustodyInfo(earliestAvailableSlot, tc.custodyGroupCount)
			require.NoError(t, err)

			service := NewService(t.Context(), WithP2P(p2p))

			actual, err := service.columnIndicesToSample()
			require.NoError(t, err)

			require.Equal(t, len(tc.expected), len(actual))
			for index := range tc.expected {
				require.Equal(t, true, actual[index])
			}
		})
	}
}
