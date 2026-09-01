package remote_web3signer

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/io/file"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func TestNewKeyManager_ChangingFileCreated(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	keyFilePath := filepath.Join(t.TempDir(), "keyfile.txt")
	bytesBuf := new(bytes.Buffer)
	_, err := bytesBuf.WriteString("8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055") // test without 0x
	require.NoError(t, err)
	_, err = bytesBuf.WriteString("\n")
	require.NoError(t, err)
	_, err = bytesBuf.WriteString("0x800057e262bfe42413c2cfce948ff77f11efeea19721f590c8b5b2f32fecb0e164cafba987c80465878408d05b97c9be")
	require.NoError(t, err)
	_, err = bytesBuf.WriteString("\n")
	require.NoError(t, err)
	err = file.WriteFile(keyFilePath, bytesBuf.Bytes())
	require.NoError(t, err)

	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)
	km, err := NewKeymanager(ctx, &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
		KeyFilePath:           keyFilePath,
		ProvidedPublicKeys:    []string{"0x800077e04f8d7496099b3d30ac5430aea64873a45e5bcfe004d2095babcbf55e21138ff0d5691abc29da190aa32755c6"},
	})
	require.NoError(t, err)
	wantSlice := []string{"0x800077e04f8d7496099b3d30ac5430aea64873a45e5bcfe004d2095babcbf55e21138ff0d5691abc29da190aa32755c6", "0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055", "0x800057e262bfe42413c2cfce948ff77f11efeea19721f590c8b5b2f32fecb0e164cafba987c80465878408d05b97c9be"}
	all := km.keys.all()
	require.Equal(t, 3, len(all))
	for _, key := range all {
		require.Equal(t, slices.Contains(wantSlice, hexutil.Encode(key[:])), true)
	}
	pubKeysChan := make(chan []pubkey)
	sub := km.SubscribeAccountChanges(pubKeysChan)
	defer sub.Unsubscribe()

	// Open the file for writing, create it if it does not exist, and truncate it if it does.
	f, err := os.OpenFile(keyFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	require.NoError(t, err)

	// Write the buffer's contents to the file.
	_, err = f.WriteString("0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055")
	require.NoError(t, err)
	require.NoError(t, f.Sync())
	require.NoError(t, f.Close())

	ks, err := km.readKeyFile()
	require.NoError(t, err)
	require.Equal(t, 1, len(ks))
	require.Equal(t, "0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055", hexutil.Encode(ks[0][:]))

	// The flag key is owned by the flag source, so dropping it from the file keeps it validating.
	want := []string{
		"0x800077e04f8d7496099b3d30ac5430aea64873a45e5bcfe004d2095babcbf55e21138ff0d5691abc29da190aa32755c6",
		"0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055",
	}
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case updatedKeys := <-pubKeysChan:
			if len(updatedKeys) != len(want) {
				continue
			}
			matched := true
			for _, key := range updatedKeys {
				matched = matched && slices.Contains(want, hexutil.Encode(key[:]))
			}
			if matched {
				return
			}
		case <-timer.C:
			t.Fatal("timed out waiting for the key file update")
		}
	}
}

func TestNewKeymanager_WarnsOnUnwritableKeyFileDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	logHook := logTest.NewGlobal()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	dir := t.TempDir()
	keyFilePath := filepath.Join(dir, "keyfile.txt")
	require.NoError(t, file.WriteFile(keyFilePath, []byte("0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055\n")))
	require.NoError(t, os.Chmod(dir, 0o555))
	defer func() { require.NoError(t, os.Chmod(dir, 0o755)) }()

	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)
	km, err := NewKeymanager(ctx, &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
		KeyFilePath:           keyFilePath,
	})
	// Startup succeeds and the key file's keys still validate; only API writes are lost.
	require.NoError(t, err)
	require.LogsContain(t, logHook, "keymanager API imports and deletions will fail")
	require.Equal(t, 1, len(km.keys.all()))
}

func TestKeyFileWatcherSurvivesAtomicReplacements(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	dir := t.TempDir()
	keyFilePath := filepath.Join(dir, "keyfile.txt")
	keys := []string{
		"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820",
		"0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055",
		"0x800057e262bfe42413c2cfce948ff77f11efeea19721f590c8b5b2f32fecb0e164cafba987c80465878408d05b97c9be",
	}
	require.NoError(t, file.WriteFile(keyFilePath, []byte(keys[0]+"\n")))
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)
	km, err := NewKeymanager(ctx, &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
		KeyFilePath:           keyFilePath,
	})
	require.NoError(t, err)

	updates := make(chan []pubkey, 2)
	sub := km.SubscribeAccountChanges(updates)
	defer sub.Unsubscribe()
	for _, key := range keys[1:] {
		tmp, err := os.CreateTemp(dir, ".replacement-*")
		require.NoError(t, err)
		_, err = tmp.WriteString(key + "\n")
		require.NoError(t, err)
		require.NoError(t, tmp.Close())
		require.NoError(t, os.Rename(tmp.Name(), keyFilePath))

		select {
		case updated := <-updates:
			require.Equal(t, 1, len(updated))
			require.Equal(t, key, hexutil.Encode(updated[0][:]))
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for atomic key file replacement")
		}
	}
}

func TestReadKeyFile_PathMissing(t *testing.T) {
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)

	require.NoError(t, err)
	km, err := NewKeymanager(context.TODO(), &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
	})
	require.NoError(t, err)
	_, err = km.readKeyFile()
	require.ErrorContains(t, "no key file path provided", err)
}

func TestRefreshRemoteKeysFromFileChangesWithRetry_failsFastBeforeInitialized(t *testing.T) {
	ctx := t.Context()
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)
	km, err := NewKeymanager(ctx, &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
	})
	require.NoError(t, err)
	km.keyFilePath = filepath.Join(t.TempDir(), "missing.txt")

	// The hour-long retry delay would hang the test if the first failure did not
	// return immediately.
	err = km.refreshRemoteKeysFromFileChangesWithRetry(ctx, time.Hour, func(error) {})
	require.ErrorContains(t, "could not stat remote signer public key file", err)
	require.Equal(t, maxRetries, km.retriesRemaining)
}

// startFailingWatcher starts the retry helper on a valid key file, waits for the
// watcher to initialize, then removes the file so every subsequent refresh fails.
func startFailingWatcher(t *testing.T, ctx context.Context, km *Keymanager, keyFilePath string, retryDelay time.Duration) chan error {
	t.Helper()
	require.NoError(t, file.WriteFile(keyFilePath, []byte("0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055\n")))
	km.keyFilePath = keyFilePath

	ready := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		var readyOnce sync.Once
		result <- km.refreshRemoteKeysFromFileChangesWithRetry(ctx, retryDelay, func(error) {
			readyOnce.Do(func() { close(ready) })
		})
	}()
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the watcher to initialize")
	}
	require.NoError(t, os.Remove(keyFilePath))
	return result
}

func TestRefreshRemoteKeysFromFileChangesWithRetry_recoveryRestoresFileKeys(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	flagKey := "0x800077e04f8d7496099b3d30ac5430aea64873a45e5bcfe004d2095babcbf55e21138ff0d5691abc29da190aa32755c6"
	fileKey := "0x800057e262bfe42413c2cfce948ff77f11efeea19721f590c8b5b2f32fecb0e164cafba987c80465878408d05b97c9be"
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)
	km, err := NewKeymanager(ctx, &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
		ProvidedPublicKeys:    []string{flagKey},
	})
	require.NoError(t, err)

	keyFilePath := filepath.Join(t.TempDir(), "keyfile.txt")
	result := startFailingWatcher(t, ctx, km, keyFilePath, time.Millisecond)

	// A failed refresh drops the file keys, so putting the file back has to restore them
	// even though the flag keys kept the validating set non-empty in the meantime.
	require.NoError(t, file.WriteFile(keyFilePath, []byte(fileKey+"\n")))
	deadline := time.Now().Add(5 * time.Second)
	for {
		keys := km.keys.all()
		if len(keys) == 2 && slices.Contains(encodeKeys(keys), fileKey) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the recovered watcher to restore the file keys")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// The recovered watcher is back in its event loop, so cancelling is a clean shutdown.
	cancel()
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not stop on cancellation")
	}
}

func TestRefreshRemoteKeysFromFileChangesWithRetry_maxRetryReached(t *testing.T) {
	ctx := t.Context()
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)
	km, err := NewKeymanager(ctx, &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
	})
	require.NoError(t, err)

	result := startFailingWatcher(t, ctx, km, filepath.Join(t.TempDir(), "keyfile.txt"), time.Millisecond)
	select {
	case err := <-result:
		require.ErrorContains(t, "file check retries remaining exceeded", err)
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for retries to be exhausted")
	}
}

func TestRefreshRemoteKeysFromFileChangesWithRetry_ctxCancelDuringRetryWait(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	logHook := logTest.NewGlobal()
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)
	km, err := NewKeymanager(ctx, &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
	})
	require.NoError(t, err)

	// The hour-long retry delay would hang the test if cancellation did not
	// interrupt the wait.
	result := startFailingWatcher(t, ctx, km, filepath.Join(t.TempDir(), "keyfile.txt"), time.Hour)
	// Wait until the helper reaches the retry wait before cancelling.
	deadline := time.Now().Add(5 * time.Second)
	for !logsContain(logHook, "Could not refresh keys") {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the retry warning")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-result:
		require.ErrorContains(t, context.Canceled.Error(), err)
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation did not interrupt the retry wait")
	}
}

func TestKeyFileDirWritable(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}

	t.Run("writable directory", func(t *testing.T) {
		keyFilePath := filepath.Join(t.TempDir(), "keyfile.txt")
		require.NoError(t, file.WriteFile(keyFilePath, []byte("")))
		require.NoError(t, keyFileDirWritable(keyFilePath))
	})

	t.Run("leaves no probe file behind", func(t *testing.T) {
		dir := t.TempDir()
		keyFilePath := filepath.Join(dir, "keyfile.txt")
		require.NoError(t, file.WriteFile(keyFilePath, []byte("")))
		require.NoError(t, keyFileDirWritable(keyFilePath))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.Equal(t, 1, len(entries))
		require.Equal(t, "keyfile.txt", entries[0].Name())
	})

	t.Run("unwritable directory", func(t *testing.T) {
		dir := t.TempDir()
		keyFilePath := filepath.Join(dir, "keyfile.txt")
		require.NoError(t, file.WriteFile(keyFilePath, []byte("")))
		// Readable and traversable, but the key file cannot be replaced in place.
		require.NoError(t, os.Chmod(dir, 0o555))
		// t.TempDir cleanup needs the write bit back.
		defer func() { require.NoError(t, os.Chmod(dir, 0o755)) }()

		require.ErrorContains(t, "permission denied", keyFileDirWritable(keyFilePath))
	})
}

func logsContain(hook *logTest.Hook, substr string) bool {
	for _, entry := range hook.AllEntries() {
		if strings.Contains(entry.Message, substr) {
			return true
		}
	}
	return false
}
