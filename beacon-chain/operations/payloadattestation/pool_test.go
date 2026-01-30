package payloadattestation

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestPool_PendingPayloadAttestations(t *testing.T) {
	t.Run("empty pool", func(t *testing.T) {
		pool := NewPool()
		atts := pool.PendingPayloadAttestations()
		assert.Equal(t, 0, len(atts))
	})

	t.Run("non-empty pool", func(t *testing.T) {
		pool := NewPool()
		sig := bls.NewAggregateSignature().Marshal()
		msg1 := &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 0,
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot:   make([]byte, 32),
				Slot:              1,
				PayloadPresent:    true,
				BlobDataAvailable: false,
			},
			Signature: sig,
		}
		msg2 := &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 1,
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot:   make([]byte, 32),
				Slot:              2,
				PayloadPresent:    false,
				BlobDataAvailable: true,
			},
			Signature: sig,
		}
		require.NoError(t, pool.InsertPayloadAttestation(msg1, 0))
		require.NoError(t, pool.InsertPayloadAttestation(msg2, 1))
		atts := pool.PendingPayloadAttestations()
		assert.Equal(t, 2, len(atts))
	})

	t.Run("filter by slot", func(t *testing.T) {
		pool := NewPool()
		sig := bls.NewAggregateSignature().Marshal()
		msg1 := &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 0,
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot:   make([]byte, 32),
				Slot:              1,
				PayloadPresent:    true,
				BlobDataAvailable: false,
			},
			Signature: sig,
		}
		msg2 := &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 1,
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot:   make([]byte, 32),
				Slot:              2,
				PayloadPresent:    false,
				BlobDataAvailable: true,
			},
			Signature: sig,
		}
		require.NoError(t, pool.InsertPayloadAttestation(msg1, 0))
		require.NoError(t, pool.InsertPayloadAttestation(msg2, 1))

		atts := pool.PendingPayloadAttestations(primitives.Slot(1))
		assert.Equal(t, 1, len(atts))
		assert.Equal(t, primitives.Slot(1), atts[0].Data.Slot)

		atts = pool.PendingPayloadAttestations(primitives.Slot(2))
		assert.Equal(t, 1, len(atts))
		assert.Equal(t, primitives.Slot(2), atts[0].Data.Slot)

		atts = pool.PendingPayloadAttestations(primitives.Slot(99))
		assert.Equal(t, 0, len(atts))
	})
}

func TestPool_InsertPayloadAttestation(t *testing.T) {
	t.Run("nil message", func(t *testing.T) {
		pool := NewPool()
		err := pool.InsertPayloadAttestation(nil, 0)
		require.ErrorContains(t, "nil payload attestation message", err)
	})

	t.Run("nil data", func(t *testing.T) {
		pool := NewPool()
		err := pool.InsertPayloadAttestation(&ethpb.PayloadAttestationMessage{}, 0)
		require.ErrorContains(t, "nil payload attestation message", err)
	})

	t.Run("insert creates new entry with correct aggregation bit", func(t *testing.T) {
		pool := NewPool()
		sig := bls.NewAggregateSignature().Marshal()
		idx := uint64(5)
		msg := &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 0,
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot:   make([]byte, 32),
				Slot:              1,
				PayloadPresent:    true,
				BlobDataAvailable: false,
			},
			Signature: sig,
		}
		require.NoError(t, pool.InsertPayloadAttestation(msg, idx))
		atts := pool.PendingPayloadAttestations()
		require.Equal(t, 1, len(atts))
		assert.Equal(t, true, atts[0].AggregationBits.BitAt(idx))
		assert.Equal(t, false, atts[0].AggregationBits.BitAt(idx+1))
	})

	t.Run("duplicate index is no-op", func(t *testing.T) {
		pool := NewPool()
		sig := bls.NewAggregateSignature().Marshal()
		idx := uint64(3)
		msg := &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 0,
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot:   make([]byte, 32),
				Slot:              1,
				PayloadPresent:    true,
				BlobDataAvailable: false,
			},
			Signature: sig,
		}
		require.NoError(t, pool.InsertPayloadAttestation(msg, idx))
		firstSig := bytesutil.SafeCopyBytes(pool.PendingPayloadAttestations()[0].Signature)

		require.NoError(t, pool.InsertPayloadAttestation(msg, idx))
		atts := pool.PendingPayloadAttestations()
		require.Equal(t, 1, len(atts))
		assert.DeepEqual(t, firstSig, atts[0].Signature)
	})

	t.Run("aggregates different indices", func(t *testing.T) {
		pool := NewPool()
		sig := bls.NewAggregateSignature().Marshal()
		root := make([]byte, 32)
		root[0] = 'r'
		data := &ethpb.PayloadAttestationData{
			BeaconBlockRoot:   root,
			Slot:              1,
			PayloadPresent:    true,
			BlobDataAvailable: false,
		}
		msg1 := &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 0,
			Data:           data,
			Signature:      sig,
		}
		msg2 := &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 1,
			Data:           data,
			Signature:      sig,
		}

		require.NoError(t, pool.InsertPayloadAttestation(msg1, 5))
		require.NoError(t, pool.InsertPayloadAttestation(msg2, 7))

		atts := pool.PendingPayloadAttestations()
		require.Equal(t, 1, len(atts))
		assert.Equal(t, true, atts[0].AggregationBits.BitAt(5))
		assert.Equal(t, true, atts[0].AggregationBits.BitAt(7))
		assert.Equal(t, false, atts[0].AggregationBits.BitAt(6))
	})

	t.Run("different data creates separate entries", func(t *testing.T) {
		pool := NewPool()
		sig := bls.NewAggregateSignature().Marshal()
		msg1 := &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 0,
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot:   make([]byte, 32),
				Slot:              1,
				PayloadPresent:    true,
				BlobDataAvailable: false,
			},
			Signature: sig,
		}
		msg2 := &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 1,
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot:   make([]byte, 32),
				Slot:              1,
				PayloadPresent:    false, // different
				BlobDataAvailable: false,
			},
			Signature: sig,
		}
		require.NoError(t, pool.InsertPayloadAttestation(msg1, 0))
		require.NoError(t, pool.InsertPayloadAttestation(msg2, 1))
		atts := pool.PendingPayloadAttestations()
		assert.Equal(t, 2, len(atts))
	})
}

func TestPool_Seen(t *testing.T) {
	pool := NewPool()
	sig := bls.NewAggregateSignature().Marshal()
	data := &ethpb.PayloadAttestationData{
		BeaconBlockRoot:   make([]byte, 32),
		Slot:              1,
		PayloadPresent:    true,
		BlobDataAvailable: false,
	}

	assert.Equal(t, false, pool.Seen(data, 5))

	msg := &ethpb.PayloadAttestationMessage{
		ValidatorIndex: 0,
		Data:           data,
		Signature:      sig,
	}
	require.NoError(t, pool.InsertPayloadAttestation(msg, 5))

	assert.Equal(t, true, pool.Seen(data, 5))
	assert.Equal(t, false, pool.Seen(data, 6))
	assert.Equal(t, false, pool.Seen(nil, 5))
}

func TestPool_MarkIncluded(t *testing.T) {
	t.Run("mark included removes from pool", func(t *testing.T) {
		pool := NewPool()
		sig := bls.NewAggregateSignature().Marshal()
		msg := &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 0,
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot:   make([]byte, 32),
				Slot:              1,
				PayloadPresent:    true,
				BlobDataAvailable: false,
			},
			Signature: sig,
		}
		require.NoError(t, pool.InsertPayloadAttestation(msg, 0))
		assert.Equal(t, 1, len(pool.PendingPayloadAttestations()))

		pool.MarkIncluded(&ethpb.PayloadAttestation{
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot:   make([]byte, 32),
				Slot:              1,
				PayloadPresent:    true,
				BlobDataAvailable: false,
			},
		})
		assert.Equal(t, 0, len(pool.PendingPayloadAttestations()))
	})

	t.Run("mark included with non-matching data does nothing", func(t *testing.T) {
		pool := NewPool()
		sig := bls.NewAggregateSignature().Marshal()
		msg := &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 0,
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot:   make([]byte, 32),
				Slot:              1,
				PayloadPresent:    true,
				BlobDataAvailable: false,
			},
			Signature: sig,
		}
		require.NoError(t, pool.InsertPayloadAttestation(msg, 0))

		pool.MarkIncluded(&ethpb.PayloadAttestation{
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot:   make([]byte, 32),
				Slot:              999,
				PayloadPresent:    true,
				BlobDataAvailable: false,
			},
		})
		assert.Equal(t, 1, len(pool.PendingPayloadAttestations()))
	})

	t.Run("mark included with nil is safe", func(t *testing.T) {
		pool := NewPool()
		pool.MarkIncluded(nil)
		pool.MarkIncluded(&ethpb.PayloadAttestation{})
	})
}
