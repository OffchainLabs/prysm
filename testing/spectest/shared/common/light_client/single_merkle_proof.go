package light_client

import (
	"context"
	"encoding/hex"
	"fmt"
	"path"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v6/container/trie"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/spectest/utils"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/golang/snappy"
)

// RunLightClientSingleMerkleProofTests executes "light_client/single_merkle_proof/{BeaconState, BeaconBlockBody}" tests.
func RunLightClientSingleMerkleProofTests(t *testing.T, config string, v int) {
	require.NoError(t, utils.SetConfig(t, config))

	_, testsFolderPath := utils.TestFolders(t, config, version.String(v), "light_client/single_merkle_proof")
	testTypes, err := util.BazelListDirectories(testsFolderPath)
	require.NoError(t, err)

	if len(testTypes) == 0 {
		t.Fatalf("No test types found for %s", testsFolderPath)
	}

	for _, testType := range testTypes {
		testFolders, testsFolderPath := utils.TestFolders(t, config, version.String(v), fmt.Sprintf("light_client/single_merkle_proof/%s", testType))
		for _, folder := range testFolders {
			helpers.ClearCache()
			t.Run(fmt.Sprintf("%v/%v", testType, folder.Name()), func(t *testing.T) {
				folderPath := path.Join(testsFolderPath, folder.Name())
				if testType == "BeaconState" {
					runLightClientSingleMerkleProofTestBeaconState(t, folderPath, folder.Name(), v)
				} else if testType == "BeaconBlockBody" {
					runLightClientSingleMerkleProofTestBeaconBlockBody(t, folderPath, v)
				} else {
					t.Fatalf("Unsupported test type: %s", testType)
				}
			})
		}
	}
}

func runLightClientSingleMerkleProofTestBeaconState(t *testing.T, testFolderPath string, testName string, v int) {
	ctx := context.Background()

	beaconStateFile, err := util.BazelFileBytes(path.Join(testFolderPath, "object.ssz_snappy"))
	require.NoError(t, err)
	beaconStateSSZ, err := snappy.Decode(nil, beaconStateFile)
	require.NoError(t, err, "Failed to decompress")

	var beaconState state.BeaconState
	switch v {
	case version.Altair:
		beaconStateBase := &ethpb.BeaconStateAltair{}
		require.NoError(t, beaconStateBase.UnmarshalSSZ(beaconStateSSZ), "Failed to unmarshal")
		beaconState, err = state_native.InitializeFromProtoAltair(beaconStateBase)
		require.NoError(t, err)
	case version.Bellatrix:
		beaconStateBase := &ethpb.BeaconStateBellatrix{}
		require.NoError(t, beaconStateBase.UnmarshalSSZ(beaconStateSSZ), "Failed to unmarshal")
		beaconState, err = state_native.InitializeFromProtoBellatrix(beaconStateBase)
		require.NoError(t, err)
	case version.Capella:
		beaconStateBase := &ethpb.BeaconStateCapella{}
		require.NoError(t, beaconStateBase.UnmarshalSSZ(beaconStateSSZ), "Failed to unmarshal")
		beaconState, err = state_native.InitializeFromProtoCapella(beaconStateBase)
		require.NoError(t, err)
	case version.Deneb:
		beaconStateBase := &ethpb.BeaconStateDeneb{}
		require.NoError(t, beaconStateBase.UnmarshalSSZ(beaconStateSSZ), "Failed to unmarshal")
		beaconState, err = state_native.InitializeFromProtoDeneb(beaconStateBase)
		require.NoError(t, err)
	case version.Electra:
		beaconStateBase := &ethpb.BeaconStateElectra{}
		require.NoError(t, beaconStateBase.UnmarshalSSZ(beaconStateSSZ), "Failed to unmarshal")
		beaconState, err = state_native.InitializeFromProtoElectra(beaconStateBase)
		require.NoError(t, err)
	case version.Fulu:
		beaconStateBase := &ethpb.BeaconStateFulu{}
		require.NoError(t, beaconStateBase.UnmarshalSSZ(beaconStateSSZ), "Failed to unmarshal")
		beaconState, err = state_native.InitializeFromProtoFulu(beaconStateBase)
		require.NoError(t, err)
	default:
		t.Fatalf("Unsupported version: %d", v)
	}

	beaconStateRoot, err := beaconState.HashTreeRoot(ctx)
	require.NoError(t, err)
	type Proof struct {
		Leaf      string   `json:"leaf"`
		LeafIndex uint64   `json:"leaf_index"`
		Branch    []string `json:"branch"`
	}
	proofFile, err := util.BazelFileBytes(path.Join(testFolderPath, "proof.yaml"))
	require.NoError(t, err)
	var proof Proof
	require.NoError(t, utils.UnmarshalYaml(proofFile, &proof))
	leaf, err := hex.DecodeString(proof.Leaf[2:])
	require.NoError(t, err)
	require.NotNil(t, leaf)
	var branch [][]byte
	for _, b := range proof.Branch {
		bBytes, err := hex.DecodeString(b[2:])
		require.NoError(t, err)
		branch = append(branch, bBytes)
	}

	var item []byte
	if strings.Contains(testName, "current_sync_committee") {
		syncCommittee, err := beaconState.CurrentSyncCommittee()
		require.NoError(t, err)
		item32, err := syncCommittee.HashTreeRoot()
		require.NoError(t, err)
		item = item32[:]
	} else if strings.Contains(testName, "next_sync_committee") {
		syncCommittee, err := beaconState.NextSyncCommittee()
		require.NoError(t, err)
		item32, err := syncCommittee.HashTreeRoot()
		require.NoError(t, err)
		item = item32[:]
	} else if strings.Contains(testName, "finality_root") {
		item = beaconState.FinalizedCheckpoint().Root
	} else {
		t.Fatalf("Unsupported test type BeaconState/%s", testName)
	}

	require.DeepSSZEqual(t, item, leaf)

	require.Equal(t, true, trie.VerifyMerkleProof(beaconStateRoot[:], item, proof.LeafIndex, branch))
}

func runLightClientSingleMerkleProofTestBeaconBlockBody(t *testing.T, testFolderPath string, v int) {
	beaconBlockBodyFile, err := util.BazelFileBytes(path.Join(testFolderPath, "object.ssz_snappy"))
	require.NoError(t, err)
	beaconBlockBodySSZ, err := snappy.Decode(nil, beaconBlockBodyFile)
	require.NoError(t, err, "Failed to decompress")

	var beaconBlockBodyRoot [32]byte
	var executionPayloadRoot [32]byte
	switch v {
	case version.Capella:
		beaconBlockBody := &ethpb.BeaconBlockBodyCapella{}
		require.NoError(t, beaconBlockBody.UnmarshalSSZ(beaconBlockBodySSZ), "Failed to unmarshal")
		beaconBlockBodyRoot, err = beaconBlockBody.HashTreeRoot()
		require.NoError(t, err)
		executionPayloadRoot, err = beaconBlockBody.ExecutionPayload.HashTreeRoot()
		require.NoError(t, err)
	case version.Deneb:
		beaconBlockBody := &ethpb.BeaconBlockBodyDeneb{}
		require.NoError(t, beaconBlockBody.UnmarshalSSZ(beaconBlockBodySSZ), "Failed to unmarshal")
		beaconBlockBodyRoot, err = beaconBlockBody.HashTreeRoot()
		require.NoError(t, err)
		executionPayloadRoot, err = beaconBlockBody.ExecutionPayload.HashTreeRoot()
		require.NoError(t, err)
	case version.Electra, version.Fulu:
		beaconBlockBody := &ethpb.BeaconBlockBodyElectra{}
		require.NoError(t, beaconBlockBody.UnmarshalSSZ(beaconBlockBodySSZ), "Failed to unmarshal")
		beaconBlockBodyRoot, err = beaconBlockBody.HashTreeRoot()
		require.NoError(t, err)
		executionPayloadRoot, err = beaconBlockBody.ExecutionPayload.HashTreeRoot()
		require.NoError(t, err)
	default:
		t.Fatalf("Unsupported version: %d", v)
	}

	type Proof struct {
		Leaf      string   `json:"leaf"`
		LeafIndex uint64   `json:"leaf_index"`
		Branch    []string `json:"branch"`
	}
	proofFile, err := util.BazelFileBytes(path.Join(testFolderPath, "proof.yaml"))
	require.NoError(t, err)
	var proof Proof
	require.NoError(t, utils.UnmarshalYaml(proofFile, &proof))
	leaf, err := hex.DecodeString(proof.Leaf[2:])
	if err != nil {
		fmt.Printf("Error decoding leaf: %v\n", err)
	}
	require.NoError(t, err)
	var branch [][]byte
	for _, b := range proof.Branch {
		bBytes, err := hex.DecodeString(b[2:])
		require.NoError(t, err)
		branch = append(branch, bBytes)
	}

	require.DeepSSZEqual(t, executionPayloadRoot[:], leaf)

	require.Equal(t, true, trie.VerifyMerkleProof(beaconBlockBodyRoot[:], executionPayloadRoot[:], proof.LeafIndex, branch))
}
