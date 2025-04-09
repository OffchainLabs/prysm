package operations

import (
	"context"
	"path"
	"testing"

	"github.com/golang/snappy"
	"github.com/pkg/errors"
	b "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/blocks"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/spectest/utils"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

// RunAttestationTest executes "operations/attestation" tests.
func RunAttestationTest(t *testing.T, config string, fork string, blockWithAttestation blockWithSSZObject, processBlock ProcessBlock, sszToState SSZToState) {
	require.NoError(t, utils.SetConfig(t, config))
	testFolders, testsFolderPath := utils.TestFolders(t, config, fork, "operations/attestation/pyspec_tests")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", config, fork, "operations/attestation/pyspec_tests")
	}
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			folderPath := path.Join(testsFolderPath, folder.Name())
			attestationFile, err := util.BazelFileBytes(folderPath, "attestation.ssz_snappy")
			require.NoError(t, err)
			attestationSSZ, err := snappy.Decode(nil /* dst */, attestationFile)
			require.NoError(t, err, "Failed to decompress")
			blk, err := blockWithAttestation(attestationSSZ)
			require.NoError(t, err)

			processAtt := func(ctx context.Context, st state.BeaconState, blk interfaces.ReadOnlySignedBeaconBlock) (state.BeaconState, error) {
				st, err = processBlock(ctx, st, blk.Block())
				if err != nil {
					return nil, err
				}
				aSet, err := b.AttestationSignatureBatch(ctx, st, blk.Block().Body().Attestations())
				if err != nil {
					return nil, err
				}
				verified, err := aSet.Verify()
				if err != nil {
					return nil, err
				}
				if !verified {
					return nil, errors.New("could not batch verify attestation signature")
				}
				return st, nil
			}

			RunBlockOperationTest(t, folderPath, blk, sszToState, processAtt)
		})
	}
}
