// EIP-8243 batch attestation composition path on the validator client.
//
// This file provides three primitives that an out-of-tree operator-side
// coordinator can drive:
//
//   1. SignBatchSeal       — each attester pre-signs a BatchSealPreimage
//                            authorizing a specific batcher for an upcoming
//                            (slot, committee) duty. Designed to be called at
//                            epoch start so seals don't sit on the slot
//                            critical path.
//
//   2. SignBatcherComposition — the elected batcher signs the BatcherPreimage
//                               binding the composition (aggregation_bits +
//                               data_root) to itself under DomainBatcher.
//
//   3. SubmitBatchedAttestation — convenience wrapper that, given the
//                                 collected seals/sigs and the duty data,
//                                 produces the BatchAttestation and calls the
//                                 beacon node's ProposeBatchAttestation RPC.
//
// The mechanism by which seals reach the batcher (in-process, keymanager
// RPC, sidecar service) is operator policy and lives outside this file.

package client

import (
	"context"

	"github.com/OffchainLabs/go-bitfield"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	validatorpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/validator-client"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// BatchContribution carries a single attester's contribution to a batch:
// their AttestationData signature (DomainBeaconAttester) and their seal
// signature (DomainBatchAttester). The batcher aggregates these into the
// batch's `signature` and `batch_seal` fields respectively.
type BatchContribution struct {
	AttesterIndex        primitives.ValidatorIndex
	AttesterCommitteePos int    // 0-based position within the committee
	AttestationSignature []byte // BLS sig over att.data under DomainBeaconAttester
	Seal                 []byte // BLS sig over BatchSealPreimage under DomainBatchAttester
}

// SignBatchSeal authorizes `batcher` to compose this validator's attestation
// for the (slot, committee_index) duty. The preimage is bound only to
// (slot, ci, batcher) so a seal can be produced once at epoch start and
// reused regardless of the eventual aggregation_bits — see EIP §"Why bind
// the seal to (slot, committee_index, batcher) only?".
func (v *validator) SignBatchSeal(
	ctx context.Context,
	pubKey [fieldparams.BLSPubkeyLength]byte,
	slot primitives.Slot,
	committeeIndex primitives.CommitteeIndex,
	batcher primitives.ValidatorIndex,
) ([]byte, error) {
	ctx, span := trace.StartSpan(ctx, "validator.SignBatchSeal")
	defer span.End()

	preimage := &ethpb.BatchSealPreimage{
		Slot:           slot,
		CommitteeIndex: committeeIndex,
		Batcher:        batcher,
	}
	signingRoot, sigDomain, err := v.batchSealSigningRoot(ctx, slot, preimage)
	if err != nil {
		return nil, err
	}
	sig, err := v.km.Sign(ctx, &validatorpb.SignRequest{
		PublicKey:       pubKey[:],
		SigningRoot:     signingRoot[:],
		SignatureDomain: sigDomain,
		Object:          &validatorpb.SignRequest_BatchSealPreimage{BatchSealPreimage: preimage},
		SigningSlot:     slot,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to sign batch seal")
	}
	return sig.Marshal(), nil
}

// SignBatcherComposition produces the batcher's signature binding it to a
// specific (slot, ci, aggregation_bits, data_root) composition. Includes
// data_root so the signature is not replayable across
// conflicting AttestationData.
func (v *validator) SignBatcherComposition(
	ctx context.Context,
	pubKey [fieldparams.BLSPubkeyLength]byte,
	slot primitives.Slot,
	committeeIndex primitives.CommitteeIndex,
	aggregationBits bitfield.Bitlist,
	data *ethpb.AttestationData,
) ([]byte, error) {
	ctx, span := trace.StartSpan(ctx, "validator.SignBatcherComposition")
	defer span.End()

	dataRoot, err := data.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute data hash tree root")
	}
	preimage := &ethpb.BatcherPreimage{
		Slot:            slot,
		CommitteeIndex:  committeeIndex,
		AggregationBits: bytesutil.SafeCopyBytes(aggregationBits),
		DataRoot:        dataRoot[:],
	}
	signingRoot, sigDomain, err := v.batcherCompositionSigningRoot(ctx, slot, preimage)
	if err != nil {
		return nil, err
	}
	sig, err := v.km.Sign(ctx, &validatorpb.SignRequest{
		PublicKey:       pubKey[:],
		SigningRoot:     signingRoot[:],
		SignatureDomain: sigDomain,
		Object:          &validatorpb.SignRequest_BatcherPreimage{BatcherPreimage: preimage},
		SigningSlot:     slot,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to sign batcher composition")
	}
	return sig.Marshal(), nil
}

// SubmitBatchedAttestation composes a BatchAttestation from the collected
// contributions, signs the composition with the batcher key, and routes it
// through the standard ProposeBatchAttestation RPC.
//
// committeeLength is the size of the (slot, committeeIndex) committee — used
// to allocate the aggregation_bits bitlist.
func (v *validator) SubmitBatchedAttestation(
	ctx context.Context,
	batcherPubKey [fieldparams.BLSPubkeyLength]byte,
	batcher primitives.ValidatorIndex,
	committeeIndex primitives.CommitteeIndex,
	committeeLength uint64,
	data *ethpb.AttestationData,
	contributions []BatchContribution,
) (*ethpb.AttestResponse, error) {
	ctx, span := trace.StartSpan(ctx, "validator.SubmitBatchedAttestation")
	defer span.End()

	if len(contributions) < 2 {
		return nil, errors.New("batch attestation requires >= 2 contributions; fall back to SingleAttestation")
	}
	if data == nil {
		return nil, errors.New("nil attestation data")
	}

	bits := bitfield.NewBitlist(committeeLength)
	attSigs := make([][]byte, 0, len(contributions))
	sealSigs := make([][]byte, 0, len(contributions))
	batcherInSet := false
	for _, c := range contributions {
		if uint64(c.AttesterCommitteePos) >= committeeLength {
			return nil, errors.Errorf("attester position %d out of committee range [0, %d)",
				c.AttesterCommitteePos, committeeLength)
		}
		bits.SetBitAt(uint64(c.AttesterCommitteePos), true)
		attSigs = append(attSigs, c.AttestationSignature)
		sealSigs = append(sealSigs, c.Seal)
		if c.AttesterIndex == batcher {
			batcherInSet = true
		}
	}
	if !batcherInSet {
		return nil, errors.New("batcher is not among the batch contributors")
	}

	aggAttSig, err := bls.AggregateCompressedSignatures(attSigs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to aggregate attestation signatures")
	}
	aggSeal, err := bls.AggregateCompressedSignatures(sealSigs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to aggregate batch seals")
	}
	batcherSig, err := v.SignBatcherComposition(ctx, batcherPubKey, data.Slot, committeeIndex, bits, data)
	if err != nil {
		return nil, err
	}

	batch := &ethpb.BatchAttestation{
		CommitteeIndex:   committeeIndex,
		AggregationBits:  bits,
		Data:             data,
		Signature:        aggAttSig.Marshal(),
		Batcher:          batcher,
		BatchSeal:        aggSeal.Marshal(),
		BatcherSignature: batcherSig,
	}
	return v.validatorClient.ProposeBatchAttestation(ctx, batch)
}

// batchSealSigningRoot computes the signing root + signature-domain bytes for
// a BatchSealPreimage. Uses DomainBatchAttester at the slot's target epoch.
func (v *validator) batchSealSigningRoot(
	ctx context.Context, slot primitives.Slot, preimage *ethpb.BatchSealPreimage,
) ([32]byte, []byte, error) {
	dom, err := v.domainData(ctx, slots.ToEpoch(slot), params.BeaconConfig().DomainBatchAttester[:])
	if err != nil {
		return [32]byte{}, nil, errors.Wrap(err, "failed to fetch DomainBatchAttester")
	}
	root, err := signingRootForSSZ(preimage, dom.SignatureDomain)
	if err != nil {
		return [32]byte{}, nil, err
	}
	return root, dom.SignatureDomain, nil
}

// batcherCompositionSigningRoot computes the signing root + signature-domain
// bytes for a BatcherPreimage. Uses DomainBatcher at the slot's target epoch.
func (v *validator) batcherCompositionSigningRoot(
	ctx context.Context, slot primitives.Slot, preimage *ethpb.BatcherPreimage,
) ([32]byte, []byte, error) {
	dom, err := v.domainData(ctx, slots.ToEpoch(slot), params.BeaconConfig().DomainBatcher[:])
	if err != nil {
		return [32]byte{}, nil, errors.Wrap(err, "failed to fetch DomainBatcher")
	}
	root, err := signingRootForSSZ(preimage, dom.SignatureDomain)
	if err != nil {
		return [32]byte{}, nil, err
	}
	return root, dom.SignatureDomain, nil
}

// signingRootForSSZ wraps signing.ComputeSigningRoot for a fastssz-marshalable
// object. Kept local to batch_attest.go to avoid disturbing other call sites.
func signingRootForSSZ(obj interface{ HashTreeRoot() ([32]byte, error) }, domain []byte) ([32]byte, error) {
	root, err := obj.HashTreeRoot()
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "failed to compute object hash tree root")
	}
	container := &ethpb.SigningData{ObjectRoot: root[:], Domain: domain}
	return container.HashTreeRoot()
}
