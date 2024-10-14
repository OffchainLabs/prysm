package mock

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/prysmaticlabs/prysm/v5/api/client/beacon"
	"github.com/prysmaticlabs/prysm/v5/api/client/event"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/proposer"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/validator/accounts/iface"
	iface2 "github.com/prysmaticlabs/prysm/v5/validator/client/iface"
	"github.com/prysmaticlabs/prysm/v5/validator/keymanager"
)

// Wallet contains an in-memory, simulated wallet implementation.
type Wallet struct {
	InnerAccountsDir  string
	Directories       []string
	Files             map[string]map[string][]byte
	EncryptedSeedFile []byte
	AccountPasswords  map[string]string
	WalletPassword    string
	UnlockAccounts    bool
	lock              sync.RWMutex
	HasWriteFileError bool
	WalletDir         string
	Kind              keymanager.Kind
}

// AccountNames --
func (w *Wallet) AccountNames() ([]string, error) {
	w.lock.RLock()
	defer w.lock.RUnlock()
	names := make([]string, 0)
	for name := range w.AccountPasswords {
		names = append(names, name)
	}
	return names, nil
}

// AccountsDir --
func (w *Wallet) AccountsDir() string {
	return w.InnerAccountsDir
}

// Dir for the wallet.
func (w *Wallet) Dir() string {
	return w.WalletDir
}

// KeymanagerKind --
func (w *Wallet) KeymanagerKind() keymanager.Kind {
	return w.Kind
}

// Exists --
func (w *Wallet) Exists() (bool, error) {
	return len(w.Directories) > 0, nil
}

// Password --
func (w *Wallet) Password() string {
	return w.WalletPassword
}

// WriteFileAtPath --
func (w *Wallet) WriteFileAtPath(_ context.Context, pathName, fileName string, data []byte) (bool, error) {
	w.lock.Lock()
	defer w.lock.Unlock()
	if w.HasWriteFileError {
		// reset the flag to not contaminate other tests
		w.HasWriteFileError = false
		return false, errors.New("could not write keystore file for accounts")
	}
	if w.Files[pathName] == nil {
		w.Files[pathName] = make(map[string][]byte)
	}
	w.Files[pathName][fileName] = data
	return true, nil
}

// ReadFileAtPath --
func (w *Wallet) ReadFileAtPath(_ context.Context, pathName, fileName string) ([]byte, error) {
	w.lock.RLock()
	defer w.lock.RUnlock()
	for f, v := range w.Files[pathName] {
		if strings.Contains(fileName, f) {
			return v, nil
		}
	}
	return nil, errors.New("no files found")
}

// InitializeKeymanager --
func (_ *Wallet) InitializeKeymanager(_ context.Context, _ iface.InitKeymanagerConfig) (keymanager.IKeymanager, error) {
	return nil, nil
}

type Validator struct {
	Km               keymanager.IKeymanager
	graffiti         string
	proposerSettings *proposer.Settings
}

func (_ *Validator) LogSubmittedSyncCommitteeMessages() {}

func (_ *Validator) Done() {
	panic("implement me")
}

func (_ *Validator) WaitForChainStart(_ context.Context) error {
	panic("implement me")
}

func (_ *Validator) WaitForSync(_ context.Context) error {
	panic("implement me")
}

func (_ *Validator) WaitForActivation(_ context.Context, _ chan [][48]byte) error {
	panic("implement me")
}

func (_ *Validator) CanonicalHeadSlot(_ context.Context) (primitives.Slot, error) {
	panic("implement me")
}

func (_ *Validator) NextSlot() <-chan primitives.Slot {
	panic("implement me")
}

func (_ *Validator) SlotDeadline(_ primitives.Slot) time.Time {
	panic("implement me")
}

func (_ *Validator) LogValidatorGainsAndLosses(_ context.Context, _ primitives.Slot) error {
	panic("implement me")
}

func (_ *Validator) UpdateDuties(_ context.Context, _ primitives.Slot) error {
	panic("implement me")
}

func (_ *Validator) RolesAt(_ context.Context, _ primitives.Slot) (map[[48]byte][]iface2.ValidatorRole, error) {
	panic("implement me")
}

func (_ *Validator) SubmitAttestation(_ context.Context, _ primitives.Slot, _ [48]byte) {
	panic("implement me")
}

func (_ *Validator) ProposeBlock(_ context.Context, _ primitives.Slot, _ [48]byte) {
	panic("implement me")
}

func (_ *Validator) SubmitAggregateAndProof(_ context.Context, _ primitives.Slot, _ [48]byte) {
	panic("implement me")
}

func (_ *Validator) SubmitSyncCommitteeMessage(_ context.Context, _ primitives.Slot, _ [48]byte) {
	panic("implement me")
}

func (_ *Validator) SubmitSignedContributionAndProof(_ context.Context, _ primitives.Slot, _ [48]byte) {
	panic("implement me")
}

func (_ *Validator) SubmitPayloadAttestationMessage(_ context.Context, _ primitives.Slot, _ [48]byte) {
	panic("implement me")
}

func (_ *Validator) LogSubmittedAtts(_ primitives.Slot) {
	panic("implement me")
}

func (_ *Validator) UpdateDomainDataCaches(_ context.Context, _ primitives.Slot) {
	panic("implement me")
}

func (_ *Validator) WaitForKeymanagerInitialization(_ context.Context) error {
	panic("implement me")
}

func (m *Validator) Keymanager() (keymanager.IKeymanager, error) {
	return m.Km, nil
}

func (_ *Validator) ReceiveSlots(_ context.Context, _ chan<- error) {
	panic("implement me")
}

func (_ *Validator) HandleKeyReload(_ context.Context, _ [][48]byte) (bool, error) {
	panic("implement me")
}

func (_ *Validator) CheckDoppelGanger(_ context.Context) error {
	panic("implement me")
}

// HasProposerSettings for mocking
func (*Validator) HasProposerSettings() bool {
	panic("implement me")
}

// PushProposerSettings for mocking
func (_ *Validator) PushProposerSettings(_ context.Context, _ keymanager.IKeymanager, _ primitives.Slot, _ bool) error {
	panic("implement me")
}

// SetPubKeyToValidatorIndexMap for mocking
func (_ *Validator) SetPubKeyToValidatorIndexMap(_ context.Context, _ keymanager.IKeymanager) error {
	panic("implement me")
}

// SignValidatorRegistrationRequest for mocking
func (_ *Validator) SignValidatorRegistrationRequest(_ context.Context, _ iface2.SigningFunc, _ *ethpb.ValidatorRegistrationV1) (*ethpb.SignedValidatorRegistrationV1, bool, error) {
	panic("implement me")
}

// ProposerSettings for mocking
func (m *Validator) ProposerSettings() *proposer.Settings {
	return m.proposerSettings
}

// SetProposerSettings for mocking
func (m *Validator) SetProposerSettings(_ context.Context, settings *proposer.Settings) error {
	m.proposerSettings = settings
	return nil
}

// Graffiti for mocking
func (m *Validator) Graffiti(_ context.Context, _ [fieldparams.BLSPubkeyLength]byte) ([]byte, error) {
	return []byte(m.graffiti), nil
}

// SetGraffiti for mocking
func (m *Validator) SetGraffiti(_ context.Context, _ [fieldparams.BLSPubkeyLength]byte, graffiti []byte) error {
	m.graffiti = string(graffiti)
	return nil
}

// DeleteGraffiti for mocking
func (m *Validator) DeleteGraffiti(_ context.Context, _ [fieldparams.BLSPubkeyLength]byte) error {
	m.graffiti = ""
	return nil
}

func (*Validator) StartEventStream(_ context.Context, _ []string, _ chan<- *event.Event) {
	panic("implement me")
}

func (*Validator) ProcessEvent(event *event.Event) {
	panic("implement me")
}

func (*Validator) EventStreamIsRunning() bool {
	panic("implement me")
}

func (*Validator) HealthTracker() *beacon.NodeHealthTracker {
	panic("implement me")
}

func (*Validator) Host() string {
	panic("implement me")
}

func (*Validator) ChangeHost() {
	panic("implement me")
}
