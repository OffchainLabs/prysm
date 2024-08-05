package epbs

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestGetAttestingIndices(t *testing.T) {
	testCases := []struct {
		name             string
		attestingIndices []primitives.ValidatorIndex
	}{
		{
			name:             "Test 1",
			attestingIndices: []primitives.ValidatorIndex{1, 2, 3},
		},
		{
			name:             "Test 2",
			attestingIndices: []primitives.ValidatorIndex{55, 66, 787},
		},
		{
			name:             "Empty AttestingIndices",
			attestingIndices: []primitives.ValidatorIndex{},
		},
		{
			name:             "Nil AttestingIndices",
			attestingIndices: []primitives.ValidatorIndex(nil),
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			pa := &IndexedPayloadAttestation{
				AttestingIndices: tt.attestingIndices,
			}
			got := pa.GetAttestingIndices()
			require.DeepEqual(t, tt.attestingIndices, got)
		})
	}
}

func TestGetData(t *testing.T) {
	testCases := []struct {
		name string
		data *eth.PayloadAttestationData
	}{
		{
			name: "Test 1",
			data: &eth.PayloadAttestationData{
				Slot: primitives.Slot(1),
			},
		},
		{
			name: "Test 2",
			data: &eth.PayloadAttestationData{
				Slot: primitives.Slot(100),
			},
		},
		{
			name: "Zero slot",
			data: &eth.PayloadAttestationData{
				Slot: primitives.Slot(0),
			},
		},
		{
			name: "Nil slot",
			data: nil,
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			pa := &IndexedPayloadAttestation{
				Data: tt.data,
			}
			got := pa.GetData()
			require.DeepEqual(t, tt.data, got)
		})
	}
}

func TestGetSignature(t *testing.T) {
	testCases := []struct {
		name string
		sig  []byte
	}{
		{
			name: "Test 1",
			sig:  []byte{1, 2},
		},
		{
			name: "Test 2",
			sig:  []byte{29, 100},
		},
		{
			name: "Zero signature",
			sig:  []byte{0},
		},
		{
			name: "Nil signature",
			sig:  nil,
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			pa := &IndexedPayloadAttestation{
				Signature: tt.sig,
			}
			got := pa.GetSignature()
			require.DeepEqual(t, tt.sig, got)
		})
	}
}
