package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/network/httputil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

func (c *beaconApiValidatorClient) proposeBeaconBlock(ctx context.Context, in *ethpb.GenericSignedBeaconBlock) (*ethpb.ProposeResponse, error) {
	var consensusVersion string
	var beaconBlockRoot [32]byte

	var err error
	var marshalledSignedBeaconBlockJson []byte
	blinded := false

	switch blockType := in.Block.(type) {
	case *ethpb.GenericSignedBeaconBlock_Phase0:
		consensusVersion = "phase0"
		beaconBlockRoot, err = blockType.Phase0.Block.HashTreeRoot()
		if err != nil {
			return nil, errors.Wrap(err, "failed to compute block root for phase0 beacon block")
		}

		marshalledSignedBeaconBlockJson, err = marshallBeaconBlockPhase0(blockType.Phase0)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshall phase0 beacon block")
		}
	case *ethpb.GenericSignedBeaconBlock_Altair:
		consensusVersion = "altair"
		beaconBlockRoot, err = blockType.Altair.Block.HashTreeRoot()
		if err != nil {
			return nil, errors.Wrap(err, "failed to compute block root for altair beacon block")
		}

		marshalledSignedBeaconBlockJson, err = marshallBeaconBlockAltair(blockType.Altair)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshall altair beacon block")
		}
	case *ethpb.GenericSignedBeaconBlock_Bellatrix:
		consensusVersion = "bellatrix"
		beaconBlockRoot, err = blockType.Bellatrix.Block.HashTreeRoot()
		if err != nil {
			return nil, errors.Wrap(err, "failed to compute block root for bellatrix beacon block")
		}

		marshalledSignedBeaconBlockJson, err = marshallBeaconBlockBellatrix(blockType.Bellatrix)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshall bellatrix beacon block")
		}
	case *ethpb.GenericSignedBeaconBlock_BlindedBellatrix:
		blinded = true
		consensusVersion = "bellatrix"
		beaconBlockRoot, err = blockType.BlindedBellatrix.Block.HashTreeRoot()
		if err != nil {
			return nil, errors.Wrap(err, "failed to compute block root for blinded bellatrix beacon block")
		}

		marshalledSignedBeaconBlockJson, err = marshallBeaconBlockBlindedBellatrix(blockType.BlindedBellatrix)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshall blinded bellatrix beacon block")
		}
	case *ethpb.GenericSignedBeaconBlock_Capella:
		consensusVersion = "capella"
		beaconBlockRoot, err = blockType.Capella.Block.HashTreeRoot()
		if err != nil {
			return nil, errors.Wrap(err, "failed to compute block root for capella beacon block")
		}

		marshalledSignedBeaconBlockJson, err = marshallBeaconBlockCapella(blockType.Capella)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshall capella beacon block")
		}
	case *ethpb.GenericSignedBeaconBlock_BlindedCapella:
		blinded = true
		consensusVersion = "capella"
		beaconBlockRoot, err = blockType.BlindedCapella.Block.HashTreeRoot()
		if err != nil {
			return nil, errors.Wrap(err, "failed to compute block root for blinded capella beacon block")
		}

		marshalledSignedBeaconBlockJson, err = marshallBeaconBlockBlindedCapella(blockType.BlindedCapella)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshall blinded capella beacon block")
		}
	case *ethpb.GenericSignedBeaconBlock_Deneb:
		consensusVersion = "deneb"
		beaconBlockRoot, err = blockType.Deneb.Block.HashTreeRoot()
		if err != nil {
			return nil, errors.Wrap(err, "failed to compute block root for deneb beacon block")
		}
		signedBlock, err := structs.SignedBeaconBlockContentsDenebFromConsensus(blockType.Deneb)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert deneb beacon block contents")
		}
		marshalledSignedBeaconBlockJson, err = json.Marshal(signedBlock)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal deneb beacon block contents")
		}
	case *ethpb.GenericSignedBeaconBlock_BlindedDeneb:
		blinded = true
		consensusVersion = "deneb"
		beaconBlockRoot, err = blockType.BlindedDeneb.HashTreeRoot()
		if err != nil {
			return nil, errors.Wrap(err, "failed to compute block root for blinded deneb beacon block")
		}
		signedBlock, err := structs.SignedBlindedBeaconBlockDenebFromConsensus(blockType.BlindedDeneb)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert blinded deneb beacon block contents")
		}
		marshalledSignedBeaconBlockJson, err = json.Marshal(signedBlock)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal blinded deneb beacon block contents")
		}
	case *ethpb.GenericSignedBeaconBlock_Electra:
		consensusVersion = "electra"
		beaconBlockRoot, err = blockType.Electra.Block.HashTreeRoot()
		if err != nil {
			return nil, errors.Wrap(err, "failed to compute block root for electra beacon block")
		}
		signedBlock, err := structs.SignedBeaconBlockContentsElectraFromConsensus(blockType.Electra)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert electra beacon block contents")
		}
		marshalledSignedBeaconBlockJson, err = json.Marshal(signedBlock)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal electra beacon block contents")
		}
	case *ethpb.GenericSignedBeaconBlock_BlindedElectra:
		blinded = true
		consensusVersion = "electra"
		beaconBlockRoot, err = blockType.BlindedElectra.HashTreeRoot()
		if err != nil {
			return nil, errors.Wrap(err, "failed to compute block root for blinded electra beacon block")
		}
		signedBlock, err := structs.SignedBlindedBeaconBlockElectraFromConsensus(blockType.BlindedElectra)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert blinded electra beacon block contents")
		}
		marshalledSignedBeaconBlockJson, err = json.Marshal(signedBlock)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal blinded electra beacon block contents")
		}
	default:
		return nil, errors.Errorf("unsupported block type %T", in.Block)
	}

	var endpoint string

	if blinded {
		endpoint = "/eth/v2/beacon/blinded_blocks"
	} else {
		endpoint = "/eth/v2/beacon/blocks"
	}

	headers := map[string]string{"Eth-Consensus-Version": consensusVersion}
	err = c.jsonRestHandler.Post(ctx, endpoint, headers, bytes.NewBuffer(marshalledSignedBeaconBlockJson), nil)
	errJson := &httputil.DefaultJsonError{}
	if err != nil {
		if !errors.As(err, &errJson) {
			return nil, err
		}
		// Error 202 means that the block was successfully broadcast, but validation failed
		if errJson.Code == http.StatusAccepted {
			return nil, errors.New("block was successfully broadcast but failed validation")
		}
		return nil, errJson
	}

	return &ethpb.ProposeResponse{BlockRoot: beaconBlockRoot[:]}, nil
}

func marshallBeaconBlockPhase0(block *ethpb.SignedBeaconBlock) ([]byte, error) {
	signedBeaconBlockJson := &structs.SignedBeaconBlock{
		Signature: hexutil.Encode(block.Signature),
		Message: &structs.BeaconBlock{
			Body: &structs.BeaconBlockBody{
				Attestations:      jsonifyAttestations(block.Block.Body.Attestations),
				AttesterSlashings: jsonifyAttesterSlashings(block.Block.Body.AttesterSlashings),
				Deposits:          jsonifyDeposits(block.Block.Body.Deposits),
				Eth1Data:          jsonifyEth1Data(block.Block.Body.Eth1Data),
				Graffiti:          hexutil.Encode(block.Block.Body.Graffiti),
				ProposerSlashings: jsonifyProposerSlashings(block.Block.Body.ProposerSlashings),
				RandaoReveal:      hexutil.Encode(block.Block.Body.RandaoReveal),
				VoluntaryExits:    JsonifySignedVoluntaryExits(block.Block.Body.VoluntaryExits),
			},
			ParentRoot:    hexutil.Encode(block.Block.ParentRoot),
			ProposerIndex: uint64ToString(block.Block.ProposerIndex),
			Slot:          uint64ToString(block.Block.Slot),
			StateRoot:     hexutil.Encode(block.Block.StateRoot),
		},
	}

	return json.Marshal(signedBeaconBlockJson)
}

func marshallBeaconBlockAltair(block *ethpb.SignedBeaconBlockAltair) ([]byte, error) {
	signedBeaconBlockAltairJson := &structs.SignedBeaconBlockAltair{
		Signature: hexutil.Encode(block.Signature),
		Message: &structs.BeaconBlockAltair{
			ParentRoot:    hexutil.Encode(block.Block.ParentRoot),
			ProposerIndex: uint64ToString(block.Block.ProposerIndex),
			Slot:          uint64ToString(block.Block.Slot),
			StateRoot:     hexutil.Encode(block.Block.StateRoot),
			Body: &structs.BeaconBlockBodyAltair{
				Attestations:      jsonifyAttestations(block.Block.Body.Attestations),
				AttesterSlashings: jsonifyAttesterSlashings(block.Block.Body.AttesterSlashings),
				Deposits:          jsonifyDeposits(block.Block.Body.Deposits),
				Eth1Data:          jsonifyEth1Data(block.Block.Body.Eth1Data),
				Graffiti:          hexutil.Encode(block.Block.Body.Graffiti),
				ProposerSlashings: jsonifyProposerSlashings(block.Block.Body.ProposerSlashings),
				RandaoReveal:      hexutil.Encode(block.Block.Body.RandaoReveal),
				VoluntaryExits:    JsonifySignedVoluntaryExits(block.Block.Body.VoluntaryExits),
				SyncAggregate: &structs.SyncAggregate{
					SyncCommitteeBits:      hexutil.Encode(block.Block.Body.SyncAggregate.SyncCommitteeBits),
					SyncCommitteeSignature: hexutil.Encode(block.Block.Body.SyncAggregate.SyncCommitteeSignature),
				},
			},
		},
	}

	return json.Marshal(signedBeaconBlockAltairJson)
}

func marshallBeaconBlockBellatrix(block *ethpb.SignedBeaconBlockBellatrix) ([]byte, error) {
	signedBeaconBlockBellatrixJson := &structs.SignedBeaconBlockBellatrix{
		Signature: hexutil.Encode(block.Signature),
		Message: &structs.BeaconBlockBellatrix{
			ParentRoot:    hexutil.Encode(block.Block.ParentRoot),
			ProposerIndex: uint64ToString(block.Block.ProposerIndex),
			Slot:          uint64ToString(block.Block.Slot),
			StateRoot:     hexutil.Encode(block.Block.StateRoot),
			Body: &structs.BeaconBlockBodyBellatrix{
				Attestations:      jsonifyAttestations(block.Block.Body.Attestations),
				AttesterSlashings: jsonifyAttesterSlashings(block.Block.Body.AttesterSlashings),
				Deposits:          jsonifyDeposits(block.Block.Body.Deposits),
				Eth1Data:          jsonifyEth1Data(block.Block.Body.Eth1Data),
				Graffiti:          hexutil.Encode(block.Block.Body.Graffiti),
				ProposerSlashings: jsonifyProposerSlashings(block.Block.Body.ProposerSlashings),
				RandaoReveal:      hexutil.Encode(block.Block.Body.RandaoReveal),
				VoluntaryExits:    JsonifySignedVoluntaryExits(block.Block.Body.VoluntaryExits),
				SyncAggregate: &structs.SyncAggregate{
					SyncCommitteeBits:      hexutil.Encode(block.Block.Body.SyncAggregate.SyncCommitteeBits),
					SyncCommitteeSignature: hexutil.Encode(block.Block.Body.SyncAggregate.SyncCommitteeSignature),
				},
				ExecutionPayload: &structs.ExecutionPayload{
					ParentHash:    hexutil.Encode(block.Block.Body.ExecutionPayload.ParentHash),
					FeeRecipient:  hexutil.Encode(block.Block.Body.ExecutionPayload.FeeRecipient),
					StateRoot:     hexutil.Encode(block.Block.Body.ExecutionPayload.StateRoot),
					ReceiptsRoot:  hexutil.Encode(block.Block.Body.ExecutionPayload.ReceiptsRoot),
					LogsBloom:     hexutil.Encode(block.Block.Body.ExecutionPayload.LogsBloom),
					PrevRandao:    hexutil.Encode(block.Block.Body.ExecutionPayload.PrevRandao),
					BlockNumber:   uint64ToString(block.Block.Body.ExecutionPayload.BlockNumber),
					GasLimit:      uint64ToString(block.Block.Body.ExecutionPayload.GasLimit),
					GasUsed:       uint64ToString(block.Block.Body.ExecutionPayload.GasUsed),
					Timestamp:     uint64ToString(block.Block.Body.ExecutionPayload.Timestamp),
					ExtraData:     hexutil.Encode(block.Block.Body.ExecutionPayload.ExtraData),
					BaseFeePerGas: bytesutil.LittleEndianBytesToBigInt(block.Block.Body.ExecutionPayload.BaseFeePerGas).String(),
					BlockHash:     hexutil.Encode(block.Block.Body.ExecutionPayload.BlockHash),
					Transactions:  jsonifyTransactions(block.Block.Body.ExecutionPayload.Transactions),
				},
			},
		},
	}

	return json.Marshal(signedBeaconBlockBellatrixJson)
}

func marshallBeaconBlockBlindedBellatrix(block *ethpb.SignedBlindedBeaconBlockBellatrix) ([]byte, error) {
	signedBeaconBlockBellatrixJson := &structs.SignedBlindedBeaconBlockBellatrix{
		Signature: hexutil.Encode(block.Signature),
		Message: &structs.BlindedBeaconBlockBellatrix{
			ParentRoot:    hexutil.Encode(block.Block.ParentRoot),
			ProposerIndex: uint64ToString(block.Block.ProposerIndex),
			Slot:          uint64ToString(block.Block.Slot),
			StateRoot:     hexutil.Encode(block.Block.StateRoot),
			Body: &structs.BlindedBeaconBlockBodyBellatrix{
				Attestations:      jsonifyAttestations(block.Block.Body.Attestations),
				AttesterSlashings: jsonifyAttesterSlashings(block.Block.Body.AttesterSlashings),
				Deposits:          jsonifyDeposits(block.Block.Body.Deposits),
				Eth1Data:          jsonifyEth1Data(block.Block.Body.Eth1Data),
				Graffiti:          hexutil.Encode(block.Block.Body.Graffiti),
				ProposerSlashings: jsonifyProposerSlashings(block.Block.Body.ProposerSlashings),
				RandaoReveal:      hexutil.Encode(block.Block.Body.RandaoReveal),
				VoluntaryExits:    JsonifySignedVoluntaryExits(block.Block.Body.VoluntaryExits),
				SyncAggregate: &structs.SyncAggregate{
					SyncCommitteeBits:      hexutil.Encode(block.Block.Body.SyncAggregate.SyncCommitteeBits),
					SyncCommitteeSignature: hexutil.Encode(block.Block.Body.SyncAggregate.SyncCommitteeSignature),
				},
				ExecutionPayloadHeader: &structs.ExecutionPayloadHeader{
					ParentHash:       hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.ParentHash),
					FeeRecipient:     hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.FeeRecipient),
					StateRoot:        hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.StateRoot),
					ReceiptsRoot:     hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.ReceiptsRoot),
					LogsBloom:        hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.LogsBloom),
					PrevRandao:       hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.PrevRandao),
					BlockNumber:      uint64ToString(block.Block.Body.ExecutionPayloadHeader.BlockNumber),
					GasLimit:         uint64ToString(block.Block.Body.ExecutionPayloadHeader.GasLimit),
					GasUsed:          uint64ToString(block.Block.Body.ExecutionPayloadHeader.GasUsed),
					Timestamp:        uint64ToString(block.Block.Body.ExecutionPayloadHeader.Timestamp),
					ExtraData:        hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.ExtraData),
					BaseFeePerGas:    bytesutil.LittleEndianBytesToBigInt(block.Block.Body.ExecutionPayloadHeader.BaseFeePerGas).String(),
					BlockHash:        hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.BlockHash),
					TransactionsRoot: hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.TransactionsRoot),
				},
			},
		},
	}

	return json.Marshal(signedBeaconBlockBellatrixJson)
}

func marshallBeaconBlockCapella(block *ethpb.SignedBeaconBlockCapella) ([]byte, error) {
	signedBeaconBlockCapellaJson := &structs.SignedBeaconBlockCapella{
		Signature: hexutil.Encode(block.Signature),
		Message: &structs.BeaconBlockCapella{
			ParentRoot:    hexutil.Encode(block.Block.ParentRoot),
			ProposerIndex: uint64ToString(block.Block.ProposerIndex),
			Slot:          uint64ToString(block.Block.Slot),
			StateRoot:     hexutil.Encode(block.Block.StateRoot),
			Body: &structs.BeaconBlockBodyCapella{
				Attestations:      jsonifyAttestations(block.Block.Body.Attestations),
				AttesterSlashings: jsonifyAttesterSlashings(block.Block.Body.AttesterSlashings),
				Deposits:          jsonifyDeposits(block.Block.Body.Deposits),
				Eth1Data:          jsonifyEth1Data(block.Block.Body.Eth1Data),
				Graffiti:          hexutil.Encode(block.Block.Body.Graffiti),
				ProposerSlashings: jsonifyProposerSlashings(block.Block.Body.ProposerSlashings),
				RandaoReveal:      hexutil.Encode(block.Block.Body.RandaoReveal),
				VoluntaryExits:    JsonifySignedVoluntaryExits(block.Block.Body.VoluntaryExits),
				SyncAggregate: &structs.SyncAggregate{
					SyncCommitteeBits:      hexutil.Encode(block.Block.Body.SyncAggregate.SyncCommitteeBits),
					SyncCommitteeSignature: hexutil.Encode(block.Block.Body.SyncAggregate.SyncCommitteeSignature),
				},
				ExecutionPayload: &structs.ExecutionPayloadCapella{
					ParentHash:    hexutil.Encode(block.Block.Body.ExecutionPayload.ParentHash),
					FeeRecipient:  hexutil.Encode(block.Block.Body.ExecutionPayload.FeeRecipient),
					StateRoot:     hexutil.Encode(block.Block.Body.ExecutionPayload.StateRoot),
					ReceiptsRoot:  hexutil.Encode(block.Block.Body.ExecutionPayload.ReceiptsRoot),
					LogsBloom:     hexutil.Encode(block.Block.Body.ExecutionPayload.LogsBloom),
					PrevRandao:    hexutil.Encode(block.Block.Body.ExecutionPayload.PrevRandao),
					BlockNumber:   uint64ToString(block.Block.Body.ExecutionPayload.BlockNumber),
					GasLimit:      uint64ToString(block.Block.Body.ExecutionPayload.GasLimit),
					GasUsed:       uint64ToString(block.Block.Body.ExecutionPayload.GasUsed),
					Timestamp:     uint64ToString(block.Block.Body.ExecutionPayload.Timestamp),
					ExtraData:     hexutil.Encode(block.Block.Body.ExecutionPayload.ExtraData),
					BaseFeePerGas: bytesutil.LittleEndianBytesToBigInt(block.Block.Body.ExecutionPayload.BaseFeePerGas).String(),
					BlockHash:     hexutil.Encode(block.Block.Body.ExecutionPayload.BlockHash),
					Transactions:  jsonifyTransactions(block.Block.Body.ExecutionPayload.Transactions),
					Withdrawals:   jsonifyWithdrawals(block.Block.Body.ExecutionPayload.Withdrawals),
				},
				BLSToExecutionChanges: jsonifyBlsToExecutionChanges(block.Block.Body.BlsToExecutionChanges),
			},
		},
	}

	return json.Marshal(signedBeaconBlockCapellaJson)
}

func marshallBeaconBlockBlindedCapella(block *ethpb.SignedBlindedBeaconBlockCapella) ([]byte, error) {
	signedBeaconBlockCapellaJson := &structs.SignedBlindedBeaconBlockCapella{
		Signature: hexutil.Encode(block.Signature),
		Message: &structs.BlindedBeaconBlockCapella{
			ParentRoot:    hexutil.Encode(block.Block.ParentRoot),
			ProposerIndex: uint64ToString(block.Block.ProposerIndex),
			Slot:          uint64ToString(block.Block.Slot),
			StateRoot:     hexutil.Encode(block.Block.StateRoot),
			Body: &structs.BlindedBeaconBlockBodyCapella{
				Attestations:      jsonifyAttestations(block.Block.Body.Attestations),
				AttesterSlashings: jsonifyAttesterSlashings(block.Block.Body.AttesterSlashings),
				Deposits:          jsonifyDeposits(block.Block.Body.Deposits),
				Eth1Data:          jsonifyEth1Data(block.Block.Body.Eth1Data),
				Graffiti:          hexutil.Encode(block.Block.Body.Graffiti),
				ProposerSlashings: jsonifyProposerSlashings(block.Block.Body.ProposerSlashings),
				RandaoReveal:      hexutil.Encode(block.Block.Body.RandaoReveal),
				VoluntaryExits:    JsonifySignedVoluntaryExits(block.Block.Body.VoluntaryExits),
				SyncAggregate: &structs.SyncAggregate{
					SyncCommitteeBits:      hexutil.Encode(block.Block.Body.SyncAggregate.SyncCommitteeBits),
					SyncCommitteeSignature: hexutil.Encode(block.Block.Body.SyncAggregate.SyncCommitteeSignature),
				},
				ExecutionPayloadHeader: &structs.ExecutionPayloadHeaderCapella{
					ParentHash:       hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.ParentHash),
					FeeRecipient:     hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.FeeRecipient),
					StateRoot:        hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.StateRoot),
					ReceiptsRoot:     hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.ReceiptsRoot),
					LogsBloom:        hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.LogsBloom),
					PrevRandao:       hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.PrevRandao),
					BlockNumber:      uint64ToString(block.Block.Body.ExecutionPayloadHeader.BlockNumber),
					GasLimit:         uint64ToString(block.Block.Body.ExecutionPayloadHeader.GasLimit),
					GasUsed:          uint64ToString(block.Block.Body.ExecutionPayloadHeader.GasUsed),
					Timestamp:        uint64ToString(block.Block.Body.ExecutionPayloadHeader.Timestamp),
					ExtraData:        hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.ExtraData),
					BaseFeePerGas:    bytesutil.LittleEndianBytesToBigInt(block.Block.Body.ExecutionPayloadHeader.BaseFeePerGas).String(),
					BlockHash:        hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.BlockHash),
					TransactionsRoot: hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.TransactionsRoot),
					WithdrawalsRoot:  hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.WithdrawalsRoot),
				},
				BLSToExecutionChanges: jsonifyBlsToExecutionChanges(block.Block.Body.BlsToExecutionChanges),
			},
		},
	}

	return json.Marshal(signedBeaconBlockCapellaJson)
}
