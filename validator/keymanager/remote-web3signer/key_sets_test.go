package remote_web3signer

import (
	"slices"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

var (
	keyA = pubkey{1}
	keyB = pubkey{2}
	keyC = pubkey{3}
)

func TestKeySets_Replace(t *testing.T) {
	t.Run("a source only replaces its own set", func(t *testing.T) {
		var k keySets

		changed, union := k.replace(sourceFlag, []pubkey{keyA})
		require.Equal(t, true, changed)
		require.Equal(t, 1, len(union))

		// The file source replacing its whole set must not drop the flag key.
		changed, union = k.replace(sourceFile, []pubkey{keyB})
		require.Equal(t, true, changed)
		require.Equal(t, 2, len(union))
		require.Equal(t, true, slices.Contains(union, keyA))

		// Clearing the file source falls back to the flag key.
		changed, union = k.replace(sourceFile, nil)
		require.Equal(t, true, changed)
		require.DeepEqual(t, []pubkey{keyA}, union)
	})

	t.Run("the union reports a change only when it really changed", func(t *testing.T) {
		tests := []struct {
			name        string
			keys        []pubkey
			src         keySource
			wantChanged bool
		}{
			{name: "same set", src: sourceURL, keys: []pubkey{keyA, keyB}, wantChanged: false},
			{name: "same set reordered", src: sourceURL, keys: []pubkey{keyB, keyA}, wantChanged: false},
			{name: "same set with duplicates", src: sourceURL, keys: []pubkey{keyA, keyA, keyB}, wantChanged: false},
			{name: "key already in the union owned elsewhere", src: sourceFile, keys: []pubkey{keyA}, wantChanged: false},
			{name: "added key", src: sourceFile, keys: []pubkey{keyC}, wantChanged: true},
			{name: "removed key", src: sourceURL, keys: []pubkey{keyA}, wantChanged: true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var k keySets
				_, _ = k.replace(sourceURL, []pubkey{keyA, keyB})

				changed, union := k.replace(tt.src, tt.keys)
				require.Equal(t, tt.wantChanged, changed)
				if !tt.wantChanged {
					require.Equal(t, 0, len(union))
				}
			})
		}
	})
}

func TestKeySets_Owner(t *testing.T) {
	t.Run("precedence is flag, then URL, then file", func(t *testing.T) {
		var k keySets
		_, _ = k.replace(sourceFlag, []pubkey{keyA})
		_, _ = k.replace(sourceURL, []pubkey{keyB})
		// keyA is also persisted in the file, but the flag still owns it.
		_, _ = k.replace(sourceFile, []pubkey{keyA, keyC})

		tests := []struct {
			name  string
			key   pubkey
			want  keySource
			found bool
		}{
			{name: "flag owned", key: keyA, want: sourceFlag, found: true},
			{name: "url owned", key: keyB, want: sourceURL, found: true},
			{name: "file owned", key: keyC, want: sourceFile, found: true},
			{name: "unknown", key: pubkey{9}, found: false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				src, ok := k.owner(tt.key)
				require.Equal(t, tt.found, ok)
				if tt.found {
					require.Equal(t, tt.want, src)
				}
			})
		}
	})

	t.Run("ownership transfers when the owner drops the key", func(t *testing.T) {
		var k keySets
		// The same key is held by all three sources at once.
		_, _ = k.replace(sourceFlag, []pubkey{keyA})
		_, _ = k.replace(sourceURL, []pubkey{keyA})
		_, _ = k.replace(sourceFile, []pubkey{keyA})

		src, ok := k.owner(keyA)
		require.Equal(t, true, ok)
		require.Equal(t, sourceFlag, src)

		// Ownership falls to the next source in precedence order, and the key keeps validating.
		_, _ = k.replace(sourceFlag, nil)
		src, _ = k.owner(keyA)
		require.Equal(t, sourceURL, src)

		_, _ = k.replace(sourceURL, nil)
		src, _ = k.owner(keyA)
		require.Equal(t, sourceFile, src)
		require.DeepEqual(t, []pubkey{keyA}, k.all())

		// Once the last holder drops it the key is gone.
		changed, union := k.replace(sourceFile, nil)
		require.Equal(t, true, changed)
		require.Equal(t, 0, len(union))
		_, ok = k.owner(keyA)
		require.Equal(t, false, ok)
	})
}

func TestKeySets_ReadersGetCopies(t *testing.T) {
	var k keySets
	_, _ = k.replace(sourceFile, []pubkey{keyA})

	t.Run("get", func(t *testing.T) {
		set := k.get(sourceFile)
		set[keyB] = struct{}{}
		delete(set, keyA)

		require.DeepEqual(t, map[pubkey]struct{}{keyA: {}}, k.get(sourceFile))
	})

	t.Run("all", func(t *testing.T) {
		all := k.all()
		all[0] = keyC

		require.DeepEqual(t, []pubkey{keyA}, k.all())
	})
}

func TestKeySets_AllReturnsSortedKeys(t *testing.T) {
	var k keySets
	_, _ = k.replace(sourceFile, []pubkey{keyB, keyA, keyC})

	all := k.all()
	require.DeepEqual(t, []pubkey{keyA, keyB, keyC}, all)
}
