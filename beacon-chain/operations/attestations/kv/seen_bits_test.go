package kv

import (
	"testing"

	"github.com/prysmaticlabs/go-bitfield"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/attestation"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func TestAttCaches_hasSeenBit(t *testing.T) {
	c := NewAttCaches()

	seenA1 := util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b10000011}})
	seenA2 := util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b11100000}})
	require.NoError(t, c.insertSeenBit(seenA1))
	require.NoError(t, c.insertSeenBit(seenA2))
	tests := []struct {
		att  *ethpb.Attestation
		want bool
	}{
		{att: util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b10000000}}), want: true},
		{att: util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b10000001}}), want: true},
		{att: util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b11100000}}), want: true},
		{att: util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b10000011}}), want: true},
		{att: util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b10001000}}), want: false},
		{att: util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b11110111}}), want: false},
	}
	for _, tt := range tests {
		got, err := c.hasSeenBit(tt.att)
		require.NoError(t, err)
		if got != tt.want {
			t.Errorf("hasSeenBit() got = %v, want %v", got, tt.want)
		}
	}
}

func TestAttCaches_insertSeenBitDuplicates(t *testing.T) {
	c := NewAttCaches()
	att1 := util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b10000011}})
	id, err := attestation.NewId(att1, attestation.Data)
	require.NoError(t, err)
	require.NoError(t, c.insertSeenBit(att1))
	require.Equal(t, 1, c.seenAtt.ItemCount())

	_, expirationTime1, ok := c.seenAtt.GetWithExpiration(id.String())
	require.Equal(t, true, ok)

	// Make sure that duplicates are not inserted, but expiration time gets updated.
	require.NoError(t, c.insertSeenBit(att1))
	require.Equal(t, 1, c.seenAtt.ItemCount())
	_, expirationprysmTime, ok := c.seenAtt.GetWithExpiration(id.String())
	require.Equal(t, true, ok)
	require.Equal(t, true, expirationprysmTime.After(expirationTime1), "Expiration time is not updated")
}
