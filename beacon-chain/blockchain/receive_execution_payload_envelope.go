package blockchain

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/epbs"
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
func (s *Service) ReceiveExecutionPayloadEnvelope(ctx context.Context, envelope interfaces.ROExecutionPayloadEnvelope, _ das.AvailabilityStore) error {
	receivedTime := time.Now()
	root, err := envelope.BeaconBlockRoot()
	if err != nil {
		return errors.Wrap(err, "could not get beacon block root")
	}
	s.payloadBeingSynced.set(root)
	defer s.payloadBeingSynced.unset(root)

	preState, err := s.getPayloadEnvelopePrestate(ctx, envelope)
	if err != nil {
		return errors.Wrap(err, "could not get prestate")
	}

	eg, _ := errgroup.WithContext(ctx)
	var postState state.BeaconState
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
	_ = isValidPayload
	_ = postState
	daStartTime := time.Now()
	// TODO: Add DA check
	daWaitedTime := time.Since(daStartTime)
	dataAvailWaitedTime.Observe(float64(daWaitedTime.Milliseconds()))
	// TODO: Add Head update, cache handling, postProcessing
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

	var lastValidHash []byte
	var versionedHashes []common.Hash
	versionedHashes, err = envelope.VersionedHashes()
	if err != nil {
		return false, errors.Wrap(err, "could not get versioned hashes to feed the engine")
	}
	root, err := envelope.BeaconBlockRoot()
	if err != nil {
		return false, errors.Wrap(err, "could not get beacon block root")
	}
	parentRoot, err := s.ParentRoot(root)
	if err != nil {
		return false, errors.Wrap(err, "could not get parent block root")
	}
	pr := common.Hash(parentRoot)
	lastValidHash, err = s.cfg.ExecutionEngineCaller.NewPayload(ctx, payload, versionedHashes, &pr)
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
	blockRoot, rootErr := e.BeaconBlockRoot()
	if rootErr != nil {
		return false, errors.Wrap(rootErr, "could not get beacon block root")
	}
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
	root, err := e.BeaconBlockRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not get beacon block root")
	}
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
