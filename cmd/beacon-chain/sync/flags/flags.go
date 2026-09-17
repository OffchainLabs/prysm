package flags

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/sync"
	"github.com/urfave/cli/v2"
)

var (
	// ThrottleBPS caps the bandwidth used to serve rpc responses to a single peer.
	ThrottleBPS = &cli.IntFlag{
		Name: "rpc-throttle-bps",
		Usage: "Per-peer limit, in bytes per second, on the bandwidth used to serve rpc responses " +
			"(blocks, blobs, data columns, etc). Set to 0 to disable rpc response throttling.",
		Value: int(sync.ThrottleBPS),
	}
	// ThrottleBurst is the token bucket capacity of the per-peer rpc response throttle.
	ThrottleBurst = &cli.IntFlag{
		Name: "rpc-throttle-burst",
		Usage: "Per-peer burst size, in bytes, for rpc response throttling. Must be >= rpc-throttle-bps. " +
			"Defaults to rpc-throttle-streams-per-peer * rpc-throttle-bps when not set.",
		Value: int(sync.ThrottleBurst),
	}
	// ThrottleStreamsPerPeer bounds how many rpc responses are served concurrently to a single peer.
	ThrottleStreamsPerPeer = &cli.IntFlag{
		Name: "rpc-throttle-streams-per-peer",
		Usage: "Number of rpc response streams a single peer may have open concurrently. " +
			"Further requests from the peer wait for a stream to free up.",
		Value: int(sync.ThrottleStreamsPerPeer),
	}
)
