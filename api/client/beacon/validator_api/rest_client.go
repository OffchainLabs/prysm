package validator_api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/client"
	"github.com/prysmaticlabs/prysm/v5/api/client/apiutil"
	"github.com/prysmaticlabs/prysm/v5/api/client/beacon/prysm_api"
	"github.com/prysmaticlabs/prysm/v5/api/client/beacon/shared_providers"
	"github.com/prysmaticlabs/prysm/v5/api/client/event"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	"github.com/prysmaticlabs/prysm/v5/network/httputil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

type ValidatorClientOpt func(*beaconApiValidatorClient)

type beaconApiValidatorClient struct {
	genesisProvider         shared_providers.Genesis
	dutiesProvider          shared_providers.Duties
	stateValidatorsProvider shared_providers.StateValidators
	jsonRestHandler         client.JsonRestHandler
	beaconBlockConverter    BeaconBlockConverter
	prysmChainClient        prysm_api.Client
	isEventStreamRunning    bool
}

func (c *beaconApiValidatorClient) waitForChainStart(ctx context.Context) (*ethpb.ChainStartResponse, error) {
	genesis, err := c.genesisProvider.Genesis(ctx)

	for err != nil {
		jsonErr := &httputil.DefaultJsonError{}
		httpNotFound := errors.As(err, &jsonErr) && jsonErr.Code == http.StatusNotFound
		if !httpNotFound {
			return nil, errors.Wrap(err, "failed to get genesis data")
		}

		// Error 404 means that the chain genesis info is not yet known, so we query it every second until it's ready
		select {
		case <-time.After(time.Second):
			genesis, err = c.genesisProvider.Genesis(ctx)
		case <-ctx.Done():
			return nil, errors.New("context canceled")
		}
	}

	genesisTime, err := strconv.ParseUint(genesis.GenesisTime, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse genesis time: %s", genesis.GenesisTime)
	}

	if !apiutil.ValidRoot(genesis.GenesisValidatorsRoot) {
		return nil, errors.Errorf("invalid genesis validators root: %s", genesis.GenesisValidatorsRoot)
	}

	genesisValidatorRoot, err := hexutil.Decode(genesis.GenesisValidatorsRoot)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode genesis validators root")
	}

	chainStartResponse := &ethpb.ChainStartResponse{
		Started:               true,
		GenesisTime:           genesisTime,
		GenesisValidatorsRoot: genesisValidatorRoot,
	}

	return chainStartResponse, nil
}

func (c *beaconApiValidatorClient) Duties(ctx context.Context, in *ethpb.DutiesRequest) (*ethpb.ValidatorDutiesContainer, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.Duties")
	defer span.End()
	return wrapInMetrics[*ethpb.ValidatorDutiesContainer]("Duties", func() (*ethpb.ValidatorDutiesContainer, error) {
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

func (c *beaconApiValidatorClient) ProposeAttestationElectra(ctx context.Context, in *ethpb.SingleAttestation) (*ethpb.AttestResponse, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.ProposeAttestationElectra")
	defer span.End()

	return wrapInMetrics[*ethpb.AttestResponse]("ProposeAttestationElectra", func() (*ethpb.AttestResponse, error) {
		return c.proposeAttestationElectra(ctx, in)
	})
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
	ctx, span := trace.StartSpan(ctx, "beacon-api.SubmitAggregateSelectionProofElectra")
	defer span.End()

	return wrapInMetrics[*ethpb.AggregateSelectionElectraResponse]("SubmitAggregateSelectionProofElectra", func() (*ethpb.AggregateSelectionElectraResponse, error) {
		return c.submitAggregateSelectionProofElectra(ctx, in, index, committeeLength)
	})
}

func (c *beaconApiValidatorClient) SubmitSignedAggregateSelectionProof(ctx context.Context, in *ethpb.SignedAggregateSubmitRequest) (*ethpb.SignedAggregateSubmitResponse, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.SubmitSignedAggregateSelectionProof")
	defer span.End()

	return wrapInMetrics[*ethpb.SignedAggregateSubmitResponse]("SubmitSignedAggregateSelectionProof", func() (*ethpb.SignedAggregateSubmitResponse, error) {
		return c.submitSignedAggregateSelectionProof(ctx, in)
	})
}

func (c *beaconApiValidatorClient) SubmitSignedAggregateSelectionProofElectra(ctx context.Context, in *ethpb.SignedAggregateSubmitElectraRequest) (*ethpb.SignedAggregateSubmitResponse, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.SubmitSignedAggregateSelectionProofElectra")
	defer span.End()

	return wrapInMetrics[*ethpb.SignedAggregateSubmitResponse]("SubmitSignedAggregateSelectionProofElectra", func() (*ethpb.SignedAggregateSubmitResponse, error) {
		return c.submitSignedAggregateSelectionProofElectra(ctx, in)
	})
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

func (c *beaconApiValidatorClient) SubscribeCommitteeSubnets(ctx context.Context, in *ethpb.CommitteeSubnetsSubscribeRequest, duties []*ethpb.ValidatorDuty) (*empty.Empty, error) {
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

func (c *beaconApiValidatorClient) AggregatedSelections(ctx context.Context, selections []BeaconCommitteeSelection) ([]BeaconCommitteeSelection, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.AggregatedSelections")
	defer span.End()

	return wrapInMetrics[[]BeaconCommitteeSelection]("AggregatedSelections", func() ([]BeaconCommitteeSelection, error) {
		return c.aggregatedSelection(ctx, selections)
	})
}

func (c *beaconApiValidatorClient) AggregatedSyncSelections(ctx context.Context, selections []SyncCommitteeSelection) ([]SyncCommitteeSelection, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-api.AggregatedSyncSelections")
	defer span.End()

	return wrapInMetrics[[]SyncCommitteeSelection]("AggregatedSyncSelections", func() ([]SyncCommitteeSelection, error) {
		return c.aggregatedSyncSelections(ctx, selections)
	})
}

func (c *beaconApiValidatorClient) aggregatedSelection(ctx context.Context, selections []BeaconCommitteeSelection) ([]BeaconCommitteeSelection, error) {
	body, err := json.Marshal(selections)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal selections")
	}

	var resp aggregatedSelectionResponse
	err = c.jsonRestHandler.Post(ctx, "/eth/v1/validator/beacon_committee_selections", nil, bytes.NewBuffer(body), &resp)
	if err != nil {
		return nil, errors.Wrap(err, "error calling post endpoint")
	}
	if len(resp.Data) == 0 {
		return nil, errors.New("no aggregated selection returned")
	}
	if len(selections) != len(resp.Data) {
		return nil, errors.New("mismatching number of selections")
	}

	return resp.Data, nil
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
