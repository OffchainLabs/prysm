package ssz_static

import (
	"errors"
	"fmt"
	"testing"

	"github.com/OffchainLabs/methodical-ssz/ssz"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	common "github.com/OffchainLabs/prysm/v7/testing/spectest/shared/common/ssz_static"
)

// RunSSZStaticTests executes "ssz_static" tests.
func RunSSZStaticTests(t *testing.T, config string) {
	common.RunSSZStaticTests(t, config, "heze", unmarshalledSSZ, nil)
}

// unmarshalledSSZ unmarshalls serialized input.
func unmarshalledSSZ(t *testing.T, serializedBytes []byte, folderName string) (any, error) {
	var obj any

	switch folderName {
	// TODO(heze): Un-skip these types when EIP-8015 removes the eth1 fields.
	case "BeaconBlock", "BeaconBlockBody", "BeaconState", "SignedBeaconBlock":
		t.Skip("eth1 fields stay until EIP-8015")

	// Heze specific types
	case "ExecutionPayloadBid":
		obj = &ethpb.ExecutionPayloadBidHeze{}
	case "SignedExecutionPayloadBid":
		obj = &ethpb.SignedExecutionPayloadBidHeze{}
	case "InclusionList":
		obj = &ethpb.InclusionList{}
	case "SignedInclusionList":
		obj = &ethpb.SignedInclusionList{}

	// Gloas types that also exist in heze
	case "PayloadAttestationData":
		obj = &ethpb.PayloadAttestationData{}
	case "PayloadAttestation":
		obj = &ethpb.PayloadAttestation{}
	case "PayloadAttestationMessage":
		obj = &ethpb.PayloadAttestationMessage{}
	case "Builder":
		obj = &ethpb.Builder{}
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
	case "IndexedPayloadAttestation", "NewPayloadRequest":
		t.Skip("Not a consensus type")
	case "DataColumnSidecar":
		obj = &ethpb.DataColumnSidecarGloas{}
	case "SignedProposerPreferences", "ProposerPreferences":
		t.Skip("p2p-only type; not part of the consensus state transition")
	case "PartialDataColumnGroupID":
		obj = &ethpb.PartialDataColumnGroupID{}
	case "PartialDataColumnSidecar":
		obj = &ethpb.PartialDataColumnSidecarGloas{}

	// Standard types that also exist in heze
	case "ExecutionPayload":
		obj = &enginev1.ExecutionPayloadGloas{}
	case "ExecutionPayloadHeader":
		obj = &enginev1.ExecutionPayloadHeaderDeneb{}
	case "Attestation":
		obj = &ethpb.AttestationGloas{}
	case "AttestationData":
		obj = &ethpb.AttestationData{}
	case "AttesterSlashing":
		obj = &ethpb.AttesterSlashingGloas{}
	case "AggregateAndProof":
		obj = &ethpb.AggregateAttestationAndProofGloas{}
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
		obj = &ethpb.IndexedAttestationGloas{}
	case "PendingAttestation":
		obj = &ethpb.PendingAttestation{}
	case "ProposerSlashing":
		obj = &ethpb.ProposerSlashing{}
	case "SignedAggregateAndProof":
		obj = &ethpb.SignedAggregateAttestationAndProofGloas{}
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
		obj = &ethpb.LightClientOptimisticUpdateGloas{}
	case "LightClientFinalityUpdate":
		obj = &ethpb.LightClientFinalityUpdateGloas{}
	case "LightClientBootstrap":
		obj = &ethpb.LightClientBootstrapGloas{}
	case "LightClientUpdate":
		obj = &ethpb.LightClientUpdateGloas{}
	case "LightClientHeader":
		obj = &ethpb.LightClientHeaderGloas{}
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
	case "BuilderDepositRequest":
		obj = &enginev1.BuilderDepositRequest{}
	case "BuilderExitRequest":
		obj = &enginev1.BuilderExitRequest{}
	case "ExecutionRequests":
		obj = &enginev1.ExecutionRequestsGloas{}
	case "DataColumnsByRootIdentifier":
		obj = &ethpb.DataColumnsByRootIdentifier{}
	case "MatrixEntry":
		t.Skip("Unused type")
	case "PartialDataColumnHeader", "PartialDataColumnPartsMetadata":
		t.Skip("Not yet implemented")
	default:
		return nil, errors.New("type not found")
	}

	var err error
	if o, ok := obj.(ssz.Unmarshaler); ok {
		err = o.UnmarshalSSZ(serializedBytes)
	} else {
		err = fmt.Errorf("selected type %T for folder %s does not implement ssz.Unmarshaler", obj, folderName)
	}

	return obj, err
}
