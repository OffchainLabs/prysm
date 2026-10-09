package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestRewriteConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  string
		deps bool
		want map[string]any
	}{
		{
			name: "go version hidden",
			cfg:  `{"ModulePath": "` + modulePath + `", "GoVersion": "go1.26", "VetxOnly": true}`,
			want: map[string]any{"ModulePath": modulePath, "GoVersion": "", "VetxOnly": true},
		},
		{
			name: "deps: module dependency reported",
			cfg:  `{"ModulePath": "` + modulePath + `", "GoVersion": "go1.26", "VetxOnly": true}`,
			deps: true,
			want: map[string]any{"ModulePath": modulePath, "GoVersion": "", "VetxOnly": false},
		},
		{
			name: "deps: other module dependency not reported",
			cfg:  `{"ModulePath": "github.com/foo/bar", "GoVersion": "go1.26", "VetxOnly": true}`,
			deps: true,
			want: map[string]any{"ModulePath": "github.com/foo/bar", "GoVersion": "", "VetxOnly": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "vet.cfg")
			require.NoError(t, os.WriteFile(path, []byte(tt.cfg), 0o600))

			args := []string{"prysm-vet", "-json", path}
			require.NoError(t, rewriteConfig(args, tt.deps))
			require.Equal(t, filepath.Join(dir, "prysm-vet.cfg"), args[2])

			data, err := os.ReadFile(args[2])
			require.NoError(t, err)

			var got map[string]any
			require.NoError(t, json.Unmarshal(data, &got))
			require.DeepEqual(t, tt.want, got)
		})
	}
}

func TestRewriteConfigHandshake(t *testing.T) {
	// Handshake invocations (`-V=full`, `-flags`) are left alone.
	args := []string{"prysm-vet", "-flags"}
	require.NoError(t, rewriteConfig(args, true))
	require.DeepEqual(t, []string{"prysm-vet", "-flags"}, args)
}

func TestDepsEnabled(t *testing.T) {
	require.Equal(t, false, depsEnabled([]string{"-json", "vet.cfg"}))
	require.Equal(t, true, depsEnabled([]string{"-deps", "vet.cfg"}))
	require.Equal(t, true, depsEnabled([]string{"--deps=true", "vet.cfg"}))
	require.Equal(t, false, depsEnabled([]string{"-deps=false", "vet.cfg"}))
}

func TestFixEnabled(t *testing.T) {
	require.Equal(t, false, fixEnabled([]string{"-json", "vet.cfg"}))
	require.Equal(t, true, fixEnabled([]string{"-fix", "vet.cfg"}))
}
