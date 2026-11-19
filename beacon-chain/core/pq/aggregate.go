package pq

import (
	"context"
	"slices"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/validator/db/signingprotection"
	"github.com/pkg/errors"
)

var ErrNotAggregator = errors.New("validator is not in the aggregation committee")

// Spec: is_aggregator
func IsAggregator(st state.ReadOnlyBeaconState, slot primitives.Slot, index primitives.CommitteeIndex, validator primitives.ValidatorIndex) (bool, error) {
	committee, err := st.AggregationCommittee(slot, index)
	if err != nil {
		return false, err
	}
	return slices.Contains(committee, validator), nil
}

// Spec: validate_beacon_aggregate_and_proof_gossip, the aggregator, signature and proof checks
func VerifyAggregateAndProof(ctx context.Context, st state.ReadOnlyBeaconState, signed *ethpb.SignedAggregateAttestationAndProofPQ) error {
	message := signed.Message
	aggregate := message.Aggregate
	data := aggregate.Data
	ok, err := IsAggregator(st, data.Slot, aggregate.GetCommitteeIndex(), message.AggregatorIndex)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotAggregator
	}
	domain, err := signing.Domain(st.Fork(), data.Target.Epoch, params.BeaconConfig().DomainAggregateAndProof, st.GenesisValidatorsRoot())
	if err != nil {
		return err
	}
	root, err := message.HashTreeRoot()
	if err != nil {
		return err
	}
	pubkey := st.PubkeyAtIndex(message.AggregatorIndex)
	if err := VerifySlotMessage(pubkey[:], signingprotection.AggregateAndProof, data.Slot, root, domain, signed.Signature); err != nil {
		return errors.Wrap(err, "aggregator signature")
	}
	statements, err := AttestationStatements(ctx, st, aggregate)
	if err != nil {
		return err
	}
	return Proofs.Verify(statements, message.Proof)
}
