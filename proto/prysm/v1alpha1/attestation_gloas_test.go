package eth_test

import (
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestAggregateAttestationAndProofGloas_Conversion(t *testing.T) {
	electra := &ethpb.AggregateAttestationAndProofElectra{
		AggregatorIndex: 42,
		Aggregate: util.HydrateAttestationElectra(&ethpb.AttestationElectra{
			AggregationBits: bitfield.NewBitlist(3),
		}),
		SelectionProof: make([]byte, 96),
	}
	electra.Aggregate.AggregationBits.SetBitAt(1, true)
	electra.Aggregate.CommitteeBits.SetBitAt(0, true)
	electra.SelectionProof[0] = 1

	gloas := ethpb.AggregateAttestationAndProofElectraToGloas(electra)
	require.Equal(t, version.Gloas, gloas.Version())
	require.Equal(t, version.Gloas, gloas.AggregateVal().Version())
	require.DeepEqual(t, electra, ethpb.AggregateAttestationAndProofGloasToElectra(gloas))

	electraBytes, err := electra.MarshalSSZ()
	require.NoError(t, err)
	gloasBytes, err := gloas.MarshalSSZ()
	require.NoError(t, err)
	require.DeepEqual(t, electraBytes, gloasBytes)

	gloas.Aggregate.Signature[0] = 2
	gloas.SelectionProof[0] = 3
	require.Equal(t, byte(0), electra.Aggregate.Signature[0], "attestation signature aliases Electra input")
	require.Equal(t, byte(1), electra.SelectionProof[0], "selection proof aliases Electra input")
}

func TestSignedAggregateAttestationAndProofGloas_Conversion(t *testing.T) {
	electra := &ethpb.SignedAggregateAttestationAndProofElectra{
		Message: &ethpb.AggregateAttestationAndProofElectra{
			Aggregate: util.HydrateAttestationElectra(&ethpb.AttestationElectra{
				AggregationBits: bitfield.NewBitlist(1),
			}),
			SelectionProof: make([]byte, 96),
		},
		Signature: make([]byte, 96),
	}

	gloas := ethpb.SignedAggregateAttestationAndProofElectraToGloas(electra)
	require.Equal(t, version.Gloas, gloas.Version())
	require.Equal(t, version.Gloas, gloas.AggregateAttestationAndProof().Version())
	require.DeepEqual(t, electra, ethpb.SignedAggregateAttestationAndProofGloasToElectra(gloas))

	electraBytes, err := electra.MarshalSSZ()
	require.NoError(t, err)
	gloasBytes, err := gloas.MarshalSSZ()
	require.NoError(t, err)
	require.DeepEqual(t, electraBytes, gloasBytes)

	gloas.Signature[0] = 1
	require.Equal(t, byte(0), electra.Signature[0], "signature aliases Electra input")
}

func TestAggregateAttestationAndProofGloas_ConversionNil(t *testing.T) {
	require.Equal(t, (*ethpb.AggregateAttestationAndProofGloas)(nil), ethpb.AggregateAttestationAndProofElectraToGloas(nil))
	require.Equal(t, (*ethpb.AggregateAttestationAndProofElectra)(nil), ethpb.AggregateAttestationAndProofGloasToElectra(nil))
	require.Equal(t, (*ethpb.SignedAggregateAttestationAndProofGloas)(nil), ethpb.SignedAggregateAttestationAndProofElectraToGloas(nil))
	require.Equal(t, (*ethpb.SignedAggregateAttestationAndProofElectra)(nil), ethpb.SignedAggregateAttestationAndProofGloasToElectra(nil))
}
