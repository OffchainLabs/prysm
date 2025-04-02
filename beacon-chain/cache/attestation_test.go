package cache

import (
	"testing"

	"github.com/prysmaticlabs/go-bitfield"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls/blst"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/attestation"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestAdd(t *testing.T) {
	k, err := blst.RandKey()
	require.NoError(t, err)
	sig := k.Sign([]byte{'X'})

	t.Run("new ID", func(t *testing.T) {
		t.Run("first ID ever", func(t *testing.T) {
			c := NewAttestationCache()
			ab := bitfield.NewBitlist(8)
			ab.SetBitAt(0, true)
			att := &ethpb.Attestation{
				Data:            &ethpb.AttestationData{Slot: 123, BeaconBlockRoot: make([]byte, 32), Source: &ethpb.Checkpoint{Root: make([]byte, 32)}, Target: &ethpb.Checkpoint{Root: make([]byte, 32)}},
				AggregationBits: ab,
				Signature:       sig.Marshal(),
			}
			id, err := attestation.NewId(att, attestation.Data)
			require.NoError(t, err)
			require.NoError(t, c.Add(att))

			require.Equal(t, 1, len(c.atts))
			group, ok := c.atts[id]
			require.Equal(t, true, ok)
			assert.Equal(t, primitives.Slot(123), group.slot)
			require.Equal(t, 1, len(group.atts))
			assert.DeepEqual(t, group.atts[0], att)
		})
		t.Run("other ID exists", func(t *testing.T) {
			c := NewAttestationCache()
			ab := bitfield.NewBitlist(8)
			ab.SetBitAt(0, true)
			existingAtt := &ethpb.Attestation{
				Data:            &ethpb.AttestationData{BeaconBlockRoot: make([]byte, 32), Source: &ethpb.Checkpoint{Root: make([]byte, 32)}, Target: &ethpb.Checkpoint{Root: make([]byte, 32)}},
				AggregationBits: ab,
				Signature:       sig.Marshal(),
			}
			existingId, err := attestation.NewId(existingAtt, attestation.Data)
			require.NoError(t, err)
			c.atts[existingId] = &attGroup{slot: existingAtt.Data.Slot, atts: []ethpb.Att{existingAtt}}

			att := &ethpb.Attestation{
				Data:            &ethpb.AttestationData{Slot: 123, BeaconBlockRoot: make([]byte, 32), Source: &ethpb.Checkpoint{Root: make([]byte, 32)}, Target: &ethpb.Checkpoint{Root: make([]byte, 32)}},
				AggregationBits: ab,
				Signature:       sig.Marshal(),
			}
			id, err := attestation.NewId(att, attestation.Data)
			require.NoError(t, err)
			require.NoError(t, c.Add(att))

			require.Equal(t, 2, len(c.atts))
			group, ok := c.atts[id]
			require.Equal(t, true, ok)
			assert.Equal(t, primitives.Slot(123), group.slot)
			require.Equal(t, 1, len(group.atts))
			assert.DeepEqual(t, group.atts[0], att)
		})
	})
	t.Run("aggregated", func(t *testing.T) {
		c := NewAttestationCache()
		existingAtt := &ethpb.Attestation{
			Data:            &ethpb.AttestationData{Slot: 123, BeaconBlockRoot: make([]byte, 32), Source: &ethpb.Checkpoint{Root: make([]byte, 32)}, Target: &ethpb.Checkpoint{Root: make([]byte, 32)}},
			AggregationBits: bitfield.NewBitlist(8),
			Signature:       sig.Marshal(),
		}
		id, err := attestation.NewId(existingAtt, attestation.Data)
		require.NoError(t, err)
		c.atts[id] = &attGroup{slot: existingAtt.Data.Slot, atts: []ethpb.Att{existingAtt}}

		att := &ethpb.Attestation{
			Data:            &ethpb.AttestationData{Slot: 123, BeaconBlockRoot: make([]byte, 32), Source: &ethpb.Checkpoint{Root: make([]byte, 32)}, Target: &ethpb.Checkpoint{Root: make([]byte, 32)}},
			AggregationBits: bitfield.NewBitlist(8),
			Signature:       sig.Marshal(),
		}
		att.AggregationBits.SetBitAt(0, true)
		att.AggregationBits.SetBitAt(1, true)
		require.NoError(t, c.Add(att))

		require.Equal(t, 1, len(c.atts))
		group, ok := c.atts[id]
		require.Equal(t, true, ok)
		assert.Equal(t, primitives.Slot(123), group.slot)
		require.Equal(t, 2, len(group.atts))
		assert.DeepEqual(t, group.atts[0], existingAtt)
		assert.DeepEqual(t, group.atts[1], att)
	})
	t.Run("unaggregated - existing bit", func(t *testing.T) {
		c := NewAttestationCache()
		existingAtt := &ethpb.Attestation{
			Data:            &ethpb.AttestationData{Slot: 123, BeaconBlockRoot: make([]byte, 32), Source: &ethpb.Checkpoint{Root: make([]byte, 32)}, Target: &ethpb.Checkpoint{Root: make([]byte, 32)}},
			AggregationBits: bitfield.NewBitlist(8),
			Signature:       sig.Marshal(),
		}
		existingAtt.AggregationBits.SetBitAt(0, true)
		id, err := attestation.NewId(existingAtt, attestation.Data)
		require.NoError(t, err)
		c.atts[id] = &attGroup{slot: existingAtt.Data.Slot, atts: []ethpb.Att{existingAtt}}

		att := &ethpb.Attestation{
			Data:            &ethpb.AttestationData{Slot: 123, BeaconBlockRoot: make([]byte, 32), Source: &ethpb.Checkpoint{Root: make([]byte, 32)}, Target: &ethpb.Checkpoint{Root: make([]byte, 32)}},
			AggregationBits: bitfield.NewBitlist(8),
			Signature:       sig.Marshal(),
		}
		att.AggregationBits.SetBitAt(0, true)
		require.NoError(t, c.Add(att))

		require.Equal(t, 1, len(c.atts))
		group, ok := c.atts[id]
		require.Equal(t, true, ok)
		assert.Equal(t, primitives.Slot(123), group.slot)
		require.Equal(t, 1, len(group.atts))
		assert.DeepEqual(t, []int{0}, group.atts[0].GetAggregationBits().BitIndices())
	})
	t.Run("unaggregated - new bit", func(t *testing.T) {
		c := NewAttestationCache()
		existingAtt := &ethpb.Attestation{
			Data:            &ethpb.AttestationData{Slot: 123, BeaconBlockRoot: make([]byte, 32), Source: &ethpb.Checkpoint{Root: make([]byte, 32)}, Target: &ethpb.Checkpoint{Root: make([]byte, 32)}},
			AggregationBits: bitfield.NewBitlist(8),
			Signature:       sig.Marshal(),
		}
		existingAtt.AggregationBits.SetBitAt(0, true)
		id, err := attestation.NewId(existingAtt, attestation.Data)
		require.NoError(t, err)
		c.atts[id] = &attGroup{slot: existingAtt.Data.Slot, atts: []ethpb.Att{existingAtt}}

		att := &ethpb.Attestation{
			Data:            &ethpb.AttestationData{Slot: 123, BeaconBlockRoot: make([]byte, 32), Source: &ethpb.Checkpoint{Root: make([]byte, 32)}, Target: &ethpb.Checkpoint{Root: make([]byte, 32)}},
			AggregationBits: bitfield.NewBitlist(8),
			Signature:       sig.Marshal(),
		}
		att.AggregationBits.SetBitAt(1, true)
		require.NoError(t, c.Add(att))

		require.Equal(t, 1, len(c.atts))
		group, ok := c.atts[id]
		require.Equal(t, true, ok)
		assert.Equal(t, primitives.Slot(123), group.slot)
		require.Equal(t, 1, len(group.atts))
		assert.DeepEqual(t, []int{0, 1}, group.atts[0].GetAggregationBits().BitIndices())
	})
}

func TestGetAll(t *testing.T) {
	c := NewAttestationCache()
	c.atts[bytesutil.ToBytes32([]byte("id1"))] = &attGroup{atts: []ethpb.Att{&ethpb.Attestation{}, &ethpb.Attestation{}}}
	c.atts[bytesutil.ToBytes32([]byte("id2"))] = &attGroup{atts: []ethpb.Att{&ethpb.Attestation{}}}

	assert.Equal(t, 3, len(c.GetAll()))
}

func TestCount(t *testing.T) {
	c := NewAttestationCache()
	c.atts[bytesutil.ToBytes32([]byte("id1"))] = &attGroup{atts: []ethpb.Att{&ethpb.Attestation{}, &ethpb.Attestation{}}}
	c.atts[bytesutil.ToBytes32([]byte("id2"))] = &attGroup{atts: []ethpb.Att{&ethpb.Attestation{}}}

	assert.Equal(t, 3, c.Count())
}

func TestDeleteCovered(t *testing.T) {
	k, err := blst.RandKey()
	require.NoError(t, err)
	sig := k.Sign([]byte{'X'})

	att1 := &ethpb.Attestation{
		Data:            &ethpb.AttestationData{Slot: 123, BeaconBlockRoot: make([]byte, 32), Source: &ethpb.Checkpoint{Root: make([]byte, 32)}, Target: &ethpb.Checkpoint{Root: make([]byte, 32)}},
		AggregationBits: bitfield.NewBitlist(8),
		Signature:       sig.Marshal(),
	}
	att1.AggregationBits.SetBitAt(0, true)

	att2 := &ethpb.Attestation{
		Data:            &ethpb.AttestationData{Slot: 123, BeaconBlockRoot: make([]byte, 32), Source: &ethpb.Checkpoint{Root: make([]byte, 32)}, Target: &ethpb.Checkpoint{Root: make([]byte, 32)}},
		AggregationBits: bitfield.NewBitlist(8),
		Signature:       sig.Marshal(),
	}
	att2.AggregationBits.SetBitAt(1, true)
	att2.AggregationBits.SetBitAt(2, true)

	att3 := &ethpb.Attestation{
		Data:            &ethpb.AttestationData{Slot: 123, BeaconBlockRoot: make([]byte, 32), Source: &ethpb.Checkpoint{Root: make([]byte, 32)}, Target: &ethpb.Checkpoint{Root: make([]byte, 32)}},
		AggregationBits: bitfield.NewBitlist(8),
		Signature:       sig.Marshal(),
	}
	att3.AggregationBits.SetBitAt(1, true)
	att3.AggregationBits.SetBitAt(3, true)
	att3.AggregationBits.SetBitAt(4, true)

	c := NewAttestationCache()
	id, err := attestation.NewId(att1, attestation.Data)
	require.NoError(t, err)
	c.atts[id] = &attGroup{slot: att1.Data.Slot, atts: []ethpb.Att{att1, att2, att3}}

	t.Run("no matching group", func(t *testing.T) {
		att := &ethpb.Attestation{
			Data:            &ethpb.AttestationData{Slot: 456, BeaconBlockRoot: make([]byte, 32), Source: &ethpb.Checkpoint{Root: make([]byte, 32)}, Target: &ethpb.Checkpoint{Root: make([]byte, 32)}},
			AggregationBits: bitfield.NewBitlist(8),
			Signature:       sig.Marshal(),
		}
		att.AggregationBits.SetBitAt(0, true)
		att.AggregationBits.SetBitAt(1, true)
		att.AggregationBits.SetBitAt(2, true)
		att.AggregationBits.SetBitAt(3, true)
		att.AggregationBits.SetBitAt(4, true)
		require.NoError(t, c.DeleteCovered(att))

		assert.Equal(t, 3, len(c.atts[id].atts))
	})
	t.Run("covered atts deleted", func(t *testing.T) {
		att := &ethpb.Attestation{
			Data:            &ethpb.AttestationData{Slot: 123, BeaconBlockRoot: make([]byte, 32), Source: &ethpb.Checkpoint{Root: make([]byte, 32)}, Target: &ethpb.Checkpoint{Root: make([]byte, 32)}},
			AggregationBits: bitfield.NewBitlist(8),
			Signature:       sig.Marshal(),
		}
		att.AggregationBits.SetBitAt(0, true)
		att.AggregationBits.SetBitAt(1, true)
		att.AggregationBits.SetBitAt(3, true)
		att.AggregationBits.SetBitAt(4, true)
		require.NoError(t, c.DeleteCovered(att))

		atts := c.atts[id].atts
		require.Equal(t, 1, len(atts))
		assert.DeepEqual(t, att2, atts[0])
	})
	t.Run("last att in group deleted", func(t *testing.T) {
		att := &ethpb.Attestation{
			Data:            &ethpb.AttestationData{Slot: 123, BeaconBlockRoot: make([]byte, 32), Source: &ethpb.Checkpoint{Root: make([]byte, 32)}, Target: &ethpb.Checkpoint{Root: make([]byte, 32)}},
			AggregationBits: bitfield.NewBitlist(8),
			Signature:       sig.Marshal(),
		}
		att.AggregationBits.SetBitAt(0, true)
		att.AggregationBits.SetBitAt(1, true)
		att.AggregationBits.SetBitAt(2, true)
		att.AggregationBits.SetBitAt(3, true)
		att.AggregationBits.SetBitAt(4, true)
		require.NoError(t, c.DeleteCovered(att))

		assert.Equal(t, 0, len(c.atts))
	})
}

func TestPruneBefore(t *testing.T) {
	c := NewAttestationCache()
	c.atts[bytesutil.ToBytes32([]byte("id1"))] = &attGroup{slot: 1, atts: []ethpb.Att{&ethpb.Attestation{}, &ethpb.Attestation{}}}
	c.atts[bytesutil.ToBytes32([]byte("id2"))] = &attGroup{slot: 3, atts: []ethpb.Att{&ethpb.Attestation{}}}
	c.atts[bytesutil.ToBytes32([]byte("id3"))] = &attGroup{slot: 2, atts: []ethpb.Att{&ethpb.Attestation{}}}

	count := c.PruneBefore(3)

	require.Equal(t, 1, len(c.atts))
	_, ok := c.atts[bytesutil.ToBytes32([]byte("id2"))]
	assert.Equal(t, true, ok)
	assert.Equal(t, uint64(3), count)
}

func TestAggregateIsRedundant(t *testing.T) {
	k, err := blst.RandKey()
	require.NoError(t, err)
	sig := k.Sign([]byte{'X'})

	c := NewAttestationCache()
	existingAtt := &ethpb.Attestation{
		Data:            &ethpb.AttestationData{Slot: 123, BeaconBlockRoot: make([]byte, 32), Source: &ethpb.Checkpoint{Root: make([]byte, 32)}, Target: &ethpb.Checkpoint{Root: make([]byte, 32)}},
		AggregationBits: bitfield.NewBitlist(8),
		Signature:       sig.Marshal(),
	}
	existingAtt.AggregationBits.SetBitAt(0, true)
	existingAtt.AggregationBits.SetBitAt(1, true)
	id, err := attestation.NewId(existingAtt, attestation.Data)
	require.NoError(t, err)
	c.atts[id] = &attGroup{slot: existingAtt.Data.Slot, atts: []ethpb.Att{existingAtt}}

	t.Run("no matching group", func(t *testing.T) {
		att := &ethpb.Attestation{
			Data:            &ethpb.AttestationData{Slot: 456, BeaconBlockRoot: make([]byte, 32), Source: &ethpb.Checkpoint{Root: make([]byte, 32)}, Target: &ethpb.Checkpoint{Root: make([]byte, 32)}},
			AggregationBits: bitfield.NewBitlist(8),
			Signature:       sig.Marshal(),
		}
		att.AggregationBits.SetBitAt(0, true)

		redundant, err := c.AggregateIsRedundant(att)
		require.NoError(t, err)
		assert.Equal(t, false, redundant)
	})
	t.Run("redundant", func(t *testing.T) {
		att := &ethpb.Attestation{
			Data:            &ethpb.AttestationData{Slot: existingAtt.Data.Slot, BeaconBlockRoot: make([]byte, 32), Source: &ethpb.Checkpoint{Root: make([]byte, 32)}, Target: &ethpb.Checkpoint{Root: make([]byte, 32)}},
			AggregationBits: bitfield.NewBitlist(8),
			Signature:       sig.Marshal(),
		}
		att.AggregationBits.SetBitAt(0, true)

		redundant, err := c.AggregateIsRedundant(att)
		require.NoError(t, err)
		assert.Equal(t, true, redundant)
	})
	t.Run("not redundant", func(t *testing.T) {
		t.Run("strictly better", func(t *testing.T) {
			att := &ethpb.Attestation{
				Data:            &ethpb.AttestationData{Slot: existingAtt.Data.Slot, BeaconBlockRoot: make([]byte, 32), Source: &ethpb.Checkpoint{Root: make([]byte, 32)}, Target: &ethpb.Checkpoint{Root: make([]byte, 32)}},
				AggregationBits: bitfield.NewBitlist(8),
				Signature:       sig.Marshal(),
			}
			att.AggregationBits.SetBitAt(0, true)
			att.AggregationBits.SetBitAt(1, true)
			att.AggregationBits.SetBitAt(2, true)

			redundant, err := c.AggregateIsRedundant(att)
			require.NoError(t, err)
			assert.Equal(t, false, redundant)
		})
		t.Run("overlapping and new bits", func(t *testing.T) {
			att := &ethpb.Attestation{
				Data:            &ethpb.AttestationData{Slot: existingAtt.Data.Slot, BeaconBlockRoot: make([]byte, 32), Source: &ethpb.Checkpoint{Root: make([]byte, 32)}, Target: &ethpb.Checkpoint{Root: make([]byte, 32)}},
				AggregationBits: bitfield.NewBitlist(8),
				Signature:       sig.Marshal(),
			}
			att.AggregationBits.SetBitAt(0, true)
			att.AggregationBits.SetBitAt(2, true)

			redundant, err := c.AggregateIsRedundant(att)
			require.NoError(t, err)
			assert.Equal(t, false, redundant)
		})
	})
}

func TestGetBySlotAndCommitteeIndex(t *testing.T) {
	c := NewAttestationCache()
	c.atts[bytesutil.ToBytes32([]byte("id1"))] = &attGroup{slot: 1, atts: []ethpb.Att{&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1, CommitteeIndex: 1}}, &ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 1, CommitteeIndex: 1}}}}
	c.atts[bytesutil.ToBytes32([]byte("id2"))] = &attGroup{slot: 2, atts: []ethpb.Att{&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 2, CommitteeIndex: 2}}}}
	c.atts[bytesutil.ToBytes32([]byte("id3"))] = &attGroup{slot: 1, atts: []ethpb.Att{&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 2, CommitteeIndex: 2}}}}

	// committeeIndex has to be small enough to fit in the bitvector
	atts := GetBySlotAndCommitteeIndex[*ethpb.Attestation](c, 1, 1)
	require.Equal(t, 2, len(atts))
	assert.Equal(t, primitives.Slot(1), atts[0].Data.Slot)
	assert.Equal(t, primitives.Slot(1), atts[1].Data.Slot)
	assert.Equal(t, primitives.CommitteeIndex(1), atts[0].Data.CommitteeIndex)
	assert.Equal(t, primitives.CommitteeIndex(1), atts[1].Data.CommitteeIndex)
}
