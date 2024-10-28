package blocks

import (
	"testing"

	"github.com/prysmaticlabs/go-bitfield"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	consensus_types "github.com/prysmaticlabs/prysm/v5/consensus-types"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func Test_EpbsBlock_SetPayloadAttestations(t *testing.T) {
	b := &SignedBeaconBlock{version: version.Deneb}
	require.ErrorIs(t, b.SetPayloadAttestations(nil), consensus_types.ErrUnsupportedField)

	b = &SignedBeaconBlock{version: version.EPBS,
		block: &BeaconBlock{version: version.EPBS,
			body: &BeaconBlockBody{version: version.EPBS}}}
	aggregationBits := bitfield.NewBitvector512()
	aggregationBits.SetBitAt(0, true)
	payloadAttestation := []*eth.PayloadAttestation{
		{
			AggregationBits: aggregationBits,
			Data: &eth.PayloadAttestationData{
				BeaconBlockRoot: bytesutil.PadTo([]byte{123}, 32),
				Slot:            1,
				PayloadStatus:   2,
			},
			Signature: bytesutil.PadTo([]byte("signature"), fieldparams.BLSSignatureLength),
		},
		{
			AggregationBits: aggregationBits,
			Data: &eth.PayloadAttestationData{
				BeaconBlockRoot: bytesutil.PadTo([]byte{123}, 32),
				Slot:            1,
				PayloadStatus:   3,
			},
		},
	}

	require.NoError(t, b.SetPayloadAttestations(payloadAttestation))
	expectedPA, err := b.block.body.PayloadAttestations()
	require.NoError(t, err)
	require.DeepEqual(t, expectedPA, payloadAttestation)
}

func Test_EpbsBlock_SetSignedExecutionPayloadHeader(t *testing.T) {
	b := &SignedBeaconBlock{version: version.Deneb}
	require.ErrorIs(t, b.SetSignedExecutionPayloadHeader(nil), consensus_types.ErrUnsupportedField)

	b = &SignedBeaconBlock{version: version.EPBS,
		block: &BeaconBlock{version: version.EPBS,
			body: &BeaconBlockBody{version: version.EPBS}}}
	signedExecutionPayloadHeader := &enginev1.SignedExecutionPayloadHeader{
		Message: &enginev1.ExecutionPayloadHeaderEPBS{
			ParentBlockHash:        []byte("parentBlockHash"),
			ParentBlockRoot:        []byte("parentBlockRoot"),
			BlockHash:              []byte("blockHash"),
			BuilderIndex:           1,
			Slot:                   2,
			Value:                  3,
			BlobKzgCommitmentsRoot: []byte("blobKzgCommitmentsRoot"),
			GasLimit:               4,
		},
		Signature: []byte("signature"),
	}
	require.NoError(t, b.SetSignedExecutionPayloadHeader(signedExecutionPayloadHeader))
	expectedHeader, err := b.block.body.SignedExecutionPayloadHeader()
	require.NoError(t, err)
	require.DeepEqual(t, expectedHeader, signedExecutionPayloadHeader)
}
