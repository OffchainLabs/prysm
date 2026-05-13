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
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/OffchainLabs/prysm/v7/api/client"
	eventClient "github.com/OffchainLabs/prysm/v7/api/client/event"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/async/event"
	"github.com/OffchainLabs/prysm/v7/cmd"
	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/config/proposer"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	accountsiface "github.com/OffchainLabs/prysm/v7/validator/accounts/iface"
	"github.com/OffchainLabs/prysm/v7/validator/accounts/wallet"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	"github.com/OffchainLabs/prysm/v7/validator/db"
	dbCommon "github.com/OffchainLabs/prysm/v7/validator/db/common"
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
	attLogsLock                  sync.Mutex
	highestValidSlotLock         sync.Mutex
	blacklistedPubkeysLock       sync.RWMutex
	prevEpochBalancesLock        sync.RWMutex
	cachedAttestationDataLock    sync.RWMutex
	submittedPrefSlotsLock       sync.RWMutex
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
	web3SignerConfig             *remoteweb3signer.SetupConfig
	proposerSettings             *proposer.Settings
	submittedPrefSlots           map[primitives.Slot]bool
	submittedAtts                map[submittedAttKey]*submittedAtt
	validatorsRegBatchSize       int
	duties                       *dutyStore
	interopKeysConfig            *local.InteropKeymanagerConfig
	domainDataCache              *ristretto.Cache[string, proto.Message]
	slotFeed                     *event.Feed
	syncCommitteeStats           syncCommitteeStats
	graffitiStruct               *graffiti.Graffiti
	submittedAggregates          map[submittedAttKey]*submittedAtt
	highestValidSlot             primitives.Slot
	eventsChannel                chan *eventClient.Event
	pubkeyToStatus               map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus
	signedValidatorRegistrations map[[fieldparams.BLSPubkeyLength]byte]*ethpb.SignedValidatorRegistrationV1
	aggSelector                  aggregatorSelector
	validatorClient              iface.ValidatorClient
	chainClient                  iface.ChainClient
	nodeClient                   iface.NodeClient
	prysmChainClient             iface.PrysmChainClient
	db                           db.Database
	conn                         validatorHelpers.NodeConnection
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

func (v *validator) indexFromPubkey(pubKey [fieldparams.BLSPubkeyLength]byte) (primitives.ValidatorIndex, error) {
	s, ok := v.pubkeyToStatus[pubKey]
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

// RolesAt slot returns the validator roles at the given slot. Returns nil if the
// validator is known to not have a roles at the slot. Returns UNKNOWN if the
// validator assignments are unknown. Otherwise, returns a valid ValidatorRole map.
func (v *validator) RolesAt(ctx context.Context, slot primitives.Slot) (map[[fieldparams.BLSPubkeyLength]byte][]iface.ValidatorRole, error) {
	ctx, span := trace.StartSpan(ctx, "validator.RolesAt")
	defer span.End()

	snap := v.duties.Snapshot()
	if !snap.initialized {
		return nil, errors.New("validator duties are not initialized")
	}

	var (
		rolesAt              = make(map[[fieldparams.BLSPubkeyLength]byte][]iface.ValidatorRole)
		syncCommitteePubkeys [][fieldparams.BLSPubkeyLength]byte
	)

	for pk, duty := range snap.currentDuties {
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

			aggregator, err := v.isAggregator(ctx, duty.CommitteeLength, slot, pk)
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
			if snap.IsNextSyncCommittee(duty.ValidatorIndex) {
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
			syncCommitteePubkeys = append(syncCommitteePubkeys, pk)
		}

		if slices.Contains(snap.PtcSlots(duty.ValidatorIndex), slot) {
			roles = append(roles, iface.RolePTCMember)
		}

		if len(roles) == 0 {
			roles = append(roles, iface.RoleUnknown)
		}

		rolesAt[pk] = roles
	}

	aggPubkeys, err := v.aggSelector.SyncCommitteeAggregators(ctx, slot, syncCommitteePubkeys)
	if err != nil {
		log.WithError(err).Error("Could not check if any validator is a sync committee aggregator")
		return rolesAt, nil
	}
	for _, pk := range aggPubkeys {
		rolesAt[pk] = append(rolesAt[pk], iface.RoleSyncCommitteeAggregator)
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

	proposerReqs := v.buildProposerSettingsRequests(filteredKeys)
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

	// TODO(gloas): add gloas flag to stop needing prepare beacon proposer post gloas
	if _, err := v.validatorClient.PrepareBeaconProposer(ctx, &ethpb.PrepareBeaconProposerRequest{
		Recipients: proposerReqs,
	}); err != nil {
		return err
	}

	prefs := v.buildProposerPreferences(ctx, km, slot)
	if len(prefs) > 0 {
		// Delay to mid-slot so the block for this slot is processed first.
		delay := time.Duration(params.BeaconConfig().SecondsPerSlot/2) * time.Second
		go func() {
			time.Sleep(delay)
			if _, err := v.validatorClient.SubmitSignedProposerPreferences(ctx, &ethpb.SubmitSignedProposerPreferencesRequest{
				SignedProposerPreferences: prefs,
			}); err != nil {
				log.WithError(err).Warn("Failed to submit proposer preferences")
			}
		}()
	}

	// TODO: figure out what to do post gloas for builder apis
	if slots.ToEpoch(slot) >= params.BeaconConfig().GloasForkEpoch {
		return nil
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
	if isEpochStart || len(v.pubkeyToStatus) != len(pubkeys) /* cache not populated or updated correctly */ {
		if err = v.updateValidatorStatusCache(ctx, pubkeys); err != nil {
			return nil, errors.Wrap(err, "failed to update validator status cache")
		}
	}
	currEpoch := slots.ToEpoch(slot)
	for k, s := range v.pubkeyToStatus {
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

// buildProposerSettingsRequests builds both PrepareBeaconProposer requests and,
// post-Gloas, signed proposer preferences from the same validator settings.
func (v *validator) buildProposerSettingsRequests(
	activePubkeys [][fieldparams.BLSPubkeyLength]byte,
) []*ethpb.PrepareBeaconProposerRequest_FeeRecipientContainer {
	var prepareProposerReqs []*ethpb.PrepareBeaconProposerRequest_FeeRecipientContainer
	ps := v.ProposerSettings()
	for _, k := range activePubkeys {
		s, ok := v.pubkeyToStatus[k]
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

// buildProposerPreferences creates signed proposer preferences for validators
// that have proposer slots in the current epoch (future slots) or next epoch.
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
	for s := range v.submittedPrefSlots {
		if s < epochStart {
			delete(v.submittedPrefSlots, s)
		}
	}
	v.submittedPrefSlotsLock.Unlock()

	snap := v.duties.Snapshot()
	if !snap.initialized {
		return nil
	}

	ps := v.ProposerSettings()
	var signedPrefs []*ethpb.SignedProposerPreferences
	var sigFailCount int

	processDuties := func(duties map[pubkey]*ethpb.ValidatorDuty, isNextEpoch bool) {
		for pk, duty := range duties {
			if len(duty.ProposerSlots) == 0 {
				continue
			}
			if duty.Status != ethpb.ValidatorStatus_ACTIVE && duty.Status != ethpb.ValidatorStatus_EXITING {
				continue
			}

			feeRecipient := common.HexToAddress(params.BeaconConfig().EthBurnAddressHex)
			gasLimit := params.BeaconConfig().DefaultBuilderGasLimit
			if ps != nil && ps.DefaultConfig != nil {
				if ps.DefaultConfig.FeeRecipientConfig != nil {
					feeRecipient = ps.DefaultConfig.FeeRecipientConfig.FeeRecipient
				}
				if ps.DefaultConfig.BuilderConfig != nil && ps.DefaultConfig.BuilderConfig.Enabled {
					gasLimit = uint64(ps.DefaultConfig.BuilderConfig.GasLimit)
				}
			}
			if ps != nil && ps.ProposeConfig != nil {
				if config, ok := ps.ProposeConfig[pk]; ok && config != nil {
					if config.FeeRecipientConfig != nil {
						feeRecipient = config.FeeRecipientConfig.FeeRecipient
					}
					if config.BuilderConfig != nil && config.BuilderConfig.Enabled {
						gasLimit = uint64(config.BuilderConfig.GasLimit)
					}
				}
			}

			for _, proposalSlot := range duty.ProposerSlots {
				v.submittedPrefSlotsLock.RLock()
				already := v.submittedPrefSlots[proposalSlot]
				v.submittedPrefSlotsLock.RUnlock()
				if already {
					continue
				}
				// Skip slots that have passed or are too close. Preferences are
				// submitted at mid-slot, so the proposer needs to be at least 1
				// full slot away for the beacon node to receive them in time.
				if !isNextEpoch && proposalSlot <= slot+1 {
					continue
				}

				pref := &ethpb.ProposerPreferences{
					ProposalSlot:   proposalSlot,
					ValidatorIndex: duty.ValidatorIndex,
					FeeRecipient:   feeRecipient[:],
					GasLimit:       gasLimit,
				}
				signedPref, err := v.signProposerPreferences(ctx, km, pk, pref)
				if err != nil {
					sigFailCount++
					continue
				}
				signedPrefs = append(signedPrefs, signedPref)
				v.submittedPrefSlotsLock.Lock()
				v.submittedPrefSlots[proposalSlot] = true
				v.submittedPrefSlotsLock.Unlock()
			}
		}
	}

	currentDuties := snap.currentDuties
	nextDuties := snap.nextDuties

	var currentProposerCount, nextProposerCount int
	for _, d := range currentDuties {
		currentProposerCount += len(d.ProposerSlots)
	}
	for _, d := range nextDuties {
		nextProposerCount += len(d.ProposerSlots)
	}

	// Current-epoch: submit after first slot of epoch to avoid stale state.
	// Only post-gloas — current-epoch prefs before gloas would be rejected.
	if currentEpoch >= gloasEpoch && slot > epochStart {
		processDuties(currentDuties, false)
	}

	// Next-epoch: submit at or after mid-epoch.
	if slot >= midEpoch {
		processDuties(nextDuties, true)
	}

	if sigFailCount > 0 {
		log.WithField("count", sigFailCount).Warn("Failed to sign proposer preferences")
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

func (v *validator) submittedPrefSlotsCount() int {
	v.submittedPrefSlotsLock.RLock()
	defer v.submittedPrefSlotsLock.RUnlock()
	return len(v.submittedPrefSlots)
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
