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

func TestComputeBlobKZGProof(t *testing.T) {
	type data struct {
		Input struct {
			Blob       string `json:"blob"`
			Commitment string `json:"commitment"`
		} `json:"input"`
		Output *string `json:"output"`
	}

	require.NoError(t, kzgPrysm.Start())
	testFolders, testFolderPath := utils.TestFolders(t, "kzg", "compute_blob_kzg_proof", "")
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
			if len(blob) != fieldparams.BlobLength || len(commitment) != len(kzgPrysm.Commitment{}) {
				require.IsNil(t, test.Output)
				return
			}
			proof, err := kzgPrysm.ComputeBlobKZGProof((*kzgPrysm.Blob)(blob), kzgPrysm.Commitment(commitment))
			if test.Output == nil {
				require.NotNil(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, *test.Output, hexutil.Encode(proof[:]))
		})
	}
}
