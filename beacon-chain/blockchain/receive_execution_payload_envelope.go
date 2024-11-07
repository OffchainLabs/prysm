package blockchain

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/epbs"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/transition"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/das"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/execution"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
	"golang.org/x/sync/errgroup"
)

// ReceiveExecutionPayloadEnvelope is a function that defines the operations (minus pubsub)
// that are performed on a received execution payload envelope. The operations consist of:
//  1. Validate the payload, apply state transition.
//  2. Apply fork choice to the processed payload
//  3. Save latest head info
func (s *Service) ReceiveExecutionPayloadEnvelope(ctx context.Context, signed interfaces.ROSignedExecutionPayloadEnvelope, _ das.AvailabilityStore) error {
	receivedTime := time.Now()
	envelope, err := signed.Envelope()
	if err != nil {
		return err
	}
	root := envelope.BeaconBlockRoot()
	s.payloadBeingSynced.set(envelope)
	defer s.payloadBeingSynced.unset(root)

	preState, err := s.getPayloadEnvelopePrestate(ctx, envelope)
	if err != nil {
		return errors.Wrap(err, "could not get prestate")
	}

	eg, _ := errgroup.WithContext(ctx)
	eg.Go(func() error {
		if err := epbs.ValidatePayloadStateTransition(ctx, preState, envelope); err != nil {
			return errors.Wrap(err, "failed to validate consensus state transition function")
		}
		return nil
	})
	var isValidPayload bool
	eg.Go(func() error {
		var err error
		isValidPayload, err = s.validateExecutionOnEnvelope(ctx, envelope)
		if err != nil {
			return errors.Wrap(err, "could not notify the engine of the new payload")
		}
		return nil
	})

	if err := eg.Wait(); err != nil {
		return err
	}
	daStartTime := time.Now()
	// TODO: Add DA check
	daWaitedTime := time.Since(daStartTime)
	dataAvailWaitedTime.Observe(float64(daWaitedTime.Milliseconds()))
	if err := s.savePostPayload(ctx, signed, preState); err != nil {
		return err
	}
	if err := s.insertPayloadEnvelope(envelope); err != nil {
		return errors.Wrap(err, "could not insert payload to forkchoice")
	}
	if isValidPayload {
		s.ForkChoicer().Lock()
		if err := s.ForkChoicer().SetOptimisticToValid(ctx, root); err != nil {
			s.ForkChoicer().Unlock()
			return errors.Wrap(err, "could not set optimistic payload to valid")
		}
		s.ForkChoicer().Unlock()
	}

	headRoot, err := s.HeadRoot(ctx)
	if err != nil {
		log.WithError(err).Error("could not get headroot to compute attributes")
		return nil
	}
	if bytes.Equal(headRoot, root[:]) {
		attr := s.getPayloadAttribute(ctx, preState, envelope.Slot()+1, headRoot)
		execution, err := envelope.Execution()
		if err != nil {
			log.WithError(err).Error("could not get execution data")
			return nil
		}
		blockHash := [32]byte(execution.BlockHash())
		payloadID, err := s.notifyForkchoiceUpdateEPBS(ctx, blockHash, attr)
		if err != nil {
			if IsInvalidBlock(err) {
				// TODO handle the lvh here
				return err
			}
			return nil
		}
		if attr != nil && !attr.IsEmpty() && payloadID != nil {
			var pid [8]byte
			copy(pid[:], payloadID[:])
			log.WithFields(logrus.Fields{
				"blockRoot": fmt.Sprintf("%#x", bytesutil.Trunc(headRoot)),
				"headSlot":  envelope.Slot(),
				"payloadID": fmt.Sprintf("%#x", bytesutil.Trunc(payloadID[:])),
			}).Info("Forkchoice updated with payload attributes for proposal")
			s.cfg.PayloadIDCache.Set(envelope.Slot()+1, root, pid)
		}
		headBlk, err := s.HeadBlock(ctx)
		if err != nil {
			log.WithError(err).Error("could not get head block")
		}
		if err := s.saveHead(ctx, root, headBlk, preState); err != nil {
			log.WithError(err).Error("could not save new head")
		}
		// update the NSC with the hash for the full block
		if err := transition.UpdateNextSlotCache(ctx, blockHash[:], preState); err != nil {
			log.WithError(err).Error("could not update next slot cache with payload")
		}

	}
	timeWithoutDaWait := time.Since(receivedTime) - daWaitedTime
	executionEngineProcessingTime.Observe(float64(timeWithoutDaWait.Milliseconds()))
	return nil
}

// notifyNewPayload signals execution engine on a new payload.
// It returns true if the EL has returned VALID for the block
func (s *Service) notifyNewEnvelope(ctx context.Context, envelope interfaces.ROExecutionPayloadEnvelope) (bool, error) {
	ctx, span := trace.StartSpan(ctx, "blockChain.notifyNewPayload")
	defer span.End()

	payload, err := envelope.Execution()
	if err != nil {
		return false, errors.Wrap(err, "could not get execution payload")
	}

	versionedHashes := envelope.VersionedHashes()
	root := envelope.BeaconBlockRoot()
	parentRoot, err := s.ParentRoot(root)
	if err != nil {
		return false, errors.Wrap(err, "could not get parent block root")
	}
	pr := common.Hash(parentRoot)
	requests := envelope.ExecutionRequests()
	lastValidHash, err := s.cfg.ExecutionEngineCaller.NewPayload(ctx, payload, versionedHashes, &pr, requests)
	switch {
	case err == nil:
		newPayloadValidNodeCount.Inc()
		return true, nil
	case errors.Is(err, execution.ErrAcceptedSyncingPayloadStatus):
		newPayloadOptimisticNodeCount.Inc()
		log.WithFields(logrus.Fields{
			"payloadBlockHash": fmt.Sprintf("%#x", bytesutil.Trunc(payload.BlockHash())),
		}).Info("Called new payload with optimistic block")
		return false, nil
	case errors.Is(err, execution.ErrInvalidPayloadStatus):
		lvh := bytesutil.ToBytes32(lastValidHash)
		return false, invalidBlock{
			error:         ErrInvalidPayload,
			lastValidHash: lvh,
		}
	default:
		return false, errors.WithMessage(ErrUndefinedExecutionEngineError, err.Error())
	}
}

// validateExecutionOnEnvelope notifies the engine of the incoming execution payload and returns true if the payload is valid
func (s *Service) validateExecutionOnEnvelope(ctx context.Context, e interfaces.ROExecutionPayloadEnvelope) (bool, error) {
	isValidPayload, err := s.notifyNewEnvelope(ctx, e)
	if err == nil {
		return isValidPayload, nil
	}
	blockRoot := e.BeaconBlockRoot()
	parentRoot, rootErr := s.ParentRoot(blockRoot)
	if rootErr != nil {
		return false, errors.Wrap(rootErr, "could not get parent block root")
	}
	s.cfg.ForkChoiceStore.Lock()
	err = s.handleInvalidExecutionError(ctx, err, blockRoot, parentRoot)
	s.cfg.ForkChoiceStore.Unlock()
	return false, err
}

func (s *Service) getPayloadEnvelopePrestate(ctx context.Context, e interfaces.ROExecutionPayloadEnvelope) (state.BeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "blockChain.getPayloadEnvelopePreState")
	defer span.End()

	// Verify incoming payload has a valid pre state.
	root := e.BeaconBlockRoot()
	// Verify the referred block is known to forkchoice
	if !s.InForkchoice(root) {
		return nil, errors.New("Cannot import execution payload envelope for unknown block")
	}
	if err := s.verifyBlkPreState(ctx, root); err != nil {
		return nil, errors.Wrap(err, "could not verify payload prestate")
	}

	preState, err := s.cfg.StateGen.StateByRoot(ctx, root)
	if err != nil {
		return nil, errors.Wrap(err, "could not get pre state")
	}
	if preState == nil || preState.IsNil() {
		return nil, errors.Wrap(err, "nil pre state")
	}
	return preState, nil
}

func (s *Service) savePostPayload(ctx context.Context, signed interfaces.ROSignedExecutionPayloadEnvelope, st state.BeaconState) error {
	if err := s.cfg.BeaconDB.SaveBlindPayloadEnvelope(ctx, signed); err != nil {
		return err
	}
	envelope, err := signed.Envelope()
	if err != nil {
		return err
	}
	execution, err := envelope.Execution()
	if err != nil {
		return err
	}
	r := envelope.BeaconBlockRoot()
	if err := s.cfg.StateGen.SaveState(ctx, [32]byte(execution.BlockHash()), st); err != nil {
		log.Warnf("Rolling back insertion of block with root %#x", r)
		if err := s.cfg.BeaconDB.DeleteBlock(ctx, r); err != nil {
			log.WithError(err).Errorf("Could not delete block with block root %#x", r)
		}
		return errors.Wrap(err, "could not save state")
	}
	return nil
}
