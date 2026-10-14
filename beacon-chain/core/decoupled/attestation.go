package decoupled

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// Spec: valid_attestation for every row against the parent σ, then process_attestation. Signatures are checked upstream.
func ProcessAttestations(st state.BeaconState, atts []*ethpb.IndexedAttestationDecoupled) error {
	if len(atts) == 0 {
		return nil
	}
	height, err := st.Height()
	if err != nil {
		return err
	}
	entry, err := st.Entry()
	if err != nil {
		return err
	}
	pending, err := st.PendingFinality()
	if err != nil {
		return err
	}
	for _, a := range atts {
		if a.Data == nil {
			return errors.New("nil attestation data")
		}
		if !valid(a.Data, height, entry.Root, pending) {
			return fmt.Errorf("attestation for height %d is not includable at height %d", a.Data.Height, height)
		}
	}
	target, err := st.TargetParticipation()
	if err != nil {
		return err
	}
	progress, err := st.Progress()
	if err != nil {
		return err
	}
	target = bytesutil.SafeCopyBytes(target)
	progress = bytesutil.SafeCopyBytes(progress)
	finality := make([]bitfield.Bitlist, len(pending))
	n := target.Len()
	for _, a := range atts {
		d := a.Data
		var bits bitfield.Bitlist
		if i := pendingIndex(pending, d.FinalityHeight, d.FinalityTarget); i >= 0 {
			if finality[i] == nil {
				finality[i] = bytesutil.SafeCopyBytes(pending[i].Participation)
			}
			bits = finality[i]
		}
		current := d.Height == height && bytes.Equal(d.Target, entry.Root)
		for _, v := range a.AttestingIndices {
			if v >= n {
				return fmt.Errorf("validator %d beyond participation length %d", v, n)
			}
			if bits != nil {
				bits.SetBitAt(v, true)
			}
			if current {
				progress.SetBitAt(v, true)
				if !d.Timeout {
					target.SetBitAt(v, true)
				}
			}
		}
	}
	if err := st.SetTargetParticipation(target); err != nil {
		return err
	}
	if err := st.SetProgress(progress); err != nil {
		return err
	}
	updated := make([]*ethpb.PendingFinality, len(pending))
	touched := false
	for i, p := range pending {
		updated[i] = p
		if finality[i] != nil {
			updated[i] = &ethpb.PendingFinality{Height: p.Height, Target: p.Target, Participation: finality[i]}
			touched = true
		}
	}
	if !touched {
		return nil
	}
	return st.SetPendingFinality(updated)
}

func valid(d *ethpb.AttestationDataDecoupled, height primitives.Height, entry []byte, pending []*ethpb.PendingFinality) bool {
	if d.Height == height && bytes.Equal(d.Target, entry) {
		return true
	}
	return pendingIndex(pending, d.FinalityHeight, d.FinalityTarget) >= 0 && d.FinalityHeight < d.Height && d.Height < height
}

func pendingIndex(pending []*ethpb.PendingFinality, height primitives.Height, root []byte) int {
	for i, p := range pending {
		if p.Height == height && bytes.Equal(p.Target.Root, root) {
			return i
		}
	}
	return -1
}
