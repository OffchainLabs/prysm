package blocks

import (
	"testing"

	pb "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func Test_EpbsBlock_ToBlinded(t *testing.T) {
	b := &SignedBeaconBlock{version: version.EPBS}
	_, err := b.ToBlinded()
	require.ErrorIs(t, err, ErrUnsupportedVersion)
}

func Test_EpbsBlock_Unblind(t *testing.T) {
	b := &SignedBeaconBlock{version: version.EPBS}
	e, err := WrappedExecutionPayload(&pb.ExecutionPayload{})
	require.NoError(t, err)
	err = b.Unblind(e)
	require.ErrorIs(t, err, ErrAlreadyUnblinded)
}

func Test_EpbsBlock_IsBlinded(t *testing.T) {
	b := &SignedBeaconBlock{version: version.EPBS}
	require.Equal(t, false, b.IsBlinded())
	bb := &BeaconBlock{version: version.EPBS}
	require.Equal(t, false, bb.IsBlinded())
	bd := &BeaconBlockBody{version: version.EPBS}
	require.Equal(t, false, bd.IsBlinded())
}

func Test_PreEPBS_Versions(t *testing.T) {
	bb := &BeaconBlockBody{version: version.Electra}
	_, err := bb.PayloadAttestations()
	require.ErrorContains(t, "PayloadAttestations", err)
	_, err = bb.SignedExecutionPayloadHeader()
	require.ErrorContains(t, "SignedExecutionPayloadHeader", err)
}
