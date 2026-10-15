package operations

import (
	"context"
	"path"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	common "github.com/OffchainLabs/prysm/v7/testing/spectest/shared/common/operations"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/utils"
)

func emptyBlockHeze() (interfaces.SignedBeaconBlock, error) {
	b := &ethpb.SignedBeaconBlockHeze{
		Block: &ethpb.BeaconBlockHeze{
			Body: &ethpb.BeaconBlockBodyHeze{},
		},
	}
	return blocks.NewSignedBeaconBlock(b)
}

func RunWithdrawalsTest(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))
	testFolders, testsFolderPath := utils.TestFolders(t, config, version.String(version.Heze), "operations/withdrawals/pyspec_tests")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", config, version.String(version.Heze), "operations/withdrawals/pyspec_tests")
	}
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			folderPath := path.Join(testsFolderPath, folder.Name())
			blk, err := emptyBlockHeze()
			require.NoError(t, err)

			common.RunBlockOperationTest(t, folderPath, blk, sszToState, func(_ context.Context, s state.BeaconState, _ interfaces.ReadOnlySignedBeaconBlock) (state.BeaconState, error) {
				if err := gloas.ProcessWithdrawals(s); err != nil {
					return nil, err
				}
				return s, nil
			})
		})
	}
}
