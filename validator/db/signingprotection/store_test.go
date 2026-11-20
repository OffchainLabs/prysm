package signingprotection

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

var genesisValidatorsRoot = [32]byte{0xaa}

func pubkeyFromByte(b byte) [PubkeyLength]byte {
	var pubkey [PubkeyLength]byte
	for i := range pubkey {
		pubkey[i] = b
	}

	return pubkey
}

func TestNewStore(t *testing.T) {
	t.Run("creates the directory", func(t *testing.T) {
		parent := t.TempDir()

		store, err := NewStore(parent, genesisValidatorsRoot)
		require.NoError(t, err)
		require.Equal(t, filepath.Join(parent, DatabaseDirName, signingProtectionDirName), store.DirPath())
		require.Equal(t, genesisValidatorsRoot, store.GenesisValidatorsRoot())

		info, err := os.Stat(store.DirPath())
		require.NoError(t, err)
		require.Equal(t, true, info.IsDir())
	})

	t.Run("reopens an existing store", func(t *testing.T) {
		parent := t.TempDir()
		pubkey := pubkeyFromByte(1)

		store, err := NewStore(parent, genesisValidatorsRoot)
		require.NoError(t, err)
		require.NoError(t, store.CheckAndSaveSlotMessage(pubkey, Block, 10))

		reopened, err := NewStore(parent, genesisValidatorsRoot)
		require.NoError(t, err)

		protection, err := reopened.SigningProtection(pubkey)
		require.NoError(t, err)
		require.DeepEqual(t, new(uint64(10)), protection.LastSignedBlockSlot)
	})
}

func TestPublicKeys(t *testing.T) {
	t.Run("empty store", func(t *testing.T) {
		store, err := NewStore(t.TempDir(), genesisValidatorsRoot)
		require.NoError(t, err)

		pubkeys, err := store.PublicKeys()
		require.NoError(t, err)
		require.Equal(t, 0, len(pubkeys))
	})

	t.Run("ignores files that are not signing protection files", func(t *testing.T) {
		store, err := NewStore(t.TempDir(), genesisValidatorsRoot)
		require.NoError(t, err)

		pubkey1, pubkey2 := pubkeyFromByte(1), pubkeyFromByte(2)
		require.NoError(t, store.CheckAndSaveSlotMessage(pubkey1, Block, 1))
		require.NoError(t, store.CheckAndSaveSlotMessage(pubkey2, Block, 1))

		permissions := params.BeaconIoConfig().ReadWritePermissions
		for _, name := range []string{"README.md", "0x01.yaml", "0xzz.yaml", filepath.Base(store.filePath(pubkey1)) + ".tmp-123"} {
			require.NoError(t, os.WriteFile(filepath.Join(store.DirPath(), name), []byte{}, permissions))
		}
		require.NoError(t, os.Mkdir(filepath.Join(store.DirPath(), filepath.Base(store.filePath(pubkeyFromByte(3)))), 0700))

		pubkeys, err := store.PublicKeys()
		require.NoError(t, err)

		actual := make(map[[PubkeyLength]byte]bool, len(pubkeys))
		for _, pubkey := range pubkeys {
			actual[pubkey] = true
		}
		require.DeepEqual(t, map[[PubkeyLength]byte]bool{pubkey1: true, pubkey2: true}, actual)
	})
}

func TestSigningProtection(t *testing.T) {
	pubkey := pubkeyFromByte(1)

	t.Run("unknown public key", func(t *testing.T) {
		store, err := NewStore(t.TempDir(), genesisValidatorsRoot)
		require.NoError(t, err)

		protection, err := store.SigningProtection(pubkey)
		require.NoError(t, err)
		require.DeepEqual(t, &ValidatorSigningProtection{}, protection)
	})

	t.Run("empty file", func(t *testing.T) {
		store, err := NewStore(t.TempDir(), genesisValidatorsRoot)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(store.filePath(pubkey), []byte{}, params.BeaconIoConfig().ReadWritePermissions))

		_, err = store.SigningProtection(pubkey)
		require.ErrorContains(t, "empty file", err)
	})

	t.Run("corrupted file", func(t *testing.T) {
		store, err := NewStore(t.TempDir(), genesisValidatorsRoot)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(store.filePath(pubkey), []byte("lastSignedBlockSlot: [\n"), params.BeaconIoConfig().ReadWritePermissions))

		_, err = store.SigningProtection(pubkey)
		require.ErrorContains(t, "decode", err)
	})

	t.Run("unknown field", func(t *testing.T) {
		store, err := NewStore(t.TempDir(), genesisValidatorsRoot)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(store.filePath(pubkey), []byte("lastSignedBlocSlot: 3\n"), params.BeaconIoConfig().ReadWritePermissions))

		_, err = store.SigningProtection(pubkey)
		require.ErrorContains(t, "lastSignedBlocSlot", err)
	})

	t.Run("missing genesis validators root", func(t *testing.T) {
		store, err := NewStore(t.TempDir(), genesisValidatorsRoot)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(store.filePath(pubkey), []byte("lastSignedBlockSlot: 3\n"), params.BeaconIoConfig().ReadWritePermissions))

		_, err = store.SigningProtection(pubkey)
		require.ErrorContains(t, "decode genesis validators root", err)
	})

	t.Run("invalid genesis validators root", func(t *testing.T) {
		store, err := NewStore(t.TempDir(), genesisValidatorsRoot)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(store.filePath(pubkey), []byte("genesisValidatorsRoot: 0xzz\n"), params.BeaconIoConfig().ReadWritePermissions))

		_, err = store.SigningProtection(pubkey)
		require.ErrorContains(t, "decode genesis validators root", err)
	})

	t.Run("other chain", func(t *testing.T) {
		parent := t.TempDir()

		store, err := NewStore(parent, genesisValidatorsRoot)
		require.NoError(t, err)
		require.NoError(t, store.CheckAndSaveSlotMessage(pubkey, Block, 10))

		other, err := NewStore(parent, [32]byte{0xbb})
		require.NoError(t, err)

		_, err = other.SigningProtection(pubkey)
		require.ErrorContains(t, "does not match", err)
		require.ErrorContains(t, "does not match", other.CheckAndSaveSlotMessage(pubkey, Block, 11))
	})

	t.Run("refuses to sign after a corrupted file", func(t *testing.T) {
		store, err := NewStore(t.TempDir(), genesisValidatorsRoot)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(store.filePath(pubkey), []byte{}, params.BeaconIoConfig().ReadWritePermissions))

		require.ErrorContains(t, "empty file", store.CheckAndSaveSlotMessage(pubkey, Block, 1))
		require.ErrorContains(t, "empty file", store.CheckAndSaveAttestation(pubkey, 0, 1))
		_, err = store.NextReservedLeaf(pubkey)
		require.ErrorContains(t, "empty file", err)
	})

	t.Run("file format", func(t *testing.T) {
		store, err := NewStore(t.TempDir(), genesisValidatorsRoot)
		require.NoError(t, err)
		require.NoError(t, store.CheckAndSaveAttestation(pubkey, 3, 4))
		require.NoError(t, store.CheckAndSaveSlotMessage(pubkey, Block, 10))
		require.NoError(t, store.CheckAndSaveSlotMessage(pubkey, ProposerPreferences, 40))
		_, err = store.NextReservedLeaf(pubkey)
		require.NoError(t, err)

		data, err := os.ReadFile(store.filePath(pubkey))
		require.NoError(t, err)

		expected := "genesisValidatorsRoot: " + hexutil.Encode(genesisValidatorsRoot[:]) + "\n" +
			"lastSignedReservedLeaf: 1\n" +
			"lastSignedAttestationSourceEpoch: 3\n" +
			"lastSignedAttestationTargetEpoch: 4\n" +
			"lastSignedBlockSlot: 10\n" +
			"lastSignedProposerPreferencesSlot: 40\n"
		require.Equal(t, expected, string(data))

		info, err := os.Stat(store.filePath(pubkey))
		require.NoError(t, err)
		require.Equal(t, params.BeaconIoConfig().ReadWritePermissions, info.Mode().Perm())
	})
}

func TestWriteFileAtomic(t *testing.T) {
	t.Run("creates and replaces a file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "file.yaml")

		require.NoError(t, writeFileAtomic(path, []byte("first")))
		require.NoError(t, writeFileAtomic(path, []byte("second")))

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, "second", string(data))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.Equal(t, 1, len(entries))
	})

	t.Run("missing directory", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing", "file.yaml")
		require.ErrorContains(t, "create temporary file", writeFileAtomic(path, []byte("data")))
	})
}

func TestUpdate(t *testing.T) {
	t.Run("concurrent updates never use the same leaf twice", func(t *testing.T) {
		store, err := NewStore(t.TempDir(), genesisValidatorsRoot)
		require.NoError(t, err)

		pubkey := pubkeyFromByte(1)

		const goroutines = 32
		results := make(chan error, goroutines)

		var wg sync.WaitGroup
		for range goroutines {
			wg.Go(func() {
				results <- store.CheckAndSaveSlotMessage(pubkey, Block, primitives.Slot(7))
			})
		}
		wg.Wait()
		close(results)

		succeeded := 0
		for err := range results {
			if err == nil {
				succeeded++
				continue
			}

			require.ErrorIs(t, err, ErrSigningRefused)
		}
		require.Equal(t, 1, succeeded)
	})

	t.Run("concurrent reserved leaf allocations are all distinct", func(t *testing.T) {
		store, err := NewStore(t.TempDir(), genesisValidatorsRoot)
		require.NoError(t, err)

		pubkey := pubkeyFromByte(1)

		const goroutines = 32
		leaves := make(chan uint64, goroutines)

		var wg sync.WaitGroup
		for range goroutines {
			wg.Go(func() {
				leaf, err := store.NextReservedLeaf(pubkey)
				if err != nil {
					t.Error(err)
					return
				}
				leaves <- leaf
			})
		}
		wg.Wait()
		close(leaves)

		seen := make(map[uint64]bool, goroutines)
		for leaf := range leaves {
			require.Equal(t, false, seen[leaf])
			seen[leaf] = true
		}
		require.Equal(t, goroutines, len(seen))
	})
}
