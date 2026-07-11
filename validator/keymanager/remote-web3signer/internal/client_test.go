package internal_test

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager/remote-web3signer/internal"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
)

// mockTransport is the mock Transport object
type mockTransport struct {
	mockResponse *http.Response
}

// RoundTrip is mocking my own implementation of the RoundTripper interface
func (m *mockTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return m.mockResponse, nil
}

func TestNewApiClient(t *testing.T) {
	apiClient, err := internal.NewApiClient("http://localhost:8545", 5*time.Second, "", "", "")
	assert.NoError(t, err)
	assert.NotNil(t, apiClient)
	assert.Equal(t, 5*time.Second, apiClient.RestClient.Timeout)
}

func TestNewApiClientTLS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte(`"ok"`))
		require.NoError(t, err)
	}))
	defer server.Close()

	caCertPath := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(caCertPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600))
	apiClient, err := internal.NewApiClient(server.URL, time.Second, caCertPath, "", "")
	require.NoError(t, err)
	status, err := apiClient.GetServerStatus(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "ok", status)
}

func TestNewApiClientMutualTLSConfig(t *testing.T) {
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()

	dir := t.TempDir()
	certPath := filepath.Join(dir, "client.pem")
	keyPath := filepath.Join(dir, "client-key.pem")
	require.NoError(t, os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600))
	key, err := x509.MarshalPKCS8PrivateKey(server.TLS.Certificates[0].PrivateKey)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0o600))
	_, err = internal.NewApiClient(server.URL, time.Second, "", certPath, keyPath)
	require.NoError(t, err)

	_, err = internal.NewApiClient(server.URL, time.Second, "", certPath, "")
	require.ErrorContains(t, "client certificate and key must be provided together", err)
}

func TestClient_Sign_HappyPath(t *testing.T) {
	jsonSig := `0xb3baa751d0a9132cfe93e4e3d5ff9075111100e3789dca219ade5a24d27e19d16b3353149da1833e9b691bb38634e8dc04469be7032132906c927d7e1a49b414730612877bc6b2810c8f202daf793d1ab0d6b5cb21d52f9e52e883859887a5d9`
	// create a new reader with that JSON
	r := io.NopCloser(bytes.NewReader([]byte(jsonSig)))
	mock := &mockTransport{mockResponse: &http.Response{
		StatusCode: 200,
		Body:       r,
	}}
	u, err := url.Parse("example.com")
	assert.NoError(t, err)
	cl := internal.ApiClient{BaseURL: u, RestClient: &http.Client{Transport: mock}}
	jsonRequest, err := json.Marshal(`{message: "hello"}`)
	assert.NoError(t, err)
	resp, err := cl.Sign(t.Context(), "a2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820", jsonRequest)
	assert.NotNil(t, resp)
	assert.Nil(t, err)
	assert.EqualValues(t, "0xb3baa751d0a9132cfe93e4e3d5ff9075111100e3789dca219ade5a24d27e19d16b3353149da1833e9b691bb38634e8dc04469be7032132906c927d7e1a49b414730612877bc6b2810c8f202daf793d1ab0d6b5cb21d52f9e52e883859887a5d9", fmt.Sprintf("%#x", resp.Marshal()))
}

func TestClient_Sign_HappyPath_Jsontype(t *testing.T) {
	byteval, err := hexutil.Decode(`0xb3baa751d0a9132cfe93e4e3d5ff9075111100e3789dca219ade5a24d27e19d16b3353149da1833e9b691bb38634e8dc04469be7032132906c927d7e1a49b414730612877bc6b2810c8f202daf793d1ab0d6b5cb21d52f9e52e883859887a5d9`)
	require.NoError(t, err)
	sigResp := &internal.SignatureResponse{
		Signature: byteval,
	}
	jsonBytes, err := json.Marshal(sigResp)
	require.NoError(t, err)
	require.NoError(t, err)
	// create a new reader with that JSON
	header := http.Header{}
	header.Set("Content-Type", "application/json;  charset=UTF-8")
	r := io.NopCloser(bytes.NewReader(jsonBytes))
	mock := &mockTransport{mockResponse: &http.Response{
		StatusCode: 200,
		Header:     header,
		Body:       r,
	}}
	u, err := url.Parse("example.com")
	assert.NoError(t, err)
	cl := internal.ApiClient{BaseURL: u, RestClient: &http.Client{Transport: mock}}
	jsonRequest, err := json.Marshal(`{message: "hello"}`)
	assert.NoError(t, err)
	resp, err := cl.Sign(t.Context(), "a2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820", jsonRequest)
	assert.NotNil(t, resp)
	assert.Nil(t, err)
	assert.EqualValues(t, "0xb3baa751d0a9132cfe93e4e3d5ff9075111100e3789dca219ade5a24d27e19d16b3353149da1833e9b691bb38634e8dc04469be7032132906c927d7e1a49b414730612877bc6b2810c8f202daf793d1ab0d6b5cb21d52f9e52e883859887a5d9", fmt.Sprintf("%#x", resp.Marshal()))
}

func TestClient_Sign_500(t *testing.T) {
	jsonSig := `0xb3baa751d0a9132cfe93e4e3d5ff9075111100e3789dca219ade5a24d27e19d16b3353149da1833e9b691bb38634e8dc04469be7032132906c927d7e1a49b414730612877bc6b2810c8f202daf793d1ab0d6b5cb21d52f9e52e883859887a5d9`
	// create a new reader with that JSON
	r := io.NopCloser(bytes.NewReader([]byte(jsonSig)))
	mock := &mockTransport{mockResponse: &http.Response{
		StatusCode: 500,
		Body:       r,
	}}
	u, err := url.Parse("example.com")
	assert.NoError(t, err)
	cl := internal.ApiClient{BaseURL: u, RestClient: &http.Client{Transport: mock}}
	jsonRequest, err := json.Marshal(`{message: "hello"}`)
	assert.NoError(t, err)
	resp, err := cl.Sign(t.Context(), "a2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820", jsonRequest)
	assert.NotNil(t, err)
	assert.Nil(t, resp)

}

func TestClient_Sign_412(t *testing.T) {
	jsonSig := `0xb3baa751d0a9132cfe93e4e3d5ff9075111100e3789dca219ade5a24d27e19d16b3353149da1833e9b691bb38634e8dc04469be7032132906c927d7e1a49b414730612877bc6b2810c8f202daf793d1ab0d6b5cb21d52f9e52e883859887a5d9`
	// create a new reader with that JSON
	r := io.NopCloser(bytes.NewReader([]byte(jsonSig)))
	mock := &mockTransport{mockResponse: &http.Response{
		StatusCode: 412,
		Body:       r,
	}}
	u, err := url.Parse("example.com")
	assert.NoError(t, err)
	cl := internal.ApiClient{BaseURL: u, RestClient: &http.Client{Transport: mock}}
	jsonRequest, err := json.Marshal(`{message: "hello"}`)
	assert.NoError(t, err)
	resp, err := cl.Sign(t.Context(), "a2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820", jsonRequest)
	assert.NotNil(t, err)
	assert.Nil(t, resp)

}

func TestClient_Sign_400(t *testing.T) {
	jsonSig := `0xb3baa751d0a9132cfe93e4e3d5ff9075111100e3789dca219ade5a24d27e19d16b3353149da1833e9b691bb38634e8dc04469be7032132906c927d7e1a49b414730612877bc6b2810c8f202daf793d1ab0d6b5cb21d52f9e52e883859887a5d9`
	// create a new reader with that JSON
	r := io.NopCloser(bytes.NewReader([]byte(jsonSig)))
	mock := &mockTransport{mockResponse: &http.Response{
		StatusCode: 400,
		Body:       r,
	}}
	u, err := url.Parse("example.com")
	assert.NoError(t, err)
	cl := internal.ApiClient{BaseURL: u, RestClient: &http.Client{Transport: mock}}
	jsonRequest, err := json.Marshal(`{message: "hello"}`)
	assert.NoError(t, err)
	resp, err := cl.Sign(t.Context(), "a2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820", jsonRequest)
	assert.NotNil(t, err)
	assert.Nil(t, resp)

}

func TestClient_GetPublicKeys_HappyPath(t *testing.T) {
	// public keys are returned hex encoded with 0x
	j := `["0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820"]`
	// create a new reader with that JSON
	r := io.NopCloser(bytes.NewReader([]byte(j)))
	mock := &mockTransport{mockResponse: &http.Response{
		StatusCode: 200,
		Body:       r,
	}}
	u, err := url.Parse("example.com")
	assert.NoError(t, err)
	cl := internal.ApiClient{BaseURL: u, RestClient: &http.Client{Transport: mock}}
	resp, err := cl.GetPublicKeys(t.Context(), "example.com/api/publickeys")
	assert.NotNil(t, resp)
	assert.Nil(t, err)
	// we would like them as 48byte base64 without 0x
	require.Equal(t, "[0xa2b5aaad9c6efefe7bb9b1243a043404f3362937cfb6b31833929833173f476630ea2cfeb0d9ddf15f97ca8685948820]", fmt.Sprintf("%v", resp))
}

// TODO: not really in use, should be revisited
func TestClient_ReloadSignerKeys_HappyPath(t *testing.T) {
	mock := &mockTransport{mockResponse: &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader(nil)),
	}}
	u, err := url.Parse("example.com")
	assert.NoError(t, err)
	cl := internal.ApiClient{BaseURL: u, RestClient: &http.Client{Transport: mock}}
	err = cl.ReloadSignerKeys(t.Context())
	assert.Nil(t, err)
}

// TODO: not really in use, should be revisited
func TestClient_GetServerStatus_HappyPath(t *testing.T) {
	j := `"some server status, not sure what it looks like, need to find some sample data"`
	r := io.NopCloser(bytes.NewReader([]byte(j)))
	mock := &mockTransport{mockResponse: &http.Response{
		StatusCode: 200,
		Body:       r,
	}}
	u, err := url.Parse("example.com")
	assert.NoError(t, err)
	cl := internal.ApiClient{BaseURL: u, RestClient: &http.Client{Transport: mock}}
	resp, err := cl.GetServerStatus(t.Context())
	assert.NotNil(t, resp)
	assert.Nil(t, err)
}
