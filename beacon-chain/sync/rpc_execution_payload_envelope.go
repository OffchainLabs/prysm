package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/encoder"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v6/network/forks"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/pkg/errors"
)

func (s *Service) executionPayloadByRootRPCHandler(ctx context.Context, msg interface{}, stream libp2pcore.Stream) error {
	SetRPCStreamDeadlines(stream)

	r, ok := msg.(*types.BeaconBlockByRootsReq)
	if !ok {
		return errors.New("message is not type BlobSidecarsByRootReq")
	}
	blockRoots := *r
	if err := s.rateLimiter.validateRequest(stream, uint64(len(blockRoots))); err != nil {
		return err
	}
	if len(blockRoots) == 0 {
		s.rateLimiter.add(stream, 1)
		s.writeErrorResponseToStream(responseCodeInvalidRequest, "no block roots provided in request", stream)
		return errors.New("no block roots provided")
	}
	for _, root := range blockRoots {
		hash, err := s.cfg.chain.HashForBlockRoot(ctx, root)
		if err != nil {
			continue
		}
		blindPayload, err := s.cfg.beaconDB.SignedBlindPayloadEnvelope(ctx, hash)
		if err != nil {
			continue
		}
		if blindPayload == nil {
			continue
		}
		SetStreamWriteDeadline(stream, defaultWriteDuration)

		constructedPayload, err := s.cfg.executionReconstructor.ReconstructPayloadEnvelope(ctx, blindPayload)
		if err != nil {
			log.WithError(err).WithField("root", root).Error("Failed to reconstruct payload envelope")
			continue
		}

		if chunkErr := writePayloadChunk(stream, s.cfg.chain, s.cfg.p2p.Encoding(), constructedPayload); chunkErr != nil {
			log.WithError(chunkErr).Debug("Could not send a chunked response")
			s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
			return chunkErr
		}
	}
	closeStream(stream, log)
	return nil
}

func writePayloadChunk(stream libp2pcore.Stream, tor blockchain.TemporalOracle, encoding encoder.NetworkEncoding, payload *enginev1.SignedExecutionPayloadEnvelope) error {
	if _, err := stream.Write([]byte{responseCodeSuccess}); err != nil {
		return err
	}
	valRoot := tor.GenesisValidatorsRoot()
	ctxBytes, err := forks.ForkDigestFromEpoch(slots.ToEpoch(payload.Message.Slot), valRoot[:])
	if err != nil {
		return err
	}

	if err := writeContextToStream(ctxBytes[:], stream); err != nil {
		return err
	}
	_, err = encoding.EncodeWithMaxLength(stream, payload)
	return err
}
