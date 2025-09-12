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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/OffchainLabs/prysm/v6/api/client"
	eventClient "github.com/OffchainLabs/prysm/v6/api/client/event"
	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/async/event"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v6/cmd"
	"github.com/OffchainLabs/prysm/v6/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/config/proposer"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/crypto/hash"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	accountsiface "github.com/OffchainLabs/prysm/v6/validator/accounts/iface"
	"github.com/OffchainLabs/prysm/v6/validator/accounts/wallet"
	"github.com/OffchainLabs/prysm/v6/validator/client/iface"
	"github.com/OffchainLabs/prysm/v6/validator/db"
	dbCommon "github.com/OffchainLabs/prysm/v6/validator/db/common"
	"github.com/OffchainLabs/prysm/v6/validator/graffiti"
	"github.com/OffchainLabs/prysm/v6/validator/keymanager"
	"github.com/OffchainLabs/prysm/v6/validator/keymanager/local"
	remoteweb3signer "github.com/OffchainLabs/prysm/v6/validator/keymanager/remote-web3signer"
	"github.com/dgraph-io/ristretto/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/metadata"
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
	duties                             *ethpb.ValidatorDutiesContainer
	ticker                             slots.Ticker
	genesisTime                        time.Time
	highestValidSlot                   primitives.Slot
	slotFeed                           *event.Feed
	startBalances                      map[[fieldparams.BLSPubkeyLength]byte]uint64
	prevEpochBalances                  map[[fieldparams.BLSPubkeyLength]byte]uint64
	blacklistedPubkeys                 map[[fieldparams.BLSPubkeyLength]byte]bool
	pubkeyToStatus                     map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus
	wallet                             *wallet.Wallet
	walletInitializedChan              chan *wallet.Wallet
	walletInitializedFeed              *event.Feed
	graffiti                           []byte
	graffitiStruct                     *graffiti.Graffiti
	graffitiOrderedIndex               uint64
	beaconNodeHosts                    []string
	currentHostIndex                   uint64
	validatorClient                    iface.ValidatorClient
	chainClient                        iface.ChainClient
	nodeClient                         iface.NodeClient
	prysmChainClient                   iface.PrysmChainClient
	db                                 db.Database
	km                                 keymanager.IKeymanager
	web3SignerConfig                   *remoteweb3signer.SetupConfig
	proposerSettings                   *proposer.Settings
	signedValidatorRegistrations       map[[fieldparams.BLSPubkeyLength]byte]*ethpb.SignedValidatorRegistrationV1
	validatorsRegBatchSize             int
	interopKeysConfig                  *local.InteropKeymanagerConfig
	attSelections                      map[attSelectionKey]iface.BeaconCommitteeSelection
	aggregatedSlotCommitteeIDCache     *lru.Cache
	domainDataCache                    *ristretto.Cache[string, proto.Message]
	voteStats                          voteStats
	syncCommitteeStats                 syncCommitteeStats
	submittedAtts                      map[submittedAttKey]*submittedAtt
	submittedAggregates                map[submittedAttKey]*submittedAtt
	logValidatorPerformance            bool
	emitAccountMetrics                 bool
	enableAPI                          bool
	distributed                        bool
	domainDataLock                     sync.RWMutex
	attLogsLock                        sync.Mutex
	aggregatedSlotCommitteeIDCacheLock sync.Mutex
	highestValidSlotLock               sync.Mutex
	prevEpochBalancesLock              sync.RWMutex
	blacklistedPubkeysLock             sync.RWMutex
	attSelectionLock                   sync.Mutex
	dutiesLock                         sync.RWMutex
	disableDutiesPolling               bool
	accountsChangedChannel             chan [][fieldparams.BLSPubkeyLength]byte
	eventsChannel                      chan *eventClient.Event
	accountChangedSub                  event.Subscription
}

type validatorStatus struct {
	publicKey []byte
	status    *ethpb.ValidatorStatusResponse
	index     primitives.ValidatorIndex
}

type attSelectionKey struct {
	slot  primitives.Slot
	index primitives.ValidatorIndex
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

	genesisRoot, err := v.db.GenesisValidatorsRoot(ctx)
	if err != nil {
		return errors.Wrap(err, "unable to retrieve valid genesis validators root while initializing key manager")
	}

	switch {
	case v.wallet != nil:
		if v.web3SignerConfig != nil {
			v.web3SignerConfig.GenesisValidatorsRoot = genesisRoot
		}
		keyManager, err := v.wallet.InitializeKeymanager(ctx, accountsiface.InitKeymanagerConfig{ListenForChanges: true, Web3SignerConfig: v.web3SignerConfig})
		if err != nil {
			return errors.Wrap(err, "could not initialize key manager")
		}
		v.km = keyManager
	case v.interopKeysConfig != nil:
		keyManager, err := local.NewInteropKeymanager(ctx, v.interopKeysConfig.Offset, v.interopKeysConfig.NumValidatorKeys)
		if err != nil {
			return errors.Wrap(err, "could not generate interop keys for key manager")
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
	v.ticker = slots.NewSlotTicker(v.genesisTime, params.BeaconConfig().SecondsPerSlot)
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
	for _, s := range v.pubkeyToStatus {
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
	secs := time.Duration((slot + 1).Mul(params.BeaconConfig().SecondsPerSlot))
	return v.genesisTime.Add(secs * time.Second)
}

// CheckDoppelGanger checks if the current actively provided keys have
// any duplicates active in the network.
func (v *validator) CheckDoppelGanger(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "validator.CheckDoppelganger")
	defer span.End()

	if !features.Get().EnableDoppelGanger {
		return nil
	}
	pubkeys, err := v.km.FetchValidatingPublicKeys(ctx)
	if err != nil {
		return err
	}
	log.WithField("keyCount", len(pubkeys)).Info("Running doppelganger check")
	// Exit early if no validating pub keys are found.
	if len(pubkeys) == 0 {
		return nil
	}
	req := &ethpb.DoppelGangerRequest{ValidatorRequests: []*ethpb.DoppelGangerRequest_ValidatorRequest{}}
	for _, pkey := range pubkeys {
		copiedKey := pkey
		attRec, err := v.db.AttestationHistoryForPubKey(ctx, copiedKey)
		if err != nil {
			return err
		}
		if len(attRec) == 0 {
			// If no history exists we simply send in a zero
			// value for the request epoch and root.
			req.ValidatorRequests = append(req.ValidatorRequests,
				&ethpb.DoppelGangerRequest_ValidatorRequest{
					PublicKey:  copiedKey[:],
					Epoch:      0,
					SignedRoot: make([]byte, fieldparams.RootLength),
				})
			continue
		}
		r := retrieveLatestRecord(attRec)
		if copiedKey != r.PubKey {
			return errors.New("attestation record mismatched public key")
		}
		req.ValidatorRequests = append(req.ValidatorRequests,
			&ethpb.DoppelGangerRequest_ValidatorRequest{
				PublicKey:  r.PubKey[:],
				Epoch:      r.Target,
				SignedRoot: r.SigningRoot,
			})
	}
	resp, err := v.validatorClient.CheckDoppelGanger(ctx, req)
	if err != nil {
		return err
	}
	// If nothing is returned by the beacon node, we return an
	// error as it is unsafe for us to proceed.
	if resp == nil || resp.Responses == nil || len(resp.Responses) == 0 {
		return errors.New("beacon node returned 0 responses for doppelganger check")
	}
	return buildDuplicateError(resp.Responses)
}

func buildDuplicateError(response []*ethpb.DoppelGangerResponse_ValidatorResponse) error {
	duplicates := make([][]byte, 0)
	for _, valRes := range response {
		if valRes.DuplicateExists {
			var copiedKey [fieldparams.BLSPubkeyLength]byte
			copy(copiedKey[:], valRes.PublicKey)
			duplicates = append(duplicates, copiedKey[:])
		}
	}
	if len(duplicates) == 0 {
		return nil
	}
	return errors.Errorf("Duplicate instances exists in the network for validator keys: %#x", duplicates)
}

// Ensures that the latest attestation history is retrieved.
func retrieveLatestRecord(recs []*dbCommon.AttestationRecord) *dbCommon.AttestationRecord {
	if len(recs) == 0 {
		return nil
	}
	lastSource := recs[len(recs)-1].Source
	chosenRec := recs[len(recs)-1]
	for i := len(recs) - 1; i >= 0; i-- {
		// Exit if we are now on a different source
		// as it is assumed that all source records are
		// byte sorted.
		if recs[i].Source != lastSource {
			break
		}
		// If we have a smaller target, we do
		// change our chosen record.
		if chosenRec.Target < recs[i].Target {
			chosenRec = recs[i]
		}
	}
	return chosenRec
}

// UpdateDuties checks the slot number to determine if the validator's
// list of upcoming assignments needs to be updated. For example, at the
// beginning of a new epoch.
func (v *validator) UpdateDuties(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "validator.UpdateDuties")
	defer span.End()

	validatingKeys, err := v.km.FetchValidatingPublicKeys(ctx)
	if err != nil {
		return err
	}

	// Filter out the slashable public keys from the duties request.
	filteredKeys := make([][fieldparams.BLSPubkeyLength]byte, 0, len(validatingKeys))
	v.blacklistedPubkeysLock.RLock()
	for _, pubKey := range validatingKeys {
		if ok := v.blacklistedPubkeys[pubKey]; !ok {
			filteredKeys = append(filteredKeys, pubKey)
		} else {
			log.WithField(
				"pubkey", fmt.Sprintf("%#x", bytesutil.Trunc(pubKey[:])),
			).Warn("Not including slashable public key from slashing protection import " +
				"in request to update validator duties")
		}
	}
	v.blacklistedPubkeysLock.RUnlock()
	epoch := slots.ToEpoch(slots.CurrentSlot(v.genesisTime) + 1)
	req := &ethpb.DutiesRequest{
		Epoch:      epoch,
		PublicKeys: bytesutil.FromBytes48Array(filteredKeys),
	}

	// If duties is nil it means we have had no prior duties and just started up.
	resp, err := v.validatorClient.Duties(ctx, req)
	if err != nil || resp == nil {
		v.dutiesLock.Lock()
		v.duties = nil // Clear assignments so we know to retry the request.
		v.dutiesLock.Unlock()
		log.WithError(err).Error("Error getting validator duties")
		return err
	}

	ss, err := slots.EpochStart(epoch)
	if err != nil {
		return err
	}
	v.dutiesLock.Lock()
	v.duties = resp
	v.logDuties(ss, v.duties.CurrentEpochDuties, v.duties.NextEpochDuties)
	v.dutiesLock.Unlock()

	allExitedCounter := 0
	for i := range resp.CurrentEpochDuties {
		if resp.CurrentEpochDuties[i].Status == ethpb.ValidatorStatus_EXITED {
			allExitedCounter++
		}
	}
	if allExitedCounter != 0 && allExitedCounter == len(resp.CurrentEpochDuties) {
		return ErrValidatorsAllExited
	}

	// Non-blocking call for beacon node to start subscriptions for aggregators.
	// Make sure to copy metadata into a new context
	md, exists := metadata.FromOutgoingContext(ctx)
	ctx = context.Background()
	if exists {
		ctx = metadata.NewOutgoingContext(ctx, md)
	}
	go func() {
		if err := v.subscribeToSubnets(ctx, resp); err != nil {
			log.WithError(err).Error("Failed to subscribe to subnets")
		}
	}()

	return nil
}

// subscribeToSubnets iterates through each validator duty, signs each slot, and asks beacon node
// to eagerly subscribe to subnets so that the aggregator has attestations to aggregate.
func (v *validator) subscribeToSubnets(ctx context.Context, duties *ethpb.ValidatorDutiesContainer) error {
	ctx, span := trace.StartSpan(ctx, "validator.subscribeToSubnets")
	defer span.End()

	subscribeSlots := make([]primitives.Slot, 0, len(duties.CurrentEpochDuties)+len(duties.NextEpochDuties))
	subscribeCommitteeIndices := make([]primitives.CommitteeIndex, 0, len(duties.CurrentEpochDuties)+len(duties.NextEpochDuties))
	subscribeIsAggregator := make([]bool, 0, len(duties.CurrentEpochDuties)+len(duties.NextEpochDuties))
	activeDuties := make([]*ethpb.ValidatorDuty, 0, len(duties.CurrentEpochDuties)+len(duties.NextEpochDuties))
	alreadySubscribed := make(map[[64]byte]bool)

	if v.distributed {
		// Get aggregated selection proofs to calculate isAggregator.
		if err := v.aggregatedSelectionProofs(ctx, duties); err != nil {
			return errors.Wrap(err, "could not get aggregated selection proofs")
		}
	}

	for _, duty := range duties.CurrentEpochDuties {
		pk := bytesutil.ToBytes48(duty.PublicKey)
		if duty.Status == ethpb.ValidatorStatus_ACTIVE || duty.Status == ethpb.ValidatorStatus_EXITING {
			attesterSlot := duty.AttesterSlot
			committeeIndex := duty.CommitteeIndex
			validatorIndex := duty.ValidatorIndex

			alreadySubscribedKey := validatorSubnetSubscriptionKey(attesterSlot, committeeIndex)
			if _, ok := alreadySubscribed[alreadySubscribedKey]; ok {
				continue
			}

			aggregator, err := v.isAggregator(ctx, duty.CommitteeLength, attesterSlot, pk, validatorIndex)
			if err != nil {
				return errors.Wrap(err, "could not check if a validator is an aggregator")
			}
			if aggregator {
				alreadySubscribed[alreadySubscribedKey] = true
			}

			subscribeSlots = append(subscribeSlots, attesterSlot)
			subscribeCommitteeIndices = append(subscribeCommitteeIndices, committeeIndex)
			subscribeIsAggregator = append(subscribeIsAggregator, aggregator)
			activeDuties = append(activeDuties, duty)
		}
	}

	for _, duty := range duties.NextEpochDuties {
		if duty.Status == ethpb.ValidatorStatus_ACTIVE || duty.Status == ethpb.ValidatorStatus_EXITING {
			attesterSlot := duty.AttesterSlot
			committeeIndex := duty.CommitteeIndex
			validatorIndex := duty.ValidatorIndex

			alreadySubscribedKey := validatorSubnetSubscriptionKey(attesterSlot, committeeIndex)
			if _, ok := alreadySubscribed[alreadySubscribedKey]; ok {
				continue
			}

			aggregator, err := v.isAggregator(ctx, duty.CommitteeLength, attesterSlot, bytesutil.ToBytes48(duty.PublicKey), validatorIndex)
			if err != nil {
				return errors.Wrap(err, "could not check if a validator is an aggregator")
			}
			if aggregator {
				alreadySubscribed[alreadySubscribedKey] = true
			}

			subscribeSlots = append(subscribeSlots, attesterSlot)
			subscribeCommitteeIndices = append(subscribeCommitteeIndices, committeeIndex)
			subscribeIsAggregator = append(subscribeIsAggregator, aggregator)
			activeDuties = append(activeDuties, duty)
		}
	}

	_, err := v.validatorClient.SubscribeCommitteeSubnets(ctx,
		&ethpb.CommitteeSubnetsSubscribeRequest{
			Slots:        subscribeSlots,
			CommitteeIds: subscribeCommitteeIndices,
			IsAggregator: subscribeIsAggregator,
		},
		activeDuties,
	)

	return err
}

// RolesAt slot returns the validator roles at the given slot. Returns nil if the
// validator is known to not have a roles at the slot. Returns UNKNOWN if the
// validator assignments are unknown. Otherwise, returns a valid ValidatorRole map.
func (v *validator) RolesAt(ctx context.Context, slot primitives.Slot) (map[[fieldparams.BLSPubkeyLength]byte][]iface.ValidatorRole, error) {
	ctx, span := trace.StartSpan(ctx, "validator.RolesAt")
	defer span.End()

	v.dutiesLock.RLock()
	defer v.dutiesLock.RUnlock()

	if v.duties == nil {
		return nil, errors.New("validator duties are not initialized")
	}

	var (
		rolesAt = make(map[[fieldparams.BLSPubkeyLength]byte][]iface.ValidatorRole)

		// store sync committee duties pubkeys and share indices in slices for
		// potential DV processing
		syncCommitteeValidators = make(map[primitives.ValidatorIndex][fieldparams.BLSPubkeyLength]byte)
	)

	for validator, duty := range v.duties.CurrentEpochDuties {
		var roles []iface.ValidatorRole

		if duty == nil {
			continue
		}
		if len(duty.ProposerSlots) > 0 {
			for _, proposerSlot := range duty.ProposerSlots {
				if proposerSlot != 0 && proposerSlot == slot {
					roles = append(roles, iface.RoleProposer)
					break
				}
			}
		}

		if duty.AttesterSlot == slot {
			roles = append(roles, iface.RoleAttester)

			aggregator, err := v.isAggregator(ctx, duty.CommitteeLength, slot, bytesutil.ToBytes48(duty.PublicKey), duty.ValidatorIndex)
			if err != nil {
				aggregator = false
				log.WithError(err).Errorf("Could not check if validator %#x is an aggregator", bytesutil.Trunc(duty.PublicKey))
			}
			if aggregator {
				roles = append(roles, iface.RoleAggregator)
			}
		}

		// Being assigned to a sync committee for a given slot means that the validator produces and
		// broadcasts signatures for `slot - 1` for inclusion in `slot`. At the last slot of the epoch,
		// the validator checks whether it's in the sync committee of following epoch.
		inSyncCommittee := false
		if slots.IsEpochEnd(slot) {
			if v.duties.NextEpochDuties[validator].IsSyncCommittee {
				roles = append(roles, iface.RoleSyncCommittee)
				inSyncCommittee = true
			}
		} else {
			if duty.IsSyncCommittee {
				roles = append(roles, iface.RoleSyncCommittee)
				inSyncCommittee = true
			}
		}

		if inSyncCommittee {
			syncCommitteeValidators[duty.ValidatorIndex] = bytesutil.ToBytes48(duty.PublicKey)
		}

		if len(roles) == 0 {
			roles = append(roles, iface.RoleUnknown)
		}

		var pubKey [fieldparams.BLSPubkeyLength]byte
		copy(pubKey[:], duty.PublicKey)
		rolesAt[pubKey] = roles
	}

	aggregator, err := v.isSyncCommitteeAggregator(
		ctx,
		slot,
		syncCommitteeValidators,
	)

	if err != nil {
		log.WithError(err).Error("Could not check if any validator is a sync committee aggregator")
		return rolesAt, nil
	}

	for valIdx, isAgg := range aggregator {
		if isAgg {
			valPubkey, ok := syncCommitteeValidators[valIdx]
			if !ok {
				log.
					WithField("pubkey", fmt.Sprintf("%#x", bytesutil.Trunc(valPubkey[:]))).
					Warn("Validator is marked as sync committee aggregator but cannot be found in sync committee validator list")
				continue
			}

			rolesAt[bytesutil.ToBytes48(valPubkey[:])] = append(rolesAt[bytesutil.ToBytes48(valPubkey[:])], iface.RoleSyncCommitteeAggregator)
		}
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
	validatorIndex primitives.ValidatorIndex,
) (bool, error) {
	ctx, span := trace.StartSpan(ctx, "validator.isAggregator")
	defer span.End()

	modulo := uint64(1)
	if committeeLength/params.BeaconConfig().TargetAggregatorsPerCommittee > 1 {
		modulo = committeeLength / params.BeaconConfig().TargetAggregatorsPerCommittee
	}

	var (
		slotSig []byte
		err     error
	)
	if v.distributed {
		// This call is blocking. It is awaitng for selection proof response from DV to be written in memory.
		slotSig, err = v.attSelection(attSelectionKey{slot: slot, index: validatorIndex})
		if err != nil {
			return false, err
		}
	} else {
		slotSig, err = v.signSlotWithSelectionProof(ctx, pubKey, slot)
		if err != nil {
			return false, err
		}
	}

	b := hash.Hash(slotSig)

	return binary.LittleEndian.Uint64(b[:8])%modulo == 0, nil
}

// isSyncCommitteeAggregator checks if a validator in an aggregator of a subcommittee for sync committee.
// it uses a modulo calculated by validator count in committee and samples randomness around it.
//
// Spec code:
// def is_sync_committee_aggregator(signature: BLSSignature) -> bool:
//
//	modulo = max(1, SYNC_COMMITTEE_SIZE // SYNC_COMMITTEE_SUBNET_COUNT // TARGET_AGGREGATORS_PER_SYNC_SUBCOMMITTEE)
//	return bytes_to_uint64(hash(signature)[0:8]) % modulo == 0
func (v *validator) isSyncCommitteeAggregator(ctx context.Context, slot primitives.Slot, validators map[primitives.ValidatorIndex][fieldparams.BLSPubkeyLength]byte) (map[primitives.ValidatorIndex]bool, error) {
	ctx, span := trace.StartSpan(ctx, "validator.isSyncCommitteeAggregator")
	defer span.End()

	var (
		selections []iface.SyncCommitteeSelection
		isAgg      = make(map[primitives.ValidatorIndex]bool)
	)

	for valIdx, pubKey := range validators {
		res, err := v.validatorClient.SyncSubcommitteeIndex(ctx, &ethpb.SyncSubcommitteeIndexRequest{
			PublicKey: pubKey[:],
			Slot:      slot,
		})

		if err != nil {
			return nil, errors.Wrap(err, "can't fetch sync subcommittee index")
		}

		for _, index := range res.Indices {
			subCommitteeSize := params.BeaconConfig().SyncCommitteeSize / params.BeaconConfig().SyncCommitteeSubnetCount
			subnet := uint64(index) / subCommitteeSize
			sig, err := v.signSyncSelectionData(ctx, pubKey, subnet, slot)
			if err != nil {
				return nil, errors.Wrap(err, "can't sign selection data")
			}

			selections = append(selections, iface.SyncCommitteeSelection{
				SelectionProof:    sig,
				Slot:              slot,
				SubcommitteeIndex: primitives.CommitteeIndex(subnet),
				ValidatorIndex:    valIdx,
			})
		}
	}

	// Override selections with aggregated ones if the node is part of a Distributed Validator.
	if v.distributed && len(selections) > 0 {
		var err error
		selections, err = v.validatorClient.AggregatedSyncSelections(ctx, selections)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get aggregated sync selections")
		}
	}

	for _, s := range selections {
		isAggregator, err := altair.IsSyncCommitteeAggregator(s.SelectionProof)
		if err != nil {
			return nil, errors.Wrap(err, "can't detect sync committee aggregator")
		}

		isAgg[s.ValidatorIndex] = isAggregator
	}

	return isAgg, nil
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

func (v *validator) logDuties(slot primitives.Slot, currentEpochDuties []*ethpb.ValidatorDuty, nextEpochDuties []*ethpb.ValidatorDuty) {
	attesterKeys := make([][]string, params.BeaconConfig().SlotsPerEpoch)
	for i := range attesterKeys {
		attesterKeys[i] = make([]string, 0)
	}
	proposerKeys := make([]string, params.BeaconConfig().SlotsPerEpoch)
	epochStartSlot, err := slots.EpochStart(slots.ToEpoch(slot))
	if err != nil {
		log.WithError(err).Error("Could not calculate epoch start. Ignoring logging duties.")
		return
	}
	var totalProposingKeys, totalAttestingKeys uint64
	for _, duty := range currentEpochDuties {
		pubkey := fmt.Sprintf("%#x", duty.PublicKey)
		if v.emitAccountMetrics {
			ValidatorStatusesGaugeVec.WithLabelValues(pubkey, fmt.Sprintf("%#x", duty.ValidatorIndex)).Set(float64(duty.Status))
		}

		// Only interested in validators who are attesting/proposing.
		// Note that SLASHING validators will have duties but their results are ignored by the network so we don't bother with them.
		if duty.Status != ethpb.ValidatorStatus_ACTIVE && duty.Status != ethpb.ValidatorStatus_EXITING {
			continue
		}

		truncatedPubkey := fmt.Sprintf("%#x", bytesutil.Trunc(duty.PublicKey))
		attesterSlotInEpoch := duty.AttesterSlot - epochStartSlot
		if attesterSlotInEpoch >= params.BeaconConfig().SlotsPerEpoch {
			log.WithField("duty", duty).Warn("Invalid attester slot")
		} else {
			attesterKeys[attesterSlotInEpoch] = append(attesterKeys[attesterSlotInEpoch], truncatedPubkey)
			totalAttestingKeys++
			if v.emitAccountMetrics {
				ValidatorNextAttestationSlotGaugeVec.WithLabelValues(pubkey).Set(float64(duty.AttesterSlot))
			}
		}
		if v.emitAccountMetrics && duty.IsSyncCommittee {
			ValidatorInSyncCommitteeGaugeVec.WithLabelValues(pubkey).Set(float64(1))
		} else if v.emitAccountMetrics && !duty.IsSyncCommittee {
			// clear the metric out if the validator is not in the current sync committee anymore otherwise it will be left at 1
			ValidatorInSyncCommitteeGaugeVec.WithLabelValues(pubkey).Set(float64(0))
		}

		for _, proposerSlot := range duty.ProposerSlots {
			proposerSlotInEpoch := proposerSlot - epochStartSlot
			if proposerSlotInEpoch >= params.BeaconConfig().SlotsPerEpoch {
				log.WithField("duty", duty).Warn("Invalid proposer slot")
			} else {
				proposerKeys[proposerSlotInEpoch] = truncatedPubkey
				totalProposingKeys++
			}
			if v.emitAccountMetrics {
				ValidatorNextProposalSlotGaugeVec.WithLabelValues(pubkey).Set(float64(proposerSlot))
			}
		}
	}
	for _, duty := range nextEpochDuties {
		// for the next epoch, currently we are only interested in whether the validator is in the next sync committee or not
		pubkey := fmt.Sprintf("%#x", duty.PublicKey)

		// Only interested in validators who are attesting/proposing.
		// Note that slashed validators will have duties but their results are ignored by the network so we don't bother with them.
		if duty.Status != ethpb.ValidatorStatus_ACTIVE && duty.Status != ethpb.ValidatorStatus_EXITING {
			continue
		}

		if v.emitAccountMetrics && duty.IsSyncCommittee {
			ValidatorInNextSyncCommitteeGaugeVec.WithLabelValues(pubkey).Set(float64(1))
		} else if v.emitAccountMetrics && !duty.IsSyncCommittee {
			// clear the metric out if the validator is now not in the next sync committee otherwise it will be left at 1
			ValidatorInNextSyncCommitteeGaugeVec.WithLabelValues(pubkey).Set(float64(0))
		}
	}

	log.WithFields(logrus.Fields{
		"proposerCount": totalProposingKeys,
		"attesterCount": totalAttestingKeys,
	}).Infof("Schedule for epoch %d", slots.ToEpoch(slot))
	for i := primitives.Slot(0); i < params.BeaconConfig().SlotsPerEpoch; i++ {
		startTime, err := slots.StartTime(v.genesisTime, epochStartSlot+i)
		if err != nil {
			log.WithError(err).WithField("slot", slot).Error("Slot overflows, unable to log duties!")
			return
		}
		durationTillDuty := (time.Until(startTime) + time.Second).Truncate(time.Second) // Round up to next second.

		slotLog := log.WithFields(logrus.Fields{})
		isProposer := proposerKeys[i] != ""
		if isProposer {
			slotLog = slotLog.WithField("proposerPubkey", proposerKeys[i])
		}
		isAttester := len(attesterKeys[i]) > 0
		if isAttester {
			slotLog = slotLog.WithFields(logrus.Fields{
				"slot":            epochStartSlot + i,
				"slotInEpoch":     (epochStartSlot + i) % params.BeaconConfig().SlotsPerEpoch,
				"attesterCount":   len(attesterKeys[i]),
				"attesterPubkeys": attesterKeys[i],
			})
		}
		if durationTillDuty > 0 {
			slotLog = slotLog.WithField("timeUntilDuty", durationTillDuty)
		}
		if isProposer || isAttester {
			slotLog.Infof("Duties schedule")
		}
	}
}

// ProposerSettings gets the current proposer settings saved in memory validator
func (v *validator) ProposerSettings() *proposer.Settings {
	return v.proposerSettings
}

// SetProposerSettings sets and saves the passed in proposer settings overriding the in memory one
func (v *validator) SetProposerSettings(ctx context.Context, settings *proposer.Settings) error {
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

	proposerReqs, err := v.buildPrepProposerReqs(filteredKeys)
	if err != nil {
		return err
	}
	if len(proposerReqs) == 0 {
		log.Warnf("Could not locate valid validator indices. Skipping prepare proposer routine")
		return nil
	}
	if len(proposerReqs) != len(pubkeys) {
		log.WithFields(logrus.Fields{
			"pubkeysCount":                 len(pubkeys),
			"proposerSettingsRequestCount": len(proposerReqs),
		}).Debugln("Request count did not match included validator count. Only keys that have been activated will be included in the request.")
	}

	if _, err := v.validatorClient.PrepareBeaconProposer(ctx, &ethpb.PrepareBeaconProposerRequest{
		Recipients: proposerReqs,
	}); err != nil {
		return err
	}
	signedRegReqs := v.buildSignedRegReqs(ctx, filteredKeys, km.Sign, slot, forceFullPush)
	if len(signedRegReqs) > 0 {
		go func() {
			if err := SubmitValidatorRegistrations(ctx, v.validatorClient, signedRegReqs, v.validatorsRegBatchSize); err != nil {
				log.WithError(errors.Wrap(ErrBuilderValidatorRegistration, err.Error())).Warn("Failed to register validator on builder")
			}
		}()
	}

	return nil
}

func (v *validator) StartEventStream(ctx context.Context, topics []string) {
	if v.EventStreamIsRunning() {
		log.Debug("EventStream is already running")
		return
	}
	log.WithField("topics", topics).Info("Starting event stream")
	v.validatorClient.StartEventStream(ctx, topics, v.eventsChannel)
}

func (v *validator) checkDependentRoots(ctx context.Context, head *structs.HeadEvent) error {
	if head == nil {
		return errors.New("received empty head event")
	}
	prevDependentRoot, err := bytesutil.DecodeHexWithLength(head.PreviousDutyDependentRoot, fieldparams.RootLength)
	if err != nil {
		return errors.Wrap(err, "failed to decode previous duty dependent root")
	}
	if bytes.Equal(prevDependentRoot, params.BeaconConfig().ZeroHash[:]) {
		return nil
	}
	epoch := slots.ToEpoch(slots.CurrentSlot(v.genesisTime) + 1)
	ss, err := slots.EpochStart(epoch + 1)
	if err != nil {
		return errors.Wrap(err, "failed to get epoch start")
	}
	deadline := v.SlotDeadline(ss - 1)
	dutiesCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	v.dutiesLock.RLock()
	needsPrevDependentRootUpdate := v.duties == nil || !bytes.Equal(prevDependentRoot, v.duties.PrevDependentRoot)
	v.dutiesLock.RUnlock()
	if needsPrevDependentRootUpdate {
		// There's an edge case when the initial duties are not set yet
		// This routine will lock and recompute them right after the initial duties finishes.
		if err := v.UpdateDuties(dutiesCtx); err != nil {
			return errors.Wrap(err, "failed to update duties")
		}
		log.Info("Updated duties due to previous dependent root change")
		return nil
	}
	currDepedentRoot, err := bytesutil.DecodeHexWithLength(head.CurrentDutyDependentRoot, fieldparams.RootLength)
	if err != nil {
		return errors.Wrap(err, "failed to decode current duty dependent root")
	}
	if bytes.Equal(currDepedentRoot, params.BeaconConfig().ZeroHash[:]) {
		return nil
	}
	v.dutiesLock.RLock()
	needsCurrDependentRootUpdate := v.duties == nil || !bytes.Equal(currDepedentRoot, v.duties.CurrDependentRoot)
	v.dutiesLock.RUnlock()
	if !needsCurrDependentRootUpdate {
		return nil
	}
	if err := v.UpdateDuties(dutiesCtx); err != nil {
		return errors.Wrap(err, "failed to update duties")
	}
	log.Info("Updated duties due to current dependent root change")
	return nil
}

func (v *validator) ProcessEvent(ctx context.Context, event *eventClient.Event) {
	if event == nil || event.Data == nil {
		log.Warn("Received empty event")
	}
	switch event.EventType {
	case eventClient.EventError:
		log.Error(string(event.Data))
	case eventClient.EventConnectionError:
		log.WithError(errors.New(string(event.Data))).Error("Event stream interrupted")
	case eventClient.EventHead:
		log.Debug("Received head event")
		head := &structs.HeadEvent{}
		if err := json.Unmarshal(event.Data, head); err != nil {
			log.WithError(err).Error("Failed to unmarshal head Event into JSON")
		}
		uintSlot, err := strconv.ParseUint(head.Slot, 10, 64)
		if err != nil {
			log.WithError(err).Error("Failed to parse slot")
			return
		}
		v.setHighestSlot(primitives.Slot(uintSlot))
		if !v.disableDutiesPolling {
			if err := v.checkDependentRoots(ctx, head); err != nil {
				log.WithError(err).Error("Failed to check dependent roots")
			}
		}
	default:
		// just keep going and log the error
		log.WithField("type", event.EventType).WithField("data", string(event.Data)).Warn("Received an unknown event")
	}
}

func (v *validator) EventStreamIsRunning() bool {
	return v.validatorClient.EventStreamIsRunning()
}

func (v *validator) Host() string {
	return v.validatorClient.Host()
}

func (v *validator) changeHost() {
	next := (v.currentHostIndex + 1) % uint64(len(v.beaconNodeHosts))
	log.WithFields(logrus.Fields{
		"currentHost": v.beaconNodeHosts[v.currentHostIndex],
		"nextHost":    v.beaconNodeHosts[next],
	}).Warn("Beacon node is not responding, switching host")
	v.validatorClient.SetHost(v.beaconNodeHosts[next])
	v.currentHostIndex = next
}

func (v *validator) FindHealthyHost(ctx context.Context) bool {
	// Tail-recursive closure keeps retry count private.
	var check func(remaining int) bool
	check = func(remaining int) bool {
		if v.nodeClient.IsHealthy(ctx) { // healthy → done
			return true
		}
		if len(v.beaconNodeHosts) == 1 && features.Get().EnableBeaconRESTApi {
			log.WithField("host", v.Host()).Warn("Beacon node is not responding, no backup node configured")
			return false
		}
		if remaining == 0 || !features.Get().EnableBeaconRESTApi {
			return false // exhausted or REST disabled
		}
		v.changeHost()
		return check(remaining - 1) // recurse
	}

	return check(len(v.beaconNodeHosts))
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
	if isEpochStart || len(v.pubkeyToStatus) != len(pubkeys) /* cache not populated or updated correctly */ {
		if err = v.updateValidatorStatusCache(ctx, pubkeys); err != nil {
			return nil, errors.Wrap(err, "failed to update validator status cache")
		}
	}
	for k, s := range v.pubkeyToStatus {
		currEpoch := primitives.Epoch(slot / params.BeaconConfig().SlotsPerEpoch)
		currActivating := s.status.Status == ethpb.ValidatorStatus_PENDING && currEpoch >= s.status.ActivationEpoch

		active := s.status.Status == ethpb.ValidatorStatus_ACTIVE
		exiting := s.status.Status == ethpb.ValidatorStatus_EXITING

		if currActivating || active || exiting {
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
		v.pubkeyToStatus = make(map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus, 0)
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
	v.pubkeyToStatus = pubkeyToStatus

	return nil
}

func (v *validator) buildPrepProposerReqs(activePubkeys [][fieldparams.BLSPubkeyLength]byte) ([]*ethpb.PrepareBeaconProposerRequest_FeeRecipientContainer, error) {
	var prepareProposerReqs []*ethpb.PrepareBeaconProposerRequest_FeeRecipientContainer
	for _, k := range activePubkeys {
		s, ok := v.pubkeyToStatus[k]
		if !ok {
			continue
		}

		// Default case: Define fee recipient to burn address
		feeRecipient := common.HexToAddress(params.BeaconConfig().EthBurnAddressHex)

		// If fee recipient is defined in default configuration, use it
		if v.ProposerSettings() != nil && v.ProposerSettings().DefaultConfig != nil && v.ProposerSettings().DefaultConfig.FeeRecipientConfig != nil {
			feeRecipient = v.ProposerSettings().DefaultConfig.FeeRecipientConfig.FeeRecipient // Use cli config for fee recipient.
		}

		// If fee recipient is defined for this specific pubkey in proposer configuration, use it
		if v.ProposerSettings() != nil && v.ProposerSettings().ProposeConfig != nil {
			config, ok := v.ProposerSettings().ProposeConfig[k]

			if ok && config != nil && config.FeeRecipientConfig != nil {
				feeRecipient = config.FeeRecipientConfig.FeeRecipient // Use file config for fee recipient.
			}
		}

		prepareProposerReqs = append(prepareProposerReqs, &ethpb.PrepareBeaconProposerRequest_FeeRecipientContainer{
			ValidatorIndex: s.index,
			FeeRecipient:   feeRecipient[:],
		})
	}
	return prepareProposerReqs, nil
}

func (v *validator) buildSignedRegReqs(
	ctx context.Context,
	activePubkeys [][fieldparams.BLSPubkeyLength]byte,
	signer iface.SigningFunc,
	slot primitives.Slot,
	forceFullPush bool,
) []*ethpb.SignedValidatorRegistrationV1 {
	ctx, span := trace.StartSpan(ctx, "validator.buildSignedRegReqs")
	defer span.End()

	var signedValRegRequests []*ethpb.SignedValidatorRegistrationV1
	if v.ProposerSettings() == nil {
		return signedValRegRequests
	}
	// if the timestamp is pre-genesis, don't create registrations
	if time.Now().Before(v.genesisTime) {
		return signedValRegRequests
	}

	if v.ProposerSettings().DefaultConfig != nil && v.ProposerSettings().DefaultConfig.FeeRecipientConfig == nil && v.ProposerSettings().DefaultConfig.BuilderConfig != nil {
		log.Warn("Builder is `enabled` in default config but will be ignored because no fee recipient was provided!")
	}

	for i, k := range activePubkeys {
		// map is populated before this function in buildPrepProposerReq
		_, ok := v.pubkeyToStatus[k]
		if !ok {
			continue
		}

		feeRecipient := common.HexToAddress(params.BeaconConfig().EthBurnAddressHex)
		gasLimit := params.BeaconConfig().DefaultBuilderGasLimit
		enabled := false

		if v.ProposerSettings().DefaultConfig != nil && v.ProposerSettings().DefaultConfig.FeeRecipientConfig != nil {
			defaultConfig := v.ProposerSettings().DefaultConfig
			feeRecipient = defaultConfig.FeeRecipientConfig.FeeRecipient // Use cli defaultBuilderConfig for fee recipient.
			defaultBuilderConfig := defaultConfig.BuilderConfig

			if defaultBuilderConfig != nil && defaultBuilderConfig.Enabled {
				gasLimit = uint64(defaultBuilderConfig.GasLimit) // Use cli config for gas limit.
				enabled = true
			}
		}

		if v.ProposerSettings().ProposeConfig != nil {
			config, ok := v.ProposerSettings().ProposeConfig[k]
			if ok && config != nil && config.FeeRecipientConfig != nil {
				feeRecipient = config.FeeRecipientConfig.FeeRecipient // Use file config for fee recipient.
				builderConfig := config.BuilderConfig
				if builderConfig != nil {
					if builderConfig.Enabled {
						gasLimit = uint64(builderConfig.GasLimit) // Use file config for gas limit.
						enabled = true
					} else {
						enabled = false // Custom config can disable validator from register.
					}
				}
			}
		}

		if !enabled {
			continue
		}

		req := &ethpb.ValidatorRegistrationV1{
			FeeRecipient: feeRecipient[:],
			GasLimit:     gasLimit,
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

func (v *validator) aggregatedSelectionProofs(ctx context.Context, duties *ethpb.ValidatorDutiesContainer) error {
	ctx, span := trace.StartSpan(ctx, "validator.aggregatedSelectionProofs")
	defer span.End()

	// Lock the selection proofs until we receive response from DV.
	v.attSelectionLock.Lock()
	defer v.attSelectionLock.Unlock()

	// Create new instance of attestation selections map.
	v.attSelections = make(map[attSelectionKey]iface.BeaconCommitteeSelection)

	var req []iface.BeaconCommitteeSelection
	for _, duty := range duties.CurrentEpochDuties {
		if duty.Status != ethpb.ValidatorStatus_ACTIVE && duty.Status != ethpb.ValidatorStatus_EXITING {
			continue
		}

		pk := bytesutil.ToBytes48(duty.PublicKey)
		slotSig, err := v.signSlotWithSelectionProof(ctx, pk, duty.AttesterSlot)
		if err != nil {
			return err
		}

		req = append(req, iface.BeaconCommitteeSelection{
			SelectionProof: slotSig,
			Slot:           duty.AttesterSlot,
			ValidatorIndex: duty.ValidatorIndex,
		})
	}

	resp, err := v.validatorClient.AggregatedSelections(ctx, req)
	if err != nil {
		return err
	}

	// Store aggregated selection proofs in state.
	for _, s := range resp {
		v.attSelections[attSelectionKey{
			slot:  s.Slot,
			index: s.ValidatorIndex,
		}] = s
	}

	return nil
}

func (v *validator) attSelection(key attSelectionKey) ([]byte, error) {
	v.attSelectionLock.Lock()
	defer v.attSelectionLock.Unlock()

	s, ok := v.attSelections[key]
	if !ok {
		return nil, errors.Errorf("selection proof not found for the given slot=%d and validator_index=%d", key.slot, key.index)
	}

	return s.SelectionProof, nil
}

// This constructs a validator subscribed key, it's used to track
// which subnet has already been pending requested.
func validatorSubnetSubscriptionKey(slot primitives.Slot, committeeIndex primitives.CommitteeIndex) [64]byte {
	return bytesutil.ToBytes64(append(bytesutil.Bytes32(uint64(slot)), bytesutil.Bytes32(uint64(committeeIndex))...))
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

// This tracks all validators' submissions for sync committees.
type syncCommitteeStats struct {
	totalMessagesSubmitted atomic.Uint64
}
