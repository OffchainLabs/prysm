package ssz_static

import (
	"context"
	"errors"
	"testing"

	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	common "github.com/OffchainLabs/prysm/v6/testing/spectest/shared/common/ssz_static"
	fssz "github.com/prysmaticlabs/fastssz"
)

// RunSSZStaticTests executes "ssz_static" tests.
func RunSSZStaticTests(t *testing.T, config string) {
	common.RunSSZStaticTests(t, config, "altair", unmarshalledSSZ, customHtr)
}

func customHtr(t *testing.T, htrs []common.HTR, object interface{}) []common.HTR {
	switch object.(type) {
	case *ethpb.BeaconStateAltair:
		htrs = append(htrs, func(s interface{}) ([32]byte, error) {
			beaconState, err := state_native.InitializeFromProtoAltair(s.(*ethpb.BeaconStateAltair))
			require.NoError(t, err)
			return beaconState.HashTreeRoot(context.Background())
		})
	}
	return htrs
}

// unmarshalledSSZ unmarshalls serialized input.
func unmarshalledSSZ(t *testing.T, serializedBytes []byte, folderName string) (interface{}, error) {
	var obj interface{}
	switch folderName {
	case "Attestation":
		obj = &ethpb.Attestation{}
	case "AttestationData":
		obj = &ethpb.AttestationData{}
	case "AttesterSlashing":
		obj = &ethpb.AttesterSlashing{}
	case "AggregateAndProof":
		obj = &ethpb.AggregateAttestationAndProof{}
	case "BeaconBlock":
		obj = &ethpb.BeaconBlockAltair{}
	case "BeaconBlockBody":
		obj = &ethpb.BeaconBlockBodyAltair{}
	case "BeaconBlockHeader":
		obj = &ethpb.BeaconBlockHeader{}
	case "BeaconState":
		obj = &ethpb.BeaconStateAltair{}
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
		return nil, nil
	case "Fork":
		obj = &ethpb.Fork{}
	case "ForkData":
		obj = &ethpb.ForkData{}
	case "HistoricalBatch":
		obj = &ethpb.HistoricalBatch{}
	case "IndexedAttestation":
		obj = &ethpb.IndexedAttestation{}
	case "LightClientHeader":
		obj = &ethpb.LightClientHeaderAltair{}
	case "PendingAttestation":
		obj = &ethpb.PendingAttestation{}
	case "ProposerSlashing":
		obj = &ethpb.ProposerSlashing{}
	case "SignedAggregateAndProof":
		obj = &ethpb.SignedAggregateAttestationAndProof{}
	case "SignedBeaconBlock":
		obj = &ethpb.SignedBeaconBlockAltair{}
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
	case "SyncAggregate":
		obj = &ethpb.SyncAggregate{}
	case "SyncAggregatorSelectionData":
		obj = &ethpb.SyncAggregatorSelectionData{}
	case "SyncCommittee":
		obj = &ethpb.SyncCommittee{}
	case "LightClientOptimisticUpdate":
		obj = &ethpb.LightClientOptimisticUpdateAltair{}
	case "LightClientFinalityUpdate":
		obj = &ethpb.LightClientFinalityUpdateAltair{}
	case "LightClientBootstrap":
		obj = &ethpb.LightClientBootstrapAltair{}
	case "LightClientUpdate":
		obj = &ethpb.LightClientUpdateAltair{}
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
