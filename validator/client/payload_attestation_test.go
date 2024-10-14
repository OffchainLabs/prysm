package client

import (
	"context"
	"testing"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util/random"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"go.uber.org/mock/gomock"
)

func TestValidator_SubmitPayloadAttestationMessage(t *testing.T) {
	// Setup the test environment.
	validator, m, validatorKey, finish := setup(t, true)
	defer finish()
	var pubKey [fieldparams.BLSPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())

	// Map to associate public keys with validator indices.
	validator.pubkeyToValidatorIndex = make(map[[fieldparams.BLSPubkeyLength]byte]primitives.ValidatorIndex)
	validatorIndex := primitives.ValidatorIndex(1)
	validator.pubkeyToValidatorIndex[pubKey] = validatorIndex

	// Generate random payload attestation data for the test.
	d := random.PayloadAttestationData(t)
	slot := primitives.Slot(1000)
	epoch := slots.ToEpoch(slot)
	d.Slot = slot

	// Expectation for the mock validator client to return the generated payload attestation data.
	m.validatorClient.EXPECT().GetPayloadAttestationData(
		gomock.Any(), // Context
		gomock.AssignableToTypeOf(&ethpb.GetPayloadAttestationDataRequest{Slot: slot}),
	).Return(d, nil)

	// Expectation for the mock validator client to return the domain data for the given epoch and domain.
	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // Context
		&ethpb.DomainRequest{
			Epoch:  epoch,
			Domain: params.BeaconConfig().DomainPTCAttester[:],
		},
	).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

	// Duplicate domain data request for computing the correct signature for matching expectations.
	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // Context
		&ethpb.DomainRequest{
			Epoch:  epoch,
			Domain: params.BeaconConfig().DomainPTCAttester[:],
		},
	).Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

	// Sign the payload attestation data using the validator's private key.
	sig, err := validator.signPayloadAttestation(context.Background(), d, pubKey)
	require.NoError(t, err)

	// Expectation for the mock validator client to submit the payload attestation with the signed data.
	m.validatorClient.EXPECT().SubmitPayloadAttestation(
		gomock.Any(), // Context
		gomock.Eq(&ethpb.PayloadAttestationMessage{
			ValidatorIndex: validatorIndex,
			Data:           d,
			Signature:      sig,
		}),
	).Return(&empty.Empty{}, nil)

	validator.SubmitPayloadAttestationMessage(context.Background(), slot, pubKey)
}

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
	pa := random.PayloadAttestationData(t)
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
