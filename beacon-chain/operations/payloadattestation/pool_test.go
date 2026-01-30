package payloadattestation

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
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
		msg1 := &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 0,
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot:   make([]byte, 32),
				Slot:              1,
				PayloadPresent:    true,
				BlobDataAvailable: false,
			},
			Signature: make([]byte, 96),
		}
		msg2 := &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 1,
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot:   make([]byte, 32),
				Slot:              2,
				PayloadPresent:    false,
				BlobDataAvailable: true,
			},
			Signature: make([]byte, 96),
		}
		require.NoError(t, pool.InsertPayloadAttestation(msg1))
		require.NoError(t, pool.InsertPayloadAttestation(msg2))
		atts := pool.PendingPayloadAttestations()
		assert.Equal(t, 2, len(atts))
	})

	t.Run("filter by slot", func(t *testing.T) {
		pool := NewPool()
		msg1 := &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 0,
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot:   make([]byte, 32),
				Slot:              1,
				PayloadPresent:    true,
				BlobDataAvailable: false,
			},
			Signature: make([]byte, 96),
		}
		msg2 := &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 1,
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot:   make([]byte, 32),
				Slot:              2,
				PayloadPresent:    false,
				BlobDataAvailable: true,
			},
			Signature: make([]byte, 96),
		}
		require.NoError(t, pool.InsertPayloadAttestation(msg1))
		require.NoError(t, pool.InsertPayloadAttestation(msg2))

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
		err := pool.InsertPayloadAttestation(nil)
		require.ErrorContains(t, "nil payload attestation message", err)
	})

	t.Run("nil data", func(t *testing.T) {
		pool := NewPool()
		err := pool.InsertPayloadAttestation(&ethpb.PayloadAttestationMessage{})
		require.ErrorContains(t, "nil payload attestation message", err)
	})

	t.Run("insert creates new entry", func(t *testing.T) {
		pool := NewPool()
		msg := &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 0,
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot:   make([]byte, 32),
				Slot:              1,
				PayloadPresent:    true,
				BlobDataAvailable: false,
			},
			Signature: make([]byte, 96),
		}
		require.NoError(t, pool.InsertPayloadAttestation(msg))
		atts := pool.PendingPayloadAttestations()
		assert.Equal(t, 1, len(atts))
	})

	t.Run("duplicate data does not create second entry", func(t *testing.T) {
		pool := NewPool()
		msg1 := &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 0,
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot:   make([]byte, 32),
				Slot:              1,
				PayloadPresent:    true,
				BlobDataAvailable: false,
			},
			Signature: make([]byte, 96),
		}
		msg2 := &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 1,
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot:   make([]byte, 32),
				Slot:              1,
				PayloadPresent:    true,
				BlobDataAvailable: false,
			},
			Signature: make([]byte, 96),
		}
		require.NoError(t, pool.InsertPayloadAttestation(msg1))
		require.NoError(t, pool.InsertPayloadAttestation(msg2))
		atts := pool.PendingPayloadAttestations()
		assert.Equal(t, 1, len(atts))
	})
}

func TestPool_MarkIncluded(t *testing.T) {
	t.Run("mark included removes from pool", func(t *testing.T) {
		pool := NewPool()
		msg := &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 0,
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot:   make([]byte, 32),
				Slot:              1,
				PayloadPresent:    true,
				BlobDataAvailable: false,
			},
			Signature: make([]byte, 96),
		}
		require.NoError(t, pool.InsertPayloadAttestation(msg))
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
		msg := &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 0,
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot:   make([]byte, 32),
				Slot:              1,
				PayloadPresent:    true,
				BlobDataAvailable: false,
			},
			Signature: make([]byte, 96),
		}
		require.NoError(t, pool.InsertPayloadAttestation(msg))

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
