package grpc_api

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/client"
	eventClient "github.com/prysmaticlabs/prysm/v5/api/client/event"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/validator/client/iface"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

type grpcValidatorClient struct {
	beaconNodeValidatorClient ethpb.BeaconNodeValidatorClient
	isEventStreamRunning      bool
}

func (c *grpcValidatorClient) Duties(ctx context.Context, in *ethpb.DutiesRequest) (*ethpb.DutiesResponse, error) {
	return c.beaconNodeValidatorClient.GetDuties(ctx, in)
}

func (c *grpcValidatorClient) CheckDoppelGanger(ctx context.Context, in *ethpb.DoppelGangerRequest) (*ethpb.DoppelGangerResponse, error) {
	return c.beaconNodeValidatorClient.CheckDoppelGanger(ctx, in)
}

func (c *grpcValidatorClient) DomainData(ctx context.Context, in *ethpb.DomainRequest) (*ethpb.DomainResponse, error) {
	return c.beaconNodeValidatorClient.DomainData(ctx, in)
}

func (c *grpcValidatorClient) AttestationData(ctx context.Context, in *ethpb.AttestationDataRequest) (*ethpb.AttestationData, error) {
	return c.beaconNodeValidatorClient.GetAttestationData(ctx, in)
}

func (c *grpcValidatorClient) BeaconBlock(ctx context.Context, in *ethpb.BlockRequest) (*ethpb.GenericBeaconBlock, error) {
	return c.beaconNodeValidatorClient.GetBeaconBlock(ctx, in)
}

func (c *grpcValidatorClient) FeeRecipientByPubKey(ctx context.Context, in *ethpb.FeeRecipientByPubKeyRequest) (*ethpb.FeeRecipientByPubKeyResponse, error) {
	return c.beaconNodeValidatorClient.GetFeeRecipientByPubKey(ctx, in)
}

func (c *grpcValidatorClient) SyncCommitteeContribution(ctx context.Context, in *ethpb.SyncCommitteeContributionRequest) (*ethpb.SyncCommitteeContribution, error) {
	return c.beaconNodeValidatorClient.GetSyncCommitteeContribution(ctx, in)
}

func (c *grpcValidatorClient) SyncMessageBlockRoot(ctx context.Context, in *empty.Empty) (*ethpb.SyncMessageBlockRootResponse, error) {
	return c.beaconNodeValidatorClient.GetSyncMessageBlockRoot(ctx, in)
}

func (c *grpcValidatorClient) SyncSubcommitteeIndex(ctx context.Context, in *ethpb.SyncSubcommitteeIndexRequest) (*ethpb.SyncSubcommitteeIndexResponse, error) {
	return c.beaconNodeValidatorClient.GetSyncSubcommitteeIndex(ctx, in)
}

func (c *grpcValidatorClient) MultipleValidatorStatus(ctx context.Context, in *ethpb.MultipleValidatorStatusRequest) (*ethpb.MultipleValidatorStatusResponse, error) {
	return c.beaconNodeValidatorClient.MultipleValidatorStatus(ctx, in)
}

func (c *grpcValidatorClient) PrepareBeaconProposer(ctx context.Context, in *ethpb.PrepareBeaconProposerRequest) (*empty.Empty, error) {
	return c.beaconNodeValidatorClient.PrepareBeaconProposer(ctx, in)
}

func (c *grpcValidatorClient) ProposeAttestation(ctx context.Context, in *ethpb.Attestation) (*ethpb.AttestResponse, error) {
	return c.beaconNodeValidatorClient.ProposeAttestation(ctx, in)
}

func (c *grpcValidatorClient) ProposeAttestationElectra(ctx context.Context, in *ethpb.AttestationElectra) (*ethpb.AttestResponse, error) {
	return c.beaconNodeValidatorClient.ProposeAttestationElectra(ctx, in)
}

func (c *grpcValidatorClient) ProposeBeaconBlock(ctx context.Context, in *ethpb.GenericSignedBeaconBlock) (*ethpb.ProposeResponse, error) {
	return c.beaconNodeValidatorClient.ProposeBeaconBlock(ctx, in)
}

func (c *grpcValidatorClient) ProposeExit(ctx context.Context, in *ethpb.SignedVoluntaryExit) (*ethpb.ProposeExitResponse, error) {
	return c.beaconNodeValidatorClient.ProposeExit(ctx, in)
}

func (c *grpcValidatorClient) StreamBlocksAltair(ctx context.Context, in *ethpb.StreamBlocksRequest) (ethpb.BeaconNodeValidator_StreamBlocksAltairClient, error) {
	return c.beaconNodeValidatorClient.StreamBlocksAltair(ctx, in)
}

func (c *grpcValidatorClient) SubmitAggregateSelectionProof(ctx context.Context, in *ethpb.AggregateSelectionRequest, _ primitives.ValidatorIndex, _ uint64) (*ethpb.AggregateSelectionResponse, error) {
	return c.beaconNodeValidatorClient.SubmitAggregateSelectionProof(ctx, in)
}

func (c *grpcValidatorClient) SubmitAggregateSelectionProofElectra(ctx context.Context, in *ethpb.AggregateSelectionRequest, _ primitives.ValidatorIndex, _ uint64) (*ethpb.AggregateSelectionElectraResponse, error) {
	return c.beaconNodeValidatorClient.SubmitAggregateSelectionProofElectra(ctx, in)
}

func (c *grpcValidatorClient) SubmitSignedAggregateSelectionProof(ctx context.Context, in *ethpb.SignedAggregateSubmitRequest) (*ethpb.SignedAggregateSubmitResponse, error) {
	return c.beaconNodeValidatorClient.SubmitSignedAggregateSelectionProof(ctx, in)
}

func (c *grpcValidatorClient) SubmitSignedAggregateSelectionProofElectra(ctx context.Context, in *ethpb.SignedAggregateSubmitElectraRequest) (*ethpb.SignedAggregateSubmitResponse, error) {
	return c.beaconNodeValidatorClient.SubmitSignedAggregateSelectionProofElectra(ctx, in)
}

func (c *grpcValidatorClient) SubmitSignedContributionAndProof(ctx context.Context, in *ethpb.SignedContributionAndProof) (*empty.Empty, error) {
	return c.beaconNodeValidatorClient.SubmitSignedContributionAndProof(ctx, in)
}

func (c *grpcValidatorClient) SubmitSyncMessage(ctx context.Context, in *ethpb.SyncCommitteeMessage) (*empty.Empty, error) {
	return c.beaconNodeValidatorClient.SubmitSyncMessage(ctx, in)
}

func (c *grpcValidatorClient) SubmitValidatorRegistrations(ctx context.Context, in *ethpb.SignedValidatorRegistrationsV1) (*empty.Empty, error) {
	return c.beaconNodeValidatorClient.SubmitValidatorRegistrations(ctx, in)
}

func (c *grpcValidatorClient) SubscribeCommitteeSubnets(ctx context.Context, in *ethpb.CommitteeSubnetsSubscribeRequest, _ []*ethpb.DutiesResponse_Duty) (*empty.Empty, error) {
	return c.beaconNodeValidatorClient.SubscribeCommitteeSubnets(ctx, in)
}

func (c *grpcValidatorClient) ValidatorIndex(ctx context.Context, in *ethpb.ValidatorIndexRequest) (*ethpb.ValidatorIndexResponse, error) {
	return c.beaconNodeValidatorClient.ValidatorIndex(ctx, in)
}

func (c *grpcValidatorClient) ValidatorStatus(ctx context.Context, in *ethpb.ValidatorStatusRequest) (*ethpb.ValidatorStatusResponse, error) {
	return c.beaconNodeValidatorClient.ValidatorStatus(ctx, in)
}

// Deprecated: Do not use.
func (c *grpcValidatorClient) WaitForChainStart(ctx context.Context, in *empty.Empty) (*ethpb.ChainStartResponse, error) {
	stream, err := c.beaconNodeValidatorClient.WaitForChainStart(ctx, in)
	if err != nil {
		return nil, errors.Wrap(
			client.ErrConnectionIssue,
			errors.Wrap(err, "could not setup beacon chain ChainStart streaming client").Error(),
		)
	}

	return stream.Recv()
}

func (c *grpcValidatorClient) AssignValidatorToSubnet(ctx context.Context, in *ethpb.AssignValidatorToSubnetRequest) (*empty.Empty, error) {
	return c.beaconNodeValidatorClient.AssignValidatorToSubnet(ctx, in)
}
func (c *grpcValidatorClient) AggregatedSigAndAggregationBits(
	ctx context.Context,
	in *ethpb.AggregatedSigAndAggregationBitsRequest,
) (*ethpb.AggregatedSigAndAggregationBitsResponse, error) {
	return c.beaconNodeValidatorClient.AggregatedSigAndAggregationBits(ctx, in)
}

func (*grpcValidatorClient) AggregatedSelections(context.Context, []iface.BeaconCommitteeSelection) ([]iface.BeaconCommitteeSelection, error) {
	return nil, iface.ErrNotSupported
}

func (*grpcValidatorClient) AggregatedSyncSelections(context.Context, []iface.SyncCommitteeSelection) ([]iface.SyncCommitteeSelection, error) {
	return nil, iface.ErrNotSupported
}

func (c *grpcValidatorClient) GetPayloadAttestationData(ctx context.Context, in *ethpb.GetPayloadAttestationDataRequest) (*ethpb.PayloadAttestationData, error) {
	return c.beaconNodeValidatorClient.GetPayloadAttestationData(ctx, in)
}

func (c *grpcValidatorClient) SubmitPayloadAttestation(ctx context.Context, in *ethpb.PayloadAttestationMessage) (*empty.Empty, error) {
	return c.beaconNodeValidatorClient.SubmitPayloadAttestation(ctx, in)
}

func NewGrpcValidatorClient(cc grpc.ClientConnInterface) iface.ValidatorClient {
	return &grpcValidatorClient{ethpb.NewBeaconNodeValidatorClient(cc), false}
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
	// TODO(13563): ONLY WORKS WITH HEAD TOPIC RIGHT NOW/ONLY PROVIDES THE SLOT
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

	stream, err := c.beaconNodeValidatorClient.StreamSlots(ctx, &ethpb.StreamSlotsRequest{VerifiedOnly: true})
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
			close(eventsChannel)
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
				Slot: strconv.FormatUint(uint64(res.Slot), 10),
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

func (*grpcValidatorClient) Host() string {
	log.Warn(iface.ErrNotSupported)
	return ""
}

func (*grpcValidatorClient) SetHost(_ string) {
	log.Warn(iface.ErrNotSupported)
}
