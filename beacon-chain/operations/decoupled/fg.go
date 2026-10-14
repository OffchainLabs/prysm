package decoupled

import (
	"slices"
	"sync"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

type fgKey struct {
	height, finalityHeight primitives.Height
	target, finalityTarget [32]byte
	timeout                bool
}

type fgAggregate struct {
	fgKey
	sig [96]byte
}

// Spec: fg.attestations in first-acceptance order. Gossip singles merge into this node's own aggregate per data, block rows are held as they came, and nothing drains since a row spent on one branch may still be needed on another.
type FGPool struct {
	lock   sync.Mutex
	atts   []*ethpb.IndexedAttestationDecoupled
	own    map[fgKey]*ethpb.IndexedAttestationDecoupled
	held   map[fgAggregate]struct{}
	pruned primitives.Height
}

func NewFGPool() *FGPool {
	return &FGPool{own: make(map[fgKey]*ethpb.IndexedAttestationDecoupled), held: make(map[fgAggregate]struct{})}
}

func keyOf(d *ethpb.AttestationDataDecoupled) fgKey {
	return fgKey{
		height:         d.Height,
		finalityHeight: d.FinalityHeight,
		target:         bytesutil.ToBytes32(d.Target),
		finalityTarget: bytesutil.ToBytes32(d.FinalityTarget),
		timeout:        d.Timeout,
	}
}

// Spec: fg_on_attestation for a gossip single.
func (p *FGPool) InsertSingle(validator primitives.ValidatorIndex, data *ethpb.AttestationDataDecoupled, sig []byte) error {
	p.lock.Lock()
	defer p.lock.Unlock()
	if data.Height < p.pruned {
		return nil
	}
	key := keyOf(data)
	agg := p.own[key]
	if agg == nil {
		agg = &ethpb.IndexedAttestationDecoupled{AttestingIndices: []uint64{uint64(validator)}, Data: data.Copy(), Signature: bytesutil.SafeCopyBytes(sig)}
		p.own[key] = agg
		p.atts = append(p.atts, agg)
		return nil
	}
	i, found := slices.BinarySearch(agg.AttestingIndices, uint64(validator))
	if found {
		return nil
	}
	merged, err := bls.AggregateCompressedSignatures([][]byte{agg.Signature, sig})
	if err != nil {
		return err
	}
	agg.Signature = merged.Marshal()
	agg.AttestingIndices = slices.Insert(agg.AttestingIndices, i, uint64(validator))
	return nil
}

// Spec: the fg_on_block tail, every distinct row once.
func (p *FGPool) InsertAggregates(atts []*ethpb.IndexedAttestationDecoupled) {
	p.lock.Lock()
	defer p.lock.Unlock()
	for _, a := range atts {
		if a.Data == nil || a.Data.Height < p.pruned {
			continue
		}
		id := fgAggregate{fgKey: keyOf(a.Data), sig: bytesutil.ToBytes96(a.Signature)}
		if _, ok := p.held[id]; ok {
			continue
		}
		p.held[id] = struct{}{}
		p.atts = append(p.atts, a)
	}
}

// Spec: proposal_attestations. keep sees every row in first-acceptance order, kept rows are copied since own aggregates keep growing.
func (p *FGPool) Select(keep func(*ethpb.IndexedAttestationDecoupled) bool) []*ethpb.IndexedAttestationDecoupled {
	p.lock.Lock()
	defer p.lock.Unlock()
	var out []*ethpb.IndexedAttestationDecoupled
	for _, a := range p.atts {
		if keep(a) {
			out = append(out, a.Copy())
		}
	}
	return out
}

// Every allowed branch descends from the entry at the accepted finalized height, so rows strictly below it are valid nowhere. Rows at that height still advance a branch that has not left it.
func (p *FGPool) Prune(finalized primitives.Height) {
	p.lock.Lock()
	defer p.lock.Unlock()
	if finalized <= p.pruned {
		return
	}
	p.pruned = finalized
	p.atts = slices.DeleteFunc(p.atts, func(a *ethpb.IndexedAttestationDecoupled) bool { return a.Data.Height < finalized })
	for k := range p.own {
		if k.height < finalized {
			delete(p.own, k)
		}
	}
	for k := range p.held {
		if k.height < finalized {
			delete(p.held, k)
		}
	}
}
