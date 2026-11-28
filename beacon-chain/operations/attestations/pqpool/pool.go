// Package pqpool holds PQ attestations until a block proof is built from them.
package pqpool

import (
	"sync"

	"google.golang.org/protobuf/proto"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// Spec: a network aggregate, one committee and the proof of its signatures
type Aggregate struct {
	Att   *ethpb.AttestationPQ
	Proof []byte
}

type aggregateKey struct {
	dataRoot  [32]byte
	committee primitives.CommitteeIndex
}

type Pool struct {
	mu         sync.RWMutex
	atts       map[primitives.Epoch]map[primitives.ValidatorIndex]*ethpb.SingleAttestationPQ
	aggregates map[primitives.Epoch]map[aggregateKey][]*Aggregate
	forkchoice []ethpb.Att
}

func New() *Pool {
	return &Pool{
		atts:       make(map[primitives.Epoch]map[primitives.ValidatorIndex]*ethpb.SingleAttestationPQ),
		aggregates: make(map[primitives.Epoch]map[aggregateKey][]*Aggregate),
	}
}

// A validator signs one attestation per target epoch, so the first one seen is kept.
func (p *Pool) Save(att *ethpb.SingleAttestationPQ) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	epoch := att.Data.Target.Epoch
	byValidator, ok := p.atts[epoch]
	if !ok {
		byValidator = make(map[primitives.ValidatorIndex]*ethpb.SingleAttestationPQ)
		p.atts[epoch] = byValidator
	}
	if _, ok := byValidator[att.AttesterIndex]; ok {
		return false
	}
	byValidator[att.AttesterIndex] = att
	p.forkchoice = append(p.forkchoice, att)
	return true
}

func (p *Pool) All() []*ethpb.SingleAttestationPQ {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var out []*ethpb.SingleAttestationPQ
	for _, byValidator := range p.atts {
		for _, att := range byValidator {
			out = append(out, att)
		}
	}
	return out
}

func (p *Pool) HasAggregate(att *ethpb.AttestationPQ) (bool, error) {
	a, err := p.Covering(att)
	return a != nil, err
}

// Spec: should_broadcast_aggregate, a seen aggregate with every attester of att makes it redundant.
func (p *Pool) Covering(att *ethpb.AttestationPQ) (*Aggregate, error) {
	root, err := att.Data.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, a := range p.aggregates[att.Data.Target.Epoch][aggregateKey{root, att.GetCommitteeIndex()}] {
		if contains(a.Att, att) {
			return a, nil
		}
	}
	return nil, nil
}

// A proof cannot be split or joined with another for the same committee, so only supersets replace.
func (p *Pool) SaveAggregate(att *ethpb.AttestationPQ, proof []byte) (bool, error) {
	root, err := att.Data.HashTreeRoot()
	if err != nil {
		return false, err
	}
	key := aggregateKey{root, att.GetCommitteeIndex()}
	p.mu.Lock()
	defer p.mu.Unlock()
	epoch := att.Data.Target.Epoch
	byKey, ok := p.aggregates[epoch]
	if !ok {
		byKey = make(map[aggregateKey][]*Aggregate)
		p.aggregates[epoch] = byKey
	}
	kept := byKey[key][:0]
	for _, a := range byKey[key] {
		if contains(a.Att, att) {
			return false, nil
		}
		if !contains(att, a.Att) {
			kept = append(kept, a)
		}
	}
	byKey[key] = append(kept, &Aggregate{Att: att, Proof: proof})
	p.forkchoice = append(p.forkchoice, att)
	return true, nil
}

// Contains reports whether a has the data and committee of b and every attester of b.
func Contains(a, b *ethpb.AttestationPQ) bool {
	return a.GetCommitteeIndex() == b.GetCommitteeIndex() && proto.Equal(a.Data, b.Data) && contains(a, b)
}

func contains(a, b *ethpb.AttestationPQ) bool {
	if a.AggregationBits.Len() != b.AggregationBits.Len() {
		return false
	}
	ok, err := a.AggregationBits.Contains(b.AggregationBits)
	return err == nil && ok
}

func (p *Pool) Aggregates() []*Aggregate {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var out []*Aggregate
	for _, byKey := range p.aggregates {
		for _, aggregates := range byKey {
			out = append(out, aggregates...)
		}
	}
	return out
}

func (p *Pool) TakeForkchoice() []ethpb.Att {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := p.forkchoice
	p.forkchoice = nil
	return out
}

func (p *Pool) PruneBefore(epoch primitives.Epoch) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for e := range p.atts {
		if e < epoch {
			delete(p.atts, e)
		}
	}
	for e := range p.aggregates {
		if e < epoch {
			delete(p.aggregates, e)
		}
	}
}
