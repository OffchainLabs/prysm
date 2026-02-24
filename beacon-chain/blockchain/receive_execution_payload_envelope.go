package blockchain

import (
	"bytes"
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/execution"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	payloadattribute "github.com/OffchainLabs/prysm/v7/consensus-types/payload-attribute"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// ExecutionPayloadEnvelopeReceiver defines the methods for receiving execution payload envelopes.
type ExecutionPayloadEnvelopeReceiver interface {
	ReceiveExecutionPayloadEnvelope(context.Context, interfaces.ROSignedExecutionPayloadEnvelope) error
}

// ReceiveExecutionPayloadEnvelope processes a signed execution payload envelope for the Gloas fork.
func (s *Service) ReceiveExecutionPayloadEnvelope(ctx context.Context, signed interfaces.ROSignedExecutionPayloadEnvelope) error {
	ctx, span := trace.StartSpan(ctx, "blockChain.ReceiveExecutionPayloadEnvelope")
	defer span.End()

	envelope, err := signed.Envelope()
	if err != nil {
		return errors.Wrap(err, "could not get envelope")
	}
	root := envelope.BeaconBlockRoot()

	err = s.payloadBeingSynced.set(root)
	if errors.Is(err, errBlockBeingSynced) {
		log.WithField("blockRoot", fmt.Sprintf("%#x", root)).Debug("Ignoring payload envelope currently being synced")
		return nil
	}
	defer s.payloadBeingSynced.unset(root)

	preState, err := s.getPayloadEnvelopePrestate(ctx, envelope)
	if err != nil {
		return err
	}

	var isValidPayload bool
	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return gloas.ProcessExecutionPayload(gCtx, preState, signed)
	})

	g.Go(func() error {
		var elErr error
		isValidPayload, elErr = s.validateExecutionOnEnvelope(gCtx, preState, envelope)
		return elErr
	})

	if err := g.Wait(); err != nil {
		return err
	}

	if err := s.savePostPayload(ctx, signed, preState); err != nil {
		return err
	}
	if err := s.InsertPayload(envelope); err != nil {
		return errors.Wrap(err, "could not insert payload into forkchoice")
	}

	if isValidPayload {
		s.cfg.ForkChoiceStore.Lock()
		if err := s.cfg.ForkChoiceStore.SetOptimisticToValid(ctx, root); err != nil {
			log.WithError(err).Error("Could not set optimistic to valid")
		}
		s.cfg.ForkChoiceStore.Unlock()
	}

	headRoot, err := s.HeadRoot(ctx)
	if err != nil {
		log.WithError(err).Error("Could not get head root")
		return nil
	}
	if err := s.postPayloadHeadUpdate(ctx, envelope, preState, root, headRoot); err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		"slot":      envelope.Slot(),
		"blockRoot": fmt.Sprintf("%#x", root),
	}).Info("Processed execution payload envelope")
	return nil
}

func (s *Service) postPayloadHeadUpdate(ctx context.Context, envelope interfaces.ROExecutionPayloadEnvelope, st state.BeaconState, root [32]byte, headRoot []byte) error {
	if !bytes.Equal(headRoot, root[:]) {
		return nil
	}
	payload, err := envelope.Execution()
	if err != nil {
		return errors.Wrap(err, "could not get execution payload from envelope")
	}
	blockHash := bytesutil.ToBytes32(payload.BlockHash())

	s.headLock.Lock()
	s.head.state = st
	s.headLock.Unlock()

	if err := transition.UpdateNextSlotCache(ctx, blockHash[:], st); err != nil {
		log.WithError(err).Error("Could not update next slot cache")
	}

	attr := s.getPayloadAttribute(ctx, st, envelope.Slot()+1, headRoot)
	if s.inRegularSync() {
		go func() {
			pid, err := s.notifyForkchoiceUpdateGloas(s.ctx, blockHash, attr)
			if err != nil {
				log.WithError(err).Error("Could not notify forkchoice update")
				return
			}
			if attr != nil && !attr.IsEmpty() && pid != nil {
				var pId [8]byte
				copy(pId[:], pid[:])
				s.cfg.PayloadIDCache.Set(envelope.Slot()+1, root, pId)
			}
		}()
	}
	return nil
}

func (s *Service) getPayloadEnvelopePrestate(ctx context.Context, envelope interfaces.ROExecutionPayloadEnvelope) (state.BeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "blockChain.getPayloadEnvelopePrestate")
	defer span.End()

	root := envelope.BeaconBlockRoot()
	if !s.InForkchoice(root) {
		return nil, fmt.Errorf("beacon block root %#x not found in forkchoice", root)
	}
	if err := s.verifyBlkPreState(ctx, root); err != nil {
		return nil, errors.Wrap(err, "could not verify pre-state")
	}
	preState, err := s.cfg.StateGen.StateByRoot(ctx, root)
	if err != nil {
		return nil, errors.Wrap(err, "could not get pre-state by root")
	}
	if preState == nil || preState.IsNil() {
		return nil, fmt.Errorf("nil pre-state for beacon block root %#x", root)
	}
	return preState, nil
}

// The returned boolean indicates whether the payload was valid or if it was accepted as syncing (optimistic).
func (s *Service) notifyNewEnvelope(ctx context.Context, st state.BeaconState, envelope interfaces.ROExecutionPayloadEnvelope) (bool, error) {
	ctx, span := trace.StartSpan(ctx, "blockChain.notifyNewEnvelope")
	defer span.End()

	payload, err := envelope.Execution()
	if err != nil {
		return false, errors.Wrap(err, "could not get execution payload from envelope")
	}

	latestBid, err := st.LatestExecutionPayloadBid()
	if err != nil {
		return false, errors.Wrap(err, "could not get latest execution payload bid")
	}
	commitments := latestBid.BlobKzgCommitments()
	versionedHashes := make([]common.Hash, len(commitments))
	for i, c := range commitments {
		versionedHashes[i] = primitives.ConvertKzgCommitmentToVersionedHash(c)
	}

	parentRoot := common.Hash(bytesutil.ToBytes32(st.LatestBlockHeader().ParentRoot))
	requests := envelope.ExecutionRequests()

	_, err = s.cfg.ExecutionEngineCaller.NewPayload(ctx, payload, versionedHashes, &parentRoot, requests)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, execution.ErrAcceptedSyncingPayloadStatus) {
		log.WithFields(logrus.Fields{
			"slot":             envelope.Slot(),
			"payloadBlockHash": fmt.Sprintf("%#x", bytesutil.Trunc(payload.BlockHash())),
		}).Info("Called new payload with optimistic envelope")
		return false, nil
	}
	if errors.Is(err, execution.ErrInvalidPayloadStatus) {
		return false, invalidBlock{error: ErrInvalidPayload}
	}
	return false, errors.WithMessage(ErrUndefinedExecutionEngineError, err.Error())
}

func (s *Service) validateExecutionOnEnvelope(ctx context.Context, st state.BeaconState, envelope interfaces.ROExecutionPayloadEnvelope) (bool, error) {
	isValid, err := s.notifyNewEnvelope(ctx, st, envelope)
	if err == nil {
		return isValid, nil
	}

	blockRoot := envelope.BeaconBlockRoot()
	parentRoot := bytesutil.ToBytes32(st.LatestBlockHeader().ParentRoot)
	payload, payloadErr := envelope.Execution()
	if payloadErr != nil {
		return false, errors.Wrap(payloadErr, "could not get execution payload from envelope")
	}
	parentHash := bytesutil.ToBytes32(payload.ParentHash())

	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	return false, s.handleInvalidExecutionError(ctx, err, blockRoot, parentRoot, parentHash)
}

func (s *Service) savePostPayload(ctx context.Context, signed interfaces.ROSignedExecutionPayloadEnvelope, st state.BeaconState) error {
	ctx, span := trace.StartSpan(ctx, "blockChain.savePostPayload")
	defer span.End()

	protoEnv, ok := signed.Proto().(*ethpb.SignedExecutionPayloadEnvelope)
	if !ok {
		return errors.New("could not type assert signed envelope to proto")
	}
	if err := s.cfg.BeaconDB.SaveExecutionPayloadEnvelope(ctx, protoEnv); err != nil {
		return errors.Wrap(err, "could not save execution payload envelope")
	}

	envelope, err := signed.Envelope()
	if err != nil {
		return errors.Wrap(err, "could not get envelope")
	}
	payload, err := envelope.Execution()
	if err != nil {
		return errors.Wrap(err, "could not get execution payload from envelope")
	}
	return s.cfg.StateGen.SaveState(ctx, bytesutil.ToBytes32(payload.BlockHash()), st)
}

// notifyForkchoiceUpdateGloas takes the block hash directly because Gloas
// blocks don't carry an execution payload in the body.
func (s *Service) notifyForkchoiceUpdateGloas(ctx context.Context, blockHash [32]byte, attributes payloadattribute.Attributer) (*enginev1.PayloadIDBytes, error) {
	ctx, span := trace.StartSpan(ctx, "blockChain.notifyForkchoiceUpdateGloas")
	defer span.End()

	s.cfg.ForkChoiceStore.RLock()
	finalizedHash := s.cfg.ForkChoiceStore.FinalizedPayloadBlockHash()
	justifiedHash := s.cfg.ForkChoiceStore.UnrealizedJustifiedPayloadBlockHash()
	s.cfg.ForkChoiceStore.RUnlock()
	fcs := &enginev1.ForkchoiceState{
		HeadBlockHash:      blockHash[:],
		SafeBlockHash:      justifiedHash[:],
		FinalizedBlockHash: finalizedHash[:],
	}
	if attributes == nil {
		attributes = payloadattribute.EmptyWithVersion(version.Gloas)
	}

	payloadID, lastValidHash, err := s.cfg.ExecutionEngineCaller.ForkchoiceUpdated(ctx, fcs, attributes)
	if err == nil {
		return payloadID, nil
	}

	switch {
	case errors.Is(err, execution.ErrAcceptedSyncingPayloadStatus):
		log.WithFields(logrus.Fields{
			"headBlockHash":             fmt.Sprintf("%#x", bytesutil.Trunc(blockHash[:])),
			"finalizedPayloadBlockHash": fmt.Sprintf("%#x", bytesutil.Trunc(finalizedHash[:])),
		}).Info("Called forkchoice updated with optimistic block (Gloas)")
		return payloadID, nil
	case errors.Is(err, execution.ErrInvalidPayloadStatus):
		if len(lastValidHash) == 0 {
			lastValidHash = defaultLatestValidHash
		}
		return nil, invalidBlock{
			error:         ErrInvalidPayload,
			lastValidHash: bytesutil.ToBytes32(lastValidHash),
		}
	default:
		log.WithError(err).Error(ErrUndefinedExecutionEngineError)
		return nil, nil
	}
}
