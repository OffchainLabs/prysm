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
	"sync"
	"testing"
	"time"

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
			wantErr: "is not a valid hex",
		},
		{
			name: "path provided public keys, some bad hex for key",
			args: &SetupConfig{
				BaseEndpoint:          "http://prysm.xyz/",
				GenesisValidatorsRoot: root,
				ProvidedPublicKeys:    []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937"},
			},
			wantErr: "is not 48 bytes",
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
				KeyFilePath:           filepath.Join(t.TempDir(), "good_keyfile.txt"),
			},
			fileContents: []string{"0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055", "800057e262bfe42413c2cfce948ff77f11efeea19721f590c8b5b2f32fecb0e164cafba987c80465878408d05b97c9be"},
			want:         []string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820", "0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055", "0x800057e262bfe42413c2cfce948ff77f11efeea19721f590c8b5b2f32fecb0e164cafba987c80465878408d05b97c9be"},
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
			all := km.keys.all()
			require.Equal(t, len(tt.want), len(all))
			for _, key := range all {
				require.Equal(t, true, slices.Contains(tt.want, hexutil.Encode(key[:])))
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
	// Both keys validate, but the file keeps holding only its own.
	wantSlice := []string{"0x800077e04f8d7496099b3d30ac5430aea64873a45e5bcfe004d2095babcbf55e21138ff0d5691abc29da190aa32755c6",
		"0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055"}
	all := km.keys.all()
	require.Equal(t, 2, len(all))
	for _, key := range all {
		require.Equal(t, slices.Contains(wantSlice, hexutil.Encode(key[:])), true)
	}
	keys, err := km.readKeyFile()
	require.NoError(t, err)
	require.Equal(t, 1, len(keys))
	require.Equal(t, "0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055", hexutil.Encode(keys[0][:]))
	// Clearing the file empties its source, leaving only the flag provided keys.
	require.NoError(t, file.WriteFile(keyFilePath, []byte(" ")))
	deadline := time.Now().Add(5 * time.Second)
	for len(km.keys.all()) != 1 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the watcher to drop the file keys")
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.LogsContain(t, logHook, "Remote signer key file has no keys, so the key file no longer contributes any validating keys")

	keys, err = km.FetchValidatingPublicKeys(context.TODO())
	require.NoError(t, err)
	require.Equal(t, 1, len(keys))
	require.Equal(t, "0x800077e04f8d7496099b3d30ac5430aea64873a45e5bcfe004d2095babcbf55e21138ff0d5691abc29da190aa32755c6", hexutil.Encode(keys[0][:]))
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

func TestKeymanager_ExternalURLUnreachable_StartsWhenPolling(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "mock error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)
	km, err := NewKeymanager(ctx, &SetupConfig{
		BaseEndpoint:          srv.URL,
		GenesisValidatorsRoot: root,
		PublicKeysURL:         srv.URL + "/api/v1/eth2/publicKeys",
		PollInterval:          time.Hour,
	})
	require.NoError(t, err)
	keys, err := km.FetchValidatingPublicKeys(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, len(keys))
}

func TestKeymanager_PollsAlongsideAKeyFile(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	droppedKey := "0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"
	keptKey := "0x800057e262bfe42413c2cfce948ff77f11efeea19721f590c8b5b2f32fecb0e164cafba987c80465878408d05b97c9be"
	fileKey := "0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055"

	var mu sync.Mutex
	served := []string{droppedKey, keptKey}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(served))
	}))
	defer srv.Close()

	keyFilePath := filepath.Join(t.TempDir(), "keyfile.txt")
	require.NoError(t, file.WriteFile(keyFilePath, []byte(fileKey+"\n")))
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)
	km, err := NewKeymanager(ctx, &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
		PublicKeysURL:         srv.URL + "/api/v1/eth2/publicKeys",
		KeyFilePath:           keyFilePath,
		PollInterval:          10 * time.Millisecond,
	})
	require.NoError(t, err)
	require.Equal(t, 3, len(km.keys.all()))

	// The signer drops a key, and the poll picks that up even though a key file is configured.
	mu.Lock()
	served = []string{keptKey}
	mu.Unlock()

	deadline := time.Now().Add(5 * time.Second)
	for {
		keys, err := km.FetchValidatingPublicKeys(ctx)
		require.NoError(t, err)
		if len(keys) == 2 && !slices.Contains(encodeKeys(keys), droppedKey) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the poll to drop the URL key")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// The file key is untouched by the poller and still deletable through the API.
	statuses, err := km.DeletePublicKeys([]string{fileKey})
	require.NoError(t, err)
	require.Equal(t, keymanager.StatusDeleted, statuses[0].Status)
}

func TestKeymanager_AddPublicKeys_NoKeyFile(t *testing.T) {
	ctx := t.Context()
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)
	km, err := NewKeymanager(ctx, &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
	})
	require.NoError(t, err)

	statuses, err := km.AddPublicKeys([]string{"0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"})
	require.NoError(t, err)
	require.Equal(t, 1, len(statuses))
	require.Equal(t, keymanager.StatusError, statuses[0].Status)
	require.StringContains(t, "validators-external-signer-key-file", statuses[0].Message)

	keys, err := km.FetchValidatingPublicKeys(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, len(keys))
}

func TestKeymanager_AddPublicKeys_DuplicateOfDerivedKey(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	flagKey := "0x800077e04f8d7496099b3d30ac5430aea64873a45e5bcfe004d2095babcbf55e21138ff0d5691abc29da190aa32755c6"
	keyFilePath := filepath.Join(t.TempDir(), "keyfile.txt")
	require.NoError(t, file.WriteFile(keyFilePath, nil))
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)
	km, err := NewKeymanager(ctx, &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
		KeyFilePath:           keyFilePath,
		ProvidedPublicKeys:    []string{flagKey},
	})
	require.NoError(t, err)

	newKey := "0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055"
	statuses, err := km.AddPublicKeys([]string{flagKey, "not-a-key", newKey, newKey})
	require.NoError(t, err)
	require.Equal(t, keymanager.StatusDuplicate, statuses[0].Status)
	require.Equal(t, keymanager.StatusError, statuses[1].Status)
	require.Equal(t, keymanager.StatusImported, statuses[2].Status)
	// The same key twice in one request is only imported once.
	require.Equal(t, keymanager.StatusDuplicate, statuses[3].Status)
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
	keys, err := km.readKeyFile()
	require.NoError(t, err)
	require.Equal(t, len(keys), len(publicKeys))
	require.Equal(t, hexutil.Encode(keys[0][:]), publicKeys[0])
}

func TestKeymanager_DeletePublicKeys_DerivedKeysAreNotDeletable(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	flagKey := "0x800077e04f8d7496099b3d30ac5430aea64873a45e5bcfe004d2095babcbf55e21138ff0d5691abc29da190aa32755c6"
	fileKey := "0x8000a9a6d3f5e22d783eefaadbcf0298146adb5d95b04db910a0d4e16976b30229d0b1e7b9cda6c7e0bfa11f72efe055"
	unknownKey := "0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"
	keyFilePath := filepath.Join(t.TempDir(), "keyfile.txt")
	require.NoError(t, file.WriteFile(keyFilePath, []byte(fileKey+"\n")))
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)
	km, err := NewKeymanager(ctx, &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
		KeyFilePath:           keyFilePath,
		ProvidedPublicKeys:    []string{flagKey},
	})
	require.NoError(t, err)

	statuses, err := km.DeletePublicKeys([]string{flagKey, fileKey, unknownKey, fileKey})
	require.NoError(t, err)
	require.Equal(t, keymanager.StatusError, statuses[0].Status)
	require.StringContains(t, "validators-external-signer-public-keys", statuses[0].Message)
	require.Equal(t, keymanager.StatusDeleted, statuses[1].Status)
	require.Equal(t, keymanager.StatusNotFound, statuses[2].Status)
	// The same key twice in one request is only deleted once.
	require.Equal(t, keymanager.StatusNotFound, statuses[3].Status)

	// Only the file key is gone, the flag key keeps validating.
	keys, err := km.FetchValidatingPublicKeys(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, len(keys))
	require.Equal(t, flagKey, hexutil.Encode(keys[0][:]))
}

func TestKeymanager_DeletePublicKeys_URLOwnedKey(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	urlKey := "0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode([]string{urlKey}))
	}))
	defer srv.Close()

	keyFilePath := filepath.Join(t.TempDir(), "keyfile.txt")
	// Older versions copied URL keys into the key file, so an upgraded deployment can
	// legitimately have the same key in both sources.
	require.NoError(t, file.WriteFile(keyFilePath, []byte(urlKey+"\n")))
	root, err := hexutil.Decode("0x270d43e74ce340de4bca2b1936beca0f4f5408d9e78aec4850920baf659d5b69")
	require.NoError(t, err)
	km, err := NewKeymanager(ctx, &SetupConfig{
		BaseEndpoint:          "http://example.com",
		GenesisValidatorsRoot: root,
		PublicKeysURL:         srv.URL + "/api/v1/eth2/publicKeys",
		KeyFilePath:           keyFilePath,
	})
	require.NoError(t, err)

	// Deleting cannot stop a URL-owned key from validating, but it must clean up the
	// stale file copy so the key does not become file-owned if the URL later drops it.
	statuses, err := km.DeletePublicKeys([]string{urlKey})
	require.NoError(t, err)
	require.Equal(t, keymanager.StatusError, statuses[0].Status)
	require.StringContains(t, "removed from the key file", statuses[0].Message)
	require.StringContains(t, "continues validating", statuses[0].Message)

	fileKeys, err := km.readKeyFile()
	require.NoError(t, err)
	require.Equal(t, 0, len(fileKeys))

	// Nor can it be re-imported as a file key.
	statuses, err = km.AddPublicKeys([]string{urlKey})
	require.NoError(t, err)
	require.Equal(t, keymanager.StatusDuplicate, statuses[0].Status)

	keys, err := km.FetchValidatingPublicKeys(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, len(keys))
	require.Equal(t, urlKey, hexutil.Encode(keys[0][:]))
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
	keys, err := km.readKeyFile()
	require.NoError(t, err)
	require.Equal(t, len(keys), 1)
	require.Equal(t, hexutil.Encode(keys[0][:]), publicKeys[1])
}

func TestKeymanager_IsReadOnly(t *testing.T) {
	flagKey, urlKey, fileKey := pubkey{1}, pubkey{2}, pubkey{3}
	sharedKey := pubkey{4}
	unknownKey := pubkey{5}

	km := &Keymanager{}
	km.keys.replace(sourceFlag, []pubkey{flagKey, sharedKey})
	km.keys.replace(sourceURL, []pubkey{urlKey})
	km.keys.replace(sourceFile, []pubkey{fileKey, sharedKey})

	tests := []struct {
		name string
		key  pubkey
		want bool
	}{
		{name: "flag key is read-only", key: flagKey, want: true},
		{name: "URL key is read-only", key: urlKey, want: true},
		{name: "file key is writable", key: fileKey, want: false},
		{name: "key in flag and file is read-only", key: sharedKey, want: true},
		{name: "unknown key is read-only", key: unknownKey, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, km.IsReadOnly(tt.key))
		})
	}
}

func encodeKeys(keys []pubkey) []string {
	encoded := make([]string, len(keys))
	for i, key := range keys {
		encoded[i] = hexutil.Encode(key[:])
	}
	return encoded
}
