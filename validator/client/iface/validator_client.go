package iface

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/api/client/event"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/golang/protobuf/ptypes/empty"
)

type BeaconCommitteeSelection struct {
	SelectionProof hexutil.Bytes             `json:"selection_proof"`
	Slot           primitives.Slot           `json:"slot,string"`
	ValidatorIndex primitives.ValidatorIndex `json:"validator_index,string"`
}

type SyncCommitteeSelection struct {
	SelectionProof    hexutil.Bytes             `json:"selection_proof"`
	Slot              primitives.Slot           `json:"slot,string"`
	SubcommitteeIndex primitives.CommitteeIndex `json:"subcommittee_index,string"`
	ValidatorIndex    primitives.ValidatorIndex `json:"validator_index,string"`
}

type ValidatorClient interface {
	// Duties is the pre-GLOAS combined endpoint (GetDuties/GetDutiesV2).
	// This will be eventually replaced by the split duty endpoints below.
	Duties(ctx context.Context, in *ethpb.DutiesRequest) (*ethpb.ValidatorDutiesContainer, error)
	// Split duty endpoints used post-GLOAS.
	AttesterDuties(ctx context.Context, epoch primitives.Epoch, validatorIndices []primitives.ValidatorIndex) (*ethpb.AttesterDutiesResponse, error)
	ProposerDuties(ctx context.Context, epoch primitives.Epoch) (*ethpb.ProposerDutiesResponse, error)
	SyncCommitteeDuties(ctx context.Context, epoch primitives.Epoch, validatorIndices []primitives.ValidatorIndex) (*ethpb.SyncCommitteeDutiesResponse, error)
	PTCDuties(ctx context.Context, epoch primitives.Epoch, validatorIndices []primitives.ValidatorIndex) (*ethpb.PTCDutiesResponse, error)
	DomainData(ctx context.Context, in *ethpb.DomainRequest) (*ethpb.DomainResponse, error)
	WaitForChainStart(ctx context.Context, in *empty.Empty) (*ethpb.ChainStartResponse, error)
	ValidatorIndex(ctx context.Context, in *ethpb.ValidatorIndexRequest) (*ethpb.ValidatorIndexResponse, error)
	ValidatorStatus(ctx context.Context, in *ethpb.ValidatorStatusRequest) (*ethpb.ValidatorStatusResponse, error)
	MultipleValidatorStatus(ctx context.Context, in *ethpb.MultipleValidatorStatusRequest) (*ethpb.MultipleValidatorStatusResponse, error)
	BeaconBlock(ctx context.Context, in *ethpb.BlockRequest) (*ethpb.GenericBeaconBlock, error)
	ProposeBeaconBlock(ctx context.Context, in *ethpb.GenericSignedBeaconBlock) (*ethpb.ProposeResponse, error)
	PrepareBeaconProposer(ctx context.Context, in *ethpb.PrepareBeaconProposerRequest) (*empty.Empty, error)
	AttestationData(ctx context.Context, in *ethpb.AttestationDataRequest) (*ethpb.AttestationData, error)
	ProposeAttestation(ctx context.Context, in *ethpb.Attestation) (*ethpb.AttestResponse, error)
	ProposeAttestationElectra(ctx context.Context, in *ethpb.SingleAttestation) (*ethpb.AttestResponse, error)
	SubmitAggregateSelectionProof(ctx context.Context, in *ethpb.AggregateSelectionRequest, index primitives.ValidatorIndex, committeeLength uint64) (*ethpb.AggregateSelectionResponse, error)
	SubmitAggregateSelectionProofElectra(ctx context.Context, in *ethpb.AggregateSelectionRequest, _ primitives.ValidatorIndex, _ uint64) (*ethpb.AggregateSelectionElectraResponse, error)
	SubmitSignedAggregateSelectionProof(ctx context.Context, in *ethpb.SignedAggregateSubmitRequest) (*ethpb.SignedAggregateSubmitResponse, error)
	SubmitSignedAggregateSelectionProofElectra(ctx context.Context, in *ethpb.SignedAggregateSubmitElectraRequest) (*ethpb.SignedAggregateSubmitResponse, error)
	ProposeExit(ctx context.Context, in *ethpb.SignedVoluntaryExit) (*ethpb.ProposeExitResponse, error)
	SubscribeCommitteeSubnets(ctx context.Context, in *ethpb.CommitteeSubnetsSubscribeRequest) (*empty.Empty, error)
	CheckDoppelGanger(ctx context.Context, in *ethpb.DoppelGangerRequest) (*ethpb.DoppelGangerResponse, error)
	SyncMessageBlockRoot(ctx context.Context, in *empty.Empty) (*ethpb.SyncMessageBlockRootResponse, error)
	SubmitSyncMessage(ctx context.Context, in *ethpb.SyncCommitteeMessage) (*empty.Empty, error)
	SyncSubcommitteeIndex(ctx context.Context, in *ethpb.SyncSubcommitteeIndexRequest) (*ethpb.SyncSubcommitteeIndexResponse, error)
	SyncCommitteeContribution(ctx context.Context, in *ethpb.SyncCommitteeContributionRequest) (*ethpb.SyncCommitteeContribution, error)
	SubmitSignedContributionAndProof(ctx context.Context, in *ethpb.SignedContributionAndProof) (*empty.Empty, error)
	SubmitValidatorRegistrations(ctx context.Context, in *ethpb.SignedValidatorRegistrationsV1) (*empty.Empty, error)
	// SubmitSignedProposerPreferences submits proposer preferences for upcoming
	// proposal slots. This replaces PrepareBeaconProposer and SubmitValidatorRegistrations
	// for Gloas+.
	SubmitSignedProposerPreferences(ctx context.Context, in *ethpb.SubmitSignedProposerPreferencesRequest) (*empty.Empty, error)
	SubmitBuilderPreferences(ctx context.Context, in *ethpb.SubmitBuilderPreferencesRequest) (*empty.Empty, error)
	SubmitSignedExecutionPayloadBid(ctx context.Context, in *ethpb.SignedExecutionPayloadBid) (*empty.Empty, error)
	StartEventStream(ctx context.Context, topics []string, eventsChannel chan<- *event.Event)
	EventStreamIsRunning() bool
	AggregatedSelections(ctx context.Context, selections []BeaconCommitteeSelection) ([]BeaconCommitteeSelection, error)
	AggregatedSyncSelections(ctx context.Context, selections []SyncCommitteeSelection) ([]SyncCommitteeSelection, error)
	Host() string
	EnsureReady(ctx context.Context) bool
	ConnectionGeneration() uint64
	GetExecutionPayloadEnvelope(ctx context.Context, slot primitives.Slot, beaconBlockRoot [32]byte) (*ethpb.ExecutionPayloadEnvelope, error)
	PublishExecutionPayloadEnvelope(ctx context.Context, in *ethpb.SignedExecutionPayloadEnvelope) (*empty.Empty, error)
	PayloadAttestationData(ctx context.Context, slot primitives.Slot) (*ethpb.PayloadAttestationData, error)
	SubmitPayloadAttestation(ctx context.Context, in *ethpb.PayloadAttestationMessage) (*empty.Empty, error)
}
