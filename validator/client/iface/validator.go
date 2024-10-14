package iface

import (
	"context"
	"time"

	"github.com/prysmaticlabs/prysm/v5/api/client/beacon"
	"github.com/prysmaticlabs/prysm/v5/api/client/event"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/proposer"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	validatorpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/validator-client"
	"github.com/prysmaticlabs/prysm/v5/validator/keymanager"
)

// ValidatorRole defines the validator role.
type ValidatorRole int8

const (
	// RoleUnknown means that the role of the validator cannot be determined.
	RoleUnknown ValidatorRole = iota
	// RoleAttester means that the validator should submit an attestation.
	RoleAttester
	// RoleProposer means that the validator should propose a block.
	RoleProposer
	// RoleAggregator means that the validator should submit an aggregation and proof.
	RoleAggregator
	// RoleSyncCommittee means that the validator should submit a sync committee message.
	RoleSyncCommittee
	// RoleSyncCommitteeAggregator means the validator should aggregate sync committee messages and submit a sync committee contribution.
	RoleSyncCommitteeAggregator
	// RolePayloadTimelinessCommittee means the validator should submit a payload attestation message.
	RolePayloadTimelinessCommittee
)

// Validator interface defines the primary methods of a validator client.
type Validator interface {
	Done()
	WaitForChainStart(ctx context.Context) error
	WaitForSync(ctx context.Context) error
	WaitForActivation(ctx context.Context, accountsChangedChan chan [][fieldparams.BLSPubkeyLength]byte) error
	CanonicalHeadSlot(ctx context.Context) (primitives.Slot, error)
	NextSlot() <-chan primitives.Slot
	SlotDeadline(slot primitives.Slot) time.Time
	LogValidatorGainsAndLosses(ctx context.Context, slot primitives.Slot) error
	UpdateDuties(ctx context.Context, slot primitives.Slot) error
	RolesAt(ctx context.Context, slot primitives.Slot) (map[[fieldparams.BLSPubkeyLength]byte][]ValidatorRole, error) // validator pubKey -> roles
	SubmitAttestation(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte)
	ProposeBlock(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte)
	SubmitAggregateAndProof(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte)
	SubmitSyncCommitteeMessage(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte)
	SubmitSignedContributionAndProof(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte)
	SubmitPayloadAttestationMessage(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte)
	LogSubmittedAtts(slot primitives.Slot)
	LogSubmittedSyncCommitteeMessages()
	UpdateDomainDataCaches(ctx context.Context, slot primitives.Slot)
	WaitForKeymanagerInitialization(ctx context.Context) error
	Keymanager() (keymanager.IKeymanager, error)
	HandleKeyReload(ctx context.Context, currentKeys [][fieldparams.BLSPubkeyLength]byte) (bool, error)
	CheckDoppelGanger(ctx context.Context) error
	PushProposerSettings(ctx context.Context, km keymanager.IKeymanager, slot primitives.Slot, forceFullPush bool) error
	SignValidatorRegistrationRequest(ctx context.Context, signer SigningFunc, newValidatorRegistration *ethpb.ValidatorRegistrationV1) (*ethpb.SignedValidatorRegistrationV1, bool /* isCached */, error)
	StartEventStream(ctx context.Context, topics []string, eventsChan chan<- *event.Event)
	EventStreamIsRunning() bool
	ProcessEvent(event *event.Event)
	ProposerSettings() *proposer.Settings
	SetProposerSettings(context.Context, *proposer.Settings) error
	Graffiti(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte) ([]byte, error)
	SetGraffiti(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte, graffiti []byte) error
	DeleteGraffiti(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte) error
	HealthTracker() *beacon.NodeHealthTracker
	Host() string
	ChangeHost()
}

// SigningFunc interface defines a type for the function that signs a message
type SigningFunc func(context.Context, *validatorpb.SignRequest) (bls.Signature, error)
