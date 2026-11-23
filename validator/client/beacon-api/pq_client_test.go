package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/rest"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/client/beacon-api/mock"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"go.uber.org/mock/gomock"
)

func testAttestationData() *ethpb.AttestationData {
	return &ethpb.AttestationData{
		Slot:            10,
		CommitteeIndex:  1,
		BeaconBlockRoot: make([]byte, fieldparams.RootLength),
		Source:          &ethpb.Checkpoint{Epoch: 0, Root: make([]byte, fieldparams.RootLength)},
		Target:          &ethpb.Checkpoint{Epoch: 1, Root: make([]byte, fieldparams.RootLength)},
	}
}

func TestBeaconApiValidatorClient_SubmitAttestationsPQ(t *testing.T) {
	handler := mock.NewMockHandler(gomock.NewController(t))
	attestation := &ethpb.SingleAttestationPQ{
		CommitteeId:   2,
		AttesterIndex: 3,
		Data:          testAttestationData(),
		Signature:     bytes.Repeat([]byte{0x01}, fieldparams.XMSSSignatureLength),
	}

	handler.EXPECT().Post(gomock.Any(), "/eth/v2/beacon/pool/attestations", map[string]string{api.VersionHeader: "pq"}, gomock.Any(), nil).DoAndReturn(
		func(_ context.Context, _ string, _ map[string]string, data *bytes.Buffer, _ any) error {
			var attestations []*structs.SingleAttestation
			require.NoError(t, json.Unmarshal(data.Bytes(), &attestations))
			require.Equal(t, 1, len(attestations))
			require.Equal(t, "2", attestations[0].CommitteeIndex)
			require.Equal(t, "3", attestations[0].AttesterIndex)
			require.Equal(t, hexutil.Encode(attestation.Signature), attestations[0].Signature)
			return nil
		},
	)

	c := &beaconApiValidatorClient{handler: handler}
	require.NoError(t, c.SubmitAttestationsPQ(t.Context(), []*ethpb.SingleAttestationPQ{attestation}))
}

func TestBeaconApiValidatorClient_XMSSPubkeys(t *testing.T) {
	first, second := [fieldparams.XMSSPubkeyLength]byte{0x01}, [fieldparams.XMSSPubkeyLength]byte{0x02}
	vrfRoot := hexutil.Encode(make([]byte, fieldparams.RootLength))

	t.Run("by validator index", func(t *testing.T) {
		handler := mock.NewMockHandler(gomock.NewController(t))
		expectPost(t, handler, "/eth/v1/beacon/states/head/validators", nil, []*structs.ValidatorContainerPQ{
			testValidatorPQ("0", hexutil.Encode(first[:]), vrfRoot),
			testValidatorPQ("7", hexutil.Encode(second[:]), vrfRoot),
		})

		c := &beaconApiValidatorClient{handler: handler}
		pubkeys, err := c.XMSSPubkeys(t.Context())
		require.NoError(t, err)
		require.Equal(t, 2, len(pubkeys))
		require.Equal(t, first, pubkeys[0])
		require.Equal(t, second, pubkeys[7])
	})

	t.Run("invalid public key", func(t *testing.T) {
		handler := mock.NewMockHandler(gomock.NewController(t))
		expectPost(t, handler, "/eth/v1/beacon/states/head/validators", nil, []*structs.ValidatorContainerPQ{
			testValidatorPQ("0", "0x01", vrfRoot),
		})

		c := &beaconApiValidatorClient{handler: handler}
		_, err := c.XMSSPubkeys(t.Context())
		require.ErrorContains(t, "decode XMSS public key 0x01", err)
	})

	t.Run("invalid index", func(t *testing.T) {
		handler := mock.NewMockHandler(gomock.NewController(t))
		expectPost(t, handler, "/eth/v1/beacon/states/head/validators", nil, []*structs.ValidatorContainerPQ{
			testValidatorPQ("x", hexutil.Encode(first[:]), vrfRoot),
		})

		c := &beaconApiValidatorClient{handler: handler}
		_, err := c.XMSSPubkeys(t.Context())
		require.ErrorContains(t, "parse validator index x", err)
	})
}

func TestBeaconApiValidatorClient_AggregatePQ(t *testing.T) {
	data := testAttestationData()
	aggregate := &ethpb.AttestationPQ{
		AggregationBits: []byte{0x03},
		Data:            data,
		CommitteeBits:   []byte{0x04, 0x00},
	}

	expectGet := func(t *testing.T, version string) *beaconApiValidatorClient {
		handler := mock.NewMockHandler(gomock.NewController(t))
		handler.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, endpoint string, resp any, _ ...rest.QueryOption) error {
				path, query, ok := strings.Cut(endpoint, "?")
				require.Equal(t, true, ok)
				require.Equal(t, "/eth/v2/validator/aggregate_attestation", path)

				params, err := url.ParseQuery(query)
				require.NoError(t, err)
				require.Equal(t, "10", params.Get("slot"))
				require.Equal(t, "2", params.Get("committee_index"))
				require.Equal(t, hexutil.Encode(bytes.Repeat([]byte{0x05}, 32)), params.Get("attestation_data_root"))

				*resp.(*pqAggregateResponse) = pqAggregateResponse{
					Version: version,
					Data:    structs.AggregatePQFromConsensus(aggregate, []byte{0x06}),
				}
				return nil
			},
		)

		return &beaconApiValidatorClient{handler: handler}
	}

	t.Run("PQ aggregate", func(t *testing.T) {
		c := expectGet(t, "pq")
		actual, proof, err := c.AggregatePQ(t.Context(), 10, [32]byte(bytes.Repeat([]byte{0x05}, 32)), 2)
		require.NoError(t, err)
		require.DeepEqual(t, []byte{0x06}, proof)
		require.DeepEqual(t, data, actual.Data)
	})

	t.Run("aggregate of another fork", func(t *testing.T) {
		c := expectGet(t, "gloas")
		_, _, err := c.AggregatePQ(t.Context(), 10, [32]byte(bytes.Repeat([]byte{0x05}, 32)), 2)
		require.ErrorContains(t, `aggregate of version "gloas"`, err)
	})
}

func TestBeaconApiValidatorClient_ProduceBlockPQ(t *testing.T) {
	handler := mock.NewMockHandler(gomock.NewController(t))
	randaoReveal := bytes.Repeat([]byte{0x07}, fieldparams.VRFProofLength*fieldparams.RootLength)

	handler.EXPECT().Post(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, endpoint string, headers map[string]string, data *bytes.Buffer, _ any) error {
			path, query, ok := strings.Cut(endpoint, "?")
			require.Equal(t, true, ok)
			require.Equal(t, "/eth/v4/validator/blocks/12", path)

			params, err := url.ParseQuery(query)
			require.NoError(t, err)
			require.Equal(t, hexutil.Encode(randaoReveal), params.Get("randao_reveal"))
			require.Equal(t, "true", params.Get("include_payload"))
			require.Equal(t, hexutil.Encode([]byte("graffiti")), params.Get("graffiti"))

			require.Equal(t, "pq", headers[api.VersionHeader])
			require.Equal(t, pqEmptyBuilderConfig, data.String())

			// The beacon node answers with a block of another fork.
			return nil
		},
	)

	c := &beaconApiValidatorClient{handler: handler}
	_, err := c.ProduceBlockPQ(t.Context(), 12, randaoReveal, []byte("graffiti"))
	require.ErrorContains(t, `block of version ""`, err)
}

func TestBeaconApiValidatorClient_SubmitProposerPreferencesPQ(t *testing.T) {
	handler := mock.NewMockHandler(gomock.NewController(t))
	preferences := &ethpb.SignedProposerPreferencesPQ{
		Message: &ethpb.ProposerPreferences{
			DependentRoot:  make([]byte, fieldparams.RootLength),
			ProposalSlot:   primitives.Slot(20),
			ValidatorIndex: 3,
			FeeRecipient:   make([]byte, fieldparams.FeeRecipientLength),
			TargetGasLimit: 60_000_000,
		},
		Signature: bytes.Repeat([]byte{0x01}, fieldparams.XMSSSignatureLength),
	}

	handler.EXPECT().Post(gomock.Any(), "/eth/v1/validator/proposer_preferences", map[string]string{api.VersionHeader: "pq"}, gomock.Any(), nil).DoAndReturn(
		func(_ context.Context, _ string, _ map[string]string, data *bytes.Buffer, _ any) error {
			var decoded []*structs.SignedProposerPreferences
			require.NoError(t, json.Unmarshal(data.Bytes(), &decoded))
			require.Equal(t, 1, len(decoded))
			require.Equal(t, "20", decoded[0].Message.ProposalSlot)
			require.Equal(t, hexutil.Encode(preferences.Signature), decoded[0].Signature)
			return nil
		},
	)

	c := &beaconApiValidatorClient{handler: handler}
	require.NoError(t, c.SubmitProposerPreferencesPQ(t.Context(), []*ethpb.SignedProposerPreferencesPQ{preferences}))
}

func TestBeaconApiValidatorClient_PublishEnvelopePQ(t *testing.T) {
	handler := mock.NewMockHandler(gomock.NewController(t))

	handler.EXPECT().Post(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), nil).DoAndReturn(
		func(_ context.Context, endpoint string, headers map[string]string, _ *bytes.Buffer, _ any) error {
			require.Equal(t, "/eth/v1/beacon/execution_payload_envelopes?broadcast_validation=consensus", endpoint)
			require.Equal(t, "pq", headers[api.VersionHeader])
			require.Equal(t, "true", headers[api.BlobDataIncludedHeader])
			return nil
		},
	)

	c := &beaconApiValidatorClient{handler: handler}
	err := c.PublishEnvelopePQ(t.Context(), testSignedEnvelopePQ(), nil, nil)
	require.NoError(t, err)
}

func TestBeaconApiValidatorClient_IsAggregatorPQ(t *testing.T) {
	t.Run("not in the PQ fork", func(t *testing.T) {
		require.Equal(t, false, (&beaconApiValidatorClient{}).IsAggregatorPQ(1, 1))
	})

	t.Run("in the PQ fork", func(t *testing.T) {
		isAggregator := true
		c := &beaconApiValidatorClient{pqAggregators: newPQAggregators()}
		require.NoError(t, c.pqAggregators.set(&structs.AttesterDuty{Slot: "1", ValidatorIndex: "1", IsAggregator: &isAggregator}))
		require.Equal(t, true, c.IsAggregatorPQ(1, 1))
	})
}

func testSignedEnvelopePQ() *ethpb.SignedExecutionPayloadEnvelopePQ {
	return &ethpb.SignedExecutionPayloadEnvelopePQ{
		Message: &ethpb.ExecutionPayloadEnvelopePQ{
			Payload:               &enginev1.ExecutionPayloadGloas{},
			ExecutionRequests:     &enginev1.ExecutionRequestsPQ{},
			BeaconBlockRoot:       make([]byte, fieldparams.RootLength),
			ParentBeaconBlockRoot: make([]byte, fieldparams.RootLength),
		},
		Signature: bytes.Repeat([]byte{0x01}, fieldparams.XMSSSignatureLength),
	}
}
