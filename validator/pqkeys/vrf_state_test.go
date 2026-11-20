package pqkeys

import (
	"bytes"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/vrftree"
)

// testVRFRecord returns the record of VRF root `root` for `slot`, whose branch nodes are filled with `fill`.
func testVRFRecord(root [32]byte, slot uint32, fill byte) (VRFRecord, []byte) {
	record := VRFRecord{Root: root}
	encoded := make([]byte, 0, vrfRecordSize)
	encoded = append(encoded, root[:]...)
	encoded = binary.LittleEndian.AppendUint32(encoded, slot)

	for level := range record.Branch {
		record.Branch[level] = [32]byte{fill, byte(level)}
		encoded = append(encoded, record.Branch[level][:]...)
	}

	return record, encoded
}

func TestNewVRFStateSource(t *testing.T) {
	for _, tc := range []struct {
		name        string
		location    string
		expected    *vrfStateSource
		expectedErr string
	}{
		{name: "path", location: "/tmp/vrf.bin", expected: &vrfStateSource{path: "/tmp/vrf.bin"}},
		{name: "relative path", location: "vrf.bin", expected: &vrfStateSource{path: "vrf.bin"}},
		{name: "file URI", location: "file:///tmp/vrf.bin", expected: &vrfStateSource{path: "/tmp/vrf.bin"}},
		{name: "HTTP URL", location: "http://localhost/vrf.bin", expected: &vrfStateSource{url: "http://localhost/vrf.bin"}},
		{name: "HTTPS URL", location: "https://example.com/vrf.bin", expected: &vrfStateSource{url: "https://example.com/vrf.bin"}},
		{name: "empty", location: "", expectedErr: "empty VRF state location"},
		{name: "unknown scheme", location: "ftp://example.com/vrf.bin", expectedErr: `scheme "ftp"`},
		{name: "file URI without a path", location: "file://", expectedErr: "without a path"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source, err := NewVRFStateSource(tc.location)
			if tc.expectedErr != "" {
				require.ErrorContains(t, tc.expectedErr, err)
				return
			}

			require.NoError(t, err)
			actual, ok := source.(*vrfStateSource)
			require.Equal(t, true, ok)
			require.Equal(t, tc.expected.path, actual.path)
			require.Equal(t, tc.expected.url, actual.url)
		})
	}
}

func TestVRFStateSource_Records(t *testing.T) {
	wanted, other, duplicated, notSlotZero := [32]byte{0x01}, [32]byte{0x02}, [32]byte{0x03}, [32]byte{0x04}

	wantedRecord, wantedBytes := testVRFRecord(wanted, 0, 0xaa)
	_, otherBytes := testVRFRecord(other, 0, 0xbb)
	_, duplicatedBytes := testVRFRecord(duplicated, 0, 0xcc)
	_, notSlotZeroBytes := testVRFRecord(notSlotZero, 1, 0xdd)

	state := bytes.Join([][]byte{otherBytes, duplicatedBytes, wantedBytes, notSlotZeroBytes, duplicatedBytes}, nil)
	roots := map[[32]byte]bool{wanted: true, duplicated: true, notSlotZero: true, {0x05}: true}

	requireRecords := func(t *testing.T, source VRFStateSource) {
		records, err := source.Records(t.Context(), roots)
		require.NoError(t, err)
		require.Equal(t, 1, len(records))
		require.Equal(t, wantedRecord, records[wanted])
	}

	t.Run("file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "vrf.bin")
		require.NoError(t, os.WriteFile(path, state, 0o600))

		source, err := NewVRFStateSource(path)
		require.NoError(t, err)
		requireRecords(t, source)
	})

	t.Run("missing file", func(t *testing.T) {
		source, err := NewVRFStateSource(filepath.Join(t.TempDir(), "vrf.bin"))
		require.NoError(t, err)

		_, err = source.Records(t.Context(), roots)
		require.ErrorContains(t, "open VRF state", err)
	})

	t.Run("HTTP", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "identity", r.Header.Get("Accept-Encoding"))
			_, err := w.Write(state)
			require.NoError(t, err)
		}))
		defer server.Close()

		source, err := NewVRFStateSource(server.URL + "/vrf.bin")
		require.NoError(t, err)
		requireRecords(t, source)
	})

	t.Run("HTTP error", func(t *testing.T) {
		server := httptest.NewServer(http.NotFoundHandler())
		defer server.Close()

		source, err := NewVRFStateSource(server.URL + "/vrf.bin")
		require.NoError(t, err)

		_, err = source.Records(t.Context(), roots)
		require.ErrorContains(t, "404", err)
	})

	t.Run("incomplete record", func(t *testing.T) {
		_, err := readVRFRecords(bytes.NewReader(state[:len(state)-1]), roots)
		require.ErrorContains(t, "incomplete record at index 4", err)
	})

	t.Run("empty", func(t *testing.T) {
		records, err := readVRFRecords(bytes.NewReader(nil), roots)
		require.NoError(t, err)
		require.Equal(t, 0, len(records))
	})

	t.Run("record size", func(t *testing.T) {
		// The size of the records Lighthouse reads.
		require.Equal(t, 1028, vrfRecordSize)
		require.Equal(t, 31, vrftree.Depth)
	})
}
