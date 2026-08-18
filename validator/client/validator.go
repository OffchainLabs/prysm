// Package client represents a gRPC polling-based implementation
// of an Ethereum validator client.
package client

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/client"
	eventClient "github.com/OffchainLabs/prysm/v7/api/client/event"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/async/event"
	"github.com/OffchainLabs/prysm/v7/cmd"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/config/proposer"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	validatortypes "github.com/OffchainLabs/prysm/v7/consensus-types/validator"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	validatorpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/validator-client"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	accountsiface "github.com/OffchainLabs/prysm/v7/validator/accounts/iface"
	"github.com/OffchainLabs/prysm/v7/validator/accounts/wallet"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	"github.com/OffchainLabs/prysm/v7/validator/db"
	"github.com/OffchainLabs/prysm/v7/validator/graffiti"
	validatorHelpers "github.com/OffchainLabs/prysm/v7/validator/helpers"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager/local"
	remoteweb3signer "github.com/OffchainLabs/prysm/v7/validator/keymanager/remote-web3signer"
	"github.com/dgraph-io/ristretto/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// keyFetchPeriod is the frequency that we try to refetch validating keys
// in case no keys were fetched previously.
var (
	ErrBuilderValidatorRegistration = errors.New("Builder API validator registration unsuccessful")
	ErrValidatorsAllExited          = errors.New("All validators are exited, no more work to perform...")
)

var (
	msgCouldNotFetchKeys = "could not fetch validating keys"
	msgNoKeysFetched     = "No validating keys fetched. Waiting for keys..."
)

type validator struct {
	distributed                  bool
	enableAPI                    bool
	disableDutiesPolling         bool
	emitAccountMetrics           bool
	logValidatorPerformance      bool
	submissionLogsLock           sync.Mutex
	highestValidSlotLock         sync.Mutex
	blacklistedPubkeysLock       sync.RWMutex
	prevEpochBalancesLock        sync.RWMutex
	attestedSlotsLock            sync.RWMutex
	cachedAttestationDataLock    sync.RWMutex
	submittedPrefSlotsLock       sync.RWMutex
	signedRequestAuthsLock       sync.Mutex
	domainDataLock               sync.RWMutex
	cachedAttestationData        *ethpb.AttestationData
	graffitiOrderedIndex         uint64
	walletInitializedFeed        *event.Feed
	walletInitializedChan        chan *wallet.Wallet
	wallet                       *wallet.Wallet
	accountsChangedChannel       chan [][fieldparams.BLSPubkeyLength]byte
	blacklistedPubkeys           map[[fieldparams.BLSPubkeyLength]byte]bool
	prevEpochBalances            map[[fieldparams.BLSPubkeyLength]byte]uint64
	startBalances                map[[fieldparams.BLSPubkeyLength]byte]uint64
	attestedSlotsByKeyByEpoch    map[primitives.Epoch]map[[fieldparams.BLSPubkeyLength]byte]primitives.Slot
	web3SignerConfig             *remoteweb3signer.SetupConfig
	proposerSettings             *proposer.Settings // clone-then-swap under proposerSettingsMu; reads are lock-free
	proposerSettingsMu           sync.Mutex
	submittedPrefSlots           map[primitives.Slot]bool
	connTracker                  connTracker // per push kind, the conn generation last confirmed pushed
	submittedAtts                map[submittedAttKey]*submittedAtt
	submittedAggregates          map[submittedAttKey]*submittedAtt
	submittedSyncMessages        map[slotRootKey][]uint64
	submittedSyncContributions   map[slotRootKey]*submittedSyncContribution
	submittedPayloadAtts         map[submittedPayloadAttKey][]uint64
	validatorsRegBatchSize       int
	duties                       *dutyStore
	nextFetchInFlight            atomic.Bool
	doppelGanger                 doppelGangerTracker
	domainDataCache              *ristretto.Cache[string, proto.Message]
	slotFeed                     *event.Feed
	graffitiStruct               *graffiti.Graffiti
	highestValidSlot             primitives.Slot
	eventsChannel                chan *eventClient.Event
	payloadAvailability          *payloadAvailability
	head                         *headTracker
	pubkeyToStatus               map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus
	pubkeyToStatusLock           sync.RWMutex // guards pubkeyToStatus; all readers go through statusCache
	signedValidatorRegistrations map[[fieldparams.BLSPubkeyLength]byte]*ethpb.SignedValidatorRegistrationV1
	signedRequestAuths           map[requestAuthKey]*ethpb.SignedRequestAuthV1
	aggSelector                  aggregatorSelector
	validatorClient              iface.ValidatorClient
	chainClient                  iface.ChainClient
	nodeClient                   iface.NodeClient
	db                           db.Database
	conn                         *validatorHelpers.NodeConnection
	accountChangedSub            event.Subscription
	ticker                       slots.Ticker
	km                           keymanager.IKeymanager
	graffiti                     []byte
	genesisTime                  time.Time
	voteStats                    voteStats
}

type validatorStatus struct {
	publicKey []byte
	status    *ethpb.ValidatorStatusResponse
	index     primitives.ValidatorIndex
}

// signingFunc defines a type for the function that signs a message.
type signingFunc func(context.Context, *validatorpb.SignRequest) (bls.Signature, error)

// statusCache returns pubkeyToStatus for lock-free reads: entries are never
// mutated in place, updateValidatorStatusCache replaces the map wholesale.
func (v *validator) statusCache() map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus {
	v.pubkeyToStatusLock.RLock()
	defer v.pubkeyToStatusLock.RUnlock()
	return v.pubkeyToStatus
}

func (v *validator) indexFromPubkey(pubKey [fieldparams.BLSPubkeyLength]byte) (primitives.ValidatorIndex, error) {
	s, ok := v.statusCache()[pubKey]
	if !ok {
		return 0, fmt.Errorf("validator index not found for pubkey %#x", pubKey)
	}
	return s.index, nil
}

// Done cleans up the validator.
func (v *validator) Done() {
	if v.accountChangedSub != nil {
		v.accountChangedSub.Unsubscribe()
	}
	if v.ticker != nil {
		v.ticker.Done()
	}
}

func (v *validator) GenesisTime() time.Time {
	return v.genesisTime
}

func (v *validator) EventsChan() <-chan *eventClient.Event {
	return v.eventsChannel
}

func (v *validator) AccountsChangedChan() <-chan [][fieldparams.BLSPubkeyLength]byte {
	return v.accountsChangedChannel
}

// WaitForKeymanagerInitialization checks if the validator needs to wait for keymanager initialization.
func (v *validator) WaitForKeymanagerInitialization(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "validator.WaitForKeymanagerInitialization")
	defer span.End()

	switch {
	// When Web3Signer is configured, initialize the keymanager separately.
	case v.web3SignerConfig != nil:
		gvr, err := v.db.GenesisValidatorsRoot(ctx)
		if err != nil {
			return errors.Wrap(err, "unable to retrieve valid genesis validators root while initializing web3signer keymanager")
		}

		v.web3SignerConfig.GenesisValidatorsRoot = gvr
		keyManager, err := remoteweb3signer.NewKeymanager(ctx, v.web3SignerConfig)
		if err != nil {
			return errors.Wrap(err, "could not initialize web3signer keymanager")
		}
		v.km = keyManager
	case v.wallet != nil:
		keyManager, err := v.wallet.InitializeKeymanager(ctx, accountsiface.InitKeymanagerConfig{ListenForChanges: true})
		if err != nil {
			return errors.Wrap(err, "could not initialize key manager")
		}
		v.km = keyManager
	case v.enableAPI:
		km, err := waitForWebWalletInitialization(ctx, v.walletInitializedFeed, v.walletInitializedChan)
		if err != nil {
			return err
		}
		v.km = km
	default:
		return wallet.ErrNoWalletFound
	}

	if v.km == nil {
		return errors.New("key manager not set")
	}
	recheckKeys(ctx, v.db, v.km)
	if err := v.snapshotBootKeysForDoppelGanger(ctx); err != nil {
		return err
	}
	v.accountChangedSub = v.km.SubscribeAccountChanges(v.accountsChangedChannel)
	return nil
}

// subscribe to channel for when the wallet is initialized
func waitForWebWalletInitialization(
	ctx context.Context,
	walletInitializedEvent *event.Feed,
	walletChan chan *wallet.Wallet,
) (keymanager.IKeymanager, error) {
	ctx, span := trace.StartSpan(ctx, "validator.waitForWebWalletInitialization")
	defer span.End()

	log.Info("Waiting for keymanager to initialize validator client with web UI or /v2/validator/wallet/create REST api")
	sub := walletInitializedEvent.Subscribe(walletChan)
	defer sub.Unsubscribe()
	for {
		select {
		case w := <-walletChan:
			keyManager, err := w.InitializeKeymanager(ctx, accountsiface.InitKeymanagerConfig{ListenForChanges: true})
			if err != nil {
				return nil, errors.Wrap(err, "could not read keymanager")
			}
			return keyManager, nil
		case <-ctx.Done():
			return nil, errors.New("context canceled")
		case <-sub.Err():
			log.Error("Subscriber closed, exiting goroutine")
			return nil, nil
		}
	}
}

// recheckKeys checks if the validator has any keys that need to be rechecked.
// The keymanager implements a subscription to push these updates to the validator.
func recheckKeys(ctx context.Context, valDB db.Database, km keymanager.IKeymanager) {
	ctx, span := trace.StartSpan(ctx, "validator.recheckKeys")
	defer span.End()

	var validatingKeys [][fieldparams.BLSPubkeyLength]byte
	var err error
	validatingKeys, err = km.FetchValidatingPublicKeys(ctx)
	if err != nil {
		log.WithError(err).Debug("Could not fetch validating keys")
	}
	if err := valDB.UpdatePublicKeysBuckets(validatingKeys); err != nil {
		go recheckValidatingKeysBucket(ctx, valDB, km)
	}
}

// to accounts changes in the keymanager, then updates those keys'
// buckets in bolt DB if a bucket for a key does not exist.
func recheckValidatingKeysBucket(ctx context.Context, valDB db.Database, km keymanager.IKeymanager) {
	ctx, span := trace.StartSpan(ctx, "validator.recheckValidatingKeysBucket")
	defer span.End()

	importedKeymanager, ok := km.(*local.Keymanager)
	if !ok {
		return
	}
	validatingPubKeysChan := make(chan [][fieldparams.BLSPubkeyLength]byte, 1)
	sub := importedKeymanager.SubscribeAccountChanges(validatingPubKeysChan)
	defer func() {
		sub.Unsubscribe()
		close(validatingPubKeysChan)
	}()
	for {
		select {
		case keys := <-validatingPubKeysChan:
			if err := valDB.UpdatePublicKeysBuckets(keys); err != nil {
				log.WithError(err).Debug("Could not update public keys buckets")
				continue
			}
		case <-ctx.Done():
			return
		case <-sub.Err():
			log.Error("Subscriber closed, exiting goroutine")
			return
		}
	}
}

// WaitForChainStart checks whether the beacon node has started its runtime. That is,
// it calls to the beacon node which then verifies the ETH1.0 deposit contract logs to check
// for the ChainStart log to have been emitted. If so, it starts a ticker based on the ChainStart
// unix timestamp which will be used to keep track of time within the validator client.
func (v *validator) WaitForChainStart(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "validator.WaitForChainStart")
	defer span.End()

	// First, check if the beacon chain has started.
	log.Info("Syncing with beacon node to align on chain genesis info")

	chainStartRes, err := v.validatorClient.WaitForChainStart(ctx, &emptypb.Empty{})
	if errors.Is(err, io.EOF) {
		return client.ErrConnectionIssue
	}

	if errors.Is(ctx.Err(), context.Canceled) {
		return errors.Wrap(ctx.Err(), "context has been canceled so shutting down the loop")
	}

	if err != nil {
		return errors.Wrap(
			client.ErrConnectionIssue,
			errors.Wrap(err, "could not receive ChainStart from stream").Error(),
		)
	}

	v.genesisTime = time.Unix(int64(chainStartRes.GenesisTime), 0)

	curGenValRoot, err := v.db.GenesisValidatorsRoot(ctx)
	if err != nil {
		return errors.Wrap(err, "could not get current genesis validators root")
	}

	if len(curGenValRoot) == 0 {
		if err := v.db.SaveGenesisValidatorsRoot(ctx, chainStartRes.GenesisValidatorsRoot); err != nil {
			return errors.Wrap(err, "could not save genesis validators root")
		}

		return nil
	}

	if !bytes.Equal(curGenValRoot, chainStartRes.GenesisValidatorsRoot) {
		log.Errorf(`The genesis validators root received from the beacon node does not match what is in
			your validator database. This could indicate that this is a database meant for another network. If
			you were previously running this validator database on another network, please run --%s to
			clear the database. If not, please file an issue at https://github.com/prysmaticlabs/prysm/issues`,
			cmd.ClearDB.Name,
		)
		return fmt.Errorf(
			"genesis validators root from beacon node (%#x) does not match root saved in validator db (%#x)",
			chainStartRes.GenesisValidatorsRoot,
			curGenValRoot,
		)
	}

	return nil
}

func (v *validator) SetTicker() {
	// If a ticker already exists, stop it before creating a new one
	// to prevent resource leaks.

	// note to reader:
	// This function chooses to adapt to the existing slot ticker instead of changing how it works
	// The slot ticker will currently start from genesis time but tick based on the current time.
	// This means that sometimes we need to reset the ticker to avoid replaying old ticks on a slow consumer of the ticks.
	// i.e.,
	// 1. tick starts at 0
	// 2. loop stops consuming on slot 10 due to accounts changed tigger with no active keys
	// 3. new active keys are added in slot 20 resolving wait for activation
	// 4. new tick starts ticking from slot 20 instead of slot 10
	if v.ticker != nil {
		v.ticker.Done()
	}
	// Once the ChainStart log is received, we update the genesis time of the validator client
	// and begin a slot ticker used to track the current slot the beacon node is in.
	v.ticker = slots.NewSlotTicker(v.genesisTime, params.BeaconConfig().SlotDuration())
	log.WithField("genesisTime", v.genesisTime).Info("Beacon chain started")
}

// WaitForSync checks whether the beacon node has sync to the latest head.
func (v *validator) WaitForSync(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "validator.WaitForSync")
	defer span.End()

	s, err := v.nodeClient.SyncStatus(ctx, &emptypb.Empty{})
	if err != nil {
		return errors.Wrap(client.ErrConnectionIssue, errors.Wrap(err, "could not get sync status").Error())
	}
	if !s.Syncing {
		return nil
	}

	for {
		select {
		// Poll every half slot.
		case <-time.After(slots.DivideSlotBy(2 /* twice per slot */)):
			s, err := v.nodeClient.SyncStatus(ctx, &emptypb.Empty{})
			if err != nil {
				return errors.Wrap(client.ErrConnectionIssue, errors.Wrap(err, "could not get sync status").Error())
			}
			if !s.Syncing {
				return nil
			}
			log.Info("Waiting for beacon node to sync to latest chain head")
		case <-ctx.Done():
			return errors.New("context has been canceled, exiting goroutine")
		}
	}
}

func (v *validator) checkAndLogValidatorStatus() bool {
	nonexistentIndex := primitives.ValidatorIndex(^uint64(0))
	var someAreActive bool
	for _, s := range v.statusCache() {
		fields := logrus.Fields{
			"pubkey": fmt.Sprintf("%#x", bytesutil.Trunc(s.publicKey)),
			"status": s.status.Status.String(),
		}
		if s.index != nonexistentIndex {
			fields["validatorIndex"] = s.index
		}
		log := log.WithFields(fields)
		if v.emitAccountMetrics {
			fmtKey, fmtIndex := fmt.Sprintf("%#x", s.publicKey), fmt.Sprintf("%#x", s.index)
			ValidatorStatusesGaugeVec.WithLabelValues(fmtKey, fmtIndex).Set(float64(s.status.Status))
		}
		switch s.status.Status {
		case ethpb.ValidatorStatus_UNKNOWN_STATUS:
			log.Info("Waiting for deposit to be observed by beacon node")
		case ethpb.ValidatorStatus_DEPOSITED:
			log.Info("Validator deposited, entering activation queue after finalization")
		case ethpb.ValidatorStatus_PENDING:
			log.Info("Waiting for activation... Check validator queue status in a block explorer")
		case ethpb.ValidatorStatus_ACTIVE, ethpb.ValidatorStatus_EXITING:
			someAreActive = true
			log.WithFields(logrus.Fields{
				"index": s.index,
			}).Info("Validator activated")
		case ethpb.ValidatorStatus_EXITED:
			log.Info("Validator exited")
		case ethpb.ValidatorStatus_INVALID:
			log.Warn("Invalid Eth1 deposit")
		default:
			log.WithFields(logrus.Fields{
				"status": s.status.Status.String(),
			}).Info("Validator status")
		}
	}
	return someAreActive
}

// NextSlot emits the next slot number at the start time of that slot.
func (v *validator) NextSlot() <-chan primitives.Slot {
	return v.ticker.C()
}

// SlotDeadline is the start time of the next slot.
func (v *validator) SlotDeadline(slot primitives.Slot) time.Time {
	return v.genesisTime.Add(params.SlotsDuration(slot+1, params.BeaconConfig()))
}

// RolesAt slot returns the validator roles at the given slot. Returns nil if the
// validator is known to not have a roles at the slot. Returns UNKNOWN if the
// validator assignments are unknown. Otherwise, returns a valid ValidatorRole map.
func (v *validator) RolesAt(ctx context.Context, slot primitives.Slot) (map[[fieldparams.BLSPubkeyLength]byte][]validatorRole, error) {
	ctx, span := trace.StartSpan(ctx, "validator.RolesAt")
	defer span.End()

	snap := v.duties.snapshot()
	if !snap.isInitialized() {
		return nil, errors.New("validator duties are not initialized")
	}

	var (
		rolesAt              = make(map[[fieldparams.BLSPubkeyLength]byte][]validatorRole)
		syncCommitteePubkeys [][fieldparams.BLSPubkeyLength]byte
	)

	for pk, duty := range snap.currentDuties() {
		var roles []validatorRole

		if duty == nil {
			continue
		}
		// The store may predate a reload; quarantined keys get no roles at all.
		if v.isDoppelGangerPending(pk) {
			continue
		}
		if len(duty.ProposerSlots) > 0 {
			for _, proposerSlot := range duty.ProposerSlots {
				if proposerSlot != 0 && proposerSlot == slot {
					roles = append(roles, roleProposer)
					break
				}
			}
		}

		if duty.AttesterSlot == slot {
			roles = append(roles, roleAttester)

			aggregator, err := v.isAggregator(ctx, duty.CommitteeLength, slot, pk)
			if err != nil {
				aggregator = false
				log.WithError(err).Errorf("Could not check if validator %#x is an aggregator", bytesutil.Trunc(duty.PublicKey))
			}
			if aggregator {
				roles = append(roles, roleAggregator)
			}
		}

		// Being assigned to a sync committee for a given slot means that the validator produces and
		// broadcasts signatures for `slot - 1` for inclusion in `slot`. At the last slot of the epoch,
		// the validator checks whether it's in the sync committee of following epoch.
		inSyncCommittee := false
		if slots.IsEpochEnd(slot) {
			if snap.isNextSyncCommittee(duty.ValidatorIndex) {
				roles = append(roles, roleSyncCommittee)
				inSyncCommittee = true
			}
		} else {
			if duty.IsSyncCommittee {
				roles = append(roles, roleSyncCommittee)
				inSyncCommittee = true
			}
		}

		if inSyncCommittee {
			syncCommitteePubkeys = append(syncCommitteePubkeys, pk)
		}

		if slices.Contains(snap.ptcSlots(duty.ValidatorIndex), slot) {
			roles = append(roles, rolePTCMember)
		}

		if len(roles) == 0 {
			roles = append(roles, roleUnknown)
		}

		rolesAt[pk] = roles
	}

	aggPubkeys, err := v.aggSelector.SyncCommitteeAggregators(ctx, slot, syncCommitteePubkeys)
	if err != nil {
		log.WithError(err).Error("Could not check if any validator is a sync committee aggregator")
		return rolesAt, nil
	}
	for _, pk := range aggPubkeys {
		rolesAt[pk] = append(rolesAt[pk], roleSyncCommitteeAggregator)
	}

	return rolesAt, nil
}

// Keymanager returns the underlying validator's keymanager.
func (v *validator) Keymanager() (keymanager.IKeymanager, error) {
	if v.km == nil {
		return nil, errors.New("keymanager is not initialized")
	}
	return v.km, nil
}

// isAggregator checks if a validator is an aggregator of a given slot and committee,
// it uses a modulo calculated by validator count in committee and samples randomness around it.
func (v *validator) isAggregator(
	ctx context.Context,
	committeeLength uint64,
	slot primitives.Slot,
	pubKey [fieldparams.BLSPubkeyLength]byte,
) (bool, error) {
	ctx, span := trace.StartSpan(ctx, "validator.isAggregator")
	defer span.End()

	modulo := uint64(1)
	if committeeLength/params.BeaconConfig().TargetAggregatorsPerCommittee > 1 {
		modulo = committeeLength / params.BeaconConfig().TargetAggregatorsPerCommittee
	}

	slotSig, err := v.aggSelector.AttestationSelectionProof(ctx, slot, pubKey)
	if errors.Is(err, errSelectionProofNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	b := hash.Hash(slotSig)

	return binary.LittleEndian.Uint64(b[:8])%modulo == 0, nil
}

// UpdateDomainDataCaches by making calls for all of the possible domain data. These can change when
// the fork version changes which can happen once per epoch. Although changing for the fork version
// is very rare, a validator should check these data every epoch to be sure the validator is
// participating on the correct fork version.
func (v *validator) UpdateDomainDataCaches(ctx context.Context, slot primitives.Slot) {
	ctx, span := trace.StartSpan(ctx, "validator.UpdateDomainDataCaches")
	defer span.End()

	for _, d := range [][]byte{
		params.BeaconConfig().DomainRandao[:],
		params.BeaconConfig().DomainBeaconAttester[:],
		params.BeaconConfig().DomainBeaconProposer[:],
		params.BeaconConfig().DomainSelectionProof[:],
		params.BeaconConfig().DomainAggregateAndProof[:],
		params.BeaconConfig().DomainSyncCommittee[:],
		params.BeaconConfig().DomainSyncCommitteeSelectionProof[:],
		params.BeaconConfig().DomainContributionAndProof[:],
		params.BeaconConfig().DomainProposerPreferences[:],
	} {
		_, err := v.domainData(ctx, slots.ToEpoch(slot), d)
		if err != nil {
			log.WithError(err).Errorf("Failed to update domain data for domain %v", d)
		}
	}
}

func (v *validator) domainData(ctx context.Context, epoch primitives.Epoch, domain []byte) (*ethpb.DomainResponse, error) {
	ctx, span := trace.StartSpan(ctx, "validator.domainData")
	defer span.End()

	v.domainDataLock.RLock()

	req := &ethpb.DomainRequest{
		Epoch:  epoch,
		Domain: domain,
	}

	key := strings.Join([]string{strconv.FormatUint(uint64(req.Epoch), 10), hex.EncodeToString(req.Domain)}, ",")

	if val, ok := v.domainDataCache.Get(key); ok {
		v.domainDataLock.RUnlock()
		return proto.Clone(val).(*ethpb.DomainResponse), nil
	}
	v.domainDataLock.RUnlock()

	// Lock as we are about to perform an expensive request to the beacon node.
	v.domainDataLock.Lock()
	defer v.domainDataLock.Unlock()

	// We check the cache again as in the event there are multiple inflight requests for
	// the same domain data, the cache might have been filled while we were waiting
	// to acquire the lock.
	if val, ok := v.domainDataCache.Get(key); ok {
		return proto.Clone(val).(*ethpb.DomainResponse), nil
	}

	res, err := v.validatorClient.DomainData(ctx, req)
	if err != nil {
		return nil, err
	}
	v.domainDataCache.Set(key, proto.Clone(res), 1)

	return res, nil
}

// getAttestationData fetches attestation data from the beacon node with caching for Electra.
// During Electra (pre-Gloas), attestation data is identical for all validators in the same slot
// (committee index is always 0), so we cache it to avoid redundant beacon node requests.
func (v *validator) getAttestationData(ctx context.Context, slot primitives.Slot, committeeIndex primitives.CommitteeIndex) (*ethpb.AttestationData, error) {
	ctx, span := trace.StartSpan(ctx, "validator.getAttestationData")
	defer span.End()

	epoch := slots.ToEpoch(slot)
	postElectra := epoch >= params.BeaconConfig().ElectraForkEpoch

	ctx, err := v.withHeadHint(ctx, slot, attestationDueComponent(slot))
	if err != nil {
		return nil, fmt.Errorf("attach freshness hint: %w", err)
	}

	// Pre-Electra: committee index varies per validator.
	// Post-Gloas: index signals payload status.
	if !postElectra {
		return v.validatorClient.AttestationData(ctx, &ethpb.AttestationDataRequest{
			Slot:           slot,
			CommitteeIndex: committeeIndex,
		})
	}

	// Post Electra: committee index is always 0 or consistent payload status, safe to cache
	v.cachedAttestationDataLock.RLock()
	if v.cachedAttestationData != nil && v.cachedAttestationData.Slot == slot {
		data := v.cachedAttestationData
		v.cachedAttestationDataLock.RUnlock()
		return data, nil
	}
	v.cachedAttestationDataLock.RUnlock()

	// Cache miss - acquire write lock and fetch
	v.cachedAttestationDataLock.Lock()
	defer v.cachedAttestationDataLock.Unlock()

	// Double-check after acquiring write lock (another goroutine may have filled the cache)
	if v.cachedAttestationData != nil && v.cachedAttestationData.Slot == slot {
		return v.cachedAttestationData, nil
	}

	data, err := v.validatorClient.AttestationData(ctx, &ethpb.AttestationDataRequest{
		Slot:           slot,
		CommitteeIndex: 0,
	})
	if err != nil {
		return nil, err
	}

	v.cachedAttestationData = data

	return data, nil
}

// ProposerSettings gets the current proposer settings saved in memory validator
func (v *validator) ProposerSettings() *proposer.Settings {
	return v.proposerSettings
}

// SetProposerSettings sets and saves the passed in proposer settings overriding the in memory one
func (v *validator) SetProposerSettings(ctx context.Context, settings *proposer.Settings) error {
	v.proposerSettingsMu.Lock()
	defer v.proposerSettingsMu.Unlock()
	return v.setProposerSettingsLocked(ctx, settings)
}

func (v *validator) setProposerSettingsLocked(ctx context.Context, settings *proposer.Settings) error {
	ctx, span := trace.StartSpan(ctx, "validator.SetProposerSettings")
	defer span.End()

	if v.db == nil {
		return errors.New("db is not set")
	}
	if err := v.db.SaveProposerSettings(ctx, settings); err != nil {
		return err
	}
	v.proposerSettings = settings
	return nil
}

// UpdateProposerSettings atomically mutates the proposer settings: mutate gets a
// deep copy (nil when unset) and returns what to persist, or nil for a no-op.
func (v *validator) UpdateProposerSettings(ctx context.Context, mutate func(*proposer.Settings) (*proposer.Settings, error)) error {
	v.proposerSettingsMu.Lock()
	defer v.proposerSettingsMu.Unlock()
	next, err := mutate(v.proposerSettings.Clone())
	if err != nil {
		return err
	}
	if next == nil {
		return nil
	}
	return v.setProposerSettingsLocked(ctx, next)
}

// PushProposerSettings calls the prepareBeaconProposer RPC to set the fee recipient and also the register validator API if using a custom builder.
func (v *validator) PushProposerSettings(ctx context.Context, slot primitives.Slot, forceFullPush bool) error {
	ctx, span := trace.StartSpan(ctx, "validator.PushProposerSettings")
	defer span.End()

	km, err := v.Keymanager()
	if err != nil {
		return err
	}

	pubkeys, err := km.FetchValidatingPublicKeys(ctx)
	if err != nil {
		return err
	}
	if len(pubkeys) == 0 {
		log.Info("No imported public keys. Skipping prepare proposer routine")
		return nil
	}
	filteredKeys, err := v.filterAndCacheActiveKeys(ctx, pubkeys, slot)
	if err != nil {
		return err
	}

	currentEpoch := slots.ToEpoch(slot)
	isPreGloas := currentEpoch < params.BeaconConfig().GloasForkEpoch

	// A fallback host switch never restarts the runner (the VC stays "healthy"),
	// so force a re-push here; each push kind retries until its own succeeds.
	// Registrations are only pushed pre-Gloas, so post-Gloas their kind is never
	// confirmed and would otherwise report a change every slot.
	connGen := v.connGeneration()
	prefsChanged := v.connTracker.changed(proposerPrefsPush, connGen)
	regsChanged := isPreGloas && v.connTracker.changed(registrationsPush, connGen)
	if prefsChanged {
		log.WithField("connGeneration", connGen).Debug("Forcing proposer preferences re-push after beacon connection change")
	}
	if regsChanged {
		log.WithField("connGeneration", connGen).Debug("Forcing validator registrations re-push after beacon connection change")
	}
	prefsForcePush := forceFullPush || prefsChanged
	regsForcePush := forceFullPush || regsChanged

	// Pre-Gloas, PrepareBeaconProposer carries the per-validator fee recipient.
	// Post-Gloas, SignedProposerPreferences (submitted below) is canonical.
	if isPreGloas {
		proposerReqs := v.buildProposerSettingsRequests(filteredKeys)
		if len(proposerReqs) == 0 {
			log.Warnf("Could not locate valid validator indices. Skipping prepare proposer routine")
			return nil
		}

		if _, err := v.validatorClient.PrepareBeaconProposer(ctx, &ethpb.PrepareBeaconProposerRequest{
			Recipients: proposerReqs,
		}); err != nil {
			return err
		}
		// Gloas preferences start being prepared one epoch ahead; v1 builder
		// settings do not apply to them, so warn while there is time to act.
		if ps := v.ProposerSettings(); currentEpoch+1 >= params.BeaconConfig().GloasForkEpoch &&
			ps.HasLegacyBuilderContent() && slots.IsEpochStart(slot) {
			log.Warn("Proposer settings contain v1 builder fields, which do not apply to gloas; proposer preferences are prepared with defaults. Configure builders via v2 proposer settings.")
		}
	} else {
		v.upgradeProposerSettingsToV2(ctx)
	}

	// prefsForcePush is set when a new runner starts (initial connect or after a
	// beacon-node disconnect/reconnect), so re-push all proposer preferences to
	// repopulate a beacon node that has no preference state.
	prefs := v.buildProposerPreferences(ctx, km, slot, prefsForcePush)
	if len(prefs) > 0 {
		// Delay to mid-slot so the block for this slot is processed first.
		delay := params.BeaconConfig().SlotDuration() / 2
		time.AfterFunc(delay, func() {
			// Detached from the slot context, which may expire before the delay elapses.
			subCtx, cancel := context.WithTimeout(context.Background(), delay)
			defer cancel()
			if _, err := v.validatorClient.SubmitSignedProposerPreferences(subCtx, &ethpb.SubmitSignedProposerPreferencesRequest{
				SignedProposerPreferences: prefs,
			}); err != nil {
				log.WithError(err).Warn("Failed to submit proposer preferences")
				v.releasePrefSlots(prefs)
				return
			}
			log.WithField("count", len(prefs)).Debug("Submitted proposer preferences")
			v.connTracker.confirm(proposerPrefsPush, connGen)
		})
	} else {
		// Nothing was reserved in the dedup cache, so there is no suppressed
		// batch to retry; safe to consume the switch signal.
		v.connTracker.confirm(proposerPrefsPush, connGen)
	}

	if reqs := v.warmBuilderRequestAuths(ctx, km, slot); len(reqs) > 0 {
		delay := params.BeaconConfig().SlotDuration() / 2
		time.AfterFunc(delay, func() {
			// Detached from the slot context, which may expire before the delay elapses.
			subCtx, cancel := context.WithTimeout(context.Background(), delay)
			defer cancel()
			v.submitBuilderPreferenceRequests(subCtx, reqs)
		})
	}

	// TODO: figure out what to do post gloas for builder apis
	if !isPreGloas {
		return nil
	}

	signedRegReqs := v.buildSignedRegReqs(ctx, filteredKeys, km.Sign, slot, regsForcePush)
	if len(signedRegReqs) > 0 {
		go func() {
			if err := SubmitValidatorRegistrations(ctx, v.validatorClient, signedRegReqs, v.validatorsRegBatchSize); err != nil {
				log.WithError(errors.Wrap(ErrBuilderValidatorRegistration, err.Error())).Warn("Failed to register validator on builder")
				return
			}
			v.connTracker.confirm(registrationsPush, connGen)
		}()
	} else {
		// A forced build includes every builder-enabled key, so an empty build
		// means the new host is not missing any registration.
		v.connTracker.confirm(registrationsPush, connGen)
	}

	return nil
}

// EnsureEventStream reconciles the event stream with the current beacon host:
// it starts the stream in a new goroutine when it is not running, replaces it
// when the connection switched hosts since it was bound, and is a synchronous
// no-op otherwise. Called every slot tick.
func (v *validator) EnsureEventStream(ctx context.Context, topics []string) {
	gen := v.connGeneration()
	running := v.EventStreamIsRunning()
	if running && !v.connTracker.changed(eventStreamBind, gen) {
		return
	}
	reason := "stream not running"
	if running {
		reason = "beacon host switched"
	}
	v.connTracker.confirm(eventStreamBind, gen)
	log.WithFields(logrus.Fields{
		"topics": topics,
		"reason": reason,
	}).Info("Starting event stream")
	go v.validatorClient.StartEventStream(ctx, topics, v.eventsChannel)
}

func (v *validator) ProcessEvent(ctx context.Context, event *eventClient.Event) {
	if event == nil || event.Data == nil {
		log.Warn("Received empty event")
		return
	}

	switch event.Type {
	case eventClient.EventError:
		log.Error(string(event.Data))
	case eventClient.EventConnectionError:
		log.WithError(errors.New(string(event.Data))).Error("Event stream interrupted")
	case eventClient.EventHead:
		head := &structs.HeadEvent{}
		if err := json.Unmarshal(event.Data, head); err != nil {
			log.WithError(err).Error("Failed to unmarshal head Event into JSON")
		}

		uintSlot, err := strconv.ParseUint(head.Slot, 10, 64)
		if err != nil {
			log.WithError(err).Error("Failed to parse slot")
			return
		}

		slot := primitives.Slot(uintSlot)

		sinceSlotStartTime, err := v.sinceSlotStartTime(slot)
		if err != nil {
			log.WithError(err).WithField("slot", slot).Error("Failed to compute time since slot start")
		}

		fields := logrus.Fields{
			"slot":                      head.Slot,
			"sinceSlotStartTime":        sinceSlotStartTime,
			"previousDutyDependentRoot": trim(head.PreviousDutyDependentRoot),
			"currentDutyDependentRoot":  trim(head.CurrentDutyDependentRoot),
			"version":                   "1",
		}

		if head.Block != "" {
			fields["blockRoot"] = trim(head.Block)
		}

		log.WithFields(fields).Debug("Received head event")

		v.setHighestSlot(slot)

		// Update the head tracker. The v1 event announces no payload status.
		if err := v.head.update(slot, head.Block, api.PayloadStatusUnknown); err != nil {
			log.WithError(err).Error("Failed to record head event block root")
		}

		if !v.disableDutiesPolling {
			if err := v.checkDependentRoots(ctx, head.PreviousDutyDependentRoot, head.CurrentDutyDependentRoot); err != nil {
				log.WithError(err).Error("Failed to check dependent roots")
			}
		}
	case eventClient.EventHeadV2:
		head := &structs.HeadEventV2{}
		if err := json.Unmarshal(event.Data, head); err != nil {
			log.WithError(err).Error("Failed to unmarshal head_v2 event into JSON")
			return
		}
		if head.Data == nil {
			log.Error("Received head_v2 event with no data")
			return
		}

		uintSlot, err := strconv.ParseUint(head.Data.Slot, 10, 64)
		if err != nil {
			log.WithError(err).Error("Failed to parse slot")
			return
		}
		slot := primitives.Slot(uintSlot)

		sinceSlotStartTime, err := v.sinceSlotStartTime(slot)
		if err != nil {
			log.WithError(err).WithField("slot", slot).Error("Failed to compute time since slot start")
		}

		log.WithFields(logrus.Fields{
			"slot":                      head.Data.Slot,
			"sinceSlotStartTime":        sinceSlotStartTime,
			"blockRoot":                 trim(head.Data.Block),
			"payloadStatus":             head.Data.PayloadStatus,
			"currentEpochDependentRoot": trim(head.Data.CurrentEpochDependentRoot),
			"nextEpochDependentRoot":    trim(head.Data.NextEpochDependentRoot),
			"version":                   "2",
		}).Debug("Received head event")

		v.setHighestSlot(slot)

		// Update the head tracker
		if err := v.head.update(slot, head.Data.Block, api.PayloadStatus(head.Data.PayloadStatus)); err != nil {
			log.WithError(err).Error("Failed to record head event block root")
		}

		if !v.disableDutiesPolling {
			if err := v.checkDependentRoots(ctx, head.Data.CurrentEpochDependentRoot, head.Data.NextEpochDependentRoot); err != nil {
				log.WithError(err).Error("Failed to check dependent roots")
			}
		}
	case eventClient.EventExecutionPayloadAvailable:
		payloadEvent := &structs.ExecutionPayloadAvailableEvent{}
		if err := json.Unmarshal(event.Data, payloadEvent); err != nil {
			log.WithError(err).Error("Failed to unmarshal execution payload event into JSON")
			return
		}
		uintSlot, err := strconv.ParseUint(payloadEvent.Slot, 10, 64)
		if err != nil {
			log.WithError(err).Error("Failed to parse execution payload event slot")
			return
		}

		root, err := decodePayloadBlockRoot(payloadEvent.BlockRoot)
		if err != nil {
			log.WithError(err).Error("Failed to decode execution payload event block root")
		}

		v.payloadAvailability.notify(primitives.Slot(uintSlot), root)
	default:
		// just keep going and log the error
		log.WithField("type", event.Type).WithField("data", string(event.Data)).Warn("Received an unknown event")
	}
}

func (v *validator) EventStreamIsRunning() bool {
	return v.validatorClient.EventStreamIsRunning()
}

// trim shortens a string (e.g. a hex-encoded root like "0x9927a089f167...") to
// its first 14 characters.
func trim(s string) string {
	const maxLen = 14 // "0x" + 12 hex characters (6 bytes).
	if len(s) <= maxLen {
		return s
	}

	return s[:maxLen]
}

func (v *validator) Host() string {
	return v.validatorClient.Host()
}

func (v *validator) EnsureReady(ctx context.Context) bool {
	return v.validatorClient.EnsureReady(ctx)
}

func (v *validator) filterAndCacheActiveKeys(ctx context.Context, pubkeys [][fieldparams.BLSPubkeyLength]byte, slot primitives.Slot) ([][fieldparams.BLSPubkeyLength]byte, error) {
	ctx, span := trace.StartSpan(ctx, "validator.filterAndCacheActiveKeys")
	defer span.End()
	isEpochStart := slots.IsEpochStart(slot)
	filteredKeys := make([][fieldparams.BLSPubkeyLength]byte, 0)
	if len(pubkeys) == 0 {
		return filteredKeys, nil
	}
	var err error
	// repopulate the statuses if epoch start or if a new key is added missing the cache
	if isEpochStart || len(v.statusCache()) != len(pubkeys) /* cache not populated or updated correctly */ {
		if err = v.updateValidatorStatusCache(ctx, pubkeys); err != nil {
			return nil, errors.Wrap(err, "failed to update validator status cache")
		}
	}
	currEpoch := slots.ToEpoch(slot)
	for k, s := range v.statusCache() {
		if isActiveForDuties(s.status, currEpoch) {
			filteredKeys = append(filteredKeys, k)
		} else {
			log.WithFields(logrus.Fields{
				"pubkey": hexutil.Encode(s.publicKey),
				"status": s.status.Status.String(),
			}).Debugf("Skipping non-active status key.")
		}
	}

	return filteredKeys, nil
}

// updateValidatorStatusCache updates the validator statuses cache, a map of keys currently used by the validator client
func (v *validator) updateValidatorStatusCache(ctx context.Context, pubkeys [][fieldparams.BLSPubkeyLength]byte) error {
	if len(pubkeys) == 0 {
		v.pubkeyToStatusLock.Lock()
		v.pubkeyToStatus = make(map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus, 0)
		v.pubkeyToStatusLock.Unlock()
		return nil
	}
	statusRequestKeys := make([][]byte, 0)
	for _, k := range pubkeys {
		statusRequestKeys = append(statusRequestKeys, k[:])
	}
	resp, err := v.validatorClient.MultipleValidatorStatus(ctx, &ethpb.MultipleValidatorStatusRequest{
		PublicKeys: statusRequestKeys,
	})
	if err != nil {
		return err
	}
	if resp == nil {
		return errors.New("response is nil")
	}
	if len(resp.Statuses) != len(resp.PublicKeys) {
		return fmt.Errorf("expected %d pubkeys in status, received %d", len(resp.Statuses), len(resp.PublicKeys))
	}
	if len(resp.Statuses) != len(resp.Indices) {
		return fmt.Errorf("expected %d indices in status, received %d", len(resp.Statuses), len(resp.Indices))
	}

	pubkeyToStatus := make(map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus, len(resp.Statuses))
	for i, s := range resp.Statuses {
		pubkeyToStatus[bytesutil.ToBytes48(resp.PublicKeys[i])] = &validatorStatus{
			publicKey: resp.PublicKeys[i],
			status:    s,
			index:     resp.Indices[i],
		}
	}
	v.pubkeyToStatusLock.Lock()
	v.pubkeyToStatus = pubkeyToStatus
	v.pubkeyToStatusLock.Unlock()

	return nil
}

// buildProposerSettingsRequests builds both PrepareBeaconProposer requests and,
// post-Gloas, signed proposer preferences from the same validator settings.
func (v *validator) buildProposerSettingsRequests(
	activePubkeys [][fieldparams.BLSPubkeyLength]byte,
) []*ethpb.PrepareBeaconProposerRequest_FeeRecipientContainer {
	var prepareProposerReqs []*ethpb.PrepareBeaconProposerRequest_FeeRecipientContainer
	ps := v.ProposerSettings()
	statuses := v.statusCache()
	for _, k := range activePubkeys {
		s, ok := statuses[k]
		if !ok {
			continue
		}

		feeRecipient := common.HexToAddress(params.BeaconConfig().EthBurnAddressHex)
		if ps != nil && ps.DefaultConfig != nil && ps.DefaultConfig.FeeRecipientConfig != nil {
			feeRecipient = ps.DefaultConfig.FeeRecipientConfig.FeeRecipient
		}
		if ps != nil && ps.ProposeConfig != nil {
			if config, ok := ps.ProposeConfig[k]; ok && config != nil && config.FeeRecipientConfig != nil {
				feeRecipient = config.FeeRecipientConfig.FeeRecipient
			}
		}

		prepareProposerReqs = append(prepareProposerReqs, &ethpb.PrepareBeaconProposerRequest_FeeRecipientContainer{
			ValidatorIndex: s.index,
			FeeRecipient:   feeRecipient[:],
		})
	}
	return prepareProposerReqs
}

// upgradeProposerSettingsToV2 is idempotent post-fork cleanup: it scrubs dead v1
// builder content transactionally; callers gate on gloas-active.
func (v *validator) upgradeProposerSettingsToV2(ctx context.Context) {
	err := v.UpdateProposerSettings(ctx, func(ps *proposer.Settings) (*proposer.Settings, error) {
		if !ps.UpgradeToV2() {
			return nil, nil
		}
		return ps, nil
	})
	if err != nil {
		log.WithError(err).Warn("Failed to persist v1->v2 proposer settings upgrade")
	}
}

// buildProposerPreferences creates signed proposer preferences for validators
// that have proposer slots in the current epoch (future slots) or next epoch. During normal operation it is
// gated to run once at mid-epoch; pass force=true to clear the submitted-slot
// dedup cache and bypass that gate (e.g. after a reorg triggers a duty change,
// or when a new runner starts after a beacon-node disconnect/reconnect).
//
// Current-epoch preferences are submitted after the first slot of the epoch
// (slot 0 is skipped to avoid stale state after epoch transition). If the
// validator client starts mid-epoch, preferences are submitted for all
// remaining future slots in the epoch.
// Next-epoch preferences are submitted at or after mid-epoch to ensure beacon
// nodes have processed the epoch transition.
// Already-submitted slots are tracked to avoid duplicate signing and RPC calls.
func (v *validator) buildProposerPreferences(
	ctx context.Context,
	km keymanager.IKeymanager,
	slot primitives.Slot,
	force bool,
) []*ethpb.SignedProposerPreferences {
	currentEpoch := slots.ToEpoch(slot)
	gloasEpoch := params.BeaconConfig().GloasForkEpoch
	if currentEpoch+1 < gloasEpoch {
		return nil
	}
	epochStart, err := slots.EpochStart(currentEpoch)
	if err != nil {
		return nil
	}
	midEpoch := epochStart + params.BeaconConfig().SlotsPerEpoch/2

	v.submittedPrefSlotsLock.Lock()
	if force {
		v.submittedPrefSlots = make(map[primitives.Slot]bool)
	} else {
		for s := range v.submittedPrefSlots {
			if s < epochStart {
				delete(v.submittedPrefSlots, s)
			}
		}
	}
	v.submittedPrefSlotsLock.Unlock()

	snap := v.duties.snapshot()
	if !snap.isInitialized() {
		return nil
	}

	var signedPrefs []*ethpb.SignedProposerPreferences

	// Per Gloas spec, dependent_root for a proposal in epoch E is the duty
	// dependent root the beacon node uses to compute proposer duties for E:
	//   - proposal in current epoch  → previous_duty_dependent_root
	//   - proposal in next epoch     → current_duty_dependent_root
	prevDepRoot, currDepRoot := v.duties.dependentRoots()

	currentDuties := snap.currentDuties()
	nextDuties := snap.nextDuties()

	var currentProposerCount, nextProposerCount int
	for _, d := range currentDuties {
		currentProposerCount += len(d.ProposerSlots)
	}
	for _, d := range nextDuties {
		nextProposerCount += len(d.ProposerSlots)
	}

	// Current-epoch: submit after first slot of epoch to avoid stale state.
	// force bypasses the timing gate for reorg resubmission.
	if currentEpoch >= gloasEpoch && (force || slot > epochStart) {
		signedPrefs = append(signedPrefs, v.processProposerDuties(ctx, km, currentDuties, slot, prevDepRoot, false)...)
	}

	// Next-epoch: submit at or after mid-epoch. The gate is not bypassed
	// by force because the beacon node may not have the next-epoch state ready.
	if slot >= midEpoch {
		signedPrefs = append(signedPrefs, v.processProposerDuties(ctx, km, nextDuties, slot, currDepRoot, true)...)
	}

	log.WithFields(logrus.Fields{
		"slot":                 slot,
		"epoch":                currentEpoch,
		"epochStart":           epochStart,
		"midEpoch":             midEpoch,
		"currentProposerSlots": currentProposerCount,
		"nextProposerSlots":    nextProposerCount,
		"prefsBuilt":           len(signedPrefs),
		"alreadySubmitted":     v.submittedPrefSlotsCount(),
	}).Debug("Build proposer preferences result")
	return signedPrefs
}

// processProposerDuties signs proposer preferences for the given duties and
// records the slots submitted, returning the signed preferences. Signing
// failures are aggregated into a single warning carrying the first error.
func (v *validator) processProposerDuties(
	ctx context.Context,
	km keymanager.IKeymanager,
	duties iter.Seq2[pubkey, *ethpb.ValidatorDuty],
	slot primitives.Slot,
	dependentRoot []byte,
	isNextEpoch bool,
) []*ethpb.SignedProposerPreferences {
	if len(dependentRoot) != fieldparams.RootLength {
		return nil
	}

	var signedPrefs []*ethpb.SignedProposerPreferences
	var sigFailCount int
	var firstErr error
	for pk, duty := range duties {
		if len(duty.ProposerSlots) == 0 {
			continue
		}
		if duty.Status != ethpb.ValidatorStatus_ACTIVE && duty.Status != ethpb.ValidatorStatus_EXITING {
			continue
		}

		for _, proposalSlot := range duty.ProposerSlots {
			// Skip slots that have passed or are too close. Preferences are
			// submitted at mid-slot, so the proposer needs to be at least 1
			// full slot away for the beacon node to receive them in time.
			if !isNextEpoch && proposalSlot <= slot+1 {
				continue
			}
			if !v.reservePrefSlot(proposalSlot) {
				continue
			}

			// Keyed on the proposal slot's own epoch, the duty store can hold next-epoch duties as current in the last slot of an epoch.
			feeRecipient, gasLimit := v.proposerConfigForKey(pk, slots.ToEpoch(proposalSlot))
			pref := &ethpb.ProposerPreferences{
				DependentRoot:  dependentRoot,
				ProposalSlot:   proposalSlot,
				ValidatorIndex: duty.ValidatorIndex,
				FeeRecipient:   feeRecipient[:],
				TargetGasLimit: gasLimit,
			}
			signedPref, err := v.signProposerPreferences(ctx, km, pk, pref)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				sigFailCount++
				v.releasePrefSlot(proposalSlot)
				continue
			}
			signedPrefs = append(signedPrefs, signedPref)
		}
	}
	if sigFailCount > 0 {
		log.WithError(firstErr).WithField("count", sigFailCount).Warn("Failed to sign proposer preferences")
	}
	return signedPrefs
}

// reservePrefSlot marks proposalSlot as submitted, returning false if another
// pass already claimed it.
func (v *validator) reservePrefSlot(proposalSlot primitives.Slot) bool {
	v.submittedPrefSlotsLock.Lock()
	defer v.submittedPrefSlotsLock.Unlock()
	if v.submittedPrefSlots[proposalSlot] {
		return false
	}
	v.submittedPrefSlots[proposalSlot] = true
	return true
}

func (v *validator) releasePrefSlot(proposalSlot primitives.Slot) {
	v.submittedPrefSlotsLock.Lock()
	defer v.submittedPrefSlotsLock.Unlock()
	delete(v.submittedPrefSlots, proposalSlot)
}

// releasePrefSlots un-reserves the slots of a batch whose submission failed so
// a later build retries them.
func (v *validator) releasePrefSlots(prefs []*ethpb.SignedProposerPreferences) {
	slots := make([]primitives.Slot, len(prefs))
	v.submittedPrefSlotsLock.Lock()
	for i, p := range prefs {
		delete(v.submittedPrefSlots, p.Message.ProposalSlot)
		slots[i] = p.Message.ProposalSlot
	}
	v.submittedPrefSlotsLock.Unlock()
	log.WithField("proposalSlots", slots).Debug("Released proposer preference reservations for retry")
}

func (v *validator) proposerConfigForKey(pk pubkey, epoch primitives.Epoch) (common.Address, uint64) {
	feeRecipient := common.HexToAddress(params.BeaconConfig().EthBurnAddressHex)
	ps := v.ProposerSettings()
	gasLimit := uint64(ps.TargetGasLimit(pk, epoch))
	if ps == nil {
		return feeRecipient, gasLimit
	}
	if ps.DefaultConfig != nil && ps.DefaultConfig.FeeRecipientConfig != nil {
		feeRecipient = ps.DefaultConfig.FeeRecipientConfig.FeeRecipient
	}
	if ps.ProposeConfig != nil {
		if config, ok := ps.ProposeConfig[pk]; ok && config != nil && config.FeeRecipientConfig != nil {
			feeRecipient = config.FeeRecipientConfig.FeeRecipient
		}
	}
	return feeRecipient, gasLimit
}

func (v *validator) submittedPrefSlotsCount() int {
	v.submittedPrefSlotsLock.RLock()
	defer v.submittedPrefSlotsLock.RUnlock()
	return len(v.submittedPrefSlots)
}

// builderTarget is one resolved builder for a proposer: a URL plus the
// preferences to submit for it, with per-entry overrides already applied.
type builderTarget struct {
	url         string
	authData    []byte
	pubkeys     [][]byte
	maxPayment  uint64
	minBid      *uint64
	boostFactor *uint64
}

// builderTargetsForKey resolves the configured builder list for pk; entries override
// config-level fallbacks. TODO(gloas): per-entry max_execution_payment, minBid,
// boost and pubkeys ride the beacon-APIs #630 wire.
func (v *validator) builderTargetsForKey(pk pubkey) []builderTarget {
	ps := v.ProposerSettings()
	if ps == nil {
		return nil
	}
	bc := ps.EffectiveBuilderConfig(pk)
	if bc == nil {
		return nil
	}
	fbMax := uint64(bc.EffectiveMaxExecutionPayment())
	fbMin := uint64Ptr(bc.MinBid)
	fbBoost := uint64Ptr(bc.BuilderBoostFactor)

	targets := make([]builderTarget, 0, len(bc.Builders))
	seen := make(map[proposer.EntryIdentity]bool, len(bc.Builders))
	for _, e := range bc.Builders {
		if e == nil || e.URL == "" {
			continue
		}
		// One request per entry: entries may share a url with different auth_data.
		if seen[e.Identity()] {
			continue
		}
		seen[e.Identity()] = true
		t := builderTarget{url: e.URL, authData: e.EffectiveAuthData(), pubkeys: e.Pubkeys, maxPayment: fbMax, minBid: fbMin, boostFactor: fbBoost}
		if e.MaxExecutionPayment != nil {
			t.maxPayment = uint64(*e.MaxExecutionPayment)
		}
		if e.MinBid != nil {
			t.minBid = uint64Ptr(e.MinBid)
		}
		if e.BuilderBoostFactor != nil {
			t.boostFactor = uint64Ptr(e.BuilderBoostFactor)
		}
		targets = append(targets, t)
	}
	return targets
}

func uint64Ptr(v *validatortypes.Uint64) *uint64 {
	if v == nil {
		return nil
	}
	u := uint64(*v)
	return &u
}

// warmBuilderRequestAuths pre-signs request auths for upcoming proposal slots and
// returns the ahead-of-time preference submissions, rebuilt every push.
func (v *validator) warmBuilderRequestAuths(ctx context.Context, km keymanager.IKeymanager, slot primitives.Slot) []*ethpb.SubmitBuilderPreferencesRequest {
	if slots.ToEpoch(slot)+1 < params.BeaconConfig().GloasForkEpoch {
		return nil
	}
	snap := v.duties.snapshot()
	if !snap.isInitialized() {
		return nil
	}
	v.pruneSignedRequestAuths(slot)
	reqs := v.warmBuilderRequestAuthsForDuties(ctx, km, slot, snap.currentDuties())
	return append(reqs, v.warmBuilderRequestAuthsForDuties(ctx, km, slot, snap.nextDuties())...)
}

func (v *validator) warmBuilderRequestAuthsForDuties(ctx context.Context, km keymanager.IKeymanager, slot primitives.Slot, duties iter.Seq2[pubkey, *ethpb.ValidatorDuty]) []*ethpb.SubmitBuilderPreferencesRequest {
	var reqs []*ethpb.SubmitBuilderPreferencesRequest
	for pk, duty := range duties {
		targets := v.builderTargetsForKey(pk)
		if len(targets) == 0 {
			continue
		}
		// The v1 wire holds one max_execution_payment per validator (no builder
		// identity), so the lowest configured value is submitted for every builder.
		minPayment := targets[0].maxPayment
		urls := make(map[string]bool, len(targets))
		for _, t := range targets {
			if t.maxPayment < minPayment {
				minPayment = t.maxPayment
			}
			urls[t.url] = true
		}
		for _, proposalSlot := range duty.ProposerSlots {
			if proposalSlot <= slot {
				continue
			}
			for url := range urls {
				signed, err := v.signRequestAuthCached(ctx, km, pk, url, proposalSlot)
				if err != nil {
					log.WithError(err).Warn("Failed to sign builder request auth")
					continue
				}
				// TODO(gloas): per-entry max_execution_payment, authData, minBid, boost
				// and pubkeys need the beacon-APIs #630 inline wire's builder identity.
				reqs = append(reqs, &ethpb.SubmitBuilderPreferencesRequest{
					ValidatorPubkey: pk[:],
					Request: &ethpb.BuilderPreferencesRequestV1{
						Preferences: &ethpb.BuilderPreferencesV1{MaxExecutionPayment: primitives.Gwei(minPayment)},
						Auth:        signed,
					},
				})
			}
		}
	}
	return reqs
}

func (v *validator) submitBuilderPreferenceRequests(ctx context.Context, reqs []*ethpb.SubmitBuilderPreferencesRequest) {
	for _, req := range reqs {
		if _, err := v.validatorClient.SubmitBuilderPreferences(ctx, req); err != nil {
			log.WithError(err).Warn("Failed to submit builder preferences")
		}
	}
}

// submitProposerPreferences builds and submits proposer preferences for the
// current slot, bypassing the mid-epoch gate. Called when duties change due to
// a reorg so that the new proposer's preferences reach the network promptly.
func (v *validator) submitProposerPreferences(ctx context.Context) {
	slot := slots.CurrentSlot(v.genesisTime)
	currentEpoch := slots.ToEpoch(slot)
	if currentEpoch+1 < params.BeaconConfig().GloasForkEpoch {
		return
	}
	km, err := v.Keymanager()
	if err != nil {
		log.WithError(err).Warn("Failed to get keymanager for proposer preference resubmission")
		return
	}
	if currentEpoch >= params.BeaconConfig().GloasForkEpoch {
		v.upgradeProposerSettingsToV2(ctx)
	}
	prefs := v.buildProposerPreferences(ctx, km, slot, true)
	if len(prefs) == 0 {
		return
	}
	delay := params.BeaconConfig().SlotDuration() / 2
	go func() {
		time.Sleep(delay)
		if _, err := v.validatorClient.SubmitSignedProposerPreferences(ctx, &ethpb.SubmitSignedProposerPreferencesRequest{
			SignedProposerPreferences: prefs,
		}); err != nil {
			log.WithError(err).Warn("Failed to resubmit proposer preferences after duty change")
			v.releasePrefSlots(prefs)
		} else {
			log.WithField("count", len(prefs)).Info("Resubmitted proposer preferences after duty change")
		}
	}()
}

func (v *validator) buildSignedRegReqs(
	ctx context.Context,
	activePubkeys [][fieldparams.BLSPubkeyLength]byte,
	signer signingFunc,
	slot primitives.Slot,
	forceFullPush bool,
) []*ethpb.SignedValidatorRegistrationV1 {
	ctx, span := trace.StartSpan(ctx, "validator.buildSignedRegReqs")
	defer span.End()

	var signedValRegRequests []*ethpb.SignedValidatorRegistrationV1
	// One snapshot for the whole batch: concurrent keymanager writes swap the
	// settings pointer, and mixing objects would tear version vs content.
	ps := v.ProposerSettings()
	if ps == nil {
		return signedValRegRequests
	}
	// if the timestamp is pre-genesis, don't create registrations
	if time.Now().Before(v.genesisTime) {
		return signedValRegRequests
	}

	if ps.DefaultConfig != nil && ps.DefaultConfig.FeeRecipientConfig == nil && ps.DefaultConfig.BuilderConfig != nil {
		if ps.Version == proposer.SchemaV2 {
			log.Warn("Default builder config has no default fee recipient; only keys with their own fee recipient can register")
		} else {
			log.Warn("Builder is `enabled` in default config but will be ignored because no fee recipient was provided!")
		}
	}

	statuses := v.statusCache()
	for i, k := range activePubkeys {
		// map is populated before this function in buildPrepProposerReq
		_, ok := statuses[k]
		if !ok {
			continue
		}

		feeRecipient, gasLimit, enabled := ps.RegistrationFor(k)
		if !enabled {
			continue
		}

		req := &ethpb.ValidatorRegistrationV1{
			FeeRecipient: feeRecipient[:],
			GasLimit:     uint64(gasLimit),
			Timestamp:    uint64(time.Now().UTC().Unix()),
			Pubkey:       activePubkeys[i][:],
		}

		signedRequest, isCached, err := v.SignValidatorRegistrationRequest(ctx, signer, req)
		if err != nil {
			log.WithFields(logrus.Fields{
				"pubkey":       fmt.Sprintf("%#x", req.Pubkey),
				"feeRecipient": feeRecipient,
			}).Error(err)
			continue
		}

		if hexutil.Encode(feeRecipient.Bytes()) == params.BeaconConfig().EthBurnAddressHex {
			log.WithFields(logrus.Fields{
				"pubkey":       fmt.Sprintf("%#x", req.Pubkey),
				"feeRecipient": feeRecipient,
			}).Warn("Fee recipient is burn address")
		}

		if slots.IsEpochStart(slot) || forceFullPush || !isCached {
			// if epoch start (or forced to) send all validator registrations
			// otherwise if slot is not epoch start then only send new non cached values
			signedValRegRequests = append(signedValRegRequests, signedRequest)
		}
	}
	return signedValRegRequests
}

// This tracks all validators' voting status.
type voteStats struct {
	startEpoch          primitives.Epoch
	totalAttestedCount  uint64
	totalRequestedCount uint64
	totalDistance       primitives.Slot
	totalCorrectSource  uint64
	totalCorrectTarget  uint64
	totalCorrectHead    uint64
}
