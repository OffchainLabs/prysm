package structs

import (
	"encoding/json"
	"strings"
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestVRFProofToConsensus(t *testing.T) {
	proof := testBlockPQProto().Body.RandaoReveal

	t.Run("round trip", func(t *testing.T) {
		nodes := VRFProofFromConsensus(proof)
		require.Equal(t, fieldparams.VRFProofLength, len(nodes))
		require.Equal(t, hexutil.Encode(proof[fieldparams.RootLength:2*fieldparams.RootLength]), nodes[1])

		decoded, err := VRFProofToConsensus(nodes)
		require.NoError(t, err)
		require.DeepEqual(t, proof, decoded)
	})

	t.Run("wrong node count", func(t *testing.T) {
		nodes := VRFProofFromConsensus(proof)
		_, err := VRFProofToConsensus(nodes[1:])
		require.ErrorContains(t, "VRF proof of 31 nodes, expected 32 nodes", err)
	})

	t.Run("wrong node length", func(t *testing.T) {
		nodes := VRFProofFromConsensus(proof)
		nodes[3] = "0x01"
		_, err := VRFProofToConsensus(nodes)
		require.ErrorContains(t, "[3]", err)
	})
}

func TestBeaconBlockPQ_ToConsensus(t *testing.T) {
	t.Run("round trip", func(t *testing.T) {
		expected := testBlockPQProto()

		actual, err := BeaconBlockPQFromConsensus(expected).ToConsensus()
		require.NoError(t, err)
		requireSameRoot(t, expected, actual)
	})

	t.Run("randao reveal encoded as a list of nodes", func(t *testing.T) {
		encoded, err := json.Marshal(BeaconBlockPQFromConsensus(testBlockPQProto()).Body)
		require.NoError(t, err)

		var body map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(encoded, &body))

		var randaoReveal []string
		require.NoError(t, json.Unmarshal(body["randao_reveal"], &randaoReveal))
		require.Equal(t, fieldparams.VRFProofLength, len(randaoReveal))
	})

	t.Run("nil body", func(t *testing.T) {
		block := BeaconBlockPQFromConsensus(testBlockPQProto())
		block.Body = nil
		_, err := block.ToConsensus()
		require.ErrorContains(t, "Body", err)
	})

	t.Run("BLS signed bid", func(t *testing.T) {
		block := BeaconBlockPQFromConsensus(testBlockPQProto())
		block.Body.SignedExecutionPayloadBid.Signature = hexutil.Encode(fillByteSlice(fieldparams.BLSSignatureLength, 0x01))
		_, err := block.ToConsensus()
		require.ErrorContains(t, "SignedExecutionPayloadBid.Signature", err)
	})

	t.Run("too many attestations", func(t *testing.T) {
		block := BeaconBlockPQFromConsensus(testBlockPQProto())
		attestation := block.Body.Attestations[0]
		block.Body.Attestations = make([]*AttestationPQ, params.BeaconConfig().MaxAttestationsPQ+1)
		for i := range block.Body.Attestations {
			block.Body.Attestations[i] = attestation
		}

		_, err := block.ToConsensus()
		require.ErrorContains(t, "Attestations", err)
	})
}

func TestSignedBeaconBlockPQ_ToConsensus(t *testing.T) {
	expected := &eth.SignedBeaconBlockPQ{Block: testBlockPQProto(), Signature: testXMSSSignature(0x71)}

	t.Run("round trip", func(t *testing.T) {
		actual, err := SignedBeaconBlockPQFromConsensus(expected).ToConsensus()
		require.NoError(t, err)
		requireSameRoot(t, expected, actual)
	})

	t.Run("short signature", func(t *testing.T) {
		block := SignedBeaconBlockPQFromConsensus(expected)
		block.Signature = "0x01"
		_, err := block.ToConsensus()
		require.ErrorContains(t, "Signature", err)
	})
}

func TestBlockContentsPQ_ToConsensus(t *testing.T) {
	expectedBlock, expectedEnvelope := testBlockPQProto(), testEnvelopePQProto()
	expectedProofs := [][]byte{fillByteSlice(fieldparams.BLSPubkeyLength, 0x72)}
	expectedBlobs := [][]byte{fillByteSlice(fieldparams.BlobLength, 0x73)}

	contents, err := BlockContentsPQFromConsensus(expectedBlock, expectedEnvelope, expectedProofs, expectedBlobs)
	require.NoError(t, err)

	block, envelope, proofs, blobs, err := contents.ToConsensus()
	require.NoError(t, err)
	requireSameRoot(t, expectedBlock, block)
	requireSameRoot(t, expectedEnvelope, envelope)
	require.DeepEqual(t, expectedProofs, proofs)
	require.DeepEqual(t, expectedBlobs, blobs)
}

func TestSignedExecutionPayloadEnvelopeContentsPQ_ToConsensus(t *testing.T) {
	expected := &eth.SignedExecutionPayloadEnvelopePQ{Message: testEnvelopePQProto(), Signature: testXMSSSignature(0x74)}
	expectedProofs := [][]byte{fillByteSlice(fieldparams.BLSPubkeyLength, 0x75)}

	contents, err := SignedExecutionPayloadEnvelopeContentsPQFromConsensus(expected, expectedProofs, nil)
	require.NoError(t, err)

	envelope, proofs, blobs, err := contents.ToConsensus()
	require.NoError(t, err)
	requireSameRoot(t, expected, envelope)
	require.DeepEqual(t, expectedProofs, proofs)
	require.Equal(t, 0, len(blobs))
}

func TestExecutionRequestsPQ_ToConsensus(t *testing.T) {
	t.Run("round trip", func(t *testing.T) {
		expected := testExecutionRequestsPQProto()

		actual, err := ExecutionRequestsPQFromConsensus(expected).ToConsensus()
		require.NoError(t, err)
		requireSameRoot(t, expected, actual)
	})

	t.Run("too many voluntary exits", func(t *testing.T) {
		requests := ExecutionRequestsPQFromConsensus(testExecutionRequestsPQProto())
		exit := requests.VoluntaryExits[0]
		requests.VoluntaryExits = make([]*VoluntaryExitRequest, params.BeaconConfig().MaxVoluntaryExitRequestsPerPayload+1)
		for i := range requests.VoluntaryExits {
			requests.VoluntaryExits[i] = exit
		}

		_, err := requests.ToConsensus()
		require.ErrorContains(t, "VoluntaryExits", err)
	})

	t.Run("BLS public key", func(t *testing.T) {
		requests := ExecutionRequestsPQFromConsensus(testExecutionRequestsPQProto())
		requests.VoluntaryExits[0].Pubkey = hexutil.Encode(fillByteSlice(fieldparams.BLSPubkeyLength, 0x01))
		_, err := requests.ToConsensus()
		require.ErrorContains(t, "VoluntaryExits[0]", err)
	})
}

func TestAggregatePQ_ToConsensus(t *testing.T) {
	t.Run("round trip", func(t *testing.T) {
		expectedAggregate, expectedProof := testAttestationPQProto(), []byte{0x01, 0x02}

		aggregate, proof, err := AggregatePQFromConsensus(expectedAggregate, expectedProof).ToConsensus()
		require.NoError(t, err)
		requireSameRoot(t, expectedAggregate, aggregate)
		require.DeepEqual(t, expectedProof, proof)
	})

	t.Run("proof too large", func(t *testing.T) {
		aggregate := AggregatePQFromConsensus(testAttestationPQProto(), nil)
		aggregate.Proof = hexutil.Encode(make([]byte, params.BeaconConfig().MaxStarkProofSize+1))
		_, _, err := aggregate.ToConsensus()
		require.ErrorContains(t, "exceeds the maximum", err)
	})

	t.Run("wrong committee bits length", func(t *testing.T) {
		aggregate := AggregatePQFromConsensus(testAttestationPQProto(), nil)
		aggregate.Aggregate.CommitteeBits = hexutil.Encode(make([]byte, 8))
		_, _, err := aggregate.ToConsensus()
		require.ErrorContains(t, "CommitteeBits", err)
	})
}

func TestSignedAggregateAttestationAndProofPQ_ToConsensus(t *testing.T) {
	expected := &eth.SignedAggregateAttestationAndProofPQ{
		Message: &eth.AggregateAttestationAndProofPQ{
			AggregatorIndex: 9,
			Aggregate:       testAttestationPQProto(),
			Proof:           []byte{0x01, 0x02},
		},
		Signature: testXMSSSignature(0x76),
	}

	actual, err := SignedAggregateAttAndProofPQFromConsensus(expected).ToConsensus()
	require.NoError(t, err)
	requireSameRoot(t, expected, actual)
}

func TestSingleAttestation_ToConsensusPQ(t *testing.T) {
	expected := &eth.SingleAttestationPQ{
		CommitteeId:   1,
		AttesterIndex: 8,
		Data:          testAttestationDataProto(),
		Signature:     testXMSSSignature(0x77),
	}

	t.Run("round trip", func(t *testing.T) {
		actual, err := SingleAttPQFromConsensus(expected).ToConsensusPQ()
		require.NoError(t, err)
		requireSameRoot(t, expected, actual)
	})

	t.Run("BLS signature", func(t *testing.T) {
		attestation := SingleAttPQFromConsensus(expected)
		attestation.Signature = hexutil.Encode(fillByteSlice(fieldparams.BLSSignatureLength, 0x01))
		_, err := attestation.ToConsensusPQ()
		require.ErrorContains(t, "Signature", err)
	})
}

func TestPayloadAttestationMessage_ToConsensusPQ(t *testing.T) {
	expected := &eth.PayloadAttestationMessagePQ{
		ValidatorIndex: 5,
		Data: &eth.PayloadAttestationData{
			BeaconBlockRoot: fillByteSlice(fieldparams.RootLength, 0x78),
			Slot:            6,
			PayloadPresent:  true,
		},
		Signature: testXMSSSignature(0x79),
	}

	actual, err := PayloadAttestationMessagePQFromConsensus(expected).ToConsensusPQ()
	require.NoError(t, err)
	requireSameRoot(t, expected, actual)
}

func TestSignedProposerPreferences_ToConsensusPQ(t *testing.T) {
	preferences, err := validProposerPreferences().ToConsensus()
	require.NoError(t, err)

	expected := &eth.SignedProposerPreferencesPQ{Message: preferences, Signature: testXMSSSignature(0x7a)}

	t.Run("round trip", func(t *testing.T) {
		actual, err := SignedProposerPreferencesPQFromConsensus(expected).ToConsensusPQ()
		require.NoError(t, err)
		requireSameRoot(t, expected, actual)
	})

	t.Run("nil message", func(t *testing.T) {
		_, err := (&SignedProposerPreferences{Signature: hexutil.Encode(testXMSSSignature(0x7a))}).ToConsensusPQ()
		require.ErrorContains(t, "Message", err)
	})
}

func TestValidatorPQ_ToConsensus(t *testing.T) {
	expected := &eth.ValidatorPQ{
		PublicKey:                  fillByteSlice(fieldparams.XMSSPubkeyLength, 0x7b),
		VrfRoot:                    fillByteSlice(fieldparams.RootLength, 0x7c),
		WithdrawalCredentials:      fillByteSlice(fieldparams.RootLength, 0x7d),
		EffectiveBalance:           32_000_000_000,
		ActivationEligibilityEpoch: 1,
		ActivationEpoch:            2,
		ExitEpoch:                  3,
		WithdrawableEpoch:          4,
	}

	t.Run("round trip", func(t *testing.T) {
		actual, err := ValidatorPQFromConsensus(expected).ToConsensus()
		require.NoError(t, err)
		requireSameRoot(t, expected, actual)
	})

	t.Run("BLS public key", func(t *testing.T) {
		validator := ValidatorPQFromConsensus(expected)
		validator.Pubkey = hexutil.Encode(fillByteSlice(fieldparams.BLSPubkeyLength, 0x01))
		_, err := validator.ToConsensus()
		require.ErrorContains(t, "Pubkey", err)
	})
}

func TestAttesterDuty(t *testing.T) {
	t.Run("is_aggregator omitted when unset", func(t *testing.T) {
		encoded, err := json.Marshal(&AttesterDuty{})
		require.NoError(t, err)
		require.Equal(t, false, strings.Contains(string(encoded), "is_aggregator"))
	})

	t.Run("is_aggregator decoded when set", func(t *testing.T) {
		var duty AttesterDuty
		require.NoError(t, json.Unmarshal([]byte(`{"is_aggregator":true}`), &duty))
		require.NotNil(t, duty.IsAggregator)
		require.Equal(t, true, *duty.IsAggregator)
	})
}

func testXMSSSignature(value byte) []byte {
	return fillByteSlice(fieldparams.XMSSSignatureLength, value)
}

func testAttestationDataProto() *eth.AttestationData {
	return &eth.AttestationData{
		Slot:            10,
		CommitteeIndex:  0,
		BeaconBlockRoot: fillByteSlice(fieldparams.RootLength, 0x01),
		Source:          &eth.Checkpoint{Epoch: 1, Root: fillByteSlice(fieldparams.RootLength, 0x02)},
		Target:          &eth.Checkpoint{Epoch: 2, Root: fillByteSlice(fieldparams.RootLength, 0x03)},
	}
}

func testAttestationPQProto() *eth.AttestationPQ {
	committeeBits := make([]byte, (params.BeaconConfig().MaxCommitteesPerSlotPQ+7)/8)
	committeeBits[0] = 0x01

	return &eth.AttestationPQ{
		AggregationBits: []byte{0b00001101},
		Data:            testAttestationDataProto(),
		CommitteeBits:   committeeBits,
	}
}

func testIndexedAttestationPQProto() *eth.IndexedAttestationPQ {
	return &eth.IndexedAttestationPQ{
		AttestingIndices: []uint64{1, 5},
		Data:             testAttestationDataProto(),
		Proof:            []byte{0x0a, 0x0b},
	}
}

func testSignedHeaderPQProto(value byte) *eth.SignedBeaconBlockHeaderPQ {
	return &eth.SignedBeaconBlockHeaderPQ{
		Header: &eth.BeaconBlockHeader{
			Slot:          3,
			ProposerIndex: 4,
			ParentRoot:    fillByteSlice(fieldparams.RootLength, value),
			StateRoot:     fillByteSlice(fieldparams.RootLength, value),
			BodyRoot:      fillByteSlice(fieldparams.RootLength, value),
		},
		Signature: testXMSSSignature(value),
	}
}

func testExecutionRequestsPQProto() *enginev1.ExecutionRequestsPQ {
	return &enginev1.ExecutionRequestsPQ{
		ValidatorPreregistrations: []*enginev1.ValidatorPreregistrationRequest{{
			Pubkey:                fillByteSlice(fieldparams.XMSSPubkeyLength, 0x51),
			WithdrawalCredentials: fillByteSlice(fieldparams.RootLength, 0x52),
			VrfRoot:               fillByteSlice(fieldparams.RootLength, 0x53),
		}},
		BuilderPreregistrations: []*enginev1.BuilderPreregistrationRequest{{
			Pubkey:                fillByteSlice(fieldparams.XMSSPubkeyLength, 0x54),
			WithdrawalCredentials: fillByteSlice(fieldparams.RootLength, 0x55),
		}},
		VoluntaryExits: []*enginev1.VoluntaryExitRequest{{
			Pubkey: fillByteSlice(fieldparams.XMSSPubkeyLength, 0x56),
		}},
	}
}

func testEnvelopePQProto() *eth.ExecutionPayloadEnvelopePQ {
	gloas := testEnvelopeProto()

	return &eth.ExecutionPayloadEnvelopePQ{
		Payload:               gloas.Payload,
		ExecutionRequests:     testExecutionRequestsPQProto(),
		BuilderIndex:          gloas.BuilderIndex,
		BeaconBlockRoot:       gloas.BeaconBlockRoot,
		ParentBeaconBlockRoot: gloas.ParentBeaconBlockRoot,
	}
}

func testBlockPQProto() *eth.BeaconBlockPQ {
	randaoReveal := make([]byte, 0, fieldparams.VRFProofLength*fieldparams.RootLength)
	for i := range fieldparams.VRFProofLength {
		randaoReveal = append(randaoReveal, fillByteSlice(fieldparams.RootLength, byte(i))...)
	}

	return &eth.BeaconBlockPQ{
		Slot:          12,
		ProposerIndex: 3,
		ParentRoot:    fillByteSlice(fieldparams.RootLength, 0x61),
		StateRoot:     fillByteSlice(fieldparams.RootLength, 0x62),
		Body: &eth.BeaconBlockBodyPQ{
			RandaoReveal: randaoReveal,
			Eth1Data: &eth.Eth1Data{
				DepositRoot:  fillByteSlice(fieldparams.RootLength, 0x63),
				DepositCount: 7,
				BlockHash:    fillByteSlice(fieldparams.RootLength, 0x64),
			},
			Graffiti: fillByteSlice(fieldparams.RootLength, 0x65),
			ProposerSlashings: []*eth.ProposerSlashingPQ{{
				Header_1: testSignedHeaderPQProto(0x66),
				Header_2: testSignedHeaderPQProto(0x67),
			}},
			AttesterSlashings: []*eth.AttesterSlashingPQ{{
				Attestation_1: testIndexedAttestationPQProto(),
				Attestation_2: testIndexedAttestationPQProto(),
			}},
			Attestations: []*eth.AttestationPQ{testAttestationPQProto()},
			Deposits:     []*eth.Deposit{},
			SignedExecutionPayloadBid: &eth.SignedExecutionPayloadBidPQ{
				Message: &eth.ExecutionPayloadBid{
					ParentBlockHash:       fillByteSlice(fieldparams.RootLength, 0x68),
					ParentBlockRoot:       fillByteSlice(fieldparams.RootLength, 0x69),
					BlockHash:             fillByteSlice(fieldparams.RootLength, 0x6a),
					PrevRandao:            fillByteSlice(fieldparams.RootLength, 0x6b),
					FeeRecipient:          fillByteSlice(fieldparams.FeeRecipientLength, 0x6c),
					GasLimit:              30_000_000,
					BuilderIndex:          2,
					Slot:                  12,
					BlobKzgCommitments:    [][]byte{},
					ExecutionRequestsRoot: fillByteSlice(fieldparams.RootLength, 0x6d),
				},
				Signature: testXMSSSignature(0x6e),
			},
			ParentExecutionRequests: testExecutionRequestsPQProto(),
			Proof:                   []byte{0x01, 0x02, 0x03},
		},
	}
}

// requireSameRoot checks that `actual` has the hash tree root of `expected`: it signs the same.
func requireSameRoot(t *testing.T, expected, actual interface{ HashTreeRoot() ([32]byte, error) }) {
	expectedRoot, err := expected.HashTreeRoot()
	require.NoError(t, err)

	actualRoot, err := actual.HashTreeRoot()
	require.NoError(t, err)

	require.Equal(t, expectedRoot, actualRoot)
}
