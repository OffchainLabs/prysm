package operations

import (
	"context"
	"path"
	"testing"

	b "github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/utils"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/golang/snappy"
	"github.com/pkg/errors"
)

// ProcessAttestations processes a block's attestations. The parentSlot
// argument is the slot of the parent block of the block being processed; it is
// only meaningful for Gloas and later forks, where it comes from the test
// case's meta.yaml.
type ProcessAttestations func(context.Context, state.BeaconState, interfaces.ReadOnlyBeaconBlock, primitives.Slot) (state.BeaconState, error)

// RunAttestationTest executes "operations/attestation" tests.
func RunAttestationTest(t *testing.T, config string, fork string, blockWithAttestation blockWithSSZObject, processAttestations ProcessAttestations, sszToState SSZToState) {
	require.NoError(t, utils.SetConfig(t, config))
	testFolders, testsFolderPath := utils.TestFolders(t, config, fork, "operations/attestation/pyspec_tests")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", config, fork, "operations/attestation/pyspec_tests")
	}
	forkVersion, err := version.FromString(fork)
	require.NoError(t, err)
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			folderPath := path.Join(testsFolderPath, folder.Name())
			attestationFile, err := util.BazelFileBytes(folderPath, "attestation.ssz_snappy")
			require.NoError(t, err)
			attestationSSZ, err := snappy.Decode(nil /* dst */, attestationFile)
			require.NoError(t, err, "Failed to decompress")
			blk, err := blockWithAttestation(attestationSSZ)
			require.NoError(t, err)
			parentSlot := attestationParentSlot(t, folderPath, forkVersion >= version.Gloas)

			processAtt := func(ctx context.Context, st state.BeaconState, blk interfaces.ReadOnlySignedBeaconBlock) (state.BeaconState, error) {
				st, err = processAttestations(ctx, st, blk.Block(), parentSlot)
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

// attestationParentSlot reads parent_slot from the test case's meta.yaml.
// Gloas and later attestation tests must provide it; earlier forks have no
// parent_slot and get 0, which is ignored by pre-Gloas processing.
func attestationParentSlot(t *testing.T, folderPath string, required bool) primitives.Slot {
	metaFile, err := util.BazelFileBytes(folderPath, "meta.yaml")
	if err != nil {
		if required {
			t.Fatalf("could not read meta.yaml, required for gloas and later attestation tests: %v", err)
		}
		return 0
	}
	meta := &struct {
		ParentSlot *uint64 `json:"parent_slot"`
	}{}
	require.NoError(t, utils.UnmarshalYaml(metaFile, meta))
	if meta.ParentSlot == nil {
		if required {
			t.Fatal("meta.yaml is missing parent_slot, required for gloas and later attestation tests")
		}
		return 0
	}
	return primitives.Slot(*meta.ParentSlot)
}
