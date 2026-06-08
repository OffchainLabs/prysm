package payloadattestation

import (
	"sync"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

var errNilPayloadAttestationMessage = errors.New("nil payload attestation message")

type payloadAttestationDataKey struct {
	beaconBlockRoot   [32]byte
	slot              primitives.Slot
	payloadPresent    bool
	blobDataAvailable bool
}

// PoolManager manages pending, aggregated payload attestations keyed by
// payload-attestation data.
type PoolManager interface {
	// PendingPayloadAttestations returns pending attestations for the requested slot.
	PendingPayloadAttestations(slot primitives.Slot) []*ethpb.PayloadAttestation
	// InsertPayloadAttestation inserts or aggregates a payload attestation
	// message into the pool. The idx parameter is the PTC committee index
	// of the validator (position in the bitvector).
	InsertPayloadAttestation(msg *ethpb.PayloadAttestationMessage, idx uint64) error
	// Seen returns true if the PTC committee index has already been seen
	// for the given PayloadAttestationData.
	Seen(data *ethpb.PayloadAttestationData, idx uint64) bool
}

// Pool is an in-memory implementation of PoolManager.
// Entries are keyed by payload-attestation data fields and store an aggregated
// PayloadAttestation value.
type Pool struct {
	lock    sync.RWMutex
	pending map[payloadAttestationDataKey]*ethpb.PayloadAttestation
}

// NewPool returns an initialized pool.
func NewPool() *Pool {
	pool := &Pool{
		pending: make(map[payloadAttestationDataKey]*ethpb.PayloadAttestation),
	}
	payloadAttestationPoolSize.Set(0)
	return pool
}

// PendingPayloadAttestations returns payload attestations for the requested slot.
func (p *Pool) PendingPayloadAttestations(slot primitives.Slot) []*ethpb.PayloadAttestation {
	p.lock.Lock()
	defer p.lock.Unlock()

	result := make([]*ethpb.PayloadAttestation, 0, len(p.pending))
	for _, att := range p.pending {
		if att.Data.Slot == slot {
			result = append(result, att)
		}
	}
	return result
}

// InsertPayloadAttestation inserts a payload attestation message into the pool.
// If an attestation with matching data already exists, it aggregates the BLS
// signature and sets the aggregation bit for idx.
// idx is the validator's position in the PTC committee bitfield. It also prunes
// stale entries with slot lower than msg.Data.Slot.
func (p *Pool) InsertPayloadAttestation(msg *ethpb.PayloadAttestationMessage, idx uint64) error {
	if msg == nil || msg.Data == nil {
		return errNilPayloadAttestationMessage
	}
	if idx >= uint64(fieldparams.PTCSize) {
		return errors.Errorf("invalid payload attestation committee index: %d", idx)
	}

	key, err := dataKey(msg.Data)
	if err != nil {
		return errors.Wrap(err, "could not compute data key")
	}

	p.lock.Lock()
	defer p.lock.Unlock()

	p.pruneOlderSlotsLocked(msg.Data.Slot)

	existing, ok := p.pending[key]
	if !ok {
		p.pending[key] = messageToPayloadAttestation(msg, idx)
		payloadAttestationPoolSize.Set(float64(len(p.pending)))
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
	payloadAttestationPoolSize.Set(float64(len(p.pending)))
	return nil
}

func (p *Pool) pruneOlderSlotsLocked(slot primitives.Slot) {
	for key, att := range p.pending {
		if att == nil || att.Data == nil || att.Data.Slot < slot {
			delete(p.pending, key)
		}
	}
	payloadAttestationPoolSize.Set(float64(len(p.pending)))
}

// Seen reports whether idx has already been observed for the given
// PayloadAttestationData.
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

// messageToPayloadAttestation creates an aggregated PayloadAttestation with a
// single bit set at idx from msg.
func messageToPayloadAttestation(msg *ethpb.PayloadAttestationMessage, idx uint64) *ethpb.PayloadAttestation {
	bits := ethpb.NewPayloadAttestationAggregationBits()
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

// aggregateSigFromMessage aggregates the existing signature with the new
// message signature.
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

// dataKey derives the map key directly from PayloadAttestationData fields.
// BeaconBlockRoot must be 32 bytes.
func dataKey(data *ethpb.PayloadAttestationData) (payloadAttestationDataKey, error) {
	if data == nil {
		return payloadAttestationDataKey{}, errNilPayloadAttestationMessage
	}
	if len(data.BeaconBlockRoot) != fieldparams.RootLength {
		return payloadAttestationDataKey{}, errors.Errorf("invalid beacon block root length: %d", len(data.BeaconBlockRoot))
	}
	return payloadAttestationDataKey{
		beaconBlockRoot:   bytesutil.ToBytes32(data.BeaconBlockRoot),
		slot:              data.Slot,
		payloadPresent:    data.PayloadPresent,
		blobDataAvailable: data.BlobDataAvailable,
	}, nil
}
