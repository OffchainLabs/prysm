package grpc_api

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api/client"
	eventClient "github.com/OffchainLabs/prysm/v7/api/client/event"
	"github.com/OffchainLabs/prysm/v7/api/fallback"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	validatorHelpers "github.com/OffchainLabs/prysm/v7/validator/helpers"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type grpcValidatorClient struct {
	*grpcClientManager[ethpb.BeaconNodeValidatorClient]
	nodeClient           *grpcNodeClient
	isEventStreamRunning bool
}

func (c *grpcValidatorClient) Duties(ctx context.Context, in *ethpb.DutiesRequest) (*ethpb.ValidatorDutiesContainer, error) {
	if features.Get().DisableDutiesV2 {
		return c.getDuties(ctx, in)
	}
	dutiesResponse, err := c.getClient().GetDutiesV2(ctx, in)
	if err != nil {
		if status.Code(err) == codes.Unimplemented {
			log.Warn("GetDutiesV2 returned status code unavailable, falling back to GetDuties")
			return c.getDuties(ctx, in)
		}
		return nil, errors.Wrap(
			client.ErrConnectionIssue,
			errors.Wrap(err, "getDutiesV2").Error(),
		)
	}
	return toValidatorDutiesContainerV2(dutiesResponse)
}

// getDuties is calling the v1 of get duties
func (c *grpcValidatorClient) getDuties(ctx context.Context, in *ethpb.DutiesRequest) (*ethpb.ValidatorDutiesContainer, error) {
	dutiesResponse, err := c.getClient().GetDuties(ctx, in)
	if err != nil {
		return nil, errors.Wrap(
			client.ErrConnectionIssue,
			errors.Wrap(err, "getDuties").Error(),
		)
	}
	return toValidatorDutiesContainer(dutiesResponse)
}

func toValidatorDutiesContainer(dutiesResponse *ethpb.DutiesResponse) (*ethpb.ValidatorDutiesContainer, error) {
	currentDuties := make([]*ethpb.ValidatorDuty, len(dutiesResponse.CurrentEpochDuties))
	for i, cd := range dutiesResponse.CurrentEpochDuties {
		duty, err := toValidatorDuty(cd)
		if err != nil {
			return nil, err
		}
		currentDuties[i] = duty
	}
	nextDuties := make([]*ethpb.ValidatorDuty, len(dutiesResponse.NextEpochDuties))
	for i, nd := range dutiesResponse.NextEpochDuties {
		duty, err := toValidatorDuty(nd)
		if err != nil {
			return nil, err
		}
		nextDuties[i] = duty
	}
	return &ethpb.ValidatorDutiesContainer{
		PrevDependentRoot:  dutiesResponse.PreviousDutyDependentRoot,
		CurrDependentRoot:  dutiesResponse.CurrentDutyDependentRoot,
		CurrentEpochDuties: currentDuties,
		NextEpochDuties:    nextDuties,
	}, nil
}

func toValidatorDuty(duty *ethpb.DutiesResponse_Duty) (*ethpb.ValidatorDuty, error) {
	var valIndexInCommittee uint64
	// valIndexInCommittee will be 0 in case we don't get a match. This is a potential false positive,
	// however it's an impossible condition because every validator must be assigned to a committee.
	for cIndex, vIndex := range duty.Committee {
		if vIndex == duty.ValidatorIndex {
			valIndexInCommittee = uint64(cIndex)
			break
		}
	}
	return &ethpb.ValidatorDuty{
		CommitteeLength:         uint64(len(duty.Committee)),
		CommitteeIndex:          duty.CommitteeIndex,
		CommitteesAtSlot:        duty.CommitteesAtSlot, // GRPC doesn't use this value though
		ValidatorCommitteeIndex: valIndexInCommittee,
		AttesterSlot:            duty.AttesterSlot,
		ProposerSlots:           duty.ProposerSlots,
		PublicKey:               bytesutil.SafeCopyBytes(duty.PublicKey),
		Status:                  duty.Status,
		ValidatorIndex:          duty.ValidatorIndex,
		IsSyncCommittee:         duty.IsSyncCommittee,
	}, nil
}

func toValidatorDutiesContainerV2(dutiesResponse *ethpb.DutiesV2Response) (*ethpb.ValidatorDutiesContainer, error) {
	currentDuties := make([]*ethpb.ValidatorDuty, len(dutiesResponse.CurrentEpochDuties))
	for i, cd := range dutiesResponse.CurrentEpochDuties {
		duty, err := toValidatorDutyV2(cd)
		if err != nil {
			return nil, err
		}
		currentDuties[i] = duty
	}
	nextDuties := make([]*ethpb.ValidatorDuty, len(dutiesResponse.NextEpochDuties))
	for i, nd := range dutiesResponse.NextEpochDuties {
		duty, err := toValidatorDutyV2(nd)
		if err != nil {
			return nil, err
		}
		nextDuties[i] = duty
	}
	return &ethpb.ValidatorDutiesContainer{
		PrevDependentRoot:  dutiesResponse.PreviousDutyDependentRoot,
		CurrDependentRoot:  dutiesResponse.CurrentDutyDependentRoot,
		CurrentEpochDuties: currentDuties,
		NextEpochDuties:    nextDuties,
	}, nil
}

func toValidatorDutyV2(duty *ethpb.DutiesV2Response_Duty) (*ethpb.ValidatorDuty, error) {
	return &ethpb.ValidatorDuty{
		CommitteeLength:         duty.CommitteeLength,
		CommitteeIndex:          duty.CommitteeIndex,
		CommitteesAtSlot:        duty.CommitteesAtSlot, // GRPC doesn't use this value though
		ValidatorCommitteeIndex: duty.ValidatorCommitteeIndex,
		AttesterSlot:            duty.AttesterSlot,
		ProposerSlots:           duty.ProposerSlots,
		PublicKey:               bytesutil.SafeCopyBytes(duty.PublicKey),
		Status:                  duty.Status,
		ValidatorIndex:          duty.ValidatorIndex,
		IsSyncCommittee:         duty.IsSyncCommittee,
		PtcSlots:                duty.PtcSlots,
	}, nil
}

func (c *grpcValidatorClient) CheckDoppelGanger(ctx context.Context, in *ethpb.DoppelGangerRequest) (*ethpb.DoppelGangerResponse, error) {
	return c.getClient().CheckDoppelGanger(ctx, in)
}

func (c *grpcValidatorClient) DomainData(ctx context.Context, in *ethpb.DomainRequest) (*ethpb.DomainResponse, error) {
	return c.getClient().DomainData(ctx, in)
}

func (c *grpcValidatorClient) AttestationData(ctx context.Context, in *ethpb.AttestationDataRequest) (*ethpb.AttestationData, error) {
	return c.getClient().GetAttestationData(ctx, in)
}

func (c *grpcValidatorClient) BeaconBlock(ctx context.Context, in *ethpb.BlockRequest) (*ethpb.GenericBeaconBlock, error) {
	return c.getClient().GetBeaconBlock(ctx, in)
}

func (c *grpcValidatorClient) FeeRecipientByPubKey(ctx context.Context, in *ethpb.FeeRecipientByPubKeyRequest) (*ethpb.FeeRecipientByPubKeyResponse, error) {
	return c.getClient().GetFeeRecipientByPubKey(ctx, in)
}

func (c *grpcValidatorClient) SyncCommitteeContribution(ctx context.Context, in *ethpb.SyncCommitteeContributionRequest) (*ethpb.SyncCommitteeContribution, error) {
	return c.getClient().GetSyncCommitteeContribution(ctx, in)
}

func (c *grpcValidatorClient) SyncMessageBlockRoot(ctx context.Context, in *empty.Empty) (*ethpb.SyncMessageBlockRootResponse, error) {
	return c.getClient().GetSyncMessageBlockRoot(ctx, in)
}

func (c *grpcValidatorClient) SyncSubcommitteeIndex(ctx context.Context, in *ethpb.SyncSubcommitteeIndexRequest) (*ethpb.SyncSubcommitteeIndexResponse, error) {
	return c.getClient().GetSyncSubcommitteeIndex(ctx, in)
}

func (c *grpcValidatorClient) MultipleValidatorStatus(ctx context.Context, in *ethpb.MultipleValidatorStatusRequest) (*ethpb.MultipleValidatorStatusResponse, error) {
	return c.getClient().MultipleValidatorStatus(ctx, in)
}

func (c *grpcValidatorClient) PrepareBeaconProposer(ctx context.Context, in *ethpb.PrepareBeaconProposerRequest) (*empty.Empty, error) {
	return c.getClient().PrepareBeaconProposer(ctx, in)
}

func (c *grpcValidatorClient) ProposeAttestation(ctx context.Context, in *ethpb.Attestation) (*ethpb.AttestResponse, error) {
	return c.getClient().ProposeAttestation(ctx, in)
}

func (c *grpcValidatorClient) ProposeAttestationElectra(ctx context.Context, in *ethpb.SingleAttestation) (*ethpb.AttestResponse, error) {
	return c.getClient().ProposeAttestationElectra(ctx, in)
}

func (c *grpcValidatorClient) ProposeBeaconBlock(ctx context.Context, in *ethpb.GenericSignedBeaconBlock) (*ethpb.ProposeResponse, error) {
	return c.getClient().ProposeBeaconBlock(ctx, in)
}

func (c *grpcValidatorClient) ProposeExit(ctx context.Context, in *ethpb.SignedVoluntaryExit) (*ethpb.ProposeExitResponse, error) {
	return c.getClient().ProposeExit(ctx, in)
}

func (c *grpcValidatorClient) StreamBlocksAltair(ctx context.Context, in *ethpb.StreamBlocksRequest) (ethpb.BeaconNodeValidator_StreamBlocksAltairClient, error) {
	return c.getClient().StreamBlocksAltair(ctx, in)
}

func (c *grpcValidatorClient) SubmitAggregateSelectionProof(ctx context.Context, in *ethpb.AggregateSelectionRequest, _ primitives.ValidatorIndex, _ uint64) (*ethpb.AggregateSelectionResponse, error) {
	return c.getClient().SubmitAggregateSelectionProof(ctx, in)
}

func (c *grpcValidatorClient) SubmitAggregateSelectionProofElectra(ctx context.Context, in *ethpb.AggregateSelectionRequest, _ primitives.ValidatorIndex, _ uint64) (*ethpb.AggregateSelectionElectraResponse, error) {
	return c.getClient().SubmitAggregateSelectionProofElectra(ctx, in)
}

func (c *grpcValidatorClient) SubmitSignedAggregateSelectionProof(ctx context.Context, in *ethpb.SignedAggregateSubmitRequest) (*ethpb.SignedAggregateSubmitResponse, error) {
	return c.getClient().SubmitSignedAggregateSelectionProof(ctx, in)
}

func (c *grpcValidatorClient) SubmitSignedAggregateSelectionProofElectra(ctx context.Context, in *ethpb.SignedAggregateSubmitElectraRequest) (*ethpb.SignedAggregateSubmitResponse, error) {
	return c.getClient().SubmitSignedAggregateSelectionProofElectra(ctx, in)
}

func (c *grpcValidatorClient) SubmitSignedContributionAndProof(ctx context.Context, in *ethpb.SignedContributionAndProof) (*empty.Empty, error) {
	return c.getClient().SubmitSignedContributionAndProof(ctx, in)
}

func (c *grpcValidatorClient) SubmitSyncMessage(ctx context.Context, in *ethpb.SyncCommitteeMessage) (*empty.Empty, error) {
	return c.getClient().SubmitSyncMessage(ctx, in)
}

func (c *grpcValidatorClient) SubmitValidatorRegistrations(ctx context.Context, in *ethpb.SignedValidatorRegistrationsV1) (*empty.Empty, error) {
	return c.getClient().SubmitValidatorRegistrations(ctx, in)
}

func (c *grpcValidatorClient) SubscribeCommitteeSubnets(ctx context.Context, in *ethpb.CommitteeSubnetsSubscribeRequest, _ []*ethpb.ValidatorDuty) (*empty.Empty, error) {
	return c.getClient().SubscribeCommitteeSubnets(ctx, in)
}

func (c *grpcValidatorClient) ValidatorIndex(ctx context.Context, in *ethpb.ValidatorIndexRequest) (*ethpb.ValidatorIndexResponse, error) {
	return c.getClient().ValidatorIndex(ctx, in)
}

func (c *grpcValidatorClient) ValidatorStatus(ctx context.Context, in *ethpb.ValidatorStatusRequest) (*ethpb.ValidatorStatusResponse, error) {
	return c.getClient().ValidatorStatus(ctx, in)
}

// Deprecated: Do not use.
func (c *grpcValidatorClient) WaitForChainStart(ctx context.Context, in *empty.Empty) (*ethpb.ChainStartResponse, error) {
	stream, err := c.getClient().WaitForChainStart(ctx, in)
	if err != nil {
		return nil, errors.Wrap(
			client.ErrConnectionIssue,
			errors.Wrap(err, "could not setup beacon chain ChainStart streaming client").Error(),
		)
	}

	return stream.Recv()
}

func (c *grpcValidatorClient) AssignValidatorToSubnet(ctx context.Context, in *ethpb.AssignValidatorToSubnetRequest) (*empty.Empty, error) {
	return c.getClient().AssignValidatorToSubnet(ctx, in)
}
func (c *grpcValidatorClient) AggregatedSigAndAggregationBits(
	ctx context.Context,
	in *ethpb.AggregatedSigAndAggregationBitsRequest,
) (*ethpb.AggregatedSigAndAggregationBitsResponse, error) {
	return c.getClient().AggregatedSigAndAggregationBits(ctx, in)
}

func (*grpcValidatorClient) AggregatedSelections(context.Context, []iface.BeaconCommitteeSelection) ([]iface.BeaconCommitteeSelection, error) {
	return nil, iface.ErrNotSupported
}

func (*grpcValidatorClient) AggregatedSyncSelections(context.Context, []iface.SyncCommitteeSelection) ([]iface.SyncCommitteeSelection, error) {
	return nil, iface.ErrNotSupported
}

// NewGrpcValidatorClient creates a new gRPC validator client that supports
// dynamic connection switching via the NodeConnection's GrpcConnectionProvider.
func NewGrpcValidatorClient(conn validatorHelpers.NodeConnection) iface.ValidatorClient {
	return &grpcValidatorClient{
		grpcClientManager: newGrpcClientManager(conn, ethpb.NewBeaconNodeValidatorClient),
		nodeClient: &grpcNodeClient{
			grpcClientManager: newGrpcClientManager(conn, ethpb.NewNodeClient),
		},
	}
}

func (c *grpcValidatorClient) StartEventStream(ctx context.Context, topics []string, eventsChannel chan<- *eventClient.Event) {
	ctx, span := trace.StartSpan(ctx, "validator.gRPCClient.StartEventStream")
	defer span.End()
	if len(topics) == 0 {
		eventsChannel <- &eventClient.Event{
			EventType: eventClient.EventError,
			Data:      []byte(errors.New("no topics were added").Error()),
		}
		return
	}
	// TODO(13563): ONLY WORKS WITH HEAD TOPIC.
	containsHead := false
	for i := range topics {
		if topics[i] == eventClient.EventHead {
			containsHead = true
		}
	}
	if !containsHead {
		eventsChannel <- &eventClient.Event{
			EventType: eventClient.EventConnectionError,
			Data:      []byte(errors.Wrap(client.ErrConnectionIssue, "gRPC only supports the head topic, and head topic was not passed").Error()),
		}
	}
	if containsHead && len(topics) > 1 {
		log.Warn("gRPC only supports the head topic, other topics will be ignored")
	}

	stream, err := c.getClient().StreamSlots(ctx, &ethpb.StreamSlotsRequest{VerifiedOnly: true})
	if err != nil {
		eventsChannel <- &eventClient.Event{
			EventType: eventClient.EventConnectionError,
			Data:      []byte(errors.Wrap(client.ErrConnectionIssue, err.Error()).Error()),
		}
		return
	}
	c.isEventStreamRunning = true
	for {
		select {
		case <-ctx.Done():
			log.Info("Context canceled, stopping event stream")
			c.isEventStreamRunning = false
			return
		default:
			if ctx.Err() != nil {
				c.isEventStreamRunning = false
				if errors.Is(ctx.Err(), context.Canceled) {
					eventsChannel <- &eventClient.Event{
						EventType: eventClient.EventConnectionError,
						Data:      []byte(errors.Wrap(client.ErrConnectionIssue, ctx.Err().Error()).Error()),
					}
					return
				}
				eventsChannel <- &eventClient.Event{
					EventType: eventClient.EventError,
					Data:      []byte(ctx.Err().Error()),
				}
				return
			}
			res, err := stream.Recv()
			if err != nil {
				c.isEventStreamRunning = false
				eventsChannel <- &eventClient.Event{
					EventType: eventClient.EventConnectionError,
					Data:      []byte(errors.Wrap(client.ErrConnectionIssue, err.Error()).Error()),
				}
				return
			}
			if res == nil {
				continue
			}
			b, err := json.Marshal(structs.HeadEvent{
				Slot:                      strconv.FormatUint(uint64(res.Slot), 10),
				PreviousDutyDependentRoot: hexutil.Encode(res.PreviousDutyDependentRoot),
				CurrentDutyDependentRoot:  hexutil.Encode(res.CurrentDutyDependentRoot),
			})
			if err != nil {
				eventsChannel <- &eventClient.Event{
					EventType: eventClient.EventError,
					Data:      []byte(errors.Wrap(err, "failed to marshal Head Event").Error()),
				}
			}
			eventsChannel <- &eventClient.Event{
				EventType: eventClient.EventHead,
				Data:      b,
			}
		}
	}
}

func (c *grpcValidatorClient) EventStreamIsRunning() bool {
	return c.isEventStreamRunning
}

func (c *grpcValidatorClient) Host() string {
	return c.grpcClientManager.conn.GetGrpcConnectionProvider().CurrentHost()
}

func (c *grpcValidatorClient) EnsureReady(ctx context.Context) bool {
	provider := c.grpcClientManager.conn.GetGrpcConnectionProvider()
	return fallback.EnsureReady(ctx, provider, c.nodeClient)
}

// Gloas Fork Methods
func (c *grpcValidatorClient) GetExecutionPayloadEnvelope(ctx context.Context, slot primitives.Slot) (*ethpb.ExecutionPayloadEnvelope, error) {
	req := &ethpb.ExecutionPayloadEnvelopeRequest{
		Slot: slot,
	}
	resp, err := c.getClient().GetExecutionPayloadEnvelope(ctx, req)
	if err != nil {
		return nil, errors.Wrap(
			client.ErrConnectionIssue,
			errors.Wrap(err, "GetExecutionPayloadEnvelope").Error(),
		)
	}
	return resp.Envelope, nil
}

func (c *grpcValidatorClient) PublishExecutionPayloadEnvelope(ctx context.Context, in *ethpb.SignedExecutionPayloadEnvelope) (*empty.Empty, error) {
	return c.getClient().PublishExecutionPayloadEnvelope(ctx, in)
}
