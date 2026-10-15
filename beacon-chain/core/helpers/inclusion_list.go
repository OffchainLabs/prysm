package helpers

import (
	"context"
	"slices"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

var (
	errNilInclusionList        = errors.New("nil inclusion list")
	errInclusionListPreHeze    = errors.New("inclusion list slot is before the Heze fork")
	errNoInclusionListMembers  = errors.New("no beacon committee members for inclusion list committee")
	errInclusionListEpochAhead = errors.New("inclusion list assignment epoch is after the next epoch")
)

// InclusionListCommittee returns the inclusion list committee for the slot, as in get_inclusion_list_committee.
func InclusionListCommittee(ctx context.Context, st state.ReadOnlyBeaconState, slot primitives.Slot) ([]primitives.ValidatorIndex, error) {
	epoch := slots.ToEpoch(slot)
	if epoch < params.BeaconConfig().HezeForkEpoch {
		return nil, errInclusionListPreHeze
	}
	activeCount, err := ActiveValidatorCount(ctx, st, epoch)
	if err != nil {
		return nil, errors.Wrap(err, "could not get active validator count")
	}
	var indices []primitives.ValidatorIndex
	for i := range SlotCommitteeCount(activeCount) {
		committee, err := BeaconCommitteeFromState(ctx, st, slot, primitives.CommitteeIndex(i))
		if err != nil {
			return nil, errors.Wrapf(err, "could not get beacon committee %d", i)
		}
		indices = append(indices, committee...)
	}
	if len(indices) == 0 {
		return nil, errNoInclusionListMembers
	}
	committee := make([]primitives.ValidatorIndex, fieldparams.InclusionListCommitteeSize)
	for i := range committee {
		committee[i] = indices[i%len(indices)]
	}
	return committee, nil
}

// InclusionListCommitteeAssignment returns the slot in the epoch where the validator is in the inclusion list committee, as in get_inclusion_list_committee_assignment.
func InclusionListCommitteeAssignment(ctx context.Context, st state.ReadOnlyBeaconState, epoch primitives.Epoch, validatorIndex primitives.ValidatorIndex) (primitives.Slot, bool, error) {
	if epoch > slots.ToEpoch(st.Slot())+1 {
		return 0, false, errInclusionListEpochAhead
	}
	start, err := slots.EpochStart(epoch)
	if err != nil {
		return 0, false, err
	}
	for slot := start; slot < start+params.BeaconConfig().SlotsPerEpoch; slot++ {
		committee, err := InclusionListCommittee(ctx, st, slot)
		if err != nil {
			return 0, false, err
		}
		if slices.Contains(committee, validatorIndex) {
			return slot, true, nil
		}
	}
	return 0, false, nil
}

// InclusionListSigningDomain returns the inclusion list domain from the fork version scheduled at the epoch, not from the state fork.
func InclusionListSigningDomain(epoch primitives.Epoch, genesisValidatorsRoot []byte) ([]byte, error) {
	fork, err := params.Fork(epoch)
	if err != nil {
		return nil, errors.Wrap(err, "could not get fork")
	}
	return signing.ComputeDomain(params.BeaconConfig().DomainInclusionListCommittee, fork.CurrentVersion, genesisValidatorsRoot)
}

// VerifyInclusionListSignature verifies the signature of the inclusion list, as in is_valid_inclusion_list_signature.
func VerifyInclusionListSignature(ctx context.Context, st state.ReadOnlyBeaconState, signed *ethpb.SignedInclusionList) error {
	if signed == nil || signed.Message == nil {
		return errNilInclusionList
	}
	if len(signed.Signature) != fieldparams.BLSSignatureLength {
		return errors.Errorf("inclusion list signature length is %d, want %d", len(signed.Signature), fieldparams.BLSSignatureLength)
	}
	val, err := st.ValidatorAtIndexReadOnly(signed.Message.ValidatorIndex)
	if err != nil {
		return errors.Wrap(err, "could not get validator")
	}
	pubkey := val.PublicKey()
	pub, err := bls.PublicKeyFromBytes(pubkey[:])
	if err != nil {
		return errors.Wrap(err, "could not convert public key")
	}
	sig, err := bls.SignatureFromBytes(signed.Signature)
	if err != nil {
		return errors.Wrap(err, "could not convert signature")
	}
	domain, err := InclusionListSigningDomain(slots.ToEpoch(signed.Message.Slot), st.GenesisValidatorsRoot())
	if err != nil {
		return err
	}
	root, err := signing.ComputeSigningRoot(signed.Message, domain)
	if err != nil {
		return errors.Wrap(err, "could not compute signing root")
	}
	if !sig.Verify(pub, root[:]) {
		return signing.ErrSigFailedToVerify
	}
	return nil
}

// ProposalInclusionLists is the local inclusion list view of a proposal at a slot.
type ProposalInclusionLists struct {
	store         *cache.InclusionListStore
	st            state.ReadOnlyBeaconState
	slot          primitives.Slot
	dependentRoot [32]byte
}

// NewProposalInclusionLists returns the inclusion list view of a proposal at slot from st, or nil before Heze.
func NewProposalInclusionLists(ctx context.Context, db GenesisBlockRootReader, store *cache.InclusionListStore, st state.ReadOnlyBeaconState, slot primitives.Slot) (*ProposalInclusionLists, error) {
	if slot == 0 || slots.ToEpoch(slot-1) < params.BeaconConfig().HezeForkEpoch {
		return nil, nil
	}
	dependentRoot, err := ProposerDependentRootOrGenesis(ctx, db, st, slot-1)
	if err != nil {
		return nil, errors.Wrap(err, "could not get inclusion list dependent root")
	}
	return &ProposalInclusionLists{store: store, st: st, slot: slot - 1, dependentRoot: dependentRoot}, nil
}

// Transactions returns get_inclusion_list_transactions for the payload attributes.
func (p *ProposalInclusionLists) Transactions() [][]byte {
	if p == nil {
		return [][]byte{}
	}
	txs := p.store.Transactions(p.slot, p.dependentRoot, false)
	if txs == nil {
		return [][]byte{}
	}
	return txs
}

// Bits returns get_inclusion_list_bits for the self-build bid.
func (p *ProposalInclusionLists) Bits(ctx context.Context) (bitfield.Bitvector16, error) {
	if p == nil {
		return bitfield.NewBitvector16(), nil
	}
	committee, err := InclusionListCommittee(ctx, p.st, p.slot)
	if err != nil {
		return nil, errors.Wrap(err, "could not get inclusion list committee")
	}
	return p.store.Bits(committee, p.slot, p.dependentRoot, false), nil
}

// IsBitsInclusive runs is_inclusion_list_bits_inclusive for a bid with bits. It is true before Heze.
func (p *ProposalInclusionLists) IsBitsInclusive(ctx context.Context, bits bitfield.Bitvector16) (bool, error) {
	if p == nil {
		return true, nil
	}
	committee, err := InclusionListCommittee(ctx, p.st, p.slot)
	if err != nil {
		return false, errors.Wrap(err, "could not get inclusion list committee")
	}
	return p.store.IsBitsInclusive(committee, p.slot, p.dependentRoot, bits, false), nil
}
