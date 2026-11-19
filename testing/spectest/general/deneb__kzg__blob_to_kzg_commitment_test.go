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

func TestBlobToKZGCommitment(t *testing.T) {
	type data struct {
		Input struct {
			Blob string `json:"blob"`
		} `json:"input"`
		Output *string `json:"output"`
	}

	require.NoError(t, kzgPrysm.Start())
	testFolders, testFolderPath := utils.TestFolders(t, "kzg", "blob_to_kzg_commitment", "")
	for _, folder := range testFolders {
		t.Run(folder.Name(), func(t *testing.T) {
			file, err := util.BazelFileBytes(path.Join(testFolderPath, folder.Name(), "data.yaml"))
			require.NoError(t, err)
			test := &data{}
			require.NoError(t, yaml.Unmarshal(file, test))

			blob, err := hexutil.Decode(test.Input.Blob)
			require.NoError(t, err)
			if len(blob) != fieldparams.BlobLength {
				require.IsNil(t, test.Output)
				return
			}
			commitment, err := kzgPrysm.BlobToKZGCommitment((*kzgPrysm.Blob)(blob))
			if test.Output == nil {
				require.NotNil(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, *test.Output, hexutil.Encode(commitment[:]))
		})
	}
}
