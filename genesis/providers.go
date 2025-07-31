package genesis

import (
	"context"
	"fmt"
	"os"

	"github.com/OffchainLabs/prysm/v6/api/client"
	"github.com/OffchainLabs/prysm/v6/api/client/beacon"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/encoding/ssz/detect"
	"github.com/pkg/errors"
)

// Provider is a type that can provide the genesis state for the initialization of the genesis package.
// Examples are getting the state from a beacon node API, reading it from a file, or from the legacy database.
type Provider interface {
	Genesis(context.Context) (state.BeaconState, error)
}

var _ Provider = &FileProvider{}
var _ Provider = &APIProvider{}

// APIProvider provides a genesis state using the given beacon node API url.
type APIProvider struct {
	c *beacon.Client
}

// NewAPIProvider creates an APIProvider, handling the set up of a beacon node api client.
func NewAPIProvider(beaconNodeHost string) (*APIProvider, error) {
	c, err := beacon.NewClient(beaconNodeHost, client.WithMaxBodySize(client.MaxBodySizeState))
	if err != nil {
		return nil, errors.Wrapf(err, "unable to parse beacon node url or hostname - %s", beaconNodeHost)
	}
	return &APIProvider{c: c}, nil
}

// Genesis satisfies the Provider interface by retrieving the genesis state from the beacon node API and unmarshaling it into a phase0 beacon state.
func (dl *APIProvider) Genesis(ctx context.Context) (state.BeaconState, error) {
	sb, err := dl.c.GetState(ctx, beacon.IdGenesis)
	if err != nil {
		return nil, err
	}
	return detect.UnmarshalState(sb)
}

// FileProvider provides the genesis state by reading the given ssz-encoded beacon state file path.
type FileProvider struct {
	statePath string
}

// NewFileProvider validates the given path information and creates a Provider which sources
// the genesis state from an ssz-encoded file on the local filesystem.
func NewFileProvider(statePath string) (*FileProvider, error) {
	var err error
	if err = existsAndIsFile(statePath); err != nil {
		return nil, err
	}
	// stat just to make sure it actually exists and is a file
	return &FileProvider{statePath: statePath}, nil
}

// Genesis satisfies the Provider interface by reading the genesis state from a file and unmarshaling it.
func (fi *FileProvider) Genesis(_ context.Context) (state.BeaconState, error) {
	return stateFromFile(fi.statePath)
}

func existsAndIsFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return errors.Wrapf(err, "error checking existence of ssz-encoded file %s for genesis state init", path)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory, please specify full path to file", path)
	}
	return nil
}
