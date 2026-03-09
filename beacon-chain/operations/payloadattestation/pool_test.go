package payloadattestation

import (
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"google.golang.org/protobuf/proto"
)

var (
	benchmarkPayloadAttestationDataHashSink   [32]byte
	benchmarkPayloadAttestationDataStructSink payloadAttestationDataKey
)

func TestPool_PendingPayloadAttestations(t *testing.T) {
	t.Run("empty pool", func(t *testing.T) {
		pool := NewPool()
		atts := pool.PendingPayloadAttestations(primitives.Slot(0))
		assert.Equal(t, 0, len(atts))
		assert.Equal(t, 0, pendingCount(pool))
	})

	t.Run("returns requested slot", func(t *testing.T) {
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
		atts := pool.PendingPayloadAttestations(primitives.Slot(2))
		assert.Equal(t, 1, len(atts))
		assert.Equal(t, primitives.Slot(2), atts[0].Data.Slot)
		assert.Equal(t, 1, pendingCount(pool))
	})

	t.Run("slot filtering keeps future entries", func(t *testing.T) {
		pool := NewPool()
		sig := bls.NewAggregateSignature().Marshal()
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
		require.NoError(t, pool.InsertPayloadAttestation(msg2, 1))

		atts := pool.PendingPayloadAttestations(primitives.Slot(1))
		assert.Equal(t, 0, len(atts))
		assert.Equal(t, 1, pendingCount(pool))

		atts = pool.PendingPayloadAttestations(primitives.Slot(2))
		assert.Equal(t, 1, len(atts))
		assert.Equal(t, primitives.Slot(2), atts[0].Data.Slot)
		assert.Equal(t, 1, pendingCount(pool))

		atts = pool.PendingPayloadAttestations(primitives.Slot(99))
		assert.Equal(t, 0, len(atts))
		assert.Equal(t, 1, pendingCount(pool))
	})

	t.Run("future slot request does not prune", func(t *testing.T) {
		pool := NewPool()
		sig := bls.NewAggregateSignature().Marshal()
		msg1 := &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 0,
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot:   bytesutil.PadTo([]byte("r1"), 32),
				Slot:              1,
				PayloadPresent:    true,
				BlobDataAvailable: false,
			},
			Signature: sig,
		}
		msg2 := &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 1,
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot:   bytesutil.PadTo([]byte("r2"), 32),
				Slot:              2,
				PayloadPresent:    true,
				BlobDataAvailable: false,
			},
			Signature: sig,
		}
		require.NoError(t, pool.InsertPayloadAttestation(msg1, 0))
		require.NoError(t, pool.InsertPayloadAttestation(msg2, 1))

		atts := pool.PendingPayloadAttestations(primitives.Slot(10))
		assert.Equal(t, 0, len(atts))
		assert.Equal(t, 1, pendingCount(pool))
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

	t.Run("invalid beacon block root length", func(t *testing.T) {
		pool := NewPool()
		msg := &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 0,
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot:   []byte{0x01, 0x02}, // invalid length
				Slot:              1,
				PayloadPresent:    true,
				BlobDataAvailable: false,
			},
			Signature: bls.NewAggregateSignature().Marshal(),
		}
		err := pool.InsertPayloadAttestation(msg, 0)
		require.ErrorContains(t, "invalid beacon block root length", err)
		assert.Equal(t, 0, pendingCount(pool))
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
		atts := pool.PendingPayloadAttestations(primitives.Slot(1))
		require.Equal(t, 1, len(atts))
		assert.Equal(t, true, atts[0].AggregationBits.BitAt(idx))
		assert.Equal(t, false, atts[0].AggregationBits.BitAt(idx+1))
	})

	t.Run("out-of-range index returns error and does not insert", func(t *testing.T) {
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
		err := pool.InsertPayloadAttestation(msg, uint64(fieldparams.PTCSize))
		require.ErrorContains(t, "invalid payload attestation committee index", err)
		assert.Equal(t, 0, pendingCount(pool))
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
		key, err := dataKey(msg.Data)
		require.NoError(t, err)
		firstSig := bytesutil.SafeCopyBytes(pool.pending[key].Signature)

		require.NoError(t, pool.InsertPayloadAttestation(msg, idx))
		atts := pool.PendingPayloadAttestations(primitives.Slot(1))
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

		atts := pool.PendingPayloadAttestations(primitives.Slot(1))
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
		atts := pool.PendingPayloadAttestations(primitives.Slot(1))
		assert.Equal(t, 2, len(atts))
	})

	t.Run("inserting newer slot prunes older slots", func(t *testing.T) {
		pool := NewPool()
		sig := bls.NewAggregateSignature().Marshal()
		msg1 := &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 0,
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot:   bytesutil.PadTo([]byte("older"), 32),
				Slot:              1,
				PayloadPresent:    true,
				BlobDataAvailable: false,
			},
			Signature: sig,
		}
		msg2 := &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 1,
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot:   bytesutil.PadTo([]byte("newer"), 32),
				Slot:              2,
				PayloadPresent:    true,
				BlobDataAvailable: false,
			},
			Signature: sig,
		}

		require.NoError(t, pool.InsertPayloadAttestation(msg1, 0))
		require.NoError(t, pool.InsertPayloadAttestation(msg2, 1))
		assert.Equal(t, 1, pendingCount(pool))

		atts := pool.PendingPayloadAttestations(primitives.Slot(1))
		assert.Equal(t, 0, len(atts))
		atts = pool.PendingPayloadAttestations(primitives.Slot(2))
		assert.Equal(t, 1, len(atts))
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

	assert.Equal(t, false, pool.Seen(&ethpb.PayloadAttestationData{
		BeaconBlockRoot: []byte{0x01}, // invalid
		Slot:            1,
	}, 5))
}

func pendingCount(pool *Pool) int {
	pool.lock.RLock()
	defer pool.lock.RUnlock()
	return len(pool.pending)
}

func BenchmarkPayloadAttestationDataKeyStrategies(b *testing.B) {
	data := &ethpb.PayloadAttestationData{
		BeaconBlockRoot:   bytesutil.PadTo([]byte("benchmark-root"), 32),
		Slot:              primitives.Slot(12345),
		PayloadPresent:    true,
		BlobDataAvailable: true,
	}

	b.Run("protoMarshal", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			key, err := dataKeyProtoMarshal(data)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkPayloadAttestationDataHashSink = key
		}
	})

	b.Run("ssz", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			key, err := data.HashTreeRoot()
			if err != nil {
				b.Fatal(err)
			}
			benchmarkPayloadAttestationDataHashSink = key
		}
	})

	b.Run("struct", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			key, err := dataKey(data)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkPayloadAttestationDataStructSink = key
		}
	})
}

func dataKeyProtoMarshal(data *ethpb.PayloadAttestationData) ([32]byte, error) {
	enc, err := proto.Marshal(data)
	if err != nil {
		return [32]byte{}, err
	}
	return hash.Hash(enc), nil
}
