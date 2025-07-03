package sync

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/math"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	libp2pcore "github.com/libp2p/go-libp2p/core"
)

// lightClientBootstrapRPCHandler handles the /eth2/beacon_chain/req/light_client_bootstrap/1/ RPC request.
func (s *Service) lightClientBootstrapRPCHandler(ctx context.Context, msg interface{}, stream libp2pcore.Stream) error {
	ctx, span := trace.StartSpan(ctx, "sync.lightClientBootstrapRPCHandler")
	defer span.End()
	ctx, cancel := context.WithTimeout(ctx, ttfbTimeout)
	defer cancel()

	logger := log.WithField("handler", p2p.LightClientBootstrapName[1:])

	SetRPCStreamDeadlines(stream)
	if err := s.rateLimiter.validateRequest(stream, 1); err != nil {
		logger.WithError(err).Error("s.rateLimiter.validateRequest")
		return err
	}
	s.rateLimiter.add(stream, 1)

	rawMsg, ok := msg.(*[fieldparams.RootLength]byte)
	if !ok {
		logger.Error("Message is not *types.LightClientBootstrapReq")
		return fmt.Errorf("message is not type %T", &[fieldparams.RootLength]byte{})
	}
	blkRoot := *rawMsg

	bootstrap, err := s.lcStore.LightClientBootstrap(ctx, blkRoot)
	if err != nil {
		s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
		tracing.AnnotateError(span, err)
		logger.WithError(err).Error("s.cfg.beaconDB.LightClientBootstrap")
		return err
	}
	if bootstrap == nil {
		s.writeErrorResponseToStream(responseCodeResourceUnavailable, types.ErrResourceUnavailable.Error(), stream)
		tracing.AnnotateError(span, err)
		logger.WithError(err).Error(fmt.Sprintf("nil bootstrap for root %#x", blkRoot))
		return err
	}

	SetStreamWriteDeadline(stream, defaultWriteDuration)
	if err = WriteLightClientBootstrapChunk(stream, s.cfg.clock, s.cfg.p2p.Encoding(), bootstrap); err != nil {
		s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
		tracing.AnnotateError(span, err)
		logger.WithError(err).Error("WriteLightClientBootstrapChunk")
		return err
	}

	logger.Info("lightClientBootstrapRPCHandler completed")

	closeStream(stream, logger)
	return nil
}

// lightClientUpdatesByRangeRPCHandler handles the /eth2/beacon_chain/req/light_client_updates_by_range/1/ RPC request.
func (s *Service) lightClientUpdatesByRangeRPCHandler(ctx context.Context, msg interface{}, stream libp2pcore.Stream) error {
	ctx, span := trace.StartSpan(ctx, "sync.lightClientUpdatesByRangeRPCHandler")
	defer span.End()
	ctx, cancel := context.WithTimeout(ctx, ttfbTimeout)
	defer cancel()

	logger := log.WithField("handler", p2p.LightClientUpdatesByRangeName[1:])

	SetRPCStreamDeadlines(stream)
	if err := s.rateLimiter.validateRequest(stream, 1); err != nil {
		logger.WithError(err).Error("s.rateLimiter.validateRequest")
		return err
	}
	s.rateLimiter.add(stream, 1)

	r, ok := msg.(*eth.LightClientUpdatesByRangeRequest)
	if !ok {
		logger.Error("Message is not *eth.LightClientUpdatesByRangeReq")
		return fmt.Errorf("message is not type %T", &eth.LightClientUpdatesByRangeRequest{})
	}

	if r.Count == 0 {
		s.writeErrorResponseToStream(responseCodeInvalidRequest, "count is 0", stream)
		s.cfg.p2p.Peers().Scorers().BadResponsesScorer().Increment(stream.Conn().RemotePeer())
		logger.Error("Count is 0")
		return nil
	}

	if r.Count > params.BeaconConfig().MaxRequestLightClientUpdates {
		r.Count = params.BeaconConfig().MaxRequestLightClientUpdates
	}

	endPeriod, err := math.Add64(r.StartPeriod, r.Count-1)
	if err != nil {
		s.writeErrorResponseToStream(responseCodeInvalidRequest, err.Error(), stream)
		s.cfg.p2p.Peers().Scorers().BadResponsesScorer().Increment(stream.Conn().RemotePeer())
		tracing.AnnotateError(span, err)
		logger.WithError(err).Error("End period overflows")
		return err
	}

	logger.Infof("LC: requesting updates by range (StartPeriod: %d, EndPeriod: %d)", r.StartPeriod, r.StartPeriod+r.Count-1)

	updates, err := s.cfg.beaconDB.LightClientUpdates(ctx, r.StartPeriod, endPeriod)
	if err != nil {
		s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
		tracing.AnnotateError(span, err)
		logger.WithError(err).Error("s.cfg.beaconDB.LightClientUpdates")
		return err
	}

	if len(updates) == 0 {
		s.writeErrorResponseToStream(responseCodeResourceUnavailable, types.ErrResourceUnavailable.Error(), stream)
		tracing.AnnotateError(span, err)
		logger.Debugf("No update available for start period %d", r.StartPeriod)
		return nil
	}

	for i := r.StartPeriod; i <= endPeriod; i++ {
		u, ok := updates[i]
		if !ok {
			break
		}

		SetStreamWriteDeadline(stream, defaultWriteDuration)
		if err = WriteLightClientUpdateChunk(stream, s.cfg.clock, s.cfg.p2p.Encoding(), u); err != nil {
			s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
			tracing.AnnotateError(span, err)
			logger.WithError(err).Error("WriteLightClientUpdateChunk")
			return err
		}
		s.rateLimiter.add(stream, 1)
	}

	logger.Info("lightClientUpdatesByRangeRPCHandler completed")

	closeStream(stream, logger)
	return nil
}

// lightClientFinalityUpdateRPCHandler handles the /eth2/beacon_chain/req/light_client_finality_update/1/ RPC request.
func (s *Service) lightClientFinalityUpdateRPCHandler(ctx context.Context, _ interface{}, stream libp2pcore.Stream) error {
	ctx, span := trace.StartSpan(ctx, "sync.lightClientFinalityUpdateRPCHandler")
	defer span.End()
	_, cancel := context.WithTimeout(ctx, ttfbTimeout)
	defer cancel()

	logger := log.WithField("handler", p2p.LightClientFinalityUpdateName[1:])

	SetRPCStreamDeadlines(stream)
	if err := s.rateLimiter.validateRequest(stream, 1); err != nil {
		logger.WithError(err).Error("s.rateLimiter.validateRequest")
		return err
	}
	s.rateLimiter.add(stream, 1)

	if s.lcStore.LastFinalityUpdate() == nil {
		s.writeErrorResponseToStream(responseCodeResourceUnavailable, types.ErrResourceUnavailable.Error(), stream)
		logger.Error("No finality update available")
		return nil
	}

	SetStreamWriteDeadline(stream, defaultWriteDuration)
	if err := WriteLightClientFinalityUpdateChunk(stream, s.cfg.clock, s.cfg.p2p.Encoding(), s.lcStore.LastFinalityUpdate()); err != nil {
		s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
		tracing.AnnotateError(span, err)
		logger.WithError(err).Error("WriteLightClientFinalityUpdateChunk")
		return err
	}

	logger.Info("lightClientFinalityUpdateRPCHandler completed")

	closeStream(stream, logger)
	return nil
}

// lightClientOptimisticUpdateRPCHandler handles the /eth2/beacon_chain/req/light_client_optimistic_update/1/ RPC request.
func (s *Service) lightClientOptimisticUpdateRPCHandler(ctx context.Context, _ interface{}, stream libp2pcore.Stream) error {
	ctx, span := trace.StartSpan(ctx, "sync.lightClientOptimisticUpdateRPCHandler")
	defer span.End()
	_, cancel := context.WithTimeout(ctx, ttfbTimeout)
	defer cancel()

	logger := log.WithField("handler", p2p.LightClientOptimisticUpdateName[1:])

	logger.Info("lightClientOptimisticUpdateRPCHandler invoked")

	SetRPCStreamDeadlines(stream)
	if err := s.rateLimiter.validateRequest(stream, 1); err != nil {
		logger.WithError(err).Error("s.rateLimiter.validateRequest")
		return err
	}
	s.rateLimiter.add(stream, 1)

	if s.lcStore.LastOptimisticUpdate() == nil {
		s.writeErrorResponseToStream(responseCodeResourceUnavailable, types.ErrResourceUnavailable.Error(), stream)
		logger.Error("No optimistic update available")
		return nil
	}

	SetStreamWriteDeadline(stream, defaultWriteDuration)
	if err := WriteLightClientOptimisticUpdateChunk(stream, s.cfg.clock, s.cfg.p2p.Encoding(), s.lcStore.LastOptimisticUpdate()); err != nil {
		s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
		tracing.AnnotateError(span, err)
		logger.WithError(err).Error("WriteLightClientOptimisticUpdateChunk")
		return err
	}

	logger.Info("lightClientOptimisticUpdateRPCHandler completed")

	closeStream(stream, logger)
	return nil
}
