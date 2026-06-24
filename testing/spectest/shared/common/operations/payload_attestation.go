package operations

import (
	"context"
	"path"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/utils"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/golang/snappy"
)

type PayloadAttestationOperation func(ctx context.Context, s state.BeaconState, att *eth.PayloadAttestation) (state.BeaconState, error)

func RunPayloadAttestationTest(t *testing.T, config string, fork string, sszToState SSZToState) {
	runPayloadAttestationTest(t, config, fork, "payload_attestation", sszToState, func(ctx context.Context, s state.BeaconState, att *eth.PayloadAttestation) (state.BeaconState, error) {
		// Create a mock block body with the payload attestation
		body, err := createMockBlockBodyWithPayloadAttestation(att)
		if err != nil {
			return s, err
		}
		// Wrap the protobuf body in the interface
		wrappedBody, err := blocks.NewBeaconBlockBody(body)
		if err != nil {
			return s, err
		}
		err = gloas.ProcessPayloadAttestations(ctx, s, wrappedBody)
		return s, err
	})
}

func runPayloadAttestationTest(t *testing.T, config string, fork string, objName string, sszToState SSZToState, operationFn PayloadAttestationOperation) {
	require.NoError(t, utils.SetConfig(t, config))
	testFolders, testsFolderPath := utils.TestFolders(t, config, fork, "operations/"+objName+"/pyspec_tests")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", config, fork, "operations/"+objName+"/pyspec_tests")
	}
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			helpers.ClearCache()
			folderPath := path.Join(testsFolderPath, folder.Name())

			// Load payload attestation from payload_attestation.ssz_snappy
			attestationFile, err := util.BazelFileBytes(folderPath, "payload_attestation.ssz_snappy")
			require.NoError(t, err)
			attestationSSZ, err := snappy.Decode(nil /* dst */, attestationFile)
			require.NoError(t, err, "Failed to decompress payload attestation")

			// Unmarshal payload attestation
			att := &eth.PayloadAttestation{}
			err = att.UnmarshalSSZ(attestationSSZ)
			require.NoError(t, err, "Failed to unmarshal payload attestation")

			runPayloadAttestationOperationTest(t, folderPath, att, sszToState, operationFn)
		})
	}
}

// runPayloadAttestationOperationTest runs a single payload attestation operation test
func runPayloadAttestationOperationTest(t *testing.T, folderPath string, att *eth.PayloadAttestation, sszToState SSZToState, operationFn PayloadAttestationOperation) {
	preBeaconStateFile, err := util.BazelFileBytes(folderPath, "pre.ssz_snappy")
	require.NoError(t, err)
	preBeaconStateSSZ, err := snappy.Decode(nil /* dst */, preBeaconStateFile)
	require.NoError(t, err, "Failed to decompress")
	beaconState, err := sszToState(preBeaconStateSSZ)
	require.NoError(t, err)

	// Check if post state exists
	postStateExists := true
	postBeaconStateFile, err := util.BazelFileBytes(folderPath, "post.ssz_snappy")
	if err != nil {
		postStateExists = false
	}

	ctx := t.Context()
	resultState, err := operationFn(ctx, beaconState, att)

	if postStateExists {
		// Test should succeed
		require.NoError(t, err, "Operation should succeed")

		// Compare with expected post state
		postBeaconStateSSZ, err := snappy.Decode(nil /* dst */, postBeaconStateFile)
		require.NoError(t, err, "Failed to decompress post state")
		expectedState, err := sszToState(postBeaconStateSSZ)
		require.NoError(t, err)

		expectedRoot, err := expectedState.HashTreeRoot(ctx)
		require.NoError(t, err)
		resultRoot, err := resultState.HashTreeRoot(ctx)
		require.NoError(t, err)
		require.DeepEqual(t, expectedRoot, resultRoot, "Post state does not match expected")
	} else {
		// Test should fail (no post.ssz_snappy means the operation should error)
		require.NotNil(t, err, "Operation should fail but succeeded")
	}
}

// createMockBlockBodyWithPayloadAttestation creates a mock block body containing the payload attestation
func createMockBlockBodyWithPayloadAttestation(att *eth.PayloadAttestation) (*eth.BeaconBlockBodyGloas, error) {
	body := &eth.BeaconBlockBodyGloas{
		PayloadAttestations: []*eth.PayloadAttestation{att},
		// Default values
		RandaoReveal:          make([]byte, 96),
		Eth1Data:              &eth.Eth1Data{},
		Graffiti:              make([]byte, 32),
		ProposerSlashings:     []*eth.ProposerSlashing{},
		AttesterSlashings:     []*eth.AttesterSlashingElectra{},
		Attestations:          []*eth.AttestationElectra{},
		Deposits:              []*eth.Deposit{},
		VoluntaryExits:        []*eth.SignedVoluntaryExit{},
		SyncAggregate:         &eth.SyncAggregate{},
		BlsToExecutionChanges: []*eth.SignedBLSToExecutionChange{},
	}
	return body, nil
}
