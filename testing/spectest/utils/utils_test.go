package utils

import (
	"os"
	"path"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestBLSSettingRunnable(t *testing.T) {
	tests := []struct {
		name    string
		meta    string // empty means no meta.yaml at all
		want    bool
		wantErr bool
	}{
		{name: "no meta.yaml", want: true},
		{name: "no bls_setting", meta: "blocks_count: 1\n", want: true},
		{name: "bls_setting 0", meta: "bls_setting: 0\n", want: true},
		{name: "bls_setting 1", meta: "bls_setting: 1\n", want: !FakeCrypto},
		{name: "bls_setting 2", meta: "bls_setting: 2\n", want: FakeCrypto},
		{name: "unknown bls_setting", meta: "bls_setting: 3\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.meta != "" {
				require.NoError(t, os.WriteFile(path.Join(dir, "meta.yaml"), []byte(tt.meta), 0600))
			}
			got, err := blsSettingRunnable(dir)
			if tt.wantErr {
				require.NotNil(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
