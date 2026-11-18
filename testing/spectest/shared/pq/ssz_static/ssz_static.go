package ssz_static

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/OffchainLabs/methodical-ssz/ssz"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	common "github.com/OffchainLabs/prysm/v7/testing/spectest/shared/common/ssz_static"
)

// RunSSZStaticTests executes "ssz_static" tests.
func RunSSZStaticTests(t *testing.T, config string) {
	common.RunSSZStaticTests(t, config, "pq", unmarshalledSSZ, customHtr)
}

func customHtr(t *testing.T, htrs []common.HTR, object any) []common.HTR {
	switch object.(type) {
	case *ethpb.BeaconStatePQ:
	case *ethpb.BeaconBlockPQ:
		return append(htrs, func(s any) ([32]byte, error) {
			blk, err := blocks.NewBeaconBlock(s.(*ethpb.BeaconBlockPQ))
			require.NoError(t, err)
			return blk.HashTreeRoot()
		})
	case *ethpb.SignedBeaconBlockPQ:
		return append(htrs, func(s any) ([32]byte, error) {
			signed, err := blocks.NewSignedBeaconBlock(s.(*ethpb.SignedBeaconBlockPQ))
			require.NoError(t, err)
			cp, err := signed.Copy()
			require.NoError(t, err)
			want, err := s.(*ethpb.SignedBeaconBlockPQ).MarshalSSZ()
			require.NoError(t, err)
			got, err := cp.MarshalSSZ()
			require.NoError(t, err)
			require.DeepEqual(t, want, got)
			pb, err := cp.Proto()
			require.NoError(t, err)
			return pb.(*ethpb.SignedBeaconBlockPQ).HashTreeRoot()
		})
	default:
		return htrs
	}
	return append(htrs, func(s any) ([32]byte, error) {
		beaconState, err := state_native.New(s.(*ethpb.BeaconStatePQ))
		require.NoError(t, err)
		roundTrip, ok := beaconState.ToContainer().(*ethpb.BeaconStatePQ)
		require.Equal(t, true, ok)
		want, err := s.(*ethpb.BeaconStatePQ).MarshalSSZ()
		require.NoError(t, err)
		got, err := roundTrip.MarshalSSZ()
		require.NoError(t, err)
		require.DeepEqual(t, want, got)
		return beaconState.HashTreeRoot(context.Background())
	})
}

// unmarshalledSSZ unmarshalls serialized input.
func unmarshalledSSZ(t *testing.T, serializedBytes []byte, folderName string) (any, error) {
	var obj any

	switch folderName {
	case "BeaconBlock":
		obj = &ethpb.BeaconBlockPQ{}
	case "BeaconBlockBody":
		obj = &ethpb.BeaconBlockBodyPQ{}
	case "BeaconState":
		obj = &ethpb.BeaconStatePQ{}
	case "SignedBeaconBlock":
		obj = &ethpb.SignedBeaconBlockPQ{}
	case "ExecutionPayloadBid":
		obj = &ethpb.ExecutionPayloadBid{}
	case "SignedExecutionPayloadBid":
		obj = &ethpb.SignedExecutionPayloadBidPQ{}
	case "ValidatorPreregistrationRequest":
		obj = &enginev1.ValidatorPreregistrationRequest{}
	case "BuilderPreregistrationRequest":
		obj = &enginev1.BuilderPreregistrationRequest{}
	case "VoluntaryExitRequest":
		obj = &enginev1.VoluntaryExitRequest{}
	case "ValidatorPreregistration":
		obj = &ethpb.ValidatorPreregistration{}
	case "BuilderPreregistration":
		obj = &ethpb.BuilderPreregistration{}

	// Gloas types that also exist in pq
	case "PayloadAttestationData":
		obj = &ethpb.PayloadAttestationData{}
	case "PayloadAttestation":
		obj = &ethpb.PayloadAttestation{}
	case "PayloadAttestationMessage":
		obj = &ethpb.PayloadAttestationMessagePQ{}
	case "Builder":
		obj = &ethpb.BuilderPQ{}
	case "BuilderPendingPayment":
		obj = &ethpb.BuilderPendingPayment{}
	case "BuilderPendingWithdrawal":
		obj = &ethpb.BuilderPendingWithdrawal{}
	case "ExecutionPayloadEnvelope":
		obj = &ethpb.ExecutionPayloadEnvelopePQ{}
	case "SignedExecutionPayloadEnvelope":
		obj = &ethpb.SignedExecutionPayloadEnvelopePQ{}
	case "ForkChoiceNode":
		t.Skip("Not a consensus type")
	case "IndexedPayloadAttestation", "NewPayloadRequest":
		t.Skip("Not a consensus type")
	case "DataColumnSidecar":
		obj = &ethpb.DataColumnSidecarGloas{}
	case "ProposerPreferences":
		obj = &ethpb.ProposerPreferences{}
	case "SignedProposerPreferences":
		obj = &ethpb.SignedProposerPreferencesPQ{}
	case "PartialDataColumnGroupID":
		obj = &ethpb.PartialDataColumnGroupID{}
	case "PartialDataColumnSidecar":
		obj = &ethpb.PartialDataColumnSidecarGloas{}

	// Standard types that also exist in pq
	case "ExecutionPayload":
		obj = &enginev1.ExecutionPayloadGloas{}
	case "ExecutionPayloadHeader":
		obj = &enginev1.ExecutionPayloadHeaderDeneb{}
	case "Attestation":
		obj = &ethpb.AttestationPQ{}
	case "AttestationData":
		obj = &ethpb.AttestationData{}
	case "AttesterSlashing":
		obj = &ethpb.AttesterSlashingPQ{}
	case "AggregateAndProof":
		obj = &ethpb.AggregateAttestationAndProofPQ{}
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
		obj = &ethpb.IndexedAttestationPQ{}
	case "PendingAttestation":
		obj = &ethpb.PendingAttestation{}
	case "ProposerSlashing":
		obj = &ethpb.ProposerSlashingPQ{}
	case "SignedAggregateAndProof":
		obj = &ethpb.SignedAggregateAttestationAndProofPQ{}
	case "SignedBeaconBlockHeader":
		obj = &ethpb.SignedBeaconBlockHeaderPQ{}
	case "SignedVoluntaryExit":
		obj = &ethpb.SignedVoluntaryExit{}
	case "SigningData":
		obj = &ethpb.SigningData{}
	case "Validator":
		obj = &ethpb.ValidatorPQ{}
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
		obj = &ethpb.SingleAttestationPQ{}
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
		obj = &enginev1.ExecutionRequestsPQ{}
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
