package blocks

import (
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	consensus_types "github.com/OffchainLabs/prysm/v6/consensus-types"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util/random"
	"github.com/prysmaticlabs/go-bitfield"
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
