// Package pqpool holds single PQ attestations with their XMSS signatures until a proof is built.
package pqpool

import (
	"sync"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

type Pool struct {
	mu         sync.RWMutex
	atts       map[primitives.Epoch]map[primitives.ValidatorIndex]*ethpb.SingleAttestationPQ
	forkchoice []*ethpb.SingleAttestationPQ
}

func New() *Pool {
	return &Pool{atts: make(map[primitives.Epoch]map[primitives.ValidatorIndex]*ethpb.SingleAttestationPQ)}
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

func (p *Pool) TakeForkchoice() []*ethpb.SingleAttestationPQ {
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
}
