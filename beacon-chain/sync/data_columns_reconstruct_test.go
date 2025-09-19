package sync

import (
	"fmt"
	"testing"
	"time"

	"math/rand"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	mockChain "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	p2ptest "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

func TestProcessDataColumnSidecarsFromReconstruction(t *testing.T) {
	const blobCount = 4
	numberOfColumns := params.BeaconConfig().NumberOfColumns

	ctx := t.Context()

	// Start the trusted setup.
	err := kzg.Start()
	require.NoError(t, err)

	roBlock, _, verifiedRoDataColumns := util.GenerateTestFuluBlockWithSidecars(t, blobCount)
	require.Equal(t, numberOfColumns, uint64(len(verifiedRoDataColumns)))

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
