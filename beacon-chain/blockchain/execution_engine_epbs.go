package blockchain

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/execution"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	payloadattribute "github.com/prysmaticlabs/prysm/v5/consensus-types/payload-attribute"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/sirupsen/logrus"
)

// notifyForkchoiceUpdate signals execution engine the fork choice updates. Execution engine should:
// 1. Re-organizes the execution payload chain and corresponding state to make head_block_hash the head.
// 2. Applies finality to the execution state: it irreversibly persists the chain of all execution payloads and corresponding state, up to and including finalized_block_hash.
func (s *Service) notifyForkchoiceUpdateEPBS(ctx context.Context, blockhash [32]byte, attributes payloadattribute.Attributer) (*enginev1.PayloadIDBytes, error) {
	ctx, span := trace.StartSpan(ctx, "blockChain.notifyForkchoiceUpdateEPBS")
	defer span.End()

	finalizedHash := s.cfg.ForkChoiceStore.FinalizedPayloadBlockHash()
	justifiedHash := s.cfg.ForkChoiceStore.UnrealizedJustifiedPayloadBlockHash()
	fcs := &enginev1.ForkchoiceState{
		HeadBlockHash:      blockhash[:],
		SafeBlockHash:      justifiedHash[:],
		FinalizedBlockHash: finalizedHash[:],
	}
	if attributes == nil {
		attributes = payloadattribute.EmptyWithVersion(version.Electra)
	}
	payloadID, lastValidHash, err := s.cfg.ExecutionEngineCaller.ForkchoiceUpdated(ctx, fcs, attributes)
	if err != nil {
		switch {
		case errors.Is(err, execution.ErrAcceptedSyncingPayloadStatus):
			forkchoiceUpdatedOptimisticNodeCount.Inc()
			log.WithFields(logrus.Fields{
				"headPayloadBlockHash":      fmt.Sprintf("%#x", bytesutil.Trunc(blockhash[:])),
				"finalizedPayloadBlockHash": fmt.Sprintf("%#x", bytesutil.Trunc(finalizedHash[:])),
			}).Info("Called fork choice updated with optimistic block")
			return payloadID, nil
		case errors.Is(err, execution.ErrInvalidPayloadStatus):
			log.WithError(err).Info("forkchoice updated to invalid block")
			return nil, invalidBlock{error: ErrInvalidPayload, root: [32]byte(lastValidHash)}
		default:
			log.WithError(err).Error(ErrUndefinedExecutionEngineError)
			return nil, nil
		}
	}
	forkchoiceUpdatedValidNodeCount.Inc()
	// If the forkchoice update call has an attribute, update the payload ID cache.
	hasAttr := attributes != nil && !attributes.IsEmpty()
	if hasAttr && payloadID == nil && !features.Get().PrepareAllPayloads {
		log.WithFields(logrus.Fields{
			"blockHash": fmt.Sprintf("%#x", blockhash[:]),
		}).Error("Received nil payload ID on VALID engine response")
	}
	return payloadID, nil
}
