package client

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	validatorpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/validator-client"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func Test_signBuilderRequestAuth(t *testing.T) {
	kp := randKeypair(t)
	km := newMockKeymanager(t, kp)
	v := validator{}
	auth := &ethpb.BuilderRequestAuth{
		Data: []byte("https://builder.example.org"),
		Slot: 123,
	}

	t.Run("signs with the fork-independent domain", func(t *testing.T) {
		signed, err := v.signBuilderRequestAuth(t.Context(), km, kp.pub, auth)
		require.NoError(t, err)
		require.Equal(t, auth, signed.Message)

		domain, err := signing.ComputeDomain(
			params.BeaconConfig().DomainBuilderRequestAuth,
			params.BeaconConfig().GenesisForkVersion,
			make([]byte, 32),
		)
		require.NoError(t, err)
		root, err := signing.ComputeSigningRoot(auth, domain)
		require.NoError(t, err)
		sig, err := bls.SignatureFromBytes(signed.Signature)
		require.NoError(t, err)
		require.Equal(t, true, sig.Verify(kp.pri.PublicKey(), root[:]))
	})

	t.Run("sign request carries the auth slot and object", func(t *testing.T) {
		_, err := v.signBuilderRequestAuth(t.Context(), km, kp.pub, auth)
		require.NoError(t, err)

		req := km.lastSignRequest()
		require.NotNil(t, req)
		require.Equal(t, auth.Slot, req.SigningSlot)
		require.DeepEqual(t, kp.pub[:], req.PublicKey)
		obj, ok := req.Object.(*validatorpb.SignRequest_BuilderRequestAuth)
		require.Equal(t, true, ok)
		require.Equal(t, auth, obj.BuilderRequestAuth)
	})

	t.Run("unknown key", func(t *testing.T) {
		other := randKeypair(t)
		_, err := v.signBuilderRequestAuth(t.Context(), km, other.pub, auth)
		require.ErrorContains(t, "could not sign builder request auth", err)
	})
}
