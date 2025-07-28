package testutil

import (
	"bytes"
	"context"
	"errors"
	"time"

	api "github.com/OffchainLabs/prysm/v6/api/client"
	"github.com/OffchainLabs/prysm/v6/api/client/event"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/config/proposer"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	prysmTime "github.com/OffchainLabs/prysm/v6/time"
	"github.com/OffchainLabs/prysm/v6/validator/client/iface"
	"github.com/OffchainLabs/prysm/v6/validator/keymanager"
	log "github.com/sirupsen/logrus"
)

var _ iface.Validator = (*FakeValidator)(nil)

// FakeValidator for mocking.
type FakeValidator struct {
	CanChangeHost                     bool
	LogValidatorGainsAndLossesCalled  bool
	SaveProtectionsCalled             bool
	DeleteProtectionCalled            bool
	SlotDeadlineCalled                bool
	HandleKeyReloadCalled             bool
	AttestToBlockHeadCalled           bool
	SlasherReadyCalled                bool
	DoneCalled                        bool
	RoleAtCalled                      bool
	IsRegularDeadline                 bool
	ProposeBlockCalled                bool
	UpdateProtectionsCalled           bool
	UpdateDutiesCalled                bool
	WaitForWalletInitializationCalled bool
	NextSlotCalled                    bool
	WaitForActivationCalled           int
	WaitForSyncCalled                 int
	RetryTillSuccess                  int
	ProposeBlockArg1                  uint64
	AttestToBlockHeadArg1             uint64
	RoleAtArg1                        uint64
	UpdateDutiesArg1                  uint64
	NextSlotRet                       <-chan primitives.Slot
	ProposerSettingWait               time.Duration
	PubkeysToStatusesMap              map[[fieldparams.BLSPubkeyLength]byte]ethpb.ValidatorStatus
	PubkeyToIndexMap                  map[[fieldparams.BLSPubkeyLength]byte]uint64
	IndexToPubkeyMap                  map[uint64][fieldparams.BLSPubkeyLength]byte
	WaitForChainStartCalled           int
	AttSubmitted                      chan interface{}
	BlockProposed                     chan interface{}
	AccountsChannel                   chan [][fieldparams.BLSPubkeyLength]byte
	GenesisT                          time.Time
	ReceiveBlocksCalled               int
	proposerSettings                  *proposer.Settings
	Balances                          map[[48]byte]uint64
	EventsChannel                     chan *event.Event
	ProposerSettingsErr               error
	Km                                keymanager.IKeymanager
	graffiti                          string
	PublicKey                         string
	UpdateDutiesRet                   error
	RolesAtRet                        []iface.ValidatorRole
}

// Done for mocking.
func (fv *FakeValidator) Done() {
	fv.DoneCalled = true
}

func (fv *FakeValidator) EventsChan() <-chan *event.Event {
	return fv.EventsChannel
}

func (fv *FakeValidator) AccountsChangedChan() <-chan [][fieldparams.BLSPubkeyLength]byte {
	return fv.AccountsChannel
}

func (fv *FakeValidator) GenesisTime() time.Time {
	return fv.GenesisT
}

// WaitForKeymanagerInitialization for mocking.
func (fv *FakeValidator) WaitForKeymanagerInitialization(_ context.Context) error {
	fv.WaitForWalletInitializationCalled = true
	return nil
}

// LogSubmittedSyncCommitteeMessages --
func (fv *FakeValidator) LogSubmittedSyncCommitteeMessages() {}

// WaitForChainStart for mocking.
func (fv *FakeValidator) WaitForChainStart(_ context.Context) error {
	fv.WaitForChainStartCalled++
	if fv.RetryTillSuccess >= fv.WaitForChainStartCalled {
		return api.ErrConnectionIssue
	}
	return nil
}

// WaitForActivation for mocking.
func (fv *FakeValidator) WaitForActivation(_ context.Context) error {
	fv.WaitForActivationCalled++
	if fv.AccountsChannel == nil {
		return nil
	}
	if fv.RetryTillSuccess >= fv.WaitForActivationCalled {
		return api.ErrConnectionIssue
	}
	return nil
}

// WaitForSync for mocking.
func (fv *FakeValidator) WaitForSync(_ context.Context) error {
	fv.WaitForSyncCalled++
	if fv.RetryTillSuccess >= fv.WaitForSyncCalled {
		return api.ErrConnectionIssue
	}
	return nil
}

// SlasherReady for mocking.
func (fv *FakeValidator) SlasherReady(_ context.Context) error {
	fv.SlasherReadyCalled = true
	return nil
}

// SlotDeadline for mocking.
func (fv *FakeValidator) SlotDeadline(_ primitives.Slot) time.Time {
	fv.SlotDeadlineCalled = true
	if fv.IsRegularDeadline {
		return prysmTime.Now().Add(time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second)
	}
	return prysmTime.Now()
}

// NextSlot for mocking.
func (fv *FakeValidator) NextSlot() <-chan primitives.Slot {
	fv.NextSlotCalled = true
	return fv.NextSlotRet
}

// UpdateDuties for mocking.
func (fv *FakeValidator) UpdateDuties(_ context.Context) error {
	fv.UpdateDutiesCalled = true
	return fv.UpdateDutiesRet
}

// UpdateProtections for mocking.
func (fv *FakeValidator) UpdateProtections(_ context.Context, _ uint64) error {
	fv.UpdateProtectionsCalled = true
	return nil
}

// LogValidatorGainsAndLosses for mocking.
func (fv *FakeValidator) LogValidatorGainsAndLosses(_ context.Context, _ primitives.Slot) error {
	fv.LogValidatorGainsAndLossesCalled = true
	return nil
}

// ResetAttesterProtectionData for mocking.
func (fv *FakeValidator) ResetAttesterProtectionData() {
	fv.DeleteProtectionCalled = true
}

// RolesAt for mocking.
func (fv *FakeValidator) RolesAt(_ context.Context, slot primitives.Slot) (map[[fieldparams.BLSPubkeyLength]byte][]iface.ValidatorRole, error) {
	fv.RoleAtCalled = true
	fv.RoleAtArg1 = uint64(slot)
	vr := make(map[[fieldparams.BLSPubkeyLength]byte][]iface.ValidatorRole)
	vr[[fieldparams.BLSPubkeyLength]byte{1}] = fv.RolesAtRet
	return vr, nil
}

// SubmitAttestation for mocking.
func (fv *FakeValidator) SubmitAttestation(_ context.Context, slot primitives.Slot, _ [fieldparams.BLSPubkeyLength]byte) {
	fv.AttestToBlockHeadCalled = true
	fv.AttestToBlockHeadArg1 = uint64(slot)
	if fv.AttSubmitted != nil {
		close(fv.AttSubmitted)
		fv.AttSubmitted = nil
	}
}

// ProposeBlock for mocking.
func (fv *FakeValidator) ProposeBlock(_ context.Context, slot primitives.Slot, _ [fieldparams.BLSPubkeyLength]byte) {
	fv.ProposeBlockCalled = true
	fv.ProposeBlockArg1 = uint64(slot)
	if fv.BlockProposed != nil {
		close(fv.BlockProposed)
		fv.BlockProposed = nil
	}
}

// SubmitAggregateAndProof for mocking.
func (*FakeValidator) SubmitAggregateAndProof(_ context.Context, _ primitives.Slot, _ [fieldparams.BLSPubkeyLength]byte) {
}

// SubmitSyncCommitteeMessage for mocking.
func (*FakeValidator) SubmitSyncCommitteeMessage(_ context.Context, _ primitives.Slot, _ [fieldparams.BLSPubkeyLength]byte) {
}

// LogSubmittedAtts for mocking.
func (*FakeValidator) LogSubmittedAtts(_ primitives.Slot) {}

// UpdateDomainDataCaches for mocking.
func (*FakeValidator) UpdateDomainDataCaches(context.Context, primitives.Slot) {}

// BalancesByPubkeys for mocking.
func (fv *FakeValidator) BalancesByPubkeys(_ context.Context) map[[fieldparams.BLSPubkeyLength]byte]uint64 {
	return fv.Balances
}

// IndicesToPubkeys for mocking.
func (fv *FakeValidator) IndicesToPubkeys(_ context.Context) map[uint64][fieldparams.BLSPubkeyLength]byte {
	return fv.IndexToPubkeyMap
}

// PubkeysToIndices for mocking.
func (fv *FakeValidator) PubkeysToIndices(_ context.Context) map[[fieldparams.BLSPubkeyLength]byte]uint64 {
	return fv.PubkeyToIndexMap
}

// PubkeysToStatuses for mocking.
func (fv *FakeValidator) PubkeysToStatuses(_ context.Context) map[[fieldparams.BLSPubkeyLength]byte]ethpb.ValidatorStatus {
	return fv.PubkeysToStatusesMap
}

// Keymanager for mocking
func (fv *FakeValidator) Keymanager() (keymanager.IKeymanager, error) {
	return fv.Km, nil
}

// CheckDoppelGanger for mocking
func (*FakeValidator) CheckDoppelGanger(_ context.Context) error {
	return nil
}

// HandleKeyReload for mocking
func (fv *FakeValidator) HandleKeyReload(_ context.Context, newKeys [][fieldparams.BLSPubkeyLength]byte) (anyActive bool, err error) {
	fv.HandleKeyReloadCalled = true
	for _, key := range newKeys {
		if bytes.Equal(key[:], ActiveKey[:]) {
			return true, nil
		}
	}
	return false, nil
}

// SubmitSignedContributionAndProof for mocking
func (*FakeValidator) SubmitSignedContributionAndProof(_ context.Context, _ primitives.Slot, _ [fieldparams.BLSPubkeyLength]byte) {
}

// HasProposerSettings for mocking
func (*FakeValidator) HasProposerSettings() bool {
	return true
}

// PushProposerSettings for mocking
func (fv *FakeValidator) PushProposerSettings(ctx context.Context, _ primitives.Slot, _ bool) error {
	time.Sleep(fv.ProposerSettingWait)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		log.Error("Deadline exceeded")
		// can't return error as it will trigger a log.fatal
		return nil
	}

	if fv.ProposerSettingsErr != nil {
		return fv.ProposerSettingsErr
	}

	log.Infoln("Mock updated proposer settings")
	return nil
}

// SetPubKeyToValidatorIndexMap for mocking
func (*FakeValidator) SetPubKeyToValidatorIndexMap(_ context.Context, _ keymanager.IKeymanager) error {
	return nil
}

// SignValidatorRegistrationRequest for mocking
func (*FakeValidator) SignValidatorRegistrationRequest(_ context.Context, _ iface.SigningFunc, _ *ethpb.ValidatorRegistrationV1) (*ethpb.SignedValidatorRegistrationV1, bool, error) {
	return nil, false, nil
}

// ProposerSettings for mocking
func (fv *FakeValidator) ProposerSettings() *proposer.Settings {
	return fv.proposerSettings
}

// SetProposerSettings for mocking
func (fv *FakeValidator) SetProposerSettings(_ context.Context, settings *proposer.Settings) error {
	fv.proposerSettings = settings
	return nil
}

// Graffiti for mocking
func (fv *FakeValidator) Graffiti(_ context.Context, _ [fieldparams.BLSPubkeyLength]byte) ([]byte, error) {
	return []byte(fv.graffiti), nil
}

// SetGraffiti for mocking
func (fv *FakeValidator) SetGraffiti(_ context.Context, _ [fieldparams.BLSPubkeyLength]byte, graffiti []byte) error {
	fv.graffiti = string(graffiti)
	return nil
}

// DeleteGraffiti for mocking
func (fv *FakeValidator) DeleteGraffiti(_ context.Context, _ [fieldparams.BLSPubkeyLength]byte) error {
	fv.graffiti = ""
	return nil
}

func (*FakeValidator) StartEventStream(_ context.Context, _ []string) {

}

func (*FakeValidator) ProcessEvent(_ context.Context, _ *event.Event) {}

func (*FakeValidator) EventStreamIsRunning() bool {
	return true
}

func (*FakeValidator) Host() string {
	return "127.0.0.1:0"
}

func (fv *FakeValidator) FindHealthyHost(_ context.Context) bool {
	return fv.CanChangeHost
}

func (fv *FakeValidator) SetTicker() {
}
