package beacon_api

import (
	"strconv"

	"github.com/OffchainLabs/prysm/v6/api/apiutil"
	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func jsonifyTransactions(transactions [][]byte) []string {
	jsonTransactions := make([]string, len(transactions))
	for index, transaction := range transactions {
		jsonTransaction := hexutil.Encode(transaction)
		jsonTransactions[index] = jsonTransaction
	}
	return jsonTransactions
}

func jsonifyEth1Data(eth1Data *ethpb.Eth1Data) *structs.Eth1Data {
	return &structs.Eth1Data{
		BlockHash:    hexutil.Encode(eth1Data.BlockHash),
		DepositCount: apiutil.Uint64ToString(eth1Data.DepositCount),
		DepositRoot:  hexutil.Encode(eth1Data.DepositRoot),
	}
}

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

func jsonifySignedBeaconBlockHeader(signedBeaconBlockHeader *ethpb.SignedBeaconBlockHeader) *structs.SignedBeaconBlockHeader {
	return &structs.SignedBeaconBlockHeader{
		Message: &structs.BeaconBlockHeader{
			BodyRoot:      hexutil.Encode(signedBeaconBlockHeader.Header.BodyRoot),
			ParentRoot:    hexutil.Encode(signedBeaconBlockHeader.Header.ParentRoot),
			ProposerIndex: apiutil.Uint64ToString(signedBeaconBlockHeader.Header.ProposerIndex),
			Slot:          apiutil.Uint64ToString(signedBeaconBlockHeader.Header.Slot),
			StateRoot:     hexutil.Encode(signedBeaconBlockHeader.Header.StateRoot),
		},
		Signature: hexutil.Encode(signedBeaconBlockHeader.Signature),
	}
}

func jsonifyIndexedAttestation(indexedAttestation *ethpb.IndexedAttestation) *structs.IndexedAttestation {
	attestingIndices := make([]string, len(indexedAttestation.AttestingIndices))
	for index, attestingIndex := range indexedAttestation.AttestingIndices {
		attestingIndex := apiutil.Uint64ToString(attestingIndex)
		attestingIndices[index] = attestingIndex
	}

	return &structs.IndexedAttestation{
		AttestingIndices: attestingIndices,
		Data:             jsonifyAttestationData(indexedAttestation.Data),
		Signature:        hexutil.Encode(indexedAttestation.Signature),
	}
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

func jsonifyWithdrawals(withdrawals []*enginev1.Withdrawal) []*structs.Withdrawal {
	jsonWithdrawals := make([]*structs.Withdrawal, len(withdrawals))
	for index, withdrawal := range withdrawals {
		jsonWithdrawals[index] = &structs.Withdrawal{
			WithdrawalIndex:  strconv.FormatUint(withdrawal.Index, 10),
			ValidatorIndex:   strconv.FormatUint(uint64(withdrawal.ValidatorIndex), 10),
			ExecutionAddress: hexutil.Encode(withdrawal.Address),
			Amount:           strconv.FormatUint(withdrawal.Amount, 10),
		}
	}
	return jsonWithdrawals
}
