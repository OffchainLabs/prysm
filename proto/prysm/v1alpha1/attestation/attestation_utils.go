// Package attestationutil contains useful helpers for converting
// attestations into indexed form.
package attestation

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/go-bitfield"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// ConvertToIndexed converts attestation to (almost) indexed-verifiable form.
//
// Note about spec pseudocode definition. The state was used by get_attesting_indices to determine
// the attestation committee. Now that we provide this as an argument, we no longer need to provide
// a state.
//
// Spec pseudocode definition:
//
//	def get_indexed_attestation(state: BeaconState, attestation: Attestation) -> IndexedAttestation:
//	 """
//	 Return the indexed attestation corresponding to ``attestation``.
//	 """
//	 attesting_indices = get_attesting_indices(state, attestation.data, attestation.aggregation_bits)
//
//	 return IndexedAttestation(
//	     attesting_indices=sorted(attesting_indices),
//	     data=attestation.data,
//	     signature=attestation.signature,
//	 )
func ConvertToIndexed(_ context.Context, attestation ethpb.Att, committees ...[]primitives.ValidatorIndex) (ethpb.IndexedAtt, error) {
	attIndices, err := AttestingIndices(attestation, committees...)
	if err != nil {
		return nil, err
	}

	sort.Slice(attIndices, func(i, j int) bool {
		return attIndices[i] < attIndices[j]
	})

	if attestation.Version() >= version.Electra {
		return &ethpb.IndexedAttestationElectra{
			Data:             attestation.GetData(),
			Signature:        attestation.GetSignature(),
			AttestingIndices: attIndices,
		}, nil
	}
	return &ethpb.IndexedAttestation{
		Data:             attestation.GetData(),
		Signature:        attestation.GetSignature(),
		AttestingIndices: attIndices,
	}, nil
}

// AttestingIndices returns the attesting participants indices from the attestation data.
// Committees are provided as an argument rather than an imported implementation from the spec definition.
// Having committees as an argument allows for re-use of beacon committees when possible.
//
// Spec pseudocode definition (Electra version):
//
//	def get_attesting_indices(state: BeaconState, attestation: Attestation) -> Set[ValidatorIndex]:
//	    """
//	    Return the set of attesting indices corresponding to ``aggregation_bits`` and ``committee_bits``.
//	    """
//	    output: Set[ValidatorIndex] = set()
//	    committee_indices = get_committee_indices(attestation.committee_bits)
//	    committee_offset = 0
//	    for index in committee_indices:
//	        committee = get_beacon_committee(state, attestation.data.slot, index)
//	        committee_attesters = set(
//	        index for i, index in enumerate(committee) if attestation.aggregation_bits[committee_offset + i])
//	        output = output.union(committee_attesters)
//
//	        committee_offset += len(committee)
//
//	    return output
func AttestingIndices(att ethpb.Att, committees ...[]primitives.ValidatorIndex) ([]uint64, error) {
	if len(committees) == 0 {
		return []uint64{}, nil
	}

	aggBits := att.GetAggregationBits()

	if att.Version() < version.Electra {
		return attestingIndicesPhase0(aggBits, committees[0])
	}

	committeesLen := 0
	for _, c := range committees {
		committeesLen += len(c)
	}
	if aggBits.Len() != uint64(committeesLen) {
		return nil, fmt.Errorf("bitfield length %d is not equal to committee length %d", aggBits.Len(), committeesLen)
	}

	attesters := make([]uint64, 0, aggBits.Count())
	committeeOffset := 0
	for _, c := range committees {
		committeeAttesters := make([]uint64, 0, len(c))
		for i, vi := range c {
			if aggBits.BitAt(uint64(committeeOffset + i)) {
				committeeAttesters = append(committeeAttesters, uint64(vi))
			}
		}
		attesters = append(attesters, committeeAttesters...)
		committeeOffset += len(c)
	}

	slices.Sort(attesters)
	return slices.Compact(attesters), nil
}

// VerifyIndexedAttestationSig this helper function performs the last part of the
// spec indexed attestation validation starting at Verify aggregate signature
// comment.
//
// Spec pseudocode definition:
//
//	def is_valid_indexed_attestation(state: BeaconState, indexed_attestation: IndexedAttestation) -> bool:
//	 """
//	 Check if ``indexed_attestation`` is not empty, has sorted and unique indices and has a valid aggregate signature.
//	 """
//	 # Verify indices are sorted and unique
//	 indices = indexed_attestation.attesting_indices
//	 if len(indices) == 0 or not indices == sorted(set(indices)):
//	     return False
//	 # Verify aggregate signature
//	 pubkeys = [state.validators[i].pubkey for i in indices]
//	 domain = get_domain(state, DOMAIN_BEACON_ATTESTER, indexed_attestation.data.target.epoch)
//	 signing_root = compute_signing_root(indexed_attestation.data, domain)
//	 return bls.FastAggregateVerify(pubkeys, signing_root, indexed_attestation.signature)
func VerifyIndexedAttestationSig(ctx context.Context, indexedAtt ethpb.IndexedAtt, pubKeys []bls.PublicKey, domain []byte) error {
	_, span := trace.StartSpan(ctx, "attestationutil.VerifyIndexedAttestationSig")
	defer span.End()
	indices := indexedAtt.GetAttestingIndices()

	messageHash, err := signing.ComputeSigningRoot(indexedAtt.GetData(), domain)
	if err != nil {
		return errors.Wrap(err, "could not get signing root of object")
	}

	sig, err := bls.SignatureFromBytes(indexedAtt.GetSignature())
	if err != nil {
		return errors.Wrap(err, "could not convert bytes to signature")
	}

	voted := len(indices) > 0
	if voted && !sig.FastAggregateVerify(pubKeys, messageHash) {
		return signing.ErrSigFailedToVerify
	}
	return nil
}

// IsValidAttestationIndices this helper function performs the first part of the
// spec indexed attestation validation starting at Check if “indexed_attestation“
// comment and ends at Verify aggregate signature comment.
//
// Spec pseudocode definition:
//
//	def is_valid_indexed_attestation(state: BeaconState, indexed_attestation: IndexedAttestation) -> bool:
//	  """
//	  Check if ``indexed_attestation`` is not empty, has sorted and unique indices and has a valid aggregate signature.
//	  """
//	  # Verify indices are sorted and unique
//	  indices = indexed_attestation.attesting_indices
//	  if len(indices) == 0 or not indices == sorted(set(indices)):
//	      return False
//	  # Verify aggregate signature
//	  pubkeys = [state.validators[i].pubkey for i in indices]
//	  domain = get_domain(state, DOMAIN_BEACON_ATTESTER, indexed_attestation.data.target.epoch)
//	  signing_root = compute_signing_root(indexed_attestation.data, domain)
//	  return bls.FastAggregateVerify(pubkeys, signing_root, indexed_attestation.signature)
func IsValidAttestationIndices(ctx context.Context, indexedAttestation ethpb.IndexedAtt) error {
	_, span := trace.StartSpan(ctx, "attestationutil.IsValidAttestationIndices")
	defer span.End()

	if indexedAttestation == nil || indexedAttestation.IsNil() || indexedAttestation.GetData().Target == nil || indexedAttestation.GetData().Source == nil {
		return errors.New("nil or missing indexed attestation data")
	}

	indices := indexedAttestation.GetAttestingIndices()
	if len(indices) == 0 {
		return errors.New("expected non-empty attesting indices")
	}
	if indexedAttestation.Version() < version.Electra {
		maxLength := params.BeaconConfig().MaxValidatorsPerCommittee
		if uint64(len(indices)) > maxLength {
			return fmt.Errorf("validator indices count exceeds MAX_VALIDATORS_PER_COMMITTEE, %d > %d", len(indices), maxLength)
		}
	} else {
		maxLength := params.BeaconConfig().MaxValidatorsPerCommittee * params.BeaconConfig().MaxCommitteesPerSlot
		if uint64(len(indices)) > maxLength {
			return fmt.Errorf("validator indices count exceeds MAX_VALIDATORS_PER_COMMITTEE * MAX_COMMITTEES_PER_SLOT, %d > %d", len(indices), maxLength)
		}
	}
	for i := 1; i < len(indices); i++ {
		if indices[i-1] >= indices[i] {
			return errors.New("attesting indices is not uniquely sorted")
		}
	}
	return nil
}

// AttDataIsEqual this function performs an equality check between 2 attestation data, if they're unequal, it will return false.
func AttDataIsEqual(attData1, attData2 *ethpb.AttestationData) bool {
	if attData1.Slot != attData2.Slot {
		return false
	}
	if attData1.CommitteeIndex != attData2.CommitteeIndex {
		return false
	}
	if !bytes.Equal(attData1.BeaconBlockRoot, attData2.BeaconBlockRoot) {
		return false
	}
	if attData1.Source.Epoch != attData2.Source.Epoch {
		return false
	}
	if !bytes.Equal(attData1.Source.Root, attData2.Source.Root) {
		return false
	}
	if attData1.Target.Epoch != attData2.Target.Epoch {
		return false
	}
	if !bytes.Equal(attData1.Target.Root, attData2.Target.Root) {
		return false
	}
	return true
}

// CheckPointIsEqual performs an equality check between 2 check points, returns false if unequal.
func CheckPointIsEqual(checkPt1, checkPt2 *ethpb.Checkpoint) bool {
	if checkPt1.Epoch != checkPt2.Epoch {
		return false
	}
	if !bytes.Equal(checkPt1.Root, checkPt2.Root) {
		return false
	}
	return true
}

// attestingIndicesPhase0 returns the attesting participants indices from the attestation data.
// Committees are provided as an argument rather than an imported implementation from the spec definition.
// Having committees as an argument allows for re-use of beacon committees when possible.
//
// Spec pseudocode definition (Phase0 version):
//
//	def get_attesting_indices(state: BeaconState, attestation: Attestation) -> Set[ValidatorIndex]:
//	    """
//	    Return the set of attesting indices corresponding to ``data`` and ``bits``.
//	    """
//	    committee = get_beacon_committee(state, attestation.data.slot, attestation.data.index)
//	    return set(index for i, index in enumerate(committee) if attestation.aggregation_bits[i])
func attestingIndicesPhase0(aggBits bitfield.Bitlist, committee []primitives.ValidatorIndex) ([]uint64, error) {
	if aggBits.Len() != uint64(len(committee)) {
		return nil, fmt.Errorf("bitfield length %d is not equal to committee length %d", aggBits.Len(), len(committee))
	}
	indices := make([]uint64, 0, aggBits.Count())
	p := aggBits.BitIndices()
	for _, idx := range p {
		if idx < len(committee) {
			indices = append(indices, uint64(committee[idx]))
		}
	}
	return indices, nil
}
