package sync

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	chainMock "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	dbtest "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/execution"
	mockExecution "github.com/OffchainLabs/prysm/v6/beacon-chain/execution/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/attestations"
	mockp2p "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	lruwrpr "github.com/OffchainLabs/prysm/v6/cache/lru"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/OffchainLabs/prysm/v6/time"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/go-bitfield"
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
				s.processSidecarsFromExecutionFromBlock(t.Context(), roBlock)
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

				s.processSidecarsFromExecutionFromBlock(t.Context(), roBlock)
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
	service := NewService(t.Context(), WithP2P(mockp2p.NewTestP2P(t)))
	service.initCaches()

	service.setSeenDataColumnIndex(slot, proposerIndex, 1)
	service.setSeenDataColumnIndex(slot, proposerIndex, 3)

	testCases := []struct {
		name     string
		toSample map[uint64]bool
		expected bool
	}{
		{
			name:     "all sidecars seen",
			toSample: map[uint64]bool{1: true, 3: true},
			expected: true,
		},
		{
			name:     "not all sidecars seen",
			toSample: map[uint64]bool{1: true, 2: true, 3: true},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := service.haveAllSidecarsBeenSeen(slot, proposerIndex, tc.toSample)
			require.Equal(t, tc.expected, actual)
		})
	}
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
