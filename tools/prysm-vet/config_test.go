package main

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestEmbeddedConfig(t *testing.T) {
	configs, err := parseConfigs(rawConfig)
	require.NoError(t, err)

	cfg := configFor(configs, "cryptorand")
	require.NotNil(t, cfg)
	require.Equal(t, true, cfg.keep("beacon-chain/sync/service.go"))
	require.Equal(t, false, cfg.keep("beacon-chain/sync/service_test.go"))
	require.Equal(t, false, cfg.keep("testing/util/block.go"))

	require.Equal(t, (*config)(nil), configFor(configs, "inspect"))
}

func TestConfigFor(t *testing.T) {
	configs, err := parseConfigs([]byte(`{
		"_base": {"exclude_files": {"external/.*": ""}, "analyzer_flags": {"a": "1"}},
		"only": {"only_files": {"beacon-chain/.*": ""}},
		"own": {"exclude_files": {"_test\\.go$": ""}}
	}`))
	require.NoError(t, err)

	tests := []struct {
		analyzer string
		file     string
		keep     bool
	}{
		// The base config applies to analyzers without their own entry...
		{analyzer: "other", file: "external/foo.go", keep: false},
		{analyzer: "other", file: "beacon-chain/foo.go", keep: true},
		// ...and fills the fields an entry does not set.
		{analyzer: "only", file: "beacon-chain/foo.go", keep: true},
		{analyzer: "only", file: "validator/foo.go", keep: false},
		{analyzer: "only", file: "beacon-chain/external/foo.go", keep: false},
		// An entry's field overrides the base one.
		{analyzer: "own", file: "external/foo.go", keep: true},
		{analyzer: "own", file: "beacon-chain/foo_test.go", keep: false},
	}

	for _, tt := range tests {
		t.Run(tt.analyzer+"/"+tt.file, func(t *testing.T) {
			cfg := configFor(configs, tt.analyzer)
			require.NotNil(t, cfg)
			require.Equal(t, tt.keep, cfg.keep(tt.file))
			require.DeepEqual(t, map[string]string{"a": "1"}, cfg.analyzerFlags)
		})
	}
}

func TestParseConfigsInvalid(t *testing.T) {
	_, err := parseConfigs([]byte(`{"a": {"only_files": {"(": ""}}}`))
	require.ErrorContains(t, "only_files", err)

	_, err = parseConfigs([]byte(`not json`))
	require.ErrorContains(t, "parse config", err)
}
