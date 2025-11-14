package ssz_static

import (
	"errors"
	"testing"

	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	common "github.com/OffchainLabs/prysm/v7/testing/spectest/shared/common/ssz_static"
	fssz "github.com/prysmaticlabs/fastssz"
)

// RunSSZStaticTests executes "ssz_static" tests.
func RunSSZStaticTests(t *testing.T, config string) {
	common.RunSSZStaticTests(t, config, "gloas", unmarshalledSSZ, customHtr)
}

func customHtr(t *testing.T, htrs []common.HTR, object any) []common.HTR {
	// TODO: Add custom HTR for BeaconStateGloas when state-native support is implemented
	// For now, only use the default fastssz HTR methods
	return htrs
}

// unmarshalledSSZ unmarshalls serialized input.
func unmarshalledSSZ(t *testing.T, serializedBytes []byte, folderName string) (any, error) {
	var obj any

	switch folderName {
	// Gloas specific types
	case "ExecutionPayloadBid":
		obj = &ethpb.ExecutionPayloadBid{}
	case "SignedExecutionPayloadBid":
		obj = &ethpb.SignedExecutionPayloadBid{}
	case "PayloadAttestationData":
		obj = &ethpb.PayloadAttestationData{}
	case "PayloadAttestation":
		obj = &ethpb.PayloadAttestation{}
	case "PayloadAttestationMessage":
		obj = &ethpb.PayloadAttestationMessage{}
	case "BeaconBlock":
		obj = &ethpb.BeaconBlockGloas{}
	case "BeaconBlockBody":
		obj = &ethpb.BeaconBlockBodyGloas{}
	case "BeaconState":
		obj = &ethpb.BeaconStateGloas{}
	case "BuilderPendingPayment":
		obj = &ethpb.BuilderPendingPayment{}
	case "BuilderPendingWithdrawal":
		obj = &ethpb.BuilderPendingWithdrawal{}
	case "ExecutionPayloadEnvelope":
		obj = &ethpb.ExecutionPayloadEnvelope{}
	case "SignedExecutionPayloadEnvelope":
		obj = &ethpb.SignedExecutionPayloadEnvelope{}
	case "ForkChoiceNode":
		t.Skip("Not a consensus type")
	case "IndexedPayloadAttestation":
		t.Skip("Not a consensus type")
	case "DataColumnSidecar":
		obj = &ethpb.DataColumnSidecarGloas{}

	// Standard types that also exist in gloas
	case "ExecutionPayload":
		obj = &enginev1.ExecutionPayloadDeneb{}
	case "ExecutionPayloadHeader":
		obj = &enginev1.ExecutionPayloadHeaderDeneb{}
	case "Attestation":
		obj = &ethpb.AttestationElectra{}
	case "AttestationData":
		obj = &ethpb.AttestationData{}
	case "AttesterSlashing":
		obj = &ethpb.AttesterSlashingElectra{}
	case "AggregateAndProof":
		obj = &ethpb.AggregateAttestationAndProofElectra{}
	case "BeaconBlockHeader":
		obj = &ethpb.BeaconBlockHeader{}
	case "Checkpoint":
		obj = &ethpb.Checkpoint{}
	case "Deposit":
		obj = &ethpb.Deposit{}
	case "DepositMessage":
		obj = &ethpb.DepositMessage{}
	case "DepositData":
		obj = &ethpb.Deposit_Data{}
	case "Eth1Data":
		obj = &ethpb.Eth1Data{}
	case "Eth1Block":
		t.Skip("Unused type")
	case "Fork":
		obj = &ethpb.Fork{}
	case "ForkData":
		obj = &ethpb.ForkData{}
	case "HistoricalBatch":
		obj = &ethpb.HistoricalBatch{}
	case "IndexedAttestation":
		obj = &ethpb.IndexedAttestationElectra{}
	case "PendingAttestation":
		obj = &ethpb.PendingAttestation{}
	case "ProposerSlashing":
		obj = &ethpb.ProposerSlashing{}
	case "SignedAggregateAndProof":
		obj = &ethpb.SignedAggregateAttestationAndProofElectra{}
	case "SignedBeaconBlock":
		obj = &ethpb.SignedBeaconBlockGloas{}
	case "SignedBeaconBlockHeader":
		obj = &ethpb.SignedBeaconBlockHeader{}
	case "SignedVoluntaryExit":
		obj = &ethpb.SignedVoluntaryExit{}
	case "SigningData":
		obj = &ethpb.SigningData{}
	case "Validator":
		obj = &ethpb.Validator{}
	case "VoluntaryExit":
		obj = &ethpb.VoluntaryExit{}
	case "SyncCommitteeMessage":
		obj = &ethpb.SyncCommitteeMessage{}
	case "SyncCommitteeContribution":
		obj = &ethpb.SyncCommitteeContribution{}
	case "ContributionAndProof":
		obj = &ethpb.ContributionAndProof{}
	case "SignedContributionAndProof":
		obj = &ethpb.SignedContributionAndProof{}
	case "SingleAttestation":
		obj = &ethpb.SingleAttestation{}
	case "SyncAggregate":
		obj = &ethpb.SyncAggregate{}
	case "SyncAggregatorSelectionData":
		obj = &ethpb.SyncAggregatorSelectionData{}
	case "SyncCommittee":
		obj = &ethpb.SyncCommittee{}
	case "LightClientOptimisticUpdate":
		obj = &ethpb.LightClientOptimisticUpdateDeneb{}
	case "LightClientFinalityUpdate":
		obj = &ethpb.LightClientFinalityUpdateElectra{}
	case "LightClientBootstrap":
		obj = &ethpb.LightClientBootstrapElectra{}
	case "LightClientUpdate":
		obj = &ethpb.LightClientUpdateElectra{}
	case "LightClientHeader":
		obj = &ethpb.LightClientHeaderDeneb{}
	case "BlobIdentifier":
		obj = &ethpb.BlobIdentifier{}
	case "BlobSidecar":
		t.Skip("Unused type")
	case "PowBlock":
		obj = &ethpb.PowBlock{}
	case "Withdrawal":
		obj = &enginev1.Withdrawal{}
	case "HistoricalSummary":
		obj = &ethpb.HistoricalSummary{}
	case "BLSToExecutionChange":
		obj = &ethpb.BLSToExecutionChange{}
	case "SignedBLSToExecutionChange":
		obj = &ethpb.SignedBLSToExecutionChange{}
	case "PendingDeposit":
		obj = &ethpb.PendingDeposit{}
	case "PendingPartialWithdrawal":
		obj = &ethpb.PendingPartialWithdrawal{}
	case "PendingConsolidation":
		obj = &ethpb.PendingConsolidation{}
	case "WithdrawalRequest":
		obj = &enginev1.WithdrawalRequest{}
	case "DepositRequest":
		obj = &enginev1.DepositRequest{}
	case "ConsolidationRequest":
		obj = &enginev1.ConsolidationRequest{}
	case "ExecutionRequests":
		obj = &enginev1.ExecutionRequests{}
	case "DataColumnsByRootIdentifier":
		obj = &ethpb.DataColumnsByRootIdentifier{}
	case "MatrixEntry":
		t.Skip("Unused type")
	default:
		return nil, errors.New("type not found")
	}

	var err error
	if o, ok := obj.(fssz.Unmarshaler); ok {
		err = o.UnmarshalSSZ(serializedBytes)
	} else {
		err = errors.New("could not unmarshal object, not a fastssz compatible object")
	}

	return obj, err
}
