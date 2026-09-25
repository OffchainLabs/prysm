package decoupled

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// Spec: the signature half of is_valid_indexed_attestation, batched per block. The Gloas batch takes ethpb.Att, which the Decoupled type does not implement.
func AttestationSignatureBatch(ctx context.Context, st state.ReadOnlyBeaconState, atts []*ethpb.AttestationDecoupled) (*bls.SignatureBatch, error) {
	set := bls.NewSet()
	if len(atts) == 0 {
		return set, nil
	}
	sigs := make([][]byte, 0, len(atts))
	pks := make([]bls.PublicKey, 0, len(atts))
	msgs := make([][32]byte, 0, len(atts))
	descs := make([]string, 0, len(atts))
	for _, a := range atts {
		committees, err := attestationCommittees(ctx, st, a)
		if err != nil {
			return nil, err
		}
		indices, err := attestingIndices(a.AggregationBits, committees)
		if err != nil {
			return nil, err
		}
		if err := validIndices(st, indices); err != nil {
			return nil, err
		}
		pk, err := st.AggregateKeyFromIndices(indices)
		if err != nil {
			return nil, err
		}
		domain, err := attestationDomain(st, a.Data.Slot)
		if err != nil {
			return nil, err
		}
		root, err := signing.ComputeSigningRoot(a.Data, domain)
		if err != nil {
			return nil, err
		}
		sigs = append(sigs, a.Signature)
		pks = append(pks, pk)
		msgs = append(msgs, root)
		descs = append(descs, signing.AttestationSignature)
	}
	set.Join(&bls.SignatureBatch{Signatures: sigs, PublicKeys: pks, Messages: msgs, Descriptions: descs})
	return set, nil
}
