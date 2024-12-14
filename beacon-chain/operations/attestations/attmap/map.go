package attmap

import (
	"sync"

	"github.com/pkg/errors"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/attestation"
)

// Attestations --
type Attestations struct {
	atts map[attestation.Id]ethpb.Att
	sync.RWMutex
}

// New --
func New() *Attestations {
	return &Attestations{atts: make(map[attestation.Id]ethpb.Att)}
}

// Save --
func (a *Attestations) Save(att ethpb.Att) error {
	if att == nil || att.IsNil() {
		return nil
	}

	id, err := attestation.NewId(att, attestation.Full)
	if err != nil {
		return errors.Wrap(err, "could not create attestation ID")
	}

	a.Lock()
	defer a.Unlock()
	a.atts[id] = att

	return nil
}

// SaveMany --
func (a *Attestations) SaveMany(atts []ethpb.Att) error {
	for _, att := range atts {
		if err := a.Save(att); err != nil {
			return err
		}
	}

	return nil
}

// GetAll --
func (a *Attestations) GetAll() []ethpb.Att {
	a.RLock()
	defer a.RUnlock()

	atts := make([]ethpb.Att, len(a.atts))
	i := 0
	for _, att := range a.atts {
		atts[i] = att.Clone()
		i++
	}

	return atts
}

// Delete --
func (a *Attestations) Delete(att ethpb.Att) error {
	if att == nil || att.IsNil() {
		return nil
	}

	id, err := attestation.NewId(att, attestation.Full)
	if err != nil {
		return errors.Wrap(err, "could not create attestation ID")
	}

	a.Lock()
	defer a.Unlock()
	delete(a.atts, id)

	return nil
}

// Count --
func (a *Attestations) Count() int {
	a.RLock()
	defer a.RUnlock()
	return len(a.atts)
}
