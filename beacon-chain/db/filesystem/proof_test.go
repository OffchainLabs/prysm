package filesystem

import (
	"encoding/binary"
	"os"
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/spf13/afero"
)

func createTestProof(t *testing.T, slot primitives.Slot, proofID uint64, blockRoot [32]byte) *ethpb.ExecutionProof {
	t.Helper()

	return &ethpb.ExecutionProof{
		ProofId:   primitives.ExecutionProofId(proofID),
		Slot:      slot,
		BlockHash: make([]byte, 32),
		BlockRoot: blockRoot[:],
		ProofData: []byte("test proof data for proofID " + string(rune('0'+proofID))),
	}
}

// assertProofsEqual compares two proofs by comparing their SSZ-encoded bytes.
func assertProofsEqual(t *testing.T, expected, actual *ethpb.ExecutionProof) {
	t.Helper()

	expectedSSZ, err := expected.MarshalSSZ()
	require.NoError(t, err)
	actualSSZ, err := actual.MarshalSSZ()
	require.NoError(t, err)
	require.DeepEqual(t, expectedSSZ, actualSSZ)
}

func TestNewProofStorage(t *testing.T) {
	ctx := t.Context()

	t.Run("No base path", func(t *testing.T) {
		_, err := NewProofStorage(ctx)
		require.ErrorIs(t, err, errNoProofBasePath)
	})

	t.Run("Nominal", func(t *testing.T) {
		dir := t.TempDir()

		storage, err := NewProofStorage(ctx, WithProofBasePath(dir))
		require.NoError(t, err)
		require.Equal(t, dir, storage.base)
	})
}

func TestProofSaveAndGet(t *testing.T) {
	t.Run("proof ID too large", func(t *testing.T) {
		_, proofStorage := NewEphemeralProofStorageAndFs(t)

		proof := &ethpb.ExecutionProof{
			ProofId:   primitives.ExecutionProofId(maxProofTypes), // too large
			Slot:      1,
			BlockHash: make([]byte, 32),
			BlockRoot: make([]byte, 32),
			ProofData: []byte("test"),
		}

		err := proofStorage.Save([]*ethpb.ExecutionProof{proof})
		require.ErrorIs(t, err, errProofIDTooLarge)
	})

	t.Run("save empty slice", func(t *testing.T) {
		_, proofStorage := NewEphemeralProofStorageAndFs(t)

		err := proofStorage.Save([]*ethpb.ExecutionProof{})
		require.NoError(t, err)
	})

	t.Run("save and get single proof", func(t *testing.T) {
		_, proofStorage := NewEphemeralProofStorageAndFs(t)

		blockRoot := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
		proof := createTestProof(t, 32, 2, blockRoot)

		err := proofStorage.Save([]*ethpb.ExecutionProof{proof})
		require.NoError(t, err)

		// Check summary
		summary := proofStorage.Summary(blockRoot)
		require.Equal(t, true, summary.HasProof(2))
		require.Equal(t, false, summary.HasProof(0))
		require.Equal(t, false, summary.HasProof(1))
		require.Equal(t, 1, summary.Count())

		// Get the proof
		proofs, err := proofStorage.Get(blockRoot, []uint64{2})
		require.NoError(t, err)
		require.Equal(t, 1, len(proofs))
		assertProofsEqual(t, proof, proofs[0])
	})

	t.Run("save and get multiple proofs", func(t *testing.T) {
		_, proofStorage := NewEphemeralProofStorageAndFs(t)

		blockRoot := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}

		// Save first proof
		proof1 := createTestProof(t, 32, 0, blockRoot)
		err := proofStorage.Save([]*ethpb.ExecutionProof{proof1})
		require.NoError(t, err)

		// Save second proof (should append to existing file)
		proof2 := createTestProof(t, 32, 3, blockRoot)
		err = proofStorage.Save([]*ethpb.ExecutionProof{proof2})
		require.NoError(t, err)

		// Save third proof
		proof3 := createTestProof(t, 32, 7, blockRoot)
		err = proofStorage.Save([]*ethpb.ExecutionProof{proof3})
		require.NoError(t, err)

		// Check summary
		summary := proofStorage.Summary(blockRoot)
		require.Equal(t, true, summary.HasProof(0))
		require.Equal(t, false, summary.HasProof(1))
		require.Equal(t, false, summary.HasProof(2))
		require.Equal(t, true, summary.HasProof(3))
		require.Equal(t, false, summary.HasProof(4))
		require.Equal(t, false, summary.HasProof(5))
		require.Equal(t, false, summary.HasProof(6))
		require.Equal(t, true, summary.HasProof(7))
		require.Equal(t, 3, summary.Count())

		// Get all proofs
		proofs, err := proofStorage.Get(blockRoot, nil)
		require.NoError(t, err)
		require.Equal(t, 3, len(proofs))

		// Get specific proofs
		proofs, err = proofStorage.Get(blockRoot, []uint64{0, 3})
		require.NoError(t, err)
		require.Equal(t, 2, len(proofs))
		assertProofsEqual(t, proof1, proofs[0])
		assertProofsEqual(t, proof2, proofs[1])
	})

	t.Run("duplicate proof is ignored", func(t *testing.T) {
		_, proofStorage := NewEphemeralProofStorageAndFs(t)

		blockRoot := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
		proof := createTestProof(t, 32, 2, blockRoot)

		// Save first time
		err := proofStorage.Save([]*ethpb.ExecutionProof{proof})
		require.NoError(t, err)

		// Save same proof again (should be silently ignored)
		err = proofStorage.Save([]*ethpb.ExecutionProof{proof})
		require.NoError(t, err)

		// Check count
		summary := proofStorage.Summary(blockRoot)
		require.Equal(t, 1, summary.Count())

		// Get the proof
		proofs, err := proofStorage.Get(blockRoot, nil)
		require.NoError(t, err)
		require.Equal(t, 1, len(proofs))
	})

	t.Run("get non-existent root", func(t *testing.T) {
		_, proofStorage := NewEphemeralProofStorageAndFs(t)

		proofs, err := proofStorage.Get([fieldparams.RootLength]byte{1}, []uint64{0, 1, 2})
		require.NoError(t, err)
		require.Equal(t, 0, len(proofs))
	})

	t.Run("get non-existent proofIDs", func(t *testing.T) {
		_, proofStorage := NewEphemeralProofStorageAndFs(t)

		blockRoot := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
		proof := createTestProof(t, 32, 2, blockRoot)

		err := proofStorage.Save([]*ethpb.ExecutionProof{proof})
		require.NoError(t, err)

		// Try to get proofIDs that don't exist
		proofs, err := proofStorage.Get(blockRoot, []uint64{0, 1, 3, 4})
		require.NoError(t, err)
		require.Equal(t, 0, len(proofs))
	})
}

func TestProofRemove(t *testing.T) {
	t.Run("remove non-existent", func(t *testing.T) {
		_, proofStorage := NewEphemeralProofStorageAndFs(t)
		err := proofStorage.Remove([fieldparams.RootLength]byte{1})
		require.NoError(t, err)
	})

	t.Run("remove existing", func(t *testing.T) {
		_, proofStorage := NewEphemeralProofStorageAndFs(t)

		blockRoot1 := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
		blockRoot2 := [32]byte{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}

		proof1 := createTestProof(t, 32, 0, blockRoot1)
		proof2 := createTestProof(t, 64, 1, blockRoot2)

		err := proofStorage.Save([]*ethpb.ExecutionProof{proof1})
		require.NoError(t, err)
		err = proofStorage.Save([]*ethpb.ExecutionProof{proof2})
		require.NoError(t, err)

		// Remove first proof
		err = proofStorage.Remove(blockRoot1)
		require.NoError(t, err)

		// Check first proof is gone
		summary := proofStorage.Summary(blockRoot1)
		require.Equal(t, 0, summary.Count())

		proofs, err := proofStorage.Get(blockRoot1, nil)
		require.NoError(t, err)
		require.Equal(t, 0, len(proofs))

		// Check second proof still exists
		summary = proofStorage.Summary(blockRoot2)
		require.Equal(t, 1, summary.Count())

		proofs, err = proofStorage.Get(blockRoot2, nil)
		require.NoError(t, err)
		require.Equal(t, 1, len(proofs))
	})
}

func TestProofClear(t *testing.T) {
	_, proofStorage := NewEphemeralProofStorageAndFs(t)

	blockRoot1 := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	blockRoot2 := [32]byte{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}

	proof1 := createTestProof(t, 32, 0, blockRoot1)
	proof2 := createTestProof(t, 64, 1, blockRoot2)

	err := proofStorage.Save([]*ethpb.ExecutionProof{proof1})
	require.NoError(t, err)
	err = proofStorage.Save([]*ethpb.ExecutionProof{proof2})
	require.NoError(t, err)

	// Clear all
	err = proofStorage.Clear()
	require.NoError(t, err)

	// Check both are gone
	summary := proofStorage.Summary(blockRoot1)
	require.Equal(t, 0, summary.Count())

	summary = proofStorage.Summary(blockRoot2)
	require.Equal(t, 0, summary.Count())
}

func TestProofWarmCache(t *testing.T) {
	fs, proofStorage := NewEphemeralProofStorageAndFs(t)

	blockRoot1 := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	blockRoot2 := [32]byte{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}

	// Save proofs
	proof1a := createTestProof(t, 32, 0, blockRoot1)
	proof1b := createTestProof(t, 32, 3, blockRoot1)
	proof2 := createTestProof(t, 64, 5, blockRoot2)

	err := proofStorage.Save([]*ethpb.ExecutionProof{proof1a})
	require.NoError(t, err)
	err = proofStorage.Save([]*ethpb.ExecutionProof{proof1b})
	require.NoError(t, err)
	err = proofStorage.Save([]*ethpb.ExecutionProof{proof2})
	require.NoError(t, err)

	// Verify files exist
	files, err := afero.ReadDir(fs, "0/1")
	require.NoError(t, err)
	require.Equal(t, 1, len(files))

	files, err = afero.ReadDir(fs, "0/2")
	require.NoError(t, err)
	require.Equal(t, 1, len(files))

	// Create a new storage with the same filesystem
	proofStorage2 := NewEphemeralProofStorageUsingFs(t, fs)

	// Before warm cache, cache should be empty
	summary := proofStorage2.Summary(blockRoot1)
	require.Equal(t, 0, summary.Count())

	// Warm cache
	proofStorage2.WarmCache()

	// After warm cache, cache should be populated
	summary = proofStorage2.Summary(blockRoot1)
	require.Equal(t, 2, summary.Count())
	require.Equal(t, true, summary.HasProof(0))
	require.Equal(t, true, summary.HasProof(3))

	summary = proofStorage2.Summary(blockRoot2)
	require.Equal(t, 1, summary.Count())
	require.Equal(t, true, summary.HasProof(5))
}

func TestProofSubscribe(t *testing.T) {
	_, proofStorage := NewEphemeralProofStorageAndFs(t)

	sub, ch := proofStorage.Subscribe()
	defer sub.Unsubscribe()

	blockRoot := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	proof := createTestProof(t, 32, 2, blockRoot)

	err := proofStorage.Save([]*ethpb.ExecutionProof{proof})
	require.NoError(t, err)

	// Should receive notification
	ident := <-ch
	require.Equal(t, blockRoot, ident.BlockRoot)
	require.DeepEqual(t, []uint64{2}, ident.ProofIDs)
	require.Equal(t, primitives.Epoch(1), ident.Epoch)
}

func TestProofReadHeader(t *testing.T) {
	t.Run("wrong version", func(t *testing.T) {
		_, proofStorage := NewEphemeralProofStorageAndFs(t)

		blockRoot := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
		proof := createTestProof(t, 32, 0, blockRoot)

		err := proofStorage.Save([]*ethpb.ExecutionProof{proof})
		require.NoError(t, err)

		// Get the file path
		filePath := proofFilePath(blockRoot, 1)

		// Alter the version
		file, err := proofStorage.fs.OpenFile(filePath, os.O_RDWR, os.FileMode(0600))
		require.NoError(t, err)

		_, err = file.Write([]byte{42}) // wrong version
		require.NoError(t, err)

		// Try to read header
		_, _, err = proofStorage.readHeader(file)
		require.ErrorIs(t, err, errWrongProofVersion)

		err = file.Close()
		require.NoError(t, err)
	})
}

func TestEncodeOffsetTable(t *testing.T) {
	var table proofOffsetTable
	table[0] = proofSlotEntry{offset: 0, size: 100}
	table[3] = proofSlotEntry{offset: 100, size: 200}
	table[7] = proofSlotEntry{offset: 300, size: 300}

	encoded := encodeOffsetTable(table)
	require.Equal(t, proofOffsetTableSize, len(encoded))

	// Decode manually and verify
	var decoded proofOffsetTable
	for i := range decoded {
		pos := i * proofSlotSize
		decoded[i].offset = binary.BigEndian.Uint32(encoded[pos : pos+proofOffsetSize])
		decoded[i].size = binary.BigEndian.Uint32(encoded[pos+proofOffsetSize : pos+proofSlotSize])
	}
	require.Equal(t, table, decoded)
}

func TestProofFilePath(t *testing.T) {
	blockRoot := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	epoch := primitives.Epoch(100)

	path := proofFilePath(blockRoot, epoch)
	require.Equal(t, "0/100/0x0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20.sszs", path)
}

func TestExtractProofFileMetadata(t *testing.T) {
	t.Run("valid path", func(t *testing.T) {
		path := "0/100/0x0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20.sszs"
		metadata, err := extractProofFileMetadata(path)
		require.NoError(t, err)

		expectedRoot := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
		require.Equal(t, uint64(0), metadata.period)
		require.Equal(t, primitives.Epoch(100), metadata.epoch)
		require.Equal(t, expectedRoot, metadata.blockRoot)
	})

	t.Run("invalid path - wrong number of parts", func(t *testing.T) {
		_, err := extractProofFileMetadata("invalid/path.sszs")
		require.ErrorContains(t, "unexpected proof file", err)
	})

	t.Run("invalid path - wrong extension", func(t *testing.T) {
		_, err := extractProofFileMetadata("0/100/0x0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20.txt")
		require.ErrorContains(t, "unexpected extension", err)
	})
}
