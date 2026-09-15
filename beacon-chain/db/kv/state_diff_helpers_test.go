package kv

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

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

func TestKeyForSnapshot_AllVersions(t *testing.T) {
	for _, v := range version.All() {
		t.Run(version.String(v), func(t *testing.T) {
			key, err := keyForSnapshot(v)
			require.NoError(t, err)
			require.NotEqual(t, 0, len(key))
		})
	}
}
