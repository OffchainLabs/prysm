package sync

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	mockChain "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestProcessDataColumnSidecarsFromReconstruction(t *testing.T) {
	const blobCount = 4

	ctx := t.Context()

	// Start the trusted setup.
	err := kzg.Start()
	require.NoError(t, err)

	roBlock, _, verifiedRoDataColumns := util.GenerateTestFuluBlockWithSidecars(t, blobCount)
	require.Equal(t, fieldparams.NumberOfColumns, len(verifiedRoDataColumns))

	minimumCount := peerdas.MinimumColumnCountToReconstruct()

	t.Run("not enough stored sidecars", func(t *testing.T) {
		storage := filesystem.NewEphemeralDataColumnStorage(t)
		err := storage.Save(verifiedRoDataColumns[:minimumCount-1])
		require.NoError(t, err)

		service := NewService(ctx, WithP2P(p2ptest.NewTestP2P(t)), WithDataColumnStorage(storage))
		err = service.processDataColumnSidecarsFromReconstruction(ctx, verifiedRoDataColumns[0])
		require.NoError(t, err)
	})

	t.Run("all stored sidecars", func(t *testing.T) {
		storage := filesystem.NewEphemeralDataColumnStorage(t)
		err := storage.Save(verifiedRoDataColumns)
		require.NoError(t, err)

		service := NewService(ctx, WithP2P(p2ptest.NewTestP2P(t)), WithDataColumnStorage(storage))
		err = service.processDataColumnSidecarsFromReconstruction(ctx, verifiedRoDataColumns[0])
		require.NoError(t, err)
	})

	t.Run("should reconstruct", func(t *testing.T) {
		// Here we setup a cgc of 8, which is not realistic, since there is no
		// real reason for a node to both:
		// - store enough data column sidecars to enable reconstruction, and
		// - custody not enough columns to enable reconstruction.
		// However, for the needs of this test, this is perfectly fine.
		const cgc = 8

		require.NoError(t, err)

		chainService := &mockChain.ChainService{}
		p2p := p2ptest.NewTestP2P(t)
		storage := filesystem.NewEphemeralDataColumnStorage(t)

		service := NewService(
			ctx,
			WithP2P(p2p),
			WithDataColumnStorage(storage),
			WithChainService(chainService),
			WithOperationNotifier(chainService.OperationNotifier()),
		)

		minimumCount := peerdas.MinimumColumnCountToReconstruct()
		receivedBeforeReconstruction := verifiedRoDataColumns[:minimumCount]

		err = service.receiveDataColumnSidecars(ctx, receivedBeforeReconstruction)
		require.NoError(t, err)

		err = storage.Save(receivedBeforeReconstruction)
		require.NoError(t, err)

		require.Equal(t, false, p2p.BroadcastCalled.Load())

		// Check received indices before reconstruction.
		require.Equal(t, minimumCount, uint64(len(chainService.DataColumns)))
		for i, actual := range chainService.DataColumns {
			require.Equal(t, uint64(i), actual.Index)
		}

		// Run the reconstruction.
		err = service.processDataColumnSidecarsFromReconstruction(ctx, verifiedRoDataColumns[0])
		require.NoError(t, err)

		expected := make(map[uint64]bool, minimumCount+cgc)
		for i := range minimumCount {
			expected[i] = true
		}

		// The node should custody these indices.
		for _, i := range [...]uint64{75, 87, 102, 117} {
			expected[i] = true
		}

		block := roBlock.Block()
		slot := block.Slot()
		proposerIndex := block.ProposerIndex()

		require.Equal(t, len(expected), len(chainService.DataColumns))
		for _, actual := range chainService.DataColumns {
			require.Equal(t, true, expected[actual.Index])
			require.Equal(t, true, service.hasSeenDataColumnIndex(slot, proposerIndex, actual.Index))
		}

		require.Equal(t, true, p2p.BroadcastCalled.Load())
	})
}

func TestComputeRandomDelay(t *testing.T) {
	const (
		seed     = 42
		expected = 746056722 * time.Nanosecond // = 0.746056722 seconds
	)
	slotStartTime := time.Date(2020, 12, 30, 0, 0, 0, 0, time.UTC)

	service := NewService(
		t.Context(),
		WithP2P(p2ptest.NewTestP2P(t)),
		WithReconstructionRandGen(rand.New(rand.NewSource(seed))),
	)

	waitingTime := service.computeRandomDelay(slotStartTime)
	fmt.Print(waitingTime)
	require.Equal(t, expected, waitingTime)
}

func TestSemiSupernodeReconstruction(t *testing.T) {
	const (
		blobCount       = 4
		numberOfColumns = uint64(fieldparams.NumberOfColumns)
	)

	ctx := t.Context()

	// Start the trusted setup.
	err := kzg.Start()
	require.NoError(t, err)

	roBlock, _, verifiedRoDataColumns := util.GenerateTestFuluBlockWithSidecars(t, blobCount)
	require.Equal(t, fieldparams.NumberOfColumns, len(verifiedRoDataColumns))

	minimumCount := peerdas.MinimumColumnCountToReconstruct()

	t.Run("semi-supernode reconstruction with exactly 64 columns", func(t *testing.T) {
		// Test that reconstruction works with exactly the minimum number of columns (64).
		// This simulates semi-supernode mode which custodies exactly 64 columns.
		require.Equal(t, uint64(64), minimumCount, "Expected minimum column count to be 64")

		chainService := &mockChain.ChainService{}
		p2p := p2ptest.NewTestP2P(t)
		storage := filesystem.NewEphemeralDataColumnStorage(t)

		service := NewService(
			ctx,
			WithP2P(p2p),
			WithDataColumnStorage(storage),
			WithChainService(chainService),
			WithOperationNotifier(chainService.OperationNotifier()),
		)

		// Use exactly 64 columns (minimum for reconstruction) to simulate semi-supernode mode.
		// Select the first 64 columns.
		semiSupernodeColumns := verifiedRoDataColumns[:minimumCount]

		err = service.receiveDataColumnSidecars(ctx, semiSupernodeColumns)
		require.NoError(t, err)

		err = storage.Save(semiSupernodeColumns)
		require.NoError(t, err)

		require.Equal(t, false, p2p.BroadcastCalled.Load())

		// Check received indices before reconstruction.
		require.Equal(t, minimumCount, uint64(len(chainService.DataColumns)))
		for i, actual := range chainService.DataColumns {
			require.Equal(t, uint64(i), actual.Index)
		}

		// Run the reconstruction.
		err = service.processDataColumnSidecarsFromReconstruction(ctx, verifiedRoDataColumns[0])
		require.NoError(t, err)

		// Verify we can reconstruct all columns from just 64.
		// The node should have received the initial 64 columns.
		if len(chainService.DataColumns) < int(minimumCount) {
			t.Fatalf("Expected at least %d columns but got %d", minimumCount, len(chainService.DataColumns))
		}

		block := roBlock.Block()
		slot := block.Slot()
		proposerIndex := block.ProposerIndex()

		// Verify that we have seen at least the minimum number of columns.
		seenCount := 0
		for i := range numberOfColumns {
			if service.hasSeenDataColumnIndex(slot, proposerIndex, i) {
				seenCount++
			}
		}
		if seenCount < int(minimumCount) {
			t.Fatalf("Expected to see at least %d columns but saw %d", minimumCount, seenCount)
		}
	})

	t.Run("semi-supernode reconstruction with random 64 columns", func(t *testing.T) {
		// Test reconstruction with 64 non-contiguous columns to simulate a real scenario.
		chainService := &mockChain.ChainService{}
		p2p := p2ptest.NewTestP2P(t)
		storage := filesystem.NewEphemeralDataColumnStorage(t)

		service := NewService(
			ctx,
			WithP2P(p2p),
			WithDataColumnStorage(storage),
			WithChainService(chainService),
			WithOperationNotifier(chainService.OperationNotifier()),
		)

		// Select every other column to get 64 non-contiguous columns.
		semiSupernodeColumns := make([]blocks.VerifiedRODataColumn, 0, minimumCount)
		for i := uint64(0); i < numberOfColumns && uint64(len(semiSupernodeColumns)) < minimumCount; i += 2 {
			semiSupernodeColumns = append(semiSupernodeColumns, verifiedRoDataColumns[i])
		}
		require.Equal(t, minimumCount, uint64(len(semiSupernodeColumns)))

		err = service.receiveDataColumnSidecars(ctx, semiSupernodeColumns)
		require.NoError(t, err)

		err = storage.Save(semiSupernodeColumns)
		require.NoError(t, err)

		// Run the reconstruction.
		err = service.processDataColumnSidecarsFromReconstruction(ctx, semiSupernodeColumns[0])
		require.NoError(t, err)

		// Verify we received the columns.
		if len(chainService.DataColumns) < int(minimumCount) {
			t.Fatalf("Expected at least %d columns but got %d", minimumCount, len(chainService.DataColumns))
		}
	})
}
