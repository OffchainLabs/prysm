package kv

import (
	"context"
	"testing"

	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/crypto/hash"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestStore_GraffitiOrderedIndex_ReadAndWrite(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t, [][fieldparams.BLSPubkeyLength]byte{})
	tests := []struct {
		name     string
		want     uint64
		write    uint64
		fileHash [32]byte
	}{
		{
			name:     "empty then write",
			want:     0,
			write:    15,
			fileHash: hash.Hash([]byte("one")),
		},
		{
			name:     "update with same file hash",
			want:     15,
			write:    20,
			fileHash: hash.Hash([]byte("one")),
		},
		{
			name:     "continued updates",
			want:     20,
			write:    21,
			fileHash: hash.Hash([]byte("one")),
		},
		{
			name:     "reset with new file hash",
			want:     0,
			write:    10,
			fileHash: hash.Hash([]byte("two")),
		},
		{
			name:     "read with new file hash",
			want:     10,
			fileHash: hash.Hash([]byte("two")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := db.GraffitiOrderedIndex(ctx, tt.fileHash)
			require.NoError(t, err)
			require.DeepEqual(t, tt.want, got)
			err = db.SaveGraffitiOrderedIndex(ctx, tt.write)
			require.NoError(t, err)
		})
	}
}

func TestStore_GraffitiFileHash(t *testing.T) {
	ctx := context.Background()

	// Creates database
	db := setupDB(t, [][fieldparams.BLSPubkeyLength]byte{})

	tests := []struct {
		name             string
		write            *[32]byte
		expectedExists   bool
		expectedFileHash [32]byte
	}{
		{
			name:             "empty",
			write:            nil,
			expectedExists:   false,
			expectedFileHash: [32]byte{0},
		},
		{
			name:             "existing",
			write:            &[32]byte{1},
			expectedExists:   true,
			expectedFileHash: [32]byte{1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.write != nil {
				// Call to GraffitiOrderedIndex set a graffiti file hash.
				_, err := db.GraffitiOrderedIndex(ctx, *tt.write)
				require.NoError(t, err)
			}

			// Retrieve the graffiti file hash.
			actualFileHash, actualExists, err := db.GraffitiFileHash()
			require.NoError(t, err)
			require.Equal(t, tt.expectedExists, actualExists)

			if tt.expectedExists {
				require.Equal(t, tt.expectedFileHash, actualFileHash)
			}
		})
	}
}
