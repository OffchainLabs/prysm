package sync

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	mockChain "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	p2ptest "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

func TestReconstructDataColumns(t *testing.T) {
	const blobCount = 4
	numberOfColumns := params.BeaconConfig().NumberOfColumns

	ctx := t.Context()

	// Start the trusted setup.
	err := kzg.Start()
	require.NoError(t, err)

	roBlock, _, verifiedRoDataColumns := util.GenerateTestFuluBlockWithSidecars(t, blobCount)
	require.Equal(t, numberOfColumns, uint64(len(verifiedRoDataColumns)))

	root, block := roBlock.Root(), roBlock.Block()
	slot, proposerIndex := block.Slot(), block.ProposerIndex()

	minimumCount := peerdas.MinimumColumnsCountToReconstruct()

	t.Run("not enough stored sidecars", func(t *testing.T) {
		storage := filesystem.NewEphemeralDataColumnStorage(t)
		err := storage.Save(verifiedRoDataColumns[:minimumCount-1])
		require.NoError(t, err)

		service := NewService(ctx, WithP2P(p2ptest.NewTestP2P(t)), WithDataColumnStorage(storage))
		err = service.reconstructSaveBroadcastDataColumnSidecars(ctx, slot, proposerIndex, root)
		require.NoError(t, err)
	})

	t.Run("all stored sidecars", func(t *testing.T) {
		storage := filesystem.NewEphemeralDataColumnStorage(t)
		err := storage.Save(verifiedRoDataColumns)
		require.NoError(t, err)

		service := NewService(ctx, WithP2P(p2ptest.NewTestP2P(t)), WithDataColumnStorage(storage))
		err = service.reconstructSaveBroadcastDataColumnSidecars(ctx, slot, proposerIndex, root)
		require.NoError(t, err)
	})

	t.Run("should reconstruct", func(t *testing.T) {
		// Here we setup a cgc of 8, which is not realistic, since there is no
		// real reason for a node to both:
		// - store enough data column sidecars to enable reconstruction, and
		// - custody not enough columns to enable reconstruction.
		// However, for the needs of this test, this is perfectly fine.
		const cgc = 8

		storage := filesystem.NewEphemeralDataColumnStorage(t)
		minimumCount := peerdas.MinimumColumnsCountToReconstruct()
		err := storage.Save(verifiedRoDataColumns[:minimumCount])
		require.NoError(t, err)

		custodyInfo := &peerdas.CustodyInfo{}
		custodyInfo.TargetGroupCount.SetValidatorsCustodyRequirement(cgc)
		custodyInfo.ToAdvertiseGroupCount.Set(cgc)

		service := NewService(
			ctx,
			WithP2P(p2ptest.NewTestP2P(t)),
			WithDataColumnStorage(storage),
			WithCustodyInfo(custodyInfo),
			WithChainService(&mockChain.ChainService{}),
		)

		err = service.reconstructSaveBroadcastDataColumnSidecars(ctx, slot, proposerIndex, root)
		require.NoError(t, err)

		expected := make(map[uint64]bool, minimumCount+cgc)
		for i := range minimumCount {
			expected[i] = true
		}

		// The node should custody these indices.
		for _, i := range [...]uint64{1, 17, 19, 42, 75, 87, 102, 117} {
			expected[i] = true
		}

		summary := storage.Summary(root)
		actual := summary.Stored()

		require.Equal(t, len(expected), len(actual))
		for index := range expected {
			require.Equal(t, true, actual[index])
		}
	})
}

func TestBroadcastMissingDataColumnSidecars(t *testing.T) {
	const (
		cgc          = 8
		blobCount    = 4
		timeIntoSlot = 0
	)

	numberOfColumns := params.BeaconConfig().NumberOfColumns
	ctx := t.Context()

	// Start the trusted setup.
	err := kzg.Start()
	require.NoError(t, err)

	roBlock, _, verifiedRoDataColumns := util.GenerateTestFuluBlockWithSidecars(t, blobCount)
	require.Equal(t, numberOfColumns, uint64(len(verifiedRoDataColumns)))

	root, block := roBlock.Root(), roBlock.Block()
	slot, proposerIndex := block.Slot(), block.ProposerIndex()

	t.Run("no missing sidecars", func(t *testing.T) {
		custodyInfo := &peerdas.CustodyInfo{}
		custodyInfo.TargetGroupCount.SetValidatorsCustodyRequirement(cgc)
		custodyInfo.ToAdvertiseGroupCount.Set(cgc)

		service := NewService(
			ctx,
			WithP2P(p2ptest.NewTestP2P(t)),
			WithCustodyInfo(custodyInfo),
		)

		for _, index := range [...]uint64{1, 17, 19, 42, 75, 87, 102, 117} {
			key := computeCacheKey(slot, proposerIndex, index)
			service.seenDataColumnCache.Add(key, true)
		}

		err := service.broadcastMissingDataColumnSidecars(slot, proposerIndex, root, timeIntoSlot)
		require.NoError(t, err)
	})

	t.Run("some missing sidecars", func(t *testing.T) {
		custodyInfo := &peerdas.CustodyInfo{}
		custodyInfo.TargetGroupCount.SetValidatorsCustodyRequirement(cgc)
		custodyInfo.ToAdvertiseGroupCount.Set(cgc)

		toSave := make([]blocks.VerifiedRODataColumn, 0, 2)
		for _, index := range [...]uint64{42, 87} {
			toSave = append(toSave, verifiedRoDataColumns[index])
		}

		p2p := p2ptest.NewTestP2P(t)
		storage := filesystem.NewEphemeralDataColumnStorage(t)
		err := storage.Save(toSave)
		require.NoError(t, err)

		service := NewService(
			ctx,
			WithP2P(p2p),
			WithCustodyInfo(custodyInfo),
			WithDataColumnStorage(storage),
		)

		for _, index := range [...]uint64{1, 17, 19, 102, 117} { // 42, 75 and 87 are missing
			key := computeCacheKey(slot, proposerIndex, index)
			service.seenDataColumnCache.Add(key, true)
		}

		for _, index := range [...]uint64{42, 75, 87} {
			seen := service.hasSeenDataColumnIndex(slot, proposerIndex, index)
			require.Equal(t, false, seen)
		}

		require.Equal(t, false, p2p.BroadcastCalled.Load())

		err = service.broadcastMissingDataColumnSidecars(slot, proposerIndex, root, timeIntoSlot)
		require.NoError(t, err)

		seen := service.hasSeenDataColumnIndex(slot, proposerIndex, 75)
		require.Equal(t, false, seen)

		for _, index := range [...]uint64{42, 87} {
			seen := service.hasSeenDataColumnIndex(slot, proposerIndex, index)
			require.Equal(t, true, seen)
		}

		require.Equal(t, true, p2p.BroadcastCalled.Load())

	})
}
