package genesis

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/OffchainLabs/prysm/v7/api/client"
	"github.com/OffchainLabs/prysm/v7/api/client/beacon"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz/detect"
	"github.com/pkg/errors"
)

// DownloadTimeout bounds a genesis state download so that a stalled server cannot hang startup.
const DownloadTimeout = 2 * time.Minute

// Provider is a type that can provide the genesis state for the initialization of the genesis package.
// Examples are getting the state from a beacon node API, reading it from a file, or from the legacy database.
type Provider interface {
	Genesis(context.Context) (state.BeaconState, error)
}

var _ Provider = &FileProvider{}
var _ Provider = &APIProvider{}
var _ Provider = &URLProvider{}

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

// URLProvider provides the genesis state by downloading an ssz-encoded beacon state.
type URLProvider struct {
	url     string
	timeout time.Duration
}

// NewURLProvider creates a Provider which downloads the genesis state from the given url.
func NewURLProvider(url string, timeout time.Duration) *URLProvider {
	return &URLProvider{url: url, timeout: timeout}
}

// Genesis satisfies the Provider interface by downloading the ssz-encoded genesis state and unmarshaling it.
func (u *URLProvider) Genesis(ctx context.Context) (state.BeaconState, error) {
	ctx, cancel := context.WithTimeout(ctx, u.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download genesis state from %s: %w", u.url, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.WithError(err).Debug("Could not close response body")
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download genesis state from %s: unexpected status %s", u.url, resp.Status)
	}
	sb, err := io.ReadAll(io.LimitReader(resp.Body, client.MaxBodySizeState))
	if err != nil {
		return nil, fmt.Errorf("read genesis state from %s: %w", u.url, err)
	}
	return detect.UnmarshalState(sb)
}
