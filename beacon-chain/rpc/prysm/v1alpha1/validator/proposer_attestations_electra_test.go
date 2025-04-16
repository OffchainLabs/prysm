package validator

import (
	"reflect"
	"testing"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/crypto/bls/blst"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/prysmaticlabs/go-bitfield"
)

func Test_computeOnChainAggregate(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.MainnetConfig()
	cfg.MaxCommitteesPerSlot = 64
	params.OverrideBeaconConfig(cfg)

	key, err := blst.RandKey()
	require.NoError(t, err)
	sig := key.Sign([]byte{'X'})

	data1 := &ethpb.AttestationData{
		Slot:            123,
		CommitteeIndex:  123,
		BeaconBlockRoot: bytesutil.PadTo([]byte("root"), 32),
		Source: &ethpb.Checkpoint{
			Epoch: 123,
			Root:  bytesutil.PadTo([]byte("root"), 32),
		},
		Target: &ethpb.Checkpoint{
			Epoch: 123,
			Root:  bytesutil.PadTo([]byte("root"), 32),
		},
	}
	data2 := &ethpb.AttestationData{
		Slot:            456,
		CommitteeIndex:  456,
		BeaconBlockRoot: bytesutil.PadTo([]byte("root"), 32),
		Source: &ethpb.Checkpoint{
			Epoch: 456,
			Root:  bytesutil.PadTo([]byte("root"), 32),
		},
		Target: &ethpb.Checkpoint{
			Epoch: 456,
			Root:  bytesutil.PadTo([]byte("root"), 32),
		},
	}

	t.Run("single aggregate", func(t *testing.T) {
		cb := primitives.NewAttestationCommitteeBits()
		cb.SetBitAt(0, true)
		att := &ethpb.AttestationElectra{
			AggregationBits: bitfield.Bitlist{0b00011111},
			Data:            data1,
			CommitteeBits:   cb,
			Signature:       sig.Marshal(),
		}
		result, err := computeOnChainAggregate([]ethpb.Att{att})
		require.NoError(t, err)
		require.Equal(t, 1, len(result))
		assert.DeepEqual(t, att.AggregationBits, result[0].GetAggregationBits())
		assert.DeepEqual(t, att.Data, result[0].GetData())
		assert.DeepEqual(t, att.CommitteeBits, result[0].CommitteeBitsVal())
	})
	t.Run("all aggregates for one root", func(t *testing.T) {
		cb := primitives.NewAttestationCommitteeBits()
		cb.SetBitAt(0, true)
		att1 := &ethpb.AttestationElectra{
			AggregationBits: bitfield.Bitlist{0b00010011}, // aggregation bits 0,1
			Data:            data1,
			CommitteeBits:   cb,
			Signature:       sig.Marshal(),
		}
		cb = primitives.NewAttestationCommitteeBits()
		cb.SetBitAt(1, true)
		att2 := &ethpb.AttestationElectra{
			AggregationBits: bitfield.Bitlist{0b00010011}, // aggregation bits 0,1
			Data:            data1,
			CommitteeBits:   cb,
			Signature:       sig.Marshal(),
		}
		result, err := computeOnChainAggregate([]ethpb.Att{att1, att2})
		require.NoError(t, err)
		require.Equal(t, 1, len(result))
		assert.DeepEqual(t, bitfield.Bitlist{0b00110011, 0b00000001}, result[0].GetAggregationBits())
		assert.DeepEqual(t, data1, result[0].GetData())
		cb = primitives.NewAttestationCommitteeBits()
		cb.SetBitAt(0, true)
		cb.SetBitAt(1, true)
		assert.DeepEqual(t, cb, result[0].CommitteeBitsVal())
	})
	t.Run("aggregates for multiple roots", func(t *testing.T) {
		cb := primitives.NewAttestationCommitteeBits()
		cb.SetBitAt(0, true)
		att1 := &ethpb.AttestationElectra{
			AggregationBits: bitfield.Bitlist{0b00010011}, // aggregation bits 0,1
			Data:            data1,
			CommitteeBits:   cb,
			Signature:       sig.Marshal(),
		}
		cb = primitives.NewAttestationCommitteeBits()
		cb.SetBitAt(1, true)
		att2 := &ethpb.AttestationElectra{
			AggregationBits: bitfield.Bitlist{0b00010011}, // aggregation bits 0,1
			Data:            data1,
			CommitteeBits:   cb,
			Signature:       sig.Marshal(),
		}
		cb = primitives.NewAttestationCommitteeBits()
		cb.SetBitAt(0, true)
		att3 := &ethpb.AttestationElectra{
			AggregationBits: bitfield.Bitlist{0b00011001}, // aggregation bits 0,3
			Data:            data2,
			CommitteeBits:   cb,
			Signature:       sig.Marshal(),
		}
		cb = primitives.NewAttestationCommitteeBits()
		cb.SetBitAt(1, true)
		att4 := &ethpb.AttestationElectra{
			AggregationBits: bitfield.Bitlist{0b00010010}, // aggregation bits 1
			Data:            data2,
			CommitteeBits:   cb,
			Signature:       sig.Marshal(),
		}
		result, err := computeOnChainAggregate([]ethpb.Att{att1, att2, att3, att4})
		require.NoError(t, err)
		require.Equal(t, 2, len(result))
		cb = primitives.NewAttestationCommitteeBits()
		cb.SetBitAt(0, true)
		cb.SetBitAt(1, true)

		expectedAggBits := bitfield.Bitlist{0b00110011, 0b00000001}
		expectedData := data1
		found := false
		for _, a := range result {
			if reflect.DeepEqual(expectedAggBits, a.GetAggregationBits()) && reflect.DeepEqual(expectedData, a.GetData()) && reflect.DeepEqual(cb, a.CommitteeBitsVal()) {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected aggregate not found")
		}

		expectedAggBits = bitfield.Bitlist{0b00101001, 0b00000001}
		expectedData = data2
		found = false
		for _, a := range result {
			if reflect.DeepEqual(expectedAggBits, a.GetAggregationBits()) && reflect.DeepEqual(expectedData, a.GetData()) && reflect.DeepEqual(cb, a.CommitteeBitsVal()) {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected aggregate not found")
		}
	})
}
