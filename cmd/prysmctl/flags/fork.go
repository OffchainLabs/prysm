package flags

import (
	"fmt"
	"strings"

	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/urfave/cli/v2"
)

// ForkFlag defines the --fork flag used to specify a specific fork version for commands.
var ForkFlag = &cli.StringFlag{
	Name:  "fork",
	Usage: "The fork version to use. Options: phase0, altair, bellatrix, capella, deneb, electra, fulu.",
	Value: "deneb",  // Default to the most stable recent fork
}

// GetForkVersion returns the integer version value from a string fork name.
func GetForkVersion(ctx *cli.Context) (int, error) {
	if !ctx.IsSet(ForkFlag.Name) {
		return version.Deneb, nil // Default to Deneb
	}
	
	forkName := strings.ToLower(ctx.String(ForkFlag.Name))
	v, err := version.FromString(forkName)
	if err != nil {
		availableForks := []string{}
		for _, id := range version.All() {
			availableForks = append(availableForks, version.String(id))
		}
		return 0, fmt.Errorf("invalid fork %q, available options: %s", 
			forkName, strings.Join(availableForks, ", "))
	}
	return v, nil
}