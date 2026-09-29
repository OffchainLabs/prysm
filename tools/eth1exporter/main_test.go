package main

import (
	"bufio"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

const testAddress = "0x0000000000000000000000000000000000000001"

func addressFile(t *testing.T, contents string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "addresses")
	require.NoError(t, err)
	_, err = f.WriteString(contents)
	closeErr := f.Close()
	require.NoError(t, err)
	require.NoError(t, closeErr)
	return f.Name()
}

func preserveWatching(t *testing.T) {
	t.Helper()
	previous := allWatching
	t.Cleanup(func() { allWatching = previous })
}

func TestOpenAddresses(t *testing.T) {
	for _, tt := range []struct {
		name     string
		contents string
		want     []*Watching
	}{
		{
			name:     "valid entries and whitespace",
			contents: "\n \t\r\n alice : " + testAddress + " \r\nbob:" + testAddress,
			want: []*Watching{
				{Name: "alice", Address: testAddress},
				{Name: "bob", Address: testAddress},
			},
		},
		{name: "empty file", want: []*Watching{}},
		{name: "blank lines", contents: "\n \t\n\r\n", want: []*Watching{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			preserveWatching(t)
			allWatching = []*Watching{{Name: "previous", Address: testAddress, Balance: "42"}}
			require.NoError(t, OpenAddresses(addressFile(t, tt.contents)))
			require.DeepEqual(t, tt.want, allWatching)
		})
	}
}

func TestOpenAddresses_InvalidEntryPreservesWatching(t *testing.T) {
	for _, tt := range []struct {
		name  string
		entry string
		err   string
	}{
		{name: "missing separator", entry: "bob", err: "expected name:address"},
		{name: "empty name", entry: " :" + testAddress, err: "name is empty"},
		{name: "empty address", entry: "bob: ", err: "invalid hex address"},
		{name: "invalid address", entry: "bob:invalid", err: "invalid hex address"},
		{name: "extra separator", entry: "bob:" + testAddress + ":extra", err: "invalid hex address"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			preserveWatching(t)
			previous := &Watching{Name: "previous", Address: testAddress, Balance: "42"}
			allWatching = []*Watching{previous}
			filename := addressFile(t, "alice:"+testAddress+"\n\n"+tt.entry)
			err := OpenAddresses(filename)
			require.ErrorContains(t, "invalid address entry on line 3: "+tt.err, err)
			require.DeepEqual(t, []*Watching{previous}, allWatching)
			require.Equal(t, previous, allWatching[0])
		})
	}
}

func TestOpenAddresses_ScanErrorPreservesWatching(t *testing.T) {
	preserveWatching(t)
	previous := &Watching{Name: "previous", Address: testAddress, Balance: "42"}
	allWatching = []*Watching{previous}
	filename := addressFile(t, "alice:"+testAddress+"\n"+strings.Repeat("a", bufio.MaxScanTokenSize))
	err := OpenAddresses(filename)
	require.Equal(t, true, errors.Is(err, bufio.ErrTooLong))
	require.DeepEqual(t, []*Watching{previous}, allWatching)
}

func TestOpenAddresses_MissingFilePreservesWatching(t *testing.T) {
	preserveWatching(t)
	previous := &Watching{Name: "previous", Address: testAddress, Balance: "42"}
	allWatching = []*Watching{previous}
	err := OpenAddresses(filepath.Join(t.TempDir(), "missing"))
	require.Equal(t, true, errors.Is(err, os.ErrNotExist))
	require.DeepEqual(t, []*Watching{previous}, allWatching)
}

func TestReloadHTTP(t *testing.T) {
	preserveWatching(t)
	previousPath := *addressFilePath
	t.Cleanup(func() { *addressFilePath = previousPath })
	previous := &Watching{Name: "previous", Address: testAddress, Balance: "42"}
	allWatching = []*Watching{previous}

	*addressFilePath = addressFile(t, "alice:"+testAddress+"\nbob")
	w := httptest.NewRecorder()
	ReloadHTTP(w, httptest.NewRequest(http.MethodGet, "/reload", nil))
	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.DeepEqual(t, []*Watching{previous}, allWatching)

	*addressFilePath = addressFile(t, "alice:"+testAddress+"\n")
	w = httptest.NewRecorder()
	ReloadHTTP(w, httptest.NewRequest(http.MethodGet, "/reload", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.DeepEqual(t, []*Watching{{Name: "alice", Address: testAddress}}, allWatching)
}
