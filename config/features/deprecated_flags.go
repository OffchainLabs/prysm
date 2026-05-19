package features

import (
	"github.com/urfave/cli/v2"
)

// Deprecated flags list.
const deprecatedUsage = "DEPRECATED. DO NOT USE."

var (
	deprecatedMaxGoroutines = &cli.IntFlag{
		Name:   "max-goroutines",
		Usage:  deprecatedUsage,
		Hidden: true,
	}
)

// Deprecated flags for both the beacon node and validator client.
var DeprecatedFlags = []cli.Flag{
	deprecatedMaxGoroutines,
}

var upcomingDeprecation = []cli.Flag{
	enableHistoricalSpaceRepresentation,
}

// deprecatedBeaconFlags contains flags that are still used by other components
// and therefore cannot be added to DeprecatedFlags
var deprecatedBeaconFlags = []cli.Flag{
	deprecatedDisableLastEpochTargets,
}
