package structs

import (
	"fmt"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api/server"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/container/slice"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

// ----------------------------------------------------------------------------
// Heze
// ----------------------------------------------------------------------------

func SignedBeaconBlockHezeFromConsensus(b *eth.SignedBeaconBlockHeze) (*SignedBeaconBlockHeze, error) {
	block, err := BeaconBlockHezeFromConsensus(b.Block)
	if err != nil {
		return nil, fmt.Errorf("convert Heze block: %w", err)
	}
	return &SignedBeaconBlockHeze{
		Message:   block,
		Signature: hexutil.Encode(b.Signature),
	}, nil
}

func BeaconBlockHezeFromConsensus(b *eth.BeaconBlockHeze) (*BeaconBlockHeze, error) {
	payloadAttestations := make([]*PayloadAttestation, len(b.Body.PayloadAttestations))
	for i, pa := range b.Body.PayloadAttestations {
		payloadAttestations[i] = PayloadAttestationFromConsensus(pa)
	}

	return &BeaconBlockHeze{
		Slot:          fmt.Sprintf("%d", b.Slot),
		ProposerIndex: fmt.Sprintf("%d", b.ProposerIndex),
		ParentRoot:    hexutil.Encode(b.ParentRoot),
		StateRoot:     hexutil.Encode(b.StateRoot),
		Body: &BeaconBlockBodyHeze{
			RandaoReveal:              hexutil.Encode(b.Body.RandaoReveal),
			Eth1Data:                  Eth1DataFromConsensus(b.Body.Eth1Data),
			Graffiti:                  hexutil.Encode(b.Body.Graffiti),
			ProposerSlashings:         ProposerSlashingsFromConsensus(b.Body.ProposerSlashings),
			AttesterSlashings:         AttesterSlashingsGloasFromConsensus(b.Body.AttesterSlashings),
			Attestations:              AttsGloasFromConsensus(b.Body.Attestations),
			Deposits:                  DepositsFromConsensus(b.Body.Deposits),
			VoluntaryExits:            SignedExitsFromConsensus(b.Body.VoluntaryExits),
			SyncAggregate:             SyncAggregateFromConsensus(b.Body.SyncAggregate),
			BLSToExecutionChanges:     SignedBLSChangesFromConsensus(b.Body.BlsToExecutionChanges),
			SignedExecutionPayloadBid: SignedExecutionPayloadBidHezeFromConsensus(b.Body.SignedExecutionPayloadBid),
			PayloadAttestations:       payloadAttestations,
			ParentExecutionRequests:   ExecutionRequestsGloasFromConsensus(b.Body.ParentExecutionRequests),
		},
	}, nil
}

func SignedExecutionPayloadBidHezeFromConsensus(b *eth.SignedExecutionPayloadBidHeze) *SignedExecutionPayloadBidHeze {
	return &SignedExecutionPayloadBidHeze{
		Message:   ExecutionPayloadBidHezeFromConsensus(b.Message),
		Signature: hexutil.Encode(b.Signature),
	}
}

func ExecutionPayloadBidHezeFromConsensus(b *eth.ExecutionPayloadBidHeze) *ExecutionPayloadBidHeze {
	blobKzgCommitments := make([]string, len(b.BlobKzgCommitments))
	for i := range b.BlobKzgCommitments {
		blobKzgCommitments[i] = hexutil.Encode(b.BlobKzgCommitments[i])
	}
	return &ExecutionPayloadBidHeze{
		ParentBlockHash:       hexutil.Encode(b.ParentBlockHash),
		ParentBlockRoot:       hexutil.Encode(b.ParentBlockRoot),
		BlockHash:             hexutil.Encode(b.BlockHash),
		PrevRandao:            hexutil.Encode(b.PrevRandao),
		FeeRecipient:          hexutil.Encode(b.FeeRecipient),
		GasLimit:              fmt.Sprintf("%d", b.GasLimit),
		BuilderIndex:          fmt.Sprintf("%d", b.BuilderIndex),
		Slot:                  fmt.Sprintf("%d", b.Slot),
		Value:                 fmt.Sprintf("%d", b.Value),
		ExecutionPayment:      fmt.Sprintf("%d", b.ExecutionPayment),
		BlobKzgCommitments:    blobKzgCommitments,
		ExecutionRequestsRoot: hexutil.Encode(b.ExecutionRequestsRoot),
		InclusionListBits:     hexutil.Encode(b.InclusionListBits),
	}
}

func ROExecutionPayloadBidHezeFromConsensus(b interfaces.ROExecutionPayloadBid) *ExecutionPayloadBidHeze {
	if b == nil {
		return nil
	}

	pbh := b.ParentBlockHash()
	pbr := b.ParentBlockRoot()
	bh := b.BlockHash()
	pr := b.PrevRandao()
	fr := b.FeeRecipient()
	commitments := b.BlobKzgCommitments()
	blobKzgCommitments := make([]string, 0, len(commitments))
	for _, commitment := range commitments {
		blobKzgCommitments = append(blobKzgCommitments, hexutil.Encode(commitment))
	}
	erRoot := b.ExecutionRequestsRoot()
	return &ExecutionPayloadBidHeze{
		ParentBlockHash:       hexutil.Encode(pbh[:]),
		ParentBlockRoot:       hexutil.Encode(pbr[:]),
		BlockHash:             hexutil.Encode(bh[:]),
		PrevRandao:            hexutil.Encode(pr[:]),
		FeeRecipient:          hexutil.Encode(fr[:]),
		GasLimit:              fmt.Sprintf("%d", b.GasLimit()),
		BuilderIndex:          fmt.Sprintf("%d", b.BuilderIndex()),
		Slot:                  fmt.Sprintf("%d", b.Slot()),
		Value:                 fmt.Sprintf("%d", b.Value()),
		ExecutionPayment:      fmt.Sprintf("%d", b.ExecutionPayment()),
		BlobKzgCommitments:    blobKzgCommitments,
		ExecutionRequestsRoot: hexutil.Encode(erRoot[:]),
		InclusionListBits:     hexutil.Encode(b.InclusionListBits()),
	}
}

func (b *SignedBeaconBlockHeze) ToGeneric() (*eth.GenericSignedBeaconBlock, error) {
	if b == nil {
		return nil, errNilValue
	}
	signed, err := b.ToConsensus()
	if err != nil {
		return nil, fmt.Errorf("convert block to consensus: %w", err)
	}
	return &eth.GenericSignedBeaconBlock{
		Block: &eth.GenericSignedBeaconBlock_Heze{Heze: signed},
	}, nil
}

func (b *SignedBeaconBlockHeze) ToConsensus() (*eth.SignedBeaconBlockHeze, error) {
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
	return &eth.SignedBeaconBlockHeze{
		Block:     block,
		Signature: sig,
	}, nil
}

func (b *BeaconBlockHeze) ToConsensus() (*eth.BeaconBlockHeze, error) {
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
	if b.Body.SignedExecutionPayloadBid == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.SignedExecutionPayloadBid")
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
	body, err := b.Body.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Body")
	}
	return &eth.BeaconBlockHeze{
		Slot:          primitives.Slot(slot),
		ProposerIndex: primitives.ValidatorIndex(proposerIndex),
		ParentRoot:    parentRoot,
		StateRoot:     stateRoot,
		Body:          body,
	}, nil
}

func (b *BeaconBlockBodyHeze) ToConsensus() (*eth.BeaconBlockBodyHeze, error) {
	if b == nil {
		return nil, errNilValue
	}
	if b.Eth1Data == nil {
		return nil, server.NewDecodeError(errNilValue, "Eth1Data")
	}
	if b.SyncAggregate == nil {
		return nil, server.NewDecodeError(errNilValue, "SyncAggregate")
	}
	if b.SignedExecutionPayloadBid == nil {
		return nil, server.NewDecodeError(errNilValue, "SignedExecutionPayloadBid")
	}

	randaoReveal, err := bytesutil.DecodeHexWithLength(b.RandaoReveal, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "RandaoReveal")
	}
	depositRoot, err := bytesutil.DecodeHexWithLength(b.Eth1Data.DepositRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Eth1Data.DepositRoot")
	}
	depositCount, err := strconv.ParseUint(b.Eth1Data.DepositCount, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Eth1Data.DepositCount")
	}
	blockHash, err := bytesutil.DecodeHexWithLength(b.Eth1Data.BlockHash, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Eth1Data.BlockHash")
	}
	graffiti, err := bytesutil.DecodeHexWithLength(b.Graffiti, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Graffiti")
	}
	proposerSlashings, err := ProposerSlashingsToConsensus(b.ProposerSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "ProposerSlashings")
	}
	attesterSlashings, err := AttesterSlashingsGloasToConsensus(b.AttesterSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "AttesterSlashings")
	}
	atts, err := AttsGloasToConsensus(b.Attestations)
	if err != nil {
		return nil, server.NewDecodeError(err, "Attestations")
	}
	deposits, err := DepositsToConsensus(b.Deposits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Deposits")
	}
	exits, err := SignedExitsToConsensus(b.VoluntaryExits)
	if err != nil {
		return nil, server.NewDecodeError(err, "VoluntaryExits")
	}
	syncCommitteeBits, err := bytesutil.DecodeHexWithLength(b.SyncAggregate.SyncCommitteeBits, fieldparams.SyncAggregateSyncCommitteeBytesLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "SyncAggregate.SyncCommitteeBits")
	}
	syncCommitteeSig, err := bytesutil.DecodeHexWithLength(b.SyncAggregate.SyncCommitteeSignature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "SyncAggregate.SyncCommitteeSignature")
	}
	blsChanges, err := SignedBLSChangesToConsensus(b.BLSToExecutionChanges)
	if err != nil {
		return nil, server.NewDecodeError(err, "BLSToExecutionChanges")
	}
	signedBid, err := b.SignedExecutionPayloadBid.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "SignedExecutionPayloadBid")
	}
	payloadAttestations, err := PayloadAttestationsToConsensus(b.PayloadAttestations)
	if err != nil {
		return nil, server.NewDecodeError(err, "PayloadAttestations")
	}
	var parentExecutionRequests *enginev1.ExecutionRequestsGloas
	if b.ParentExecutionRequests != nil {
		parentExecutionRequests, err = b.ParentExecutionRequests.ToConsensus()
		if err != nil {
			return nil, server.NewDecodeError(err, "ParentExecutionRequests")
		}
	}

	return &eth.BeaconBlockBodyHeze{
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
		BlsToExecutionChanges:     blsChanges,
		SignedExecutionPayloadBid: signedBid,
		PayloadAttestations:       payloadAttestations,
		ParentExecutionRequests:   parentExecutionRequests,
	}, nil
}

func (b *BeaconBlockHeze) ToGeneric() (*eth.GenericBeaconBlock, error) {
	block, err := b.ToConsensus()
	if err != nil {
		return nil, errors.Wrap(err, "could not convert heze block to consensus")
	}
	return &eth.GenericBeaconBlock{Block: &eth.GenericBeaconBlock_Heze{Heze: block}}, nil
}

// BlockContentsHezeFromConsensus converts a proto Heze block, envelope, and
// blob data to the API struct.
func BlockContentsHezeFromConsensus(block *eth.BeaconBlockHeze, envelope *eth.ExecutionPayloadEnvelope, kzgProofs [][]byte, blobs [][]byte) (*BlockContentsHeze, error) {
	b, err := BeaconBlockHezeFromConsensus(block)
	if err != nil {
		return nil, fmt.Errorf("convert Heze block: %w", err)
	}
	env, err := ExecutionPayloadEnvelopeFromConsensus(envelope)
	if err != nil {
		return nil, fmt.Errorf("convert execution payload envelope: %w", err)
	}
	encodedProofs := make([]string, len(kzgProofs))
	for i, p := range kzgProofs {
		encodedProofs[i] = hexutil.Encode(p)
	}
	encodedBlobs := make([]string, len(blobs))
	for i, b := range blobs {
		encodedBlobs[i] = hexutil.Encode(b)
	}
	return &BlockContentsHeze{
		Block:                    b,
		ExecutionPayloadEnvelope: env,
		KzgProofs:                encodedProofs,
		Blobs:                    encodedBlobs,
	}, nil
}

func (b *SignedExecutionPayloadBidHeze) ToConsensus() (*eth.SignedExecutionPayloadBidHeze, error) {
	if b == nil {
		return nil, errNilValue
	}
	sig, err := bytesutil.DecodeHexWithLength(b.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}
	message, err := b.Message.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Message")
	}
	return &eth.SignedExecutionPayloadBidHeze{
		Message:   message,
		Signature: sig,
	}, nil
}

func (b *ExecutionPayloadBidHeze) ToConsensus() (*eth.ExecutionPayloadBidHeze, error) {
	if b == nil {
		return nil, errNilValue
	}
	parentBlockHash, err := bytesutil.DecodeHexWithLength(b.ParentBlockHash, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ParentBlockHash")
	}
	parentBlockRoot, err := bytesutil.DecodeHexWithLength(b.ParentBlockRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ParentBlockRoot")
	}
	blockHash, err := bytesutil.DecodeHexWithLength(b.BlockHash, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "BlockHash")
	}
	prevRandao, err := bytesutil.DecodeHexWithLength(b.PrevRandao, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "PrevRandao")
	}
	feeRecipient, err := bytesutil.DecodeHexWithLength(b.FeeRecipient, fieldparams.FeeRecipientLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "FeeRecipient")
	}
	gasLimit, err := strconv.ParseUint(b.GasLimit, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "GasLimit")
	}
	builderIndex, err := strconv.ParseUint(b.BuilderIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "BuilderIndex")
	}
	slot, err := strconv.ParseUint(b.Slot, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Slot")
	}
	value, err := strconv.ParseUint(b.Value, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Value")
	}
	executionPayment, err := strconv.ParseUint(b.ExecutionPayment, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionPayment")
	}
	err = slice.VerifyMaxLength(b.BlobKzgCommitments, fieldparams.MaxBlobCommitmentsPerBlock)
	if err != nil {
		return nil, server.NewDecodeError(err, "BlobKzgCommitments")
	}
	blobKzgCommitments := make([][]byte, len(b.BlobKzgCommitments))
	for i, commitment := range b.BlobKzgCommitments {
		kzg, err := bytesutil.DecodeHexWithLength(commitment, fieldparams.BLSPubkeyLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("BlobKzgCommitments[%d]", i))
		}
		blobKzgCommitments[i] = kzg
	}
	executionRequestsRoot, err := bytesutil.DecodeHexWithLength(b.ExecutionRequestsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ExecutionRequestsRoot")
	}
	inclusionListBits, err := bytesutil.DecodeHexWithLength(b.InclusionListBits, (fieldparams.InclusionListCommitteeSize+7)/8)
	if err != nil {
		return nil, server.NewDecodeError(err, "InclusionListBits")
	}
	return &eth.ExecutionPayloadBidHeze{
		ParentBlockHash:       parentBlockHash,
		ParentBlockRoot:       parentBlockRoot,
		BlockHash:             blockHash,
		PrevRandao:            prevRandao,
		FeeRecipient:          feeRecipient,
		GasLimit:              gasLimit,
		BuilderIndex:          primitives.BuilderIndex(builderIndex),
		Slot:                  primitives.Slot(slot),
		Value:                 primitives.Gwei(value),
		ExecutionPayment:      primitives.Gwei(executionPayment),
		BlobKzgCommitments:    blobKzgCommitments,
		ExecutionRequestsRoot: executionRequestsRoot,
		InclusionListBits:     inclusionListBits,
	}, nil
}
