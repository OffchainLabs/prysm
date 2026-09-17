package sync

import (
	"flag"
	"testing"

	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/sync/flags"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/urfave/cli/v2"
)

func newThrottleContext(t *testing.T, args ...string) *cli.Context {
	set := flag.NewFlagSet("test", flag.ContinueOnError)
	for _, f := range Flags {
		require.NoError(t, f.Apply(set))
	}
	require.NoError(t, set.Parse(args))
	return cli.NewContext(cli.NewApp(), set, nil)
}

func TestThrottleSettings(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		wantBPS     int
		wantBurst   int
		wantStreams int
		wantErr     string
	}{
		{
			name:        "defaults",
			wantBPS:     flags.ThrottleBPS.Value,
			wantBurst:   flags.ThrottleBurst.Value,
			wantStreams: flags.ThrottleStreamsPerPeer.Value,
		},
		{
			name:        "burst follows bps when not set",
			args:        []string{"--rpc-throttle-bps=1000", "--rpc-throttle-streams-per-peer=3"},
			wantBPS:     1000,
			wantBurst:   3000,
			wantStreams: 3,
		},
		{
			name:        "explicit burst is kept",
			args:        []string{"--rpc-throttle-bps=1000", "--rpc-throttle-burst=5000"},
			wantBPS:     1000,
			wantBurst:   5000,
			wantStreams: flags.ThrottleStreamsPerPeer.Value,
		},
		{
			name:        "zero bps disables throttling",
			args:        []string{"--rpc-throttle-bps=0"},
			wantBPS:     0,
			wantBurst:   0,
			wantStreams: flags.ThrottleStreamsPerPeer.Value,
		},
		{
			name:    "negative bps rejected",
			args:    []string{"--rpc-throttle-bps=-1"},
			wantErr: "must be >= 0",
		},
		{
			name:    "burst below bps rejected",
			args:    []string{"--rpc-throttle-bps=1000", "--rpc-throttle-burst=999"},
			wantErr: "must be >= --rpc-throttle-bps",
		},
		{
			name:    "zero streams rejected",
			args:    []string{"--rpc-throttle-streams-per-peer=0"},
			wantErr: "must be >= 1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bps, burst, streams, err := throttleSettings(newThrottleContext(t, tc.args...))
			if tc.wantErr != "" {
				require.ErrorContains(t, tc.wantErr, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantBPS, bps)
			require.Equal(t, tc.wantBurst, burst)
			require.Equal(t, tc.wantStreams, streams)
		})
	}
}

func TestBeaconNodeOptions_RejectsInvalidFlags(t *testing.T) {
	_, err := BeaconNodeOptions(newThrottleContext(t, "--rpc-throttle-bps=-5"))
	require.ErrorContains(t, "must be >= 0", err)

	opts, err := BeaconNodeOptions(newThrottleContext(t))
	require.NoError(t, err)
	require.Equal(t, 1, len(opts))
}
