package config

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/sirupsen/logrus/hooks/test"
)

func TestUnmarshalFromURL_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{"key":"value"}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	var result map[string]string
	err := UnmarshalFromURL(context.Background(), server.URL, &result)
	if err != nil {
		t.Errorf("UnmarshalFromURL failed: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("Expected value to be 'value', got '%s'", result["key"])
	}
}

func TestUnmarshalFromFile_Success(t *testing.T) {
	// Temporarily create a YAML file
	tmpFile, err := os.CreateTemp(t.TempDir(), "example.*.yaml")
	require.NoError(t, err)
	defer require.NoError(t, os.Remove(tmpFile.Name())) // Clean up

	content := []byte("key: value")

	require.NoError(t, os.WriteFile(tmpFile.Name(), content, params.BeaconIoConfig().ReadWritePermissions))
	require.NoError(t, tmpFile.Close())

	var result map[string]string
	require.NoError(t, UnmarshalFromFile(tmpFile.Name(), &result))
	require.Equal(t, result["key"], "value")
}

func TestWarnNonChecksummedAddress(t *testing.T) {
	logHook := test.NewGlobal()
	address := "0x967646dCD8d34F4E02204faeDcbAe0cC96fB9245"
	err := WarnNonChecksummedAddress(address)
	require.NoError(t, err)
	assert.LogsDoNotContain(t, logHook, "is not a checksum Ethereum address")
	address = strings.ToLower("0x967646dCD8d34F4E02204faeDcbAe0cC96fB9244")
	err = WarnNonChecksummedAddress(address)
	require.NoError(t, err)
	assert.LogsContain(t, logHook, "is not a checksum Ethereum address")
}
