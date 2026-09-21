package features

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"gopkg.in/yaml.v2"
)

const (
	// Ephemery re-genesises every reset period, so its spec, bootnodes and genesis state are fetched at runtime.
	ephemeryBaseURL = "https://ephemery.dev/latest"
	// EphemeryGenesisURL is the ssz-encoded genesis state of the running Ephemery iteration.
	EphemeryGenesisURL = ephemeryBaseURL + "/genesis.ssz"

	ephemeryFetchTimeout = 30 * time.Second
	// config.yaml and boot_enr.txt are a few KB each.
	ephemeryMaxFileSize = 1 << 20
)

// useEphemeryNetworkConfig fetches the spec and bootnodes of the running Ephemery iteration and activates them.
func useEphemeryNetworkConfig(ctx context.Context, baseURL string) error {
	cfg, err := fetchEphemeryConfig(ctx, baseURL+"/config.yaml")
	if err != nil {
		return fmt.Errorf("fetch ephemery config from %s: %w", baseURL, err)
	}

	bootnodes, err := fetchEphemeryBootnodes(ctx, baseURL+"/boot_enr.txt")
	if err != nil {
		return fmt.Errorf("fetch ephemery bootnodes from %s: %w", baseURL, err)
	}

	if err := params.SetActive(cfg); err != nil {
		return fmt.Errorf("activate the Ephemery chain config: %w", err)
	}

	nc := params.BeaconNetworkConfig().Copy()
	nc.ContractDeploymentBlock = 0
	nc.BootstrapNodes = bootnodes
	params.OverrideBeaconNetworkConfig(nc)

	return nil
}

func fetchEphemeryConfig(ctx context.Context, url string) (*params.BeaconChainConfig, error) {
	y, err := fetchEphemeryFile(ctx, url)
	if err != nil {
		return nil, err
	}
	// The keys the file sets, so the fork versions it omits can be told apart from the ones it pins.
	keys := make(map[string]any)
	if err := yaml.Unmarshal(y, &keys); err != nil {
		return nil, fmt.Errorf("could not parse %s: %w", url, err)
	}

	cfg, err := params.UnmarshalConfigStrict(y, nil)
	var typeError *yaml.TypeError
	if errors.As(err, &typeError) && !slices.ContainsFunc(typeError.Errors, func(e string) bool {
		return !strings.Contains(e, "not found in type")
	}) {
		log.Warnf("Ignoring unknown keys in the Ephemery config: %s", strings.Join(typeError.Errors, "; "))
		err = nil
	}
	if err != nil {
		return nil, fmt.Errorf("could not parse %s: %w", url, err)
	}

	// Upstream ships CONFIG_NAME: testnet, which says nothing about which network this is.
	// Override it to avoid confusion with the mainnet testnet config.
	cfg.ConfigName = params.EphemeryName

	// Fork versions the config omits keep their mainnet values, which would collide with the mainnet config, so number them up from the genesis version instead.
	if gv := cfg.GenesisForkVersion; len(gv) == 4 {
		for _, f := range []struct {
			key   string
			index byte
			field *[]byte
		}{
			{"ALTAIR_FORK_VERSION", 2, &cfg.AltairForkVersion},
			{"BELLATRIX_FORK_VERSION", 3, &cfg.BellatrixForkVersion},
			{"CAPELLA_FORK_VERSION", 4, &cfg.CapellaForkVersion},
			{"DENEB_FORK_VERSION", 5, &cfg.DenebForkVersion},
			{"ELECTRA_FORK_VERSION", 6, &cfg.ElectraForkVersion},
			{"FULU_FORK_VERSION", 7, &cfg.FuluForkVersion},
			{"GLOAS_FORK_VERSION", 8, &cfg.GloasForkVersion},
		} {
			if _, ok := keys[f.key]; ok {
				continue
			}
			version := []byte{f.index << 4, gv[1], gv[2], gv[3]}
			log.Warnf("Ephemery config does not set %s, defaulting to %#x", f.key, version)
			*f.field = version
		}
	}

	return cfg, nil
}

// fetchEphemeryBootnodes reads a bootnode list holding one ENR per line.
func fetchEphemeryBootnodes(ctx context.Context, url string) ([]string, error) {
	b, err := fetchEphemeryFile(ctx, url)
	if err != nil {
		return nil, err
	}
	var enrs []string
	for line := range strings.SplitSeq(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, err := enode.Parse(enode.ValidSchemes, line); err != nil {
			log.WithError(err).Warnf("Ignoring unparseable Ephemery bootnode entry: %q", line)
			continue
		}
		enrs = append(enrs, line)
	}
	if len(enrs) == 0 {
		return nil, fmt.Errorf("no valid Ephemery bootnodes found at %s", url)
	}
	return enrs, nil
}

func fetchEphemeryFile(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, ephemeryFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.WithError(err).Debug("Could not close response body")
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: unexpected status %s", url, resp.Status)
	}
	// One byte past the limit distinguishes a file that is exactly at the limit from a truncated one.
	b, err := io.ReadAll(io.LimitReader(resp.Body, ephemeryMaxFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}
	if len(b) > ephemeryMaxFileSize {
		return nil, fmt.Errorf("%s is larger than the %d byte limit", url, ephemeryMaxFileSize)
	}
	return b, nil
}
