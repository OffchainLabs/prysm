package sync

import (
	"context"
	"testing"
	"time"

	blockchaintesting "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	dbtesting "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

// TestDataColumnSubscriber_InvalidMessage tests error handling for invalid messages
func TestDataColumnSubscriber_InvalidMessage(t *testing.T) {
	s := &Service{}

	// Test with invalid message type (use a proto message that's not VerifiedRODataColumn)
	invalidMsg := &ethpb.SignedBeaconBlock{}
	err := s.dataColumnSubscriber(context.Background(), invalidMsg)
	require.ErrorContains(t, "message was not type blocks.VerifiedRODataColumn", err)
}

// TestTriggerGetBlobsV2ForDataColumnSidecar_BlockAvailability tests block availability checking
func TestTriggerGetBlobsV2ForDataColumnSidecar_BlockAvailability(t *testing.T) {
	ctx := context.Background()
	blockRoot := [32]byte{1, 2, 3}

	// Test when block is not available
	t.Run("block not available", func(t *testing.T) {
		mockChain := &blockchaintesting.ChainService{}
		db := dbtesting.SetupDB(t)

		s := &Service{
			cfg: &config{
				chain:    mockChain,
				beaconDB: db,
			},
		}

		err := s.triggerGetBlobsV2ForDataColumnSidecar(ctx, blockRoot)
		require.NoError(t, err)
	})

	// Test when HasBlock returns true but block is not in database
	t.Run("HasBlock true but not in database", func(t *testing.T) {
		mockChain := &blockchaintesting.ChainService{}
		// Mock HasBlock to return true
		mockChain.CanonicalRoots = map[[32]byte]bool{blockRoot: true}

		db := dbtesting.SetupDB(t)

		s := &Service{
			cfg: &config{
				chain:    mockChain,
				beaconDB: db,
			},
		}

		err := s.triggerGetBlobsV2ForDataColumnSidecar(ctx, blockRoot)
		require.NoError(t, err)
	})
}

// TestTriggerGetBlobsV2ForDataColumnSidecar_WithValidBlock tests with a valid block
func TestTriggerGetBlobsV2ForDataColumnSidecar_WithValidBlock(t *testing.T) {
	ctx := context.Background()

	// Create a test block with KZG commitments
	slot := primitives.Slot(100)
	block := util.NewBeaconBlockDeneb()
	block.Block.Slot = slot

	// Add KZG commitments to trigger getBlobsV2 retry logic
	commitment := [48]byte{1, 2, 3}
	block.Block.Body.BlobKzgCommitments = [][]byte{commitment[:]}

	signedBlock, err := blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)

	blockRoot, err := signedBlock.Block().HashTreeRoot()
	require.NoError(t, err)

	t.Run("block with KZG commitments triggers retry", func(t *testing.T) {
		// Mock execution reconstructor to track calls
		mockReconstructor := &MockExecutionReconstructor{
			reconstructCalled: false,
		}

		db := dbtesting.SetupDB(t)

		// Save block to database
		require.NoError(t, db.SaveBlock(ctx, signedBlock))

		mockChain := &blockchaintesting.ChainService{
			DB: db, // Set the DB so HasBlock can find the block
		}

		s := &Service{
			cfg: &config{
				chain:                  mockChain,
				beaconDB:               db,
				executionReconstructor: mockReconstructor,
			},
		}

		err := s.triggerGetBlobsV2ForDataColumnSidecar(ctx, blockRoot)
		require.NoError(t, err)

		// Wait a bit for the goroutine to execute
		time.Sleep(10 * time.Millisecond)

		// Verify that the execution reconstructor was called
		if !mockReconstructor.reconstructCalled {
			t.Errorf("Expected ReconstructDataColumnSidecars to be called")
		}
	})

	t.Run("block without KZG commitments does not trigger retry", func(t *testing.T) {
		// Create block without KZG commitments
		blockNoCommitments := util.NewBeaconBlockDeneb()
		blockNoCommitments.Block.Slot = slot
		blockNoCommitments.Block.Body.BlobKzgCommitments = [][]byte{} // No commitments

		signedBlockNoCommitments, err := blocks.NewSignedBeaconBlock(blockNoCommitments)
		require.NoError(t, err)

		blockRootNoCommitments, err := signedBlockNoCommitments.Block().HashTreeRoot()
		require.NoError(t, err)

		mockReconstructor := &MockExecutionReconstructor{
			reconstructCalled: false,
		}

		db := dbtesting.SetupDB(t)

		// Save block to database
		require.NoError(t, db.SaveBlock(ctx, signedBlockNoCommitments))

		mockChain := &blockchaintesting.ChainService{
			DB: db, // Set the DB so HasBlock can find the block
		}

		s := &Service{
			cfg: &config{
				chain:                  mockChain,
				beaconDB:               db,
				executionReconstructor: mockReconstructor,
			},
		}

		err = s.triggerGetBlobsV2ForDataColumnSidecar(ctx, blockRootNoCommitments)
		require.NoError(t, err)

		// Wait a bit to ensure no goroutine was started
		time.Sleep(10 * time.Millisecond)

		// Verify that the execution reconstructor was NOT called
		if mockReconstructor.reconstructCalled {
			t.Errorf("Expected ReconstructDataColumnSidecars NOT to be called for block without commitments")
		}
	})
}

// MockExecutionReconstructor is a mock implementation for testing
type MockExecutionReconstructor struct {
	reconstructCalled bool
	reconstructError  error
	reconstructResult []blocks.VerifiedRODataColumn
}

func (m *MockExecutionReconstructor) ReconstructFullBlock(ctx context.Context, blindedBlock interfaces.ReadOnlySignedBeaconBlock) (interfaces.SignedBeaconBlock, error) {
	return nil, nil
}

func (m *MockExecutionReconstructor) ReconstructFullBellatrixBlockBatch(ctx context.Context, blindedBlocks []interfaces.ReadOnlySignedBeaconBlock) ([]interfaces.SignedBeaconBlock, error) {
	return nil, nil
}

func (m *MockExecutionReconstructor) ReconstructBlobSidecars(ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock, blockRoot [fieldparams.RootLength]byte, hi func(uint64) bool) ([]blocks.VerifiedROBlob, error) {
	return nil, nil
}

func (m *MockExecutionReconstructor) ReconstructDataColumnSidecars(ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock, blockRoot [fieldparams.RootLength]byte) ([]blocks.VerifiedRODataColumn, error) {
	m.reconstructCalled = true
	return m.reconstructResult, m.reconstructError
}
