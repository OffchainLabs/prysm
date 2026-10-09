package structs

import (
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api/server"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// ToConsensus converts a JSON signed execution proof envelope to its consensus
// type.
func (p *SignedExecutionProofEnvelope) ToConsensus() (*eth.SignedExecutionProofEnvelope, error) {
	if p == nil {
		return nil, errNilValue
	}

	message, err := p.Message.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Message")
	}

	validatorIndex, err := strconv.ParseUint(p.ValidatorIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ValidatorIndex")
	}

	signature, err := bytesutil.DecodeHexWithLength(p.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}

	return &eth.SignedExecutionProofEnvelope{
		Message:        message,
		ValidatorIndex: primitives.ValidatorIndex(validatorIndex),
		Signature:      signature,
	}, nil
}

// ToConsensus converts a JSON execution proof envelope to its consensus type.
func (p *ExecutionProofEnvelope) ToConsensus() (*eth.ExecutionProofEnvelope, error) {
	if p == nil {
		return nil, errNilValue
	}

	proofData, err := bytesutil.DecodeHexWithMaxLength(p.ProofData, params.BeaconConfig().MaxProofSize)
	if err != nil {
		return nil, server.NewDecodeError(err, "ProofData")
	}

	proofType, err := strconv.ParseUint(p.ProofType, 10, 8)
	if err != nil {
		return nil, server.NewDecodeError(err, "ProofType")
	}

	beaconBlockRoot, err := bytesutil.DecodeHexWithLength(p.BeaconBlockRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "BeaconBlockRoot")
	}

	return &eth.ExecutionProofEnvelope{
		ProofData:       proofData,
		ProofType:       []byte{byte(proofType)},
		BeaconBlockRoot: beaconBlockRoot,
	}, nil
}
