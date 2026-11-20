package pq

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/OffchainLabs/prysm/v7/validator/db/signingprotection"
)

// Spec: validate_payload_attestation_message_gossip, the signature check
func VerifyPayloadAttestationMessage(st state.ReadOnlyBeaconState, msg *ethpb.PayloadAttestationMessagePQ) error {
	domain, err := signing.Domain(st.Fork(), slots.ToEpoch(msg.Data.Slot), params.BeaconConfig().DomainPTCAttester, st.GenesisValidatorsRoot())
	if err != nil {
		return err
	}
	root, err := msg.Data.HashTreeRoot()
	if err != nil {
		return err
	}
	pubkey := st.PubkeyAtIndex(msg.ValidatorIndex)
	return VerifySlotMessage(pubkey[:], signingprotection.PayloadAttestation, msg.Data.Slot, root, domain, msg.Signature)
}

// Spec: validate_proposer_preferences_gossip, the signature check
func VerifyProposerPreferences(st state.ReadOnlyBeaconState, signed *ethpb.SignedProposerPreferencesPQ) error {
	epoch := slots.ToEpoch(signed.Message.ProposalSlot)
	fork, err := params.Fork(epoch)
	if err != nil {
		return err
	}
	domain, err := signing.Domain(fork, epoch, params.BeaconConfig().DomainProposerPreferences, st.GenesisValidatorsRoot())
	if err != nil {
		return err
	}
	root, err := signed.Message.HashTreeRoot()
	if err != nil {
		return err
	}
	pubkey := st.PubkeyAtIndex(signed.Message.ValidatorIndex)
	return VerifySlotMessage(pubkey[:], signingprotection.ProposerPreferences, signed.Message.ProposalSlot, root, domain, signed.Signature)
}
