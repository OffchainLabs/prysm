package remote_web3signer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/io/file"
	validatorpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/validator-client"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager/remote-web3signer/internal"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager/remote-web3signer/types/mock"
	"github.com/ethereum/go-ethereum/common/hexutil"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
)

type MockClient struct {
	Signature       string
	PublicKeys      []string
	isThrowingError bool
}

func (mc *MockClient) Sign(_ context.Context, _ string, _ internal.SignRequestJson) (bls.Signature, error) {
	decoded, err := hexutil.Decode(mc.Signature)
	if err != nil {
		return nil, err
	}
	return bls.SignatureFromBytes(decoded)
}
func (mc *MockClient) GetPublicKeys(_ context.Context, _ string) ([]string, error) {
	return mc.PublicKeys, nil
}

func TestNewKeymanager(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode([]string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"})
		require.NoError(t, err)
	}))
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)
	tests := []struct {
		name         string
		args         *SetupConfig
		fileContents []string
		want         []string
		wantErr      string
		wantLog      string
	}{
		{
			name: "happy path public key url",
			args: &SetupConfig{
				BaseEndpoint:          "http://prysm.xyz/",
				GenesisValidatorsRoot: root,
				PublicKeysURL:         srv.URL + "/public_keys",
			},
			want: []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"},
		},
		{
			name: "bad public key url",
			args: &SetupConfig{
				BaseEndpoint:          "http://prysm.xyz/",
				GenesisValidatorsRoot: root,
				PublicKeysURL:         "0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69",
			},
			wantErr: "could not get public keys from remote server URL",
		},
		{
			name: "happy path provided public keys",
			args: &SetupConfig{
				BaseEndpoint:          "http://prysm.xyz/",
				GenesisValidatorsRoot: root,
				ProvidedPublicKeys:    []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"},
			},
			want: []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"},
		},
		{
			name: "path provided public keys, some bad key",
			args: &SetupConfig{
				BaseEndpoint:          "http://prysm.xyz/",
				GenesisValidatorsRoot: root,
				ProvidedPublicKeys:    []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820", "http://prysm.xyz/"},
			},
			wantErr: "could not decode public key",
		},
		{
			name: "path provided public keys, some bad hex for key",
			args: &SetupConfig{
				BaseEndpoint:          "http://prysm.xyz/",
				GenesisValidatorsRoot: root,
				ProvidedPublicKeys:    []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937"},
			},
			wantErr: "has invalid length",
		},
		{
			name: "happy path key file",
			args: &SetupConfig{
				BaseEndpoint:          "http://prysm.xyz/",
				GenesisValidatorsRoot: root,
				KeyFilePath:           filepath.Join(t.TempDir(), "good_keyfile.txt"),
			},
			fileContents: []string{"8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055", "0x800057e262bfe42413c2cfce948ff77f11efeea19721f590c8b5b2f32fecb0e164cafba987c80465878408d05b97c9be"},
			want:         []string{"0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055", "0x800057e262bfe42413c2cfce948ff77f11efeea19721f590c8b5b2f32fecb0e164cafba987c80465878408d05b97c9be"},
		},
		{
			name: "happy path public key url with good keyfile",
			args: &SetupConfig{
				BaseEndpoint:          "http://prysm.xyz/",
				GenesisValidatorsRoot: root,
				PublicKeysURL:         srv.URL + "/public_keys",
				KeyFilePath:           filepath.Join(t.TempDir(), "good_keyfile.txt"),
			},
			fileContents: []string{"0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055", "800057e262bfe42413c2cfce948ff77f11efeea19721f590c8b5b2f32fecb0e164cafba987c80465878408d05b97c9be"},
			want:         []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820", "0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055", "0x800057e262bfe42413c2cfce948ff77f11efeea19721f590c8b5b2f32fecb0e164cafba987c80465878408d05b97c9be"},
		},
		{
			name: "happy path provided public keys with good keyfile",
			args: &SetupConfig{
				BaseEndpoint:          "http://prysm.xyz/",
				GenesisValidatorsRoot: root,
				ProvidedPublicKeys:    []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"},
			},
			want: []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820", "0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055", "0x800057e262bfe42413c2cfce948ff77f11efeea19721f590c8b5b2f32fecb0e164cafba987c80465878408d05b97c9be"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logHook := logTest.NewGlobal()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tt.args.KeyFilePath != "" && len(tt.fileContents) != 0 {
				bytesBuf := new(bytes.Buffer)
				for _, content := range tt.fileContents {
					_, err := bytesBuf.WriteString(content) // test without 0x
					require.NoError(t, err)
					_, err = bytesBuf.WriteString("\n")
					require.NoError(t, err)
				}
				err = file.WriteFile(tt.args.KeyFilePath, bytesBuf.Bytes())
				require.NoError(t, err)
			}

			km, err := NewKeymanager(ctx, tt.args)
			if tt.wantLog != "" {
				require.LogsContain(t, logHook, tt.wantLog)
			}
			if tt.wantErr != "" {
				require.ErrorContains(t, tt.wantErr, err)
				return
			}
			keys := make([]string, len(km.providedPublicKeys))
			for i, key := range km.providedPublicKeys {
				keys[i] = hexutil.Encode(key[:])
				require.Equal(t, true, slices.Contains(tt.want, keys[i]))
			}
		})
	}
}

func TestNewKeyManager_fileMissing(t *testing.T) {
	keyFilePath := filepath.Join(t.TempDir(), "keyfile.txt")
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)
	_, err = NewKeymanager(context.TODO(), &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
		KeyFilePath:           keyFilePath,
		ProvidedPublicKeys:    []string{"0x800077e04f8d7496099b3d30ac5430aea64873a45e5bcfe004d2095babcbf55e21138ff0d5691abc29da190aa32755c6"},
	})
	require.ErrorContains(t, "no file exists in remote signer key file path", err)
}

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

func TestNewKeyManager_FileAndFlagsWithDifferentKeys(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	logHook := logTest.NewGlobal()
	keyFilePath := filepath.Join(t.TempDir(), "keyfile.txt")
	bytesBuf := new(bytes.Buffer)
	_, err := bytesBuf.WriteString("8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055") // test without 0x
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
	wantSlice := []string{"0x800077e04f8d7496099b3d30ac5430aea64873a45e5bcfe004d2095babcbf55e21138ff0d5691abc29da190aa32755c6",
		"0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055"}
	// provided public keys are saved to the file
	keys, _, err := km.readKeyFile()
	require.NoError(t, err)
	for _, key := range keys {
		require.Equal(t, slices.Contains(wantSlice, hexutil.Encode(key[:])), true)
	}
	// wait for reading to be done
	time.Sleep(2 * time.Second)
	// test fall back by clearing file
	go func() {
		require.NoError(t, file.WriteFile(keyFilePath, []byte(" ")))
	}()
	// waiting for writing to be done
	time.Sleep(2 * time.Second)
	require.LogsContain(t, logHook, "Remote signer key file no longer has keys, defaulting to flag provided keys")

	// fall back to flag provided keys
	keys, err = km.FetchValidatingPublicKeys(context.TODO())
	require.NoError(t, err)
	require.Equal(t, 1, len(keys))
	require.Equal(t, "0x800077e04f8d7496099b3d30ac5430aea64873a45e5bcfe004d2095babcbf55e21138ff0d5691abc29da190aa32755c6", hexutil.Encode(keys[0][:]))
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

func TestKeymanager_Sign(t *testing.T) {
	client := &MockClient{
		Signature: "0xb3baa751d0a9132cfe93e4e3d5ff9075111100e3789dca219ade5a24d27e19d16b3353149da1833e9b691bb38634e8dc04469be7032132906c927d7e1a49b414730612877bc6b2810c8f202daf793d1ab0d6b5cb21d52f9e52e883859887a5d9",
	}
	ctx := t.Context()
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)
	config := &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
		PublicKeysURL:         "http://example2.com/api/v1/eth2/publicKeys",
	}
	km, err := NewKeymanager(ctx, config)
	require.NoError(t, err)
	km.client = client
	desiredSigBytes, err := hexutil.Decode(client.Signature)
	require.NoError(t, err)
	desiredSig, err := bls.SignatureFromBytes(desiredSigBytes)
	require.NoError(t, err)
	type args struct {
		request *validatorpb.SignRequest
	}
	tests := []struct {
		name    string
		args    args
		want    bls.Signature
		wantErr bool
	}{
		{
			name: "AGGREGATION_SLOT",
			args: args{
				request: mock.GetMockSignRequest("AGGREGATION_SLOT"),
			},
			want:    desiredSig,
			wantErr: false,
		},
		{
			name: "AGGREGATE_AND_PROOF_V2",
			args: args{
				request: mock.GetMockSignRequest("AGGREGATE_AND_PROOF_V2"),
			},
			want:    desiredSig,
			wantErr: false,
		},
		{
			name: "ATTESTATION",
			args: args{
				request: mock.GetMockSignRequest("ATTESTATION"),
			},
			want:    desiredSig,
			wantErr: false,
		},
		{
			name: "BLOCK",
			args: args{
				request: mock.GetMockSignRequest("BLOCK"),
			},
			want:    desiredSig,
			wantErr: false,
		},
		{
			name: "BLOCK_V2",
			args: args{
				request: mock.GetMockSignRequest("BLOCK_V2"),
			},
			want:    desiredSig,
			wantErr: false,
		},
		{
			name: "RANDAO_REVEAL",
			args: args{
				request: mock.GetMockSignRequest("RANDAO_REVEAL"),
			},
			want:    desiredSig,
			wantErr: false,
		},
		{
			name: "SYNC_COMMITTEE_CONTRIBUTION_AND_PROOF",
			args: args{
				request: mock.GetMockSignRequest("SYNC_COMMITTEE_CONTRIBUTION_AND_PROOF"),
			},
			want:    desiredSig,
			wantErr: false,
		},
		{
			name: "SYNC_COMMITTEE_MESSAGE",
			args: args{
				request: mock.GetMockSignRequest("SYNC_COMMITTEE_MESSAGE"),
			},
			want:    desiredSig,
			wantErr: false,
		},
		{
			name: "SYNC_COMMITTEE_SELECTION_PROOF",
			args: args{
				request: mock.GetMockSignRequest("SYNC_COMMITTEE_SELECTION_PROOF"),
			},
			want:    desiredSig,
			wantErr: false,
		},
		{
			name: "VOLUNTARY_EXIT",
			args: args{
				request: mock.GetMockSignRequest("VOLUNTARY_EXIT"),
			},
			want:    desiredSig,
			wantErr: false,
		},
		{
			name: "VALIDATOR_REGISTRATION",
			args: args{
				request: mock.GetMockSignRequest("VALIDATOR_REGISTRATION"),
			},
			want:    desiredSig,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := km.Sign(ctx, tt.args.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("name:%s error = %v, wantErr %v", tt.name, err, tt.wantErr)
				return
			}
			require.DeepEqual(t, got, tt.want)
		})
	}

}

func TestKeymanager_FetchValidatingPublicKeys_HappyPath_WithKeyList(t *testing.T) {
	ctx := t.Context()
	decodedKey, err := hexutil.Decode("0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820")
	require.NoError(t, err)
	keys := [][48]byte{
		bytesutil.ToBytes48(decodedKey),
	}
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)
	config := &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
		ProvidedPublicKeys:    []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"},
	}
	km, err := NewKeymanager(ctx, config)
	require.NoError(t, err)
	resp, err := km.FetchValidatingPublicKeys(ctx)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Nil(t, err)
	assert.EqualValues(t, resp, keys)
}

func TestKeymanager_FetchValidatingPublicKeys_HappyPath_WithExternalURL(t *testing.T) {
	ctx := t.Context()
	decodedKey, err := hexutil.Decode("0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820")
	require.NoError(t, err)
	keys := [][48]byte{
		bytesutil.ToBytes48(decodedKey),
	}
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode([]string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"})
		require.NoError(t, err)
	}))
	defer srv.Close()
	config := &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
		PublicKeysURL:         srv.URL + "/api/v1/eth2/publicKeys",
	}
	km, err := NewKeymanager(ctx, config)
	require.NoError(t, err)
	resp, err := km.FetchValidatingPublicKeys(ctx)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.EqualValues(t, resp, keys)
}

func TestKeymanager_FetchValidatingPublicKeys_WithExternalURL_ThrowsError(t *testing.T) {
	ctx := t.Context()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "mock error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)
	config := &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
		PublicKeysURL:         srv.URL + "/api/v1/eth2/publicKeys",
	}
	km, err := NewKeymanager(ctx, config)
	require.ErrorContains(t, fmt.Sprintf("could not get public keys from remote server URL %s/api/v1/eth2/publicKeys", srv.URL), err)
	assert.Nil(t, km)
}

func TestKeymanager_AddPublicKeys(t *testing.T) {
	ctx := t.Context()
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)
	config := &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
	}
	km, err := NewKeymanager(ctx, config)
	require.NoError(t, err)
	publicKeys := []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"}
	statuses, err := km.AddPublicKeys(publicKeys)
	require.NoError(t, err)
	for _, status := range statuses {
		require.Equal(t, keymanager.StatusImported, status.Status)
	}
	statuses, err = km.AddPublicKeys(publicKeys)
	require.NoError(t, err)
	for _, status := range statuses {
		require.Equal(t, keymanager.StatusDuplicate, status.Status)
	}
}

func TestKeymanager_AddPublicKeys_WithFile(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	dir := t.TempDir()
	stdOutFile, err := os.Create(filepath.Clean(path.Join(dir, "keyfile.txt")))
	require.NoError(t, err)
	require.NoError(t, stdOutFile.Chmod(os.FileMode(0600)))
	keyFilePath := filepath.Join(dir, "keyfile.txt")
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)
	config := &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
		KeyFilePath:           keyFilePath,
	}
	km, err := NewKeymanager(ctx, config)
	require.NoError(t, err)
	publicKeys := []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"}
	statuses, err := km.AddPublicKeys(publicKeys)
	require.NoError(t, err)
	for _, status := range statuses {
		require.Equal(t, keymanager.StatusImported, status.Status)
	}
	statuses, err = km.AddPublicKeys(publicKeys)
	require.NoError(t, err)
	for _, status := range statuses {
		require.Equal(t, keymanager.StatusDuplicate, status.Status)
	}
	keys, _, err := km.readKeyFile()
	require.NoError(t, err)
	require.Equal(t, len(keys), len(publicKeys))
	require.Equal(t, hexutil.Encode(keys[0][:]), publicKeys[0])
}

func TestKeymanager_DeletePublicKeys(t *testing.T) {
	ctx := t.Context()
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)
	config := &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
	}
	km, err := NewKeymanager(ctx, config)
	require.NoError(t, err)
	publicKeys := []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"}
	statuses, err := km.AddPublicKeys(publicKeys)
	require.NoError(t, err)
	for _, status := range statuses {
		require.Equal(t, keymanager.StatusImported, status.Status)
	}

	s, err := km.DeletePublicKeys(publicKeys)
	require.NoError(t, err)
	for _, status := range s {
		require.Equal(t, keymanager.StatusDeleted, status.Status)
	}

	s, err = km.DeletePublicKeys(publicKeys)
	require.NoError(t, err)
	for _, status := range s {
		require.Equal(t, keymanager.StatusNotFound, status.Status)
	}
}

func TestKeymanager_DeletePublicKeys_WithFile(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	dir := t.TempDir()
	stdOutFile, err := os.Create(filepath.Clean(path.Join(dir, "keyfile.txt")))
	require.NoError(t, err)
	require.NoError(t, stdOutFile.Chmod(os.FileMode(0600)))
	keyFilePath := filepath.Join(dir, "keyfile.txt")
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)
	config := &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
		KeyFilePath:           keyFilePath,
	}
	km, err := NewKeymanager(ctx, config)
	require.NoError(t, err)
	publicKeys := []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820", "0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055"}
	statuses, err := km.AddPublicKeys(publicKeys)
	require.NoError(t, err)
	for _, status := range statuses {
		require.Equal(t, keymanager.StatusImported, status.Status)
	}

	s, err := km.DeletePublicKeys([]string{publicKeys[0]})
	require.NoError(t, err)
	for _, status := range s {
		require.Equal(t, keymanager.StatusDeleted, status.Status)
	}

	s, err = km.DeletePublicKeys([]string{publicKeys[0]})
	require.NoError(t, err)
	for _, status := range s {
		require.Equal(t, keymanager.StatusNotFound, status.Status)
	}
	keys, _, err := km.readKeyFile()
	require.NoError(t, err)
	require.Equal(t, len(keys), 1)
	require.Equal(t, hexutil.Encode(keys[0][:]), publicKeys[1])
}
