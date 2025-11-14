package features

import (
	"github.com/urfave/cli/v2"
)

// Deprecated flags list.
const deprecatedUsage = "DEPRECATED. DO NOT USE."

// Deprecated flags for both the beacon node and validator client.
var deprecatedFlags = []cli.Flag{}

var upcomingDeprecation = []cli.Flag{
	enableHistoricalSpaceRepresentation,
}

// deprecatedBeaconFlags contains flags that are still used by other components
// and therefore cannot be added to deprecatedFlags
var deprecatedBeaconFlags = []cli.Flag{}
