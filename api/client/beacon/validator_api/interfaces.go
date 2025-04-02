package validator_api

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/prysmaticlabs/prysm/v5/api/client/beacon"
	"github.com/prysmaticlabs/prysm/v5/api/client/event"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

type Client interface {
	Duties(ctx context.Context, in *ethpb.DutiesRequest) (*ethpb.ValidatorDutiesContainer, error)
	DomainData(ctx context.Context, in *ethpb.DomainRequest) (*ethpb.DomainResponse, error)
	WaitForChainStart(ctx context.Context, in *empty.Empty) (*ethpb.ChainStartResponse, error)
	ValidatorIndex(ctx context.Context, in *ethpb.ValidatorIndexRequest) (*ethpb.ValidatorIndexResponse, error)
	ValidatorStatus(ctx context.Context, in *ethpb.ValidatorStatusRequest) (*ethpb.ValidatorStatusResponse, error)
	MultipleValidatorStatus(ctx context.Context, in *ethpb.MultipleValidatorStatusRequest) (*ethpb.MultipleValidatorStatusResponse, error)
	BeaconBlock(ctx context.Context, in *ethpb.BlockRequest) (*ethpb.GenericBeaconBlock, error)
	ProposeBeaconBlock(ctx context.Context, in *ethpb.GenericSignedBeaconBlock) (*ethpb.ProposeResponse, error)
	PrepareBeaconProposer(ctx context.Context, in *ethpb.PrepareBeaconProposerRequest) (*empty.Empty, error)
	FeeRecipientByPubKey(ctx context.Context, in *ethpb.FeeRecipientByPubKeyRequest) (*ethpb.FeeRecipientByPubKeyResponse, error)
	AttestationData(ctx context.Context, in *ethpb.AttestationDataRequest) (*ethpb.AttestationData, error)
	ProposeAttestation(ctx context.Context, in *ethpb.Attestation) (*ethpb.AttestResponse, error)
	ProposeAttestationElectra(ctx context.Context, in *ethpb.SingleAttestation) (*ethpb.AttestResponse, error)
	SubmitAggregateSelectionProof(ctx context.Context, in *ethpb.AggregateSelectionRequest, index primitives.ValidatorIndex, committeeLength uint64) (*ethpb.AggregateSelectionResponse, error)
	SubmitAggregateSelectionProofElectra(ctx context.Context, in *ethpb.AggregateSelectionRequest, _ primitives.ValidatorIndex, _ uint64) (*ethpb.AggregateSelectionElectraResponse, error)
	SubmitSignedAggregateSelectionProof(ctx context.Context, in *ethpb.SignedAggregateSubmitRequest) (*ethpb.SignedAggregateSubmitResponse, error)
	SubmitSignedAggregateSelectionProofElectra(ctx context.Context, in *ethpb.SignedAggregateSubmitElectraRequest) (*ethpb.SignedAggregateSubmitResponse, error)
	ProposeExit(ctx context.Context, in *ethpb.SignedVoluntaryExit) (*ethpb.ProposeExitResponse, error)
	SubscribeCommitteeSubnets(ctx context.Context, in *ethpb.CommitteeSubnetsSubscribeRequest, duties []*ethpb.ValidatorDuty) (*empty.Empty, error)
	CheckDoppelGanger(ctx context.Context, in *ethpb.DoppelGangerRequest) (*ethpb.DoppelGangerResponse, error)
	SyncMessageBlockRoot(ctx context.Context, in *empty.Empty) (*ethpb.SyncMessageBlockRootResponse, error)
	SubmitSyncMessage(ctx context.Context, in *ethpb.SyncCommitteeMessage) (*empty.Empty, error)
	SyncSubcommitteeIndex(ctx context.Context, in *ethpb.SyncSubcommitteeIndexRequest) (*ethpb.SyncSubcommitteeIndexResponse, error)
	SyncCommitteeContribution(ctx context.Context, in *ethpb.SyncCommitteeContributionRequest) (*ethpb.SyncCommitteeContribution, error)
	SubmitSignedContributionAndProof(ctx context.Context, in *ethpb.SignedContributionAndProof) (*empty.Empty, error)
	SubmitValidatorRegistrations(ctx context.Context, in *ethpb.SignedValidatorRegistrationsV1) (*empty.Empty, error)
	StartEventStream(ctx context.Context, topics []string, eventsChannel chan<- *event.Event)
	EventStreamIsRunning() bool
	AggregatedSelections(ctx context.Context, selections []beacon.BeaconCommitteeSelection) ([]beacon.BeaconCommitteeSelection, error)
	AggregatedSyncSelections(ctx context.Context, selections []beacon.SyncCommitteeSelection) ([]beacon.SyncCommitteeSelection, error)
	Host() string
	SetHost(host string)
}
