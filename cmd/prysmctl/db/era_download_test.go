package db

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestListEraFiles(t *testing.T) {
	page := `<a href="../">../</a>
<a href="mainnet-00000-4b363db9.era">mainnet-00000-4b363db9.era</a>
<a href="mainnet-01886-653d1f74.era">mainnet-01886-653d1f74.era</a>
<a href="hoodi-00100-925638dd.era">hoodi-00100-925638dd.era</a>
<a href="mainnet-00010-a1b2c3d4.era1">mainnet-00010-a1b2c3d4.era1</a>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()

	files, err := listEraFiles(t.Context(), srv.Client(), srv.URL, "mainnet")
	require.NoError(t, err)
	require.DeepEqual(t, map[uint64]string{
		0:    "mainnet-00000-4b363db9.era",
		1886: "mainnet-01886-653d1f74.era",
	}, files)
}

func TestDownloadFile(t *testing.T) {
	downloadRetryDelay = 0
	defer func() { downloadRetryDelay = 10 * time.Second }()
	content := bytes.Repeat([]byte("era file "), 1000)

	var ranges []string
	serve := func(ignoreRange bool) *httptest.Server {
		ranges = nil
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ranges = append(ranges, r.Header.Get("Range"))
			if ignoreRange {
				_, _ = w.Write(content)
				return
			}
			http.ServeContent(w, r, "f.era", time.Time{}, bytes.NewReader(content))
		}))
	}

	tests := []struct {
		name        string
		part        []byte
		ignoreRange bool
		wantRanges  []string
	}{
		{"no partial file", nil, false, []string{""}},
		{"resumes a partial file", content[:3000], false, []string{"bytes=3000-"}},
		{"partial file already complete", content, false, []string{"bytes=9000-"}},
		{"server ignores the range", content[:3000], true, []string{"bytes=3000-"}},
		{"partial file longer than the file", append(append([]byte{}, content...), 'x'), false, []string{"bytes=9001-", ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := serve(tt.ignoreRange)
			defer srv.Close()
			path := filepath.Join(t.TempDir(), "f.era")
			if tt.part != nil {
				require.NoError(t, os.WriteFile(path+".part", tt.part, 0o600))
			}

			require.NoError(t, downloadFile(t.Context(), srv.Client(), srv.URL, path))
			got, err := os.ReadFile(path) // #nosec G304
			require.NoError(t, err)
			require.DeepEqual(t, content, got)
			require.DeepEqual(t, tt.wantRanges, ranges)
			_, err = os.Stat(path + ".part")
			require.Equal(t, true, os.IsNotExist(err))
		})
	}
}

func TestDownloadFile_Stall(t *testing.T) {
	downloadRetryDelay, downloadStallTimeout = 0, 100*time.Millisecond
	defer func() { downloadRetryDelay, downloadStallTimeout = 10*time.Second, time.Minute }()
	content := bytes.Repeat([]byte("era file "), 1000)

	// The first response sends part of the file and then nothing, like a dead connection.
	var ranges []string
	stalled := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ranges = append(ranges, r.Header.Get("Range"))
		if len(ranges) == 1 {
			w.Header().Set("Content-Length", strconv.Itoa(len(content)))
			_, _ = w.Write(content[:3000])
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
			case <-stalled:
			}
			return
		}
		http.ServeContent(w, r, "f.era", time.Time{}, bytes.NewReader(content))
	}))
	defer srv.Close()
	defer close(stalled)
	path := filepath.Join(t.TempDir(), "f.era")

	require.NoError(t, downloadFile(t.Context(), srv.Client(), srv.URL, path))
	got, err := os.ReadFile(path) // #nosec G304
	require.NoError(t, err)
	require.DeepEqual(t, content, got)
	require.DeepEqual(t, []string{"", "bytes=3000-"}, ranges)
}
