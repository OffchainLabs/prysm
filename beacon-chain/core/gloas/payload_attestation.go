package gloas

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"slices"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensus_types "github.com/OffchainLabs/prysm/v7/consensus-types"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// ProcessPayloadAttestations validates payload attestations in a block body.
// Spec v1.7.0-alpha.0 (pseudocode):
// process_payload_attestation(state: BeaconState, payload_attestation: PayloadAttestation):
//
//	data = payload_attestation.data
//	assert data.beacon_block_root == state.latest_block_header.parent_root
//	assert data.slot + 1 == state.slot
//	indexed = get_indexed_payload_attestation(state, data.slot, payload_attestation)
//	assert is_valid_indexed_payload_attestation(state, indexed)
func ProcessPayloadAttestations(ctx context.Context, st state.BeaconState, body interfaces.ReadOnlyBeaconBlockBody) error {
	atts, err := body.PayloadAttestations()
	if err != nil {
		return errors.Wrap(err, "failed to get payload attestations from block body")
	}
	if len(atts) == 0 {
		return nil
	}

	header := st.LatestBlockHeader()

	for i, att := range atts {
		data := att.Data
		if !bytes.Equal(data.BeaconBlockRoot, header.ParentRoot) {
			return fmt.Errorf("payload attestation %d has wrong parent: got %x want %x", i, data.BeaconBlockRoot, header.ParentRoot)
		}

		dataSlot, err := data.Slot.SafeAdd(1)
		if err != nil {
			return errors.Wrapf(err, "payload attestation %d has invalid slot addition", i)
		}
		if dataSlot != st.Slot() {
			return fmt.Errorf("payload attestation %d has wrong slot: got %d want %d", i, data.Slot+1, st.Slot())
		}

		indexed, err := indexedPayloadAttestation(ctx, st, att)
		if err != nil {
			return errors.Wrapf(err, "payload attestation %d failed to convert to indexed form", i)
		}
		if err := validIndexedPayloadAttestation(st, indexed); err != nil {
			return errors.Wrapf(err, "payload attestation %d failed to verify indexed form", i)
		}
	}
	return nil
}

// indexedPayloadAttestation converts a payload attestation into its indexed form.
func indexedPayloadAttestation(ctx context.Context, st state.ReadOnlyBeaconState, att *eth.PayloadAttestation) (*consensus_types.IndexedPayloadAttestation, error) {
	committee, err := payloadCommittee(ctx, st, att.Data.Slot)
	if err != nil {
		return nil, err
	}
	indices := make([]primitives.ValidatorIndex, 0, len(committee))
	for i, idx := range committee {
		if att.AggregationBits.BitAt(uint64(i)) {
			indices = append(indices, idx)
		}
	}
	slices.Sort(indices)

	return &consensus_types.IndexedPayloadAttestation{
		AttestingIndices: indices,
		Data:             att.Data,
		Signature:        att.Signature,
	}, nil
}

// payloadCommittee returns the payload timeliness committee for a given slot for the state.
// Spec v1.7.0-alpha.0 (pseudocode):
// get_ptc(state: BeaconState, slot: Slot) -> Vector[ValidatorIndex, PTC_SIZE]:
//
//	epoch = compute_epoch_at_slot(slot)
//	seed = hash(get_seed(state, epoch, DOMAIN_PTC_ATTESTER) + uint_to_bytes(slot))
//	indices = []
//	committees_per_slot = get_committee_count_per_slot(state, epoch)
//	for i in range(committees_per_slot):
//	  committee = get_beacon_committee(state, slot, CommitteeIndex(i))
//	  indices.extend(committee)
//	return compute_balance_weighted_selection(state, indices, seed, size=PTC_SIZE, shuffle_indices=False)
func payloadCommittee(ctx context.Context, st state.ReadOnlyBeaconState, slot primitives.Slot) ([]primitives.ValidatorIndex, error) {
	epoch := slots.ToEpoch(slot)
	seed, err := ptcSeed(st, epoch, slot)
	if err != nil {
		return nil, err
	}

	activeCount, err := helpers.ActiveValidatorCount(ctx, st, epoch)
	if err != nil {
		return nil, err
	}

	committeesPerSlot := helpers.SlotCommitteeCount(activeCount)

	selected := make([]primitives.ValidatorIndex, 0, fieldparams.PTCSize)
	var i uint64
	for uint64(len(selected)) < fieldparams.PTCSize {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		for committeeIndex := primitives.CommitteeIndex(0); committeeIndex < primitives.CommitteeIndex(committeesPerSlot); committeeIndex++ {
			if uint64(len(selected)) >= fieldparams.PTCSize {
				break
			}

			committee, err := helpers.BeaconCommitteeFromState(ctx, st, slot, committeeIndex)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get beacon committee %d", committeeIndex)
			}

			selected, i, err = selectByBalanceFill(ctx, st, committee, seed, selected, i)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to sample beacon committee %d", committeeIndex)
			}
		}
	}

	return selected, nil
}

// ptcSeed computes the seed for the payload timeliness committee.
func ptcSeed(st state.ReadOnlyBeaconState, epoch primitives.Epoch, slot primitives.Slot) ([32]byte, error) {
	seed, err := helpers.Seed(st, epoch, params.BeaconConfig().DomainPTCAttester)
	if err != nil {
		return [32]byte{}, err
	}
	return hash.Hash(append(seed[:], bytesutil.Bytes8(uint64(slot))...)), nil
}

// selectByBalance selects a balance-weighted subset of input candidates.
// Spec v1.7.0-alpha.0 (pseudocode):
// compute_balance_weighted_selection(state, indices, seed, size, shuffle_indices):
// Note: shuffle_indices is false for PTC.
//
//	total = len(indices); selected = []; i = 0
//	while len(selected) < size:
//	  next = i % total
//	  if shuffle_indices: next = compute_shuffled_index(next, total, seed)
//	  if compute_balance_weighted_acceptance(state, indices[next], seed, i):
//	    selected.append(indices[next])
//	  i += 1
func selectByBalanceFill(
	ctx context.Context,
	st state.ReadOnlyBeaconState,
	candidates []primitives.ValidatorIndex,
	seed [32]byte,
	selected []primitives.ValidatorIndex,
	i uint64,
) ([]primitives.ValidatorIndex, uint64, error) {
	hashFunc := hash.CustomSHA256Hasher()
	// Pre-allocate buffer for hash input: seed (32 bytes) + round counter (8 bytes).
	var buf [40]byte
	copy(buf[:], seed[:])
	maxBalance := params.BeaconConfig().MaxEffectiveBalanceElectra

	for _, idx := range candidates {
		if ctx.Err() != nil {
			return nil, i, ctx.Err()
		}

		ok, err := acceptByBalance(st, idx, buf[:], hashFunc, maxBalance, i)
		if err != nil {
			return nil, i, err
		}
		if ok {
			selected = append(selected, idx)
		}
		if uint64(len(selected)) == fieldparams.PTCSize {
			break
		}
		i++
	}

	return selected, i, nil
}

// acceptByBalance determines if a validator is accepted based on its effective balance.
// Spec v1.7.0-alpha.0 (pseudocode):
// compute_balance_weighted_acceptance(state, index, seed, i):
//
//	MAX_RANDOM_VALUE = 2**16 - 1
//	random_bytes = hash(seed + uint_to_bytes(i // 16))
//	offset = i % 16 * 2
//	random_value = bytes_to_uint64(random_bytes[offset:offset+2])
//	effective_balance = state.validators[index].effective_balance
//	return effective_balance * MAX_RANDOM_VALUE >= MAX_EFFECTIVE_BALANCE_ELECTRA * random_value
func acceptByBalance(st state.ReadOnlyBeaconState, idx primitives.ValidatorIndex, seedBuf []byte, hashFunc func([]byte) [32]byte, maxBalance uint64, round uint64) (bool, error) {
	// Reuse the seed buffer by overwriting the last 8 bytes with the round counter.
	binary.LittleEndian.PutUint64(seedBuf[len(seedBuf)-8:], round/16)
	random := hashFunc(seedBuf)
	offset := (round % 16) * 2
	randomValue := uint64(binary.LittleEndian.Uint16(random[offset : offset+2])) // 16-bit draw per spec

	val, err := st.ValidatorAtIndex(idx)
	if err != nil {
		return false, errors.Wrapf(err, "validator %d", idx)
	}

	return val.EffectiveBalance*fieldparams.MaxRandomValueElectra >= maxBalance*randomValue, nil
}

// validIndexedPayloadAttestation verifies the signature of an indexed payload attestation.
// Spec v1.7.0-alpha.0 (pseudocode):
// is_valid_indexed_payload_attestation(state: BeaconState, indexed_payload_attestation: IndexedPayloadAttestation) -> bool:
//
//	indices = indexed_payload_attestation.attesting_indices
//	return len(indices) > 0 and indices == sorted(indices) and
//	  bls.FastAggregateVerify(
//	    [state.validators[i].pubkey for i in indices],
//	    compute_signing_root(indexed_payload_attestation.data, get_domain(state, DOMAIN_PTC_ATTESTER, compute_epoch_at_slot(attestation.data.slot)),
//	    indexed_payload_attestation.signature,
//	  )
func validIndexedPayloadAttestation(st state.ReadOnlyBeaconState, att *consensus_types.IndexedPayloadAttestation) error {
	indices := att.AttestingIndices
	if len(indices) == 0 || !slices.IsSorted(indices) {
		return errors.New("attesting indices empty or unsorted")
	}

	pubkeys := make([]bls.PublicKey, len(indices))
	for i, idx := range indices {
		val, err := st.ValidatorAtIndexReadOnly(idx)
		if err != nil {
			return errors.Wrapf(err, "validator %d", idx)
		}
		keyBytes := val.PublicKey()
		key, err := bls.PublicKeyFromBytes(keyBytes[:])
		if err != nil {
			return errors.Wrapf(err, "pubkey %d", idx)
		}
		pubkeys[i] = key
	}

	domain, err := signing.Domain(st.Fork(), slots.ToEpoch(att.Data.Slot), params.BeaconConfig().DomainPTCAttester, st.GenesisValidatorsRoot())
	if err != nil {
		return err
	}
	root, err := signing.ComputeSigningRoot(att.Data, domain)
	if err != nil {
		return err
	}
	sig, err := bls.SignatureFromBytes(att.Signature)
	if err != nil {
		return err
	}

	if !sig.FastAggregateVerify(pubkeys, root) {
		return errors.New("invalid signature")
	}
	return nil
}
