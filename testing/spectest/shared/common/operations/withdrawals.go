package operations

import (
	"context"
	"path"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	consensusblocks "github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/spectest/utils"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/golang/snappy"
)

func RunWithdrawalsTest(t *testing.T, config string, fork string, sszToBlock SSZToBlock, sszToState SSZToState) {
	require.NoError(t, utils.SetConfig(t, config))
	testFolders, testsFolderPath := utils.TestFolders(t, config, fork, "operations/withdrawals/pyspec_tests")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", config, fork, "operations/withdrawals/pyspec_tests")
	}
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			folderPath := path.Join(testsFolderPath, folder.Name())
			payloadFile, err := util.BazelFileBytes(folderPath, "execution_payload.ssz_snappy")
			require.NoError(t, err)
			payloadSSZ, err := snappy.Decode(nil /* dst */, payloadFile)
			require.NoError(t, err, "Failed to decompress")
			blk, err := sszToBlock(payloadSSZ)
			require.NoError(t, err)

			RunBlockOperationTest(t, folderPath, blk, sszToState, func(_ context.Context, s state.BeaconState, b interfaces.ReadOnlySignedBeaconBlock) (state.BeaconState, error) {
				payload, err := b.Block().Body().Execution()
				if err != nil {
					return nil, err
				}
				withdrawals, err := payload.Withdrawals()
				if err != nil {
					return nil, err
				}
				p, err := consensusblocks.WrappedExecutionPayloadCapella(&enginev1.ExecutionPayloadCapella{Withdrawals: withdrawals})
				require.NoError(t, err)
				return blocks.ProcessWithdrawals(s, p)
			})
		})
	}
}
