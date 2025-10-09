package peerdas_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

func TestDataColumnsAlignWithBlock(t *testing.T) {
	// Start the trusted setup.
	err := kzg.Start()
	require.NoError(t, err)

	params.BeaconConfig().FuluForkEpoch = params.BeaconConfig().ElectraForkEpoch + 4096*2
	fs := util.SlotAtEpoch(t, params.BeaconConfig().ElectraForkEpoch)
	require.NoError(t, err)
	fuluMax := params.BeaconConfig().MaxBlobsPerBlock(fs)
	t.Run("pre fulu", func(t *testing.T) {
		block, _ := util.GenerateTestElectraBlockWithSidecar(t, [fieldparams.RootLength]byte{}, fs, 0)
		err := peerdas.DataColumnsAlignWithBlock(block, nil)
		require.NoError(t, err)
	})

	t.Run("too many commitments", func(t *testing.T) {
		block, _, _ := util.GenerateTestFuluBlockWithSidecars(t, fuluMax+1, util.WithSlot(fs))
		err := peerdas.DataColumnsAlignWithBlock(block, nil)
		require.ErrorIs(t, err, peerdas.ErrTooManyCommitments)
	})

	t.Run("root mismatch", func(t *testing.T) {
		_, sidecars, _ := util.GenerateTestFuluBlockWithSidecars(t, 2, util.WithSlot(fs))
		block, _, _ := util.GenerateTestFuluBlockWithSidecars(t, 0, util.WithSlot(fs))
		err := peerdas.DataColumnsAlignWithBlock(block, sidecars)
		require.ErrorIs(t, err, peerdas.ErrRootMismatch)
	})

	t.Run("column size mismatch", func(t *testing.T) {
		block, sidecars, _ := util.GenerateTestFuluBlockWithSidecars(t, 2, util.WithSlot(fs))
		sidecars[0].Column = [][]byte{}
		err := peerdas.DataColumnsAlignWithBlock(block, sidecars)
		require.ErrorIs(t, err, peerdas.ErrBlockColumnSizeMismatch)
	})

	t.Run("KZG commitments size mismatch", func(t *testing.T) {
		block, sidecars, _ := util.GenerateTestFuluBlockWithSidecars(t, 2, util.WithSlot(fs))
		sidecars[0].KzgCommitments = [][]byte{}
		err := peerdas.DataColumnsAlignWithBlock(block, sidecars)
		require.ErrorIs(t, err, peerdas.ErrBlockColumnSizeMismatch)
	})

	t.Run("KZG proofs mismatch", func(t *testing.T) {
		block, sidecars, _ := util.GenerateTestFuluBlockWithSidecars(t, 2, util.WithSlot(fs))
		sidecars[0].KzgProofs = [][]byte{}
		err := peerdas.DataColumnsAlignWithBlock(block, sidecars)
		require.ErrorIs(t, err, peerdas.ErrBlockColumnSizeMismatch)
	})

	t.Run("commitment mismatch", func(t *testing.T) {
		block, _, _ := util.GenerateTestFuluBlockWithSidecars(t, 2, util.WithSlot(fs))
		_, alteredSidecars, _ := util.GenerateTestFuluBlockWithSidecars(t, 2, util.WithSlot(fs))
		alteredSidecars[1].KzgCommitments[0][0]++ // Overflow is OK
		err := peerdas.DataColumnsAlignWithBlock(block, alteredSidecars)
		require.ErrorIs(t, err, peerdas.ErrCommitmentMismatch)
	})

	t.Run("nominal", func(t *testing.T) {
		block, sidecars, _ := util.GenerateTestFuluBlockWithSidecars(t, 2, util.WithSlot(fs))
		err := peerdas.DataColumnsAlignWithBlock(block, sidecars)
		require.NoError(t, err)
	})
}
