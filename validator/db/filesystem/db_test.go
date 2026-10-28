package filesystem

import (
	"fmt"
	"os"
	"path"
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/proposer"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/io/file"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
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
	t.Run("success", func(t *testing.T) {
		pubkeys := getPubKeys(t, 5)
		_, err := NewStore(t.TempDir(), &Config{PubKeys: pubkeys})
		require.NoError(t, err, "NewStore should not return an error")
	})

	t.Run("initialization error", func(t *testing.T) {
		blocker := path.Join(t.TempDir(), "blocked-parent")
		const contents = "existing regular file\n"
		require.NoError(t, os.WriteFile(blocker, []byte(contents), 0o600))

		s, err := NewStore(blocker, &Config{PubKeys: getPubKeys(t, 1)})
		require.ErrorContains(t, "could not create directory", err)
		require.IsNil(t, s)
		requireProtectionBytes(t, blocker, contents)
	})

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
	err = s.SaveProposerSettings(t.Context(), &proposer.Settings{
		DefaultConfig: &proposer.Option{
			FeeRecipientConfig: &proposer.FeeRecipientConfig{
				FeeRecipient: common.Address{},
			},
		},
	})
	require.NoError(t, err, "SaveProposerSettings should not return an error")

	// Backup the DB.
	require.NoError(t, s.Backup(t.Context(), backupsPath, true), "Backup should not return an error")

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

func invalidProtectionRecords() []struct{ name, data string } {
	return []struct{ name, data string }{
		{"empty", ""},
		{"whitespace", " \n\t\n"},
		{"comments", "# protection\n# missing data\n"},
		{"null", "null\n"},
		{"tilde", "~\n"},
		{"empty mapping", "{}\n"},
		{"missing source", "latestSignedBlockSlot: 12\nlastSignedAttestationTargetEpoch: 4\n"},
		{"unknown only", "futureField: 1\n"},
		{"null source", "lastSignedAttestationSourceEpoch: null\n"},
		{"malformed", "lastSignedAttestationSourceEpoch: [\n"},
		{"sequence", "- lastSignedAttestationSourceEpoch: 0\n"},
		{"scalar", "42\n"},
		{"string source", "lastSignedAttestationSourceEpoch: text\n"},
		{"fractional source", "lastSignedAttestationSourceEpoch: 1.5\n"},
		{"floating source", "lastSignedAttestationSourceEpoch: 1.0\n"},
		{"mapping source", "lastSignedAttestationSourceEpoch: {}\n"},
		{"boolean source", "lastSignedAttestationSourceEpoch: true\n"},
		{"negative source", "lastSignedAttestationSourceEpoch: -1\n"},
		{"overflow source", "lastSignedAttestationSourceEpoch: 18446744073709551616\n"},
		{"duplicate source", "lastSignedAttestationSourceEpoch: 0\nlastSignedAttestationSourceEpoch: 1\n"},
		{"duplicate slot", "lastSignedAttestationSourceEpoch: 0\nlatestSignedBlockSlot: 1\nlatestSignedBlockSlot: 2\n"},
		{"duplicate target", "lastSignedAttestationSourceEpoch: 0\nlastSignedAttestationTargetEpoch: 1\nlastSignedAttestationTargetEpoch: 2\n"},
		{"invalid slot", "lastSignedAttestationSourceEpoch: 0\nlatestSignedBlockSlot: text\n"},
		{"overflow slot", "lastSignedAttestationSourceEpoch: 0\nlatestSignedBlockSlot: 18446744073709551616\n"},
		{"overflow target", "lastSignedAttestationSourceEpoch: 0\nlastSignedAttestationTargetEpoch: 18446744073709551616\n"},
		{"invalid target", "lastSignedAttestationSourceEpoch: 0\nlastSignedAttestationTargetEpoch: -1\n"},
		{"multiple documents", "lastSignedAttestationSourceEpoch: 0\n---\nlastSignedAttestationSourceEpoch: 1\n"},
		{"empty second document", "lastSignedAttestationSourceEpoch: 0\n---\n"},
		{"malformed second document", "lastSignedAttestationSourceEpoch: 0\n---\n[\n"},
		{"direct merge", "<<: {lastSignedAttestationSourceEpoch: 0}\n"},
		{"alias merge", "defaults: &defaults {lastSignedAttestationSourceEpoch: 0}\n<<: *defaults\n"},
		{"sequence merge", "<<: [{lastSignedAttestationSourceEpoch: 0}]\n"},
		{"alias source key", "futureField: &source lastSignedAttestationSourceEpoch\n*source: 0\n"},
		{"nonstring key", "lastSignedAttestationSourceEpoch: 0\n42: ignored\n"},
		{"sequence key", "lastSignedAttestationSourceEpoch: 0\n? [future, key]\n: ignored\n"},
	}
}

func writeProtectionFixture(t *testing.T, s *Store, pubkey [fieldparams.BLSPubkeyLength]byte, data string) string {
	t.Helper()
	require.NoError(t, file.MkdirAll(s.slashingProtectionDirPath()))
	filename := s.pubkeySlashingProtectionFilePath(pubkey)
	require.NoError(t, file.WriteFile(filename, []byte(data)))
	return filename
}
func requireProtectionBytes(t *testing.T, filename, expected string) {
	t.Helper()
	actual, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Equal(t, expected, string(actual), "protection data must not be replaced")
}
func TestStore_validatorSlashingProtection_InvalidRecords(t *testing.T) {
	pubkey := getPubKeys(t, 1)[0]
	for _, tt := range invalidProtectionRecords() {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewStore(t.TempDir(), nil)
			require.NoError(t, err)
			filename := writeProtectionFixture(t, s, pubkey, tt.data)
			actual, err := s.validatorSlashingProtection(pubkey)
			require.ErrorContains(t, filename, err)
			require.Equal(t, (*ValidatorSlashingProtection)(nil), actual)
			requireProtectionBytes(t, filename, tt.data)
		})
	}
}
func TestStore_validatorSlashingProtection_ValidRecords(t *testing.T) {
	zero, slot, target := uint64(0), uint64(12), uint64(4)
	for _, tt := range []struct {
		name, data string
		want       *ValidatorSlashingProtection
	}{
		{"fresh", "lastSignedAttestationSourceEpoch: 0\n", &ValidatorSlashingProtection{}},
		{"null optionals", "lastSignedAttestationSourceEpoch: 0\nlatestSignedBlockSlot: null\nlastSignedAttestationTargetEpoch: null\n", &ValidatorSlashingProtection{}},
		{"zero optionals", "lastSignedAttestationSourceEpoch: 0\nlatestSignedBlockSlot: 0\nlastSignedAttestationTargetEpoch: 0\n", &ValidatorSlashingProtection{LatestSignedBlockSlot: &zero, LastSignedAttestationTargetEpoch: &zero}},
		{"history", "lastSignedAttestationSourceEpoch: 3\nlatestSignedBlockSlot: 12\nlastSignedAttestationTargetEpoch: 4\n", &ValidatorSlashingProtection{LatestSignedBlockSlot: &slot, LastSignedAttestationSourceEpoch: 3, LastSignedAttestationTargetEpoch: &target}},
		{"unknown field", "lastSignedAttestationSourceEpoch: 0\nfutureField: {value: 1}\n", &ValidatorSlashingProtection{}},
		{"maximum source", "lastSignedAttestationSourceEpoch: 18446744073709551615\n", &ValidatorSlashingProtection{LastSignedAttestationSourceEpoch: ^uint64(0)}},
		{"quoted merge key", "lastSignedAttestationSourceEpoch: 0\n\"<<\": {futureField: 1}\n", &ValidatorSlashingProtection{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewStore(t.TempDir(), nil)
			require.NoError(t, err)
			pubkey := getPubKeys(t, 1)[0]
			filename := writeProtectionFixture(t, s, pubkey, tt.data)
			reopened, err := NewStore(s.DatabasePath(), nil)
			require.NoError(t, err)
			actual, err := reopened.validatorSlashingProtection(pubkey)
			require.NoError(t, err)
			require.DeepEqual(t, tt.want, actual)
			requireProtectionBytes(t, filename, tt.data)
		})
	}
}
func TestStore_validatorSlashingProtection_MissingRecords(t *testing.T) {
	for _, existingDirectory := range []bool{false, true} {
		t.Run(fmt.Sprintf("directory exists %t", existingDirectory), func(t *testing.T) {
			s, err := NewStore(t.TempDir(), nil)
			require.NoError(t, err)
			if existingDirectory {
				require.NoError(t, file.MkdirAll(s.slashingProtectionDirPath()))
			}
			pubkey := getPubKeys(t, 1)[0]
			require.NoError(t, s.saveValidatorSlashingProtection(pubkey, nil))
			actual, err := s.validatorSlashingProtection(pubkey)
			require.NoError(t, err)
			require.Equal(t, (*ValidatorSlashingProtection)(nil), actual)
			_, err = os.Stat(s.pubkeySlashingProtectionFilePath(pubkey))
			require.Equal(t, true, os.IsNotExist(err))
		})
	}
}
func TestStore_validatorSlashingProtection_GeneratedFreshRecord(t *testing.T) {
	pubkeys := getPubKeys(t, 1)
	directory := t.TempDir()
	_, err := NewStore(directory, &Config{PubKeys: pubkeys})
	require.NoError(t, err)
	reopened, err := NewStore(directory, nil)
	require.NoError(t, err)
	actual, err := reopened.validatorSlashingProtection(pubkeys[0])
	require.NoError(t, err)
	require.DeepEqual(t, &ValidatorSlashingProtection{}, actual)
}
