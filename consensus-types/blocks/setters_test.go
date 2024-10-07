package blocks

import (
	"testing"

	"github.com/prysmaticlabs/go-bitfield"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	consensus_types "github.com/prysmaticlabs/prysm/v5/consensus-types"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util/random"
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
	signedExecutionPayloadHeader := random.SignedExecutionPayloadHeader(t)
	ws, err := WrappedROSignedExecutionPayloadHeader(signedExecutionPayloadHeader)
	require.NoError(t, err)
	require.NoError(t, b.SetSignedExecutionPayloadHeader(signedExecutionPayloadHeader))
	expectedHeader, err := b.block.body.SignedExecutionPayloadHeader()
	require.NoError(t, err)
	require.DeepEqual(t, expectedHeader, ws)
}
