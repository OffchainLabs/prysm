package proofnode

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/time/slots"

	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// The proof node exchanges request bodies as SSZ rather than JSON. The
// containers below are small and live only on this boundary, so they are
// encoded by hand instead of being modelled as protos. Field order and list
// bounds follow github.com/eth-act/zkboost (crates/types/src/lib.rs) and
// github.com/eth-act/ere-guests (stateless-validator-common guest input).

const (
	// offsetSize is the width of an SSZ variable-length field offset.
	offsetSize = 4
	// forkActivationFixedSize covers the two list offsets of ForkActivation.
	forkActivationFixedSize = 2 * offsetSize
	// forkConfigFixedSize covers the single activation offset of ForkConfig.
	forkConfigFixedSize = offsetSize
	// chainConfigFixedSize covers chain_id plus the active_fork offset.
	chainConfigFixedSize = 8 + offsetSize
	// verificationBodyFixedSize covers fork, new_payload_request_root, the
	// chain_config offset, proof_type and the proof offset.
	verificationBodyFixedSize = 1 + 32 + offsetSize + 1 + offsetSize
	// requestBodyFixedSize covers fork plus the new_payload_request,
	// chain_config and proof_types offsets.
	requestBodyFixedSize = 1 + 3*offsetSize
)

// ChainConfig is the chain the proof is generated or verified against. Exactly
// one of ActivationBlockNumber and ActivationTimestamp is set: pre-merge forks
// activate by block number, post-merge forks by timestamp.
type ChainConfig struct {
	ChainID               uint64
	ActivationBlockNumber *uint64
	ActivationTimestamp   *uint64
}

// ChainConfigAt returns the chain configuration the proof node should generate
// or verify against, given this chain's genesis time. The active fork is the
// execution-layer fork whose stateless input schema the payload is encoded
// under, which post-merge activates by timestamp.
func ChainConfigAt(genesisTime time.Time) ChainConfig {
	cfg := params.BeaconConfig()
	activation := uint64(0)
	if startSlot, err := slots.EpochStart(cfg.GloasForkEpoch); err == nil {
		if startTime, err := slots.StartTime(genesisTime, startSlot); err == nil {
			activation = uint64(startTime.Unix())
		}
	}
	return ChainConfig{
		ChainID:             cfg.DepositChainID,
		ActivationTimestamp: &activation,
	}
}

func appendOffset(dst []byte, offset int) []byte {
	return binary.LittleEndian.AppendUint32(dst, uint32(offset))
}

// marshalForkActivation encodes ForkActivation, whose two fields are each an
// SSZ list of at most one uint64.
func (c ChainConfig) marshalForkActivation() []byte {
	var blockNumber, timestamp []byte
	if c.ActivationBlockNumber != nil {
		blockNumber = binary.LittleEndian.AppendUint64(nil, *c.ActivationBlockNumber)
	}
	if c.ActivationTimestamp != nil {
		timestamp = binary.LittleEndian.AppendUint64(nil, *c.ActivationTimestamp)
	}

	out := make([]byte, 0, forkActivationFixedSize+len(blockNumber)+len(timestamp))
	out = appendOffset(out, forkActivationFixedSize)
	out = appendOffset(out, forkActivationFixedSize+len(blockNumber))
	out = append(out, blockNumber...)
	out = append(out, timestamp...)
	return out
}

// marshal encodes ChainConfig, which wraps ForkActivation in a ForkConfig.
func (c ChainConfig) marshal() []byte {
	activation := c.marshalForkActivation()

	forkConfig := make([]byte, 0, forkConfigFixedSize+len(activation))
	forkConfig = appendOffset(forkConfig, forkConfigFixedSize)
	forkConfig = append(forkConfig, activation...)

	out := make([]byte, 0, chainConfigFixedSize+len(forkConfig))
	out = binary.LittleEndian.AppendUint64(out, c.ChainID)
	out = appendOffset(out, chainConfigFixedSize)
	out = append(out, forkConfig...)
	return out
}

// marshalVerificationBody encodes the ProofVerificationBody
// (sent to POST /v1/execution_proof_verifications).
func marshalVerificationBody(
	fork uint8,
	newPayloadRequestRoot [32]byte,
	cfg ChainConfig,
	proofType ethpb.ProofType,
	proof []byte,
) []byte {
	chainConfig := cfg.marshal()

	out := make([]byte, 0, verificationBodyFixedSize+len(chainConfig)+len(proof))
	out = append(out, fork)
	out = append(out, newPayloadRequestRoot[:]...)
	out = appendOffset(out, verificationBodyFixedSize)
	out = append(out, byte(proofType))
	out = appendOffset(out, verificationBodyFixedSize+len(chainConfig))
	out = append(out, chainConfig...)
	out = append(out, proof...)
	return out
}

// marshalRequestBody encodes the ProofRequestBody sent to
// POST /v1/execution_proof_requests.
//
// new_payload_request is a transparent SSZ union: only the variant selected by
// fork is encoded, with no discriminant of its own.
func marshalRequestBody(
	fork uint8,
	newPayloadRequest *enginev1.SSZNewPayloadRequest,
	cfg ChainConfig,
	proofTypes []ethpb.ProofType,
) ([]byte, error) {
	payload, err := newPayloadRequest.MarshalSSZ()
	if err != nil {
		return nil, fmt.Errorf("marshal new payload request: %w", err)
	}
	chainConfig := cfg.marshal()

	types := make([]byte, 0, len(proofTypes))
	for _, proofType := range proofTypes {
		types = append(types, byte(proofType))
	}

	out := make([]byte, 0, requestBodyFixedSize+len(payload)+len(chainConfig)+len(types))
	out = append(out, fork)
	out = appendOffset(out, requestBodyFixedSize)
	out = appendOffset(out, requestBodyFixedSize+len(payload))
	out = appendOffset(out, requestBodyFixedSize+len(payload)+len(chainConfig))
	out = append(out, payload...)
	out = append(out, chainConfig...)
	out = append(out, types...)
	return out, nil
}
