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

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
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
	keys := make([]string, len(km.providedPublicKeys))
	require.Equal(t, 3, len(km.providedPublicKeys))
	for i, key := range km.providedPublicKeys {
		keys[i] = hexutil.Encode(key[:])
		require.Equal(t, slices.Contains(wantSlice, keys[i]), true)
	}
	pubKeysChan := make(chan [][fieldparams.BLSPubkeyLength]byte)
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

	ks, _, err := km.readKeyFile()
	require.NoError(t, err)
	require.Equal(t, 1, len(ks))
	require.Equal(t, "0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055", hexutil.Encode(ks[0][:]))

	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case updatedKeys := <-pubKeysChan:
			if len(updatedKeys) == 1 && hexutil.Encode(updatedKeys[0][:]) == "0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055" {
				return
			}
		case <-timer.C:
			t.Fatal("timed out waiting for the key file update")
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
	_, _, err = km.readKeyFile()
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

func logsContain(hook *logTest.Hook, substr string) bool {
	for _, entry := range hook.AllEntries() {
		if strings.Contains(entry.Message, substr) {
			return true
		}
	}
	return false
}
