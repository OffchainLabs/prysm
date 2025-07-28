package kv

import (
	"os"
	"path/filepath"
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/validator/db/common"
)

func TestStore_Backup(t *testing.T) {
	db := setupDB(t, nil)
	ctx := t.Context()
	root := [32]byte{1}
	require.NoError(t, db.SaveGenesisValidatorsRoot(ctx, root[:]))
	require.NoError(t, db.Backup(ctx, "", true))

	backupsPath := filepath.Join(db.databasePath, backupsDirectoryName)
	files, err := os.ReadDir(backupsPath)
	require.NoError(t, err)
	require.NotEqual(t, 0, len(files), "No backups created")
	require.NoError(t, db.Close(), "Failed to close database")

	oldFilePath := filepath.Join(backupsPath, files[0].Name())
	newFilePath := filepath.Join(backupsPath, ProtectionDbFileName)
	// We rename the file to match the database file name
	// our NewKVStore function expects when opening a database.
	require.NoError(t, os.Rename(oldFilePath, newFilePath))

	backedDB, err := NewKVStore(ctx, backupsPath, &Config{})
	require.NoError(t, err, "Failed to instantiate DB")
	t.Cleanup(func() {
		require.NoError(t, backedDB.Close(), "Failed to close database")
	})
	genesisRoot, err := backedDB.GenesisValidatorsRoot(ctx)
	require.NoError(t, err)
	require.DeepEqual(t, root[:], genesisRoot)
}

func TestStore_NestedBackup(t *testing.T) {
	keys := [][fieldparams.BLSPubkeyLength]byte{{'A'}, {'B'}}
	db := setupDB(t, keys)
	ctx := t.Context()
	root := [32]byte{1}
	idxAtt := &ethpb.IndexedAttestation{
		AttestingIndices: nil,
		Data: &ethpb.AttestationData{
			Slot:            0,
			CommitteeIndex:  0,
			BeaconBlockRoot: root[:],
			Source: &ethpb.Checkpoint{
				Epoch: 10,
				Root:  root[:],
			},
			Target: &ethpb.Checkpoint{
				Epoch: 0,
				Root:  root[:],
			},
		},
		Signature: make([]byte, 96),
	}
	require.NoError(t, db.SaveGenesisValidatorsRoot(ctx, root[:]))
	require.NoError(t, db.SaveAttestationForPubKey(t.Context(), keys[0], [32]byte{'C'}, idxAtt))
	require.NoError(t, db.SaveAttestationForPubKey(t.Context(), keys[1], [32]byte{'C'}, idxAtt))
	require.NoError(t, db.Backup(ctx, "", true))

	backupsPath := filepath.Join(db.databasePath, backupsDirectoryName)
	files, err := os.ReadDir(backupsPath)
	require.NoError(t, err)
	require.NotEqual(t, 0, len(files), "No backups created")
	require.NoError(t, db.Close(), "Failed to close database")

	oldFilePath := filepath.Join(backupsPath, files[0].Name())
	newFilePath := filepath.Join(backupsPath, ProtectionDbFileName)
	// We rename the file to match the database file name
	// our NewKVStore function expects when opening a database.
	require.NoError(t, os.Rename(oldFilePath, newFilePath))

	backedDB, err := NewKVStore(ctx, backupsPath, &Config{})
	require.NoError(t, err, "Failed to instantiate DB")
	t.Cleanup(func() {
		require.NoError(t, backedDB.Close(), "Failed to close database")
	})
	genesisRoot, err := backedDB.GenesisValidatorsRoot(ctx)
	require.NoError(t, err)
	require.DeepEqual(t, root[:], genesisRoot)

	signingRoot32 := [32]byte{'C'}

	hist, err := backedDB.AttestationHistoryForPubKey(t.Context(), keys[0])
	require.NoError(t, err)
	require.DeepEqual(t, &common.AttestationRecord{
		PubKey:      keys[0],
		Source:      10,
		Target:      0,
		SigningRoot: signingRoot32[:],
	}, hist[0])

	hist, err = backedDB.AttestationHistoryForPubKey(t.Context(), keys[1])
	require.NoError(t, err)
	require.DeepEqual(t, &common.AttestationRecord{
		PubKey:      keys[1],
		Source:      10,
		Target:      0,
		SigningRoot: signingRoot32[:],
	}, hist[0])

	ep, exists, err := backedDB.LowestSignedSourceEpoch(t.Context(), keys[0])
	require.NoError(t, err)
	require.Equal(t, true, exists)
	require.Equal(t, 10, int(ep))

	ep, exists, err = backedDB.LowestSignedSourceEpoch(t.Context(), keys[1])
	require.NoError(t, err)
	require.Equal(t, true, exists)
	require.Equal(t, 10, int(ep))
}
