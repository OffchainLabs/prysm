package operations

import (
	"path"
	"testing"

	"github.com/golang/snappy"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/spectest/utils"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func runSlashingTest(t *testing.T, config string, fork string, objName string, block blockWithSSZObject, sszToState SSZToState, operationFn BlockOperation) {
	require.NoError(t, utils.SetConfig(t, config))
	testFolders, testsFolderPath := utils.TestFolders(t, config, fork, "operations/"+objName+"/pyspec_tests")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", config, fork, "operations/"+objName+"/pyspec_tests")
	}
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			folderPath := path.Join(testsFolderPath, folder.Name())
			slashingFile, err := util.BazelFileBytes(folderPath, objName+".ssz_snappy")
			require.NoError(t, err)
			slashingSSZ, err := snappy.Decode(nil /* dst */, slashingFile)
			require.NoError(t, err, "Failed to decompress")
			blk, err := block(slashingSSZ)
			require.NoError(t, err)
			RunBlockOperationTest(t, folderPath, blk, sszToState, operationFn)
		})
	}
}
