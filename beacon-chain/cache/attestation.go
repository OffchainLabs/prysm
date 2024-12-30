package cache

import (
	"sync"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/operations/attestations/attmap"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/attestation"
	log "github.com/sirupsen/logrus"
)

type attGroup struct {
	slot primitives.Slot
	atts []ethpb.Att
}

// AttestationCache holds a map of attGroup items that group together all attestations for a single slot.
// When we add an attestation to the cache by calling Add, we either create a new group with this attestation
// (if this is the first attestation for some slot) or two things can happen:
//
//   - If the attestation is unaggregated, we add its attestation bit to attestation bits of the first
//     attestation in the group.
//   - If the attestation is aggregated, we append it to the group. There should be no redundancy
//     in the list because we ignore redundant aggregates in gossip.
//
// The first bullet point above means that we keep one aggregate attestation to which we keep appending bits
// as new single-bit attestations arrive. This means that at any point during seconds 0-4 of a slot
// we will have only one attestation for this slot in the cache.
//
// NOTE: This design in principle can result in worse aggregates since we lose the ability to aggregate some
// single bit attestations in case of overlaps with incoming aggregates.
//
// The cache also keeps forkchoice attestations in a separate struct. These attestations are used for
// forkchoice-related operations.
type AttestationCache struct {
	atts map[attestation.Id]*attGroup
	sync.RWMutex
	forkchoiceAtts *attmap.Attestations
}

// NewAttestationCache creates a new cache instance.
func NewAttestationCache() *AttestationCache {
	return &AttestationCache{
		atts:           make(map[attestation.Id]*attGroup),
		forkchoiceAtts: attmap.New(),
	}
}

// Add does one of two things:
//
//   - For unaggregated attestations, it adds the attestation bit to attestation bits of the running aggregate,
//     which is the first aggregate for the slot.
//   - For aggregated attestations, it appends the attestation to the existng list of attestations for the slot.
func (c *AttestationCache) Add(att ethpb.Att) error {
	if att.IsNil() {
		log.Debug("Attempted to add a nil attestation to the attestation cache")
		return nil
	}
	if len(att.GetAggregationBits().BitIndices()) == 0 {
		log.Debug("Attempted to add an attestation with 0 bits set to the attestation cache")
		return nil
	}

	c.Lock()
	defer c.Unlock()

	id, err := attestation.NewId(att, attestation.Data)
	if err != nil {
		return errors.Wrapf(err, "could not create attestation ID")
	}

	group := c.atts[id]
	if group == nil {
		group = &attGroup{
			slot: att.GetData().Slot,
			atts: []ethpb.Att{att},
		}
		c.atts[id] = group
		return nil
	}

	if att.IsAggregated() {
		group.atts = append(group.atts, att.Clone())
		return nil
	}

	// This should never happen because we return early for a new group.
	if len(group.atts) == 0 {
		log.Error("Attestation group contains no attestations, skipping insertion")
		return nil
	}

	a := group.atts[0]

	// Indexing is safe because we have guarded against 0 bits set.
	bit := att.GetAggregationBits().BitIndices()[0]
	if a.GetAggregationBits().BitAt(uint64(bit)) {
		return nil
	}
	sig, err := aggregateSig(a, att)
	if err != nil {
		return errors.Wrapf(err, "could not aggregate signatures")
	}

	a.GetAggregationBits().SetBitAt(uint64(bit), true)
	a.SetSignature(sig)

	return nil
}

// GetAll returns all attestations in the cache, excluding forkchoice attestations.
func (c *AttestationCache) GetAll() []ethpb.Att {
	c.RLock()
	defer c.RUnlock()

	var result []ethpb.Att
	for _, group := range c.atts {
		result = append(result, group.atts...)
	}
	return result
}

// Count returns the number of all attestations in the cache, excluding forkchoice attestations.
func (c *AttestationCache) Count() int {
	c.RLock()
	defer c.RUnlock()

	count := 0
	for _, group := range c.atts {
		count += len(group.atts)
	}
	return count
}

// DeleteCovered removes all attestations whose attestation bits are a proper subset of the passed-in attestation.
func (c *AttestationCache) DeleteCovered(att ethpb.Att) error {
	if att.IsNil() {
		return nil
	}

	c.Lock()
	defer c.Unlock()

	id, err := attestation.NewId(att, attestation.Data)
	if err != nil {
		return errors.Wrapf(err, "could not create attestation ID")
	}

	group := c.atts[id]
	if group == nil {
		return nil
	}

	idx := 0
	for _, a := range group.atts {
		if covered, err := att.GetAggregationBits().Contains(a.GetAggregationBits()); err != nil {
			return err
		} else if !covered {
			group.atts[idx] = a
			idx++
		}
	}
	group.atts = group.atts[:idx]

	if len(group.atts) == 0 {
		delete(c.atts, id)
	}

	return nil
}

// PruneBefore removes all attestations whose slot is earlier than the passed-in slot.
func (c *AttestationCache) PruneBefore(slot primitives.Slot) uint64 {
	c.Lock()
	defer c.Unlock()

	var pruneCount int
	for id, group := range c.atts {
		if group.slot < slot {
			pruneCount += len(group.atts)
			delete(c.atts, id)
		}
	}
	return uint64(pruneCount)
}

// AggregateIsRedundant checks whether all attestation bits of the passed-in aggregate
// are already included by any aggregate in the cache.
func (c *AttestationCache) AggregateIsRedundant(att ethpb.Att) (bool, error) {
	if att.IsNil() {
		return true, nil
	}

	c.RLock()
	defer c.RUnlock()

	id, err := attestation.NewId(att, attestation.Data)
	if err != nil {
		return true, errors.Wrapf(err, "could not create attestation ID")
	}

	group := c.atts[id]
	if group == nil {
		return false, nil
	}

	for _, a := range group.atts {
		if redundant, err := a.GetAggregationBits().Contains(att.GetAggregationBits()); err != nil {
			return true, err
		} else if redundant {
			return true, nil
		}
	}

	return false, nil
}

// SaveForkchoiceAttestations saves forkchoice attestations.
func (c *AttestationCache) SaveForkchoiceAttestations(att []ethpb.Att) error {
	return c.forkchoiceAtts.SaveMany(att)
}

// ForkchoiceAttestations returns all forkchoice attestations.
func (c *AttestationCache) ForkchoiceAttestations() []ethpb.Att {
	return c.forkchoiceAtts.GetAll()
}

// DeleteForkchoiceAttestation deletes a forkchoice attestation.
func (c *AttestationCache) DeleteForkchoiceAttestation(att ethpb.Att) error {
	return c.forkchoiceAtts.Delete(att)
}

// GetBySlotAndCommitteeIndex returns all attestations in the cache that match the provided slot
// and committee index. Forkchoice attestations are not returned.
//
// NOTE: This function cannot be declared as a method on the AttestationCache because it is a generic function.
func GetBySlotAndCommitteeIndex[T ethpb.Att](c *AttestationCache, slot primitives.Slot, committeeIndex primitives.CommitteeIndex) []T {
	c.RLock()
	defer c.RUnlock()

	var result []T

	for _, group := range c.atts {
		if len(group.atts) > 0 {
			// We can safely compare the first attestation because all attestations in a group
			// must have the same slot and committee index, since they are under the same key.
			a, ok := group.atts[0].(T)
			if ok && a.GetData().Slot == slot && a.CommitteeBitsVal().BitAt(uint64(committeeIndex)) {
				for _, a := range group.atts {
					a, ok := a.(T)
					if ok {
						result = append(result, a)
					}
				}
			}
		}
	}

	return result
}

func aggregateSig(agg ethpb.Att, att ethpb.Att) ([]byte, error) {
	aggSig, err := bls.SignatureFromBytesNoValidation(agg.GetSignature())
	if err != nil {
		return nil, err
	}
	attSig, err := bls.SignatureFromBytesNoValidation(att.GetSignature())
	if err != nil {
		return nil, err
	}
	return bls.AggregateSignatures([]bls.Signature{aggSig, attSig}).Marshal(), nil
}
