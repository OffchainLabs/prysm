package payloadattestation

import (
	"sync"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

var errNilPayloadAttestationMessage = errors.New("nil payload attestation message")

// PoolManager maintains pending payload attestations.
// This pool is used by proposers to insert payload attestations into new blocks.
type PoolManager interface {
	// PendingPayloadAttestations returns all pending aggregated payload attestations.
	// If a slot is provided, only attestations for that slot are returned.
	PendingPayloadAttestations(slot ...primitives.Slot) []*ethpb.PayloadAttestation
	// InsertPayloadAttestation inserts or aggregates a payload attestation
	// message into the pool. The idx parameter is the PTC committee index
	// of the validator (position in the bitvector).
	InsertPayloadAttestation(msg *ethpb.PayloadAttestationMessage, idx uint64) error
	// Seen returns true if the PTC committee index has already been seen
	// for the given PayloadAttestationData.
	Seen(data *ethpb.PayloadAttestationData, idx uint64) bool
	// MarkIncluded removes the attestation matching the given data from the pool.
	MarkIncluded(att *ethpb.PayloadAttestation)
}

// Pool is a concrete implementation of PoolManager.
// Keyed by hash of PayloadAttestationData; stores aggregated PayloadAttestation.
type Pool struct {
	lock    sync.RWMutex
	pending map[[32]byte]*ethpb.PayloadAttestation
}

// NewPool returns an initialized pool.
func NewPool() *Pool {
	return &Pool{
		pending: make(map[[32]byte]*ethpb.PayloadAttestation),
	}
}

// PendingPayloadAttestations returns all pending payload attestations.
// If a slot filter is provided, only attestations for that slot are returned.
func (p *Pool) PendingPayloadAttestations(slot ...primitives.Slot) []*ethpb.PayloadAttestation {
	p.lock.RLock()
	defer p.lock.RUnlock()

	result := make([]*ethpb.PayloadAttestation, 0, len(p.pending))
	for _, att := range p.pending {
		if len(slot) > 0 && att.Data.Slot != slot[0] {
			continue
		}
		result = append(result, att)
	}
	return result
}

// InsertPayloadAttestation inserts a payload attestation message into the pool,
// aggregating it with any existing attestation that shares the same PayloadAttestationData.
// The idx parameter is the PTC committee index used to set the aggregation bit.
func (p *Pool) InsertPayloadAttestation(msg *ethpb.PayloadAttestationMessage, idx uint64) error {
	if msg == nil || msg.Data == nil {
		return errNilPayloadAttestationMessage
	}

	key, err := dataKey(msg.Data)
	if err != nil {
		return errors.Wrap(err, "could not compute data key")
	}

	p.lock.Lock()
	defer p.lock.Unlock()

	existing, ok := p.pending[key]
	if !ok {
		p.pending[key] = messageToPayloadAttestation(msg, idx)
		return nil
	}

	if existing.AggregationBits.BitAt(idx) {
		return nil
	}

	sig, err := aggregateSigFromMessage(existing, msg)
	if err != nil {
		return errors.Wrap(err, "could not aggregate signatures")
	}
	existing.Signature = sig
	existing.AggregationBits.SetBitAt(idx, true)
	return nil
}

// Seen returns true if the PTC committee index has already been seen
// for the given PayloadAttestationData.
func (p *Pool) Seen(data *ethpb.PayloadAttestationData, idx uint64) bool {
	if data == nil {
		return false
	}

	key, err := dataKey(data)
	if err != nil {
		return false
	}

	p.lock.RLock()
	defer p.lock.RUnlock()

	existing, ok := p.pending[key]
	if !ok {
		return false
	}
	return existing.AggregationBits.BitAt(idx)
}

// MarkIncluded removes the attestation with matching data from the pool.
func (p *Pool) MarkIncluded(att *ethpb.PayloadAttestation) {
	if att == nil || att.Data == nil {
		return
	}

	key, err := dataKey(att.Data)
	if err != nil {
		return
	}

	p.lock.Lock()
	defer p.lock.Unlock()

	delete(p.pending, key)
}

// messageToPayloadAttestation creates a PayloadAttestation with a single
// aggregated bit from the passed PayloadAttestationMessage.
func messageToPayloadAttestation(msg *ethpb.PayloadAttestationMessage, idx uint64) *ethpb.PayloadAttestation {
	bits := bitfield.NewBitvector512()
	bits.SetBitAt(idx, true)
	data := &ethpb.PayloadAttestationData{
		BeaconBlockRoot:   bytesutil.SafeCopyBytes(msg.Data.BeaconBlockRoot),
		Slot:              msg.Data.Slot,
		PayloadPresent:    msg.Data.PayloadPresent,
		BlobDataAvailable: msg.Data.BlobDataAvailable,
	}
	return &ethpb.PayloadAttestation{
		AggregationBits: bits,
		Data:            data,
		Signature:       bytesutil.SafeCopyBytes(msg.Signature),
	}
}

// aggregateSigFromMessage returns the aggregated signature by combining the
// existing aggregated signature with the message's signature.
func aggregateSigFromMessage(aggregated *ethpb.PayloadAttestation, message *ethpb.PayloadAttestationMessage) ([]byte, error) {
	aggSig, err := bls.SignatureFromBytesNoValidation(aggregated.Signature)
	if err != nil {
		return nil, err
	}
	sig, err := bls.SignatureFromBytesNoValidation(message.Signature)
	if err != nil {
		return nil, err
	}
	return bls.AggregateSignatures([]bls.Signature{aggSig, sig}).Marshal(), nil
}

// dataKey computes a deterministic key for PayloadAttestationData
// by hashing its serialized form.
func dataKey(data *ethpb.PayloadAttestationData) ([32]byte, error) {
	enc, err := proto.Marshal(data)
	if err != nil {
		return [32]byte{}, err
	}
	return hash.Hash(enc), nil
}
