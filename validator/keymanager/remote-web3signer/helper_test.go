package remote_web3signer

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestSortedKeys(t *testing.T) {
	// Differs from keyA only in a later byte, so the tie is broken past the first one.
	keyA2 := pubkey{1, 9}

	t.Run("empty set", func(t *testing.T) {
		require.Equal(t, 0, len(sortedKeys(nil)))
	})

	t.Run("ascending byte order", func(t *testing.T) {
		set := map[pubkey]struct{}{keyC: {}, keyA2: {}, keyB: {}, keyA: {}}
		require.DeepEqual(t, []pubkey{keyA, keyA2, keyB, keyC}, sortedKeys(set))
	})

	t.Run("stable across calls", func(t *testing.T) {
		set := map[pubkey]struct{}{keyA: {}, keyB: {}, keyC: {}, keyA2: {}}
		want := sortedKeys(set)
		for range 20 {
			require.DeepEqual(t, want, sortedKeys(set))
		}
	})
}

func TestDecodePublicKeys(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		wantLen int
		wantErr string
	}{
		{name: "empty input", input: nil, wantLen: 0},
		{name: "single key", input: []string{testKeyA}, wantLen: 1},
		{name: "dedupes repeated keys", input: []string{testKeyA, testKeyA, testKeyB}, wantLen: 2},
		{name: "invalid hex", input: []string{"not-hex"}, wantErr: "decode public key"},
		{name: "wrong length", input: []string{"0x1234"}, wantErr: "decode public key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodePublicKeys(tt.input)
			if tt.wantErr != "" {
				require.ErrorContains(t, tt.wantErr, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantLen, len(got))
		})
	}
}
