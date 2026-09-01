package remote_web3signer

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/async/event"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/io/file"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	validatorpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/validator-client"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/OffchainLabs/prysm/v7/validator/accounts/petnames"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager/remote-web3signer/internal"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager/remote-web3signer/types"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-playground/validator/v10"
	"github.com/logrusorgru/aurora"
	"github.com/pkg/errors"
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

	// PollInterval makes the keymanager re-fetch the URL (PublicKeysURL) on this interval to
	// hot-reload the validating public key set. A failed or empty response keeps the current keys.
	// Note: Zero or negative disables polling.
	PollInterval time.Duration
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
	// serialize updatePublicKeys as we have multiple concurrent updaters
	// - URL poller, file watcher, Keymanager API.
	updateLock sync.Mutex
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
		switch {
		case err == nil:
			ppk = providedPublicKeys
		case cfg.PollInterval > 0 && !keyFileExists:
			// Polling will retry, so start with no keys rather than refusing to start.
			erroredResponsesTotal.Inc()
			log.
				WithError(err).
				WithField("url", api.RedactEndpoint(cfg.PublicKeysURL)).
				Warn("Could not get public keys from the remote signer URL, starting with no validating keys and retrying on the next poll")
		default:
			erroredResponsesTotal.Inc()
			return nil, errors.Wrapf(err, "could not get public keys from remote server URL %v", api.RedactEndpoint(cfg.PublicKeysURL))
		}
	} else if len(cfg.ProvidedPublicKeys) != 0 {
		ppk = cfg.ProvidedPublicKeys
	}

	// use a map to remove duplicates
	flagLoadedKeys := make(map[string][48]byte)

	// Populate the map with existing keys
	for _, key := range ppk {
		b, err := bytesutil.DecodeHex48(key)
		if err != nil {
			return nil, fmt.Errorf("decode public key %s: %w", key, err)
		}
		flagLoadedKeys[key] = b
	}
	km.flagLoadedKeysMap = flagLoadedKeys

	// File-based key source is not set here. Start polling from the URL if configured, and hot-reload from file if configured.
	if !keyFileExists {
		// Load provided public keys in memory.
		km.lock.Lock()
		km.providedPublicKeys = slices.Collect(maps.Values(flagLoadedKeys))
		km.lock.Unlock()

		go km.pollRemoteKeysFromURL(ctx, cfg.PublicKeysURL, cfg.PollInterval)

		return km, nil
	}

	log.WithField("file", km.keyFilePath).Info("Loading keys from file")

	// Notify users that poll interval flag is a no-op when a key file is provided.
	if cfg.PollInterval > 0 {
		log.Warn("Web3Signer key poll interval is set but key file is configured. " +
			"The poll interval will be ignored since the key file takes precedence over the URL.")
	}

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
	km.providedPublicKeys = slices.Collect(maps.Values(fileKeys))
	km.lock.Unlock()
	watcherReady := make(chan error, 1)
	var watcherReadyOnce sync.Once
	markWatcherReady := func(err error) {
		watcherReadyOnce.Do(func() { watcherReady <- err })
	}
	go func() {
		watchErr := km.refreshRemoteKeysFromFileChangesWithRetry(ctx, retryDelay, markWatcherReady)
		if watchErr != nil {
			markWatcherReady(watchErr)
			log.WithError(watchErr).Error("Could not refresh remote keys from file changes")
		}
	}()
	select {
	case err := <-watcherReady:
		if err != nil {
			return nil, errors.Wrap(err, "could not initialize remote signer key file watcher")
		}
	case <-ctx.Done():
		return nil, errors.Wrap(ctx.Err(), "could not initialize remote signer key file watcher")
	}

	return km, nil
}

// equalKeySet reports whether a and b contain the same set of keys, ignoring
// order and duplicates.
func equalKeySet(a, b [][fieldparams.BLSPubkeyLength]byte) bool {
	setA := make(map[[fieldparams.BLSPubkeyLength]byte]struct{}, len(a))
	for _, k := range a {
		setA[k] = struct{}{}
	}
	distinctB := make(map[[fieldparams.BLSPubkeyLength]byte]struct{}, len(b))
	for _, k := range b {
		if _, ok := setA[k]; !ok {
			return false // b has a key that a does not.
		}
		distinctB[k] = struct{}{}
	}

	return len(setA) == len(distinctB)
}

func (km *Keymanager) updatePublicKeys(keys [][48]byte) {
	km.updateLock.Lock()
	defer km.updateLock.Unlock()

	km.lock.Lock()
	if equalKeySet(km.providedPublicKeys, keys) {
		km.lock.Unlock()
		return
	}
	km.providedPublicKeys = keys
	km.lock.Unlock()

	km.accountsChangedFeed.Send(keys)
	log.WithField("count", len(keys)).Debug("Updated public keys")
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
	ver := slots.ToForkVersion(request.SigningSlot)
	switch request.Object.(type) {
	case *validatorpb.SignRequest_Block:
		return handleBlock(ctx, validator, request, genesisValidatorsRoot)
	case *validatorpb.SignRequest_AttestationData:
		return handleAttestationData(ctx, validator, request, genesisValidatorsRoot)
	case *validatorpb.SignRequest_AggregateAttestationAndProof, *validatorpb.SignRequest_AggregateAttestationAndProofElectra:
		return handleAggregateAttestationAndProofV2(ctx, ver, validator, request, genesisValidatorsRoot)
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
	case *validatorpb.SignRequest_BlockFulu:
		return handleBlockFulu(ctx, validator, request, genesisValidatorsRoot)
	case *validatorpb.SignRequest_BlindedBlockFulu:
		return handleBlindedBlockFulu(ctx, validator, request, genesisValidatorsRoot)
	case *validatorpb.SignRequest_BlockGloas:
		// TODO: Implement Gloas block signing for web3signer.
		return nil, fmt.Errorf("web3signer Gloas block signing not yet implemented")
	case *validatorpb.SignRequest_ExecutionPayloadEnvelope:
		// TODO: Implement execution payload envelope signing for web3signer.
		return nil, fmt.Errorf("web3signer execution payload envelope signing not yet implemented")
	case *validatorpb.SignRequest_ProposerPreference:
		// TODO: Implement proposer preferences signing for web3signer.
		return nil, fmt.Errorf("web3signer proposer preferences signing not yet implemented")
	case *validatorpb.SignRequest_BuilderRequestAuth:
		// TODO: Implement builder request auth signing for web3signer.
		return nil, fmt.Errorf("web3signer builder request auth signing not yet implemented")

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
	remoteBlockSignRequestsTotal.WithLabelValues("phase0", "false").Inc()
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
	remoteBlockSignRequestsTotal.WithLabelValues("altair", "false").Inc()
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
	remoteBlockSignRequestsTotal.WithLabelValues("bellatrix", "false").Inc()
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
	remoteBlockSignRequestsTotal.WithLabelValues("bellatrix", "true").Inc()
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
	remoteBlockSignRequestsTotal.WithLabelValues("capella", "false").Inc()
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
	remoteBlockSignRequestsTotal.WithLabelValues("capella", "true").Inc()
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
	remoteBlockSignRequestsTotal.WithLabelValues("deneb", "false").Inc()
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
	remoteBlockSignRequestsTotal.WithLabelValues("deneb", "true").Inc()
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

func handleBlockFulu(ctx context.Context, validator *validator.Validate, request *validatorpb.SignRequest, genesisValidatorsRoot []byte) ([]byte, error) {
	blockv2FuluSignRequest, err := types.GetBlockV2BlindedSignRequest(request, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	if err = validator.StructCtx(ctx, blockv2FuluSignRequest); err != nil {
		return nil, err
	}
	remoteBlockSignRequestsTotal.WithLabelValues("fulu", "false").Inc()
	return json.Marshal(blockv2FuluSignRequest)
}

func handleBlindedBlockFulu(ctx context.Context, validator *validator.Validate, request *validatorpb.SignRequest, genesisValidatorsRoot []byte) ([]byte, error) {
	blindedBlockv2FuluSignRequest, err := types.GetBlockV2BlindedSignRequest(request, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	if err = validator.StructCtx(ctx, blindedBlockv2FuluSignRequest); err != nil {
		return nil, err
	}
	remoteBlockSignRequestsTotal.WithLabelValues("fulu", "true").Inc()
	return json.Marshal(blindedBlockv2FuluSignRequest)
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
	for i := range validatingPubKeys {
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
			km.updatePublicKeys(slices.Collect(maps.Values(combinedKeys)))
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
		km.lock.RUnlock()
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
			km.updatePublicKeys(slices.Collect(maps.Values(combinedKeys)))
		}
	}

	return deletedRemoteKeysStatuses, nil
}
