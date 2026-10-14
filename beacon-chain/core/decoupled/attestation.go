package decoupled

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

var errNilAttestationData = errors.New("nil attestation data")

// Spec: the σ fields process_attestation writes, copied once so block rows and proposer packing apply to one working view.
type Participation struct {
	height           primitives.Height
	entry            []byte
	pending          []*ethpb.PendingFinality
	target, progress bitfield.Bitlist
	finality         []bitfield.Bitlist
}

func NewParticipation(st state.ReadOnlyBeaconState) (*Participation, error) {
	height, err := st.Height()
	if err != nil {
		return nil, err
	}
	entry, err := st.Entry()
	if err != nil {
		return nil, err
	}
	pending, err := st.PendingFinality()
	if err != nil {
		return nil, err
	}
	target, err := st.TargetParticipation()
	if err != nil {
		return nil, err
	}
	progress, err := st.Progress()
	if err != nil {
		return nil, err
	}
	return &Participation{
		height:   height,
		entry:    entry.Root,
		pending:  pending,
		target:   bytesutil.SafeCopyBytes(target),
		progress: bytesutil.SafeCopyBytes(progress),
		finality: make([]bitfield.Bitlist, len(pending)),
	}, nil
}

// Spec: valid_attestation.
func (p *Participation) Valid(d *ethpb.AttestationDataDecoupled) bool {
	if d.Height == p.height && bytes.Equal(d.Target, p.entry) {
		return true
	}
	return pendingIndex(p.pending, d.FinalityHeight, d.FinalityTarget) >= 0 && d.FinalityHeight < d.Height && d.Height < p.height
}

// Spec: process_attestation. Reports whether any bit was newly set, the proposer's inclusion test.
func (p *Participation) Add(a *ethpb.IndexedAttestationDecoupled) (bool, error) {
	d := a.Data
	var bits bitfield.Bitlist
	if i := pendingIndex(p.pending, d.FinalityHeight, d.FinalityTarget); i >= 0 {
		if p.finality[i] == nil {
			p.finality[i] = bytesutil.SafeCopyBytes(p.pending[i].Participation)
		}
		bits = p.finality[i]
	}
	n := p.target.Len()
	if uint64(len(a.AttestingIndices)) > n {
		return false, fmt.Errorf("%d attesting indices for %d validators", len(a.AttestingIndices), n)
	}
	for i, v := range a.AttestingIndices {
		if v >= n {
			return false, fmt.Errorf("validator %d beyond participation length %d", v, n)
		}
		if i > 0 && v <= a.AttestingIndices[i-1] {
			return false, fmt.Errorf("attesting indices not strictly increasing at %d", i)
		}
	}
	current := d.Height == p.height && bytes.Equal(d.Target, p.entry)
	changed := false
	for _, v := range a.AttestingIndices {
		if bits != nil && !bits.BitAt(v) {
			bits.SetBitAt(v, true)
			changed = true
		}
		if !current {
			continue
		}
		if !p.progress.BitAt(v) {
			p.progress.SetBitAt(v, true)
			changed = true
		}
		if !d.Timeout && !p.target.BitAt(v) {
			p.target.SetBitAt(v, true)
			changed = true
		}
	}
	return changed, nil
}

func (p *Participation) commit(st state.BeaconState) error {
	if err := st.SetTargetParticipation(p.target); err != nil {
		return err
	}
	if err := st.SetProgress(p.progress); err != nil {
		return err
	}
	updated := make([]*ethpb.PendingFinality, len(p.pending))
	touched := false
	for i, pf := range p.pending {
		updated[i] = pf
		if p.finality[i] != nil {
			updated[i] = &ethpb.PendingFinality{Height: pf.Height, Target: pf.Target, Participation: p.finality[i]}
			touched = true
		}
	}
	if !touched {
		return nil
	}
	return st.SetPendingFinality(updated)
}

// Spec: valid_attestation for every row against the parent σ, then process_attestation. Signatures are checked upstream. A useful aggregate sets a fresh bit in one of K_W + 2 bitlists, so more rows than that is spam.
func ProcessAttestations(st state.BeaconState, atts []*ethpb.IndexedAttestationDecoupled) error {
	if len(atts) == 0 {
		return nil
	}
	if limit := uint64(fieldparams.MaxPendingFinality+2) * uint64(st.NumValidators()); uint64(len(atts)) > limit {
		return fmt.Errorf("%d attestations exceed the cap of %d", len(atts), limit)
	}
	p, err := NewParticipation(st)
	if err != nil {
		return err
	}
	for _, a := range atts {
		if err := ValidateFinalityShape(a.Data); err != nil {
			return err
		}
		if !p.Valid(a.Data) {
			return fmt.Errorf("attestation for height %d is not includable at height %d", a.Data.Height, p.height)
		}
	}
	for _, a := range atts {
		if _, err := p.Add(a); err != nil {
			return err
		}
	}
	return p.commit(st)
}

func pendingIndex(pending []*ethpb.PendingFinality, height primitives.Height, root []byte) int {
	for i, p := range pending {
		if p.Height == height && bytes.Equal(p.Target.Root, root) {
			return i
		}
	}
	return -1
}
