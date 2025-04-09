package historycmd

import (
	"encoding/json"
	"flag"
	"path/filepath"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/cmd"
	"github.com/prysmaticlabs/prysm/v5/cmd/validator/flags"
	"github.com/prysmaticlabs/prysm/v5/io/file"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/validator/db/common"
	dbTest "github.com/prysmaticlabs/prysm/v5/validator/db/testing"
	"github.com/prysmaticlabs/prysm/v5/validator/slashing-protection-history/format"
	mocks "github.com/prysmaticlabs/prysm/v5/validator/testing"
	"github.com/urfave/cli/v2"
)

func setupCliCtx(
	tb testing.TB,
	dbPath,
	protectionFilePath,
	outputDir string,
) *cli.Context {
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.String(cmd.DataDirFlag.Name, dbPath, "")
	set.String(flags.SlashingProtectionJSONFileFlag.Name, protectionFilePath, "")
	set.String(flags.SlashingProtectionExportDirFlag.Name, outputDir, "")
	require.NoError(tb, set.Set(flags.SlashingProtectionJSONFileFlag.Name, protectionFilePath))
	assert.NoError(tb, set.Set(cmd.DataDirFlag.Name, dbPath))
	assert.NoError(tb, set.Set(flags.SlashingProtectionExportDirFlag.Name, outputDir))
	return cli.NewContext(&app, set, nil)
}

// TestImportExportSlashingProtectionCli_RoundTrip imports a EIP-3076 interchange format JSON file,
// and exports it back to disk. It then compare the exported file to the original file.
// This test is only suitable for complete slashing protection history database, since minimal
// slashing protection history database will keep only the latest signed block slot / attestations,
// and thus will not be able to export the same data as the original file.
func TestImportExportSlashingProtectionCli_RoundTrip(t *testing.T) {
	numValidators := 10
	outputPath := filepath.Join(t.TempDir(), "slashing-exports")
	err := file.MkdirAll(outputPath)
	require.NoError(t, err)
	protectionFileName := "slashing_history_import.json"

	// Create some mock slashing protection history. and JSON file
	pubKeys, err := mocks.CreateRandomPubKeys(numValidators)
	require.NoError(t, err)
	attestingHistory, proposalHistory := mocks.MockAttestingAndProposalHistories(pubKeys)
	require.NoError(t, err)
	mockJSON, err := mocks.MockSlashingProtectionJSON(pubKeys, attestingHistory, proposalHistory)
	require.NoError(t, err)

	// We JSON encode the protection file and save it to disk as a JSON file.
	encoded, err := json.Marshal(mockJSON)
	require.NoError(t, err)

	protectionFilePath := filepath.Join(outputPath, protectionFileName)
	err = file.WriteFile(protectionFilePath, encoded)
	require.NoError(t, err)

	// We create a CLI context with the required values, such as the database datadir and output directory.
	isSlashingProtectionMinimal := false
	validatorDB := dbTest.SetupDB(t, pubKeys, isSlashingProtectionMinimal)
	dbPath := validatorDB.DatabasePath()
	require.NoError(t, validatorDB.Close())
	cliCtx := setupCliCtx(t, dbPath, protectionFilePath, outputPath)

	// We import the slashing protection history file via CLI.
	err = importSlashingProtectionJSON(cliCtx)
	require.NoError(t, err)

	// We export the slashing protection history file via CLI.
	err = exportSlashingProtectionJSON(cliCtx)
	require.NoError(t, err)

	// Attempt to read the exported file from the output directory.
	enc, err := file.ReadFileAsBytes(filepath.Join(outputPath, jsonExportFileName))
	require.NoError(t, err)

	receivedJSON := &format.EIPSlashingProtectionFormat{}
	err = json.Unmarshal(enc, receivedJSON)
	require.NoError(t, err)

	// We verify the parsed JSON file matches. Given there is no guarantee of order,
	// we will have to carefully compare and sort values as needed.
	//
	// First, we compare basic data such as the MetadataV0 value in the JSON file.
	require.DeepEqual(t, mockJSON.Metadata, receivedJSON.Metadata)
	wantedHistoryByPublicKey := make(map[string]*format.ProtectionData)
	for _, item := range mockJSON.Data {
		wantedHistoryByPublicKey[item.Pubkey] = item
	}

	// Next, we compare all the data for each validator public key.
	for _, item := range receivedJSON.Data {
		wanted, ok := wantedHistoryByPublicKey[item.Pubkey]
		require.Equal(t, true, ok)
		wantedAttsByRoot := make(map[string]*format.SignedAttestation)
		for _, att := range wanted.SignedAttestations {
			wantedAttsByRoot[att.SigningRoot] = att
		}
		for _, att := range item.SignedAttestations {
			wantedAtt, ok := wantedAttsByRoot[att.SigningRoot]
			require.Equal(t, true, ok)
			require.DeepEqual(t, wantedAtt, att)
		}
		require.Equal(t, len(wanted.SignedBlocks), len(item.SignedBlocks))
		require.DeepEqual(t, wanted.SignedBlocks, item.SignedBlocks)
	}
}

// TestImportExportSlashingProtectionCli_EmptyData imports a EIP-3076 interchange format JSON file,
// and exports it back to disk. It then compare the exported file to the original file.
// This test is only suitable for complete slashing protection history database, since minimal
// slashing protection history database will keep only the latest signed block slot / attestations,
// and thus will not be able to export the same data as the original file.
func TestImportExportSlashingProtectionCli_EmptyData(t *testing.T) {
	numValidators := 10
	outputPath := filepath.Join(t.TempDir(), "slashing-exports")
	err := file.MkdirAll(outputPath)
	require.NoError(t, err)
	protectionFileName := "slashing_history_import.json"

	// Create some mock slashing protection history. and JSON file
	pubKeys, err := mocks.CreateRandomPubKeys(numValidators)
	require.NoError(t, err)
	attestingHistory := make([][]*common.AttestationRecord, 0)
	proposalHistory := make([]common.ProposalHistoryForPubkey, len(pubKeys))
	for i := 0; i < len(pubKeys); i++ {
		proposalHistory[i].Proposals = make([]common.Proposal, 0)
	}
	mockJSON, err := mocks.MockSlashingProtectionJSON(pubKeys, attestingHistory, proposalHistory)
	require.NoError(t, err)

	// We JSON encode the protection file and save it to disk as a JSON file.
	encoded, err := json.Marshal(mockJSON)
	require.NoError(t, err)

	protectionFilePath := filepath.Join(outputPath, protectionFileName)
	err = file.WriteFile(protectionFilePath, encoded)
	require.NoError(t, err)

	// We create a CLI context with the required values, such as the database datadir and output directory.
	isSlashingProtectionMinimal := false
	validatorDB := dbTest.SetupDB(t, pubKeys, isSlashingProtectionMinimal)
	dbPath := validatorDB.DatabasePath()
	require.NoError(t, validatorDB.Close())
	cliCtx := setupCliCtx(t, dbPath, protectionFilePath, outputPath)

	// We import the slashing protection history file via CLI.
	err = importSlashingProtectionJSON(cliCtx)
	require.NoError(t, err)

	// We export the slashing protection history file via CLI.
	err = exportSlashingProtectionJSON(cliCtx)
	require.NoError(t, err)

	// Attempt to read the exported file from the output directory.
	enc, err := file.ReadFileAsBytes(filepath.Join(outputPath, jsonExportFileName))
	require.NoError(t, err)

	receivedJSON := &format.EIPSlashingProtectionFormat{}
	err = json.Unmarshal(enc, receivedJSON)
	require.NoError(t, err)

	// We verify the parsed JSON file matches. Given there is no guarantee of order,
	// we will have to carefully compare and sort values as needed.
	//
	// First, we compare basic data such as the MetadataV0 value in the JSON file.
	require.DeepEqual(t, mockJSON.Metadata, receivedJSON.Metadata)
	wantedHistoryByPublicKey := make(map[string]*format.ProtectionData)
	for _, item := range mockJSON.Data {
		wantedHistoryByPublicKey[item.Pubkey] = item
	}

	// Next, we compare all the data for each validator public key.
	for _, item := range receivedJSON.Data {
		wanted, ok := wantedHistoryByPublicKey[item.Pubkey]
		require.Equal(t, true, ok)
		require.Equal(t, len(wanted.SignedBlocks), len(item.SignedBlocks))
		require.Equal(t, len(wanted.SignedAttestations), len(item.SignedAttestations))
		require.DeepEqual(t, make([]*format.SignedBlock, 0), item.SignedBlocks)
		require.DeepEqual(t, make([]*format.SignedAttestation, 0), item.SignedAttestations)
	}
}
