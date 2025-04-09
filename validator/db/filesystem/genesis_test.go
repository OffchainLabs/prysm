package filesystem

import (
	"context"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestStore_GenesisValidatorsRoot(t *testing.T) {
	ctx := context.Background()

	genesisValidatorRootString := "0x0100"
	genesisValidatorRootBytes := []byte{1, 0}

	for _, tt := range []struct {
		name                         string
		savedConfiguration           *Configuration
		expectedGenesisValidatorRoot []byte
	}{
		{
			name:                         "configuration is nil",
			savedConfiguration:           nil,
			expectedGenesisValidatorRoot: nil,
		},
		{
			name:                         "configuration.GenesisValidatorsRoot is nil",
			savedConfiguration:           &Configuration{GenesisValidatorsRoot: nil},
			expectedGenesisValidatorRoot: nil,
		},
		{
			name:                         "configuration.GenesisValidatorsRoot is something",
			savedConfiguration:           &Configuration{GenesisValidatorsRoot: &genesisValidatorRootString},
			expectedGenesisValidatorRoot: genesisValidatorRootBytes,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new store.
			store, err := NewStore(t.TempDir(), nil)
			require.NoError(t, err)

			// Save the configuration.
			err = store.saveConfiguration(tt.savedConfiguration)
			require.NoError(t, err, "save configuration should not error")

			// Get genesis validators root.
			actualGenesisValidatorRoot, err := store.GenesisValidatorsRoot(ctx)
			require.NoError(t, err, "get genesis validators root should not error")
			require.DeepEqual(t, tt.expectedGenesisValidatorRoot, actualGenesisValidatorRoot, "genesis validators root should be equal")
		})
	}
}

func TestStore_SaveGenesisValidatorsRoot(t *testing.T) {
	ctx := context.Background()
	genesisValidatorRootString := "0x0100"

	for _, tt := range []struct {
		name                  string
		initialConfiguration  *Configuration
		genesisValidatorRoot  []byte
		expectedConfiguration *Configuration
	}{
		{
			name:                  "genValRoot is nil",
			initialConfiguration:  nil,
			genesisValidatorRoot:  nil,
			expectedConfiguration: nil,
		},
		{
			name:                  "initial configuration is nil",
			initialConfiguration:  nil,
			genesisValidatorRoot:  []byte{1, 0},
			expectedConfiguration: &Configuration{GenesisValidatorsRoot: &genesisValidatorRootString},
		},
		{
			name:                  "initial configuration exists",
			initialConfiguration:  &Configuration{},
			genesisValidatorRoot:  []byte{1, 0},
			expectedConfiguration: &Configuration{GenesisValidatorsRoot: &genesisValidatorRootString},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new store.
			store, err := NewStore(t.TempDir(), nil)
			require.NoError(t, err)

			// Save the initial configuration.
			err = store.saveConfiguration(tt.initialConfiguration)
			require.NoError(t, err, "save configuration should not error")

			// Save genesis validators root.
			err = store.SaveGenesisValidatorsRoot(ctx, tt.genesisValidatorRoot)
			require.NoError(t, err, "save genesis validators root should not error")

			// Get configuration.
			actualConfiguration, err := store.configuration()
			require.NoError(t, err, "get configuration should not error")
			require.DeepEqual(t, tt.expectedConfiguration, actualConfiguration, "configuration should be equal")
		})
	}
}
