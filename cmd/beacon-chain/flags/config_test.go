package flags

import (
	"flag"
	"strconv"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/urfave/cli/v2"
)

func TestValidateStateDiffExponents(t *testing.T) {
	tests := []struct {
		exponents []int
		wantErr   bool
		errMsg    string
	}{
		{exponents: []int{0, 1, 2}, wantErr: true, errMsg: "at least 5"},
		{exponents: []int{1, 2, 3}, wantErr: true, errMsg: "at least 5"},
		{exponents: []int{9, 8, 4}, wantErr: true, errMsg: "at least 5"},
		{exponents: []int{3, 4, 5}, wantErr: true, errMsg: "decreasing"},
		{exponents: []int{15, 14, 14, 12, 11}, wantErr: true, errMsg: "decreasing"},
		{exponents: []int{15, 14, 13, 12, 11}, wantErr: false},
		{exponents: []int{21, 18, 16, 13, 11, 9, 5}, wantErr: false},
		{exponents: []int{30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 18, 16, 13, 11, 9, 5}, wantErr: true, errMsg: "between 1 and 15 values"},
		{exponents: []int{}, wantErr: true, errMsg: "between 1 and 15 values"},
		{exponents: []int{30, 18, 16, 13, 11, 9, 5}, wantErr: false},
		{exponents: []int{31, 18, 16, 13, 11, 9, 5}, wantErr: true, errMsg: "<= 30"},
	}

	for i, tt := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			err := validateStateDiffExponents(tt.exponents)
			if tt.wantErr {
				require.ErrorContains(t, tt.errMsg, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfigureGlobalFlags_SupernodeMutualExclusivity(t *testing.T) {
	tests := []struct {
		name             string
		supernodeSet     bool
		semiSupernodeSet bool
		wantErr          bool
		errMsg           string
	}{
		{
			name:             "both flags not set",
			supernodeSet:     false,
			semiSupernodeSet: false,
			wantErr:          false,
		},
		{
			name:             "only supernode set",
			supernodeSet:     true,
			semiSupernodeSet: false,
			wantErr:          false,
		},
		{
			name:             "only semi-supernode set",
			supernodeSet:     false,
			semiSupernodeSet: true,
			wantErr:          false,
		},
		{
			name:             "both flags set - should error",
			supernodeSet:     true,
			semiSupernodeSet: true,
			wantErr:          true,
			errMsg:           "cannot set both --supernode and --semi-supernode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a flag set and app for testing
			app := cli.NewApp()
			set := flag.NewFlagSet("test", 0)
			set.Bool(Supernode.Name, tt.supernodeSet, "")
			set.Bool(SemiSupernode.Name, tt.semiSupernodeSet, "")
			ctx := cli.NewContext(app, set, nil)

			err := ConfigureGlobalFlags(ctx)
			if tt.wantErr {
				require.ErrorContains(t, tt.errMsg, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
