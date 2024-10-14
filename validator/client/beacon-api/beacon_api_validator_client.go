package beacon_api

import (
	"context"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/client/event"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/validator/client/iface"
)

type ValidatorClientOpt func(*beaconApiValidatorClient)

type beaconApiValidatorClient struct {
	genesisProvider         GenesisProvider
	dutiesProvider          dutiesProvider
	stateValidatorsProvider StateValidatorsProvider
	jsonRestHandler         JsonRestHandler
	beaconBlockConverter    BeaconBlockConverter
	prysmChainClient        iface.PrysmChainClient
	isEventStreamRunning    bool
}

func NewBeaconApiValidatorClient(jsonRestHandler JsonRestHandler, opts ...ValidatorClientOpt) iface.ValidatorClient {
	c := &beaconApiValidatorClient{
		genesisProvider:         &beaconApiGenesisProvider{jsonRestHandler: jsonRestHandler},
		dutiesProvider:          beaconApiDutiesProvider{jsonRestHandler: jsonRestHandler},
		stateValidatorsProvider: beaconApiStateValidatorsProvider{jsonRestHandler: jsonRestHandler},
		jsonRestHandler:         jsonRestHandler,
		beaconBlockConverter:    beaconApiBeaconBlockConverter{},
		prysmChainClient: prysmChainClient{
			nodeClient:      &beaconApiNodeClient{jsonRestHandler: jsonRestHandler},
			jsonRestHandler: jsonRestHandler,
		},
		isEventStreamRunning: false,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *beaconApiValidatorClient) Duties(ctx context.Context, in *ethpb.DutiesRequest) (*ethpb.DutiesResponse, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.Duties")
	defer span.End()
	return wrapInMetrics[*ethpb.DutiesResponse]("Duties", func() (*ethpb.DutiesResponse, error) {
		return c.duties(ctx, in)
	})
}

func (c *beaconApiValidatorClient) CheckDoppelGanger(ctx context.Context, in *ethpb.DoppelGangerRequest) (*ethpb.DoppelGangerResponse, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.CheckDoppelGanger")
	defer span.End()
	return wrapInMetrics[*ethpb.DoppelGangerResponse]("CheckDoppelGanger", func() (*ethpb.DoppelGangerResponse, error) {
		return c.checkDoppelGanger(ctx, in)
	})
}

func (c *beaconApiValidatorClient) DomainData(ctx context.Context, in *ethpb.DomainRequest) (*ethpb.DomainResponse, error) {
	if len(in.Domain) != 4 {
		return nil, errors.Errorf("invalid domain type: %s", hexutil.Encode(in.Domain))
	}

	ctx, span := trace.StartSpan(ctx, "beacon-api.DomainData")
	defer span.End()

	domainType := bytesutil.ToBytes4(in.Domain)

	return wrapInMetrics[*ethpb.DomainResponse]("DomainData", func() (*ethpb.DomainResponse, error) {
		return c.domainData(ctx, in.Epoch, domainType)
	})
}

func (c *beaconApiValidatorClient) AttestationData(ctx context.Context, in *ethpb.AttestationDataRequest) (*ethpb.AttestationData, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.AttestationData")
	defer span.End()

	return wrapInMetrics[*ethpb.AttestationData]("AttestationData", func() (*ethpb.AttestationData, error) {
		return c.attestationData(ctx, in.Slot, in.CommitteeIndex)
	})
}

func (c *beaconApiValidatorClient) BeaconBlock(ctx context.Context, in *ethpb.BlockRequest) (*ethpb.GenericBeaconBlock, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.BeaconBlock")
	defer span.End()

	return wrapInMetrics[*ethpb.GenericBeaconBlock]("BeaconBlock", func() (*ethpb.GenericBeaconBlock, error) {
		return c.beaconBlock(ctx, in.Slot, in.RandaoReveal, in.Graffiti)
	})
}

func (c *beaconApiValidatorClient) FeeRecipientByPubKey(_ context.Context, _ *ethpb.FeeRecipientByPubKeyRequest) (*ethpb.FeeRecipientByPubKeyResponse, error) {
	return nil, nil
}

func (c *beaconApiValidatorClient) SyncCommitteeContribution(ctx context.Context, in *ethpb.SyncCommitteeContributionRequest) (*ethpb.SyncCommitteeContribution, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.SyncCommitteeContribution")
	defer span.End()

	return wrapInMetrics[*ethpb.SyncCommitteeContribution]("SyncCommitteeContribution", func() (*ethpb.SyncCommitteeContribution, error) {
		return c.syncCommitteeContribution(ctx, in)
	})
}

func (c *beaconApiValidatorClient) SyncMessageBlockRoot(ctx context.Context, _ *empty.Empty) (*ethpb.SyncMessageBlockRootResponse, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.SyncMessageBlockRoot")
	defer span.End()

	return wrapInMetrics[*ethpb.SyncMessageBlockRootResponse]("SyncMessageBlockRoot", func() (*ethpb.SyncMessageBlockRootResponse, error) {
		return c.syncMessageBlockRoot(ctx)
	})
}

func (c *beaconApiValidatorClient) SyncSubcommitteeIndex(ctx context.Context, in *ethpb.SyncSubcommitteeIndexRequest) (*ethpb.SyncSubcommitteeIndexResponse, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.SyncSubcommitteeIndex")
	defer span.End()

	return wrapInMetrics[*ethpb.SyncSubcommitteeIndexResponse]("SyncSubcommitteeIndex", func() (*ethpb.SyncSubcommitteeIndexResponse, error) {
		return c.syncSubcommitteeIndex(ctx, in)
	})
}

func (c *beaconApiValidatorClient) MultipleValidatorStatus(ctx context.Context, in *ethpb.MultipleValidatorStatusRequest) (*ethpb.MultipleValidatorStatusResponse, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.MultipleValidatorStatus")
	defer span.End()

	return wrapInMetrics[*ethpb.MultipleValidatorStatusResponse]("MultipleValidatorStatus", func() (*ethpb.MultipleValidatorStatusResponse, error) {
		return c.multipleValidatorStatus(ctx, in)
	})
}

func (c *beaconApiValidatorClient) PrepareBeaconProposer(ctx context.Context, in *ethpb.PrepareBeaconProposerRequest) (*empty.Empty, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.PrepareBeaconProposer")
	defer span.End()

	return wrapInMetrics[*empty.Empty]("PrepareBeaconProposer", func() (*empty.Empty, error) {
		return new(empty.Empty), c.prepareBeaconProposer(ctx, in.Recipients)
	})
}

func (c *beaconApiValidatorClient) ProposeAttestation(ctx context.Context, in *ethpb.Attestation) (*ethpb.AttestResponse, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.ProposeAttestation")
	defer span.End()

	return wrapInMetrics[*ethpb.AttestResponse]("ProposeAttestation", func() (*ethpb.AttestResponse, error) {
		return c.proposeAttestation(ctx, in)
	})
}

func (c *beaconApiValidatorClient) ProposeAttestationElectra(ctx context.Context, in *ethpb.AttestationElectra) (*ethpb.AttestResponse, error) {
	return nil, errors.New("ProposeAttestationElectra is not implemented")
}

func (c *beaconApiValidatorClient) ProposeBeaconBlock(ctx context.Context, in *ethpb.GenericSignedBeaconBlock) (*ethpb.ProposeResponse, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.ProposeBeaconBlock")
	defer span.End()

	return wrapInMetrics[*ethpb.ProposeResponse]("ProposeBeaconBlock", func() (*ethpb.ProposeResponse, error) {
		return c.proposeBeaconBlock(ctx, in)
	})
}

func (c *beaconApiValidatorClient) ProposeExit(ctx context.Context, in *ethpb.SignedVoluntaryExit) (*ethpb.ProposeExitResponse, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.ProposeExit")
	defer span.End()

	return wrapInMetrics[*ethpb.ProposeExitResponse]("ProposeExit", func() (*ethpb.ProposeExitResponse, error) {
		return c.proposeExit(ctx, in)
	})
}

func (c *beaconApiValidatorClient) StreamBlocksAltair(ctx context.Context, in *ethpb.StreamBlocksRequest) (ethpb.BeaconNodeValidator_StreamBlocksAltairClient, error) {
	return c.streamBlocks(ctx, in, time.Second), nil
}

func (c *beaconApiValidatorClient) SubmitAggregateSelectionProof(ctx context.Context, in *ethpb.AggregateSelectionRequest, index primitives.ValidatorIndex, committeeLength uint64) (*ethpb.AggregateSelectionResponse, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.SubmitAggregateSelectionProof")
	defer span.End()

	return wrapInMetrics[*ethpb.AggregateSelectionResponse]("SubmitAggregateSelectionProof", func() (*ethpb.AggregateSelectionResponse, error) {
		return c.submitAggregateSelectionProof(ctx, in, index, committeeLength)
	})
}

func (c *beaconApiValidatorClient) SubmitAggregateSelectionProofElectra(ctx context.Context, in *ethpb.AggregateSelectionRequest, index primitives.ValidatorIndex, committeeLength uint64) (*ethpb.AggregateSelectionElectraResponse, error) {
	return nil, errors.New("SubmitAggregateSelectionProofElectra is not implemented")
}

func (c *beaconApiValidatorClient) SubmitSignedAggregateSelectionProof(ctx context.Context, in *ethpb.SignedAggregateSubmitRequest) (*ethpb.SignedAggregateSubmitResponse, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.SubmitSignedAggregateSelectionProof")
	defer span.End()

	return wrapInMetrics[*ethpb.SignedAggregateSubmitResponse]("SubmitSignedAggregateSelectionProof", func() (*ethpb.SignedAggregateSubmitResponse, error) {
		return c.submitSignedAggregateSelectionProof(ctx, in)
	})
}

func (c *beaconApiValidatorClient) SubmitSignedAggregateSelectionProofElectra(ctx context.Context, in *ethpb.SignedAggregateSubmitElectraRequest) (*ethpb.SignedAggregateSubmitResponse, error) {
	return nil, errors.New("SubmitSignedAggregateSelectionProofElectra is not implemented")
}

func (c *beaconApiValidatorClient) SubmitSignedContributionAndProof(ctx context.Context, in *ethpb.SignedContributionAndProof) (*empty.Empty, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.SubmitSignedContributionAndProof")
	defer span.End()

	return wrapInMetrics[*empty.Empty]("SubmitSignedContributionAndProof", func() (*empty.Empty, error) {
		return new(empty.Empty), c.submitSignedContributionAndProof(ctx, in)
	})
}

func (c *beaconApiValidatorClient) SubmitSyncMessage(ctx context.Context, in *ethpb.SyncCommitteeMessage) (*empty.Empty, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.SubmitSyncMessage")
	defer span.End()

	return wrapInMetrics[*empty.Empty]("SubmitSyncMessage", func() (*empty.Empty, error) {
		return new(empty.Empty), c.submitSyncMessage(ctx, in)
	})
}

func (c *beaconApiValidatorClient) SubmitValidatorRegistrations(ctx context.Context, in *ethpb.SignedValidatorRegistrationsV1) (*empty.Empty, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.SubmitValidatorRegistrations")
	defer span.End()

	return wrapInMetrics[*empty.Empty]("SubmitValidatorRegistrations", func() (*empty.Empty, error) {
		return new(empty.Empty), c.submitValidatorRegistrations(ctx, in.Messages)
	})
}

func (c *beaconApiValidatorClient) SubscribeCommitteeSubnets(ctx context.Context, in *ethpb.CommitteeSubnetsSubscribeRequest, duties []*ethpb.DutiesResponse_Duty) (*empty.Empty, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.SubscribeCommitteeSubnets")
	defer span.End()

	return wrapInMetrics[*empty.Empty]("SubscribeCommitteeSubnets", func() (*empty.Empty, error) {
		return new(empty.Empty), c.subscribeCommitteeSubnets(ctx, in, duties)
	})
}

func (c *beaconApiValidatorClient) ValidatorIndex(ctx context.Context, in *ethpb.ValidatorIndexRequest) (*ethpb.ValidatorIndexResponse, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.ValidatorIndex")
	defer span.End()

	return wrapInMetrics[*ethpb.ValidatorIndexResponse]("ValidatorIndex", func() (*ethpb.ValidatorIndexResponse, error) {
		return c.validatorIndex(ctx, in)
	})
}

func (c *beaconApiValidatorClient) ValidatorStatus(ctx context.Context, in *ethpb.ValidatorStatusRequest) (*ethpb.ValidatorStatusResponse, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.ValidatorStatus")
	defer span.End()

	return c.validatorStatus(ctx, in)
}

// Deprecated: Do not use.
func (c *beaconApiValidatorClient) WaitForChainStart(ctx context.Context, _ *empty.Empty) (*ethpb.ChainStartResponse, error) {
	return c.waitForChainStart(ctx)
}

func (c *beaconApiValidatorClient) StartEventStream(ctx context.Context, topics []string, eventsChannel chan<- *event.Event) {
	client := &http.Client{} // event stream should not be subject to the same settings as other api calls, so we won't use c.jsonRestHandler.HttpClient()
	eventStream, err := event.NewEventStream(ctx, client, c.jsonRestHandler.Host(), topics)
	if err != nil {
		eventsChannel <- &event.Event{
			EventType: event.EventError,
			Data:      []byte(errors.Wrap(err, "failed to start event stream").Error()),
		}
		return
	}
	c.isEventStreamRunning = true
	eventStream.Subscribe(eventsChannel)
	c.isEventStreamRunning = false
}

func (c *beaconApiValidatorClient) EventStreamIsRunning() bool {
	return c.isEventStreamRunning
}

func (c *beaconApiValidatorClient) AggregatedSelections(ctx context.Context, selections []iface.BeaconCommitteeSelection) ([]iface.BeaconCommitteeSelection, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.AggregatedSelections")
	defer span.End()

	return wrapInMetrics[[]iface.BeaconCommitteeSelection]("AggregatedSelections", func() ([]iface.BeaconCommitteeSelection, error) {
		return c.aggregatedSelection(ctx, selections)
	})
}

func (c *beaconApiValidatorClient) AggregatedSyncSelections(ctx context.Context, selections []iface.SyncCommitteeSelection) ([]iface.SyncCommitteeSelection, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.AggregatedSyncSelections")
	defer span.End()

	return wrapInMetrics[[]iface.SyncCommitteeSelection]("AggregatedSyncSelections", func() ([]iface.SyncCommitteeSelection, error) {
		return c.aggregatedSyncSelections(ctx, selections)
	})
}

func (c *beaconApiValidatorClient) GetPayloadAttestationData(ctx context.Context, in *ethpb.GetPayloadAttestationDataRequest) (*ethpb.PayloadAttestationData, error) {
	return nil, errors.New("not implemented")
}

func (c *beaconApiValidatorClient) SubmitPayloadAttestation(ctx context.Context, in *ethpb.PayloadAttestationMessage) (*empty.Empty, error) {
	return nil, errors.New("not implemented")
}

func wrapInMetrics[Resp any](action string, f func() (Resp, error)) (Resp, error) {
	now := time.Now()
	resp, err := f()
	httpActionCount.WithLabelValues(action).Inc()
	if err == nil {
		httpActionLatency.WithLabelValues(action).Observe(time.Since(now).Seconds())
	} else {
		failedHTTPActionCount.WithLabelValues(action).Inc()
	}
	return resp, err
}

func (c *beaconApiValidatorClient) Host() string {
	return c.jsonRestHandler.Host()
}

func (c *beaconApiValidatorClient) SetHost(host string) {
	c.jsonRestHandler.SetHost(host)
}
