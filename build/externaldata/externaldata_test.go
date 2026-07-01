//go:build !bazel

package externaldata

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestArchiveByName(t *testing.T) {
	t.Run("known", func(t *testing.T) {
		a, idx, ok := archiveByName(BLSSpecTests)
		require.Equal(t, true, ok)
		require.Equal(t, BLSSpecTests, a.name)
		require.Equal(t, true, idx >= 0 && idx < len(allArchives()))
	})

	t.Run("unknown", func(t *testing.T) {
		_, _, ok := archiveByName("definitely_not_an_archive")
		require.Equal(t, false, ok)
	})
}

func TestAllArchives(t *testing.T) {
	all := allArchives()
	require.Equal(t, len(manifest())+len(e2eArchives()), len(all))

	found := false
	for _, a := range all {
		if a.name == Lighthouse {
			found = true
		}
	}
	require.Equal(t, true, found, "allArchives() does not include the e2e Lighthouse archive")
}

func TestDestDir(t *testing.T) {
	t.Run("unknown archive", func(t *testing.T) {
		_, ok := DestDir("definitely_not_an_archive")
		require.Equal(t, false, ok)
	})

	t.Run("root extraction", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0o600))
		t.Chdir(root)

		// ConsensusSpecTestsGeneral extracts into the test-data root (dest ".").
		got, ok := DestDir(ConsensusSpecTestsGeneral)
		require.Equal(t, true, ok)
		require.Equal(t, filepath.Join(root, "third_party", "testdata"), got)
	})

	t.Run("subdirectory extraction", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0o600))
		t.Chdir(root)

		// ConsensusSpec extracts into external/consensus_spec under the root.
		got, ok := DestDir(ConsensusSpec)
		require.Equal(t, true, ok)
		require.Equal(t, filepath.Join(root, "third_party", "testdata", "external", "consensus_spec"), got)
	})

	t.Run("no module root", func(t *testing.T) {
		t.Chdir(t.TempDir())
		_, ok := DestDir(ConsensusSpecTestsGeneral)
		require.Equal(t, false, ok)
	})
}

func TestNames(t *testing.T) {
	names := Names()
	require.Equal(t, len(manifest()), len(names))

	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}

	require.Equal(t, true, set[ConsensusSpecTestsGeneral], "Names() is missing a manifest archive")
	// e2e-only archives are not part of the manifest.
	require.Equal(t, false, set[Lighthouse], "Names() unexpectedly includes an e2e-only archive")
}

func TestFetchUnknownViaWrapper(t *testing.T) {
	require.ErrorContains(t, "unknown archive", Fetch("unknown_archive_for_wrapper_test"))
}

func TestFetchAllCached(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0o600))
	t.Chdir(root)

	// Seed a valid marker for every manifest archive so FetchAll resolves entirely
	// from cache, without any network access.
	markers := filepath.Join(root, "third_party", "testdata", ".markers")
	require.NoError(t, os.MkdirAll(markers, 0o755))
	for _, a := range manifest() {
		wantHex, err := normalizeSha(a.sha256)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(markers, a.name), []byte(wantHex+"\n0\n"), 0o644))
	}

	require.NoError(t, FetchAll())
}

// func TestRoot(t *testing.T) {
// 	t.Run("found", func(t *testing.T) {
// 		root := t.TempDir()
// 		require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0o600))
// 		t.Chdir(root)

// 		require.Equal(t, filepath.Join(root, "third_party", "testdata"), Root())
// 	})

// 	t.Run("not found", func(t *testing.T) {
// 		t.Chdir(t.TempDir())
// 		require.Equal(t, "", Root())
// 	})
// }

func TestFetch(t *testing.T) {
	t.Run("cached marker hits without download", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0o600))
		t.Chdir(root)

		a, _, ok := archiveByName(BLSSpecTests)
		require.Equal(t, true, ok)
		wantHex, err := normalizeSha(a.sha256)
		require.NoError(t, err)

		// Seed the marker so fetch short-circuits to the cached path. The archive
		// URL is never contacted.
		markers := filepath.Join(root, "third_party", "testdata", ".markers")
		require.NoError(t, os.MkdirAll(markers, 0o755))
		const cachedSize = int64(1234)
		body := wantHex + "\n" + strconv.FormatInt(cachedSize, 10) + "\n"
		require.NoError(t, os.WriteFile(filepath.Join(markers, BLSSpecTests), []byte(body), 0o644))

		size, err := fetch(BLSSpecTests)
		require.NoError(t, err)
		require.Equal(t, cachedSize, size)
	})

	t.Run("no module root", func(t *testing.T) {
		t.Chdir(t.TempDir())
		_, err := fetch(BLSSpecTests)
		require.ErrorContains(t, "could not locate test-data root", err)
	})

	// Integrity check: like Bazel, fetch verifies the downloaded bytes against the
	// expected sha256. These cases share a module root and stub the download.
	t.Run("verifies hash", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0o600))
		t.Chdir(root)

		orig := httpDownload
		t.Cleanup(func() { httpDownload = orig })

		t.Run("rejects a hash mismatch", func(t *testing.T) {
			httpDownload = func(string) ([]byte, error) { return []byte("not the real archive"), nil }

			_, err := fetch(BLSSpecTests)
			require.ErrorContains(t, "sha256 mismatch", err)
		})

		t.Run("accepts a matching hash and extracts", func(t *testing.T) {
			// Build a small archive and point the manifest entry's expected hash at it
			// so the whole download -> verify -> extract -> marker path runs offline.
			data := makeTarGz(t, []tarEntry{{name: "aggregate/case.yaml", body: "ok"}})
			sum := fmt.Sprintf("%x", sha256.Sum256(data))

			m := manifest()
			idx := -1
			for i := range m {
				if m[i].name == BLSSpecTests { // dest ".", strip 0
					idx = i
				}
			}
			require.NotEqual(t, -1, idx)
			origSha := m[idx].sha256
			m[idx].sha256 = sum
			t.Cleanup(func() { m[idx].sha256 = origSha })

			httpDownload = func(string) ([]byte, error) { return data, nil }

			size, err := fetch(BLSSpecTests)
			require.NoError(t, err)
			require.Equal(t, int64(len(data)), size)

			got, err := os.ReadFile(filepath.Join(root, "third_party", "testdata", "aggregate", "case.yaml"))
			require.NoError(t, err)
			require.Equal(t, "ok", string(got))
		})

		t.Run("extracts into a subdirectory", func(t *testing.T) {
			// ConsensusSpec has dest "external/consensus_spec" and strips one leading
			// path component, exercising the non-"." extraction branch.
			data := makeTarGz(t, []tarEntry{{name: "consensus-specs-1.2.3/spec.yaml", body: "spec"}})
			sum := fmt.Sprintf("%x", sha256.Sum256(data))

			m := manifest()
			idx := -1
			for i := range m {
				if m[i].name == ConsensusSpec {
					idx = i
				}
			}
			require.NotEqual(t, -1, idx)
			origSha := m[idx].sha256
			m[idx].sha256 = sum
			t.Cleanup(func() { m[idx].sha256 = origSha })

			httpDownload = func(string) ([]byte, error) { return data, nil }

			_, err := fetch(ConsensusSpec)
			require.NoError(t, err)

			got, err := os.ReadFile(filepath.Join(root, "third_party", "testdata", "external", "consensus_spec", "spec.yaml"))
			require.NoError(t, err)
			require.Equal(t, "spec", string(got))
		})
	})
}

func TestDownload(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("payload"))
		}))
		t.Cleanup(srv.Close)

		got, err := download(srv.URL)
		require.NoError(t, err)
		require.Equal(t, "payload", string(got))
	})

	t.Run("non-200 status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(srv.Close)

		_, err := download(srv.URL)
		require.NotNil(t, err, "download of a 404 should error")
	})
}

// func TestReadMarker(t *testing.T) {
// 	dir := t.TempDir()

// 	write := func(t *testing.T, body string) string {
// 		t.Helper()
// 		p := filepath.Join(dir, filepath.Base(t.Name()))
// 		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
// 		return p
// 	}

// 	t.Run("absent file", func(t *testing.T) {
// 		sha, size := readMarker(filepath.Join(dir, "does-not-exist"))
// 		require.Equal(t, "", sha)
// 		require.Equal(t, int64(0), size)
// 	})

// 	t.Run("sha and size", func(t *testing.T) {
// 		sha, size := readMarker(write(t, "deadbeef\n42\n"))
// 		require.Equal(t, "deadbeef", sha)
// 		require.Equal(t, int64(42), size)
// 	})

// 	t.Run("legacy marker without size", func(t *testing.T) {
// 		sha, size := readMarker(write(t, "deadbeef\n"))
// 		require.Equal(t, "deadbeef", sha)
// 		require.Equal(t, int64(0), size)
// 	})
// }

// func TestNormalizeSha(t *testing.T) {
// 	t.Run("plain hex passthrough", func(t *testing.T) {
// 		const hexSha = "93c7d006e7c5b882cbd11dc9ec6c5d0e07f4a8c6b27a32f964eb17cf2db9763a"
// 		got, err := normalizeSha(hexSha)
// 		require.NoError(t, err)
// 		require.Equal(t, hexSha, got)
// 	})

// 	t.Run("sha256-base64 decoded to hex", func(t *testing.T) {
// 		sum := sha256.Sum256([]byte("payload"))
// 		input := "sha256-" + base64.StdEncoding.EncodeToString(sum[:])
// 		got, err := normalizeSha(input)
// 		require.NoError(t, err)
// 		require.Equal(t, hex.EncodeToString(sum[:]), got)
// 	})

// 	t.Run("invalid base64", func(t *testing.T) {
// 		_, err := normalizeSha("sha256-not!valid!base64")
// 		require.ErrorContains(t, "bad sha256-base64", err)
// 	})
// }

// tarEntry describes a single archive member for makeTarGz.
type tarEntry struct {
	name     string
	body     string
	typeflag byte
	linkname string
}

func makeTarGz(t *testing.T, entries []tarEntry) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		flag := e.typeflag
		if flag == 0 {
			flag = tar.TypeReg
		}
		hdr := &tar.Header{Name: e.name, Mode: 0o644, Typeflag: flag, Linkname: e.linkname, Size: int64(len(e.body))}
		require.NoError(t, tw.WriteHeader(hdr))
		if flag == tar.TypeReg {
			_, err := tw.Write([]byte(e.body))
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func TestExtract(t *testing.T) {
	t.Run("include glob filters entries", func(t *testing.T) {
		target := t.TempDir()
		data := makeTarGz(t, []tarEntry{
			{name: "x/assets/eip-4881/c.txt", body: "keep"},
			{name: "x/other/d.txt", body: "drop"},
		})
		require.NoError(t, extract(data, target, 0, "*/assets/eip-4881/*"))

		_, err := os.Stat(filepath.Join(target, "x", "assets", "eip-4881", "c.txt"))
		require.NoError(t, err, "matching entry should be extracted")
		_, err = os.Stat(filepath.Join(target, "x", "other", "d.txt"))
		require.NotNil(t, err, "non-matching entry should be skipped")
	})

	t.Run("skips fully stripped entries", func(t *testing.T) {
		target := t.TempDir()
		// "top/" strips down to nothing with strip=1, so it is skipped; the nested
		// file still lands at the target root.
		data := makeTarGz(t, []tarEntry{
			{name: "top/", typeflag: tar.TypeDir},
			{name: "top/file.txt", body: "ok"},
		})
		require.NoError(t, extract(data, target, 1, ""))

		got, err := os.ReadFile(filepath.Join(target, "file.txt"))
		require.NoError(t, err)
		require.Equal(t, "ok", string(got))
	})

	t.Run("skips the archive root entry", func(t *testing.T) {
		target := t.TempDir()
		// "." resolves to the target itself and is skipped.
		data := makeTarGz(t, []tarEntry{{name: ".", typeflag: tar.TypeDir}})
		require.NoError(t, extract(data, target, 0, ""))
	})

	t.Run("creates directories", func(t *testing.T) {
		target := t.TempDir()
		data := makeTarGz(t, []tarEntry{{name: "sub/", typeflag: tar.TypeDir}})
		require.NoError(t, extract(data, target, 0, ""))

		info, err := os.Stat(filepath.Join(target, "sub"))
		require.NoError(t, err)
		require.Equal(t, true, info.IsDir())
	})

	t.Run("rejects unsafe paths", func(t *testing.T) {
		target := t.TempDir()
		data := makeTarGz(t, []tarEntry{{name: "../evil.txt", body: "boom"}})
		require.ErrorContains(t, "unsafe path", extract(data, target, 0, ""))
	})

	t.Run("skips symlinks", func(t *testing.T) {
		target := t.TempDir()
		data := makeTarGz(t, []tarEntry{{name: "link", typeflag: tar.TypeSymlink, linkname: "/etc/passwd"}})
		require.NoError(t, extract(data, target, 0, ""))

		_, err := os.Lstat(filepath.Join(target, "link"))
		require.NotNil(t, err, "symlink entry should not be created")
	})
}

func TestStripComponents(t *testing.T) {
	tests := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"no strip", "a/b/c", 0, "a/b/c"},
		{"strip one", "a/b/c", 1, "b/c"},
		{"strip leading dot-slash", "./a/b", 1, "b"},
		{"strip equal to depth", "a/b/c", 3, ""},
		{"strip beyond depth", "a/b", 5, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, stripComponents(tt.in, tt.n))
		})
	}
}
