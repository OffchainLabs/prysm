package structs

import (
	"fmt"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/server"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/container/slice"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

var ErrUnsupportedConversion = errors.New("Could not determine api struct type to use for value")

// ----------------------------------------------------------------------------
// Phase 0
// ----------------------------------------------------------------------------

func (h *SignedBeaconBlockHeader) ToConsensus() (*eth.SignedBeaconBlockHeader, error) {
	if h == nil {
		return nil, errNilValue
	}
	msg, err := h.Message.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Message")
	}
	sig, err := bytesutil.DecodeHexWithLength(h.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}

	return &eth.SignedBeaconBlockHeader{
		Header:    msg,
		Signature: sig,
	}, nil
}

func (h *BeaconBlockHeader) ToConsensus() (*eth.BeaconBlockHeader, error) {
	if h == nil {
		return nil, errNilValue
	}
	s, err := strconv.ParseUint(h.Slot, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Slot")
	}
	pi, err := strconv.ParseUint(h.ProposerIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ProposerIndex")
	}
	pr, err := bytesutil.DecodeHexWithLength(h.ParentRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ParentRoot")
	}
	sr, err := bytesutil.DecodeHexWithLength(h.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "StateRoot")
	}
	br, err := bytesutil.DecodeHexWithLength(h.BodyRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "BodyRoot")
	}

	return &eth.BeaconBlockHeader{
		Slot:          primitives.Slot(s),
		ProposerIndex: primitives.ValidatorIndex(pi),
		ParentRoot:    pr,
		StateRoot:     sr,
		BodyRoot:      br,
	}, nil
}

func SignedBeaconBlockHeaderFromConsensus(src *eth.SignedBeaconBlockHeader) *SignedBeaconBlockHeader {
	return &SignedBeaconBlockHeader{
		Message: &BeaconBlockHeader{
			Slot:          fmt.Sprintf("%d", src.Header.Slot),
			ProposerIndex: fmt.Sprintf("%d", src.Header.ProposerIndex),
			ParentRoot:    hexutil.Encode(src.Header.ParentRoot),
			StateRoot:     hexutil.Encode(src.Header.StateRoot),
			BodyRoot:      hexutil.Encode(src.Header.BodyRoot),
		},
		Signature: hexutil.Encode(src.Signature),
	}
}

func (b *SignedBeaconBlock) ToGeneric() (*eth.GenericSignedBeaconBlock, error) {
	if b == nil {
		return nil, errNilValue
	}

	sig, err := bytesutil.DecodeHexWithLength(b.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}

	bl, err := b.Message.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Message")
	}

	block := &eth.SignedBeaconBlock{
		Block:     bl,
		Signature: sig,
	}
	return &eth.GenericSignedBeaconBlock{Block: &eth.GenericSignedBeaconBlock_Phase0{Phase0: block}}, nil
}

func (b *BeaconBlock) ToGeneric() (*eth.GenericBeaconBlock, error) {
	block, err := b.ToConsensus()
	if err != nil {
		return nil, err
	}
	return &eth.GenericBeaconBlock{Block: &eth.GenericBeaconBlock_Phase0{Phase0: block}}, nil
}

func (b *BeaconBlock) ToConsensus() (*eth.BeaconBlock, error) {
	if b == nil {
		return nil, errNilValue
	}
	if b.Body == nil {
		return nil, server.NewDecodeError(errNilValue, "Body")
	}
	if b.Body.Eth1Data == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.Eth1Data")
	}

	slot, err := strconv.ParseUint(b.Slot, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Slot")
	}
	proposerIndex, err := strconv.ParseUint(b.ProposerIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ProposerIndex")
	}
	parentRoot, err := bytesutil.DecodeHexWithLength(b.ParentRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ParentRoot")
	}
	stateRoot, err := bytesutil.DecodeHexWithLength(b.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "StateRoot")
	}
	randaoReveal, err := bytesutil.DecodeHexWithLength(b.Body.RandaoReveal, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.RandaoReveal")
	}
	depositRoot, err := bytesutil.DecodeHexWithLength(b.Body.Eth1Data.DepositRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.DepositRoot")
	}
	depositCount, err := strconv.ParseUint(b.Body.Eth1Data.DepositCount, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.DepositCount")
	}
	blockHash, err := bytesutil.DecodeHexWithLength(b.Body.Eth1Data.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.BlockHash")
	}
	graffiti, err := bytesutil.DecodeHexWithLength(b.Body.Graffiti, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Graffiti")
	}
	proposerSlashings, err := ProposerSlashingsToConsensus(b.Body.ProposerSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ProposerSlashings")
	}
	attesterSlashings, err := AttesterSlashingsToConsensus(b.Body.AttesterSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.AttesterSlashings")
	}
	atts, err := AttsToConsensus(b.Body.Attestations)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Attestations")
	}
	deposits, err := DepositsToConsensus(b.Body.Deposits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Deposits")
	}
	exits, err := SignedExitsToConsensus(b.Body.VoluntaryExits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.VoluntaryExits")
	}

	return &eth.BeaconBlock{
		Slot:          primitives.Slot(slot),
		ProposerIndex: primitives.ValidatorIndex(proposerIndex),
		ParentRoot:    parentRoot,
		StateRoot:     stateRoot,
		Body: &eth.BeaconBlockBody{
			RandaoReveal: randaoReveal,
			Eth1Data: &eth.Eth1Data{
				DepositRoot:  depositRoot,
				DepositCount: depositCount,
				BlockHash:    blockHash,
			},
			Graffiti:          graffiti,
			ProposerSlashings: proposerSlashings,
			AttesterSlashings: attesterSlashings,
			Attestations:      atts,
			Deposits:          deposits,
			VoluntaryExits:    exits,
		},
	}, nil
}

func BeaconBlockHeaderFromConsensus(h *eth.BeaconBlockHeader) *BeaconBlockHeader {
	return &BeaconBlockHeader{
		Slot:          fmt.Sprintf("%d", h.Slot),
		ProposerIndex: fmt.Sprintf("%d", h.ProposerIndex),
		ParentRoot:    hexutil.Encode(h.ParentRoot),
		StateRoot:     hexutil.Encode(h.StateRoot),
		BodyRoot:      hexutil.Encode(h.BodyRoot),
	}
}

func BeaconBlockFromConsensus(b *eth.BeaconBlock) *BeaconBlock {
	return &BeaconBlock{
		Slot:          fmt.Sprintf("%d", b.Slot),
		ProposerIndex: fmt.Sprintf("%d", b.ProposerIndex),
		ParentRoot:    hexutil.Encode(b.ParentRoot),
		StateRoot:     hexutil.Encode(b.StateRoot),
		Body: &BeaconBlockBody{
			RandaoReveal:      hexutil.Encode(b.Body.RandaoReveal),
			Eth1Data:          Eth1DataFromConsensus(b.Body.Eth1Data),
			Graffiti:          hexutil.Encode(b.Body.Graffiti),
			ProposerSlashings: ProposerSlashingsFromConsensus(b.Body.ProposerSlashings),
			AttesterSlashings: AttesterSlashingsFromConsensus(b.Body.AttesterSlashings),
			Attestations:      AttsFromConsensus(b.Body.Attestations),
			Deposits:          DepositsFromConsensus(b.Body.Deposits),
			VoluntaryExits:    SignedExitsFromConsensus(b.Body.VoluntaryExits),
		},
	}
}

func SignedBeaconBlockMessageJsoner(block interfaces.ReadOnlySignedBeaconBlock) (SignedMessageJsoner, error) {
	pb, err := block.Proto()
	if err != nil {
		return nil, err
	}
	switch pbStruct := pb.(type) {
	case *eth.SignedBeaconBlock:
		return SignedBeaconBlockPhase0FromConsensus(pbStruct), nil
	case *eth.SignedBeaconBlockAltair:
		return SignedBeaconBlockAltairFromConsensus(pbStruct), nil
	case *eth.SignedBlindedBeaconBlockBellatrix:
		return SignedBlindedBeaconBlockBellatrixFromConsensus(pbStruct)
	case *eth.SignedBeaconBlockBellatrix:
		return SignedBeaconBlockBellatrixFromConsensus(pbStruct)
	case *eth.SignedBlindedBeaconBlockCapella:
		return SignedBlindedBeaconBlockCapellaFromConsensus(pbStruct)
	case *eth.SignedBeaconBlockCapella:
		return SignedBeaconBlockCapellaFromConsensus(pbStruct)
	case *eth.SignedBlindedBeaconBlockDeneb:
		return SignedBlindedBeaconBlockDenebFromConsensus(pbStruct)
	case *eth.SignedBeaconBlockDeneb:
		return SignedBeaconBlockDenebFromConsensus(pbStruct)
	case *eth.SignedBlindedBeaconBlockElectra:
		return SignedBlindedBeaconBlockElectraFromConsensus(pbStruct)
	case *eth.SignedBeaconBlockElectra:
		return SignedBeaconBlockElectraFromConsensus(pbStruct)
	case *eth.SignedBlindedBeaconBlockFulu:
		return SignedBlindedBeaconBlockFuluFromConsensus(pbStruct)
	case *eth.SignedBeaconBlockFulu:
		return SignedBeaconBlockFuluFromConsensus(pbStruct)
	default:
		return nil, ErrUnsupportedConversion
	}
}

func SignedBeaconBlockPhase0FromConsensus(b *eth.SignedBeaconBlock) *SignedBeaconBlock {
	return &SignedBeaconBlock{
		Message:   BeaconBlockFromConsensus(b.Block),
		Signature: hexutil.Encode(b.Signature),
	}
}

func ExecutionPayloadFromConsensus(payload *enginev1.ExecutionPayload) (*ExecutionPayload, error) {
	baseFeePerGas, err := sszBytesToUint256String(payload.BaseFeePerGas)
	if err != nil {
		return nil, err
	}
	transactions := make([]string, len(payload.Transactions))
	for i, tx := range payload.Transactions {
		transactions[i] = hexutil.Encode(tx)
	}

	return &ExecutionPayload{
		ParentHash:    hexutil.Encode(payload.ParentHash),
		FeeRecipient:  hexutil.Encode(payload.FeeRecipient),
		StateRoot:     hexutil.Encode(payload.StateRoot),
		ReceiptsRoot:  hexutil.Encode(payload.ReceiptsRoot),
		LogsBloom:     hexutil.Encode(payload.LogsBloom),
		PrevRandao:    hexutil.Encode(payload.PrevRandao),
		BlockNumber:   fmt.Sprintf("%d", payload.BlockNumber),
		GasLimit:      fmt.Sprintf("%d", payload.GasLimit),
		GasUsed:       fmt.Sprintf("%d", payload.GasUsed),
		Timestamp:     fmt.Sprintf("%d", payload.Timestamp),
		ExtraData:     hexutil.Encode(payload.ExtraData),
		BaseFeePerGas: baseFeePerGas,
		BlockHash:     hexutil.Encode(payload.BlockHash),
		Transactions:  transactions,
	}, nil
}

func ExecutionPayloadHeaderFromConsensus(payload *enginev1.ExecutionPayloadHeader) (*ExecutionPayloadHeader, error) {
	baseFeePerGas, err := sszBytesToUint256String(payload.BaseFeePerGas)
	if err != nil {
		return nil, err
	}

	return &ExecutionPayloadHeader{
		ParentHash:       hexutil.Encode(payload.ParentHash),
		FeeRecipient:     hexutil.Encode(payload.FeeRecipient),
		StateRoot:        hexutil.Encode(payload.StateRoot),
		ReceiptsRoot:     hexutil.Encode(payload.ReceiptsRoot),
		LogsBloom:        hexutil.Encode(payload.LogsBloom),
		PrevRandao:       hexutil.Encode(payload.PrevRandao),
		BlockNumber:      fmt.Sprintf("%d", payload.BlockNumber),
		GasLimit:         fmt.Sprintf("%d", payload.GasLimit),
		GasUsed:          fmt.Sprintf("%d", payload.GasUsed),
		Timestamp:        fmt.Sprintf("%d", payload.Timestamp),
		ExtraData:        hexutil.Encode(payload.ExtraData),
		BaseFeePerGas:    baseFeePerGas,
		BlockHash:        hexutil.Encode(payload.BlockHash),
		TransactionsRoot: hexutil.Encode(payload.TransactionsRoot),
	}, nil
}

// ----------------------------------------------------------------------------
// Altair
// ----------------------------------------------------------------------------

func (b *SignedBeaconBlockAltair) ToGeneric() (*eth.GenericSignedBeaconBlock, error) {
	if b == nil {
		return nil, errNilValue
	}

	sig, err := bytesutil.DecodeHexWithLength(b.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}
	bl, err := b.Message.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Message")
	}
	block := &eth.SignedBeaconBlockAltair{
		Block:     bl,
		Signature: sig,
	}
	return &eth.GenericSignedBeaconBlock{Block: &eth.GenericSignedBeaconBlock_Altair{Altair: block}}, nil
}

func (b *BeaconBlockAltair) ToGeneric() (*eth.GenericBeaconBlock, error) {
	block, err := b.ToConsensus()
	if err != nil {
		return nil, err
	}
	return &eth.GenericBeaconBlock{Block: &eth.GenericBeaconBlock_Altair{Altair: block}}, nil
}

func (b *BeaconBlockAltair) ToConsensus() (*eth.BeaconBlockAltair, error) {
	if b == nil {
		return nil, errNilValue
	}
	if b.Body == nil {
		return nil, server.NewDecodeError(errNilValue, "Body")
	}
	if b.Body.Eth1Data == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.Eth1Data")
	}
	if b.Body.SyncAggregate == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.SyncAggregate")
	}

	slot, err := strconv.ParseUint(b.Slot, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Slot")
	}
	proposerIndex, err := strconv.ParseUint(b.ProposerIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ProposerIndex")
	}
	parentRoot, err := bytesutil.DecodeHexWithLength(b.ParentRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ParentRoot")
	}
	stateRoot, err := bytesutil.DecodeHexWithLength(b.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "StateRoot")
	}
	randaoReveal, err := bytesutil.DecodeHexWithLength(b.Body.RandaoReveal, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.RandaoReveal")
	}
	depositRoot, err := bytesutil.DecodeHexWithLength(b.Body.Eth1Data.DepositRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.DepositRoot")
	}
	depositCount, err := strconv.ParseUint(b.Body.Eth1Data.DepositCount, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.DepositCount")
	}
	blockHash, err := bytesutil.DecodeHexWithLength(b.Body.Eth1Data.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.BlockHash")
	}
	graffiti, err := bytesutil.DecodeHexWithLength(b.Body.Graffiti, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Graffiti")
	}
	proposerSlashings, err := ProposerSlashingsToConsensus(b.Body.ProposerSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ProposerSlashings")
	}
	attesterSlashings, err := AttesterSlashingsToConsensus(b.Body.AttesterSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.AttesterSlashings")
	}
	atts, err := AttsToConsensus(b.Body.Attestations)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Attestations")
	}
	deposits, err := DepositsToConsensus(b.Body.Deposits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Deposits")
	}
	exits, err := SignedExitsToConsensus(b.Body.VoluntaryExits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.VoluntaryExits")
	}
	syncCommitteeBits, err := bytesutil.DecodeHexWithLength(b.Body.SyncAggregate.SyncCommitteeBits, fieldparams.SyncAggregateSyncCommitteeBytesLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeBits")
	}
	syncCommitteeSig, err := bytesutil.DecodeHexWithLength(b.Body.SyncAggregate.SyncCommitteeSignature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeSignature")
	}
	return &eth.BeaconBlockAltair{
		Slot:          primitives.Slot(slot),
		ProposerIndex: primitives.ValidatorIndex(proposerIndex),
		ParentRoot:    parentRoot,
		StateRoot:     stateRoot,
		Body: &eth.BeaconBlockBodyAltair{
			RandaoReveal: randaoReveal,
			Eth1Data: &eth.Eth1Data{
				DepositRoot:  depositRoot,
				DepositCount: depositCount,
				BlockHash:    blockHash,
			},
			Graffiti:          graffiti,
			ProposerSlashings: proposerSlashings,
			AttesterSlashings: attesterSlashings,
			Attestations:      atts,
			Deposits:          deposits,
			VoluntaryExits:    exits,
			SyncAggregate: &eth.SyncAggregate{
				SyncCommitteeBits:      syncCommitteeBits,
				SyncCommitteeSignature: syncCommitteeSig,
			},
		},
	}, nil
}

func BeaconBlockAltairFromConsensus(b *eth.BeaconBlockAltair) *BeaconBlockAltair {
	return &BeaconBlockAltair{
		Slot:          fmt.Sprintf("%d", b.Slot),
		ProposerIndex: fmt.Sprintf("%d", b.ProposerIndex),
		ParentRoot:    hexutil.Encode(b.ParentRoot),
		StateRoot:     hexutil.Encode(b.StateRoot),
		Body: &BeaconBlockBodyAltair{
			RandaoReveal:      hexutil.Encode(b.Body.RandaoReveal),
			Eth1Data:          Eth1DataFromConsensus(b.Body.Eth1Data),
			Graffiti:          hexutil.Encode(b.Body.Graffiti),
			ProposerSlashings: ProposerSlashingsFromConsensus(b.Body.ProposerSlashings),
			AttesterSlashings: AttesterSlashingsFromConsensus(b.Body.AttesterSlashings),
			Attestations:      AttsFromConsensus(b.Body.Attestations),
			Deposits:          DepositsFromConsensus(b.Body.Deposits),
			VoluntaryExits:    SignedExitsFromConsensus(b.Body.VoluntaryExits),
			SyncAggregate: &SyncAggregate{
				SyncCommitteeBits:      hexutil.Encode(b.Body.SyncAggregate.SyncCommitteeBits),
				SyncCommitteeSignature: hexutil.Encode(b.Body.SyncAggregate.SyncCommitteeSignature),
			},
		},
	}
}

func SignedBeaconBlockAltairFromConsensus(b *eth.SignedBeaconBlockAltair) *SignedBeaconBlockAltair {
	return &SignedBeaconBlockAltair{
		Message:   BeaconBlockAltairFromConsensus(b.Block),
		Signature: hexutil.Encode(b.Signature),
	}
}

// ----------------------------------------------------------------------------
// Bellatrix
// ----------------------------------------------------------------------------

func (b *SignedBeaconBlockBellatrix) ToGeneric() (*eth.GenericSignedBeaconBlock, error) {
	if b == nil {
		return nil, errNilValue
	}

	sig, err := bytesutil.DecodeHexWithLength(b.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}
	bl, err := b.Message.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Message")
	}
	block := &eth.SignedBeaconBlockBellatrix{
		Block:     bl,
		Signature: sig,
	}
	return &eth.GenericSignedBeaconBlock{Block: &eth.GenericSignedBeaconBlock_Bellatrix{Bellatrix: block}}, nil
}

func (b *BeaconBlockBellatrix) ToGeneric() (*eth.GenericBeaconBlock, error) {
	block, err := b.ToConsensus()
	if err != nil {
		return nil, err
	}
	return &eth.GenericBeaconBlock{Block: &eth.GenericBeaconBlock_Bellatrix{Bellatrix: block}}, nil
}

func (b *BeaconBlockBellatrix) ToConsensus() (*eth.BeaconBlockBellatrix, error) {
	if b == nil {
		return nil, errNilValue
	}
	if b.Body == nil {
		return nil, server.NewDecodeError(errNilValue, "Body")
	}
	if b.Body.Eth1Data == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.Eth1Data")
	}
	if b.Body.SyncAggregate == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.SyncAggregate")
	}
	if b.Body.ExecutionPayload == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.ExecutionPayload")
	}

	slot, err := strconv.ParseUint(b.Slot, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Slot")
	}
	proposerIndex, err := strconv.ParseUint(b.ProposerIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ProposerIndex")
	}
	parentRoot, err := bytesutil.DecodeHexWithLength(b.ParentRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ParentRoot")
	}
	stateRoot, err := bytesutil.DecodeHexWithLength(b.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "StateRoot")
	}
	randaoReveal, err := bytesutil.DecodeHexWithLength(b.Body.RandaoReveal, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.RandaoReveal")
	}
	depositRoot, err := bytesutil.DecodeHexWithLength(b.Body.Eth1Data.DepositRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.DepositRoot")
	}
	depositCount, err := strconv.ParseUint(b.Body.Eth1Data.DepositCount, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.DepositCount")
	}
	blockHash, err := bytesutil.DecodeHexWithLength(b.Body.Eth1Data.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.BlockHash")
	}
	graffiti, err := bytesutil.DecodeHexWithLength(b.Body.Graffiti, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Graffiti")
	}
	proposerSlashings, err := ProposerSlashingsToConsensus(b.Body.ProposerSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ProposerSlashings")
	}
	attesterSlashings, err := AttesterSlashingsToConsensus(b.Body.AttesterSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.AttesterSlashings")
	}
	atts, err := AttsToConsensus(b.Body.Attestations)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Attestations")
	}
	deposits, err := DepositsToConsensus(b.Body.Deposits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Deposits")
	}
	exits, err := SignedExitsToConsensus(b.Body.VoluntaryExits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.VoluntaryExits")
	}
	syncCommitteeBits, err := bytesutil.DecodeHexWithLength(b.Body.SyncAggregate.SyncCommitteeBits, fieldparams.SyncAggregateSyncCommitteeBytesLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeBits")
	}
	syncCommitteeSig, err := bytesutil.DecodeHexWithLength(b.Body.SyncAggregate.SyncCommitteeSignature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeSignature")
	}
	payloadParentHash, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.ParentHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.ParentHash")
	}
	payloadFeeRecipient, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.FeeRecipient, fieldparams.FeeRecipientLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.FeeRecipient")
	}
	payloadStateRoot, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.StateRoot")
	}
	payloadReceiptsRoot, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.ReceiptsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.ReceiptsRoot")
	}
	payloadLogsBloom, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.LogsBloom, fieldparams.LogsBloomLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.LogsBloom")
	}
	payloadPrevRandao, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.PrevRandao, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.PrevRandao")
	}
	payloadBlockNumber, err := strconv.ParseUint(b.Body.ExecutionPayload.BlockNumber, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.BlockNumber")
	}
	payloadGasLimit, err := strconv.ParseUint(b.Body.ExecutionPayload.GasLimit, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.GasLimit")
	}
	payloadGasUsed, err := strconv.ParseUint(b.Body.ExecutionPayload.GasUsed, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.GasUsed")
	}
	payloadTimestamp, err := strconv.ParseUint(b.Body.ExecutionPayload.Timestamp, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.Timestamp")
	}
	payloadExtraData, err := bytesutil.DecodeHexWithMaxLength(b.Body.ExecutionPayload.ExtraData, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.ExtraData")
	}
	payloadBaseFeePerGas, err := bytesutil.Uint256ToSSZBytes(b.Body.ExecutionPayload.BaseFeePerGas)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.BaseFeePerGas")
	}
	payloadBlockHash, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.BlockHash")
	}
	err = slice.VerifyMaxLength(b.Body.ExecutionPayload.Transactions, fieldparams.MaxTxsPerPayloadLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.Transactions")
	}
	payloadTxs := make([][]byte, len(b.Body.ExecutionPayload.Transactions))
	for i, tx := range b.Body.ExecutionPayload.Transactions {
		payloadTxs[i], err = bytesutil.DecodeHexWithMaxLength(tx, fieldparams.MaxBytesPerTxLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Transactions[%d]", i))
		}
	}

	return &eth.BeaconBlockBellatrix{
		Slot:          primitives.Slot(slot),
		ProposerIndex: primitives.ValidatorIndex(proposerIndex),
		ParentRoot:    parentRoot,
		StateRoot:     stateRoot,
		Body: &eth.BeaconBlockBodyBellatrix{
			RandaoReveal: randaoReveal,
			Eth1Data: &eth.Eth1Data{
				DepositRoot:  depositRoot,
				DepositCount: depositCount,
				BlockHash:    blockHash,
			},
			Graffiti:          graffiti,
			ProposerSlashings: proposerSlashings,
			AttesterSlashings: attesterSlashings,
			Attestations:      atts,
			Deposits:          deposits,
			VoluntaryExits:    exits,
			SyncAggregate: &eth.SyncAggregate{
				SyncCommitteeBits:      syncCommitteeBits,
				SyncCommitteeSignature: syncCommitteeSig,
			},
			ExecutionPayload: &enginev1.ExecutionPayload{
				ParentHash:    payloadParentHash,
				FeeRecipient:  payloadFeeRecipient,
				StateRoot:     payloadStateRoot,
				ReceiptsRoot:  payloadReceiptsRoot,
				LogsBloom:     payloadLogsBloom,
				PrevRandao:    payloadPrevRandao,
				BlockNumber:   payloadBlockNumber,
				GasLimit:      payloadGasLimit,
				GasUsed:       payloadGasUsed,
				Timestamp:     payloadTimestamp,
				ExtraData:     payloadExtraData,
				BaseFeePerGas: payloadBaseFeePerGas,
				BlockHash:     payloadBlockHash,
				Transactions:  payloadTxs,
			},
		},
	}, nil
}

func (b *SignedBlindedBeaconBlockBellatrix) ToGeneric() (*eth.GenericSignedBeaconBlock, error) {
	if b == nil {
		return nil, errNilValue
	}

	sig, err := bytesutil.DecodeHexWithLength(b.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}
	bl, err := b.Message.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Message")
	}
	block := &eth.SignedBlindedBeaconBlockBellatrix{
		Block:     bl,
		Signature: sig,
	}
	return &eth.GenericSignedBeaconBlock{Block: &eth.GenericSignedBeaconBlock_BlindedBellatrix{BlindedBellatrix: block}, IsBlinded: true}, nil
}

func (b *BlindedBeaconBlockBellatrix) ToGeneric() (*eth.GenericBeaconBlock, error) {
	block, err := b.ToConsensus()
	if err != nil {
		return nil, err
	}
	return &eth.GenericBeaconBlock{Block: &eth.GenericBeaconBlock_BlindedBellatrix{BlindedBellatrix: block}, IsBlinded: true}, nil
}

func (b *BlindedBeaconBlockBellatrix) ToConsensus() (*eth.BlindedBeaconBlockBellatrix, error) {
	if b == nil {
		return nil, errNilValue
	}
	if b.Body == nil {
		return nil, server.NewDecodeError(errNilValue, "Body")
	}
	if b.Body.Eth1Data == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.Eth1Data")
	}
	if b.Body.SyncAggregate == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.SyncAggregate")
	}
	if b.Body.ExecutionPayloadHeader == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.ExecutionPayloadHeader")
	}

	slot, err := strconv.ParseUint(b.Slot, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Slot")
	}
	proposerIndex, err := strconv.ParseUint(b.ProposerIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ProposerIndex")
	}
	parentRoot, err := bytesutil.DecodeHexWithLength(b.ParentRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ParentRoot")
	}
	stateRoot, err := bytesutil.DecodeHexWithLength(b.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "StateRoot")
	}
	randaoReveal, err := bytesutil.DecodeHexWithLength(b.Body.RandaoReveal, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.RandaoReveal")
	}
	depositRoot, err := bytesutil.DecodeHexWithLength(b.Body.Eth1Data.DepositRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.DepositRoot")
	}
	depositCount, err := strconv.ParseUint(b.Body.Eth1Data.DepositCount, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.DepositCount")
	}
	blockHash, err := bytesutil.DecodeHexWithLength(b.Body.Eth1Data.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.BlockHash")
	}
	graffiti, err := bytesutil.DecodeHexWithLength(b.Body.Graffiti, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Graffiti")
	}
	proposerSlashings, err := ProposerSlashingsToConsensus(b.Body.ProposerSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ProposerSlashings")
	}
	attesterSlashings, err := AttesterSlashingsToConsensus(b.Body.AttesterSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.AttesterSlashings")
	}
	atts, err := AttsToConsensus(b.Body.Attestations)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Attestations")
	}
	deposits, err := DepositsToConsensus(b.Body.Deposits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Deposits")
	}
	exits, err := SignedExitsToConsensus(b.Body.VoluntaryExits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.VoluntaryExits")
	}
	syncCommitteeBits, err := bytesutil.DecodeHexWithLength(b.Body.SyncAggregate.SyncCommitteeBits, fieldparams.SyncAggregateSyncCommitteeBytesLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeBits")
	}
	syncCommitteeSig, err := bytesutil.DecodeHexWithLength(b.Body.SyncAggregate.SyncCommitteeSignature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeSignature")
	}
	payloadParentHash, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.ParentHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.ParentHash")
	}
	payloadFeeRecipient, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.FeeRecipient, fieldparams.FeeRecipientLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.FeeRecipient")
	}
	payloadStateRoot, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.StateRoot")
	}
	payloadReceiptsRoot, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.ReceiptsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.ReceiptsRoot")
	}
	payloadLogsBloom, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.LogsBloom, fieldparams.LogsBloomLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.LogsBloom")
	}
	payloadPrevRandao, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.PrevRandao, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.PrevRandao")
	}
	payloadBlockNumber, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.BlockNumber, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.BlockNumber")
	}
	payloadGasLimit, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.GasLimit, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.GasLimit")
	}
	payloadGasUsed, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.GasUsed, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.GasUsed")
	}
	payloadTimestamp, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.Timestamp, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.Timestamp")
	}
	payloadExtraData, err := bytesutil.DecodeHexWithMaxLength(b.Body.ExecutionPayloadHeader.ExtraData, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.ExtraData")
	}
	payloadBaseFeePerGas, err := bytesutil.Uint256ToSSZBytes(b.Body.ExecutionPayloadHeader.BaseFeePerGas)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.BaseFeePerGas")
	}
	payloadBlockHash, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.BlockHash")
	}
	payloadTxsRoot, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.TransactionsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.TransactionsRoot")
	}
	return &eth.BlindedBeaconBlockBellatrix{
		Slot:          primitives.Slot(slot),
		ProposerIndex: primitives.ValidatorIndex(proposerIndex),
		ParentRoot:    parentRoot,
		StateRoot:     stateRoot,
		Body: &eth.BlindedBeaconBlockBodyBellatrix{
			RandaoReveal: randaoReveal,
			Eth1Data: &eth.Eth1Data{
				DepositRoot:  depositRoot,
				DepositCount: depositCount,
				BlockHash:    blockHash,
			},
			Graffiti:          graffiti,
			ProposerSlashings: proposerSlashings,
			AttesterSlashings: attesterSlashings,
			Attestations:      atts,
			Deposits:          deposits,
			VoluntaryExits:    exits,
			SyncAggregate: &eth.SyncAggregate{
				SyncCommitteeBits:      syncCommitteeBits,
				SyncCommitteeSignature: syncCommitteeSig,
			},
			ExecutionPayloadHeader: &enginev1.ExecutionPayloadHeader{
				ParentHash:       payloadParentHash,
				FeeRecipient:     payloadFeeRecipient,
				StateRoot:        payloadStateRoot,
				ReceiptsRoot:     payloadReceiptsRoot,
				LogsBloom:        payloadLogsBloom,
				PrevRandao:       payloadPrevRandao,
				BlockNumber:      payloadBlockNumber,
				GasLimit:         payloadGasLimit,
				GasUsed:          payloadGasUsed,
				Timestamp:        payloadTimestamp,
				ExtraData:        payloadExtraData,
				BaseFeePerGas:    payloadBaseFeePerGas,
				BlockHash:        payloadBlockHash,
				TransactionsRoot: payloadTxsRoot,
			},
		},
	}, nil
}

func BlindedBeaconBlockBellatrixFromConsensus(b *eth.BlindedBeaconBlockBellatrix) (*BlindedBeaconBlockBellatrix, error) {
	payload, err := ExecutionPayloadHeaderFromConsensus(b.Body.ExecutionPayloadHeader)
	if err != nil {
		return nil, err
	}

	return &BlindedBeaconBlockBellatrix{
		Slot:          fmt.Sprintf("%d", b.Slot),
		ProposerIndex: fmt.Sprintf("%d", b.ProposerIndex),
		ParentRoot:    hexutil.Encode(b.ParentRoot),
		StateRoot:     hexutil.Encode(b.StateRoot),
		Body: &BlindedBeaconBlockBodyBellatrix{
			RandaoReveal:      hexutil.Encode(b.Body.RandaoReveal),
			Eth1Data:          Eth1DataFromConsensus(b.Body.Eth1Data),
			Graffiti:          hexutil.Encode(b.Body.Graffiti),
			ProposerSlashings: ProposerSlashingsFromConsensus(b.Body.ProposerSlashings),
			AttesterSlashings: AttesterSlashingsFromConsensus(b.Body.AttesterSlashings),
			Attestations:      AttsFromConsensus(b.Body.Attestations),
			Deposits:          DepositsFromConsensus(b.Body.Deposits),
			VoluntaryExits:    SignedExitsFromConsensus(b.Body.VoluntaryExits),
			SyncAggregate: &SyncAggregate{
				SyncCommitteeBits:      hexutil.Encode(b.Body.SyncAggregate.SyncCommitteeBits),
				SyncCommitteeSignature: hexutil.Encode(b.Body.SyncAggregate.SyncCommitteeSignature),
			},
			ExecutionPayloadHeader: payload,
		},
	}, nil
}

func SignedBlindedBeaconBlockBellatrixFromConsensus(b *eth.SignedBlindedBeaconBlockBellatrix) (*SignedBlindedBeaconBlockBellatrix, error) {
	blindedBlock, err := BlindedBeaconBlockBellatrixFromConsensus(b.Block)
	if err != nil {
		return nil, err
	}
	return &SignedBlindedBeaconBlockBellatrix{
		Message:   blindedBlock,
		Signature: hexutil.Encode(b.Signature),
	}, nil
}

func BeaconBlockBellatrixFromConsensus(b *eth.BeaconBlockBellatrix) (*BeaconBlockBellatrix, error) {
	payload, err := ExecutionPayloadFromConsensus(b.Body.ExecutionPayload)
	if err != nil {
		return nil, err
	}

	return &BeaconBlockBellatrix{
		Slot:          fmt.Sprintf("%d", b.Slot),
		ProposerIndex: fmt.Sprintf("%d", b.ProposerIndex),
		ParentRoot:    hexutil.Encode(b.ParentRoot),
		StateRoot:     hexutil.Encode(b.StateRoot),
		Body: &BeaconBlockBodyBellatrix{
			RandaoReveal:      hexutil.Encode(b.Body.RandaoReveal),
			Eth1Data:          Eth1DataFromConsensus(b.Body.Eth1Data),
			Graffiti:          hexutil.Encode(b.Body.Graffiti),
			ProposerSlashings: ProposerSlashingsFromConsensus(b.Body.ProposerSlashings),
			AttesterSlashings: AttesterSlashingsFromConsensus(b.Body.AttesterSlashings),
			Attestations:      AttsFromConsensus(b.Body.Attestations),
			Deposits:          DepositsFromConsensus(b.Body.Deposits),
			VoluntaryExits:    SignedExitsFromConsensus(b.Body.VoluntaryExits),
			SyncAggregate: &SyncAggregate{
				SyncCommitteeBits:      hexutil.Encode(b.Body.SyncAggregate.SyncCommitteeBits),
				SyncCommitteeSignature: hexutil.Encode(b.Body.SyncAggregate.SyncCommitteeSignature),
			},
			ExecutionPayload: payload,
		},
	}, nil
}

func SignedBeaconBlockBellatrixFromConsensus(b *eth.SignedBeaconBlockBellatrix) (*SignedBeaconBlockBellatrix, error) {
	block, err := BeaconBlockBellatrixFromConsensus(b.Block)
	if err != nil {
		return nil, err
	}
	return &SignedBeaconBlockBellatrix{
		Message:   block,
		Signature: hexutil.Encode(b.Signature),
	}, nil
}

// ----------------------------------------------------------------------------
// Capella
// ----------------------------------------------------------------------------

func (b *SignedBeaconBlockCapella) ToGeneric() (*eth.GenericSignedBeaconBlock, error) {
	if b == nil {
		return nil, errNilValue
	}

	sig, err := bytesutil.DecodeHexWithLength(b.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}
	bl, err := b.Message.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Message")
	}
	block := &eth.SignedBeaconBlockCapella{
		Block:     bl,
		Signature: sig,
	}
	return &eth.GenericSignedBeaconBlock{Block: &eth.GenericSignedBeaconBlock_Capella{Capella: block}}, nil
}

func (b *BeaconBlockCapella) ToGeneric() (*eth.GenericBeaconBlock, error) {
	block, err := b.ToConsensus()
	if err != nil {
		return nil, err
	}
	return &eth.GenericBeaconBlock{Block: &eth.GenericBeaconBlock_Capella{Capella: block}}, nil
}

func (b *BeaconBlockCapella) ToConsensus() (*eth.BeaconBlockCapella, error) {
	if b == nil {
		return nil, errNilValue
	}
	if b.Body == nil {
		return nil, server.NewDecodeError(errNilValue, "Body")
	}
	if b.Body.Eth1Data == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.Eth1Data")
	}
	if b.Body.SyncAggregate == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.SyncAggregate")
	}
	if b.Body.ExecutionPayload == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.ExecutionPayload")
	}

	slot, err := strconv.ParseUint(b.Slot, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Slot")
	}
	proposerIndex, err := strconv.ParseUint(b.ProposerIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ProposerIndex")
	}
	parentRoot, err := bytesutil.DecodeHexWithLength(b.ParentRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ParentRoot")
	}
	stateRoot, err := bytesutil.DecodeHexWithLength(b.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "StateRoot")
	}
	randaoReveal, err := bytesutil.DecodeHexWithLength(b.Body.RandaoReveal, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.RandaoReveal")
	}
	depositRoot, err := bytesutil.DecodeHexWithLength(b.Body.Eth1Data.DepositRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.DepositRoot")
	}
	depositCount, err := strconv.ParseUint(b.Body.Eth1Data.DepositCount, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.DepositCount")
	}
	blockHash, err := bytesutil.DecodeHexWithLength(b.Body.Eth1Data.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.BlockHash")
	}
	graffiti, err := bytesutil.DecodeHexWithLength(b.Body.Graffiti, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Graffiti")
	}
	proposerSlashings, err := ProposerSlashingsToConsensus(b.Body.ProposerSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ProposerSlashings")
	}
	attesterSlashings, err := AttesterSlashingsToConsensus(b.Body.AttesterSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.AttesterSlashings")
	}
	atts, err := AttsToConsensus(b.Body.Attestations)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Attestations")
	}
	deposits, err := DepositsToConsensus(b.Body.Deposits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Deposits")
	}
	exits, err := SignedExitsToConsensus(b.Body.VoluntaryExits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.VoluntaryExits")
	}
	syncCommitteeBits, err := bytesutil.DecodeHexWithLength(b.Body.SyncAggregate.SyncCommitteeBits, fieldparams.SyncAggregateSyncCommitteeBytesLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeBits")
	}
	syncCommitteeSig, err := bytesutil.DecodeHexWithLength(b.Body.SyncAggregate.SyncCommitteeSignature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeSignature")
	}
	payloadParentHash, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.ParentHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.ParentHash")
	}
	payloadFeeRecipient, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.FeeRecipient, fieldparams.FeeRecipientLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.FeeRecipient")
	}
	payloadStateRoot, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.StateRoot")
	}
	payloadReceiptsRoot, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.ReceiptsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.ReceiptsRoot")
	}
	payloadLogsBloom, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.LogsBloom, fieldparams.LogsBloomLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.LogsBloom")
	}
	payloadPrevRandao, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.PrevRandao, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.PrevRandao")
	}
	payloadBlockNumber, err := strconv.ParseUint(b.Body.ExecutionPayload.BlockNumber, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.BlockNumber")
	}
	payloadGasLimit, err := strconv.ParseUint(b.Body.ExecutionPayload.GasLimit, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.GasLimit")
	}
	payloadGasUsed, err := strconv.ParseUint(b.Body.ExecutionPayload.GasUsed, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.GasUsed")
	}
	payloadTimestamp, err := strconv.ParseUint(b.Body.ExecutionPayload.Timestamp, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.Timestamp")
	}
	payloadExtraData, err := bytesutil.DecodeHexWithMaxLength(b.Body.ExecutionPayload.ExtraData, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.ExtraData")
	}
	payloadBaseFeePerGas, err := bytesutil.Uint256ToSSZBytes(b.Body.ExecutionPayload.BaseFeePerGas)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.BaseFeePerGas")
	}
	payloadBlockHash, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.BlockHash")
	}
	err = slice.VerifyMaxLength(b.Body.ExecutionPayload.Transactions, fieldparams.MaxTxsPerPayloadLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.Transactions")
	}
	payloadTxs := make([][]byte, len(b.Body.ExecutionPayload.Transactions))
	for i, tx := range b.Body.ExecutionPayload.Transactions {
		payloadTxs[i], err = bytesutil.DecodeHexWithMaxLength(tx, fieldparams.MaxBytesPerTxLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Transactions[%d]", i))
		}
	}
	err = slice.VerifyMaxLength(b.Body.ExecutionPayload.Withdrawals, fieldparams.MaxWithdrawalsPerPayload)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.Withdrawals")
	}
	withdrawals := make([]*enginev1.Withdrawal, len(b.Body.ExecutionPayload.Withdrawals))
	for i, w := range b.Body.ExecutionPayload.Withdrawals {
		withdrawalIndex, err := strconv.ParseUint(w.WithdrawalIndex, 10, 64)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Withdrawals[%d].WithdrawalIndex", i))
		}
		validatorIndex, err := strconv.ParseUint(w.ValidatorIndex, 10, 64)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Withdrawals[%d].ValidatorIndex", i))
		}
		address, err := bytesutil.DecodeHexWithLength(w.ExecutionAddress, common.AddressLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Withdrawals[%d].ExecutionAddress", i))
		}
		amount, err := strconv.ParseUint(w.Amount, 10, 64)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Withdrawals[%d].Amount", i))
		}
		withdrawals[i] = &enginev1.Withdrawal{
			Index:          withdrawalIndex,
			ValidatorIndex: primitives.ValidatorIndex(validatorIndex),
			Address:        address,
			Amount:         amount,
		}
	}
	blsChanges, err := SignedBLSChangesToConsensus(b.Body.BLSToExecutionChanges)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.BLSToExecutionChanges")
	}

	return &eth.BeaconBlockCapella{
		Slot:          primitives.Slot(slot),
		ProposerIndex: primitives.ValidatorIndex(proposerIndex),
		ParentRoot:    parentRoot,
		StateRoot:     stateRoot,
		Body: &eth.BeaconBlockBodyCapella{
			RandaoReveal: randaoReveal,
			Eth1Data: &eth.Eth1Data{
				DepositRoot:  depositRoot,
				DepositCount: depositCount,
				BlockHash:    blockHash,
			},
			Graffiti:          graffiti,
			ProposerSlashings: proposerSlashings,
			AttesterSlashings: attesterSlashings,
			Attestations:      atts,
			Deposits:          deposits,
			VoluntaryExits:    exits,
			SyncAggregate: &eth.SyncAggregate{
				SyncCommitteeBits:      syncCommitteeBits,
				SyncCommitteeSignature: syncCommitteeSig,
			},
			ExecutionPayload: &enginev1.ExecutionPayloadCapella{
				ParentHash:    payloadParentHash,
				FeeRecipient:  payloadFeeRecipient,
				StateRoot:     payloadStateRoot,
				ReceiptsRoot:  payloadReceiptsRoot,
				LogsBloom:     payloadLogsBloom,
				PrevRandao:    payloadPrevRandao,
				BlockNumber:   payloadBlockNumber,
				GasLimit:      payloadGasLimit,
				GasUsed:       payloadGasUsed,
				Timestamp:     payloadTimestamp,
				ExtraData:     payloadExtraData,
				BaseFeePerGas: payloadBaseFeePerGas,
				BlockHash:     payloadBlockHash,
				Transactions:  payloadTxs,
				Withdrawals:   withdrawals,
			},
			BlsToExecutionChanges: blsChanges,
		},
	}, nil
}

func (b *SignedBlindedBeaconBlockCapella) ToGeneric() (*eth.GenericSignedBeaconBlock, error) {
	if b == nil {
		return nil, errNilValue
	}

	sig, err := bytesutil.DecodeHexWithLength(b.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}
	bl, err := b.Message.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Message")
	}
	block := &eth.SignedBlindedBeaconBlockCapella{
		Block:     bl,
		Signature: sig,
	}
	return &eth.GenericSignedBeaconBlock{Block: &eth.GenericSignedBeaconBlock_BlindedCapella{BlindedCapella: block}, IsBlinded: true}, nil
}

func (b *BlindedBeaconBlockCapella) ToGeneric() (*eth.GenericBeaconBlock, error) {
	block, err := b.ToConsensus()
	if err != nil {
		return nil, err
	}
	return &eth.GenericBeaconBlock{Block: &eth.GenericBeaconBlock_BlindedCapella{BlindedCapella: block}, IsBlinded: true}, nil
}

func (b *BlindedBeaconBlockCapella) ToConsensus() (*eth.BlindedBeaconBlockCapella, error) {
	if b == nil {
		return nil, errNilValue
	}
	if b.Body == nil {
		return nil, server.NewDecodeError(errNilValue, "Body")
	}
	if b.Body.Eth1Data == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.Eth1Data")
	}
	if b.Body.SyncAggregate == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.SyncAggregate")
	}
	if b.Body.ExecutionPayloadHeader == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.ExecutionPayloadHeader")
	}

	slot, err := strconv.ParseUint(b.Slot, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Slot")
	}
	proposerIndex, err := strconv.ParseUint(b.ProposerIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ProposerIndex")
	}
	parentRoot, err := bytesutil.DecodeHexWithLength(b.ParentRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ParentRoot")
	}
	stateRoot, err := bytesutil.DecodeHexWithLength(b.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "StateRoot")
	}
	randaoReveal, err := bytesutil.DecodeHexWithLength(b.Body.RandaoReveal, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.RandaoReveal")
	}
	depositRoot, err := bytesutil.DecodeHexWithLength(b.Body.Eth1Data.DepositRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.DepositRoot")
	}
	depositCount, err := strconv.ParseUint(b.Body.Eth1Data.DepositCount, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.DepositCount")
	}
	blockHash, err := bytesutil.DecodeHexWithLength(b.Body.Eth1Data.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.BlockHash")
	}
	graffiti, err := bytesutil.DecodeHexWithLength(b.Body.Graffiti, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Graffiti")
	}
	proposerSlashings, err := ProposerSlashingsToConsensus(b.Body.ProposerSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ProposerSlashings")
	}
	attesterSlashings, err := AttesterSlashingsToConsensus(b.Body.AttesterSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.AttesterSlashings")
	}
	atts, err := AttsToConsensus(b.Body.Attestations)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Attestations")
	}
	deposits, err := DepositsToConsensus(b.Body.Deposits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Deposits")
	}
	exits, err := SignedExitsToConsensus(b.Body.VoluntaryExits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.VoluntaryExits")
	}
	syncCommitteeBits, err := bytesutil.DecodeHexWithLength(b.Body.SyncAggregate.SyncCommitteeBits, fieldparams.SyncAggregateSyncCommitteeBytesLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeBits")
	}
	syncCommitteeSig, err := bytesutil.DecodeHexWithLength(b.Body.SyncAggregate.SyncCommitteeSignature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeSignature")
	}
	payloadParentHash, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.ParentHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.ParentHash")
	}
	payloadFeeRecipient, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.FeeRecipient, fieldparams.FeeRecipientLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.FeeRecipient")
	}
	payloadStateRoot, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.StateRoot")
	}
	payloadReceiptsRoot, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.ReceiptsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.ReceiptsRoot")
	}
	payloadLogsBloom, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.LogsBloom, fieldparams.LogsBloomLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.LogsBloom")
	}
	payloadPrevRandao, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.PrevRandao, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.PrevRandao")
	}
	payloadBlockNumber, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.BlockNumber, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.BlockNumber")
	}
	payloadGasLimit, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.GasLimit, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.GasLimit")
	}
	payloadGasUsed, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.GasUsed, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.GasUsed")
	}
	payloadTimestamp, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.Timestamp, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.Timestamp")
	}
	payloadExtraData, err := bytesutil.DecodeHexWithMaxLength(b.Body.ExecutionPayloadHeader.ExtraData, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.ExtraData")
	}
	payloadBaseFeePerGas, err := bytesutil.Uint256ToSSZBytes(b.Body.ExecutionPayloadHeader.BaseFeePerGas)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.BaseFeePerGas")
	}
	payloadBlockHash, err := bytesutil.DecodeHexWithMaxLength(b.Body.ExecutionPayloadHeader.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.BlockHash")
	}
	payloadTxsRoot, err := bytesutil.DecodeHexWithMaxLength(b.Body.ExecutionPayloadHeader.TransactionsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.TransactionsRoot")
	}
	payloadWithdrawalsRoot, err := bytesutil.DecodeHexWithMaxLength(b.Body.ExecutionPayloadHeader.WithdrawalsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.WithdrawalsRoot")
	}
	blsChanges, err := SignedBLSChangesToConsensus(b.Body.BLSToExecutionChanges)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.BLSToExecutionChanges")
	}

	return &eth.BlindedBeaconBlockCapella{
		Slot:          primitives.Slot(slot),
		ProposerIndex: primitives.ValidatorIndex(proposerIndex),
		ParentRoot:    parentRoot,
		StateRoot:     stateRoot,
		Body: &eth.BlindedBeaconBlockBodyCapella{
			RandaoReveal: randaoReveal,
			Eth1Data: &eth.Eth1Data{
				DepositRoot:  depositRoot,
				DepositCount: depositCount,
				BlockHash:    blockHash,
			},
			Graffiti:          graffiti,
			ProposerSlashings: proposerSlashings,
			AttesterSlashings: attesterSlashings,
			Attestations:      atts,
			Deposits:          deposits,
			VoluntaryExits:    exits,
			SyncAggregate: &eth.SyncAggregate{
				SyncCommitteeBits:      syncCommitteeBits,
				SyncCommitteeSignature: syncCommitteeSig,
			},
			ExecutionPayloadHeader: &enginev1.ExecutionPayloadHeaderCapella{
				ParentHash:       payloadParentHash,
				FeeRecipient:     payloadFeeRecipient,
				StateRoot:        payloadStateRoot,
				ReceiptsRoot:     payloadReceiptsRoot,
				LogsBloom:        payloadLogsBloom,
				PrevRandao:       payloadPrevRandao,
				BlockNumber:      payloadBlockNumber,
				GasLimit:         payloadGasLimit,
				GasUsed:          payloadGasUsed,
				Timestamp:        payloadTimestamp,
				ExtraData:        payloadExtraData,
				BaseFeePerGas:    payloadBaseFeePerGas,
				BlockHash:        payloadBlockHash,
				TransactionsRoot: payloadTxsRoot,
				WithdrawalsRoot:  payloadWithdrawalsRoot,
			},
			BlsToExecutionChanges: blsChanges,
		},
	}, nil
}

func BlindedBeaconBlockCapellaFromConsensus(b *eth.BlindedBeaconBlockCapella) (*BlindedBeaconBlockCapella, error) {
	payload, err := ExecutionPayloadHeaderCapellaFromConsensus(b.Body.ExecutionPayloadHeader)
	if err != nil {
		return nil, err
	}

	return &BlindedBeaconBlockCapella{
		Slot:          fmt.Sprintf("%d", b.Slot),
		ProposerIndex: fmt.Sprintf("%d", b.ProposerIndex),
		ParentRoot:    hexutil.Encode(b.ParentRoot),
		StateRoot:     hexutil.Encode(b.StateRoot),
		Body: &BlindedBeaconBlockBodyCapella{
			RandaoReveal:      hexutil.Encode(b.Body.RandaoReveal),
			Eth1Data:          Eth1DataFromConsensus(b.Body.Eth1Data),
			Graffiti:          hexutil.Encode(b.Body.Graffiti),
			ProposerSlashings: ProposerSlashingsFromConsensus(b.Body.ProposerSlashings),
			AttesterSlashings: AttesterSlashingsFromConsensus(b.Body.AttesterSlashings),
			Attestations:      AttsFromConsensus(b.Body.Attestations),
			Deposits:          DepositsFromConsensus(b.Body.Deposits),
			VoluntaryExits:    SignedExitsFromConsensus(b.Body.VoluntaryExits),
			SyncAggregate: &SyncAggregate{
				SyncCommitteeBits:      hexutil.Encode(b.Body.SyncAggregate.SyncCommitteeBits),
				SyncCommitteeSignature: hexutil.Encode(b.Body.SyncAggregate.SyncCommitteeSignature),
			},
			ExecutionPayloadHeader: payload,
			BLSToExecutionChanges:  SignedBLSChangesFromConsensus(b.Body.BlsToExecutionChanges),
		},
	}, nil
}

func SignedBlindedBeaconBlockCapellaFromConsensus(b *eth.SignedBlindedBeaconBlockCapella) (*SignedBlindedBeaconBlockCapella, error) {
	blindedBlock, err := BlindedBeaconBlockCapellaFromConsensus(b.Block)
	if err != nil {
		return nil, err
	}
	return &SignedBlindedBeaconBlockCapella{
		Message:   blindedBlock,
		Signature: hexutil.Encode(b.Signature),
	}, nil
}

func BeaconBlockCapellaFromConsensus(b *eth.BeaconBlockCapella) (*BeaconBlockCapella, error) {
	payload, err := ExecutionPayloadCapellaFromConsensus(b.Body.ExecutionPayload)
	if err != nil {
		return nil, err
	}

	return &BeaconBlockCapella{
		Slot:          fmt.Sprintf("%d", b.Slot),
		ProposerIndex: fmt.Sprintf("%d", b.ProposerIndex),
		ParentRoot:    hexutil.Encode(b.ParentRoot),
		StateRoot:     hexutil.Encode(b.StateRoot),
		Body: &BeaconBlockBodyCapella{
			RandaoReveal:      hexutil.Encode(b.Body.RandaoReveal),
			Eth1Data:          Eth1DataFromConsensus(b.Body.Eth1Data),
			Graffiti:          hexutil.Encode(b.Body.Graffiti),
			ProposerSlashings: ProposerSlashingsFromConsensus(b.Body.ProposerSlashings),
			AttesterSlashings: AttesterSlashingsFromConsensus(b.Body.AttesterSlashings),
			Attestations:      AttsFromConsensus(b.Body.Attestations),
			Deposits:          DepositsFromConsensus(b.Body.Deposits),
			VoluntaryExits:    SignedExitsFromConsensus(b.Body.VoluntaryExits),
			SyncAggregate: &SyncAggregate{
				SyncCommitteeBits:      hexutil.Encode(b.Body.SyncAggregate.SyncCommitteeBits),
				SyncCommitteeSignature: hexutil.Encode(b.Body.SyncAggregate.SyncCommitteeSignature),
			},
			ExecutionPayload:      payload,
			BLSToExecutionChanges: SignedBLSChangesFromConsensus(b.Body.BlsToExecutionChanges),
		},
	}, nil
}

func SignedBeaconBlockCapellaFromConsensus(b *eth.SignedBeaconBlockCapella) (*SignedBeaconBlockCapella, error) {
	block, err := BeaconBlockCapellaFromConsensus(b.Block)
	if err != nil {
		return nil, err
	}
	return &SignedBeaconBlockCapella{
		Message:   block,
		Signature: hexutil.Encode(b.Signature),
	}, nil
}

func ExecutionPayloadCapellaFromConsensus(payload *enginev1.ExecutionPayloadCapella) (*ExecutionPayloadCapella, error) {
	baseFeePerGas, err := sszBytesToUint256String(payload.BaseFeePerGas)
	if err != nil {
		return nil, err
	}
	transactions := make([]string, len(payload.Transactions))
	for i, tx := range payload.Transactions {
		transactions[i] = hexutil.Encode(tx)
	}

	return &ExecutionPayloadCapella{
		ParentHash:    hexutil.Encode(payload.ParentHash),
		FeeRecipient:  hexutil.Encode(payload.FeeRecipient),
		StateRoot:     hexutil.Encode(payload.StateRoot),
		ReceiptsRoot:  hexutil.Encode(payload.ReceiptsRoot),
		LogsBloom:     hexutil.Encode(payload.LogsBloom),
		PrevRandao:    hexutil.Encode(payload.PrevRandao),
		BlockNumber:   fmt.Sprintf("%d", payload.BlockNumber),
		GasLimit:      fmt.Sprintf("%d", payload.GasLimit),
		GasUsed:       fmt.Sprintf("%d", payload.GasUsed),
		Timestamp:     fmt.Sprintf("%d", payload.Timestamp),
		ExtraData:     hexutil.Encode(payload.ExtraData),
		BaseFeePerGas: baseFeePerGas,
		BlockHash:     hexutil.Encode(payload.BlockHash),
		Transactions:  transactions,
		Withdrawals:   WithdrawalsFromConsensus(payload.Withdrawals),
	}, nil
}

func ExecutionPayloadHeaderCapellaFromConsensus(payload *enginev1.ExecutionPayloadHeaderCapella) (*ExecutionPayloadHeaderCapella, error) {
	baseFeePerGas, err := sszBytesToUint256String(payload.BaseFeePerGas)
	if err != nil {
		return nil, err
	}

	return &ExecutionPayloadHeaderCapella{
		ParentHash:       hexutil.Encode(payload.ParentHash),
		FeeRecipient:     hexutil.Encode(payload.FeeRecipient),
		StateRoot:        hexutil.Encode(payload.StateRoot),
		ReceiptsRoot:     hexutil.Encode(payload.ReceiptsRoot),
		LogsBloom:        hexutil.Encode(payload.LogsBloom),
		PrevRandao:       hexutil.Encode(payload.PrevRandao),
		BlockNumber:      fmt.Sprintf("%d", payload.BlockNumber),
		GasLimit:         fmt.Sprintf("%d", payload.GasLimit),
		GasUsed:          fmt.Sprintf("%d", payload.GasUsed),
		Timestamp:        fmt.Sprintf("%d", payload.Timestamp),
		ExtraData:        hexutil.Encode(payload.ExtraData),
		BaseFeePerGas:    baseFeePerGas,
		BlockHash:        hexutil.Encode(payload.BlockHash),
		TransactionsRoot: hexutil.Encode(payload.TransactionsRoot),
		WithdrawalsRoot:  hexutil.Encode(payload.WithdrawalsRoot),
	}, nil
}

// ----------------------------------------------------------------------------
// Deneb
// ----------------------------------------------------------------------------

func (b *SignedBeaconBlockContentsDeneb) ToGeneric() (*eth.GenericSignedBeaconBlock, error) {
	if b == nil {
		return nil, errNilValue
	}

	signedDenebBlock, err := b.SignedBlock.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "SignedBlock")
	}
	proofs := make([][]byte, len(b.KzgProofs))
	for i, proof := range b.KzgProofs {
		proofs[i], err = bytesutil.DecodeHexWithLength(proof, fieldparams.BLSPubkeyLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("KzgProofs[%d]", i))
		}
	}
	blbs := make([][]byte, len(b.Blobs))
	for i, blob := range b.Blobs {
		blbs[i], err = bytesutil.DecodeHexWithLength(blob, fieldparams.BlobLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Blobs[%d]", i))
		}
	}
	blk := &eth.SignedBeaconBlockContentsDeneb{
		Block:     signedDenebBlock,
		KzgProofs: proofs,
		Blobs:     blbs,
	}
	return &eth.GenericSignedBeaconBlock{Block: &eth.GenericSignedBeaconBlock_Deneb{Deneb: blk}}, nil
}

func (b *SignedBeaconBlockContentsDeneb) ToUnsigned() *BeaconBlockContentsDeneb {
	return &BeaconBlockContentsDeneb{
		Block:     b.SignedBlock.Message,
		KzgProofs: b.KzgProofs,
		Blobs:     b.Blobs,
	}
}

func (b *BeaconBlockContentsDeneb) ToGeneric() (*eth.GenericBeaconBlock, error) {
	block, err := b.ToConsensus()
	if err != nil {
		return nil, err
	}

	return &eth.GenericBeaconBlock{Block: &eth.GenericBeaconBlock_Deneb{Deneb: block}}, nil
}

func (b *BeaconBlockContentsDeneb) ToConsensus() (*eth.BeaconBlockContentsDeneb, error) {
	if b == nil {
		return nil, errNilValue
	}

	denebBlock, err := b.Block.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Block")
	}
	proofs := make([][]byte, len(b.KzgProofs))
	for i, proof := range b.KzgProofs {
		proofs[i], err = bytesutil.DecodeHexWithLength(proof, fieldparams.BLSPubkeyLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("KzgProofs[%d]", i))
		}
	}
	blbs := make([][]byte, len(b.Blobs))
	for i, blob := range b.Blobs {
		blbs[i], err = bytesutil.DecodeHexWithLength(blob, fieldparams.BlobLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Blobs[%d]", i))
		}
	}
	return &eth.BeaconBlockContentsDeneb{
		Block:     denebBlock,
		KzgProofs: proofs,
		Blobs:     blbs,
	}, nil
}

func (b *BeaconBlockDeneb) ToConsensus() (*eth.BeaconBlockDeneb, error) {
	if b == nil {
		return nil, errNilValue
	}
	if b.Body == nil {
		return nil, server.NewDecodeError(errNilValue, "Body")
	}
	if b.Body.Eth1Data == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.Eth1Data")
	}
	if b.Body.SyncAggregate == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.SyncAggregate")
	}
	if b.Body.ExecutionPayload == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.ExecutionPayload")
	}

	slot, err := strconv.ParseUint(b.Slot, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Slot")
	}
	proposerIndex, err := strconv.ParseUint(b.ProposerIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ProposerIndex")
	}
	parentRoot, err := bytesutil.DecodeHexWithLength(b.ParentRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ParentRoot")
	}
	stateRoot, err := bytesutil.DecodeHexWithLength(b.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "StateRoot")
	}
	randaoReveal, err := bytesutil.DecodeHexWithLength(b.Body.RandaoReveal, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.RandaoReveal")
	}
	depositRoot, err := bytesutil.DecodeHexWithLength(b.Body.Eth1Data.DepositRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.DepositRoot")
	}
	depositCount, err := strconv.ParseUint(b.Body.Eth1Data.DepositCount, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.DepositCount")
	}
	blockHash, err := bytesutil.DecodeHexWithLength(b.Body.Eth1Data.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.BlockHash")
	}
	graffiti, err := bytesutil.DecodeHexWithLength(b.Body.Graffiti, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Graffiti")
	}
	proposerSlashings, err := ProposerSlashingsToConsensus(b.Body.ProposerSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ProposerSlashings")
	}
	attesterSlashings, err := AttesterSlashingsToConsensus(b.Body.AttesterSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.AttesterSlashings")
	}
	atts, err := AttsToConsensus(b.Body.Attestations)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Attestations")
	}
	deposits, err := DepositsToConsensus(b.Body.Deposits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Deposits")
	}
	exits, err := SignedExitsToConsensus(b.Body.VoluntaryExits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.VoluntaryExits")
	}
	syncCommitteeBits, err := bytesutil.DecodeHexWithLength(b.Body.SyncAggregate.SyncCommitteeBits, fieldparams.SyncAggregateSyncCommitteeBytesLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeBits")
	}
	syncCommitteeSig, err := bytesutil.DecodeHexWithLength(b.Body.SyncAggregate.SyncCommitteeSignature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeSignature")
	}
	payloadParentHash, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.ParentHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.ParentHash")
	}
	payloadFeeRecipient, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.FeeRecipient, fieldparams.FeeRecipientLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.FeeRecipient")
	}
	payloadStateRoot, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.StateRoot")
	}
	payloadReceiptsRoot, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.ReceiptsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.ReceiptsRoot")
	}
	payloadLogsBloom, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.LogsBloom, fieldparams.LogsBloomLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.LogsBloom")
	}
	payloadPrevRandao, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.PrevRandao, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.PrevRandao")
	}
	payloadBlockNumber, err := strconv.ParseUint(b.Body.ExecutionPayload.BlockNumber, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.BlockNumber")
	}
	payloadGasLimit, err := strconv.ParseUint(b.Body.ExecutionPayload.GasLimit, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.GasLimit")
	}
	payloadGasUsed, err := strconv.ParseUint(b.Body.ExecutionPayload.GasUsed, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.GasUsed")
	}
	payloadTimestamp, err := strconv.ParseUint(b.Body.ExecutionPayload.Timestamp, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.Timestamp")
	}
	payloadExtraData, err := bytesutil.DecodeHexWithMaxLength(b.Body.ExecutionPayload.ExtraData, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.ExtraData")
	}
	payloadBaseFeePerGas, err := bytesutil.Uint256ToSSZBytes(b.Body.ExecutionPayload.BaseFeePerGas)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.BaseFeePerGas")
	}
	payloadBlockHash, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.BlockHash")
	}
	err = slice.VerifyMaxLength(b.Body.ExecutionPayload.Transactions, fieldparams.MaxTxsPerPayloadLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.Transactions")
	}
	txs := make([][]byte, len(b.Body.ExecutionPayload.Transactions))
	for i, tx := range b.Body.ExecutionPayload.Transactions {
		txs[i], err = bytesutil.DecodeHexWithMaxLength(tx, fieldparams.MaxBytesPerTxLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Transactions[%d]", i))
		}
	}
	err = slice.VerifyMaxLength(b.Body.ExecutionPayload.Withdrawals, fieldparams.MaxWithdrawalsPerPayload)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.Withdrawals")
	}
	withdrawals := make([]*enginev1.Withdrawal, len(b.Body.ExecutionPayload.Withdrawals))
	for i, w := range b.Body.ExecutionPayload.Withdrawals {
		withdrawalIndex, err := strconv.ParseUint(w.WithdrawalIndex, 10, 64)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Withdrawals[%d].WithdrawalIndex", i))
		}
		validatorIndex, err := strconv.ParseUint(w.ValidatorIndex, 10, 64)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Withdrawals[%d].ValidatorIndex", i))
		}
		address, err := bytesutil.DecodeHexWithLength(w.ExecutionAddress, common.AddressLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Withdrawals[%d].ExecutionAddress", i))
		}
		amount, err := strconv.ParseUint(w.Amount, 10, 64)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Withdrawals[%d].Amount", i))
		}
		withdrawals[i] = &enginev1.Withdrawal{
			Index:          withdrawalIndex,
			ValidatorIndex: primitives.ValidatorIndex(validatorIndex),
			Address:        address,
			Amount:         amount,
		}
	}

	payloadBlobGasUsed, err := strconv.ParseUint(b.Body.ExecutionPayload.BlobGasUsed, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.BlobGasUsed")
	}
	payloadExcessBlobGas, err := strconv.ParseUint(b.Body.ExecutionPayload.ExcessBlobGas, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.ExcessBlobGas")
	}
	blsChanges, err := SignedBLSChangesToConsensus(b.Body.BLSToExecutionChanges)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.BLSToExecutionChanges")
	}
	err = slice.VerifyMaxLength(b.Body.BlobKzgCommitments, fieldparams.MaxBlobCommitmentsPerBlock)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.BlobKzgCommitments")
	}
	blobKzgCommitments := make([][]byte, len(b.Body.BlobKzgCommitments))
	for i, b := range b.Body.BlobKzgCommitments {
		kzg, err := bytesutil.DecodeHexWithLength(b, fieldparams.BLSPubkeyLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.BlobKzgCommitments[%d]", i))
		}
		blobKzgCommitments[i] = kzg
	}
	return &eth.BeaconBlockDeneb{
		Slot:          primitives.Slot(slot),
		ProposerIndex: primitives.ValidatorIndex(proposerIndex),
		ParentRoot:    parentRoot,
		StateRoot:     stateRoot,
		Body: &eth.BeaconBlockBodyDeneb{
			RandaoReveal: randaoReveal,
			Eth1Data: &eth.Eth1Data{
				DepositRoot:  depositRoot,
				DepositCount: depositCount,
				BlockHash:    blockHash,
			},
			Graffiti:          graffiti,
			ProposerSlashings: proposerSlashings,
			AttesterSlashings: attesterSlashings,
			Attestations:      atts,
			Deposits:          deposits,
			VoluntaryExits:    exits,
			SyncAggregate: &eth.SyncAggregate{
				SyncCommitteeBits:      syncCommitteeBits,
				SyncCommitteeSignature: syncCommitteeSig,
			},
			ExecutionPayload: &enginev1.ExecutionPayloadDeneb{
				ParentHash:    payloadParentHash,
				FeeRecipient:  payloadFeeRecipient,
				StateRoot:     payloadStateRoot,
				ReceiptsRoot:  payloadReceiptsRoot,
				LogsBloom:     payloadLogsBloom,
				PrevRandao:    payloadPrevRandao,
				BlockNumber:   payloadBlockNumber,
				GasLimit:      payloadGasLimit,
				GasUsed:       payloadGasUsed,
				Timestamp:     payloadTimestamp,
				ExtraData:     payloadExtraData,
				BaseFeePerGas: payloadBaseFeePerGas,
				BlockHash:     payloadBlockHash,
				Transactions:  txs,
				Withdrawals:   withdrawals,
				BlobGasUsed:   payloadBlobGasUsed,
				ExcessBlobGas: payloadExcessBlobGas,
			},
			BlsToExecutionChanges: blsChanges,
			BlobKzgCommitments:    blobKzgCommitments,
		},
	}, nil
}

func (b *SignedBeaconBlockDeneb) ToConsensus() (*eth.SignedBeaconBlockDeneb, error) {
	if b == nil {
		return nil, errNilValue
	}

	sig, err := bytesutil.DecodeHexWithLength(b.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}
	block, err := b.Message.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Message")
	}
	return &eth.SignedBeaconBlockDeneb{
		Block:     block,
		Signature: sig,
	}, nil
}

func (b *SignedBlindedBeaconBlockDeneb) ToConsensus() (*eth.SignedBlindedBeaconBlockDeneb, error) {
	if b == nil {
		return nil, errNilValue
	}

	sig, err := bytesutil.DecodeHexWithLength(b.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}
	blindedBlock, err := b.Message.ToConsensus()
	if err != nil {
		return nil, err
	}
	return &eth.SignedBlindedBeaconBlockDeneb{
		Message:   blindedBlock,
		Signature: sig,
	}, nil
}

func (b *SignedBlindedBeaconBlockDeneb) ToGeneric() (*eth.GenericSignedBeaconBlock, error) {
	if b == nil {
		return nil, errNilValue
	}
	sig, err := bytesutil.DecodeHexWithLength(b.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}
	blindedBlock, err := b.Message.ToConsensus()
	if err != nil {
		return nil, err
	}
	return &eth.GenericSignedBeaconBlock{Block: &eth.GenericSignedBeaconBlock_BlindedDeneb{BlindedDeneb: &eth.SignedBlindedBeaconBlockDeneb{
		Message:   blindedBlock,
		Signature: sig,
	}}, IsBlinded: true}, nil
}

func (b *BlindedBeaconBlockDeneb) ToConsensus() (*eth.BlindedBeaconBlockDeneb, error) {
	if b == nil {
		return nil, errNilValue
	}
	if b.Body == nil {
		return nil, server.NewDecodeError(errNilValue, "Body")
	}
	if b.Body.Eth1Data == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.Eth1Data")
	}
	if b.Body.SyncAggregate == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.SyncAggregate")
	}
	if b.Body.ExecutionPayloadHeader == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.ExecutionPayloadHeader")
	}

	slot, err := strconv.ParseUint(b.Slot, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Slot")
	}
	proposerIndex, err := strconv.ParseUint(b.ProposerIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ProposerIndex")
	}
	parentRoot, err := bytesutil.DecodeHexWithLength(b.ParentRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ParentRoot")
	}
	stateRoot, err := bytesutil.DecodeHexWithLength(b.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "StateRoot")
	}
	randaoReveal, err := bytesutil.DecodeHexWithLength(b.Body.RandaoReveal, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.RandaoReveal")
	}
	depositRoot, err := bytesutil.DecodeHexWithLength(b.Body.Eth1Data.DepositRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.DepositRoot")
	}
	depositCount, err := strconv.ParseUint(b.Body.Eth1Data.DepositCount, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.DepositCount")
	}
	blockHash, err := bytesutil.DecodeHexWithLength(b.Body.Eth1Data.BlockHash, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.BlockHash")
	}
	graffiti, err := bytesutil.DecodeHexWithLength(b.Body.Graffiti, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Graffiti")
	}
	proposerSlashings, err := ProposerSlashingsToConsensus(b.Body.ProposerSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ProposerSlashings")
	}
	attesterSlashings, err := AttesterSlashingsToConsensus(b.Body.AttesterSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.AttesterSlashings")
	}
	atts, err := AttsToConsensus(b.Body.Attestations)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Attestations")
	}
	deposits, err := DepositsToConsensus(b.Body.Deposits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Deposits")
	}
	exits, err := SignedExitsToConsensus(b.Body.VoluntaryExits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.VoluntaryExits")
	}
	syncCommitteeBits, err := bytesutil.DecodeHexWithLength(b.Body.SyncAggregate.SyncCommitteeBits, fieldparams.SyncAggregateSyncCommitteeBytesLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeBits")
	}
	syncCommitteeSig, err := bytesutil.DecodeHexWithLength(b.Body.SyncAggregate.SyncCommitteeSignature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeSignature")
	}
	payloadParentHash, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.ParentHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.ParentHash")
	}
	payloadFeeRecipient, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.FeeRecipient, fieldparams.FeeRecipientLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.FeeRecipient")
	}
	payloadStateRoot, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.StateRoot")
	}
	payloadReceiptsRoot, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.ReceiptsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.ReceiptsRoot")
	}
	payloadLogsBloom, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.LogsBloom, fieldparams.LogsBloomLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.LogsBloom")
	}
	payloadPrevRandao, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.PrevRandao, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.PrevRandao")
	}
	payloadBlockNumber, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.BlockNumber, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.BlockNumber")
	}
	payloadGasLimit, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.GasLimit, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.GasLimit")
	}
	payloadGasUsed, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.GasUsed, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.GasUsed")
	}
	payloadTimestamp, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.Timestamp, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.Timestamp")
	}
	payloadExtraData, err := bytesutil.DecodeHexWithMaxLength(b.Body.ExecutionPayloadHeader.ExtraData, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.ExtraData")
	}
	payloadBaseFeePerGas, err := bytesutil.Uint256ToSSZBytes(b.Body.ExecutionPayloadHeader.BaseFeePerGas)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.BaseFeePerGas")
	}
	payloadBlockHash, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.BlockHash")
	}
	payloadTxsRoot, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.TransactionsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.TransactionsRoot")
	}
	payloadWithdrawalsRoot, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.WithdrawalsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.WithdrawalsRoot")
	}

	payloadBlobGasUsed, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.BlobGasUsed, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.BlobGasUsed")
	}
	payloadExcessBlobGas, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.ExcessBlobGas, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.ExcessBlobGas")
	}

	blsChanges, err := SignedBLSChangesToConsensus(b.Body.BLSToExecutionChanges)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.BLSToExecutionChanges")
	}
	err = slice.VerifyMaxLength(b.Body.BlobKzgCommitments, fieldparams.MaxBlobCommitmentsPerBlock)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.BlobKzgCommitments")
	}
	blobKzgCommitments := make([][]byte, len(b.Body.BlobKzgCommitments))
	for i, b := range b.Body.BlobKzgCommitments {
		kzg, err := bytesutil.DecodeHexWithLength(b, fieldparams.BLSPubkeyLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.BlobKzgCommitments[%d]", i))
		}
		blobKzgCommitments[i] = kzg
	}

	return &eth.BlindedBeaconBlockDeneb{
		Slot:          primitives.Slot(slot),
		ProposerIndex: primitives.ValidatorIndex(proposerIndex),
		ParentRoot:    parentRoot,
		StateRoot:     stateRoot,
		Body: &eth.BlindedBeaconBlockBodyDeneb{
			RandaoReveal: randaoReveal,
			Eth1Data: &eth.Eth1Data{
				DepositRoot:  depositRoot,
				DepositCount: depositCount,
				BlockHash:    blockHash,
			},
			Graffiti:          graffiti,
			ProposerSlashings: proposerSlashings,
			AttesterSlashings: attesterSlashings,
			Attestations:      atts,
			Deposits:          deposits,
			VoluntaryExits:    exits,
			SyncAggregate: &eth.SyncAggregate{
				SyncCommitteeBits:      syncCommitteeBits,
				SyncCommitteeSignature: syncCommitteeSig,
			},
			ExecutionPayloadHeader: &enginev1.ExecutionPayloadHeaderDeneb{
				ParentHash:       payloadParentHash,
				FeeRecipient:     payloadFeeRecipient,
				StateRoot:        payloadStateRoot,
				ReceiptsRoot:     payloadReceiptsRoot,
				LogsBloom:        payloadLogsBloom,
				PrevRandao:       payloadPrevRandao,
				BlockNumber:      payloadBlockNumber,
				GasLimit:         payloadGasLimit,
				GasUsed:          payloadGasUsed,
				Timestamp:        payloadTimestamp,
				ExtraData:        payloadExtraData,
				BaseFeePerGas:    payloadBaseFeePerGas,
				BlockHash:        payloadBlockHash,
				TransactionsRoot: payloadTxsRoot,
				WithdrawalsRoot:  payloadWithdrawalsRoot,
				BlobGasUsed:      payloadBlobGasUsed,
				ExcessBlobGas:    payloadExcessBlobGas,
			},
			BlsToExecutionChanges: blsChanges,
			BlobKzgCommitments:    blobKzgCommitments,
		},
	}, nil
}

func (b *BlindedBeaconBlockDeneb) ToGeneric() (*eth.GenericBeaconBlock, error) {
	if b == nil {
		return nil, errNilValue
	}

	blindedBlock, err := b.ToConsensus()
	if err != nil {
		return nil, err
	}
	return &eth.GenericBeaconBlock{Block: &eth.GenericBeaconBlock_BlindedDeneb{BlindedDeneb: blindedBlock}, IsBlinded: true}, nil
}

func BeaconBlockContentsDenebFromConsensus(b *eth.BeaconBlockContentsDeneb) (*BeaconBlockContentsDeneb, error) {
	block, err := BeaconBlockDenebFromConsensus(b.Block)
	if err != nil {
		return nil, err
	}
	proofs := make([]string, len(b.KzgProofs))
	for i, proof := range b.KzgProofs {
		proofs[i] = hexutil.Encode(proof)
	}
	blbs := make([]string, len(b.Blobs))
	for i, blob := range b.Blobs {
		blbs[i] = hexutil.Encode(blob)
	}
	return &BeaconBlockContentsDeneb{
		Block:     block,
		KzgProofs: proofs,
		Blobs:     blbs,
	}, nil
}

func SignedBeaconBlockContentsDenebFromConsensus(b *eth.SignedBeaconBlockContentsDeneb) (*SignedBeaconBlockContentsDeneb, error) {
	block, err := SignedBeaconBlockDenebFromConsensus(b.Block)
	if err != nil {
		return nil, err
	}

	proofs := make([]string, len(b.KzgProofs))
	for i, proof := range b.KzgProofs {
		proofs[i] = hexutil.Encode(proof)
	}

	blbs := make([]string, len(b.Blobs))
	for i, blob := range b.Blobs {
		blbs[i] = hexutil.Encode(blob)
	}

	return &SignedBeaconBlockContentsDeneb{
		SignedBlock: block,
		KzgProofs:   proofs,
		Blobs:       blbs,
	}, nil
}

func BlindedBeaconBlockDenebFromConsensus(b *eth.BlindedBeaconBlockDeneb) (*BlindedBeaconBlockDeneb, error) {
	blobKzgCommitments := make([]string, len(b.Body.BlobKzgCommitments))
	for i := range b.Body.BlobKzgCommitments {
		blobKzgCommitments[i] = hexutil.Encode(b.Body.BlobKzgCommitments[i])
	}
	payload, err := ExecutionPayloadHeaderDenebFromConsensus(b.Body.ExecutionPayloadHeader)
	if err != nil {
		return nil, err
	}

	return &BlindedBeaconBlockDeneb{
		Slot:          fmt.Sprintf("%d", b.Slot),
		ProposerIndex: fmt.Sprintf("%d", b.ProposerIndex),
		ParentRoot:    hexutil.Encode(b.ParentRoot),
		StateRoot:     hexutil.Encode(b.StateRoot),
		Body: &BlindedBeaconBlockBodyDeneb{
			RandaoReveal:      hexutil.Encode(b.Body.RandaoReveal),
			Eth1Data:          Eth1DataFromConsensus(b.Body.Eth1Data),
			Graffiti:          hexutil.Encode(b.Body.Graffiti),
			ProposerSlashings: ProposerSlashingsFromConsensus(b.Body.ProposerSlashings),
			AttesterSlashings: AttesterSlashingsFromConsensus(b.Body.AttesterSlashings),
			Attestations:      AttsFromConsensus(b.Body.Attestations),
			Deposits:          DepositsFromConsensus(b.Body.Deposits),
			VoluntaryExits:    SignedExitsFromConsensus(b.Body.VoluntaryExits),
			SyncAggregate: &SyncAggregate{
				SyncCommitteeBits:      hexutil.Encode(b.Body.SyncAggregate.SyncCommitteeBits),
				SyncCommitteeSignature: hexutil.Encode(b.Body.SyncAggregate.SyncCommitteeSignature),
			},
			ExecutionPayloadHeader: payload,
			BLSToExecutionChanges:  SignedBLSChangesFromConsensus(b.Body.BlsToExecutionChanges),
			BlobKzgCommitments:     blobKzgCommitments,
		},
	}, nil
}

func SignedBlindedBeaconBlockDenebFromConsensus(b *eth.SignedBlindedBeaconBlockDeneb) (*SignedBlindedBeaconBlockDeneb, error) {
	block, err := BlindedBeaconBlockDenebFromConsensus(b.Message)
	if err != nil {
		return nil, err
	}
	return &SignedBlindedBeaconBlockDeneb{
		Message:   block,
		Signature: hexutil.Encode(b.Signature),
	}, nil
}

func BeaconBlockDenebFromConsensus(b *eth.BeaconBlockDeneb) (*BeaconBlockDeneb, error) {
	baseFeePerGas, err := sszBytesToUint256String(b.Body.ExecutionPayload.BaseFeePerGas)
	if err != nil {
		return nil, err
	}
	transactions := make([]string, len(b.Body.ExecutionPayload.Transactions))
	for i, tx := range b.Body.ExecutionPayload.Transactions {
		transactions[i] = hexutil.Encode(tx)
	}
	blobKzgCommitments := make([]string, len(b.Body.BlobKzgCommitments))
	for i := range b.Body.BlobKzgCommitments {
		blobKzgCommitments[i] = hexutil.Encode(b.Body.BlobKzgCommitments[i])
	}

	return &BeaconBlockDeneb{
		Slot:          fmt.Sprintf("%d", b.Slot),
		ProposerIndex: fmt.Sprintf("%d", b.ProposerIndex),
		ParentRoot:    hexutil.Encode(b.ParentRoot),
		StateRoot:     hexutil.Encode(b.StateRoot),
		Body: &BeaconBlockBodyDeneb{
			RandaoReveal:      hexutil.Encode(b.Body.RandaoReveal),
			Eth1Data:          Eth1DataFromConsensus(b.Body.Eth1Data),
			Graffiti:          hexutil.Encode(b.Body.Graffiti),
			ProposerSlashings: ProposerSlashingsFromConsensus(b.Body.ProposerSlashings),
			AttesterSlashings: AttesterSlashingsFromConsensus(b.Body.AttesterSlashings),
			Attestations:      AttsFromConsensus(b.Body.Attestations),
			Deposits:          DepositsFromConsensus(b.Body.Deposits),
			VoluntaryExits:    SignedExitsFromConsensus(b.Body.VoluntaryExits),
			SyncAggregate: &SyncAggregate{
				SyncCommitteeBits:      hexutil.Encode(b.Body.SyncAggregate.SyncCommitteeBits),
				SyncCommitteeSignature: hexutil.Encode(b.Body.SyncAggregate.SyncCommitteeSignature),
			},
			ExecutionPayload: &ExecutionPayloadDeneb{
				ParentHash:    hexutil.Encode(b.Body.ExecutionPayload.ParentHash),
				FeeRecipient:  hexutil.Encode(b.Body.ExecutionPayload.FeeRecipient),
				StateRoot:     hexutil.Encode(b.Body.ExecutionPayload.StateRoot),
				ReceiptsRoot:  hexutil.Encode(b.Body.ExecutionPayload.ReceiptsRoot),
				LogsBloom:     hexutil.Encode(b.Body.ExecutionPayload.LogsBloom),
				PrevRandao:    hexutil.Encode(b.Body.ExecutionPayload.PrevRandao),
				BlockNumber:   fmt.Sprintf("%d", b.Body.ExecutionPayload.BlockNumber),
				GasLimit:      fmt.Sprintf("%d", b.Body.ExecutionPayload.GasLimit),
				GasUsed:       fmt.Sprintf("%d", b.Body.ExecutionPayload.GasUsed),
				Timestamp:     fmt.Sprintf("%d", b.Body.ExecutionPayload.Timestamp),
				ExtraData:     hexutil.Encode(b.Body.ExecutionPayload.ExtraData),
				BaseFeePerGas: baseFeePerGas,
				BlockHash:     hexutil.Encode(b.Body.ExecutionPayload.BlockHash),
				Transactions:  transactions,
				Withdrawals:   WithdrawalsFromConsensus(b.Body.ExecutionPayload.Withdrawals),
				BlobGasUsed:   fmt.Sprintf("%d", b.Body.ExecutionPayload.BlobGasUsed),
				ExcessBlobGas: fmt.Sprintf("%d", b.Body.ExecutionPayload.ExcessBlobGas),
			},
			BLSToExecutionChanges: SignedBLSChangesFromConsensus(b.Body.BlsToExecutionChanges),
			BlobKzgCommitments:    blobKzgCommitments,
		},
	}, nil
}

func SignedBeaconBlockDenebFromConsensus(b *eth.SignedBeaconBlockDeneb) (*SignedBeaconBlockDeneb, error) {
	block, err := BeaconBlockDenebFromConsensus(b.Block)
	if err != nil {
		return nil, err
	}
	return &SignedBeaconBlockDeneb{
		Message:   block,
		Signature: hexutil.Encode(b.Signature),
	}, nil
}

func ExecutionPayloadDenebFromConsensus(payload *enginev1.ExecutionPayloadDeneb) (*ExecutionPayloadDeneb, error) {
	baseFeePerGas, err := sszBytesToUint256String(payload.BaseFeePerGas)
	if err != nil {
		return nil, err
	}
	transactions := make([]string, len(payload.Transactions))
	for i, tx := range payload.Transactions {
		transactions[i] = hexutil.Encode(tx)
	}

	return &ExecutionPayloadDeneb{
		ParentHash:    hexutil.Encode(payload.ParentHash),
		FeeRecipient:  hexutil.Encode(payload.FeeRecipient),
		StateRoot:     hexutil.Encode(payload.StateRoot),
		ReceiptsRoot:  hexutil.Encode(payload.ReceiptsRoot),
		LogsBloom:     hexutil.Encode(payload.LogsBloom),
		PrevRandao:    hexutil.Encode(payload.PrevRandao),
		BlockNumber:   fmt.Sprintf("%d", payload.BlockNumber),
		GasLimit:      fmt.Sprintf("%d", payload.GasLimit),
		GasUsed:       fmt.Sprintf("%d", payload.GasUsed),
		Timestamp:     fmt.Sprintf("%d", payload.Timestamp),
		ExtraData:     hexutil.Encode(payload.ExtraData),
		BaseFeePerGas: baseFeePerGas,
		BlockHash:     hexutil.Encode(payload.BlockHash),
		Transactions:  transactions,
		Withdrawals:   WithdrawalsFromConsensus(payload.Withdrawals),
		BlobGasUsed:   fmt.Sprintf("%d", payload.BlobGasUsed),
		ExcessBlobGas: fmt.Sprintf("%d", payload.ExcessBlobGas),
	}, nil
}

func ExecutionPayloadHeaderDenebFromConsensus(payload *enginev1.ExecutionPayloadHeaderDeneb) (*ExecutionPayloadHeaderDeneb, error) {
	baseFeePerGas, err := sszBytesToUint256String(payload.BaseFeePerGas)
	if err != nil {
		return nil, err
	}

	return &ExecutionPayloadHeaderDeneb{
		ParentHash:       hexutil.Encode(payload.ParentHash),
		FeeRecipient:     hexutil.Encode(payload.FeeRecipient),
		StateRoot:        hexutil.Encode(payload.StateRoot),
		ReceiptsRoot:     hexutil.Encode(payload.ReceiptsRoot),
		LogsBloom:        hexutil.Encode(payload.LogsBloom),
		PrevRandao:       hexutil.Encode(payload.PrevRandao),
		BlockNumber:      fmt.Sprintf("%d", payload.BlockNumber),
		GasLimit:         fmt.Sprintf("%d", payload.GasLimit),
		GasUsed:          fmt.Sprintf("%d", payload.GasUsed),
		Timestamp:        fmt.Sprintf("%d", payload.Timestamp),
		ExtraData:        hexutil.Encode(payload.ExtraData),
		BaseFeePerGas:    baseFeePerGas,
		BlockHash:        hexutil.Encode(payload.BlockHash),
		TransactionsRoot: hexutil.Encode(payload.TransactionsRoot),
		WithdrawalsRoot:  hexutil.Encode(payload.WithdrawalsRoot),
		BlobGasUsed:      fmt.Sprintf("%d", payload.BlobGasUsed),
		ExcessBlobGas:    fmt.Sprintf("%d", payload.ExcessBlobGas),
	}, nil
}

// ----------------------------------------------------------------------------
// Electra
// ----------------------------------------------------------------------------

func (b *SignedBeaconBlockContentsElectra) ToGeneric() (*eth.GenericSignedBeaconBlock, error) {
	if b == nil {
		return nil, errNilValue
	}

	signedElectraBlock, err := b.SignedBlock.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "SignedBlock")
	}
	proofs := make([][]byte, len(b.KzgProofs))
	for i, proof := range b.KzgProofs {
		proofs[i], err = bytesutil.DecodeHexWithLength(proof, fieldparams.BLSPubkeyLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("KzgProofs[%d]", i))
		}
	}
	blbs := make([][]byte, len(b.Blobs))
	for i, blob := range b.Blobs {
		blbs[i], err = bytesutil.DecodeHexWithLength(blob, fieldparams.BlobLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Blobs[%d]", i))
		}
	}
	blk := &eth.SignedBeaconBlockContentsElectra{
		Block:     signedElectraBlock,
		KzgProofs: proofs,
		Blobs:     blbs,
	}
	return &eth.GenericSignedBeaconBlock{Block: &eth.GenericSignedBeaconBlock_Electra{Electra: blk}}, nil
}

func (b *SignedBeaconBlockContentsElectra) ToUnsigned() *BeaconBlockContentsElectra {
	return &BeaconBlockContentsElectra{
		Block:     b.SignedBlock.Message,
		KzgProofs: b.KzgProofs,
		Blobs:     b.Blobs,
	}
}

func (b *BeaconBlockContentsElectra) ToGeneric() (*eth.GenericBeaconBlock, error) {
	block, err := b.ToConsensus()
	if err != nil {
		return nil, err
	}

	return &eth.GenericBeaconBlock{Block: &eth.GenericBeaconBlock_Electra{Electra: block}}, nil
}

func (b *BeaconBlockContentsElectra) ToConsensus() (*eth.BeaconBlockContentsElectra, error) {
	if b == nil {
		return nil, errNilValue
	}

	electraBlock, err := b.Block.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Block")
	}
	proofs := make([][]byte, len(b.KzgProofs))
	for i, proof := range b.KzgProofs {
		proofs[i], err = bytesutil.DecodeHexWithLength(proof, fieldparams.BLSPubkeyLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("KzgProofs[%d]", i))
		}
	}
	blbs := make([][]byte, len(b.Blobs))
	for i, blob := range b.Blobs {
		blbs[i], err = bytesutil.DecodeHexWithLength(blob, fieldparams.BlobLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Blobs[%d]", i))
		}
	}
	return &eth.BeaconBlockContentsElectra{
		Block:     electraBlock,
		KzgProofs: proofs,
		Blobs:     blbs,
	}, nil
}

func (b *BeaconBlockElectra) ToConsensus() (*eth.BeaconBlockElectra, error) {
	if b == nil {
		return nil, errNilValue
	}
	if b.Body == nil {
		return nil, server.NewDecodeError(errNilValue, "Body")
	}
	if b.Body.Eth1Data == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.Eth1Data")
	}
	if b.Body.SyncAggregate == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.SyncAggregate")
	}
	if b.Body.ExecutionPayload == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.ExecutionPayload")
	}

	slot, err := strconv.ParseUint(b.Slot, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Slot")
	}
	proposerIndex, err := strconv.ParseUint(b.ProposerIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ProposerIndex")
	}
	parentRoot, err := bytesutil.DecodeHexWithLength(b.ParentRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ParentRoot")
	}
	stateRoot, err := bytesutil.DecodeHexWithLength(b.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "StateRoot")
	}
	randaoReveal, err := bytesutil.DecodeHexWithLength(b.Body.RandaoReveal, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.RandaoReveal")
	}
	depositRoot, err := bytesutil.DecodeHexWithLength(b.Body.Eth1Data.DepositRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.DepositRoot")
	}
	depositCount, err := strconv.ParseUint(b.Body.Eth1Data.DepositCount, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.DepositCount")
	}
	blockHash, err := bytesutil.DecodeHexWithLength(b.Body.Eth1Data.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.BlockHash")
	}
	graffiti, err := bytesutil.DecodeHexWithLength(b.Body.Graffiti, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Graffiti")
	}
	proposerSlashings, err := ProposerSlashingsToConsensus(b.Body.ProposerSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ProposerSlashings")
	}
	attesterSlashings, err := AttesterSlashingsElectraToConsensus(b.Body.AttesterSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.AttesterSlashings")
	}
	atts, err := AttsElectraToConsensus(b.Body.Attestations)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Attestations")
	}
	deposits, err := DepositsToConsensus(b.Body.Deposits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Deposits")
	}
	exits, err := SignedExitsToConsensus(b.Body.VoluntaryExits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.VoluntaryExits")
	}
	syncCommitteeBits, err := bytesutil.DecodeHexWithLength(b.Body.SyncAggregate.SyncCommitteeBits, fieldparams.SyncAggregateSyncCommitteeBytesLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeBits")
	}
	syncCommitteeSig, err := bytesutil.DecodeHexWithLength(b.Body.SyncAggregate.SyncCommitteeSignature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeSignature")
	}
	payloadParentHash, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.ParentHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.ParentHash")
	}
	payloadFeeRecipient, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.FeeRecipient, fieldparams.FeeRecipientLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.FeeRecipient")
	}
	payloadStateRoot, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.StateRoot")
	}
	payloadReceiptsRoot, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.ReceiptsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.ReceiptsRoot")
	}
	payloadLogsBloom, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.LogsBloom, fieldparams.LogsBloomLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.LogsBloom")
	}
	payloadPrevRandao, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.PrevRandao, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.PrevRandao")
	}
	payloadBlockNumber, err := strconv.ParseUint(b.Body.ExecutionPayload.BlockNumber, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.BlockNumber")
	}
	payloadGasLimit, err := strconv.ParseUint(b.Body.ExecutionPayload.GasLimit, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.GasLimit")
	}
	payloadGasUsed, err := strconv.ParseUint(b.Body.ExecutionPayload.GasUsed, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.GasUsed")
	}
	payloadTimestamp, err := strconv.ParseUint(b.Body.ExecutionPayload.Timestamp, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.Timestamp")
	}
	payloadExtraData, err := bytesutil.DecodeHexWithMaxLength(b.Body.ExecutionPayload.ExtraData, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.ExtraData")
	}
	payloadBaseFeePerGas, err := bytesutil.Uint256ToSSZBytes(b.Body.ExecutionPayload.BaseFeePerGas)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.BaseFeePerGas")
	}
	payloadBlockHash, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.BlockHash")
	}
	err = slice.VerifyMaxLength(b.Body.ExecutionPayload.Transactions, fieldparams.MaxTxsPerPayloadLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.Transactions")
	}
	txs := make([][]byte, len(b.Body.ExecutionPayload.Transactions))
	for i, tx := range b.Body.ExecutionPayload.Transactions {
		txs[i], err = bytesutil.DecodeHexWithMaxLength(tx, fieldparams.MaxBytesPerTxLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Transactions[%d]", i))
		}
	}
	err = slice.VerifyMaxLength(b.Body.ExecutionPayload.Withdrawals, fieldparams.MaxWithdrawalsPerPayload)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.Withdrawals")
	}
	withdrawals := make([]*enginev1.Withdrawal, len(b.Body.ExecutionPayload.Withdrawals))
	for i, w := range b.Body.ExecutionPayload.Withdrawals {
		withdrawalIndex, err := strconv.ParseUint(w.WithdrawalIndex, 10, 64)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Withdrawals[%d].WithdrawalIndex", i))
		}
		validatorIndex, err := strconv.ParseUint(w.ValidatorIndex, 10, 64)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Withdrawals[%d].ValidatorIndex", i))
		}
		address, err := bytesutil.DecodeHexWithLength(w.ExecutionAddress, common.AddressLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Withdrawals[%d].ExecutionAddress", i))
		}
		amount, err := strconv.ParseUint(w.Amount, 10, 64)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Withdrawals[%d].Amount", i))
		}
		withdrawals[i] = &enginev1.Withdrawal{
			Index:          withdrawalIndex,
			ValidatorIndex: primitives.ValidatorIndex(validatorIndex),
			Address:        address,
			Amount:         amount,
		}
	}

	payloadBlobGasUsed, err := strconv.ParseUint(b.Body.ExecutionPayload.BlobGasUsed, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.BlobGasUsed")
	}
	payloadExcessBlobGas, err := strconv.ParseUint(b.Body.ExecutionPayload.ExcessBlobGas, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.ExcessBlobGas")
	}

	if b.Body.ExecutionRequests == nil {
		return nil, server.NewDecodeError(errors.New("nil execution requests"), "Body.ExequtionRequests")
	}

	depositRequests := make([]*enginev1.DepositRequest, len(b.Body.ExecutionRequests.Deposits))
	for i, d := range b.Body.ExecutionRequests.Deposits {
		depositRequests[i], err = d.ToConsensus()
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionRequests.Deposits[%d]", i))
		}
	}

	withdrawalRequests := make([]*enginev1.WithdrawalRequest, len(b.Body.ExecutionRequests.Withdrawals))
	for i, w := range b.Body.ExecutionRequests.Withdrawals {
		withdrawalRequests[i], err = w.ToConsensus()
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionRequests.Withdrawals[%d]", i))
		}
	}

	consolidationRequests := make([]*enginev1.ConsolidationRequest, len(b.Body.ExecutionRequests.Consolidations))
	for i, c := range b.Body.ExecutionRequests.Consolidations {
		consolidationRequests[i], err = c.ToConsensus()
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionRequests.Consolidations[%d]", i))
		}
	}

	blsChanges, err := SignedBLSChangesToConsensus(b.Body.BLSToExecutionChanges)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.BLSToExecutionChanges")
	}
	err = slice.VerifyMaxLength(b.Body.BlobKzgCommitments, fieldparams.MaxBlobCommitmentsPerBlock)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.BlobKzgCommitments")
	}
	blobKzgCommitments := make([][]byte, len(b.Body.BlobKzgCommitments))
	for i, b := range b.Body.BlobKzgCommitments {
		kzg, err := bytesutil.DecodeHexWithLength(b, fieldparams.BLSPubkeyLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.BlobKzgCommitments[%d]", i))
		}
		blobKzgCommitments[i] = kzg
	}
	return &eth.BeaconBlockElectra{
		Slot:          primitives.Slot(slot),
		ProposerIndex: primitives.ValidatorIndex(proposerIndex),
		ParentRoot:    parentRoot,
		StateRoot:     stateRoot,
		Body: &eth.BeaconBlockBodyElectra{
			RandaoReveal: randaoReveal,
			Eth1Data: &eth.Eth1Data{
				DepositRoot:  depositRoot,
				DepositCount: depositCount,
				BlockHash:    blockHash,
			},
			Graffiti:          graffiti,
			ProposerSlashings: proposerSlashings,
			AttesterSlashings: attesterSlashings,
			Attestations:      atts,
			Deposits:          deposits,
			VoluntaryExits:    exits,
			SyncAggregate: &eth.SyncAggregate{
				SyncCommitteeBits:      syncCommitteeBits,
				SyncCommitteeSignature: syncCommitteeSig,
			},
			ExecutionPayload: &enginev1.ExecutionPayloadDeneb{
				ParentHash:    payloadParentHash,
				FeeRecipient:  payloadFeeRecipient,
				StateRoot:     payloadStateRoot,
				ReceiptsRoot:  payloadReceiptsRoot,
				LogsBloom:     payloadLogsBloom,
				PrevRandao:    payloadPrevRandao,
				BlockNumber:   payloadBlockNumber,
				GasLimit:      payloadGasLimit,
				GasUsed:       payloadGasUsed,
				Timestamp:     payloadTimestamp,
				ExtraData:     payloadExtraData,
				BaseFeePerGas: payloadBaseFeePerGas,
				BlockHash:     payloadBlockHash,
				Transactions:  txs,
				Withdrawals:   withdrawals,
				BlobGasUsed:   payloadBlobGasUsed,
				ExcessBlobGas: payloadExcessBlobGas,
			},
			BlsToExecutionChanges: blsChanges,
			BlobKzgCommitments:    blobKzgCommitments,
			ExecutionRequests: &enginev1.ExecutionRequests{
				Deposits:       depositRequests,
				Withdrawals:    withdrawalRequests,
				Consolidations: consolidationRequests,
			},
		},
	}, nil
}

func (b *SignedBeaconBlockElectra) ToConsensus() (*eth.SignedBeaconBlockElectra, error) {
	if b == nil {
		return nil, errNilValue
	}

	sig, err := bytesutil.DecodeHexWithLength(b.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}
	block, err := b.Message.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Message")
	}
	return &eth.SignedBeaconBlockElectra{
		Block:     block,
		Signature: sig,
	}, nil
}

func (b *SignedBlindedBeaconBlockElectra) ToConsensus() (*eth.SignedBlindedBeaconBlockElectra, error) {
	if b == nil {
		return nil, errNilValue
	}

	sig, err := bytesutil.DecodeHexWithLength(b.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}
	blindedBlock, err := b.Message.ToConsensus()
	if err != nil {
		return nil, err
	}
	return &eth.SignedBlindedBeaconBlockElectra{
		Message:   blindedBlock,
		Signature: sig,
	}, nil
}

func (b *SignedBlindedBeaconBlockElectra) ToGeneric() (*eth.GenericSignedBeaconBlock, error) {
	if b == nil {
		return nil, errNilValue
	}
	sig, err := bytesutil.DecodeHexWithLength(b.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}
	blindedBlock, err := b.Message.ToConsensus()
	if err != nil {
		return nil, err
	}
	return &eth.GenericSignedBeaconBlock{Block: &eth.GenericSignedBeaconBlock_BlindedElectra{BlindedElectra: &eth.SignedBlindedBeaconBlockElectra{
		Message:   blindedBlock,
		Signature: sig,
	}}, IsBlinded: true}, nil
}

func (b *BlindedBeaconBlockElectra) ToConsensus() (*eth.BlindedBeaconBlockElectra, error) {
	if b == nil {
		return nil, errNilValue
	}
	if b.Body == nil {
		return nil, server.NewDecodeError(errNilValue, "Body")
	}
	if b.Body.Eth1Data == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.Eth1Data")
	}
	if b.Body.SyncAggregate == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.SyncAggregate")
	}
	if b.Body.ExecutionPayloadHeader == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.ExecutionPayloadHeader")
	}

	slot, err := strconv.ParseUint(b.Slot, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Slot")
	}
	proposerIndex, err := strconv.ParseUint(b.ProposerIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ProposerIndex")
	}
	parentRoot, err := bytesutil.DecodeHexWithLength(b.ParentRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ParentRoot")
	}
	stateRoot, err := bytesutil.DecodeHexWithLength(b.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "StateRoot")
	}
	randaoReveal, err := bytesutil.DecodeHexWithLength(b.Body.RandaoReveal, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.RandaoReveal")
	}
	depositRoot, err := bytesutil.DecodeHexWithLength(b.Body.Eth1Data.DepositRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.DepositRoot")
	}
	depositCount, err := strconv.ParseUint(b.Body.Eth1Data.DepositCount, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.DepositCount")
	}
	blockHash, err := bytesutil.DecodeHexWithLength(b.Body.Eth1Data.BlockHash, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.BlockHash")
	}
	graffiti, err := bytesutil.DecodeHexWithLength(b.Body.Graffiti, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Graffiti")
	}
	proposerSlashings, err := ProposerSlashingsToConsensus(b.Body.ProposerSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ProposerSlashings")
	}
	attesterSlashings, err := AttesterSlashingsElectraToConsensus(b.Body.AttesterSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.AttesterSlashings")
	}
	atts, err := AttsElectraToConsensus(b.Body.Attestations)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Attestations")
	}
	deposits, err := DepositsToConsensus(b.Body.Deposits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Deposits")
	}
	exits, err := SignedExitsToConsensus(b.Body.VoluntaryExits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.VoluntaryExits")
	}
	syncCommitteeBits, err := bytesutil.DecodeHexWithLength(b.Body.SyncAggregate.SyncCommitteeBits, fieldparams.SyncAggregateSyncCommitteeBytesLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeBits")
	}
	syncCommitteeSig, err := bytesutil.DecodeHexWithLength(b.Body.SyncAggregate.SyncCommitteeSignature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeSignature")
	}
	payloadParentHash, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.ParentHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.ParentHash")
	}
	payloadFeeRecipient, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.FeeRecipient, fieldparams.FeeRecipientLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.FeeRecipient")
	}
	payloadStateRoot, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.StateRoot")
	}
	payloadReceiptsRoot, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.ReceiptsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.ReceiptsRoot")
	}
	payloadLogsBloom, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.LogsBloom, fieldparams.LogsBloomLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.LogsBloom")
	}
	payloadPrevRandao, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.PrevRandao, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.PrevRandao")
	}
	payloadBlockNumber, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.BlockNumber, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.BlockNumber")
	}
	payloadGasLimit, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.GasLimit, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.GasLimit")
	}
	payloadGasUsed, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.GasUsed, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.GasUsed")
	}
	payloadTimestamp, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.Timestamp, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.Timestamp")
	}
	payloadExtraData, err := bytesutil.DecodeHexWithMaxLength(b.Body.ExecutionPayloadHeader.ExtraData, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.ExtraData")
	}
	payloadBaseFeePerGas, err := bytesutil.Uint256ToSSZBytes(b.Body.ExecutionPayloadHeader.BaseFeePerGas)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.BaseFeePerGas")
	}
	payloadBlockHash, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.BlockHash")
	}
	payloadTxsRoot, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.TransactionsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.TransactionsRoot")
	}
	payloadWithdrawalsRoot, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.WithdrawalsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.WithdrawalsRoot")
	}
	payloadBlobGasUsed, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.BlobGasUsed, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.BlobGasUsed")
	}
	payloadExcessBlobGas, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.ExcessBlobGas, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.ExcessBlobGas")
	}
	if b.Body.ExecutionRequests == nil {
		return nil, server.NewDecodeError(errors.New("nil execution requests"), "Body.ExecutionRequests")
	}
	depositRequests := make([]*enginev1.DepositRequest, len(b.Body.ExecutionRequests.Deposits))
	for i, d := range b.Body.ExecutionRequests.Deposits {
		depositRequests[i], err = d.ToConsensus()
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionRequests.Deposits[%d]", i))
		}
	}

	withdrawalRequests := make([]*enginev1.WithdrawalRequest, len(b.Body.ExecutionRequests.Withdrawals))
	for i, w := range b.Body.ExecutionRequests.Withdrawals {
		withdrawalRequests[i], err = w.ToConsensus()
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionRequests.Withdrawals[%d]", i))
		}
	}

	consolidationRequests := make([]*enginev1.ConsolidationRequest, len(b.Body.ExecutionRequests.Consolidations))
	for i, c := range b.Body.ExecutionRequests.Consolidations {
		consolidationRequests[i], err = c.ToConsensus()
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionRequests.Consolidations[%d]", i))
		}
	}

	blsChanges, err := SignedBLSChangesToConsensus(b.Body.BLSToExecutionChanges)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.BLSToExecutionChanges")
	}
	err = slice.VerifyMaxLength(b.Body.BlobKzgCommitments, fieldparams.MaxBlobCommitmentsPerBlock)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.BlobKzgCommitments")
	}
	blobKzgCommitments := make([][]byte, len(b.Body.BlobKzgCommitments))
	for i, b := range b.Body.BlobKzgCommitments {
		kzg, err := bytesutil.DecodeHexWithLength(b, fieldparams.BLSPubkeyLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.BlobKzgCommitments[%d]", i))
		}
		blobKzgCommitments[i] = kzg
	}

	return &eth.BlindedBeaconBlockElectra{
		Slot:          primitives.Slot(slot),
		ProposerIndex: primitives.ValidatorIndex(proposerIndex),
		ParentRoot:    parentRoot,
		StateRoot:     stateRoot,
		Body: &eth.BlindedBeaconBlockBodyElectra{
			RandaoReveal: randaoReveal,
			Eth1Data: &eth.Eth1Data{
				DepositRoot:  depositRoot,
				DepositCount: depositCount,
				BlockHash:    blockHash,
			},
			Graffiti:          graffiti,
			ProposerSlashings: proposerSlashings,
			AttesterSlashings: attesterSlashings,
			Attestations:      atts,
			Deposits:          deposits,
			VoluntaryExits:    exits,
			SyncAggregate: &eth.SyncAggregate{
				SyncCommitteeBits:      syncCommitteeBits,
				SyncCommitteeSignature: syncCommitteeSig,
			},
			ExecutionPayloadHeader: &enginev1.ExecutionPayloadHeaderDeneb{
				ParentHash:       payloadParentHash,
				FeeRecipient:     payloadFeeRecipient,
				StateRoot:        payloadStateRoot,
				ReceiptsRoot:     payloadReceiptsRoot,
				LogsBloom:        payloadLogsBloom,
				PrevRandao:       payloadPrevRandao,
				BlockNumber:      payloadBlockNumber,
				GasLimit:         payloadGasLimit,
				GasUsed:          payloadGasUsed,
				Timestamp:        payloadTimestamp,
				ExtraData:        payloadExtraData,
				BaseFeePerGas:    payloadBaseFeePerGas,
				BlockHash:        payloadBlockHash,
				TransactionsRoot: payloadTxsRoot,
				WithdrawalsRoot:  payloadWithdrawalsRoot,
				BlobGasUsed:      payloadBlobGasUsed,
				ExcessBlobGas:    payloadExcessBlobGas,
			},
			BlsToExecutionChanges: blsChanges,
			BlobKzgCommitments:    blobKzgCommitments,
			ExecutionRequests: &enginev1.ExecutionRequests{
				Deposits:       depositRequests,
				Withdrawals:    withdrawalRequests,
				Consolidations: consolidationRequests,
			},
		},
	}, nil
}

func (b *BlindedBeaconBlockElectra) ToGeneric() (*eth.GenericBeaconBlock, error) {
	if b == nil {
		return nil, errNilValue
	}

	blindedBlock, err := b.ToConsensus()
	if err != nil {
		return nil, err
	}
	return &eth.GenericBeaconBlock{Block: &eth.GenericBeaconBlock_BlindedElectra{BlindedElectra: blindedBlock}, IsBlinded: true}, nil
}

func SignedBeaconBlockContentsElectraFromConsensus(b *eth.SignedBeaconBlockContentsElectra) (*SignedBeaconBlockContentsElectra, error) {
	block, err := SignedBeaconBlockElectraFromConsensus(b.Block)
	if err != nil {
		return nil, err
	}

	proofs := make([]string, len(b.KzgProofs))
	for i, proof := range b.KzgProofs {
		proofs[i] = hexutil.Encode(proof)
	}

	blbs := make([]string, len(b.Blobs))
	for i, blob := range b.Blobs {
		blbs[i] = hexutil.Encode(blob)
	}

	return &SignedBeaconBlockContentsElectra{
		SignedBlock: block,
		KzgProofs:   proofs,
		Blobs:       blbs,
	}, nil
}

func BlindedBeaconBlockElectraFromConsensus(b *eth.BlindedBeaconBlockElectra) (*BlindedBeaconBlockElectra, error) {
	blobKzgCommitments := make([]string, len(b.Body.BlobKzgCommitments))
	for i := range b.Body.BlobKzgCommitments {
		blobKzgCommitments[i] = hexutil.Encode(b.Body.BlobKzgCommitments[i])
	}
	payload, err := ExecutionPayloadHeaderElectraFromConsensus(b.Body.ExecutionPayloadHeader)
	if err != nil {
		return nil, err
	}

	return &BlindedBeaconBlockElectra{
		Slot:          fmt.Sprintf("%d", b.Slot),
		ProposerIndex: fmt.Sprintf("%d", b.ProposerIndex),
		ParentRoot:    hexutil.Encode(b.ParentRoot),
		StateRoot:     hexutil.Encode(b.StateRoot),
		Body: &BlindedBeaconBlockBodyElectra{
			RandaoReveal:      hexutil.Encode(b.Body.RandaoReveal),
			Eth1Data:          Eth1DataFromConsensus(b.Body.Eth1Data),
			Graffiti:          hexutil.Encode(b.Body.Graffiti),
			ProposerSlashings: ProposerSlashingsFromConsensus(b.Body.ProposerSlashings),
			AttesterSlashings: AttesterSlashingsElectraFromConsensus(b.Body.AttesterSlashings),
			Attestations:      AttsElectraFromConsensus(b.Body.Attestations),
			Deposits:          DepositsFromConsensus(b.Body.Deposits),
			VoluntaryExits:    SignedExitsFromConsensus(b.Body.VoluntaryExits),
			SyncAggregate: &SyncAggregate{
				SyncCommitteeBits:      hexutil.Encode(b.Body.SyncAggregate.SyncCommitteeBits),
				SyncCommitteeSignature: hexutil.Encode(b.Body.SyncAggregate.SyncCommitteeSignature),
			},
			ExecutionPayloadHeader: payload,
			BLSToExecutionChanges:  SignedBLSChangesFromConsensus(b.Body.BlsToExecutionChanges),
			BlobKzgCommitments:     blobKzgCommitments,
			ExecutionRequests:      ExecutionRequestsFromConsensus(b.Body.ExecutionRequests),
		},
	}, nil
}

func BeaconBlockContentsElectraFromConsensus(b *eth.BeaconBlockContentsElectra) (*BeaconBlockContentsElectra, error) {
	block, err := BeaconBlockElectraFromConsensus(b.Block)
	if err != nil {
		return nil, err
	}
	proofs := make([]string, len(b.KzgProofs))
	for i, proof := range b.KzgProofs {
		proofs[i] = hexutil.Encode(proof)
	}
	blbs := make([]string, len(b.Blobs))
	for i, blob := range b.Blobs {
		blbs[i] = hexutil.Encode(blob)
	}
	return &BeaconBlockContentsElectra{
		Block:     block,
		KzgProofs: proofs,
		Blobs:     blbs,
	}, nil
}

func ExecutionRequestsFromConsensus(er *enginev1.ExecutionRequests) *ExecutionRequests {
	return &ExecutionRequests{
		Deposits:       DepositRequestsFromConsensus(er.Deposits),
		Withdrawals:    WithdrawalRequestsFromConsensus(er.Withdrawals),
		Consolidations: ConsolidationRequestsFromConsensus(er.Consolidations),
	}
}

func SignedBlindedBeaconBlockElectraFromConsensus(b *eth.SignedBlindedBeaconBlockElectra) (*SignedBlindedBeaconBlockElectra, error) {
	block, err := BlindedBeaconBlockElectraFromConsensus(b.Message)
	if err != nil {
		return nil, err
	}
	return &SignedBlindedBeaconBlockElectra{
		Message:   block,
		Signature: hexutil.Encode(b.Signature),
	}, nil
}

func BeaconBlockElectraFromConsensus(b *eth.BeaconBlockElectra) (*BeaconBlockElectra, error) {
	payload, err := ExecutionPayloadElectraFromConsensus(b.Body.ExecutionPayload)
	if err != nil {
		return nil, err
	}
	blobKzgCommitments := make([]string, len(b.Body.BlobKzgCommitments))
	for i := range b.Body.BlobKzgCommitments {
		blobKzgCommitments[i] = hexutil.Encode(b.Body.BlobKzgCommitments[i])
	}

	return &BeaconBlockElectra{
		Slot:          fmt.Sprintf("%d", b.Slot),
		ProposerIndex: fmt.Sprintf("%d", b.ProposerIndex),
		ParentRoot:    hexutil.Encode(b.ParentRoot),
		StateRoot:     hexutil.Encode(b.StateRoot),
		Body: &BeaconBlockBodyElectra{
			RandaoReveal:      hexutil.Encode(b.Body.RandaoReveal),
			Eth1Data:          Eth1DataFromConsensus(b.Body.Eth1Data),
			Graffiti:          hexutil.Encode(b.Body.Graffiti),
			ProposerSlashings: ProposerSlashingsFromConsensus(b.Body.ProposerSlashings),
			AttesterSlashings: AttesterSlashingsElectraFromConsensus(b.Body.AttesterSlashings),
			Attestations:      AttsElectraFromConsensus(b.Body.Attestations),
			Deposits:          DepositsFromConsensus(b.Body.Deposits),
			VoluntaryExits:    SignedExitsFromConsensus(b.Body.VoluntaryExits),
			SyncAggregate: &SyncAggregate{
				SyncCommitteeBits:      hexutil.Encode(b.Body.SyncAggregate.SyncCommitteeBits),
				SyncCommitteeSignature: hexutil.Encode(b.Body.SyncAggregate.SyncCommitteeSignature),
			},
			ExecutionPayload:      payload,
			BLSToExecutionChanges: SignedBLSChangesFromConsensus(b.Body.BlsToExecutionChanges),
			BlobKzgCommitments:    blobKzgCommitments,
			ExecutionRequests:     ExecutionRequestsFromConsensus(b.Body.ExecutionRequests),
		},
	}, nil
}

func SignedBeaconBlockElectraFromConsensus(b *eth.SignedBeaconBlockElectra) (*SignedBeaconBlockElectra, error) {
	block, err := BeaconBlockElectraFromConsensus(b.Block)
	if err != nil {
		return nil, err
	}
	return &SignedBeaconBlockElectra{
		Message:   block,
		Signature: hexutil.Encode(b.Signature),
	}, nil
}

var (
	ExecutionPayloadElectraFromConsensus       = ExecutionPayloadDenebFromConsensus
	ExecutionPayloadHeaderElectraFromConsensus = ExecutionPayloadHeaderDenebFromConsensus
)

// ----------------------------------------------------------------------------
// Fulu
// ----------------------------------------------------------------------------

func (b *SignedBeaconBlockContentsFulu) ToGeneric() (*eth.GenericSignedBeaconBlock, error) {
	if b == nil {
		return nil, errNilValue
	}

	signedFuluBlock, err := b.SignedBlock.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "SignedBlock")
	}
	proofs := make([][]byte, len(b.KzgProofs))
	for i, proof := range b.KzgProofs {
		proofs[i], err = bytesutil.DecodeHexWithLength(proof, fieldparams.BLSPubkeyLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("KzgProofs[%d]", i))
		}
	}
	blbs := make([][]byte, len(b.Blobs))
	for i, blob := range b.Blobs {
		blbs[i], err = bytesutil.DecodeHexWithLength(blob, fieldparams.BlobLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Blobs[%d]", i))
		}
	}
	blk := &eth.SignedBeaconBlockContentsFulu{
		Block:     signedFuluBlock,
		KzgProofs: proofs,
		Blobs:     blbs,
	}
	return &eth.GenericSignedBeaconBlock{Block: &eth.GenericSignedBeaconBlock_Fulu{Fulu: blk}}, nil
}

func (b *SignedBeaconBlockContentsFulu) ToUnsigned() *BeaconBlockContentsFulu {
	return &BeaconBlockContentsFulu{
		Block:     b.SignedBlock.Message,
		KzgProofs: b.KzgProofs,
		Blobs:     b.Blobs,
	}
}

func (b *BeaconBlockContentsFulu) ToGeneric() (*eth.GenericBeaconBlock, error) {
	block, err := b.ToConsensus()
	if err != nil {
		return nil, err
	}

	return &eth.GenericBeaconBlock{Block: &eth.GenericBeaconBlock_Fulu{Fulu: block}}, nil
}

func (b *BeaconBlockContentsFulu) ToConsensus() (*eth.BeaconBlockContentsFulu, error) {
	if b == nil {
		return nil, errNilValue
	}

	fuluBlock, err := b.Block.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Block")
	}
	proofs := make([][]byte, len(b.KzgProofs))
	for i, proof := range b.KzgProofs {
		proofs[i], err = bytesutil.DecodeHexWithLength(proof, fieldparams.BLSPubkeyLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("KzgProofs[%d]", i))
		}
	}
	blbs := make([][]byte, len(b.Blobs))
	for i, blob := range b.Blobs {
		blbs[i], err = bytesutil.DecodeHexWithLength(blob, fieldparams.BlobLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Blobs[%d]", i))
		}
	}
	return &eth.BeaconBlockContentsFulu{
		Block:     fuluBlock,
		KzgProofs: proofs,
		Blobs:     blbs,
	}, nil
}

func (b *BeaconBlockFulu) ToConsensus() (*eth.BeaconBlockFulu, error) {
	if b == nil {
		return nil, errNilValue
	}
	if b.Body == nil {
		return nil, server.NewDecodeError(errNilValue, "Body")
	}
	if b.Body.Eth1Data == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.Eth1Data")
	}
	if b.Body.SyncAggregate == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.SyncAggregate")
	}
	if b.Body.ExecutionPayload == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.ExecutionPayload")
	}

	slot, err := strconv.ParseUint(b.Slot, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Slot")
	}
	proposerIndex, err := strconv.ParseUint(b.ProposerIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ProposerIndex")
	}
	parentRoot, err := bytesutil.DecodeHexWithLength(b.ParentRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ParentRoot")
	}
	stateRoot, err := bytesutil.DecodeHexWithLength(b.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "StateRoot")
	}
	randaoReveal, err := bytesutil.DecodeHexWithLength(b.Body.RandaoReveal, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.RandaoReveal")
	}
	depositRoot, err := bytesutil.DecodeHexWithLength(b.Body.Eth1Data.DepositRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.DepositRoot")
	}
	depositCount, err := strconv.ParseUint(b.Body.Eth1Data.DepositCount, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.DepositCount")
	}
	blockHash, err := bytesutil.DecodeHexWithLength(b.Body.Eth1Data.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.BlockHash")
	}
	graffiti, err := bytesutil.DecodeHexWithLength(b.Body.Graffiti, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Graffiti")
	}
	proposerSlashings, err := ProposerSlashingsToConsensus(b.Body.ProposerSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ProposerSlashings")
	}
	attesterSlashings, err := AttesterSlashingsElectraToConsensus(b.Body.AttesterSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.AttesterSlashings")
	}
	atts, err := AttsElectraToConsensus(b.Body.Attestations)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Attestations")
	}
	deposits, err := DepositsToConsensus(b.Body.Deposits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Deposits")
	}
	exits, err := SignedExitsToConsensus(b.Body.VoluntaryExits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.VoluntaryExits")
	}
	syncCommitteeBits, err := bytesutil.DecodeHexWithLength(b.Body.SyncAggregate.SyncCommitteeBits, fieldparams.SyncAggregateSyncCommitteeBytesLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeBits")
	}
	syncCommitteeSig, err := bytesutil.DecodeHexWithLength(b.Body.SyncAggregate.SyncCommitteeSignature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeSignature")
	}
	payloadParentHash, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.ParentHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.ParentHash")
	}
	payloadFeeRecipient, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.FeeRecipient, fieldparams.FeeRecipientLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.FeeRecipient")
	}
	payloadStateRoot, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.StateRoot")
	}
	payloadReceiptsRoot, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.ReceiptsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.ReceiptsRoot")
	}
	payloadLogsBloom, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.LogsBloom, fieldparams.LogsBloomLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.LogsBloom")
	}
	payloadPrevRandao, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.PrevRandao, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.PrevRandao")
	}
	payloadBlockNumber, err := strconv.ParseUint(b.Body.ExecutionPayload.BlockNumber, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.BlockNumber")
	}
	payloadGasLimit, err := strconv.ParseUint(b.Body.ExecutionPayload.GasLimit, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.GasLimit")
	}
	payloadGasUsed, err := strconv.ParseUint(b.Body.ExecutionPayload.GasUsed, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.GasUsed")
	}
	payloadTimestamp, err := strconv.ParseUint(b.Body.ExecutionPayload.Timestamp, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.Timestamp")
	}
	payloadExtraData, err := bytesutil.DecodeHexWithMaxLength(b.Body.ExecutionPayload.ExtraData, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.ExtraData")
	}
	payloadBaseFeePerGas, err := bytesutil.Uint256ToSSZBytes(b.Body.ExecutionPayload.BaseFeePerGas)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.BaseFeePerGas")
	}
	payloadBlockHash, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayload.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.BlockHash")
	}
	err = slice.VerifyMaxLength(b.Body.ExecutionPayload.Transactions, fieldparams.MaxTxsPerPayloadLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.Transactions")
	}
	txs := make([][]byte, len(b.Body.ExecutionPayload.Transactions))
	for i, tx := range b.Body.ExecutionPayload.Transactions {
		txs[i], err = bytesutil.DecodeHexWithMaxLength(tx, fieldparams.MaxBytesPerTxLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Transactions[%d]", i))
		}
	}
	err = slice.VerifyMaxLength(b.Body.ExecutionPayload.Withdrawals, fieldparams.MaxWithdrawalsPerPayload)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.Withdrawals")
	}
	withdrawals := make([]*enginev1.Withdrawal, len(b.Body.ExecutionPayload.Withdrawals))
	for i, w := range b.Body.ExecutionPayload.Withdrawals {
		withdrawalIndex, err := strconv.ParseUint(w.WithdrawalIndex, 10, 64)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Withdrawals[%d].WithdrawalIndex", i))
		}
		validatorIndex, err := strconv.ParseUint(w.ValidatorIndex, 10, 64)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Withdrawals[%d].ValidatorIndex", i))
		}
		address, err := bytesutil.DecodeHexWithLength(w.ExecutionAddress, common.AddressLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Withdrawals[%d].ExecutionAddress", i))
		}
		amount, err := strconv.ParseUint(w.Amount, 10, 64)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Withdrawals[%d].Amount", i))
		}
		withdrawals[i] = &enginev1.Withdrawal{
			Index:          withdrawalIndex,
			ValidatorIndex: primitives.ValidatorIndex(validatorIndex),
			Address:        address,
			Amount:         amount,
		}
	}

	payloadBlobGasUsed, err := strconv.ParseUint(b.Body.ExecutionPayload.BlobGasUsed, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.BlobGasUsed")
	}
	payloadExcessBlobGas, err := strconv.ParseUint(b.Body.ExecutionPayload.ExcessBlobGas, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.ExcessBlobGas")
	}

	if b.Body.ExecutionRequests == nil {
		return nil, server.NewDecodeError(errors.New("nil execution requests"), "Body.ExequtionRequests")
	}

	depositRequests := make([]*enginev1.DepositRequest, len(b.Body.ExecutionRequests.Deposits))
	for i, d := range b.Body.ExecutionRequests.Deposits {
		depositRequests[i], err = d.ToConsensus()
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionRequests.Deposits[%d]", i))
		}
	}

	withdrawalRequests := make([]*enginev1.WithdrawalRequest, len(b.Body.ExecutionRequests.Withdrawals))
	for i, w := range b.Body.ExecutionRequests.Withdrawals {
		withdrawalRequests[i], err = w.ToConsensus()
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionRequests.Withdrawals[%d]", i))
		}
	}

	consolidationRequests := make([]*enginev1.ConsolidationRequest, len(b.Body.ExecutionRequests.Consolidations))
	for i, c := range b.Body.ExecutionRequests.Consolidations {
		consolidationRequests[i], err = c.ToConsensus()
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionRequests.Consolidations[%d]", i))
		}
	}

	blsChanges, err := SignedBLSChangesToConsensus(b.Body.BLSToExecutionChanges)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.BLSToExecutionChanges")
	}
	err = slice.VerifyMaxLength(b.Body.BlobKzgCommitments, fieldparams.MaxBlobCommitmentsPerBlock)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.BlobKzgCommitments")
	}
	blobKzgCommitments := make([][]byte, len(b.Body.BlobKzgCommitments))
	for i, b := range b.Body.BlobKzgCommitments {
		kzg, err := bytesutil.DecodeHexWithLength(b, fieldparams.BLSPubkeyLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.BlobKzgCommitments[%d]", i))
		}
		blobKzgCommitments[i] = kzg
	}
	return &eth.BeaconBlockFulu{
		Slot:          primitives.Slot(slot),
		ProposerIndex: primitives.ValidatorIndex(proposerIndex),
		ParentRoot:    parentRoot,
		StateRoot:     stateRoot,
		Body: &eth.BeaconBlockBodyFulu{
			RandaoReveal: randaoReveal,
			Eth1Data: &eth.Eth1Data{
				DepositRoot:  depositRoot,
				DepositCount: depositCount,
				BlockHash:    blockHash,
			},
			Graffiti:          graffiti,
			ProposerSlashings: proposerSlashings,
			AttesterSlashings: attesterSlashings,
			Attestations:      atts,
			Deposits:          deposits,
			VoluntaryExits:    exits,
			SyncAggregate: &eth.SyncAggregate{
				SyncCommitteeBits:      syncCommitteeBits,
				SyncCommitteeSignature: syncCommitteeSig,
			},
			ExecutionPayload: &enginev1.ExecutionPayloadDeneb{
				ParentHash:    payloadParentHash,
				FeeRecipient:  payloadFeeRecipient,
				StateRoot:     payloadStateRoot,
				ReceiptsRoot:  payloadReceiptsRoot,
				LogsBloom:     payloadLogsBloom,
				PrevRandao:    payloadPrevRandao,
				BlockNumber:   payloadBlockNumber,
				GasLimit:      payloadGasLimit,
				GasUsed:       payloadGasUsed,
				Timestamp:     payloadTimestamp,
				ExtraData:     payloadExtraData,
				BaseFeePerGas: payloadBaseFeePerGas,
				BlockHash:     payloadBlockHash,
				Transactions:  txs,
				Withdrawals:   withdrawals,
				BlobGasUsed:   payloadBlobGasUsed,
				ExcessBlobGas: payloadExcessBlobGas,
			},
			BlsToExecutionChanges: blsChanges,
			BlobKzgCommitments:    blobKzgCommitments,
			ExecutionRequests: &enginev1.ExecutionRequests{
				Deposits:       depositRequests,
				Withdrawals:    withdrawalRequests,
				Consolidations: consolidationRequests,
			},
		},
	}, nil
}

func (b *SignedBeaconBlockFulu) ToConsensus() (*eth.SignedBeaconBlockFulu, error) {
	if b == nil {
		return nil, errNilValue
	}

	sig, err := bytesutil.DecodeHexWithLength(b.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}
	block, err := b.Message.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Message")
	}
	return &eth.SignedBeaconBlockFulu{
		Block:     block,
		Signature: sig,
	}, nil
}

func (b *SignedBlindedBeaconBlockFulu) ToConsensus() (*eth.SignedBlindedBeaconBlockFulu, error) {
	if b == nil {
		return nil, errNilValue
	}

	sig, err := bytesutil.DecodeHexWithLength(b.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}
	blindedBlock, err := b.Message.ToConsensus()
	if err != nil {
		return nil, err
	}
	return &eth.SignedBlindedBeaconBlockFulu{
		Message:   blindedBlock,
		Signature: sig,
	}, nil
}

func (b *SignedBlindedBeaconBlockFulu) ToGeneric() (*eth.GenericSignedBeaconBlock, error) {
	if b == nil {
		return nil, errNilValue
	}
	sig, err := bytesutil.DecodeHexWithLength(b.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}
	blindedBlock, err := b.Message.ToConsensus()
	if err != nil {
		return nil, err
	}
	return &eth.GenericSignedBeaconBlock{Block: &eth.GenericSignedBeaconBlock_BlindedFulu{BlindedFulu: &eth.SignedBlindedBeaconBlockFulu{
		Message:   blindedBlock,
		Signature: sig,
	}}, IsBlinded: true}, nil
}

func (b *BlindedBeaconBlockFulu) ToConsensus() (*eth.BlindedBeaconBlockFulu, error) {
	if b == nil {
		return nil, errNilValue
	}
	if b.Body == nil {
		return nil, server.NewDecodeError(errNilValue, "Body")
	}
	if b.Body.Eth1Data == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.Eth1Data")
	}
	if b.Body.SyncAggregate == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.SyncAggregate")
	}
	if b.Body.ExecutionPayloadHeader == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.ExecutionPayloadHeader")
	}

	slot, err := strconv.ParseUint(b.Slot, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Slot")
	}
	proposerIndex, err := strconv.ParseUint(b.ProposerIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ProposerIndex")
	}
	parentRoot, err := bytesutil.DecodeHexWithLength(b.ParentRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ParentRoot")
	}
	stateRoot, err := bytesutil.DecodeHexWithLength(b.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "StateRoot")
	}
	randaoReveal, err := bytesutil.DecodeHexWithLength(b.Body.RandaoReveal, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.RandaoReveal")
	}
	depositRoot, err := bytesutil.DecodeHexWithLength(b.Body.Eth1Data.DepositRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.DepositRoot")
	}
	depositCount, err := strconv.ParseUint(b.Body.Eth1Data.DepositCount, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.DepositCount")
	}
	blockHash, err := bytesutil.DecodeHexWithLength(b.Body.Eth1Data.BlockHash, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.BlockHash")
	}
	graffiti, err := bytesutil.DecodeHexWithLength(b.Body.Graffiti, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Graffiti")
	}
	proposerSlashings, err := ProposerSlashingsToConsensus(b.Body.ProposerSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ProposerSlashings")
	}
	attesterSlashings, err := AttesterSlashingsElectraToConsensus(b.Body.AttesterSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.AttesterSlashings")
	}
	atts, err := AttsElectraToConsensus(b.Body.Attestations)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Attestations")
	}
	deposits, err := DepositsToConsensus(b.Body.Deposits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Deposits")
	}
	exits, err := SignedExitsToConsensus(b.Body.VoluntaryExits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.VoluntaryExits")
	}
	syncCommitteeBits, err := bytesutil.DecodeHexWithLength(b.Body.SyncAggregate.SyncCommitteeBits, fieldparams.SyncAggregateSyncCommitteeBytesLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeBits")
	}
	syncCommitteeSig, err := bytesutil.DecodeHexWithLength(b.Body.SyncAggregate.SyncCommitteeSignature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeSignature")
	}
	payloadParentHash, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.ParentHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.ParentHash")
	}
	payloadFeeRecipient, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.FeeRecipient, fieldparams.FeeRecipientLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.FeeRecipient")
	}
	payloadStateRoot, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.StateRoot")
	}
	payloadReceiptsRoot, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.ReceiptsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.ReceiptsRoot")
	}
	payloadLogsBloom, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.LogsBloom, fieldparams.LogsBloomLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.LogsBloom")
	}
	payloadPrevRandao, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.PrevRandao, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.PrevRandao")
	}
	payloadBlockNumber, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.BlockNumber, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.BlockNumber")
	}
	payloadGasLimit, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.GasLimit, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.GasLimit")
	}
	payloadGasUsed, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.GasUsed, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.GasUsed")
	}
	payloadTimestamp, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.Timestamp, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.Timestamp")
	}
	payloadExtraData, err := bytesutil.DecodeHexWithMaxLength(b.Body.ExecutionPayloadHeader.ExtraData, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.ExtraData")
	}
	payloadBaseFeePerGas, err := bytesutil.Uint256ToSSZBytes(b.Body.ExecutionPayloadHeader.BaseFeePerGas)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.BaseFeePerGas")
	}
	payloadBlockHash, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.BlockHash")
	}
	payloadTxsRoot, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.TransactionsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.TransactionsRoot")
	}
	payloadWithdrawalsRoot, err := bytesutil.DecodeHexWithLength(b.Body.ExecutionPayloadHeader.WithdrawalsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayloadHeader.WithdrawalsRoot")
	}
	payloadBlobGasUsed, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.BlobGasUsed, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.BlobGasUsed")
	}
	payloadExcessBlobGas, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.ExcessBlobGas, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ExecutionPayload.ExcessBlobGas")
	}
	if b.Body.ExecutionRequests == nil {
		return nil, server.NewDecodeError(errors.New("nil execution requests"), "Body.ExecutionRequests")
	}
	depositRequests := make([]*enginev1.DepositRequest, len(b.Body.ExecutionRequests.Deposits))
	for i, d := range b.Body.ExecutionRequests.Deposits {
		depositRequests[i], err = d.ToConsensus()
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionRequests.Deposits[%d]", i))
		}
	}

	withdrawalRequests := make([]*enginev1.WithdrawalRequest, len(b.Body.ExecutionRequests.Withdrawals))
	for i, w := range b.Body.ExecutionRequests.Withdrawals {
		withdrawalRequests[i], err = w.ToConsensus()
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionRequests.Withdrawals[%d]", i))
		}
	}

	consolidationRequests := make([]*enginev1.ConsolidationRequest, len(b.Body.ExecutionRequests.Consolidations))
	for i, c := range b.Body.ExecutionRequests.Consolidations {
		consolidationRequests[i], err = c.ToConsensus()
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.ExecutionRequests.Consolidations[%d]", i))
		}
	}

	blsChanges, err := SignedBLSChangesToConsensus(b.Body.BLSToExecutionChanges)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.BLSToExecutionChanges")
	}
	err = slice.VerifyMaxLength(b.Body.BlobKzgCommitments, fieldparams.MaxBlobCommitmentsPerBlock)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.BlobKzgCommitments")
	}
	blobKzgCommitments := make([][]byte, len(b.Body.BlobKzgCommitments))
	for i, b := range b.Body.BlobKzgCommitments {
		kzg, err := bytesutil.DecodeHexWithLength(b, fieldparams.BLSPubkeyLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.BlobKzgCommitments[%d]", i))
		}
		blobKzgCommitments[i] = kzg
	}

	return &eth.BlindedBeaconBlockFulu{
		Slot:          primitives.Slot(slot),
		ProposerIndex: primitives.ValidatorIndex(proposerIndex),
		ParentRoot:    parentRoot,
		StateRoot:     stateRoot,
		Body: &eth.BlindedBeaconBlockBodyFulu{
			RandaoReveal: randaoReveal,
			Eth1Data: &eth.Eth1Data{
				DepositRoot:  depositRoot,
				DepositCount: depositCount,
				BlockHash:    blockHash,
			},
			Graffiti:          graffiti,
			ProposerSlashings: proposerSlashings,
			AttesterSlashings: attesterSlashings,
			Attestations:      atts,
			Deposits:          deposits,
			VoluntaryExits:    exits,
			SyncAggregate: &eth.SyncAggregate{
				SyncCommitteeBits:      syncCommitteeBits,
				SyncCommitteeSignature: syncCommitteeSig,
			},
			ExecutionPayloadHeader: &enginev1.ExecutionPayloadHeaderDeneb{
				ParentHash:       payloadParentHash,
				FeeRecipient:     payloadFeeRecipient,
				StateRoot:        payloadStateRoot,
				ReceiptsRoot:     payloadReceiptsRoot,
				LogsBloom:        payloadLogsBloom,
				PrevRandao:       payloadPrevRandao,
				BlockNumber:      payloadBlockNumber,
				GasLimit:         payloadGasLimit,
				GasUsed:          payloadGasUsed,
				Timestamp:        payloadTimestamp,
				ExtraData:        payloadExtraData,
				BaseFeePerGas:    payloadBaseFeePerGas,
				BlockHash:        payloadBlockHash,
				TransactionsRoot: payloadTxsRoot,
				WithdrawalsRoot:  payloadWithdrawalsRoot,
				BlobGasUsed:      payloadBlobGasUsed,
				ExcessBlobGas:    payloadExcessBlobGas,
			},
			BlsToExecutionChanges: blsChanges,
			BlobKzgCommitments:    blobKzgCommitments,
			ExecutionRequests: &enginev1.ExecutionRequests{
				Deposits:       depositRequests,
				Withdrawals:    withdrawalRequests,
				Consolidations: consolidationRequests,
			},
		},
	}, nil
}

func (b *BlindedBeaconBlockFulu) ToGeneric() (*eth.GenericBeaconBlock, error) {
	if b == nil {
		return nil, errNilValue
	}

	blindedBlock, err := b.ToConsensus()
	if err != nil {
		return nil, err
	}
	return &eth.GenericBeaconBlock{Block: &eth.GenericBeaconBlock_BlindedFulu{BlindedFulu: blindedBlock}, IsBlinded: true}, nil
}

func BeaconBlockContentsFuluFromConsensus(b *eth.BeaconBlockContentsFulu) (*BeaconBlockContentsFulu, error) {
	block, err := BeaconBlockFuluFromConsensus(b.Block)
	if err != nil {
		return nil, err
	}
	proofs := make([]string, len(b.KzgProofs))
	for i, proof := range b.KzgProofs {
		proofs[i] = hexutil.Encode(proof)
	}
	blbs := make([]string, len(b.Blobs))
	for i, blob := range b.Blobs {
		blbs[i] = hexutil.Encode(blob)
	}
	return &BeaconBlockContentsFulu{
		Block:     block,
		KzgProofs: proofs,
		Blobs:     blbs,
	}, nil
}

func SignedBeaconBlockContentsFuluFromConsensus(b *eth.SignedBeaconBlockContentsFulu) (*SignedBeaconBlockContentsFulu, error) {
	block, err := SignedBeaconBlockFuluFromConsensus(b.Block)
	if err != nil {
		return nil, err
	}

	proofs := make([]string, len(b.KzgProofs))
	for i, proof := range b.KzgProofs {
		proofs[i] = hexutil.Encode(proof)
	}

	blbs := make([]string, len(b.Blobs))
	for i, blob := range b.Blobs {
		blbs[i] = hexutil.Encode(blob)
	}

	return &SignedBeaconBlockContentsFulu{
		SignedBlock: block,
		KzgProofs:   proofs,
		Blobs:       blbs,
	}, nil
}

func BlindedBeaconBlockFuluFromConsensus(b *eth.BlindedBeaconBlockFulu) (*BlindedBeaconBlockFulu, error) {
	blobKzgCommitments := make([]string, len(b.Body.BlobKzgCommitments))
	for i := range b.Body.BlobKzgCommitments {
		blobKzgCommitments[i] = hexutil.Encode(b.Body.BlobKzgCommitments[i])
	}
	payload, err := ExecutionPayloadHeaderFuluFromConsensus(b.Body.ExecutionPayloadHeader)
	if err != nil {
		return nil, err
	}

	return &BlindedBeaconBlockFulu{
		Slot:          fmt.Sprintf("%d", b.Slot),
		ProposerIndex: fmt.Sprintf("%d", b.ProposerIndex),
		ParentRoot:    hexutil.Encode(b.ParentRoot),
		StateRoot:     hexutil.Encode(b.StateRoot),
		Body: &BlindedBeaconBlockBodyFulu{
			RandaoReveal:      hexutil.Encode(b.Body.RandaoReveal),
			Eth1Data:          Eth1DataFromConsensus(b.Body.Eth1Data),
			Graffiti:          hexutil.Encode(b.Body.Graffiti),
			ProposerSlashings: ProposerSlashingsFromConsensus(b.Body.ProposerSlashings),
			AttesterSlashings: AttesterSlashingsElectraFromConsensus(b.Body.AttesterSlashings),
			Attestations:      AttsElectraFromConsensus(b.Body.Attestations),
			Deposits:          DepositsFromConsensus(b.Body.Deposits),
			VoluntaryExits:    SignedExitsFromConsensus(b.Body.VoluntaryExits),
			SyncAggregate: &SyncAggregate{
				SyncCommitteeBits:      hexutil.Encode(b.Body.SyncAggregate.SyncCommitteeBits),
				SyncCommitteeSignature: hexutil.Encode(b.Body.SyncAggregate.SyncCommitteeSignature),
			},
			ExecutionPayloadHeader: payload,
			BLSToExecutionChanges:  SignedBLSChangesFromConsensus(b.Body.BlsToExecutionChanges),
			BlobKzgCommitments:     blobKzgCommitments,
			ExecutionRequests:      ExecutionRequestsFromConsensus(b.Body.ExecutionRequests),
		},
	}, nil
}

func SignedBlindedBeaconBlockFuluFromConsensus(b *eth.SignedBlindedBeaconBlockFulu) (*SignedBlindedBeaconBlockFulu, error) {
	block, err := BlindedBeaconBlockFuluFromConsensus(b.Message)
	if err != nil {
		return nil, err
	}
	return &SignedBlindedBeaconBlockFulu{
		Message:   block,
		Signature: hexutil.Encode(b.Signature),
	}, nil
}

func BeaconBlockFuluFromConsensus(b *eth.BeaconBlockFulu) (*BeaconBlockFulu, error) {
	payload, err := ExecutionPayloadFuluFromConsensus(b.Body.ExecutionPayload)
	if err != nil {
		return nil, err
	}
	blobKzgCommitments := make([]string, len(b.Body.BlobKzgCommitments))
	for i := range b.Body.BlobKzgCommitments {
		blobKzgCommitments[i] = hexutil.Encode(b.Body.BlobKzgCommitments[i])
	}

	return &BeaconBlockFulu{
		Slot:          fmt.Sprintf("%d", b.Slot),
		ProposerIndex: fmt.Sprintf("%d", b.ProposerIndex),
		ParentRoot:    hexutil.Encode(b.ParentRoot),
		StateRoot:     hexutil.Encode(b.StateRoot),
		Body: &BeaconBlockBodyFulu{
			RandaoReveal:      hexutil.Encode(b.Body.RandaoReveal),
			Eth1Data:          Eth1DataFromConsensus(b.Body.Eth1Data),
			Graffiti:          hexutil.Encode(b.Body.Graffiti),
			ProposerSlashings: ProposerSlashingsFromConsensus(b.Body.ProposerSlashings),
			AttesterSlashings: AttesterSlashingsElectraFromConsensus(b.Body.AttesterSlashings),
			Attestations:      AttsElectraFromConsensus(b.Body.Attestations),
			Deposits:          DepositsFromConsensus(b.Body.Deposits),
			VoluntaryExits:    SignedExitsFromConsensus(b.Body.VoluntaryExits),
			SyncAggregate: &SyncAggregate{
				SyncCommitteeBits:      hexutil.Encode(b.Body.SyncAggregate.SyncCommitteeBits),
				SyncCommitteeSignature: hexutil.Encode(b.Body.SyncAggregate.SyncCommitteeSignature),
			},
			ExecutionPayload:      payload,
			BLSToExecutionChanges: SignedBLSChangesFromConsensus(b.Body.BlsToExecutionChanges),
			BlobKzgCommitments:    blobKzgCommitments,
			ExecutionRequests:     ExecutionRequestsFromConsensus(b.Body.ExecutionRequests),
		},
	}, nil
}

func SignedBeaconBlockFuluFromConsensus(b *eth.SignedBeaconBlockFulu) (*SignedBeaconBlockFulu, error) {
	block, err := BeaconBlockFuluFromConsensus(b.Block)
	if err != nil {
		return nil, err
	}
	return &SignedBeaconBlockFulu{
		Message:   block,
		Signature: hexutil.Encode(b.Signature),
	}, nil
}

var (
	ExecutionPayloadFuluFromConsensus       = ExecutionPayloadDenebFromConsensus
	ExecutionPayloadHeaderFuluFromConsensus = ExecutionPayloadHeaderDenebFromConsensus
)
