package decoupled

import (
	"errors"
	"fmt"

	"github.com/OffchainLabs/methodical-ssz/ssz"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// Spec: the FG wire checks on the data, height at least 1, a nonempty target, a finality pair absent or nonempty below the height.
func ValidateFinalityShape(d *ethpb.AttestationDataDecoupled) error {
	if d == nil {
		return errNilAttestationData
	}
	if d.Height == 0 {
		return errors.New("height must be at least 1")
	}
	if bytesutil.ZeroRoot(d.Target) {
		return errors.New("empty target")
	}
	if d.FinalityHeight == 0 {
		if !bytesutil.ZeroRoot(d.FinalityTarget) {
			return errors.New("finality target without a height")
		}
		return nil
	}
	if bytesutil.ZeroRoot(d.FinalityTarget) || d.FinalityHeight >= d.Height {
		return errors.New("finality pair must be nonempty and below the height")
	}
	return nil
}

// Pinned to the Decoupled fork version so a delayed row verifies the same whenever it arrives.
func FinalityDomain(st state.ReadOnlyBeaconState) ([]byte, error) {
	cfg := params.BeaconConfig()
	return signing.ComputeDomain(cfg.DomainFinalityAttestation, cfg.DecoupledForkVersion, st.GenesisValidatorsRoot())
}

func AvailableDomain(st state.ReadOnlyBeaconState, slot primitives.Slot) ([]byte, error) {
	return signing.Domain(st.Fork(), slots.ToEpoch(slot), params.BeaconConfig().DomainAvailableAttestation, st.GenesisValidatorsRoot())
}

func pubkey(st state.ReadOnlyBeaconState, v primitives.ValidatorIndex) ([]byte, error) {
	if uint64(v) >= uint64(st.NumValidators()) {
		return nil, fmt.Errorf("validator %d is not in the registry", v)
	}
	pk := st.PubkeyAtIndex(v)
	return pk[:], nil
}

// Spec: the FG wire checks, signer in V, data shape and signature.
func VerifyFinalityAttestation(st state.ReadOnlyBeaconState, m *ethpb.FinalityAttestationMessage) error {
	if err := ValidateFinalityShape(m.Data); err != nil {
		return err
	}
	pk, err := pubkey(st, m.ValidatorIndex)
	if err != nil {
		return err
	}
	domain, err := FinalityDomain(st)
	if err != nil {
		return err
	}
	return signing.VerifySigningRoot(m.Data, pk, m.Signature, domain)
}

// Spec: the AC wire checks, signer in the slot's committee, which is every validator, and the signature.
func VerifyAvailableAttestation(st state.ReadOnlyBeaconState, m *ethpb.AvailableAttestationMessage) error {
	if m.Data == nil {
		return errNilAttestationData
	}
	pk, err := pubkey(st, m.ValidatorIndex)
	if err != nil {
		return err
	}
	domain, err := AvailableDomain(st, m.Data.Slot)
	if err != nil {
		return err
	}
	return signing.VerifySigningRoot(m.Data, pk, m.Signature, domain)
}

// Every aggregate against the aggregate key of its signers, AC rows over the bitlist and FG rows over the index list, joined so a block costs one batch check.
func SignatureBatch(st state.ReadOnlyBeaconState, body interfaces.ReadOnlyBeaconBlockBody) (*bls.SignatureBatch, error) {
	set := bls.NewSet()
	votes, err := body.AvailableAttestations()
	if err != nil {
		return nil, err
	}
	for _, a := range votes {
		if a.Data == nil {
			return nil, errNilAttestationData
		}
		domain, err := AvailableDomain(st, a.Data.Slot)
		if err != nil {
			return nil, err
		}
		if err := addAggregate(set, st, a.AggregationBits.BitIndices(), a.Data, a.Signature, domain, "available attestation"); err != nil {
			return nil, err
		}
	}
	atts, err := body.AttestationsDecoupled()
	if err != nil {
		return nil, err
	}
	if len(atts) == 0 {
		return set, nil
	}
	domain, err := FinalityDomain(st)
	if err != nil {
		return nil, err
	}
	for _, a := range atts {
		indices := make([]int, len(a.AttestingIndices))
		for i, v := range a.AttestingIndices {
			indices[i] = int(v)
		}
		if err := addAggregate(set, st, indices, a.Data, a.Signature, domain, "finality attestation"); err != nil {
			return nil, err
		}
	}
	return set, nil
}

func addAggregate(set *bls.SignatureBatch, st state.ReadOnlyBeaconState, indices []int, data ssz.HashRoot, sig, domain []byte, desc string) error {
	if len(indices) == 0 {
		return fmt.Errorf("%s without signers", desc)
	}
	keys := make([]bls.PublicKey, 0, len(indices))
	for _, i := range indices {
		raw, err := pubkey(st, primitives.ValidatorIndex(i))
		if err != nil {
			return err
		}
		key, err := bls.PublicKeyFromBytes(raw)
		if err != nil {
			return err
		}
		keys = append(keys, key)
	}
	root, err := signing.ComputeSigningRoot(data, domain)
	if err != nil {
		return err
	}
	set.Signatures = append(set.Signatures, sig)
	set.PublicKeys = append(set.PublicKeys, bls.AggregateMultiplePubkeys(keys))
	set.Messages = append(set.Messages, root)
	set.Descriptions = append(set.Descriptions, desc)
	return nil
}
