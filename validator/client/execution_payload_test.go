package client

import (
	"context"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util/random"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"go.uber.org/mock/gomock"
)

func Test_validator_signExecutionPayloadEnvelope(t *testing.T) {
	v, m, vk, finish := setup(t, false)
	defer finish()

	// Define constants and mock expectations
	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), // ctx
			&ethpb.DomainRequest{
				Epoch:  slots.ToEpoch(slots.CurrentSlot(v.genesisTime)),
				Domain: params.BeaconConfig().DomainBeaconBuilder[:],
			}). // epoch
		Return(&ethpb.DomainResponse{
			SignatureDomain: bytesutil.PadTo([]byte("signatureDomain"), 32),
		}, nil)

	// Generate random payload attestation data
	env := random.ExecutionPayloadEnvelope(t)

	// Perform the signature operation
	ctx := context.Background()
	sig, err := v.signExecutionPayloadEnvelope(ctx, env, [48]byte(vk.PublicKey().Marshal()))
	require.NoError(t, err)

	// Verify the signature
	pb, err := bls.PublicKeyFromBytes(vk.PublicKey().Marshal())
	require.NoError(t, err)
	signature, err := bls.SignatureFromBytes(sig)
	require.NoError(t, err)
	sr, err := signing.ComputeSigningRoot(env, bytesutil.PadTo([]byte("signatureDomain"), 32))
	require.NoError(t, err)
	require.Equal(t, true, signature.Verify(pb, sr[:]))
}
