package filesystem

import (
	"context"
	"testing"

	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestStore_SaveGraffitiOrderedIndex(t *testing.T) {
	graffitiOrderedIndex := uint64(42)

	for _, tt := range []struct {
		name          string
		configuration *Configuration
	}{
		{name: "nil configuration", configuration: nil},
		{name: "configuration without graffiti", configuration: &Configuration{}},
		{name: "configuration with graffiti", configuration: &Configuration{Graffiti: &Graffiti{}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new store.
			store, err := NewStore(t.TempDir(), nil)
			require.NoError(t, err)

			// Save configuration.
			err = store.saveConfiguration(tt.configuration)
			require.NoError(t, err)

			// Save graffiti ordered index.
			err = store.SaveGraffitiOrderedIndex(context.Background(), graffitiOrderedIndex)
			require.NoError(t, err)
		})
	}
}

func TestStore_GraffitiOrderedIndex(t *testing.T) {
	FileHash1 := [fieldparams.RootLength]byte{1}
	FileHash1Str := "0x0100000000000000000000000000000000000000000000000000000000000000"
	FileHash2Str := "0x0200000000000000000000000000000000000000000000000000000000000000"

	for _, tt := range []struct {
		name                         string
		configuration                *Configuration
		fileHash                     [fieldparams.RootLength]byte
		expectedGraffitiOrderedIndex uint64
	}{
		{
			name:                         "nil configuration saved",
			configuration:                nil,
			fileHash:                     FileHash1,
			expectedGraffitiOrderedIndex: 0,
		},
		{
			name:                         "configuration without graffiti saved",
			configuration:                &Configuration{},
			fileHash:                     FileHash1,
			expectedGraffitiOrderedIndex: 0,
		},
		{
			name:                         "graffiti without graffiti file hash saved",
			configuration:                &Configuration{Graffiti: &Graffiti{FileHash: nil}},
			fileHash:                     FileHash1,
			expectedGraffitiOrderedIndex: 0,
		},
		{
			name:                         "graffiti with different graffiti file hash saved",
			configuration:                &Configuration{Graffiti: &Graffiti{OrderedIndex: 42, FileHash: &FileHash2Str}},
			fileHash:                     FileHash1,
			expectedGraffitiOrderedIndex: 0,
		},
		{
			name:                         "graffiti with same graffiti file hash saved",
			configuration:                &Configuration{Graffiti: &Graffiti{OrderedIndex: 42, FileHash: &FileHash1Str}},
			fileHash:                     FileHash1,
			expectedGraffitiOrderedIndex: 42,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new store.
			store, err := NewStore(t.TempDir(), nil)
			require.NoError(t, err)

			// Save configuration.
			err = store.saveConfiguration(tt.configuration)
			require.NoError(t, err)

			// Get graffiti ordered index.
			actualGraffitiOrderedIndex, err := store.GraffitiOrderedIndex(context.Background(), tt.fileHash)
			require.NoError(t, err)
			require.Equal(t, tt.expectedGraffitiOrderedIndex, actualGraffitiOrderedIndex)
		})
	}
}

func TestStore_GraffitiFileHash(t *testing.T) {
	fileHashStr := "0x0100000000000000000000000000000000000000000000000000000000000000"

	for _, tt := range []struct {
		name             string
		configuration    *Configuration
		expectedExists   bool
		expectedFileHash [fieldparams.RootLength]byte
	}{
		{
			name:             "nil configuration saved",
			configuration:    nil,
			expectedExists:   false,
			expectedFileHash: [fieldparams.RootLength]byte{0},
		},
		{
			name:             "configuration without graffiti saved",
			configuration:    &Configuration{},
			expectedExists:   false,
			expectedFileHash: [fieldparams.RootLength]byte{0},
		},
		{
			name:             "graffiti without graffiti file hash saved",
			configuration:    &Configuration{Graffiti: &Graffiti{FileHash: nil}},
			expectedExists:   false,
			expectedFileHash: [fieldparams.RootLength]byte{0},
		},
		{
			name:             "graffiti with graffiti file hash saved",
			configuration:    &Configuration{Graffiti: &Graffiti{FileHash: &fileHashStr}},
			expectedExists:   true,
			expectedFileHash: [fieldparams.RootLength]byte{1},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new store.
			store, err := NewStore(t.TempDir(), nil)
			require.NoError(t, err)

			// Save configuration.
			err = store.saveConfiguration(tt.configuration)
			require.NoError(t, err)

			// Get graffiti file hash.
			actualFileHash, actualExists, err := store.GraffitiFileHash()
			require.NoError(t, err)
			require.Equal(t, tt.expectedExists, actualExists)

			if tt.expectedExists {
				require.Equal(t, tt.expectedFileHash, actualFileHash)
			}
		})
	}
}
