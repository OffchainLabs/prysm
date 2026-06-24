package beacon_api

import (
	"github.com/OffchainLabs/prysm/v7/api/apiutil"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func jsonifyAttestations(attestations []*ethpb.Attestation) []*structs.Attestation {
	jsonAttestations := make([]*structs.Attestation, len(attestations))
	for index, attestation := range attestations {
		jsonAttestations[index] = jsonifyAttestation(attestation)
	}
	return jsonAttestations
}

func jsonifySingleAttestations(attestations []*ethpb.SingleAttestation) []*structs.SingleAttestation {
	jsonAttestations := make([]*structs.SingleAttestation, len(attestations))
	for index, attestation := range attestations {
		jsonAttestations[index] = jsonifySingleAttestation(attestation)
	}
	return jsonAttestations
}

func jsonifyBatchAttestation(attestation *ethpb.BatchAttestation) *structs.BatchAttestation {
	return structs.BatchAttFromConsensus(attestation)
}

// JsonifySignedVoluntaryExits converts an array of voluntary exit structs to a JSON hex string compatible format.
func JsonifySignedVoluntaryExits(voluntaryExits []*ethpb.SignedVoluntaryExit) []*structs.SignedVoluntaryExit {
	jsonSignedVoluntaryExits := make([]*structs.SignedVoluntaryExit, len(voluntaryExits))
	for index, signedVoluntaryExit := range voluntaryExits {
		jsonSignedVoluntaryExit := &structs.SignedVoluntaryExit{
			Message: &structs.VoluntaryExit{
				Epoch:          apiutil.Uint64ToString(signedVoluntaryExit.Exit.Epoch),
				ValidatorIndex: apiutil.Uint64ToString(signedVoluntaryExit.Exit.ValidatorIndex),
			},
			Signature: hexutil.Encode(signedVoluntaryExit.Signature),
		}
		jsonSignedVoluntaryExits[index] = jsonSignedVoluntaryExit
	}
	return jsonSignedVoluntaryExits
}

func jsonifyAttestationData(attestationData *ethpb.AttestationData) *structs.AttestationData {
	return &structs.AttestationData{
		BeaconBlockRoot: hexutil.Encode(attestationData.BeaconBlockRoot),
		CommitteeIndex:  apiutil.Uint64ToString(attestationData.CommitteeIndex),
		Slot:            apiutil.Uint64ToString(attestationData.Slot),
		Source: &structs.Checkpoint{
			Epoch: apiutil.Uint64ToString(attestationData.Source.Epoch),
			Root:  hexutil.Encode(attestationData.Source.Root),
		},
		Target: &structs.Checkpoint{
			Epoch: apiutil.Uint64ToString(attestationData.Target.Epoch),
			Root:  hexutil.Encode(attestationData.Target.Root),
		},
	}
}

func jsonifyAttestation(attestation *ethpb.Attestation) *structs.Attestation {
	return &structs.Attestation{
		AggregationBits: hexutil.Encode(attestation.AggregationBits),
		Data:            jsonifyAttestationData(attestation.Data),
		Signature:       hexutil.Encode(attestation.Signature),
	}
}

func jsonifyAttestationElectra(attestation *ethpb.AttestationElectra) *structs.AttestationElectra {
	return &structs.AttestationElectra{
		AggregationBits: hexutil.Encode(attestation.AggregationBits),
		Data:            jsonifyAttestationData(attestation.Data),
		Signature:       hexutil.Encode(attestation.Signature),
		CommitteeBits:   hexutil.Encode(attestation.CommitteeBits),
	}
}

func jsonifySingleAttestation(attestation *ethpb.SingleAttestation) *structs.SingleAttestation {
	return &structs.SingleAttestation{
		CommitteeIndex: apiutil.Uint64ToString(attestation.CommitteeId),
		AttesterIndex:  apiutil.Uint64ToString(attestation.AttesterIndex),
		Data:           jsonifyAttestationData(attestation.Data),
		Signature:      hexutil.Encode(attestation.Signature),
	}
}

func jsonifySignedAggregateAndProof(signedAggregateAndProof *ethpb.SignedAggregateAttestationAndProof) *structs.SignedAggregateAttestationAndProof {
	return &structs.SignedAggregateAttestationAndProof{
		Message: &structs.AggregateAttestationAndProof{
			AggregatorIndex: apiutil.Uint64ToString(signedAggregateAndProof.Message.AggregatorIndex),
			Aggregate:       jsonifyAttestation(signedAggregateAndProof.Message.Aggregate),
			SelectionProof:  hexutil.Encode(signedAggregateAndProof.Message.SelectionProof),
		},
		Signature: hexutil.Encode(signedAggregateAndProof.Signature),
	}
}

func jsonifySignedAggregateAndProofElectra(signedAggregateAndProof *ethpb.SignedAggregateAttestationAndProofElectra) *structs.SignedAggregateAttestationAndProofElectra {
	return &structs.SignedAggregateAttestationAndProofElectra{
		Message: &structs.AggregateAttestationAndProofElectra{
			AggregatorIndex: apiutil.Uint64ToString(signedAggregateAndProof.Message.AggregatorIndex),
			Aggregate:       jsonifyAttestationElectra(signedAggregateAndProof.Message.Aggregate),
			SelectionProof:  hexutil.Encode(signedAggregateAndProof.Message.SelectionProof),
		},
		Signature: hexutil.Encode(signedAggregateAndProof.Signature),
	}
}
