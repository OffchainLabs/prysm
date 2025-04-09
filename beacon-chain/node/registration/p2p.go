package registration

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/cmd"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v2"
)

// P2PPreregistration prepares data for p2p.Service's registration.
func P2PPreregistration(cliCtx *cli.Context) (bootstrapNodeAddrs []string, dataDir string, err error) {
	// Bootnode ENR may be a filepath to a YAML file
	bootnodesTemp := params.BeaconNetworkConfig().BootstrapNodes // actual CLI values
	bootstrapNodeAddrs = make([]string, 0)                       // dest of final list of nodes
	for _, addr := range bootnodesTemp {
		if filepath.Ext(addr) == ".yaml" {
			fileNodes, err := readbootNodes(addr)
			if err != nil {
				return nil, "", err
			}
			bootstrapNodeAddrs = append(bootstrapNodeAddrs, fileNodes...)
		} else {
			bootstrapNodeAddrs = append(bootstrapNodeAddrs, addr)
		}
	}

	dataDir = cliCtx.String(cmd.DataDirFlag.Name)
	if dataDir == "" {
		dataDir = cmd.DefaultDataDir()
		if dataDir == "" {
			err = errors.Errorf(
				"Could not determine your system's HOME path, please specify a --%s you wish to use for your chain data",
				cmd.DataDirFlag.Name,
			)
		}
	}

	return
}

func readbootNodes(fileName string) ([]string, error) {
	fileContent, err := os.ReadFile(fileName) // #nosec G304
	if err != nil {
		return nil, err
	}
	listNodes := make([]string, 0)
	err = yaml.UnmarshalStrict(fileContent, &listNodes)
	if err != nil {
		var typeError *yaml.TypeError
		if !errors.As(err, &typeError) {
			return nil, err
		} else {
			log.WithError(err).Error("There were some issues parsing the bootnodes from a yaml file.")
		}
	}
	return listNodes, nil
}
