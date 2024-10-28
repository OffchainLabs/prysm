package client

import (
	"context"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"go.uber.org/mock/gomock"
)

func Test_validator_signPayloadAttestation(t *testing.T) {
	v, m, vk, finish := setup(t, false)
	defer finish()

	// Define constants and mock expectations
	e := primitives.Epoch(1000)
	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), // ctx
			&ethpb.DomainRequest{
				Epoch:  e,
				Domain: params.BeaconConfig().DomainPTCAttester[:],
			}). // epoch
		Return(&ethpb.DomainResponse{
			SignatureDomain: bytesutil.PadTo([]byte("signatureDomain"), 32),
		}, nil)

	// Generate random payload attestation data
	pa := util.GenerateRandomPayloadAttestationData(t)
	pa.Slot = primitives.Slot(e) * params.BeaconConfig().SlotsPerEpoch // Verify that go mock EXPECT() gets the correct epoch.

	// Perform the signature operation
	ctx := context.Background()
	sig, err := v.signPayloadAttestation(ctx, pa, [48]byte(vk.PublicKey().Marshal()))
	require.NoError(t, err)

	// Verify the signature
	pb, err := bls.PublicKeyFromBytes(vk.PublicKey().Marshal())
	require.NoError(t, err)
	signature, err := bls.SignatureFromBytes(sig)
	require.NoError(t, err)
	sr, err := signing.ComputeSigningRoot(pa, bytesutil.PadTo([]byte("signatureDomain"), 32))
	require.NoError(t, err)
	require.Equal(t, true, signature.Verify(pb, sr[:]))
}
