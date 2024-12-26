package encoder_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"testing"

	gogo "github.com/gogo/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	fastssz "github.com/prysmaticlabs/fastssz"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/encoder"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/testing/protocmp"
)

// Define an interface that combines fastssz.Marshaler and proto.Message
type MarshalerProtoMessage interface {
	fastssz.Marshaler
	proto.Message
	fastssz.Unmarshaler
}

type MarshalerProtoCreator interface {
	Create() MarshalerProtoMessage
}

type AttestationCreator struct{}
type AttestationElectraCreator struct{}
type AggregateAttestationAndProofCreator struct{}
type AggregateAttestationAndProofElectraCreator struct{}
type SignedAggregateAttestationAndProofCreator struct{}
type SignedAggregateAttestationAndProofElectraCreator struct{}
type AttestationDataCreator struct{}
type CheckpointCreator struct{}
type BeaconBlockCreator struct{}
type SignedBeaconBlockCreator struct{}
type BeaconBlockAltairCreator struct{}
type SignedBeaconBlockAltairCreator struct{}
type BeaconBlockBodyCreator struct{}
type BeaconBlockBodyAltairCreator struct{}
type ProposerSlashingCreator struct{}
type AttesterSlashingCreator struct{}
type AttesterSlashingElectraCreator struct{}
type DepositCreator struct{}
type VoluntaryExitCreator struct{}
type SignedVoluntaryExitCreator struct{}
type Eth1DataCreator struct{}
type BeaconBlockHeaderCreator struct{}
type SignedBeaconBlockHeaderCreator struct{}
type IndexedAttestationCreator struct{}
type IndexedAttestationElectraCreator struct{}
type SyncAggregateCreator struct{}
type SignedBeaconBlockBellatrixCreator struct{}
type BeaconBlockBellatrixCreator struct{}
type BeaconBlockBodyBellatrixCreator struct{}
type SignedBlindedBeaconBlockBellatrixCreator struct{}
type BlindedBeaconBlockBellatrixCreator struct{}
type BlindedBeaconBlockBodyBellatrixCreator struct{}
type SignedBeaconBlockContentsDenebCreator struct{}
type BeaconBlockContentsDenebCreator struct{}
type SignedBeaconBlockDenebCreator struct{}
type BeaconBlockDenebCreator struct{}
type BeaconBlockBodyDenebCreator struct{}
type SignedBeaconBlockCapellaCreator struct{}
type BeaconBlockCapellaCreator struct{}
type BeaconBlockBodyCapellaCreator struct{}
type SignedBlindedBeaconBlockCapellaCreator struct{}
type BlindedBeaconBlockCapellaCreator struct{}
type BlindedBeaconBlockBodyCapellaCreator struct{}
type SignedBlindedBeaconBlockDenebCreator struct{}
type BlindedBeaconBlockDenebCreator struct{}
type BlindedBeaconBlockBodyDenebCreator struct{}
type SignedBeaconBlockElectraCreator struct{}
type BeaconBlockElectraCreator struct{}
type BeaconBlockBodyElectraCreator struct{}
type SignedBlindedBeaconBlockElectraCreator struct{}
type BlindedBeaconBlockElectraCreator struct{}
type BlindedBeaconBlockBodyElectraCreator struct{}
type ValidatorRegistrationV1Creator struct{}
type SignedValidatorRegistrationV1Creator struct{}
type BuilderBidCreator struct{}
type BuilderBidCapellaCreator struct{}
type BuilderBidDenebCreator struct{}
type BlobSidecarCreator struct{}
type BlobSidecarsCreator struct{}
type Deposit_DataCreator struct{}
type BeaconStateCreator struct{}
type BeaconStateAltairCreator struct{}
type ForkCreator struct{}
type PendingAttestationCreator struct{}
type HistoricalBatchCreator struct{}
type SigningDataCreator struct{}
type ForkDataCreator struct{}
type DepositMessageCreator struct{}
type SyncCommitteeCreator struct{}
type SyncAggregatorSelectionDataCreator struct{}
type BeaconStateBellatrixCreator struct{}
type BeaconStateCapellaCreator struct{}
type BeaconStateDenebCreator struct{}
type BeaconStateElectraCreator struct{}
type PowBlockCreator struct{}
type HistoricalSummaryCreator struct{}
type BlobIdentifierCreator struct{}
type PendingDepositCreator struct{}
type PendingPartialWithdrawalCreator struct{}
type PendingConsolidationCreator struct{}
type StatusCreator struct{}
type BeaconBlocksByRangeRequestCreator struct{}
type ENRForkIDCreator struct{}
type MetaDataV0Creator struct{}
type MetaDataV1Creator struct{}
type BlobSidecarsByRangeRequestCreator struct{}
type DepositSnapshotCreator struct{}
type SyncCommitteeMessageCreator struct{}
type SyncCommitteeContributionCreator struct{}
type ContributionAndProofCreator struct{}
type SignedContributionAndProofCreator struct{}
type ValidatorCreator struct{}
type BLSToExecutionChangeCreator struct{}
type SignedBLSToExecutionChangeCreator struct{}

func (AttestationCreator) Create() MarshalerProtoMessage        { return &ethpb.Attestation{} }
func (AttestationElectraCreator) Create() MarshalerProtoMessage { return &ethpb.AttestationElectra{} }
func (AggregateAttestationAndProofCreator) Create() MarshalerProtoMessage {
	return &ethpb.AggregateAttestationAndProof{}
}
func (AggregateAttestationAndProofElectraCreator) Create() MarshalerProtoMessage {
	return &ethpb.AggregateAttestationAndProofElectra{}
}
func (SignedAggregateAttestationAndProofCreator) Create() MarshalerProtoMessage {
	return &ethpb.SignedAggregateAttestationAndProof{}
}
func (SignedAggregateAttestationAndProofElectraCreator) Create() MarshalerProtoMessage {
	return &ethpb.SignedAggregateAttestationAndProofElectra{}
}
func (AttestationDataCreator) Create() MarshalerProtoMessage   { return &ethpb.AttestationData{} }
func (CheckpointCreator) Create() MarshalerProtoMessage        { return &ethpb.Checkpoint{} }
func (BeaconBlockCreator) Create() MarshalerProtoMessage       { return &ethpb.BeaconBlock{} }
func (SignedBeaconBlockCreator) Create() MarshalerProtoMessage { return &ethpb.SignedBeaconBlock{} }
func (BeaconBlockAltairCreator) Create() MarshalerProtoMessage { return &ethpb.BeaconBlockAltair{} }
func (SignedBeaconBlockAltairCreator) Create() MarshalerProtoMessage {
	return &ethpb.SignedBeaconBlockAltair{}
}
func (BeaconBlockBodyCreator) Create() MarshalerProtoMessage { return &ethpb.BeaconBlockBody{} }
func (BeaconBlockBodyAltairCreator) Create() MarshalerProtoMessage {
	return &ethpb.BeaconBlockBodyAltair{}
}
func (ProposerSlashingCreator) Create() MarshalerProtoMessage { return &ethpb.ProposerSlashing{} }
func (AttesterSlashingCreator) Create() MarshalerProtoMessage { return &ethpb.AttesterSlashing{} }
func (AttesterSlashingElectraCreator) Create() MarshalerProtoMessage {
	return &ethpb.AttesterSlashingElectra{}
}
func (DepositCreator) Create() MarshalerProtoMessage       { return &ethpb.Deposit{} }
func (VoluntaryExitCreator) Create() MarshalerProtoMessage { return &ethpb.VoluntaryExit{} }
func (SignedVoluntaryExitCreator) Create() MarshalerProtoMessage {
	return &ethpb.SignedVoluntaryExit{}
}
func (Eth1DataCreator) Create() MarshalerProtoMessage          { return &ethpb.Eth1Data{} }
func (BeaconBlockHeaderCreator) Create() MarshalerProtoMessage { return &ethpb.BeaconBlockHeader{} }
func (SignedBeaconBlockHeaderCreator) Create() MarshalerProtoMessage {
	return &ethpb.SignedBeaconBlockHeader{}
}
func (IndexedAttestationCreator) Create() MarshalerProtoMessage { return &ethpb.IndexedAttestation{} }
func (IndexedAttestationElectraCreator) Create() MarshalerProtoMessage {
	return &ethpb.IndexedAttestationElectra{}
}
func (SyncAggregateCreator) Create() MarshalerProtoMessage { return &ethpb.SyncAggregate{} }
func (SignedBeaconBlockBellatrixCreator) Create() MarshalerProtoMessage {
	return &ethpb.SignedBeaconBlockBellatrix{}
}
func (BeaconBlockBellatrixCreator) Create() MarshalerProtoMessage {
	return &ethpb.BeaconBlockBellatrix{}
}
func (BeaconBlockBodyBellatrixCreator) Create() MarshalerProtoMessage {
	return &ethpb.BeaconBlockBodyBellatrix{}
}
func (SignedBlindedBeaconBlockBellatrixCreator) Create() MarshalerProtoMessage {
	return &ethpb.SignedBlindedBeaconBlockBellatrix{}
}
func (BlindedBeaconBlockBellatrixCreator) Create() MarshalerProtoMessage {
	return &ethpb.BlindedBeaconBlockBellatrix{}
}
func (BlindedBeaconBlockBodyBellatrixCreator) Create() MarshalerProtoMessage {
	return &ethpb.BlindedBeaconBlockBodyBellatrix{}
}
func (SignedBeaconBlockContentsDenebCreator) Create() MarshalerProtoMessage {
	return &ethpb.SignedBeaconBlockContentsDeneb{}
}
func (BeaconBlockContentsDenebCreator) Create() MarshalerProtoMessage {
	return &ethpb.BeaconBlockContentsDeneb{}
}
func (SignedBeaconBlockDenebCreator) Create() MarshalerProtoMessage {
	return &ethpb.SignedBeaconBlockDeneb{}
}
func (BeaconBlockDenebCreator) Create() MarshalerProtoMessage { return &ethpb.BeaconBlockDeneb{} }
func (BeaconBlockBodyDenebCreator) Create() MarshalerProtoMessage {
	return &ethpb.BeaconBlockBodyDeneb{}
}
func (SignedBeaconBlockCapellaCreator) Create() MarshalerProtoMessage {
	return &ethpb.SignedBeaconBlockCapella{}
}
func (BeaconBlockCapellaCreator) Create() MarshalerProtoMessage { return &ethpb.BeaconBlockCapella{} }
func (BeaconBlockBodyCapellaCreator) Create() MarshalerProtoMessage {
	return &ethpb.BeaconBlockBodyCapella{}
}
func (SignedBlindedBeaconBlockCapellaCreator) Create() MarshalerProtoMessage {
	return &ethpb.SignedBlindedBeaconBlockCapella{}
}
func (BlindedBeaconBlockCapellaCreator) Create() MarshalerProtoMessage {
	return &ethpb.BlindedBeaconBlockCapella{}
}
func (BlindedBeaconBlockBodyCapellaCreator) Create() MarshalerProtoMessage {
	return &ethpb.BlindedBeaconBlockBodyCapella{}
}
func (SignedBlindedBeaconBlockDenebCreator) Create() MarshalerProtoMessage {
	return &ethpb.SignedBlindedBeaconBlockDeneb{}
}
func (BlindedBeaconBlockDenebCreator) Create() MarshalerProtoMessage {
	return &ethpb.BlindedBeaconBlockDeneb{}
}
func (BlindedBeaconBlockBodyDenebCreator) Create() MarshalerProtoMessage {
	return &ethpb.BlindedBeaconBlockBodyDeneb{}
}
func (SignedBeaconBlockElectraCreator) Create() MarshalerProtoMessage {
	return &ethpb.SignedBeaconBlockElectra{}
}
func (BeaconBlockElectraCreator) Create() MarshalerProtoMessage { return &ethpb.BeaconBlockElectra{} }
func (BeaconBlockBodyElectraCreator) Create() MarshalerProtoMessage {
	return &ethpb.BeaconBlockBodyElectra{}
}
func (SignedBlindedBeaconBlockElectraCreator) Create() MarshalerProtoMessage {
	return &ethpb.SignedBlindedBeaconBlockElectra{}
}
func (BlindedBeaconBlockElectraCreator) Create() MarshalerProtoMessage {
	return &ethpb.BlindedBeaconBlockElectra{}
}
func (BlindedBeaconBlockBodyElectraCreator) Create() MarshalerProtoMessage {
	return &ethpb.BlindedBeaconBlockBodyElectra{}
}
func (ValidatorRegistrationV1Creator) Create() MarshalerProtoMessage {
	return &ethpb.ValidatorRegistrationV1{}
}
func (SignedValidatorRegistrationV1Creator) Create() MarshalerProtoMessage {
	return &ethpb.SignedValidatorRegistrationV1{}
}
func (BuilderBidCreator) Create() MarshalerProtoMessage         { return &ethpb.BuilderBid{} }
func (BuilderBidCapellaCreator) Create() MarshalerProtoMessage  { return &ethpb.BuilderBidCapella{} }
func (BuilderBidDenebCreator) Create() MarshalerProtoMessage    { return &ethpb.BuilderBidDeneb{} }
func (BlobSidecarCreator) Create() MarshalerProtoMessage        { return &ethpb.BlobSidecar{} }
func (BlobSidecarsCreator) Create() MarshalerProtoMessage       { return &ethpb.BlobSidecars{} }
func (Deposit_DataCreator) Create() MarshalerProtoMessage       { return &ethpb.Deposit_Data{} }
func (BeaconStateCreator) Create() MarshalerProtoMessage        { return &ethpb.BeaconState{} }
func (BeaconStateAltairCreator) Create() MarshalerProtoMessage  { return &ethpb.BeaconStateAltair{} }
func (ForkCreator) Create() MarshalerProtoMessage               { return &ethpb.Fork{} }
func (PendingAttestationCreator) Create() MarshalerProtoMessage { return &ethpb.PendingAttestation{} }
func (HistoricalBatchCreator) Create() MarshalerProtoMessage    { return &ethpb.HistoricalBatch{} }
func (SigningDataCreator) Create() MarshalerProtoMessage        { return &ethpb.SigningData{} }
func (ForkDataCreator) Create() MarshalerProtoMessage           { return &ethpb.ForkData{} }
func (DepositMessageCreator) Create() MarshalerProtoMessage     { return &ethpb.DepositMessage{} }
func (SyncCommitteeCreator) Create() MarshalerProtoMessage      { return &ethpb.SyncCommittee{} }
func (SyncAggregatorSelectionDataCreator) Create() MarshalerProtoMessage {
	return &ethpb.SyncAggregatorSelectionData{}
}
func (BeaconStateBellatrixCreator) Create() MarshalerProtoMessage {
	return &ethpb.BeaconStateBellatrix{}
}
func (BeaconStateCapellaCreator) Create() MarshalerProtoMessage { return &ethpb.BeaconStateCapella{} }
func (BeaconStateDenebCreator) Create() MarshalerProtoMessage   { return &ethpb.BeaconStateDeneb{} }
func (BeaconStateElectraCreator) Create() MarshalerProtoMessage { return &ethpb.BeaconStateElectra{} }
func (PowBlockCreator) Create() MarshalerProtoMessage           { return &ethpb.PowBlock{} }
func (HistoricalSummaryCreator) Create() MarshalerProtoMessage  { return &ethpb.HistoricalSummary{} }
func (BlobIdentifierCreator) Create() MarshalerProtoMessage     { return &ethpb.BlobIdentifier{} }
func (PendingDepositCreator) Create() MarshalerProtoMessage {
	return &ethpb.PendingDeposit{}
}
func (PendingPartialWithdrawalCreator) Create() MarshalerProtoMessage {
	return &ethpb.PendingPartialWithdrawal{}
}
func (PendingConsolidationCreator) Create() MarshalerProtoMessage {
	return &ethpb.PendingConsolidation{}
}
func (StatusCreator) Create() MarshalerProtoMessage { return &ethpb.Status{} }
func (BeaconBlocksByRangeRequestCreator) Create() MarshalerProtoMessage {
	return &ethpb.BeaconBlocksByRangeRequest{}
}
func (ENRForkIDCreator) Create() MarshalerProtoMessage  { return &ethpb.ENRForkID{} }
func (MetaDataV0Creator) Create() MarshalerProtoMessage { return &ethpb.MetaDataV0{} }
func (MetaDataV1Creator) Create() MarshalerProtoMessage { return &ethpb.MetaDataV1{} }
func (BlobSidecarsByRangeRequestCreator) Create() MarshalerProtoMessage {
	return &ethpb.BlobSidecarsByRangeRequest{}
}
func (DepositSnapshotCreator) Create() MarshalerProtoMessage { return &ethpb.DepositSnapshot{} }
func (SyncCommitteeMessageCreator) Create() MarshalerProtoMessage {
	return &ethpb.SyncCommitteeMessage{}
}
func (SyncCommitteeContributionCreator) Create() MarshalerProtoMessage {
	return &ethpb.SyncCommitteeContribution{}
}
func (ContributionAndProofCreator) Create() MarshalerProtoMessage {
	return &ethpb.ContributionAndProof{}
}
func (SignedContributionAndProofCreator) Create() MarshalerProtoMessage {
	return &ethpb.SignedContributionAndProof{}
}
func (ValidatorCreator) Create() MarshalerProtoMessage { return &ethpb.Validator{} }
func (BLSToExecutionChangeCreator) Create() MarshalerProtoMessage {
	return &ethpb.BLSToExecutionChange{}
}
func (SignedBLSToExecutionChangeCreator) Create() MarshalerProtoMessage {
	return &ethpb.SignedBLSToExecutionChange{}
}

var creators = []MarshalerProtoCreator{
	AttestationCreator{},
	AttestationElectraCreator{},
	AggregateAttestationAndProofCreator{},
	AggregateAttestationAndProofElectraCreator{},
	SignedAggregateAttestationAndProofCreator{},
	SignedAggregateAttestationAndProofElectraCreator{},
	AttestationDataCreator{},
	CheckpointCreator{},
	BeaconBlockCreator{},
	SignedBeaconBlockCreator{},
	BeaconBlockAltairCreator{},
	SignedBeaconBlockAltairCreator{},
	BeaconBlockBodyCreator{},
	BeaconBlockBodyAltairCreator{},
	ProposerSlashingCreator{},
	AttesterSlashingCreator{},
	AttesterSlashingElectraCreator{},
	DepositCreator{},
	VoluntaryExitCreator{},
	SignedVoluntaryExitCreator{},
	Eth1DataCreator{},
	BeaconBlockHeaderCreator{},
	SignedBeaconBlockHeaderCreator{},
	IndexedAttestationCreator{},
	IndexedAttestationElectraCreator{},
	SyncAggregateCreator{},
	SignedBeaconBlockBellatrixCreator{},
	BeaconBlockBellatrixCreator{},
	BeaconBlockBodyBellatrixCreator{},
	SignedBlindedBeaconBlockBellatrixCreator{},
	BlindedBeaconBlockBellatrixCreator{},
	BlindedBeaconBlockBodyBellatrixCreator{},
	SignedBeaconBlockContentsDenebCreator{},
	BeaconBlockContentsDenebCreator{},
	SignedBeaconBlockDenebCreator{},
	BeaconBlockDenebCreator{},
	BeaconBlockBodyDenebCreator{},
	SignedBeaconBlockCapellaCreator{},
	BeaconBlockCapellaCreator{},
	BeaconBlockBodyCapellaCreator{},
	SignedBlindedBeaconBlockCapellaCreator{},
	BlindedBeaconBlockCapellaCreator{},
	BlindedBeaconBlockBodyCapellaCreator{},
	SignedBlindedBeaconBlockDenebCreator{},
	BlindedBeaconBlockDenebCreator{},
	BlindedBeaconBlockBodyDenebCreator{},
	SignedBeaconBlockElectraCreator{},
	BeaconBlockElectraCreator{},
	BeaconBlockBodyElectraCreator{},
	SignedBlindedBeaconBlockElectraCreator{},
	BlindedBeaconBlockElectraCreator{},
	BlindedBeaconBlockBodyElectraCreator{},
	ValidatorRegistrationV1Creator{},
	SignedValidatorRegistrationV1Creator{},
	BuilderBidCreator{},
	BuilderBidCapellaCreator{},
	BuilderBidDenebCreator{},
	BlobSidecarCreator{},
	BlobSidecarsCreator{},
	Deposit_DataCreator{},
	BeaconStateCreator{},
	BeaconStateAltairCreator{},
	ForkCreator{},
	PendingAttestationCreator{},
	HistoricalBatchCreator{},
	SigningDataCreator{},
	ForkDataCreator{},
	DepositMessageCreator{},
	SyncCommitteeCreator{},
	SyncAggregatorSelectionDataCreator{},
	BeaconStateBellatrixCreator{},
	BeaconStateCapellaCreator{},
	BeaconStateDenebCreator{},
	BeaconStateElectraCreator{},
	PowBlockCreator{},
	HistoricalSummaryCreator{},
	BlobIdentifierCreator{},
	PendingDepositCreator{},
	PendingPartialWithdrawalCreator{},
	PendingConsolidationCreator{},
	StatusCreator{},
	BeaconBlocksByRangeRequestCreator{},
	ENRForkIDCreator{},
	MetaDataV0Creator{},
	MetaDataV1Creator{},
	BlobSidecarsByRangeRequestCreator{},
	DepositSnapshotCreator{},
	SyncCommitteeMessageCreator{},
	SyncCommitteeContributionCreator{},
	ContributionAndProofCreator{},
	SignedContributionAndProofCreator{},
	ValidatorCreator{},
	BLSToExecutionChangeCreator{},
	SignedBLSToExecutionChangeCreator{},
}

func assertProtoMessagesEqual(t *testing.T, decoded, msg proto.Message) {
	// Check if two proto messages are equal
	if proto.Equal(decoded, msg) {
		return
	}

	// If they are not equal, check if their unknown values are equal
	// Ignore unknown fields when comparing proto messages
	a := decoded.ProtoReflect().GetUnknown()
	b := msg.ProtoReflect().GetUnknown()
	if !bytes.Equal(a, b) {
		return
	}

	// If unknown values are equal, check if any of the fields of the proto message are proto messages themselves
	containsNestedProtoMessage := false
	decoded.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		// Check if the field is a message
		if fd.Kind() == protoreflect.MessageKind {
			containsNestedProtoMessage = true

			// Get the corresponding field from the other message
			otherValue := msg.ProtoReflect().Get(fd)

			// If the field is not set in the other message, skip it
			if !otherValue.IsValid() {
				return true
			}

			// Recursively compare the fields
			assertProtoMessagesEqual(t, v.Message().Interface(), otherValue.Message().Interface())
		}

		return true
	})

	// If there are no proto messages contained inside, then throw an error
	// The error is thrown iff the decoded message is not equal to the original message
	// after ignoring unknown fields in (nested) proto message(s).
	if !containsNestedProtoMessage {
		t.Log(cmp.Diff(decoded, msg, protocmp.Transform()))
		t.Fatal("Decoded message is not the same as original")
	}
}

// Refactor the unmarshal logic into a private function.
func unmarshalProtoMessage(data []byte, creator MarshalerProtoCreator) (MarshalerProtoMessage, error) {
	msg := creator.Create()
	if err := proto.Unmarshal(data, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func gossipRoundTripHelper(t *testing.T, msg MarshalerProtoMessage, creator MarshalerProtoCreator) {
	e := &encoder.SszNetworkEncoder{}
	buf := new(bytes.Buffer)

	// Example of calling a function that requires MarshalerProtoMessage
	_, err := e.EncodeGossip(buf, msg)
	if err != nil {
		t.Logf("Failed to encode: %v", err)
		return
	}

	decoded := creator.Create()

	if err := e.DecodeGossip(buf.Bytes(), decoded); err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}
	assertProtoMessagesEqual(t, decoded, msg)
}

func lengthRoundTripHelper(t *testing.T, msg MarshalerProtoMessage, creator MarshalerProtoCreator) {
	e := &encoder.SszNetworkEncoder{}
	buf := new(bytes.Buffer)

	// Example of calling a function that requires MarshalerProtoMessage
	_, err := e.EncodeWithMaxLength(buf, msg)
	if err != nil {
		t.Logf("Failed to encode: %v", err)
		return
	}

	decoded := creator.Create()

	if err := e.DecodeWithMaxLength(buf, decoded); err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}

	assertProtoMessagesEqual(t, decoded, msg)
}

func FuzzRoundTripWithGossip(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte, index int) {
		if index < 0 || index >= len(creators) {
			t.Skip()
		}
		// Select a random creator from the list.
		creator := creators[index]
		msg, err := unmarshalProtoMessage(data, creator)
		if err != nil {
			t.Logf("Failed to unmarshal: %v", err)
			return
		}
		gossipRoundTripHelper(t, msg, creator)
		lengthRoundTripHelper(t, msg, creator)
	})
}

func TestSszNetworkEncoder_RoundTrip_SignedVoluntaryExit(t *testing.T) {
	e := &encoder.SszNetworkEncoder{}
	buf := new(bytes.Buffer)

	data := []byte("\x12`000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000\n\n0000000000")
	msg := &ethpb.SignedVoluntaryExit{}

	if err := proto.Unmarshal(data, msg); err != nil {
		t.Logf("Failed to unmarshal: %v", err)
		return
	}

	_, err := e.EncodeGossip(buf, msg)
	require.NoError(t, err)
	decoded := &ethpb.SignedVoluntaryExit{}
	require.NoError(t, e.DecodeGossip(buf.Bytes(), decoded))
	assertProtoMessagesEqual(t, decoded, msg)
}

func TestSszNetworkEncoder_RoundTrip(t *testing.T) {
	e := &encoder.SszNetworkEncoder{}
	testRoundTripWithLength(t, e)
	testRoundTripWithGossip(t, e)
}

func TestSszNetworkEncoder_FailsSnappyLength(t *testing.T) {
	e := &encoder.SszNetworkEncoder{}
	att := &ethpb.Fork{}
	data := make([]byte, 32)
	binary.PutUvarint(data, encoder.MaxGossipSize+32)
	err := e.DecodeGossip(data, att)
	require.ErrorContains(t, "snappy message exceeds max size", err)
}

func testRoundTripWithLength(t *testing.T, e *encoder.SszNetworkEncoder) {
	buf := new(bytes.Buffer)
	msg := &ethpb.Fork{
		PreviousVersion: []byte("fooo"),
		CurrentVersion:  []byte("barr"),
		Epoch:           9001,
	}
	_, err := e.EncodeWithMaxLength(buf, msg)
	require.NoError(t, err)
	decoded := &ethpb.Fork{}
	require.NoError(t, e.DecodeWithMaxLength(buf, decoded))
	if !proto.Equal(decoded, msg) {
		t.Logf("decoded=%+v\n", decoded)
		t.Error("Decoded message is not the same as original")
	}
}

func testRoundTripWithGossip(t *testing.T, e *encoder.SszNetworkEncoder) {
	buf := new(bytes.Buffer)
	msg := &ethpb.Fork{
		PreviousVersion: []byte("fooo"),
		CurrentVersion:  []byte("barr"),
		Epoch:           9001,
	}
	_, err := e.EncodeGossip(buf, msg)
	require.NoError(t, err)
	decoded := &ethpb.Fork{}
	require.NoError(t, e.DecodeGossip(buf.Bytes(), decoded))
	if !proto.Equal(decoded, msg) {
		t.Logf("decoded=%+v\n", decoded)
		t.Error("Decoded message is not the same as original")
	}
}

func TestSszNetworkEncoder_EncodeWithMaxLength(t *testing.T) {
	buf := new(bytes.Buffer)
	msg := &ethpb.Fork{
		PreviousVersion: []byte("fooo"),
		CurrentVersion:  []byte("barr"),
		Epoch:           9001,
	}
	e := &encoder.SszNetworkEncoder{}
	params.SetupTestConfigCleanup(t)
	c := params.BeaconNetworkConfig()
	encoder.MaxChunkSize = uint64(5)
	params.OverrideBeaconNetworkConfig(c)
	_, err := e.EncodeWithMaxLength(buf, msg)
	wanted := fmt.Sprintf("which is larger than the provided max limit of %d", encoder.MaxChunkSize)
	assert.ErrorContains(t, wanted, err)
}

func TestSszNetworkEncoder_DecodeWithMaxLength(t *testing.T) {
	buf := new(bytes.Buffer)
	msg := &ethpb.Fork{
		PreviousVersion: []byte("fooo"),
		CurrentVersion:  []byte("barr"),
		Epoch:           4242,
	}
	e := &encoder.SszNetworkEncoder{}
	params.SetupTestConfigCleanup(t)
	c := params.BeaconNetworkConfig()
	maxChunkSize := uint64(5)
	encoder.MaxChunkSize = maxChunkSize
	params.OverrideBeaconNetworkConfig(c)
	_, err := e.EncodeGossip(buf, msg)
	require.NoError(t, err)
	decoded := &ethpb.Fork{}
	err = e.DecodeWithMaxLength(buf, decoded)
	wanted := fmt.Sprintf("goes over the provided max limit of %d", maxChunkSize)
	assert.ErrorContains(t, wanted, err)
}

func TestSszNetworkEncoder_DecodeWithMultipleFrames(t *testing.T) {
	buf := new(bytes.Buffer)
	st, _ := util.DeterministicGenesisState(t, 100)
	e := &encoder.SszNetworkEncoder{}
	params.SetupTestConfigCleanup(t)
	c := params.BeaconNetworkConfig()
	// 4 * 1 Mib
	maxChunkSize := uint64(1 << 22)
	encoder.MaxChunkSize = maxChunkSize
	params.OverrideBeaconNetworkConfig(c)
	_, err := e.EncodeWithMaxLength(buf, st.ToProtoUnsafe().(*ethpb.BeaconState))
	require.NoError(t, err)
	// Max snappy block size
	if buf.Len() <= 76490 {
		t.Errorf("buffer smaller than expected, wanted > %d but got %d", 76490, buf.Len())
	}
	decoded := new(ethpb.BeaconState)
	err = e.DecodeWithMaxLength(buf, decoded)
	assert.NoError(t, err)
}
func TestSszNetworkEncoder_NegativeMaxLength(t *testing.T) {
	e := &encoder.SszNetworkEncoder{}
	length, err := e.MaxLength(0xfffffffffff)

	assert.Equal(t, 0, length, "Received non zero length on bad message length")
	assert.ErrorContains(t, "max encoded length is negative", err)
}

func TestSszNetworkEncoder_MaxInt64(t *testing.T) {
	e := &encoder.SszNetworkEncoder{}
	length, err := e.MaxLength(math.MaxInt64 + 1)

	assert.Equal(t, 0, length, "Received non zero length on bad message length")
	assert.ErrorContains(t, "invalid length provided", err)
}

func TestSszNetworkEncoder_DecodeWithBadSnappyStream(t *testing.T) {
	st := newBadSnappyStream()
	e := &encoder.SszNetworkEncoder{}
	decoded := new(ethpb.Fork)
	err := e.DecodeWithMaxLength(st, decoded)
	assert.ErrorContains(t, io.EOF.Error(), err)
}

type badSnappyStream struct {
	varint []byte
	header []byte
	repeat []byte
	i      int
	// count how many times it was read
	counter int
	// count bytes read so far
	total int
}

func newBadSnappyStream() *badSnappyStream {
	const (
		magicBody  = "sNaPpY"
		magicChunk = "\xff\x06\x00\x00" + magicBody
	)

	header := make([]byte, len(magicChunk))
	// magicChunk == chunkTypeStreamIdentifier byte ++ 3 byte little endian len(magic body) ++ 6 byte magic body

	// header is a special chunk type, with small fixed length, to add some magic to claim it's really snappy.
	copy(header, magicChunk) // snappy library constants help us construct the common header chunk easily.

	payload := make([]byte, 4)

	// byte 0 is chunk type
	// Exploit any fancy ignored chunk type
	//   Section 4.4 Padding (chunk type 0xfe).
	//   Section 4.6. Reserved skippable chunks (chunk types 0x80-0xfd).
	payload[0] = 0xfe

	// byte 1,2,3 are chunk length (little endian)
	payload[1] = 0
	payload[2] = 0
	payload[3] = 0

	return &badSnappyStream{
		varint:  gogo.EncodeVarint(1000),
		header:  header,
		repeat:  payload,
		i:       0,
		counter: 0,
		total:   0,
	}
}

func (b *badSnappyStream) Read(p []byte) (n int, err error) {
	// Stream out varint bytes first to make test happy.
	if len(b.varint) > 0 {
		copy(p, b.varint[:1])
		b.varint = b.varint[1:]
		return 1, nil
	}
	defer func() {
		b.counter += 1
		b.total += n
	}()
	if len(b.repeat) == 0 {
		panic("no bytes to repeat")
	}
	if len(b.header) > 0 {
		n = copy(p, b.header)
		b.header = b.header[n:]
		return
	}
	for n < len(p) {
		n += copy(p[n:], b.repeat[b.i:])
		b.i = (b.i + n) % len(b.repeat)
	}
	return
}
