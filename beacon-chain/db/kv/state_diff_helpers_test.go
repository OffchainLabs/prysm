package kv

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/OffchainLabs/methodical-ssz/ssz"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/golang/snappy"
)

func TestEncodeStateWithKey(t *testing.T) {
	st, err := util.NewBeaconStateElectra()
	require.NoError(t, err)
	pb, ok := st.ToProtoUnsafe().(ssz.Marshaler)
	require.Equal(t, true, ok)

	t.Run("marshals into the prefixed buffer", func(t *testing.T) {
		// A regrow would mean SizeSSZ under-reported, silently bringing the extra copy back.
		buf := make([]byte, len(ElectraKey), len(ElectraKey)+pb.SizeSSZ())
		copy(buf, ElectraKey)
		out, err := pb.MarshalSSZTo(buf)
		require.NoError(t, err)
		require.Equal(t, true, &buf[0] == &out[0], "MarshalSSZTo reallocated the buffer")
		require.Equal(t, cap(buf), len(out))
	})

	t.Run("allocates only the buffer and the snappy output", func(t *testing.T) {
		// Ensures memory allocations are as expected.
		allocs := testing.AllocsPerRun(5, func() {
			if _, err := encodeProtoWithKey(version.Electra, pb); err != nil {
				t.Fatal(err)
			}
		})

		// Allocations: 1 for the prefixed SSZ buffer, 1 for the snappy output.
		require.Equal(t, 2.0, allocs)
	})

	t.Run("round trip", func(t *testing.T) {
		stateBytes, err := st.MarshalSSZ()
		require.NoError(t, err)
		want, err := addKey(version.Electra, stateBytes)
		require.NoError(t, err)

		got, err := encodeStateWithKey(st)
		require.NoError(t, err)
		decoded, err := snappy.Decode(nil, got)
		require.NoError(t, err)
		require.DeepEqual(t, want, decoded)
	})
}

func TestMakeKeyForStateDiffTree_KeyLength(t *testing.T) {
	// Existing databases store state diff keys at this exact length. Changing
	// it would make all persisted keys unreadable on restart.
	key := makeKeyForStateDiffTree(0, 0)
	require.Equal(t, 16, len(key))

	key = makeKeyForStateDiffTree(3, 1<<40)
	require.Equal(t, 16, len(key))
}

func TestIsStateDiffTreeKey(t *testing.T) {
	setStateDiffExponents([]int{7, 5})

	// A tree key with a level byte, a slot, and zero padding, then the same with an entry suffix.
	treeKey := makeKeyForStateDiffTree(1, 320)
	suffixedKey := append(bytes.Clone(treeKey), stateSuffix...)

	// A key that is shaped like a tree key up to its padding, which a tree key never sets.
	paddedKey := bytes.Clone(treeKey)
	paddedKey[stateDiffTreeKeySlotEnd] = 'x'

	tests := []struct {
		name string
		key  []byte
		want bool
	}{
		{name: "tree key", key: treeKey, want: true},
		{name: "suffixed tree key", key: suffixedKey, want: true},
		{name: "offset metadata key", key: offsetKey, want: false},
		{name: "exponents metadata key", key: exponentsKey, want: false},
		{name: "metadata key longer than a tree key", key: []byte("a-long-metadata-key-here"), want: false},
		{name: "level byte out of range", key: append([]byte("m"), make([]byte, stateDiffTreeKeyLength)...), want: false},
		{name: "non-zero padding", key: paddedKey, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isStateDiffTreeKey(tt.key))
		})
	}
}

func TestStateKeyByVersion_AllVersions(t *testing.T) {
	for _, v := range version.All() {
		t.Run(version.String(v), func(t *testing.T) {
			key, ok := stateKeyByVersion[v]
			require.Equal(t, true, ok)
			require.NotEqual(t, 0, len(key))
		})
	}
}

func BenchmarkEncodeProtoWithKey(b *testing.B) {
	st, err := util.NewBeaconStateElectra()
	require.NoError(b, err)
	pb := st.ToProtoUnsafe().(ssz.Marshaler)

	b.Run("append-copy", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			raw, err := pb.MarshalSSZ()
			require.NoError(b, err)
			_ = snappy.Encode(nil, append(ElectraKey, raw...))
		}
	})
	b.Run("marshal-into-key", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := encodeProtoWithKey(version.Electra, pb)
			require.NoError(b, err)
		}
	})
}

// addKey is the reference encoding: version key followed by the raw SSZ bytes.
func addKey(v int, bytes []byte) ([]byte, error) {
	key, ok := stateKeyByVersion[v]
	if !ok {
		return nil, fmt.Errorf("no state key for fork %s", version.String(v))
	}
	return append(append([]byte{}, key...), bytes...), nil
}
