// Package decoupled holds the validator's FG signing record and the rules that choose an FG attestation from it.
package decoupled

import "github.com/OffchainLabs/prysm/v7/consensus-types/primitives"

// Spec: (h_c, T_c), the height and entry read from the state of fg_confirmed. A zero height means no source.
type Source struct {
	Height primitives.Height
	Entry  [32]byte
}

// Spec: (h_j, J, h_F), read from the state of the head.
type Head struct {
	JustifiedHeight primitives.Height
	Justified       [32]byte
	FinalizedHeight primitives.Height
}

// Spec: the FG attestation (h, T, τ, h_f, T_f). A zero FinalityHeight means no finality pair.
type Attestation struct {
	Height         primitives.Height
	Target         [32]byte
	Timeout        bool
	FinalityHeight primitives.Height
	FinalityTarget [32]byte
}

// Spec: the anti-slashing record Λ. It belongs to the validator and must survive restarts and reorgs.
type Record struct {
	targets  map[primitives.Height][32]byte
	timeouts map[primitives.Height]bool
	locks    map[primitives.Height][32]byte
	named    map[primitives.Height]map[[32]byte]struct{}
}

func NewRecord() *Record {
	return &Record{
		targets:  make(map[primitives.Height][32]byte),
		timeouts: make(map[primitives.Height]bool),
		locks:    make(map[primitives.Height][32]byte),
		named:    make(map[primitives.Height]map[[32]byte]struct{}),
	}
}

// Spec: create_attestation. The finality pair is chosen first and acts as this attestation's own lock for the height pair. Nothing is returned without a height pair, and the record is written before the attestation is released.
func (r *Record) Attest(src Source, head Head) (Attestation, bool) {
	a := r.finalityPair(head)
	if !r.heightPair(&a, src) {
		return Attestation{}, false
	}
	// Spec: a target only as the first vote at h, and each entry named at most once at h.
	entries := r.named[a.Height]
	if _, seen := entries[a.Target]; seen || (!a.Timeout && len(entries) > 0) {
		return Attestation{}, false
	}
	if a.FinalityHeight != 0 && a.Height <= a.FinalityHeight {
		a.FinalityHeight, a.FinalityTarget = 0, [32]byte{}
	}
	r.record(a)
	return a, true
}

// Spec: finality_pair. A justification not yet finalized, allowed by the record at its height.
func (r *Record) finalityPair(head Head) Attestation {
	h := head.JustifiedHeight
	if h <= head.FinalizedHeight || r.timeouts[h] {
		return Attestation{}
	}
	if t, ok := r.targets[h]; ok && t != head.Justified {
		return Attestation{}
	}
	if l, ok := r.locks[h]; ok && l != head.Justified {
		return Attestation{}
	}
	return Attestation{FinalityHeight: h, FinalityTarget: head.Justified}
}

// Spec: height_pair, the rows in order. A lock elsewhere at this height means abstaining, a different recorded target means a timeout.
func (r *Record) heightPair(a *Attestation, src Source) bool {
	if src.Height == 0 {
		return false
	}
	a.Height, a.Target = src.Height, src.Entry
	if r.timeouts[src.Height] {
		a.Timeout = true
		return true
	}
	lock, locked := r.locks[src.Height]
	if a.FinalityHeight == src.Height {
		lock, locked = a.FinalityTarget, true
	}
	if locked {
		return lock == src.Entry
	}
	if t, ok := r.targets[src.Height]; ok && t != src.Entry {
		a.Timeout = true
	}
	return true
}

// Rebuilds Λ from the rows this key signed, in the order they were signed.
func (r *Record) Restore(a Attestation) { r.record(a) }

// Spec: record_attestation, the only writer of Λ.
func (r *Record) record(a Attestation) {
	if r.named[a.Height] == nil {
		r.named[a.Height] = make(map[[32]byte]struct{})
	}
	r.named[a.Height][a.Target] = struct{}{}
	if a.FinalityHeight != 0 {
		r.locks[a.FinalityHeight] = a.FinalityTarget
	}
	if a.Timeout {
		r.timeouts[a.Height] = true
		return
	}
	if _, ok := r.targets[a.Height]; !ok {
		r.targets[a.Height] = a.Target
	}
}
