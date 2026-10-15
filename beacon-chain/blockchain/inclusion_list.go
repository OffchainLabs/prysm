package blockchain

import (
	"context"
	"fmt"
	"slices"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	prysmTime "github.com/OffchainLabs/prysm/v7/time"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Errors returned by OnInclusionList, one per on_inclusion_list check, so that gossip can map each one to IGNORE or REJECT.
var (
	ErrNilInclusionList                       = errors.New("nil inclusion list")
	ErrInclusionListPreHeze                   = errors.New("inclusion list slot is before the Heze fork")
	ErrInclusionListDependentRootLength       = errors.New("inclusion list dependent root is not 32 bytes")
	ErrInclusionListOutOfWindow               = errors.New("inclusion list slot is outside the retention window")
	ErrInclusionListNoTransactions            = errors.New("inclusion list has no transaction bytes")
	ErrInclusionListTooLarge                  = errors.New("inclusion list transactions exceed the maximum size")
	ErrInclusionListEmptyTransaction          = errors.New("inclusion list contains an empty transaction")
	ErrInclusionListUnknownDependentBlock     = errors.New("inclusion list dependent block is unknown")
	ErrInclusionListDependentBlockTooLate     = errors.New("inclusion list dependent block is after the shuffling dependent slot")
	ErrInclusionListInvalidDependentRoot      = errors.New("inclusion list dependent block is not a possible dependent block")
	ErrInclusionListDependentStateUnavailable = errors.New("inclusion list dependent state is not available")
	ErrInclusionListNotInCommittee            = errors.New("inclusion list validator is not in the inclusion list committee")
	ErrInclusionListInvalidSignature          = errors.New("inclusion list signature is not valid")
)

// InclusionListReceiver processes inclusion lists into the shared inclusion list store.
type InclusionListReceiver interface {
	OnInclusionList(ctx context.Context, signed *ethpb.SignedInclusionList) error
}

// InclusionListStore returns the inclusion list store that the node shares between fork choice, gossip, and the API.
func (s *Service) InclusionListStore() *cache.InclusionListStore {
	return s.cfg.InclusionListStore
}

// OnInclusionList runs on_inclusion_list. It changes the store only after every check passes.
func (s *Service) OnInclusionList(ctx context.Context, signed *ethpb.SignedInclusionList) error {
	ctx, span := trace.StartSpan(ctx, "blockChain.OnInclusionList")
	defer span.End()

	if signed == nil || signed.Message == nil {
		return ErrNilInclusionList
	}
	il := signed.Message
	if len(il.DependentRoot) != fieldparams.RootLength {
		return ErrInclusionListDependentRootLength
	}
	epoch := slots.ToEpoch(il.Slot)
	if epoch < params.BeaconConfig().HezeForkEpoch {
		return ErrInclusionListPreHeze
	}

	now := prysmTime.Now()
	currentSlot := slots.At(s.genesisTime, now)
	s.cfg.InclusionListStore.Prune(currentSlot)

	// The slot must be within the retention window. Gossip accepts a list for the next slot within
	// MAXIMUM_GOSSIP_CLOCK_DISPARITY, so the window does too.
	latestSlot := slots.At(s.genesisTime, now.Add(params.BeaconConfig().MaximumGossipClockDisparityDuration()))
	if il.Slot > latestSlot || il.Slot+primitives.Slot(params.BeaconConfig().MinSlotsForInclusionListsRequests) < currentSlot {
		return errors.Wrapf(ErrInclusionListOutOfWindow, "slot %d, current slot %d", il.Slot, currentSlot)
	}

	// The transactions must be non-empty and not exceed the maximum size, and every transaction must be non-empty.
	txs := il.Transactions.Slice()
	var size uint64
	for _, tx := range txs {
		size += uint64(len(tx))
	}
	if size == 0 {
		return ErrInclusionListNoTransactions
	}
	if size > params.BeaconConfig().MaxTransactionsBytesPerInclusionList {
		return errors.Wrapf(ErrInclusionListTooLarge, "%d bytes", size)
	}
	if slices.ContainsFunc(txs, func(tx []byte) bool { return len(tx) == 0 }) {
		return ErrInclusionListEmptyTransaction
	}

	// The dependent block must be known. A forkchoice node always has a computed state.
	dependentRoot := bytesutil.ToBytes32(il.DependentRoot)
	dependentBlockSlot, err := s.RecentBlockSlot(dependentRoot)
	if err != nil {
		return errors.Wrapf(ErrInclusionListUnknownDependentBlock, "%#x", dependentRoot)
	}

	// The dependent block's slot must not be after the shuffling dependent slot.
	dependentSlot, err := slots.ShufflingDependentSlot(epoch)
	if err != nil {
		return errors.Wrap(err, "shuffling dependent slot")
	}
	if dependentBlockSlot > dependentSlot {
		return errors.Wrapf(ErrInclusionListDependentBlockTooLate, "block slot %d, dependent slot %d", dependentBlockSlot, dependentSlot)
	}

	// The dependent block must be a possible dependent block for the committee lookahead.
	st, err := s.LookaheadState(ctx, dependentRoot, slots.ShufflingLookaheadEpoch(epoch))
	switch {
	case errors.Is(err, ErrInvalidDependentRoot):
		return errors.Wrapf(ErrInclusionListInvalidDependentRoot, "%#x", dependentRoot)
	case errors.Is(err, ErrDependentStateUnavailable):
		return errors.Wrap(ErrInclusionListDependentStateUnavailable, err.Error())
	case err != nil:
		return err
	}

	// Verify the validator is in the inclusion list committee.
	committee, err := helpers.InclusionListCommittee(ctx, st, il.Slot)
	if err != nil {
		return errors.Wrap(err, "inclusion list committee")
	}
	if !slices.Contains(committee, il.ValidatorIndex) {
		return errors.Wrapf(ErrInclusionListNotInCommittee, "validator %d, slot %d", il.ValidatorIndex, il.Slot)
	}

	// Verify the signature.
	if err := helpers.VerifyInclusionListSignature(ctx, st, signed); err != nil {
		return errors.Wrap(ErrInclusionListInvalidSignature, err.Error())
	}

	// The inclusion list is timely if it arrives before the deadline of its slot. A list for the
	// next slot arrives before that slot starts, so it is timely.
	timely := il.Slot > currentSlot
	if il.Slot == currentSlot {
		intoSlot, err := slots.SinceSlotStart(currentSlot, s.genesisTime, now)
		if err != nil {
			return errors.Wrap(err, "time into slot")
		}
		cfg := params.BeaconConfig()
		timely = intoSlot < cfg.SlotComponentDuration(cfg.InclusionListDueBPS)
	}

	s.cfg.InclusionListStore.Process(signed, timely)
	return nil
}

// inclusionListTransactions returns the timely inclusion list transactions for the payload of the block at blockRoot and slot.
func (s *Service) inclusionListTransactions(blockRoot [32]byte, slot primitives.Slot) [][]byte {
	if slot == 0 {
		return [][]byte{}
	}
	ilSlot := slot - 1
	dependentRoot, err := s.DependentRootForEpoch(blockRoot, slots.ShufflingLookaheadEpoch(slots.ToEpoch(ilSlot)))
	if err != nil {
		log.WithError(err).WithField("blockRoot", fmt.Sprintf("%#x", blockRoot)).Debug("Could not get the inclusion list dependent root")
		return [][]byte{}
	}
	txs := s.cfg.InclusionListStore.Transactions(ilSlot, dependentRoot, true)
	if txs == nil {
		return [][]byte{}
	}
	return txs
}

// proposalInclusionListTransactions returns the inclusion list transactions for the payload attributes of a proposal at slot.
func (s *Service) proposalInclusionListTransactions(ctx context.Context, st state.ReadOnlyBeaconState, slot primitives.Slot) [][]byte {
	ils, err := helpers.NewProposalInclusionLists(ctx, s.cfg.BeaconDB, s.cfg.InclusionListStore, st, slot)
	if err != nil {
		log.WithError(err).WithField("slot", slot).Error("Could not get the inclusion list dependent root for payload attributes")
		return [][]byte{}
	}
	return ils.Transactions()
}

// recordForkchoiceInclusionListSatisfaction records the flag of a VALID forkchoice update for the optimistic Heze payload with blockHash, found from headRoot or its ancestors.
func (s *Service) recordForkchoiceInclusionListSatisfaction(headRoot, blockHash [32]byte, satisfied bool) {
	fc := s.cfg.ForkChoiceStore
	fc.Lock()
	defer fc.Unlock()
	root := headRoot

	// The head hash belongs to the head or to the ancestor whose payload the head chain builds on.
	// The walk is bounded to one epoch of ancestors so that it does not hold the write lock for long; more than one epoch of withheld payloads leaves the flag unrecorded.
	for range params.BeaconConfig().SlotsPerEpoch {
		slot, err := fc.Slot(root)
		if err != nil || slots.ToEpoch(slot) < params.BeaconConfig().HezeForkEpoch {
			return
		}
		if h, err := fc.BlockHash(root); err == nil && h == blockHash && fc.HasFullNode(root) {
			// Only a NOT_VALIDATED payload takes the flag. A VALID payload keeps the flag from its own newPayload call.
			if optimistic, err := fc.IsOptimistic(root); err != nil || !optimistic {
				return
			}
			if err := fc.SetPayloadInclusionListSatisfied(root, satisfied); err != nil {
				log.WithError(err).Error("Could not record inclusion list satisfaction")
				return
			}
			log.WithFields(logrus.Fields{
				"blockRoot": fmt.Sprintf("%#x", bytesutil.Trunc(root[:])),
				"satisfied": satisfied,
			}).Debug("Recorded inclusion list satisfaction from forkchoice update")
			return
		}
		parent, err := fc.ParentRoot(root)
		if err != nil || parent == [32]byte{} {
			return
		}
		root = parent
	}
}
