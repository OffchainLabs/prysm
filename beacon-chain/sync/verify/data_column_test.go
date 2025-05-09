package verify

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

func TestDataColumnsAlignWithBlock(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.FuluForkEpoch = 0
	config.DeprecatedMaxBlobsPerBlockFulu = 2
	params.OverrideBeaconConfig(config)

	// Start the trusted setup.
	err := kzg.Start()
	require.NoError(t, err)

	t.Run("pre fulu", func(t *testing.T) {
		block, _ := util.GenerateTestElectraBlockWithSidecar(t, [fieldparams.RootLength]byte{}, 0, 0)
		err := DataColumnsAlignWithBlock(block, nil)
		require.NoError(t, err)
	})

	t.Run("too many commitmnets", func(t *testing.T) {
		block, _ := util.GenerateTestFuluBlockWithSidecars(t, [fieldparams.RootLength]byte{}, 0, 3)
		err := DataColumnsAlignWithBlock(block, nil)
		require.ErrorIs(t, err, errTooManyCommitments)
	})

	t.Run("root mismatch", func(t *testing.T) {
		_, sidecars := util.GenerateTestFuluBlockWithSidecars(t, [fieldparams.RootLength]byte{1}, 0, 2)
		block, _ := util.GenerateTestFuluBlockWithSidecars(t, [fieldparams.RootLength]byte{2}, 0, 0)
		err := DataColumnsAlignWithBlock(block, sidecars)
		require.ErrorIs(t, err, errRootMismatch)
	})

	t.Run("column size mismatch", func(t *testing.T) {
		block, sidecars := util.GenerateTestFuluBlockWithSidecars(t, [fieldparams.RootLength]byte{}, 0, 2)
		sidecars[0].Column = [][]byte{}
		err := DataColumnsAlignWithBlock(block, sidecars)
		require.ErrorIs(t, err, errSizeMismatch)
	})

	t.Run("KZG commitments size mismatch", func(t *testing.T) {
		block, sidecars := util.GenerateTestFuluBlockWithSidecars(t, [fieldparams.RootLength]byte{}, 0, 2)
		sidecars[0].KzgCommitments = [][]byte{}
		err := DataColumnsAlignWithBlock(block, sidecars)
		require.ErrorIs(t, err, errSizeMismatch)
	})

	t.Run("KZG proofs mismatch", func(t *testing.T) {
		block, sidecars := util.GenerateTestFuluBlockWithSidecars(t, [fieldparams.RootLength]byte{}, 0, 2)
		sidecars[0].KzgProofs = [][]byte{}
		err := DataColumnsAlignWithBlock(block, sidecars)
		require.ErrorIs(t, err, errSizeMismatch)
	})

	t.Run("commitment mismatch", func(t *testing.T) {
		block, _ := util.GenerateTestFuluBlockWithSidecars(t, [fieldparams.RootLength]byte{}, 0, 2)
		_, alteredSidecars := util.GenerateTestFuluBlockWithSidecars(t, [fieldparams.RootLength]byte{}, 0, 2)
		alteredSidecars[1].KzgCommitments[0][0]++ // Overflow is OK
		err := DataColumnsAlignWithBlock(block, alteredSidecars)
		require.ErrorIs(t, err, errCommitmentMismatch)
	})

	t.Run("nominal", func(t *testing.T) {
		block, sidecars := util.GenerateTestFuluBlockWithSidecars(t, [fieldparams.RootLength]byte{}, 0, 2)
		err := DataColumnsAlignWithBlock(block, sidecars)
		require.NoError(t, err)
	})
}
