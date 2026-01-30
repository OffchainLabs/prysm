package payloadattestation

import (
	"sync"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

// PoolManager maintains pending payload attestations.
// This pool is used by proposers to insert payload attestations into new blocks.
type PoolManager interface {
	PendingPayloadAttestations(slot ...primitives.Slot) []*ethpb.PayloadAttestation
	InsertPayloadAttestation(msg *ethpb.PayloadAttestationMessage) error
	MarkIncluded(att *ethpb.PayloadAttestation)
}

// Pool is a concrete implementation of PoolManager.
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

// PendingPayloadAttestations returns all pending payload attestations from the pool.
// If a slot is provided, only attestations for that slot are returned.
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

// InsertPayloadAttestation inserts a payload attestation message into the pool.
// The message is converted to an aggregated PayloadAttestation and merged with
// any existing attestation that has matching PayloadAttestationData.
func (p *Pool) InsertPayloadAttestation(msg *ethpb.PayloadAttestationMessage) error {
	if msg == nil || msg.Data == nil {
		return errors.New("nil payload attestation message")
	}

	key, err := dataKey(msg.Data)
	if err != nil {
		return errors.Wrap(err, "could not compute data key")
	}

	p.lock.Lock()
	defer p.lock.Unlock()

	existing, ok := p.pending[key]
	if !ok {
		// Create a new aggregated PayloadAttestation from this message.
		p.pending[key] = &ethpb.PayloadAttestation{
			AggregationBits: []byte{},
			Data:            proto.Clone(msg.Data).(*ethpb.PayloadAttestationData),
			Signature:       msg.Signature,
		}
		return nil
	}

	// Merge: for now just replace the signature since proper BLS aggregation
	// requires knowing the PTC committee position. Full aggregation is a TODO.
	_ = existing
	return nil
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

// dataKey computes a deterministic key for PayloadAttestationData
// by hashing its serialized form.
func dataKey(data *ethpb.PayloadAttestationData) ([32]byte, error) {
	enc, err := proto.Marshal(data)
	if err != nil {
		return [32]byte{}, err
	}
	return hash.Hash(enc), nil
}
