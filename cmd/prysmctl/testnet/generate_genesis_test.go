package testnet

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"testing"

	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	"github.com/OffchainLabs/prysm/v6/runtime/interop"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

func Test_genesisStateFromJSONValidators(t *testing.T) {
	numKeys := 5
	jsonData := createGenesisDepositData(t, numKeys)
	jsonInput, err := json.Marshal(jsonData)
	require.NoError(t, err)
	_, dds, err := depositEntriesFromJSON(jsonInput)
	require.NoError(t, err)
	for i := range dds {
		assert.DeepEqual(t, fmt.Sprintf("%#x", dds[i].PublicKey), jsonData[i].PubKey)
	}
}

func createGenesisDepositData(t *testing.T, numKeys int) []*depositDataJSON {
	pubKeys := make([]bls.PublicKey, numKeys)
	privKeys := make([]bls.SecretKey, numKeys)
	for i := 0; i < numKeys; i++ {
		randKey, err := bls.RandKey()
		require.NoError(t, err)
		privKeys[i] = randKey
		pubKeys[i] = randKey.PublicKey()
	}
	dataList, _, err := interop.DepositDataFromKeys(privKeys, pubKeys)
	require.NoError(t, err)
	jsonData := make([]*depositDataJSON, numKeys)
	for i := 0; i < numKeys; i++ {
		dataRoot, err := dataList[i].HashTreeRoot()
		require.NoError(t, err)
		jsonData[i] = &depositDataJSON{
			PubKey:                fmt.Sprintf("%#x", dataList[i].PublicKey),
			Amount:                dataList[i].Amount,
			WithdrawalCredentials: fmt.Sprintf("%#x", dataList[i].WithdrawalCredentials),
			DepositDataRoot:       fmt.Sprintf("%#x", dataRoot),
			Signature:             fmt.Sprintf("%#x", dataList[i].Signature),
		}
	}
	return jsonData
}

func Test_generateGenesis_BaseFeeValidation(t *testing.T) {
	tests := []struct {
		name        string
		forkVersion int
		baseFee     *big.Int
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Pre-merge Altair network without baseFee - should use default",
			forkVersion: version.Altair,
			baseFee:     nil,
			expectError: false,
		},
		{
			name:        "Post-merge Bellatrix network without baseFee - should error",
			forkVersion: version.Bellatrix,
			baseFee:     nil,
			expectError: true,
			errorMsg:    "baseFeePerGas must be set in genesis.json for Post-Merge networks (after Altair)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original flags
			originalFlags := generateGenesisStateFlags
			defer func() {
				generateGenesisStateFlags = originalFlags
			}()

			// Set up test flags
			generateGenesisStateFlags.NumValidators = 2
			generateGenesisStateFlags.GenesisTime = 1609459200
			generateGenesisStateFlags.ForkName = version.String(tt.forkVersion)

			// Create a minimal genesis JSON for testing
			genesis := &core.Genesis{
				BaseFee:    tt.baseFee,
				Difficulty: big.NewInt(0),
				GasLimit:   15000000,
				Alloc:      types.GenesisAlloc{},
				Config: &params.ChainConfig{
					ChainID: big.NewInt(32382),
				},
			}

			// Create temporary genesis JSON file
			genesisJSON, err := json.Marshal(genesis)
			require.NoError(t, err)

			tmpFile := t.TempDir() + "/genesis.json"
			err = writeFile(tmpFile, genesisJSON)
			require.NoError(t, err)

			generateGenesisStateFlags.GethGenesisJsonIn = tmpFile

			ctx := context.Background()
			_, err = generateGenesis(ctx)

			if tt.expectError {
				require.ErrorContains(t, tt.errorMsg, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}
