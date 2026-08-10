package remote_web3signer

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

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
