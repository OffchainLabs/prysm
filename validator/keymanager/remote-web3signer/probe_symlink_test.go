package remote_web3signer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/io/file"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// Temporary adversarial probe; not for commit.
func TestProbeSymlinkedKeyFileEndToEnd(t *testing.T) {
	const (
		debounce = 50 * time.Millisecond
		keyA     = "0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"
		keyB     = "0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055"
	)
	resetFeatures := features.InitWithReset(&features.Flags{KeystoreImportDebounceInterval: debounce})
	defer resetFeatures()

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	dirA := t.TempDir()
	dirB := t.TempDir()
	target := filepath.Join(dirB, "real.txt")
	link := filepath.Join(dirA, "keyfile.txt")
	require.NoError(t, file.WriteFile(target, []byte(keyA+"\n")))
	require.NoError(t, os.Symlink(target, link))

	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)
	km, err := NewKeymanager(ctx, &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
		KeyFilePath:           link,
		keyFileRemovalGrace:   200 * time.Millisecond,
		keyFileReadRetryDelay: 10 * time.Millisecond,
	})
	require.NoError(t, err)
	changes := make(chan [][fieldparams.BLSPubkeyLength]byte, 10)
	sub := km.SubscribeAccountChanges(changes)
	t.Cleanup(sub.Unsubscribe)

	// API save writes through the symlink; watcher (watching the resolved dir) must see it.
	_, err = km.AddPublicKeys([]string{keyB})
	require.NoError(t, err)
	requireAccountChange(t, changes, keyA, keyB)
	// Symlink must remain a symlink; the target must hold both keys.
	info, err := os.Lstat(link)
	require.NoError(t, err)
	require.Equal(t, true, info.Mode()&os.ModeSymlink != 0)
	got, err := os.ReadFile(target)
	require.NoError(t, err)
	t.Logf("target contents after API save:\n%s", got)
	// No spurious reconcile-driven changes afterwards.
	requireNoAccountChange(t, changes, 6*debounce)

	// External edit of the target file directly.
	require.NoError(t, file.WriteFile(target, []byte(keyB+"\n")))
	requireAccountChange(t, changes, keyB)
	requireNoAccountChange(t, changes, 4*debounce)

	// External edit through the symlink path.
	require.NoError(t, os.WriteFile(link, []byte(keyA+"\n"), 0o600))
	requireAccountChange(t, changes, keyA)
	requireNoAccountChange(t, changes, 4*debounce)

	// No temp files left in either directory.
	for _, d := range []string{dirA, dirB} {
		m, err := filepath.Glob(filepath.Join(d, ".*tmp*"))
		require.NoError(t, err)
		require.Equal(t, 0, len(m))
	}
}
