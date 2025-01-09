package sync

import (
	"context"
	"fmt"

	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/types"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/math"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

// lightClientBootstrapRPCHandler handles the /eth2/beacon_chain/req/light_client_bootstrap/1/ RPC request.
func (s *Service) lightClientBootstrapRPCHandler(ctx context.Context, msg interface{}, stream libp2pcore.Stream) error {
	ctx, span := trace.StartSpan(ctx, "sync.lightClientBootstrapRPCHandler")
	defer span.End()
	ctx, cancel := context.WithTimeout(ctx, ttfbTimeout)
	defer cancel()

	// TODO: What should we log?
	log := log.WithField("handler", p2p.LightClientBootstrapName[1:]) // slice the leading slash off the name var

	log.Info("LC: lightClientBootstrapRPCHandler invoked")

	SetRPCStreamDeadlines(stream)
	if err := s.rateLimiter.validateRequest(stream, 1); err != nil {
		log.WithError(err).Error("LC: s.rateLimiter.validateRequest")
		return err
	}
	s.rateLimiter.add(stream, 1)

	rawMsg, ok := msg.(*types.LightClientBootstrapReq)
	if !ok {
		log.Error("Message is not *types.LightClientBootstrapReq")
		return fmt.Errorf("message is not type %T", &types.LightClientBootstrapReq{})
	}
	blkRoot := *rawMsg

	bootstrap, err := s.cfg.beaconDB.LightClientBootstrap(ctx, blkRoot[:])
	if err != nil {
		s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
		tracing.AnnotateError(span, err)
		log.WithError(err).Error("LC: s.cfg.beaconDB.LightClientBootstrap")
		return err
	}
	if bootstrap == nil {
		s.writeErrorResponseToStream(responseCodeResourceUnavailable, types.ErrResourceUnavailable.Error(), stream)
		tracing.AnnotateError(span, err)
		log.WithError(err).Error(fmt.Sprintf("LC: nil bootstrap for root %#x", blkRoot))
		return err
	}

	SetStreamWriteDeadline(stream, defaultWriteDuration)
	if err = WriteLightClientBootstrapChunk(stream, s.cfg.clock, s.cfg.p2p.Encoding(), bootstrap); err != nil {
		s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
		tracing.AnnotateError(span, err)
		log.WithError(err).Error("LC: WriteLightClientBootstrapChunk")
		return err
	}

	log.Info("LC: lightClientBootstrapRPCHandler completed")

	closeStream(stream, log)
	return nil
}

// lightClientUpdatesByRangeRPCHandler handles the /eth2/beacon_chain/req/light_client_updates_by_range/1/ RPC request.
func (s *Service) lightClientUpdatesByRangeRPCHandler(ctx context.Context, msg interface{}, stream libp2pcore.Stream) error {
	ctx, span := trace.StartSpan(ctx, "sync.lightClientUpdatesByRangeRPCHandler")
	defer span.End()
	ctx, cancel := context.WithTimeout(ctx, ttfbTimeout)
	defer cancel()

	// TODO: What should we log?
	log := log.WithField("handler", p2p.LightClientUpdatesByRangeName[1:]) // slice the leading slash off the name var

	log.Info("LC: lightClientUpdatesByRangeRPCHandler invoked")

	SetRPCStreamDeadlines(stream)
	if err := s.rateLimiter.validateRequest(stream, 1); err != nil {
		log.WithError(err).Error("LC: s.rateLimiter.validateRequest")
		return err
	}
	s.rateLimiter.add(stream, 1)

	r, ok := msg.(*eth.LightClientUpdatesByRangeReq)
	if !ok {
		log.Error("Message is not *eth.LightClientUpdatesByRangeReq")
		return fmt.Errorf("message is not type %T", &eth.LightClientUpdatesByRangeReq{})
	}

	if r.Count == 0 {
		s.writeErrorResponseToStream(responseCodeInvalidRequest, "count is 0", stream)
		s.cfg.p2p.Peers().Scorers().BadResponsesScorer().Increment(stream.Conn().RemotePeer())
		log.Error("LC: Count is 0")
		// TODO: Is returning nil OK?
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
		log.WithError(err).Error("LC: End period overflows")
		return err
	}

	log.Infof("LC: requesting updates by range (StartPeriod: %d, EndPeriod: %d)", r.StartPeriod, r.StartPeriod+r.Count-1)

	updates, err := s.cfg.beaconDB.LightClientUpdates(ctx, r.StartPeriod, endPeriod)
	if err != nil {
		s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
		tracing.AnnotateError(span, err)
		log.WithError(err).Error("LC: s.cfg.beaconDB.LightClientUpdates")
		return err
	}
	for _, u := range updates {
		SetStreamWriteDeadline(stream, defaultWriteDuration)
		if err = WriteLightClientUpdateChunk(stream, s.cfg.clock, s.cfg.p2p.Encoding(), u); err != nil {
			s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
			tracing.AnnotateError(span, err)
			log.WithError(err).Error("LC: WriteLightClientUpdateChunk")
			return err
		}
		s.rateLimiter.add(stream, 1)
	}

	log.Info("LC: lightClientUpdatesByRangeRPCHandler completed")

	closeStream(stream, log)
	return nil
}

// lightClientFinalityUpdateRPCHandler handles the /eth2/beacon_chain/req/light_client_finality_update/1/ RPC request.
func (s *Service) lightClientFinalityUpdateRPCHandler(ctx context.Context, _ interface{}, stream libp2pcore.Stream) error {
	ctx, span := trace.StartSpan(ctx, "sync.lightClientFinalityUpdateRPCHandler")
	defer span.End()
	_, cancel := context.WithTimeout(ctx, ttfbTimeout)
	defer cancel()

	// TODO: What should we log?
	log := log.WithField("handler", p2p.LightClientFinalityUpdateName[1:]) // slice the leading slash off the name var

	log.Info("LC: lightClientFinalityUpdateRPCHandler invoked")

	SetRPCStreamDeadlines(stream)
	if err := s.rateLimiter.validateRequest(stream, 1); err != nil {
		log.WithError(err).Error("s.rateLimiter.validateRequest")
		return err
	}
	s.rateLimiter.add(stream, 1)

	if s.lcStore.LastLCFinalityUpdate == nil {
		s.writeErrorResponseToStream(responseCodeResourceUnavailable, types.ErrResourceUnavailable.Error(), stream)
		log.Error("LC: No finality update available")
		return nil
	}

	SetStreamWriteDeadline(stream, defaultWriteDuration)
	if err := WriteLightClientFinalityUpdateChunk(stream, s.cfg.clock, s.cfg.p2p.Encoding(), s.lcStore.LastLCFinalityUpdate); err != nil {
		s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
		tracing.AnnotateError(span, err)
		log.WithError(err).Error("LC: WriteLightClientFinalityUpdateChunk")
		return err
	}

	log.Info("LC: lightClientFinalityUpdateRPCHandler completed")

	closeStream(stream, log)
	return nil
}

// lightClientOptimisticUpdateRPCHandler handles the /eth2/beacon_chain/req/light_client_optimistic_update/1/ RPC request.
func (s *Service) lightClientOptimisticUpdateRPCHandler(ctx context.Context, _ interface{}, stream libp2pcore.Stream) error {
	ctx, span := trace.StartSpan(ctx, "sync.lightClientOptimisticUpdateRPCHandler")
	defer span.End()
	_, cancel := context.WithTimeout(ctx, ttfbTimeout)
	defer cancel()

	// TODO: What should we log?
	log := log.WithField("handler", p2p.LightClientOptimisticUpdateName[1:]) // slice the leading slash off the name var

	log.Info("LC: lightClientOptimisticUpdateRPCHandler invoked")

	SetRPCStreamDeadlines(stream)
	if err := s.rateLimiter.validateRequest(stream, 1); err != nil {
		log.WithError(err).Error("s.rateLimiter.validateRequest")
		return err
	}
	s.rateLimiter.add(stream, 1)

	if s.lcStore.LastLCOptimisticUpdate == nil {
		s.writeErrorResponseToStream(responseCodeResourceUnavailable, types.ErrResourceUnavailable.Error(), stream)
		log.Error("LC: No optimistic update available")
		return nil
	}

	SetStreamWriteDeadline(stream, defaultWriteDuration)
	if err := WriteLightClientOptimisticUpdateChunk(stream, s.cfg.clock, s.cfg.p2p.Encoding(), s.lcStore.LastLCOptimisticUpdate); err != nil {
		s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
		tracing.AnnotateError(span, err)
		log.WithError(err).Error("LC: WriteLightClientOptimisticUpdateChunk")
		return err
	}

	log.Info("LC: lightClientOptimisticUpdateRPCHandler completed")

	closeStream(stream, log)
	return nil
}
