package pq

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/OffchainLabs/prysm/v7/validator/db/signingprotection"
	"github.com/OffchainLabs/prysm/v7/validator/xmsssigner"
	"github.com/nalepae/go-leanxmss"
	"github.com/pkg/errors"
)

const xmssPubkeyLength = 32

var ErrInvalidXMSSSignature = errors.New("invalid XMSS signature")

// The state keeps keys as 48 bytes in memory, 16 zero bytes then the XMSS key.
func VerifyXMSS(pubkey []byte, leafIndex uint32, message [32]byte, sig []byte) error {
	if len(pubkey) == fieldparams.BLSPubkeyLength {
		pubkey = pubkey[fieldparams.BLSPubkeyLength-xmssPubkeyLength:]
	}
	pk, err := leanxmss.PublicKeyFromBytes(pubkey)
	if err != nil {
		return errors.Wrap(ErrInvalidXMSSSignature, err.Error())
	}
	s, err := leanxmss.SignatureFromBytes(sig)
	if err != nil {
		return errors.Wrap(ErrInvalidXMSSSignature, err.Error())
	}
	if !leanxmss.Verify(pk, leafIndex, message, s) {
		return ErrInvalidXMSSSignature
	}
	return nil
}

func verifyProposerSignature(st state.ReadOnlyBeaconState, proposer primitives.ValidatorIndex, slot primitives.Slot, root [32]byte, sig []byte) error {
	domain, err := signing.Domain(st.Fork(), slots.ToEpoch(slot), params.BeaconConfig().DomainBeaconProposer, st.GenesisValidatorsRoot())
	if err != nil {
		return err
	}
	signingRoot, err := signing.ComputeSigningRootForRoot(root, domain)
	if err != nil {
		return err
	}
	leafIndex, err := xmsssigner.SlotLeafIndex(slot, signingprotection.Block)
	if err != nil {
		return err
	}
	pubkey := st.PubkeyAtIndex(proposer)
	return VerifyXMSS(pubkey[:], leafIndex, signingRoot, sig)
}

// Spec: verify_block_signature
func VerifyBlockSignature(st state.ReadOnlyBeaconState, signed interfaces.ReadOnlySignedBeaconBlock) error {
	blk := signed.Block()
	root, err := blk.HashTreeRoot()
	if err != nil {
		return err
	}
	if err := verifyProposerSignature(st, blk.ProposerIndex(), blk.Slot(), root, signed.XMSSSignature()); err != nil {
		return errors.Wrap(err, "block signature")
	}
	return nil
}

// Spec: process_proposer_slashing, the signature check
func VerifyBlockHeaderSignature(st state.ReadOnlyBeaconState, header *ethpb.SignedBeaconBlockHeaderPQ) error {
	root, err := header.Header.HashTreeRoot()
	if err != nil {
		return err
	}
	return verifyProposerSignature(st, header.Header.ProposerIndex, header.Header.Slot, root, header.Signature)
}

// Spec: verify_execution_payload_bid_signature
func VerifyBuilderBidSignature(st state.ReadOnlyBeaconState, signedBid interfaces.ROSignedExecutionPayloadBid, sig []byte) error {
	bid := signedBid.Bid()
	s, err := leanxmss.SignatureFromBytes(sig)
	if err != nil {
		return errors.Wrap(ErrInvalidXMSSSignature, err.Error())
	}
	leafIndex := s.LeafIndex()
	start := xmsssigner.ReservedLeafCount + xmsssigner.AttestationLeafCount
	if uint64(leafIndex) < start || uint64(leafIndex) >= start+xmsssigner.BuilderBidLeafCount {
		return errors.Wrapf(ErrInvalidXMSSSignature, "leaf %d is not a builder bid leaf", leafIndex)
	}
	domain, err := signing.Domain(st.Fork(), slots.ToEpoch(bid.Slot()), params.BeaconConfig().DomainBeaconBuilder, st.GenesisValidatorsRoot())
	if err != nil {
		return err
	}
	signingRoot, err := signedBid.SigningRoot(domain)
	if err != nil {
		return err
	}
	pubkey, err := st.BuilderPubkey(bid.BuilderIndex())
	if err != nil {
		return err
	}
	return VerifyXMSS(pubkey[:], leafIndex, signingRoot, sig)
}
