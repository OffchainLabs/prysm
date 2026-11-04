package accounts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/io/file"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/validator/client"
	beacon_api "github.com/OffchainLabs/prysm/v7/validator/client/beacon-api"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// PerformExitCfg for account voluntary exits.
type PerformExitCfg struct {
	ValidatorClient  iface.ValidatorClient
	GenesisTime      *timestamppb.Timestamp
	Keymanager       keymanager.IKeymanager
	RawPubKeys       [][]byte
	FormattedPubKeys []string
	OutputDirectory  string
}

// Exit performs a voluntary exit on one or more accounts.
func (acm *CLIManager) Exit(ctx context.Context) error {
	// User decided to cancel the voluntary exit.
	if acm.rawPubKeys == nil && acm.formattedPubKeys == nil {
		return nil
	}

	validatorClient, nodeClient, err := acm.prepareBeaconClients(ctx)
	if err != nil {
		return err
	}
	if nodeClient == nil {
		return errors.New("could not prepare beacon node client")
	}
	syncStatus, err := nodeClient.SyncStatus(ctx, &emptypb.Empty{})
	if err != nil {
		return err
	}
	if syncStatus == nil {
		return errors.New("could not get sync status")
	}

	if syncStatus.Syncing {
		return errors.New("could not perform exit: beacon node is syncing.")
	}
	genesisResponse, err := nodeClient.Genesis(ctx, &emptypb.Empty{})
	if err != nil {
		return fmt.Errorf("could not get genesis: %w", err)
	}

	cfg := PerformExitCfg{
		validatorClient,
		genesisResponse.GenesisTime,
		acm.keymanager,
		acm.rawPubKeys,
		acm.formattedPubKeys,
		acm.exitJSONOutputPath,
	}
	rawExitedKeys, trimmedExitedKeys, err := PerformVoluntaryExit(ctx, cfg)
	if err != nil {
		return err
	}
	if len(acm.exitJSONOutputPath) > 0 {
		displayExitJSONInfo(trimmedExitedKeys, acm.exitJSONOutputPath)
	} else {
		displayExitInfo(beaconChaBaseURL(genesisResponse.GenesisValidatorsRoot), rawExitedKeys, trimmedExitedKeys)
	}

	return nil
}

// PerformVoluntaryExit uses gRPC clients to submit a voluntary exit message to a beacon node.
func PerformVoluntaryExit(
	ctx context.Context, cfg PerformExitCfg,
) (rawExitedKeys [][]byte, formattedExitedKeys []string, err error) {
	var rawNotExitedKeys [][]byte
	epoch, err := client.CurrentEpoch(cfg.GenesisTime)
	if err != nil {
		return nil, nil, fmt.Errorf("could not get current epoch: %w", err)
	}
	for i, key := range cfg.RawPubKeys {
		// When output directory is present, only create the signed exit, but do not propose it.
		// Otherwise, propose the exit immediately.
		if len(cfg.OutputDirectory) > 0 {
			sve, err := client.CreateSignedVoluntaryExit(ctx, cfg.ValidatorClient, cfg.Keymanager.Sign, key, epoch)
			if err != nil {
				rawNotExitedKeys = append(rawNotExitedKeys, key)
				msg := err.Error()
				if strings.Contains(msg, blocks.ValidatorAlreadyExitedMsg) ||
					strings.Contains(msg, blocks.ValidatorCannotExitYetMsg) {
					log.Warningf("Could not create voluntary exit for account %s: %s", cfg.FormattedPubKeys[i], msg)
				} else {
					log.WithError(err).Errorf("Voluntary exit failed for account %s", cfg.FormattedPubKeys[i])
				}
			} else if err := writeSignedVoluntaryExitJSON(sve, cfg.OutputDirectory); err != nil {
				log.WithError(err).Error("Failed to write voluntary exit")
			}
		} else if err := client.ProposeExit(ctx, cfg.ValidatorClient, cfg.Keymanager.Sign, key, epoch); err != nil {
			rawNotExitedKeys = append(rawNotExitedKeys, key)

			msg := err.Error()
			if strings.Contains(msg, blocks.ValidatorAlreadyExitedMsg) ||
				strings.Contains(msg, blocks.ValidatorCannotExitYetMsg) {
				log.Warningf("Could not perform voluntary exit for account %s: %s", cfg.FormattedPubKeys[i], msg)
			} else {
				log.WithError(err).Errorf("Voluntary exit failed for account %s", cfg.FormattedPubKeys[i])
			}
		}
	}

	rawExitedKeys = make([][]byte, 0)
	formattedExitedKeys = make([]string, 0)
	for i, key := range cfg.RawPubKeys {
		found := false
		for _, notExited := range rawNotExitedKeys {
			if bytes.Equal(notExited, key) {
				found = true
				break
			}
		}
		if !found {
			rawExitedKeys = append(rawExitedKeys, key)
			formattedExitedKeys = append(formattedExitedKeys, cfg.FormattedPubKeys[i])
		}
	}

	return rawExitedKeys, formattedExitedKeys, nil
}

func prepareAllKeys(validatingKeys [][fieldparams.BLSPubkeyLength]byte) (raw [][]byte, formatted []string) {
	raw = make([][]byte, len(validatingKeys))
	formatted = make([]string, len(validatingKeys))
	for i, pk := range validatingKeys {
		raw[i] = make([]byte, len(pk))
		copy(raw[i], pk[:])
		formatted[i] = fmt.Sprintf("%#x", bytesutil.Trunc(pk[:]))
	}
	return
}

// displayExitInfo logs each exited account, with a link to track its exit when baseURL is not empty.
func displayExitInfo(baseURL string, rawExitedKeys [][]byte, trimmedExitedKeys []string) {
	if len(rawExitedKeys) == 0 {
		log.Info("No successful voluntary exits")
		return
	}

	for i, key := range rawExitedKeys {
		logger := log.WithField("pubkey", trimmedExitedKeys[i])
		if baseURL != "" {
			logger = logger.WithField("url", fmt.Sprintf("%s/validator/%s", baseURL, hexutil.Encode(key)[2:]))
		}

		logger.Info("Voluntary exit was successful")
	}

	log.WithField("count", len(rawExitedKeys)).Info("Voluntary exits were successful")
}

// displayExitJSONInfo logs each account whose signed voluntary exit was written to outputDirectory.
func displayExitJSONInfo(trimmedExitedKeys []string, outputDirectory string) {
	if len(trimmedExitedKeys) == 0 {
		log.Info("No signed voluntary exit files were written")
		return
	}

	for _, key := range trimmedExitedKeys {
		log.WithField("pubkey", key).Info("Signed voluntary exit file was written")
	}

	log.WithFields(logrus.Fields{
		"count":           len(trimmedExitedKeys),
		"outputDirectory": outputDirectory,
	}).Info("Signed voluntary exit files were written. No exits were broadcast to the network. " +
		"The validators listed will NOT exit until these files are submitted to a beacon node.")
}

// beaconChaBaseURLs maps the genesis validators roots of the networks tracked by beaconcha.in to their base URLs.
var beaconChaBaseURLs = map[[32]byte]string{
	params.MainnetConfig().GenesisValidatorsRoot: "https://beaconcha.in",
	params.SepoliaConfig().GenesisValidatorsRoot: "https://sepolia.beaconcha.in",
	params.HoodiConfig().GenesisValidatorsRoot:   "https://hoodi.beaconcha.in",
}

// beaconChaBaseURL returns the beaconcha.in base URL of the network with the given genesis validators root,
// or an empty string if beaconcha.in does not track this network.
func beaconChaBaseURL(genesisValidatorsRoot []byte) string {
	return beaconChaBaseURLs[bytesutil.ToBytes32(genesisValidatorsRoot)]
}

func writeSignedVoluntaryExitJSON(sve *eth.SignedVoluntaryExit, outputDirectory string) error {
	if err := file.MkdirAll(outputDirectory); err != nil {
		return err
	}

	jsve := beacon_api.JsonifySignedVoluntaryExits([]*eth.SignedVoluntaryExit{sve})[0]
	b, err := json.Marshal(jsve)
	if err != nil {
		return errors.Wrap(err, "failed to marshal JSON signed voluntary exit")
	}

	filepath := path.Join(outputDirectory, fmt.Sprintf("validator-exit-%s.json", jsve.Message.ValidatorIndex))
	if err := file.WriteFile(filepath, b); err != nil {
		return errors.Wrap(err, "failed to write validator exit json")
	}

	log.Infof("Wrote signed validator exit JSON to %s", filepath)

	return nil
}
