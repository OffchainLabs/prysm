package operations

import (
	"context"
	"path"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/electra"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/spectest/utils"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/golang/snappy"
)

func RunConsolidationTest(t *testing.T, config string, fork string, block blockWithSSZObject, sszToState SSZToState) {
	require.NoError(t, utils.SetConfig(t, config))
	testFolders, testsFolderPath := utils.TestFolders(t, config, fork, "operations/consolidation_request/pyspec_tests")
	require.NotEqual(t, 0, len(testFolders), "missing tests for consolidation operation in folder")
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			folderPath := path.Join(testsFolderPath, folder.Name())
			consolidationFile, err := util.BazelFileBytes(folderPath, "consolidation_request.ssz_snappy")
			require.NoError(t, err)
			consolidationSSZ, err := snappy.Decode(nil /* dst */, consolidationFile)
			require.NoError(t, err, "Failed to decompress")
			blk, err := block(consolidationSSZ)
			require.NoError(t, err)
			RunBlockOperationTest(t, folderPath, blk, sszToState, func(ctx context.Context, s state.BeaconState, b interfaces.ReadOnlySignedBeaconBlock) (state.BeaconState, error) {
				er, err := b.Block().Body().ExecutionRequests()
				if err != nil {
					return nil, err
				}
				return s, electra.ProcessConsolidationRequests(ctx, s, er.Consolidations)
			})
		})
	}
}
