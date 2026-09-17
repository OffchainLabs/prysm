package sync

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/node"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/sync"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/sync/flags"
	"github.com/urfave/cli/v2"
	"golang.org/x/time/rate"
)

// BeaconNodeOptions sets the appropriate functional opts on the *node.BeaconNode value, to decouple options
// from flag parsing.
func BeaconNodeOptions(c *cli.Context) ([]node.Option, error) {
	bps, burst, streams, err := throttleSettings(c)
	if err != nil {
		return nil, err
	}
	opt := func(node *node.BeaconNode) (err error) {
		node.SyncOptions = append(node.SyncOptions,
			sync.WithThrottleMuxOptions(
				sync.WithThrottleBPS(rate.Limit(bps)),
				sync.WithThrottleBurst(burst),
				sync.WithThrottleStreamsPerPeer(streams),
			),
		)
		return nil
	}
	return []node.Option{opt}, nil
}

// throttleSettings validates the rpc throttle flags. bps == 0 disables throttling; an unset burst defaults
// to streams * bps.
func throttleSettings(c *cli.Context) (bps, burst, streams int, err error) {
	bps = c.Int(flags.ThrottleBPS.Name)
	if bps < 0 {
		return 0, 0, 0, fmt.Errorf("--%s must be >= 0, got %d", flags.ThrottleBPS.Name, bps)
	}
	streams = c.Int(flags.ThrottleStreamsPerPeer.Name)
	if streams < 1 {
		return 0, 0, 0, fmt.Errorf("--%s must be >= 1, got %d", flags.ThrottleStreamsPerPeer.Name, streams)
	}
	burst = c.Int(flags.ThrottleBurst.Name)
	if !c.IsSet(flags.ThrottleBurst.Name) {
		burst = streams * bps
	}
	if burst < bps {
		return 0, 0, 0, fmt.Errorf("--%s (%d) must be >= --%s (%d)", flags.ThrottleBurst.Name, burst, flags.ThrottleBPS.Name, bps)
	}
	return bps, burst, streams, nil
}

// Flags exports the flags subpackage vars for easy inclusion in main.
var Flags = []cli.Flag{
	flags.ThrottleBPS,
	flags.ThrottleBurst,
	flags.ThrottleStreamsPerPeer,
}
