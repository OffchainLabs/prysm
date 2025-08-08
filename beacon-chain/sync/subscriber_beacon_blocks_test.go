package sync

import (
	"context"
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
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
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

func TestReconstructAndBroadcastBlobs(t *testing.T) {
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
				s.processSidecarsFromExecution(context.Background(), sb)
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

		// Create a chain service that returns ErrDataNotAvailable to trigger execution service calls
		chainService := &ChainServiceDataNotAvailable{
			ChainService: &chainMock.ChainService{
				Genesis: time.Now(),
			},
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
				expectedDataColumnCount: 4, // default is 4
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

				s.processSidecarsFromExecution(context.Background(), sb)
				require.Equal(t, tt.expectedDataColumnCount, len(chainService.DataColumns))
			})
		}
	})

}

// TestProcessDataColumnSidecarsFromExecution_DataAvailabilityCheck tests the data availability optimization
func TestProcessDataColumnSidecarsFromExecution_DataAvailabilityCheck(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MinimalSpecConfig())

	ctx := context.Background()

	// Create a test block with KZG commitments
	block := util.NewBeaconBlockDeneb()
	block.Block.Slot = 100
	commitment := [48]byte{1, 2, 3}
	block.Block.Body.BlobKzgCommitments = [][]byte{commitment[:]}

	signedBlock, err := blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)

	t.Run("skips execution call when data is available", func(t *testing.T) {
		mockChain := &MockChainServiceTrackingCalls{
			ChainService:          &chainMock.ChainService{},
			dataAvailable:         true, // Data is available
			availabilityError:     nil,
			isDataAvailableCalled: false,
		}

		mockExecutionClient := &MockExecutionClientTrackingCalls{
			EngineClient:      &mockExecution.EngineClient{},
			reconstructCalled: false,
		}

		s := &Service{
			cfg: &config{
				chain:                  mockChain,
				executionReconstructor: mockExecutionClient,
			},
		}

		// This should call IsDataAvailable and return early without calling execution client
		s.processDataColumnSidecarsFromExecution(ctx, signedBlock)

		// Verify the expected call pattern
		assert.Equal(t, true, mockChain.isDataAvailableCalled, "Expected IsDataAvailable to be called")
		assert.Equal(t, false, mockExecutionClient.reconstructCalled, "Expected execution client NOT to be called when data is available")
	})

	t.Run("returns early when IsDataAvailable returns error", func(t *testing.T) {
		mockChain := &MockChainServiceTrackingCalls{
			ChainService:          &chainMock.ChainService{},
			dataAvailable:         false, // This should be ignored due to error
			availabilityError:     errors.New("test error from IsDataAvailable"),
			isDataAvailableCalled: false,
		}

		mockExecutionClient := &MockExecutionClientTrackingCalls{
			EngineClient:      &mockExecution.EngineClient{},
			reconstructCalled: false,
		}

		s := &Service{
			cfg: &config{
				chain:                  mockChain,
				executionReconstructor: mockExecutionClient,
			},
		}

		// This should call IsDataAvailable, get an error, and return early without calling execution client
		s.processDataColumnSidecarsFromExecution(ctx, signedBlock)

		// Verify the expected call pattern
		assert.Equal(t, true, mockChain.isDataAvailableCalled, "Expected IsDataAvailable to be called")
		assert.Equal(t, false, mockExecutionClient.reconstructCalled, "Expected execution client NOT to be called when IsDataAvailable returns error")
	})

	t.Run("calls execution client when data not available", func(t *testing.T) {
		mockChain := &MockChainServiceTrackingCalls{
			ChainService:          &chainMock.ChainService{},
			dataAvailable:         false, // Data not available
			availabilityError:     nil,
			isDataAvailableCalled: false,
		}

		mockExecutionClient := &MockExecutionClientTrackingCalls{
			EngineClient: &mockExecution.EngineClient{
				DataColumnSidecars: []blocks.VerifiedRODataColumn{}, // Empty response is fine for this test
			},
			reconstructCalled: false,
		}

		s := &Service{
			cfg: &config{
				chain:                  mockChain,
				executionReconstructor: mockExecutionClient,
			},
		}

		// This should call IsDataAvailable, get false, and proceed to call execution client
		s.processDataColumnSidecarsFromExecution(ctx, signedBlock)

		// Verify the expected call pattern
		assert.Equal(t, true, mockChain.isDataAvailableCalled, "Expected IsDataAvailable to be called")
		assert.Equal(t, true, mockExecutionClient.reconstructCalled, "Expected execution client to be called when data is not available")
	})

	t.Run("returns early when block has no KZG commitments", func(t *testing.T) {
		// Create a block without KZG commitments
		blockNoCommitments := util.NewBeaconBlockDeneb()
		blockNoCommitments.Block.Slot = 100
		blockNoCommitments.Block.Body.BlobKzgCommitments = [][]byte{} // No commitments

		signedBlockNoCommitments, err := blocks.NewSignedBeaconBlock(blockNoCommitments)
		require.NoError(t, err)

		mockChain := &MockChainServiceTrackingCalls{
			ChainService:          &chainMock.ChainService{},
			dataAvailable:         false,
			availabilityError:     nil,
			isDataAvailableCalled: false,
		}

		mockExecutionClient := &MockExecutionClientTrackingCalls{
			EngineClient:      &mockExecution.EngineClient{},
			reconstructCalled: false,
		}

		s := &Service{
			cfg: &config{
				chain:                  mockChain,
				executionReconstructor: mockExecutionClient,
			},
		}

		// This should return early before checking data availability or calling execution client
		s.processDataColumnSidecarsFromExecution(ctx, signedBlockNoCommitments)

		// Verify neither method was called since there are no commitments
		assert.Equal(t, false, mockChain.isDataAvailableCalled, "Expected IsDataAvailable NOT to be called when no KZG commitments")
		assert.Equal(t, false, mockExecutionClient.reconstructCalled, "Expected execution client NOT to be called when no KZG commitments")
	})
}

// MockChainServiceTrackingCalls tracks calls to IsDataAvailable for testing
type MockChainServiceTrackingCalls struct {
	isDataAvailableCalled bool
	dataAvailable         bool
	*chainMock.ChainService
	availabilityError error
}

func (m *MockChainServiceTrackingCalls) IsDataAvailable(ctx context.Context, blockRoot [32]byte, signedBlock interfaces.ReadOnlySignedBeaconBlock) error {
	m.isDataAvailableCalled = true
	if m.availabilityError != nil {
		return m.availabilityError
	}
	if !m.dataAvailable {
		return blockchain.ErrDataNotAvailable
	}
	return nil
}

// MockExecutionClientTrackingCalls tracks calls to ReconstructDataColumnSidecars for testing
type MockExecutionClientTrackingCalls struct {
	*mockExecution.EngineClient
	reconstructCalled bool
}

func (m *MockExecutionClientTrackingCalls) ReconstructDataColumnSidecars(ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock, blockRoot [32]byte) ([]blocks.VerifiedRODataColumn, error) {
	m.reconstructCalled = true
	return m.EngineClient.DataColumnSidecars, m.EngineClient.ErrorDataColumnSidecars
}

func (m *MockExecutionClientTrackingCalls) ReconstructFullBlock(ctx context.Context, blindedBlock interfaces.ReadOnlySignedBeaconBlock) (interfaces.SignedBeaconBlock, error) {
	return m.EngineClient.ReconstructFullBlock(ctx, blindedBlock)
}

func (m *MockExecutionClientTrackingCalls) ReconstructFullBellatrixBlockBatch(ctx context.Context, blindedBlocks []interfaces.ReadOnlySignedBeaconBlock) ([]interfaces.SignedBeaconBlock, error) {
	return m.EngineClient.ReconstructFullBellatrixBlockBatch(ctx, blindedBlocks)
}

func (m *MockExecutionClientTrackingCalls) ReconstructBlobSidecars(ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock, blockRoot [32]byte, hasIndex func(uint64) bool) ([]blocks.VerifiedROBlob, error) {
	return m.EngineClient.ReconstructBlobSidecars(ctx, block, blockRoot, hasIndex)
}

// ChainServiceDataNotAvailable wraps ChainService and overrides IsDataAvailable to return ErrDataNotAvailable
type ChainServiceDataNotAvailable struct {
	*chainMock.ChainService
}

func (c *ChainServiceDataNotAvailable) IsDataAvailable(ctx context.Context, blockRoot [32]byte, signedBlock interfaces.ReadOnlySignedBeaconBlock) error {
	return blockchain.ErrDataNotAvailable
}
