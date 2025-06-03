package flags

import (
	"strings"

	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

// ForkFlag defines the fork flag for prysmctl commands
var ForkFlag = &cli.StringFlag{
	Name:  "fork",
	Usage: "Fork version to use for the command. Available options: phase0, altair, bellatrix, capella, deneb, electra, fulu",
	Value: "electra",
}

// GetForkVersion converts a fork name string to its corresponding version constant
func GetForkVersion(forkName string) (int, error) {
	if forkName == "" {
		return version.Electra, nil // Default to electra
	}

	forkVersion, err := version.FromString(forkName)
	if err != nil {
		availableForks := make([]string, 0)
		for _, v := range version.All() {
			availableForks = append(availableForks, version.String(v))
		}
		return 0, errors.Errorf("invalid fork %q, available options are %s", forkName, strings.Join(availableForks, ", "))
	}

	return forkVersion, nil
}

// ProcessForkFlag processes the fork flag from CLI context and returns the fork version
func ProcessForkFlag(ctx *cli.Context) (int, error) {
	forkName := ctx.String("fork")
	return GetForkVersion(forkName)
}

// ValidateForkForFeature validates that a fork supports a specific feature
func ValidateForkForFeature(forkVersion int, feature string) error {
	switch feature {
	case "blobs":
		if forkVersion < version.Deneb {
			return errors.New("blob sidecars are only available from Deneb fork onwards")
		}
	}
	return nil
}
