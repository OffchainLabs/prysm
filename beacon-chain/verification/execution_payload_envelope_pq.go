package verification

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/pq"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/validator/db/signingprotection"
	"github.com/OffchainLabs/prysm/v7/validator/xmsssigner"
	"github.com/pkg/errors"
)

func verifyPayloadEnvelopeSignaturePQ(e interfaces.ROSignedExecutionPayloadEnvelope, pubkey []byte, fork *ethpb.Fork, epoch primitives.Epoch, genesisValidatorsRoot, sig []byte) error {
	env, err := e.Envelope()
	if err != nil {
		return errors.Wrap(err, "failed to get envelope")
	}
	domain, err := signing.Domain(fork, epoch, params.BeaconConfig().DomainBeaconBuilder, genesisValidatorsRoot)
	if err != nil {
		return errors.Wrap(err, "failed to compute signing domain")
	}
	root, err := e.SigningRoot(domain)
	if err != nil {
		return errors.Wrap(err, "failed to compute signing root")
	}
	leafIndex, err := xmsssigner.SlotLeafIndex(env.Slot(), signingprotection.ExecutionPayloadEnvelope)
	if err != nil {
		return err
	}
	return pq.VerifyXMSS(pubkey, leafIndex, root, sig)
}
