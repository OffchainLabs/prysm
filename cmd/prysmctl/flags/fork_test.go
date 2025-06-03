package flags

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

func TestGetForkVersion(t *testing.T) {
	tests := []struct {
		name        string
		forkName    string
		expected    int
		expectError bool
	}{
		{
			name:     "valid fork - phase0",
			forkName: "phase0",
			expected: version.Phase0,
		},
		{
			name:     "valid fork - altair",
			forkName: "altair",
			expected: version.Altair,
		},
		{
			name:     "valid fork - bellatrix",
			forkName: "bellatrix",
			expected: version.Bellatrix,
		},
		{
			name:     "valid fork - capella",
			forkName: "capella",
			expected: version.Capella,
		},
		{
			name:     "valid fork - deneb",
			forkName: "deneb",
			expected: version.Deneb,
		},
		{
			name:     "valid fork - electra",
			forkName: "electra",
			expected: version.Electra,
		},
		{
			name:     "valid fork - fulu",
			forkName: "fulu",
			expected: version.Fulu,
		},
		{
			name:     "empty fork name defaults to electra",
			forkName: "",
			expected: version.Electra,
		},
		{
			name:        "invalid fork name",
			forkName:    "invalid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetForkVersion(tt.forkName)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid fork")
				assert.Contains(t, err.Error(), "available options are")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestProcessForkFlag(t *testing.T) {
	tests := []struct {
		name        string
		forkValue   string
		expected    int
		expectError bool
	}{
		{
			name:      "process deneb fork",
			forkValue: "deneb",
			expected:  version.Deneb,
		},
		{
			name:      "process phase0 fork",
			forkValue: "phase0",
			expected:  version.Phase0,
		},
		{
			name:        "process invalid fork",
			forkValue:   "invalid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &cli.App{
				Flags: []cli.Flag{ForkFlag},
				Action: func(ctx *cli.Context) error {
					result, err := ProcessForkFlag(ctx)
					if tt.expectError {
						require.Error(t, err)
					} else {
						require.NoError(t, err)
						assert.Equal(t, tt.expected, result)
					}
					return nil
				},
			}

			args := []string{"test", "--fork", tt.forkValue}
			err := app.Run(args)
			if !tt.expectError {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateForkForFeature(t *testing.T) {
	tests := []struct {
		name        string
		forkVersion int
		feature     string
		expectError bool
	}{
		{
			name:        "blobs not supported in phase0",
			forkVersion: version.Phase0,
			feature:     "blobs",
			expectError: true,
		},
		{
			name:        "blobs not supported in altair",
			forkVersion: version.Altair,
			feature:     "blobs",
			expectError: true,
		},
		{
			name:        "blobs not supported in bellatrix",
			forkVersion: version.Bellatrix,
			feature:     "blobs",
			expectError: true,
		},
		{
			name:        "blobs not supported in capella",
			forkVersion: version.Capella,
			feature:     "blobs",
			expectError: true,
		},
		{
			name:        "blobs supported in deneb",
			forkVersion: version.Deneb,
			feature:     "blobs",
			expectError: false,
		},
		{
			name:        "blobs supported in electra",
			forkVersion: version.Electra,
			feature:     "blobs",
			expectError: false,
		},
		{
			name:        "blobs supported in fulu",
			forkVersion: version.Fulu,
			feature:     "blobs",
			expectError: false,
		},
		{
			name:        "unknown feature",
			forkVersion: version.Deneb,
			feature:     "unknown",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateForkForFeature(tt.forkVersion, tt.feature)
			if tt.expectError {
				require.Error(t, err)
				if tt.feature == "blobs" {
					assert.Contains(t, err.Error(), "blob sidecars are only available from Deneb fork onwards")
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}
