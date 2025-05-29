package general

import (
	"encoding/hex"
	"path"
	"testing"

	kzgPrysm "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/spectest/utils"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/ghodss/yaml"
)

type KZGTestDataInput struct {
	Blobs       []string `json:"blobs"`
	Commitments []string `json:"commitments"`
	Proofs      []string `json:"proofs"`
}

type KZGTestData struct {
	Input  KZGTestDataInput `json:"input"`
	Output bool             `json:"output"`
}

func TestVerifyBlobKZGProofBatch(t *testing.T) {
	require.NoError(t, kzgPrysm.Start())
	testFolders, testFolderPath := utils.TestFolders(t, "general", "deneb", "kzg/verify_blob_kzg_proof_batch/kzg-mainnet")
	if len(testFolders) == 0 {
		t.Fatalf("No test folders found for %s/%s/%s", "general", "deneb", "kzg/verify_blob_kzg_proof_batch/kzg-mainnet")
	}
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			file, err := util.BazelFileBytes(path.Join(testFolderPath, folder.Name(), "data.yaml"))
			require.NoError(t, err)
			test := &KZGTestData{}
			require.NoError(t, yaml.Unmarshal(file, test))
			var sidecars []blocks.ROBlob
			blobs := test.Input.Blobs
			proofs := test.Input.Proofs
			kzgs := test.Input.Commitments
			if len(proofs) != len(blobs) {
				require.Equal(t, false, test.Output)
				return
			}
			if len(kzgs) != len(blobs) {
				require.Equal(t, false, test.Output)
				return
			}

			for i, blob := range blobs {
				blobBytes, err := hex.DecodeString(blob[2:])
				require.NoError(t, err)
				proofBytes, err := hex.DecodeString(proofs[i][2:])
				require.NoError(t, err)
				kzgBytes, err := hex.DecodeString(kzgs[i][2:])
				require.NoError(t, err)
				sidecar := &ethpb.BlobSidecar{
					Blob:          blobBytes,
					KzgProof:      proofBytes,
					KzgCommitment: kzgBytes,
				}
				sidecar.SignedBlockHeader = util.HydrateSignedBeaconHeader(&ethpb.SignedBeaconBlockHeader{})
				sc, err := blocks.NewROBlob(sidecar)
				require.NoError(t, err)
				sidecars = append(sidecars, sc)
			}
			if test.Output {
				require.NoError(t, kzgPrysm.Verify(sidecars...))
			} else {
				require.NotNil(t, kzgPrysm.Verify(sidecars...))
			}
		})
	}
}
