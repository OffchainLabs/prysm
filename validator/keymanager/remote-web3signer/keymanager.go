package remote_web3signer

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fsnotify/fsnotify"
	"github.com/go-playground/validator/v10"
	"github.com/logrusorgru/aurora"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/async/event"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/io/file"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	validatorpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/validator-client"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/validator/accounts/petnames"
	"github.com/prysmaticlabs/prysm/v5/validator/keymanager"
	"github.com/prysmaticlabs/prysm/v5/validator/keymanager/remote-web3signer/internal"
	"github.com/prysmaticlabs/prysm/v5/validator/keymanager/remote-web3signer/types"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/maps"
)

const (
	maxRetries = 60
	retryDelay = 10 * time.Second
)

// SetupConfig includes configuration values for initializing.
// a keymanager, such as passwords, the wallet, and more.
// Web3Signer contains one public keys option. Either through a URL or a static key list.
type SetupConfig struct {
	KeyFilePath           string
	BaseEndpoint          string
	GenesisValidatorsRoot []byte

	// Either URL or keylist must be set.
	// If the URL is set, the keymanager will fetch the public keys from the URL.
	// caution: this option is susceptible to slashing if the web3signer's validator keys are shared across validators
	PublicKeysURL string

	// Either URL or keylist must be set.
	// a static list of public keys to be passed by the user to determine what accounts should sign.
	// This will provide a layer of safety against slashing if the web3signer is shared across validators.
	ProvidedPublicKeys []string
}

// Keymanager defines the web3signer keymanager.
type Keymanager struct {
	client                internal.HttpSignerClient
	genesisValidatorsRoot []byte
	providedPublicKeys    [][48]byte          // (source of truth) flag loaded + file loaded + api loaded keys
	flagLoadedKeysMap     map[string][48]byte // stores what was provided from flag ( as opposed to from file )
	accountsChangedFeed   *event.Feed
	validator             *validator.Validate
	retriesRemaining      int
	keyFilePath           string
	lock                  sync.RWMutex
}

// NewKeymanager instantiates a new web3signer key manager.
func NewKeymanager(ctx context.Context, cfg *SetupConfig) (*Keymanager, error) {
	ctx, span := trace.StartSpan(ctx, "remote-keymanager.NewKeymanager")
	defer span.End()
	if cfg.BaseEndpoint == "" || !bytesutil.IsValidRoot(cfg.GenesisValidatorsRoot) {
		return nil, fmt.Errorf("invalid setup config, one or more configs are empty: BaseEndpoint: %v, GenesisValidatorsRoot: %#x", cfg.BaseEndpoint, cfg.GenesisValidatorsRoot)
	}
	client, err := internal.NewApiClient(cfg.BaseEndpoint)
	if err != nil {
		return nil, errors.Wrap(err, "could not create apiClient")
	}

	km := &Keymanager{
		client:                internal.HttpSignerClient(client),
		genesisValidatorsRoot: cfg.GenesisValidatorsRoot,
		accountsChangedFeed:   new(event.Feed),
		validator:             validator.New(),
		retriesRemaining:      maxRetries,
		keyFilePath:           cfg.KeyFilePath,
	}

	keyFileExists := false
	if km.keyFilePath != "" {
		keyFileExists, err = file.Exists(km.keyFilePath, file.Regular)
		if err != nil {
			return nil, errors.Wrapf(err, "could not check if remote signer persistent keys exist in %s", km.keyFilePath)
		}
		if !keyFileExists {
			return nil, fmt.Errorf("no file exists in remote signer key file path %s", km.keyFilePath)
		}
	}

	var ppk []string
	// load key values
	if cfg.PublicKeysURL != "" {
		providedPublicKeys, err := km.client.GetPublicKeys(ctx, cfg.PublicKeysURL)
		if err != nil {
			erroredResponsesTotal.Inc()
			return nil, errors.Wrapf(err, "could not get public keys from remote server URL %v", cfg.PublicKeysURL)
		}
		ppk = providedPublicKeys
	} else if len(cfg.ProvidedPublicKeys) != 0 {
		ppk = cfg.ProvidedPublicKeys
	}

	// use a map to remove duplicates
	flagLoadedKeys := make(map[string][48]byte)

	// Populate the map with existing keys
	for _, key := range ppk {
		decodedKey, err := hexutil.Decode(key)
		if err != nil {
			return nil, errors.Wrapf(err, "could not decode public key %s", key)
		}
		if len(decodedKey) != fieldparams.BLSPubkeyLength {
			return nil, fmt.Errorf("public key %s has invalid length (expected %d, got %d)", decodedKey, fieldparams.BLSPubkeyLength, len(decodedKey))
		}
		flagLoadedKeys[key] = bytesutil.ToBytes48(decodedKey)
	}
	km.flagLoadedKeysMap = flagLoadedKeys

	// load from file
	if keyFileExists {
		log.WithField("file", km.keyFilePath).Info("Loading keys from file")
		_, fileKeys, err := km.readKeyFile()
		if err != nil {
			return nil, errors.Wrap(err, "could not read key file")
		}
		if len(flagLoadedKeys) != 0 {
			log.WithField("flagLoadedKeyCount", len(flagLoadedKeys)).WithField("fileLoadedKeyCount", len(fileKeys)).Info("Combining flag loaded keys and file loaded keys.")
			maps.Copy(fileKeys, flagLoadedKeys)
			if err = km.savePublicKeysToFile(fileKeys); err != nil {
				return nil, errors.Wrap(err, "could not save public keys to file")
			}
		}
		km.lock.Lock()
		km.providedPublicKeys = maps.Values(fileKeys)
		km.lock.Unlock()
		// create a file watcher
		go func() {
			err = km.refreshRemoteKeysFromFileChangesWithRetry(ctx, retryDelay)
			if err != nil {
				log.WithError(err).Error("Could not refresh remote keys from file changes")
			}
		}()
	} else {
		km.lock.Lock()
		km.providedPublicKeys = maps.Values(flagLoadedKeys)
		km.lock.Unlock()
	}

	return km, nil
}

func (km *Keymanager) refreshRemoteKeysFromFileChangesWithRetry(ctx context.Context, retryDelay time.Duration) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if km.retriesRemaining == 0 {
		return errors.New("file check retries remaining exceeded")
	}
	err := km.refreshRemoteKeysFromFileChanges(ctx)
	if err != nil {
		km.updatePublicKeys(maps.Values(km.flagLoadedKeysMap)) // update the keys to flag provided defaults
		km.retriesRemaining--
		log.WithError(err).Debug("Error occurred on key refresh")
		log.WithFields(logrus.Fields{"path": km.keyFilePath, "retriesRemaining": km.retriesRemaining, "retryDelay": retryDelay}).Warnf("Could not refresh keys. Retrying...")
		time.Sleep(retryDelay)
		return km.refreshRemoteKeysFromFileChangesWithRetry(ctx, retryDelay)
	}
	return nil
}

func (km *Keymanager) readKeyFile() ([][48]byte, map[string][48]byte, error) {
	km.lock.RLock()
	defer km.lock.RUnlock()

	if km.keyFilePath == "" {
		return nil, nil, errors.New("no key file path provided")
	}
	f, err := os.Open(filepath.Clean(km.keyFilePath))
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not open remote signer public key file")
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.WithError(err).Error("Could not close remote signer public key file")
		}
	}()
	// Use a map to track and skip duplicate lines
	seenKeys := make(map[string][48]byte)
	scanner := bufio.NewScanner(f)
	var keys [][48]byte
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		pubkeyLength := (fieldparams.BLSPubkeyLength * 2) + 2
		if line == "" {
			// skip empty line
			continue
		}
		// allow for pubkeys without the 0x
		if len(line) == pubkeyLength-2 && !strings.HasPrefix(line, "0x") {
			line = "0x" + line
		}
		if len(line) != pubkeyLength {
			log.WithFields(logrus.Fields{
				"filepath": km.keyFilePath,
				"key":      line,
			}).Error("Invalid public key in remote signer key file")
			continue
		}
		if _, found := seenKeys[line]; !found {
			// If it's a new line, mark it as seen and process it
			pubkey, err := hexutil.Decode(line)
			if err != nil {
				return nil, nil, errors.Wrapf(err, "could not decode public key %s in remote signer key file", line)
			}
			bPubkey := bytesutil.ToBytes48(pubkey)
			seenKeys[line] = bPubkey
			keys = append(keys, bPubkey)
		}
	}
	// Check for scanning errors
	if err := scanner.Err(); err != nil {
		return nil, nil, errors.Wrap(err, "could not scan remote signer public key file")
	}
	if len(keys) == 0 {
		log.Warn("Remote signer key file: no valid public keys found. Defaulting to flag provided keys if any exist.")
	}
	return keys, seenKeys, nil
}

func (km *Keymanager) savePublicKeysToFile(providedPublicKeys map[string][48]byte) error {
	if km.keyFilePath == "" {
		return errors.New("no key file provided")
	}
	pubkeys := make([][48]byte, 0)
	// Open the file with write and truncate permissions
	f, err := os.OpenFile(km.keyFilePath, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			log.WithError(err).Error("Could not close file, proceeding without closing the file")
		}
	}(f)

	// Iterate through all lines in the slice and write them to the file
	for key, value := range providedPublicKeys {
		if _, err := f.WriteString(key + "\n"); err != nil {
			return fmt.Errorf("error writing key %s to file: %w", value, err)
		}
		pubkeys = append(pubkeys, value)
	}
	km.updatePublicKeys(pubkeys)
	return nil
}

func (km *Keymanager) arePublicKeysEmpty() bool {
	km.lock.RLock()
	defer km.lock.RUnlock()
	return len(km.providedPublicKeys) == 0
}

func (km *Keymanager) refreshRemoteKeysFromFileChanges(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return errors.Wrap(err, "could not initialize file watcher")
	}
	defer func() {
		if err := watcher.Close(); err != nil {
			log.WithError(err).Error("Could not close file watcher")
		}
	}()
	initialFileInfo, err := os.Stat(km.keyFilePath)
	if err != nil {
		return errors.Wrap(err, "could not stat remote signer public key file")
	}
	initialFileSize := initialFileInfo.Size()
	if err := watcher.Add(km.keyFilePath); err != nil {
		return errors.Wrap(err, "could not add file to file watcher")
	}
	log.WithField("path", km.keyFilePath).Info("Successfully initialized file watcher")
	km.retriesRemaining = maxRetries // reset retries to default
	// reinitialize keys if watcher reinitialized
	if km.arePublicKeysEmpty() {
		_, fk, err := km.readKeyFile()
		if err != nil {
			return errors.Wrap(err, "could not read key file")
		}
		maps.Copy(fk, km.flagLoadedKeysMap)
		if err = km.savePublicKeysToFile(fk); err != nil {
			return errors.Wrap(err, "could not save public keys to file")
		}
		km.updatePublicKeys(maps.Values(fk))
	}
	for {
		select {
		case e, ok := <-watcher.Events:
			if !ok { // Channel was closed (i.e. Watcher.Close() was called).
				log.Info("Closing file watcher")
				return nil
			}
			log.WithFields(logrus.Fields{
				"event": e.Name,
				"op":    e.Op.String(),
			}).Debug("Remote signer key file event triggered")
			if e.Has(fsnotify.Remove) {
				return errors.New("remote signer key file was removed")
			}
			currentFileInfo, err := os.Stat(km.keyFilePath)
			if err != nil {
				return errors.Wrap(err, "could not stat remote signer public key file")
			}
			if currentFileInfo.Size() != initialFileSize {
				log.Info("Remote signer key file updated")
				fileKeys, _, err := km.readKeyFile()
				if err != nil {
					return errors.New("could not read key file")
				}
				// prioritize file keys over flag keys
				if len(fileKeys) == 0 {
					log.Warnln("Remote signer key file no longer has keys, defaulting to flag provided keys")
					fileKeys = maps.Values(km.flagLoadedKeysMap)
				}
				currentKeys, err := km.FetchValidatingPublicKeys(ctx)
				if err != nil {
					return errors.Wrap(err, "could not fetch current keys")
				}
				if !slices.Equal(currentKeys, fileKeys) {
					km.updatePublicKeys(fileKeys)
				}
				initialFileSize = currentFileInfo.Size()
			}
		case err, ok := <-watcher.Errors:
			if !ok { // Channel was closed (i.e. Watcher.Close() was called).
				log.Info("Closing file watcher")
				return nil
			}
			return errors.Wrap(err, "could not watch for file changes")
		case <-ctx.Done():
			log.Info("Closing file watcher")
			return nil
		}
	}
}

func (km *Keymanager) updatePublicKeys(keys [][48]byte) {
	km.lock.Lock()
	defer km.lock.Unlock()
	km.providedPublicKeys = keys
	km.accountsChangedFeed.Send(keys)
	log.WithField("count", len(km.providedPublicKeys)).Debug("Updated public keys")
}

// FetchValidatingPublicKeys fetches the validating public keys
func (km *Keymanager) FetchValidatingPublicKeys(_ context.Context) ([][fieldparams.BLSPubkeyLength]byte, error) {
	km.lock.RLock()
	defer km.lock.RUnlock()
	log.WithField("count", len(km.providedPublicKeys)).Debug("Fetched validating public keys")
	return km.providedPublicKeys, nil
}

// Sign signs the message by using a remote web3signer server.
func (km *Keymanager) Sign(ctx context.Context, request *validatorpb.SignRequest) (bls.Signature, error) {
	signRequest, err := getSignRequestJson(ctx, km.validator, request, km.genesisValidatorsRoot)
	if err != nil {
		erroredResponsesTotal.Inc()
		return nil, err
	}
	signature, err := km.client.Sign(ctx, hexutil.Encode(request.PublicKey), signRequest)
	if err != nil {
		erroredResponsesTotal.Inc()
		return nil, errors.Wrap(err, "failed to sign the request")
	}
	log.WithField("publicKey", request.PublicKey).Debug("Successfully signed the request")
	signRequestsTotal.Inc()
	return signature, nil
}

// getSignRequestJson returns a json request based on the SignRequest type.
func getSignRequestJson(ctx context.Context, validator *validator.Validate, request *validatorpb.SignRequest, genesisValidatorsRoot []byte) (internal.SignRequestJson, error) {
	if request == nil {
		return nil, errors.New("nil sign request provided")
	}
	if !bytesutil.IsValidRoot(genesisValidatorsRoot) {
		return nil, fmt.Errorf("invalid genesis validators root length, genesis root: %v", genesisValidatorsRoot)
	}
	switch request.Object.(type) {
	case *validatorpb.SignRequest_Block:
		return handleBlock(ctx, validator, request, genesisValidatorsRoot)
	case *validatorpb.SignRequest_AttestationData:
		return handleAttestationData(ctx, validator, request, genesisValidatorsRoot)
	case *validatorpb.SignRequest_AggregateAttestationAndProof:
		// TODO: update to V2 sometime after release
		return handleAggregateAttestationAndProof(ctx, validator, request, genesisValidatorsRoot)
	case *validatorpb.SignRequest_AggregateAttestationAndProofElectra:
		return handleAggregateAttestationAndProofV2(ctx, version.Electra, validator, request, genesisValidatorsRoot)
	case *validatorpb.SignRequest_Slot:
		return handleAggregationSlot(ctx, validator, request, genesisValidatorsRoot)
	case *validatorpb.SignRequest_BlockAltair:
		return handleBlockAltair(ctx, validator, request, genesisValidatorsRoot)
	case *validatorpb.SignRequest_BlockBellatrix:
		return handleBlockBellatrix(ctx, validator, request, genesisValidatorsRoot)
	case *validatorpb.SignRequest_BlindedBlockBellatrix:
		return handleBlindedBlockBellatrix(ctx, validator, request, genesisValidatorsRoot)
	case *validatorpb.SignRequest_BlockCapella:
		return handleBlockCapella(ctx, validator, request, genesisValidatorsRoot)
	case *validatorpb.SignRequest_BlindedBlockCapella:
		return handleBlindedBlockCapella(ctx, validator, request, genesisValidatorsRoot)
	case *validatorpb.SignRequest_BlockDeneb:
		return handleBlockDeneb(ctx, validator, request, genesisValidatorsRoot)
	case *validatorpb.SignRequest_BlindedBlockDeneb:
		return handleBlindedBlockDeneb(ctx, validator, request, genesisValidatorsRoot)
	case *validatorpb.SignRequest_BlockElectra:
		return handleBlockElectra(ctx, validator, request, genesisValidatorsRoot)
	case *validatorpb.SignRequest_BlindedBlockElectra:
		return handleBlindedBlockElectra(ctx, validator, request, genesisValidatorsRoot)
	// We do not support "DEPOSIT" type.
	/*
		case *validatorpb.:
		return "DEPOSIT", nil
	*/

	case *validatorpb.SignRequest_Epoch:
		// tech debt that prysm uses signing type epoch
		return handleRandaoReveal(ctx, validator, request, genesisValidatorsRoot)
	case *validatorpb.SignRequest_Exit:
		return handleVoluntaryExit(ctx, validator, request, genesisValidatorsRoot)
	case *validatorpb.SignRequest_SyncMessageBlockRoot:
		return handleSyncMessageBlockRoot(ctx, validator, request, genesisValidatorsRoot)
	case *validatorpb.SignRequest_SyncAggregatorSelectionData:
		return handleSyncAggregatorSelectionData(ctx, validator, request, genesisValidatorsRoot)
	case *validatorpb.SignRequest_ContributionAndProof:
		return handleContributionAndProof(ctx, validator, request, genesisValidatorsRoot)
	case *validatorpb.SignRequest_Registration:
		return handleRegistration(ctx, validator, request)
	default:
		return nil, fmt.Errorf("web3signer sign request type %T not supported", request.Object)
	}
}

func handleBlock(ctx context.Context, validator *validator.Validate, request *validatorpb.SignRequest, genesisValidatorsRoot []byte) ([]byte, error) {
	bockSignRequest, err := types.GetBlockSignRequest(request, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	if err = validator.StructCtx(ctx, bockSignRequest); err != nil {
		return nil, err
	}
	blockSignRequestsTotal.Inc()
	return json.Marshal(bockSignRequest)
}

func handleAttestationData(ctx context.Context, validator *validator.Validate, request *validatorpb.SignRequest, genesisValidatorsRoot []byte) ([]byte, error) {
	attestationSignRequest, err := types.GetAttestationSignRequest(request, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	if err = validator.StructCtx(ctx, attestationSignRequest); err != nil {
		return nil, err
	}
	attestationSignRequestsTotal.Inc()
	return json.Marshal(attestationSignRequest)
}

func handleAggregateAttestationAndProof(ctx context.Context, validator *validator.Validate, request *validatorpb.SignRequest, genesisValidatorsRoot []byte) ([]byte, error) {
	aggregateAndProofSignRequest, err := types.GetAggregateAndProofSignRequest(request, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	if err = validator.StructCtx(ctx, aggregateAndProofSignRequest); err != nil {
		return nil, err
	}
	aggregateAndProofSignRequestsTotal.Inc()
	return json.Marshal(aggregateAndProofSignRequest)
}

func handleAggregateAttestationAndProofV2(ctx context.Context, fork int, validator *validator.Validate, request *validatorpb.SignRequest, genesisValidatorsRoot []byte) ([]byte, error) {
	aggregateAndProofSignRequestV2, err := types.GetAggregateAndProofV2SignRequest(fork, request, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	if err = validator.StructCtx(ctx, aggregateAndProofSignRequestV2); err != nil {
		return nil, err
	}
	aggregateAndProofSignRequestsTotal.Inc()
	return json.Marshal(aggregateAndProofSignRequestV2)
}

func handleAggregationSlot(ctx context.Context, validator *validator.Validate, request *validatorpb.SignRequest, genesisValidatorsRoot []byte) ([]byte, error) {
	aggregationSlotSignRequest, err := types.GetAggregationSlotSignRequest(request, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	if err = validator.StructCtx(ctx, aggregationSlotSignRequest); err != nil {
		return nil, err
	}
	aggregationSlotSignRequestsTotal.Inc()
	return json.Marshal(aggregationSlotSignRequest)
}

func handleBlockAltair(ctx context.Context, validator *validator.Validate, request *validatorpb.SignRequest, genesisValidatorsRoot []byte) ([]byte, error) {
	blockv2AltairSignRequest, err := types.GetBlockAltairSignRequest(request, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	if err = validator.StructCtx(ctx, blockv2AltairSignRequest); err != nil {
		return nil, err
	}
	blockAltairSignRequestsTotal.Inc()
	return json.Marshal(blockv2AltairSignRequest)
}

func handleBlockBellatrix(ctx context.Context, validator *validator.Validate, request *validatorpb.SignRequest, genesisValidatorsRoot []byte) ([]byte, error) {
	blockv2BellatrixSignRequest, err := types.GetBlockV2BlindedSignRequest(request, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	if err = validator.StructCtx(ctx, blockv2BellatrixSignRequest); err != nil {
		return nil, err
	}
	blockBellatrixSignRequestsTotal.Inc()
	return json.Marshal(blockv2BellatrixSignRequest)
}

func handleBlindedBlockBellatrix(ctx context.Context, validator *validator.Validate, request *validatorpb.SignRequest, genesisValidatorsRoot []byte) ([]byte, error) {
	blindedBlockv2SignRequest, err := types.GetBlockV2BlindedSignRequest(request, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	if err = validator.StructCtx(ctx, blindedBlockv2SignRequest); err != nil {
		return nil, err
	}
	blindedBlockBellatrixSignRequestsTotal.Inc()
	return json.Marshal(blindedBlockv2SignRequest)
}

func handleBlockCapella(ctx context.Context, validator *validator.Validate, request *validatorpb.SignRequest, genesisValidatorsRoot []byte) ([]byte, error) {
	blockv2CapellaSignRequest, err := types.GetBlockV2BlindedSignRequest(request, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	if err = validator.StructCtx(ctx, blockv2CapellaSignRequest); err != nil {
		return nil, err
	}
	blockCapellaSignRequestsTotal.Inc()
	return json.Marshal(blockv2CapellaSignRequest)
}

func handleBlindedBlockCapella(ctx context.Context, validator *validator.Validate, request *validatorpb.SignRequest, genesisValidatorsRoot []byte) ([]byte, error) {
	blindedBlockv2CapellaSignRequest, err := types.GetBlockV2BlindedSignRequest(request, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	if err = validator.StructCtx(ctx, blindedBlockv2CapellaSignRequest); err != nil {
		return nil, err
	}
	blindedBlockCapellaSignRequestsTotal.Inc()
	return json.Marshal(blindedBlockv2CapellaSignRequest)
}

func handleBlockDeneb(ctx context.Context, validator *validator.Validate, request *validatorpb.SignRequest, genesisValidatorsRoot []byte) ([]byte, error) {
	blockv2DenebSignRequest, err := types.GetBlockV2BlindedSignRequest(request, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	if err = validator.StructCtx(ctx, blockv2DenebSignRequest); err != nil {
		return nil, err
	}
	blockDenebSignRequestsTotal.Inc()
	return json.Marshal(blockv2DenebSignRequest)
}

func handleBlindedBlockDeneb(ctx context.Context, validator *validator.Validate, request *validatorpb.SignRequest, genesisValidatorsRoot []byte) ([]byte, error) {
	blindedBlockv2DenebSignRequest, err := types.GetBlockV2BlindedSignRequest(request, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	if err = validator.StructCtx(ctx, blindedBlockv2DenebSignRequest); err != nil {
		return nil, err
	}
	blindedBlockDenebSignRequestsTotal.Inc()
	return json.Marshal(blindedBlockv2DenebSignRequest)
}

func handleBlockElectra(ctx context.Context, validator *validator.Validate, request *validatorpb.SignRequest, genesisValidatorsRoot []byte) ([]byte, error) {
	blockv2ElectraSignRequest, err := types.GetBlockV2BlindedSignRequest(request, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	if err = validator.StructCtx(ctx, blockv2ElectraSignRequest); err != nil {
		return nil, err
	}
	remoteBlockSignRequestsTotal.WithLabelValues("electra", "false").Inc()
	return json.Marshal(blockv2ElectraSignRequest)
}

func handleBlindedBlockElectra(ctx context.Context, validator *validator.Validate, request *validatorpb.SignRequest, genesisValidatorsRoot []byte) ([]byte, error) {
	blindedBlockv2ElectraSignRequest, err := types.GetBlockV2BlindedSignRequest(request, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	if err = validator.StructCtx(ctx, blindedBlockv2ElectraSignRequest); err != nil {
		return nil, err
	}
	remoteBlockSignRequestsTotal.WithLabelValues("electra", "true").Inc()
	return json.Marshal(blindedBlockv2ElectraSignRequest)
}

func handleRandaoReveal(ctx context.Context, validator *validator.Validate, request *validatorpb.SignRequest, genesisValidatorsRoot []byte) ([]byte, error) {
	randaoRevealSignRequest, err := types.GetRandaoRevealSignRequest(request, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	if err = validator.StructCtx(ctx, randaoRevealSignRequest); err != nil {
		return nil, err
	}
	randaoRevealSignRequestsTotal.Inc()
	return json.Marshal(randaoRevealSignRequest)
}

func handleVoluntaryExit(ctx context.Context, validator *validator.Validate, request *validatorpb.SignRequest, genesisValidatorsRoot []byte) ([]byte, error) {
	voluntaryExitRequest, err := types.GetVoluntaryExitSignRequest(request, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	if err = validator.StructCtx(ctx, voluntaryExitRequest); err != nil {
		return nil, err
	}
	voluntaryExitSignRequestsTotal.Inc()
	return json.Marshal(voluntaryExitRequest)
}

func handleSyncMessageBlockRoot(ctx context.Context, validator *validator.Validate, request *validatorpb.SignRequest, genesisValidatorsRoot []byte) ([]byte, error) {
	syncCommitteeMessageRequest, err := types.GetSyncCommitteeMessageSignRequest(request, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	if err = validator.StructCtx(ctx, syncCommitteeMessageRequest); err != nil {
		return nil, err
	}
	syncCommitteeMessageSignRequestsTotal.Inc()
	return json.Marshal(syncCommitteeMessageRequest)
}

func handleSyncAggregatorSelectionData(ctx context.Context, validator *validator.Validate, request *validatorpb.SignRequest, genesisValidatorsRoot []byte) ([]byte, error) {
	syncCommitteeSelectionProofRequest, err := types.GetSyncCommitteeSelectionProofSignRequest(request, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	if err = validator.StructCtx(ctx, syncCommitteeSelectionProofRequest); err != nil {
		return nil, err
	}
	syncCommitteeSelectionProofSignRequestsTotal.Inc()
	return json.Marshal(syncCommitteeSelectionProofRequest)
}

func handleContributionAndProof(ctx context.Context, validator *validator.Validate, request *validatorpb.SignRequest, genesisValidatorsRoot []byte) ([]byte, error) {
	contributionAndProofRequest, err := types.GetSyncCommitteeContributionAndProofSignRequest(request, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	if err = validator.StructCtx(ctx, contributionAndProofRequest); err != nil {
		return nil, err
	}
	syncCommitteeContributionAndProofSignRequestsTotal.Inc()
	return json.Marshal(contributionAndProofRequest)
}

func handleRegistration(ctx context.Context, validator *validator.Validate, request *validatorpb.SignRequest) ([]byte, error) {
	validatorRegistrationRequest, err := types.GetValidatorRegistrationSignRequest(request)
	if err != nil {
		return nil, err
	}
	if err = validator.StructCtx(ctx, validatorRegistrationRequest); err != nil {
		return nil, err
	}
	validatorRegistrationSignRequestsTotal.Inc()
	return json.Marshal(validatorRegistrationRequest)
}

// SubscribeAccountChanges returns the event subscription for changes to public keys.
func (km *Keymanager) SubscribeAccountChanges(pubKeysChan chan [][fieldparams.BLSPubkeyLength]byte) event.Subscription {
	return km.accountsChangedFeed.Subscribe(pubKeysChan)
}

// ExtractKeystores is not supported for the remote-web3signer keymanager type.
func (*Keymanager) ExtractKeystores(
	_ context.Context, _ []bls.PublicKey, _ string,
) ([]*keymanager.Keystore, error) {
	return nil, errors.New("extracting keys is not supported for a web3signer keymanager")
}

// DeleteKeystores is not supported for the remote-web3signer keymanager type.
func (km *Keymanager) DeleteKeystores(context.Context, [][]byte) ([]*keymanager.KeyStatus, error) {
	return nil, errors.New("Wrong wallet type: web3-signer. Only Imported or Derived wallets can delete accounts")
}

func (km *Keymanager) ListKeymanagerAccounts(ctx context.Context, cfg keymanager.ListKeymanagerAccountConfig) error {
	au := aurora.NewAurora(true)
	fmt.Printf("(keymanager kind) %s\n", au.BrightGreen("web3signer").Bold())
	fmt.Printf(
		"(configuration file path) %s\n",
		au.BrightGreen(filepath.Join(cfg.WalletAccountsDir, cfg.KeymanagerConfigFileName)).Bold(),
	)
	fmt.Println(" ")
	fmt.Printf("%s\n", au.BrightGreen("Setup Configuration").Bold())
	fmt.Println(" ")
	//TODO: add config options, may require refactor again
	validatingPubKeys, err := km.FetchValidatingPublicKeys(ctx)
	if err != nil {
		return errors.Wrap(err, "could not fetch validating public keys")
	}
	if len(validatingPubKeys) == 1 {
		fmt.Print("Showing 1 validator account\n")
	} else if len(validatingPubKeys) == 0 {
		fmt.Print("No accounts found\n")
		return nil
	} else {
		fmt.Printf("Showing %d validator accounts\n", len(validatingPubKeys))
	}
	DisplayRemotePublicKeys(validatingPubKeys)
	return nil
}

// DisplayRemotePublicKeys prints remote public keys to stdout.
func DisplayRemotePublicKeys(validatingPubKeys [][48]byte) {
	au := aurora.NewAurora(true)
	for i := 0; i < len(validatingPubKeys); i++ {
		fmt.Println("")
		fmt.Printf(
			"%s\n", au.BrightGreen(petnames.DeterministicName(validatingPubKeys[i][:], "-")).Bold(),
		)
		// Retrieve the validating key account metadata.
		fmt.Printf("%s %#x\n", au.BrightCyan("[validating public key]").Bold(), validatingPubKeys[i])
		fmt.Println(" ")
	}
}

// AddPublicKeys imports a list of public keys into the keymanager for web3signer use. Returns status with message.
func (km *Keymanager) AddPublicKeys(pubKeys []string) ([]*keymanager.KeyStatus, error) {
	importedRemoteKeysStatuses := make([]*keymanager.KeyStatus, len(pubKeys))
	// Using a map to track both existing and new public keys efficiently
	combinedKeys := make(map[string][48]byte)

	// Populate the map with existing keys
	km.lock.RLock()
	originalKeysLen := len(km.providedPublicKeys)
	for _, key := range km.providedPublicKeys {
		encodedKey := hexutil.Encode(key[:])
		combinedKeys[encodedKey] = key
	}
	km.lock.RUnlock()

	for i, pubkey := range pubKeys {
		pubkeyBytes, err := hexutil.Decode(pubkey)
		if err != nil {
			importedRemoteKeysStatuses[i] = &keymanager.KeyStatus{
				Status:  keymanager.StatusError,
				Message: err.Error(),
			}
			continue
		}
		if len(pubkeyBytes) != fieldparams.BLSPubkeyLength {
			importedRemoteKeysStatuses[i] = &keymanager.KeyStatus{
				Status:  keymanager.StatusError,
				Message: fmt.Sprintf("pubkey byte length (%d) did not match bls pubkey byte length (%d)", len(pubkeyBytes), fieldparams.BLSPubkeyLength),
			}
			continue
		}

		encodedPubkey := hexutil.Encode(pubkeyBytes)
		if _, exists := combinedKeys[encodedPubkey]; exists {
			importedRemoteKeysStatuses[i] = &keymanager.KeyStatus{
				Status:  keymanager.StatusDuplicate,
				Message: fmt.Sprintf("Duplicate pubkey: %v, already in use", pubkey),
			}
			continue
		}

		// Add the new key to the map
		combinedKeys[encodedPubkey] = bytesutil.ToBytes48(pubkeyBytes)
		importedRemoteKeysStatuses[i] = &keymanager.KeyStatus{
			Status:  keymanager.StatusImported,
			Message: fmt.Sprintf("Successfully added pubkey: %v", pubkey),
		}
		log.Debug("Added pubkey to keymanager for web3signer", "pubkey", pubkey)
	}

	if originalKeysLen != len(combinedKeys) {
		if km.keyFilePath != "" {
			if err := km.savePublicKeysToFile(combinedKeys); err != nil {
				return nil, err
			}
		} else {
			km.updatePublicKeys(maps.Values(combinedKeys))
		}
	}

	return importedRemoteKeysStatuses, nil
}

// DeletePublicKeys removes a list of public keys from the keymanager for web3signer use. Returns status with message.
func (km *Keymanager) DeletePublicKeys(publicKeys []string) ([]*keymanager.KeyStatus, error) {
	deletedRemoteKeysStatuses := make([]*keymanager.KeyStatus, len(publicKeys))
	// Using a map to track both existing and new public keys efficiently
	combinedKeys := make(map[string][48]byte)
	km.lock.RLock()
	originalKeysLen := len(km.providedPublicKeys)
	if originalKeysLen == 0 {
		for i := range deletedRemoteKeysStatuses {
			deletedRemoteKeysStatuses[i] = &keymanager.KeyStatus{
				Status:  keymanager.StatusNotFound,
				Message: "No pubkeys are set in validator",
			}
		}
		return deletedRemoteKeysStatuses, nil
	}

	// Populate the map with existing keys
	for _, key := range km.providedPublicKeys {
		encodedKey := hexutil.Encode(key[:])
		combinedKeys[encodedKey] = key
	}
	km.lock.RUnlock()

	for i, pubkey := range publicKeys {
		pubkeyBytes, err := hexutil.Decode(pubkey)
		if err != nil {
			deletedRemoteKeysStatuses[i] = &keymanager.KeyStatus{
				Status:  keymanager.StatusError,
				Message: err.Error(),
			}
			continue
		}
		if len(pubkeyBytes) != fieldparams.BLSPubkeyLength {
			deletedRemoteKeysStatuses[i] = &keymanager.KeyStatus{
				Status:  keymanager.StatusError,
				Message: fmt.Sprintf("pubkey byte length (%d) did not match bls pubkey byte length (%d)", len(pubkeyBytes), fieldparams.BLSPubkeyLength),
			}
			continue
		}
		_, exists := combinedKeys[pubkey]
		if !exists {
			deletedRemoteKeysStatuses[i] = &keymanager.KeyStatus{
				Status:  keymanager.StatusNotFound,
				Message: fmt.Sprintf("Pubkey: %v not found", pubkey),
			}
			continue
		}
		delete(combinedKeys, pubkey)
		deletedRemoteKeysStatuses[i] = &keymanager.KeyStatus{
			Status:  keymanager.StatusDeleted,
			Message: fmt.Sprintf("Successfully deleted pubkey: %v", pubkey),
		}
		log.WithField("pubkey", pubkey).Debug("Deleted pubkey from keymanager for remote signer")
	}

	if originalKeysLen != len(combinedKeys) {
		if km.keyFilePath != "" {
			if err := km.savePublicKeysToFile(combinedKeys); err != nil {
				return nil, err
			}
		} else {
			km.updatePublicKeys(maps.Values(combinedKeys))
		}
	}

	return deletedRemoteKeysStatuses, nil
}
