package general

import (
	"path"
	"testing"

	kzgPrysm "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/utils"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ghodss/yaml"
)

func TestVerifyBlobKZGProof(t *testing.T) {
	type data struct {
		Input struct {
			Blob       string `json:"blob"`
			Commitment string `json:"commitment"`
			Proof      string `json:"proof"`
		} `json:"input"`
		Output *bool `json:"output"`
	}

	require.NoError(t, kzgPrysm.Start())
	testFolders, testFolderPath := utils.TestFolders(t, "kzg", "verify_blob_kzg_proof", "")
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			file, err := util.BazelFileBytes(path.Join(testFolderPath, folder.Name(), "data.yaml"))
			require.NoError(t, err)
			test := &data{}
			require.NoError(t, yaml.Unmarshal(file, test))

			blob, err := hexutil.Decode(test.Input.Blob)
			require.NoError(t, err)
			commitment, err := hexutil.Decode(test.Input.Commitment)
			require.NoError(t, err)
			proof, err := hexutil.Decode(test.Input.Proof)
			require.NoError(t, err)
			if len(blob) != fieldparams.BlobLength || len(commitment) != len(kzgPrysm.Commitment{}) || len(proof) != len(kzgPrysm.Proof{}) {
				require.IsNil(t, test.Output)
				return
			}
			// Prysm only reports pass or fail, so an invalid input (null) and a bad proof (false) both fail.
			err = kzgPrysm.VerifyBlobKZGProofBatch([][]byte{blob}, [][]byte{commitment}, [][]byte{proof})
			require.Equal(t, test.Output != nil && *test.Output, err == nil)
		})
	}
}
