package filesystem

import (
	"context"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/proposer"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/io/file"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func getPubKeys(t *testing.T, count int) [][fieldparams.BLSPubkeyLength]byte {
	pubKeys := make([][fieldparams.BLSPubkeyLength]byte, count)

	for i := range pubKeys {
		validatorKey, err := bls.RandKey()
		require.NoError(t, err, "RandKey should not return an error")

		copy(pubKeys[i][:], validatorKey.PublicKey().Marshal())
	}

	return pubKeys
}

func TestStore_NewStore(t *testing.T) {
	// Create some pubkeys.
	pubkeys := getPubKeys(t, 5)

	// Just check `NewStore` does not return an error.
	_, err := NewStore(t.TempDir(), &Config{PubKeys: pubkeys})
	require.NoError(t, err, "NewStore should not return an error")
}

func TestStore_Close(t *testing.T) {
	// Create a new store.
	s, err := NewStore(t.TempDir(), nil)
	require.NoError(t, err, "NewStore should not return an error")

	// Close the DB.
	require.NoError(t, s.Close(), "Close should not return an error")
}

func TestStore_DatabasePath(t *testing.T) {
	// Get a database parent path.
	databaseParentPath := t.TempDir()

	// Create a new store.
	s, err := NewStore(databaseParentPath, nil)
	require.NoError(t, err, "NewStore should not return an error")

	expected := databaseParentPath
	actual := s.DatabasePath()

	require.Equal(t, expected, actual)
}

func TestStore_ClearDB(t *testing.T) {
	// Get a database parent path.
	databaseParentPath := t.TempDir()

	// Compute slashing protection directory and configuration file paths.
	databasePath := path.Join(databaseParentPath, DatabaseDirName)

	// Create some pubkeys.
	pubkeys := getPubKeys(t, 5)

	// Create a new store.
	s, err := NewStore(databaseParentPath, &Config{PubKeys: pubkeys})
	require.NoError(t, err, "NewStore should not return an error")

	// Check the presence of the database directory.
	exists, err := file.Exists(databasePath, file.Directory)
	require.NoError(t, err, "file.Exists should not return an error")
	require.Equal(t, true, exists, "file.Exists should return true")

	// Clear the DB.
	err = s.ClearDB()
	require.NoError(t, err, "ClearDB should not return an error")

	// Check the absence of the database directory.
	exists, err = file.Exists(databasePath, file.Directory)
	require.NoError(t, err, "file.Exists should not return an error")
	require.Equal(t, false, exists, "file.Exists should return false")
}

func TestStore_Backup(t *testing.T) {
	// Get a database parent path.
	databaseParentPath := t.TempDir()
	originalDatabaseDirPath := path.Join(databaseParentPath, DatabaseDirName)

	// Get a backups directory path.
	backupsPath := t.TempDir()

	// Create some pubkeys.
	pubkeys := getPubKeys(t, 5)

	// Create a new store.
	s, err := NewStore(databaseParentPath, &Config{PubKeys: pubkeys})
	require.NoError(t, err, "NewStore should not return an error")

	// Update the proposer settings.
	err = s.SaveProposerSettings(context.Background(), &proposer.Settings{
		DefaultConfig: &proposer.Option{
			FeeRecipientConfig: &proposer.FeeRecipientConfig{
				FeeRecipient: common.Address{},
			},
		},
	})
	require.NoError(t, err, "SaveProposerSettings should not return an error")

	// Backup the DB.
	require.NoError(t, s.Backup(context.Background(), backupsPath, true), "Backup should not return an error")

	// Get the directory path of the backup.
	files, err := os.ReadDir(path.Join(backupsPath, backupsDirectoryName))
	require.NoError(t, err, "os.ReadDir should not return an error")
	require.Equal(t, 1, len(files), "os.ReadDir should return one file")
	backupDirEntry := files[0]
	require.Equal(t, true, backupDirEntry.IsDir(), "os.ReadDir should return a directory")
	backupDirPath := path.Join(backupsPath, backupsDirectoryName, backupDirEntry.Name())

	// Get the path database directory.
	backupDatabaseDirPath := path.Join(backupDirPath, DatabaseDirName)

	// Compare the content of the slashing protection directory.
	require.Equal(t, true, file.DirsEqual(originalDatabaseDirPath, backupDatabaseDirPath))
}

func TestStore_UpdatePublickKeysBuckets(t *testing.T) {
	// Get a database path.
	databasePath := t.TempDir()

	// Create some pubkeys.
	pubkeys := getPubKeys(t, 5)

	// Create a new store.
	s, err := NewStore(databasePath, &Config{PubKeys: pubkeys})
	require.NoError(t, err, "NewStore should not return an error")

	// Update the public keys.
	err = s.UpdatePublicKeysBuckets(pubkeys)
	require.NoError(t, err, "UpdatePublicKeysBuckets should not return an error")

	// Check if the public keys files have been created.
	for i := range pubkeys {
		pubkeyHex := hexutil.Encode(pubkeys[i][:])
		pubkeyFile := path.Join(databasePath, DatabaseDirName, slashingProtectionDirName, fmt.Sprintf("%s.yaml", pubkeyHex))

		exists, err := file.Exists(pubkeyFile, file.Regular)
		require.NoError(t, err, "file.Exists should not return an error")
		require.Equal(t, true, exists, "file.Exists should return true")
	}
}

func TestStore_slashingProtectionDirPath(t *testing.T) {
	// Get a database path.
	databasePath := t.TempDir()

	// Create a new store.
	s, err := NewStore(databasePath, nil)
	require.NoError(t, err, "NewStore should not return an error")

	// Check the slashing protection directory path.
	expected := path.Join(databasePath, DatabaseDirName, slashingProtectionDirName)
	actual := s.slashingProtectionDirPath()
	require.Equal(t, expected, actual)
}

func TestStore_pubkeySlashingProtectionFilePath(t *testing.T) {
	// Get a database path.
	databasePath := t.TempDir()

	// Create a new store.
	s, err := NewStore(databasePath, nil)
	require.NoError(t, err, "NewStore should not return an error")

	// Create a pubkey.
	pubkey := getPubKeys(t, 1)[0]

	// Check the pubkey slashing protection file path.
	expected := path.Join(databasePath, DatabaseDirName, slashingProtectionDirName, hexutil.Encode(pubkey[:])+".yaml")
	actual := s.pubkeySlashingProtectionFilePath(pubkey)
	require.Equal(t, path.Join(databasePath, DatabaseDirName, slashingProtectionDirName, hexutil.Encode(pubkey[:])+".yaml"), s.pubkeySlashingProtectionFilePath(pubkey))
	require.Equal(t, expected, actual)
}

func TestStore_configurationFilePath(t *testing.T) {
	// Get a database path.
	databasePath := t.TempDir()

	// Create a new store.
	s, err := NewStore(databasePath, nil)
	require.NoError(t, err, "NewStore should not return an error")

	// Check the configuration file path.
	expected := path.Join(databasePath, DatabaseDirName, configurationFileName)
	actual := s.configurationFilePath()
	require.Equal(t, expected, actual)
}

func TestStore_configuration_saveConfiguration(t *testing.T) {
	for _, tt := range []struct {
		name                  string
		expectedConfiguration *Configuration
	}{
		{
			name:                  "nil configuration",
			expectedConfiguration: nil,
		},
		{
			name:                  "some configuration",
			expectedConfiguration: &Configuration{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Create a database path.
			databasePath := t.TempDir()

			// Create a new store.
			s, err := NewStore(databasePath, nil)
			require.NoError(t, err, "NewStore should not return an error")

			// Save the configuration.
			err = s.saveConfiguration(tt.expectedConfiguration)
			require.NoError(t, err, "saveConfiguration should not return an error")

			// Retrieve the configuration.
			actualConfiguration, err := s.configuration()
			require.NoError(t, err, "configuration should not return an error")

			// Compare the configurations.
			require.DeepEqual(t, tt.expectedConfiguration, actualConfiguration)
		})
	}

}

func TestStore_validatorSlashingProtection_saveValidatorSlashingProtection(t *testing.T) {
	// We get a database path
	databasePath := t.TempDir()

	// We create a new store
	s, err := NewStore(databasePath, nil)
	require.NoError(t, err, "NewStore should not return an error")

	// We create a pubkey
	pubkey := getPubKeys(t, 1)[0]

	// We save an empty validator slashing protection for the pubkey
	err = s.saveValidatorSlashingProtection(pubkey, nil)
	require.NoError(t, err, "saveValidatorSlashingProtection should not return an error")

	// We check the validator slashing protection for the pubkey
	var expected *ValidatorSlashingProtection
	actual, err := s.validatorSlashingProtection(pubkey)
	require.NoError(t, err, "validatorSlashingProtection should not return an error")
	require.Equal(t, expected, actual)

	// We update the validator slashing protection for the pubkey
	epoch := uint64(1)
	validatorSlashingProtection := &ValidatorSlashingProtection{LatestSignedBlockSlot: &epoch}
	err = s.saveValidatorSlashingProtection(pubkey, validatorSlashingProtection)
	require.NoError(t, err, "saveValidatorSlashingProtection should not return an error")

	// We check the validator slashing protection for the pubkey
	expected = &ValidatorSlashingProtection{LatestSignedBlockSlot: &epoch}
	actual, err = s.validatorSlashingProtection(pubkey)
	require.NoError(t, err, "validatorSlashingProtection should not return an error")
	require.DeepEqual(t, expected, actual)
}

func TestStore_publicKeys(t *testing.T) {
	// We get a database path
	databasePath := t.TempDir()

	// We create some pubkeys
	pubkeys := getPubKeys(t, 5)

	// We create a new store
	s, err := NewStore(databasePath, &Config{PubKeys: pubkeys})
	require.NoError(t, err, "NewStore should not return an error")

	// We check the public keys
	expected := pubkeys
	actual, err := s.publicKeys()
	require.NoError(t, err, "publicKeys should not return an error")

	// We cannot compare the slices directly because the order is not guaranteed,
	// so we compare sets instead.

	expectedSet := make(map[[fieldparams.BLSPubkeyLength]byte]bool)
	for _, pubkey := range expected {
		expectedSet[pubkey] = true
	}

	actualSet := make(map[[fieldparams.BLSPubkeyLength]byte]bool)
	for _, pubkey := range actual {
		actualSet[pubkey] = true
	}

	require.DeepEqual(t, expectedSet, actualSet)
}
