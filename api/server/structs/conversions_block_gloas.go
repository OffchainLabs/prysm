package structs

import (
	"fmt"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api/server"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// ExecutionPayloadEnvelopeFromConsensus converts a proto envelope to the API struct.
func ExecutionPayloadEnvelopeFromConsensus(e *eth.ExecutionPayloadEnvelope) (*ExecutionPayloadEnvelope, error) {
	payload, err := ExecutionPayloadGloasFromConsensus(e.Payload)
	if err != nil {
		return nil, err
	}
	var requests *ExecutionRequestsGloas
	if e.ExecutionRequests != nil {
		requests = ExecutionRequestsGloasFromConsensus(e.ExecutionRequests)
	}
	return &ExecutionPayloadEnvelope{
		Payload:               payload,
		ExecutionRequests:     requests,
		BuilderIndex:          fmt.Sprintf("%d", e.BuilderIndex),
		BeaconBlockRoot:       hexutil.Encode(e.BeaconBlockRoot),
		ParentBeaconBlockRoot: hexutil.Encode(e.ParentBeaconBlockRoot),
	}, nil
}

// SignedExecutionPayloadEnvelopeFromConsensus converts a signed proto envelope to the API struct.
func SignedExecutionPayloadEnvelopeFromConsensus(e *eth.SignedExecutionPayloadEnvelope) (*SignedExecutionPayloadEnvelope, error) {
	envelope, err := ExecutionPayloadEnvelopeFromConsensus(e.Message)
	if err != nil {
		return nil, err
	}
	return &SignedExecutionPayloadEnvelope{
		Message:   envelope,
		Signature: hexutil.Encode(e.Signature),
	}, nil
}

// BlockContentsGloasFromConsensus converts a proto Gloas block, envelope, and
// blob data to the API struct.
func BlockContentsGloasFromConsensus(block *eth.BeaconBlockGloas, envelope *eth.ExecutionPayloadEnvelope, kzgProofs [][]byte, blobs [][]byte) (*BlockContentsGloas, error) {
	b, err := BeaconBlockGloasFromConsensus(block)
	if err != nil {
		return nil, err
	}
	env, err := ExecutionPayloadEnvelopeFromConsensus(envelope)
	if err != nil {
		return nil, err
	}
	encodedProofs := make([]string, len(kzgProofs))
	for i, p := range kzgProofs {
		encodedProofs[i] = hexutil.Encode(p)
	}
	encodedBlobs := make([]string, len(blobs))
	for i, b := range blobs {
		encodedBlobs[i] = hexutil.Encode(b)
	}
	return &BlockContentsGloas{
		Block:                    b,
		ExecutionPayloadEnvelope: env,
		KzgProofs:                encodedProofs,
		Blobs:                    encodedBlobs,
	}, nil
}

// ToConsensus converts the API struct to a proto ExecutionPayloadEnvelope.
func (e *ExecutionPayloadEnvelope) ToConsensus() (*eth.ExecutionPayloadEnvelope, error) {
	if e == nil {
		return nil, server.NewDecodeError(errNilValue, "ExecutionPayloadEnvelope")
	}
	payload, err := e.Payload.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Payload")
	}
	var requests *enginev1.ExecutionRequestsGloas
	if e.ExecutionRequests != nil {
		requests, err = e.ExecutionRequests.ToConsensus()
		if err != nil {
			return nil, server.NewDecodeError(err, "ExecutionRequests")
		}
	}
	builderIndex, err := strconv.ParseUint(e.BuilderIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "BuilderIndex")
	}
	beaconBlockRoot, err := bytesutil.DecodeHexWithLength(e.BeaconBlockRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "BeaconBlockRoot")
	}
	parentBeaconBlockRoot, err := bytesutil.DecodeHexWithLength(e.ParentBeaconBlockRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ParentBeaconBlockRoot")
	}
	return &eth.ExecutionPayloadEnvelope{
		Payload:               payload,
		ExecutionRequests:     requests,
		BuilderIndex:          primitives.BuilderIndex(builderIndex),
		BeaconBlockRoot:       beaconBlockRoot,
		ParentBeaconBlockRoot: parentBeaconBlockRoot,
	}, nil
}

// ToConsensus converts the API struct to a proto SignedExecutionPayloadEnvelope.
func (e *SignedExecutionPayloadEnvelope) ToConsensus() (*eth.SignedExecutionPayloadEnvelope, error) {
	if e == nil {
		return nil, server.NewDecodeError(errNilValue, "SignedExecutionPayloadEnvelope")
	}
	msg, err := e.Message.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Message")
	}
	sig, err := bytesutil.DecodeHexWithLength(e.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}
	return &eth.SignedExecutionPayloadEnvelope{
		Message:   msg,
		Signature: sig,
	}, nil
}

func BlindedExecutionPayloadEnvelopeFromConsensus(b *eth.WireBlindedExecutionPayloadEnvelope) (*BlindedExecutionPayloadEnvelope, error) {
	if b == nil {
		return nil, errNilValue
	}
	var requests *ExecutionRequestsGloas
	if b.ExecutionRequests != nil {
		requests = ExecutionRequestsGloasFromConsensus(b.ExecutionRequests)
	}
	return &BlindedExecutionPayloadEnvelope{
		PayloadRoot:           hexutil.Encode(b.PayloadRoot),
		ExecutionRequests:     requests,
		BuilderIndex:          fmt.Sprintf("%d", b.BuilderIndex),
		BeaconBlockRoot:       hexutil.Encode(b.BeaconBlockRoot),
		ParentBeaconBlockRoot: hexutil.Encode(b.ParentBeaconBlockRoot),
	}, nil
}

func (b *BlindedExecutionPayloadEnvelope) ToConsensus() (*eth.WireBlindedExecutionPayloadEnvelope, error) {
	if b == nil {
		return nil, server.NewDecodeError(errNilValue, "BlindedExecutionPayloadEnvelope")
	}
	payloadRoot, err := bytesutil.DecodeHexWithLength(b.PayloadRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "PayloadRoot")
	}
	var requests *enginev1.ExecutionRequestsGloas
	if b.ExecutionRequests != nil {
		requests, err = b.ExecutionRequests.ToConsensus()
		if err != nil {
			return nil, server.NewDecodeError(err, "ExecutionRequests")
		}
	}
	builderIndex, err := strconv.ParseUint(b.BuilderIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "BuilderIndex")
	}
	beaconBlockRoot, err := bytesutil.DecodeHexWithLength(b.BeaconBlockRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "BeaconBlockRoot")
	}
	parentBeaconBlockRoot, err := bytesutil.DecodeHexWithLength(b.ParentBeaconBlockRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ParentBeaconBlockRoot")
	}
	return &eth.WireBlindedExecutionPayloadEnvelope{
		PayloadRoot:           payloadRoot,
		ExecutionRequests:     requests,
		BuilderIndex:          primitives.BuilderIndex(builderIndex),
		BeaconBlockRoot:       beaconBlockRoot,
		ParentBeaconBlockRoot: parentBeaconBlockRoot,
	}, nil
}

func (s *SignedBlindedExecutionPayloadEnvelope) ToConsensus() (*eth.SignedWireBlindedExecutionPayloadEnvelope, error) {
	if s == nil {
		return nil, server.NewDecodeError(errNilValue, "SignedBlindedExecutionPayloadEnvelope")
	}
	msg, err := s.Message.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Message")
	}
	sig, err := bytesutil.DecodeHexWithLength(s.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}
	return &eth.SignedWireBlindedExecutionPayloadEnvelope{
		Message:   msg,
		Signature: sig,
	}, nil
}

// SignedExecutionPayloadEnvelopeContentsFromConsensus builds the API struct
// used for stateless envelope publishing from native components.
func SignedExecutionPayloadEnvelopeContentsFromConsensus(signed *eth.SignedExecutionPayloadEnvelope, kzgProofs [][]byte, blobs [][]byte) (*SignedExecutionPayloadEnvelopeContents, error) {
	signedJSON, err := SignedExecutionPayloadEnvelopeFromConsensus(signed)
	if err != nil {
		return nil, err
	}
	encodedProofs := make([]string, len(kzgProofs))
	for i, p := range kzgProofs {
		encodedProofs[i] = hexutil.Encode(p)
	}
	encodedBlobs := make([]string, len(blobs))
	for i, b := range blobs {
		encodedBlobs[i] = hexutil.Encode(b)
	}
	return &SignedExecutionPayloadEnvelopeContents{
		SignedExecutionPayloadEnvelope: signedJSON,
		KzgProofs:                      encodedProofs,
		Blobs:                          encodedBlobs,
	}, nil
}

// ToConsensus decodes the API struct into the signed envelope plus raw blob and
// KZG proof bytes used by the stateless publish path.
func (c *SignedExecutionPayloadEnvelopeContents) ToConsensus() (*eth.SignedExecutionPayloadEnvelope, [][]byte, [][]byte, error) {
	if c == nil {
		return nil, nil, nil, server.NewDecodeError(errNilValue, "SignedExecutionPayloadEnvelopeContents")
	}
	signed, err := c.SignedExecutionPayloadEnvelope.ToConsensus()
	if err != nil {
		return nil, nil, nil, server.NewDecodeError(err, "SignedExecutionPayloadEnvelope")
	}
	proofs := make([][]byte, len(c.KzgProofs))
	for i, p := range c.KzgProofs {
		proof, err := bytesutil.DecodeHexWithLength(p, 48)
		if err != nil {
			return nil, nil, nil, server.NewDecodeError(err, fmt.Sprintf("KzgProofs[%d]", i))
		}
		proofs[i] = proof
	}
	blobs := make([][]byte, len(c.Blobs))
	for i, b := range c.Blobs {
		blob, err := bytesutil.DecodeHexWithLength(b, fieldparams.BlobSize)
		if err != nil {
			return nil, nil, nil, server.NewDecodeError(err, fmt.Sprintf("Blobs[%d]", i))
		}
		blobs[i] = blob
	}
	return signed, proofs, blobs, nil
}

// WireBlindedFromFull derives the spec-wire blinded envelope from a full one: payload_root is
// HashTreeRoot(payload), so HashTreeRoot(blinded) == HashTreeRoot(full) and a validator signature
// over either form is valid against the other.
func WireBlindedFromFull(full *eth.ExecutionPayloadEnvelope) (*eth.WireBlindedExecutionPayloadEnvelope, error) {
	if full == nil {
		return nil, nil
	}
	payloadRoot, err := full.Payload.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	return &eth.WireBlindedExecutionPayloadEnvelope{
		PayloadRoot:           payloadRoot[:],
		ExecutionRequests:     full.ExecutionRequests,
		BuilderIndex:          full.BuilderIndex,
		BeaconBlockRoot:       bytesutil.SafeCopyBytes(full.BeaconBlockRoot),
		ParentBeaconBlockRoot: bytesutil.SafeCopyBytes(full.ParentBeaconBlockRoot),
	}, nil
}

// SignedWireBlindedFromFull lifts a signed envelope to its blinded form, preserving the signature.
func SignedWireBlindedFromFull(full *eth.SignedExecutionPayloadEnvelope) (*eth.SignedWireBlindedExecutionPayloadEnvelope, error) {
	if full == nil {
		return nil, nil
	}
	msg, err := WireBlindedFromFull(full.Message)
	if err != nil {
		return nil, err
	}
	return &eth.SignedWireBlindedExecutionPayloadEnvelope{
		Message:   msg,
		Signature: bytesutil.SafeCopyBytes(full.Signature),
	}, nil
}
