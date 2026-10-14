package decoupled

import (
	"fmt"
	"sync"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

type voteKey struct {
	slot      primitives.Slot
	validator primitives.ValidatorIndex
	root      [32]byte
}

type voterKey struct {
	slot      primitives.Slot
	validator primitives.ValidatorIndex
}

// Spec: the signed AC messages of the current and previous slot, the proposer aggregates the engine's snapshot from them by root.
type VotePool struct {
	lock   sync.Mutex
	votes  map[voteKey][]byte
	counts map[voterKey]int
}

func NewVotePool() *VotePool {
	return &VotePool{votes: make(map[voteKey][]byte), counts: make(map[voterKey]int)}
}

func (p *VotePool) Insert(validator primitives.ValidatorIndex, slot primitives.Slot, root [32]byte, sig []byte) {
	p.lock.Lock()
	defer p.lock.Unlock()
	k := voteKey{slot: slot, validator: validator, root: root}
	if _, ok := p.votes[k]; !ok {
		p.votes[k] = bytesutil.SafeCopyBytes(sig)
		p.counts[voterKey{slot: slot, validator: validator}]++
	}
}

// Spec: the gossip IGNORE rules, a repeat and a third distinct vote by one validator in a slot. Two distinct votes travel so every node sees the equivocation.
func (p *VotePool) Seen(validator primitives.ValidatorIndex, slot primitives.Slot, root [32]byte) bool {
	p.lock.Lock()
	defer p.lock.Unlock()
	if _, ok := p.votes[voteKey{slot: slot, validator: validator, root: root}]; ok {
		return true
	}
	return p.counts[voterKey{slot: slot, validator: validator}] >= 2
}

// Spec: B.votes. One aggregate per root over exactly the validators the engine returned, bit i is validator i.
func (p *VotePool) Aggregate(slot primitives.Slot, root [32]byte, validators []primitives.ValidatorIndex, size uint64) (*ethpb.AvailableAttestation, error) {
	p.lock.Lock()
	defer p.lock.Unlock()
	bits := bitfield.NewBitlist(size)
	sigs := make([][]byte, 0, len(validators))
	for _, v := range validators {
		sig, ok := p.votes[voteKey{slot: slot, validator: v, root: root}]
		if !ok {
			return nil, fmt.Errorf("no signed vote from validator %d for %#x at slot %d", v, root, slot)
		}
		bits.SetBitAt(uint64(v), true)
		sigs = append(sigs, sig)
	}
	agg, err := bls.AggregateCompressedSignatures(sigs)
	if err != nil {
		return nil, err
	}
	return &ethpb.AvailableAttestation{
		AggregationBits: bits,
		Data:            &ethpb.AvailableAttestationData{Slot: slot, BeaconBlockRoot: root[:]},
		Signature:       agg.Marshal(),
	}, nil
}

// The slot s proposer needs slot s-1 votes, anything older serves no proposal.
func (p *VotePool) Prune(current primitives.Slot) {
	p.lock.Lock()
	defer p.lock.Unlock()
	for k := range p.votes {
		if k.slot+1 < current {
			delete(p.votes, k)
		}
	}
	for k := range p.counts {
		if k.slot+1 < current {
			delete(p.counts, k)
		}
	}
}
