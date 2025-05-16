package light_client

import (
	"context"
	"encoding/hex"
	"fmt"
	"path"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v6/container/trie"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/spectest/utils"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/golang/snappy"
)

// RunLightClientSingleMerkleProofTests executes "light_client/single_merkle_proof/{BeaconState}" tests.
func RunLightClientSingleMerkleProofTests(t *testing.T, config string) {
	require.NoError(t, utils.SetConfig(t, config))

	_, testsFolderPath := utils.TestFolders(t, config, "deneb", "light_client/single_merkle_proof")
	testTypes, err := util.BazelListDirectories(testsFolderPath)
	require.NoError(t, err)

	if len(testTypes) == 0 {
		t.Fatalf("No test types found for %s", testsFolderPath)
	}

	for _, testType := range testTypes {
		testFolders, testsFolderPath := utils.TestFolders(t, config, "deneb", fmt.Sprintf("light_client/single_merkle_proof/%s", testType))
		for _, folder := range testFolders {
			helpers.ClearCache()
			t.Run(fmt.Sprintf("%v/%v", testType, folder.Name()), func(t *testing.T) {
				folderPath := path.Join(testsFolderPath, folder.Name())
				if testType == "BeaconState" {
					runLightClientSingleMerkleProofTestBeaconState(t, folderPath, folder.Name())
				} else if testType == "BeaconBlockBody" {
					runLightClientSingleMerkleProofTestBeaconBlockBody(t, folderPath)
				}
			})
		}
	}
}

func runLightClientSingleMerkleProofTestBeaconState(t *testing.T, testFolderPath string, testName string) {
	ctx := context.Background()

	beaconStateFile, err := util.BazelFileBytes(path.Join(testFolderPath, "object.ssz_snappy"))
	require.NoError(t, err)
	beaconStateSSZ, err := snappy.Decode(nil, beaconStateFile)
	require.NoError(t, err, "Failed to decompress")
	beaconStateBase := &ethpb.BeaconStateDeneb{}
	require.NoError(t, beaconStateBase.UnmarshalSSZ(beaconStateSSZ), "Failed to unmarshal")
	beaconState, err := state_native.InitializeFromProtoDeneb(beaconStateBase)
	require.NoError(t, err)
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
	}

	require.DeepSSZEqual(t, item, leaf)

	require.Equal(t, true, trie.VerifyMerkleProof(beaconStateRoot[:], item, proof.LeafIndex, branch))
}

func runLightClientSingleMerkleProofTestBeaconBlockBody(t *testing.T, testFolderPath string) {
	beaconBlockBodyFile, err := util.BazelFileBytes(path.Join(testFolderPath, "object.ssz_snappy"))
	require.NoError(t, err)
	beaconBlockBodySSZ, err := snappy.Decode(nil, beaconBlockBodyFile)
	require.NoError(t, err, "Failed to decompress")
	beaconBlockBody := &ethpb.BeaconBlockBodyDeneb{}
	require.NoError(t, beaconBlockBody.UnmarshalSSZ(beaconBlockBodySSZ), "Failed to unmarshal")
	beaconBlockBodyRoot, err := beaconBlockBody.HashTreeRoot()
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

	item, err := beaconBlockBody.ExecutionPayload.HashTreeRoot()
	require.NoError(t, err)

	require.DeepSSZEqual(t, item[:], leaf)

	require.Equal(t, true, trie.VerifyMerkleProof(beaconBlockBodyRoot[:], item[:], proof.LeafIndex, branch))
}
