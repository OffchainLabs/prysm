package sync

import (
	"context"
	"reflect"
	"runtime/debug"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	p2ptypes "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
	"github.com/sirupsen/logrus"
)

var (
	// Time to first byte timeout. The maximum time to wait for first byte of
	// request response (time-to-first-byte). The client is expected to give up if
	// they don't receive the first byte within 5 seconds.
	ttfbTimeout = params.BeaconConfig().TtfbTimeoutDuration()

	// respTimeout is the maximum time for complete response transfer.
	respTimeout = params.BeaconConfig().RespTimeoutDuration()
)

// rpcHandler is responsible for handling and responding to any incoming message.
// This method may return an error to internal monitoring, but the error will
// not be relayed to the peer.
type rpcHandler func(context.Context, interface{}, libp2pcore.Stream) error

// rpcHandlerByTopicFromFork returns the RPC handlers for a given fork index.
func (s *Service) rpcHandlerByTopicFromFork(forkIndex int) (map[string]rpcHandler, error) {
	// Fulu: https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/p2p-interface.md#messages
	if forkIndex >= version.Fulu {
		return map[string]rpcHandler{
			p2p.RPCStatusTopicV2:                    s.statusRPCHandler, // Updated in Fulu
			p2p.RPCGoodByeTopicV1:                   s.goodbyeRPCHandler,
			p2p.RPCBlocksByRangeTopicV2:             s.beaconBlocksByRangeRPCHandler,
			p2p.RPCBlocksByRootTopicV2:              s.beaconBlocksRootRPCHandler,
			p2p.RPCPingTopicV1:                      s.pingHandler,
			p2p.RPCMetaDataTopicV3:                  s.metaDataHandler,                     // Updated in Fulu
			p2p.RPCBlobSidecarsByRootTopicV1:        s.blobSidecarByRootRPCHandler,         // Modified in Fulu
			p2p.RPCBlobSidecarsByRangeTopicV1:       s.blobSidecarsByRangeRPCHandler,       // Modified in Fulu
			p2p.RPCDataColumnSidecarsByRootTopicV1:  s.dataColumnSidecarByRootRPCHandler,   // Added in Fulu
			p2p.RPCDataColumnSidecarsByRangeTopicV1: s.dataColumnSidecarsByRangeRPCHandler, // Added in Fulu
		}, nil
	}

	// Electra: https://github.com/ethereum/consensus-specs/blob/master/specs/electra/p2p-interface.md#messages
	if forkIndex >= version.Electra {
		return map[string]rpcHandler{
			p2p.RPCStatusTopicV1:              s.statusRPCHandler,
			p2p.RPCGoodByeTopicV1:             s.goodbyeRPCHandler,
			p2p.RPCBlocksByRangeTopicV2:       s.beaconBlocksByRangeRPCHandler, // Modified in Electra
			p2p.RPCBlocksByRootTopicV2:        s.beaconBlocksRootRPCHandler,    // Modified in Electra
			p2p.RPCPingTopicV1:                s.pingHandler,
			p2p.RPCMetaDataTopicV2:            s.metaDataHandler,
			p2p.RPCBlobSidecarsByRootTopicV1:  s.blobSidecarByRootRPCHandler,   // Modified in Electra
			p2p.RPCBlobSidecarsByRangeTopicV1: s.blobSidecarsByRangeRPCHandler, // Modified in Electra
		}, nil
	}

	// Deneb: https://github.com/ethereum/consensus-specs/blob/master/specs/deneb/p2p-interface.md#messages
	if forkIndex >= version.Deneb {
		return map[string]rpcHandler{
			p2p.RPCStatusTopicV1:              s.statusRPCHandler,
			p2p.RPCGoodByeTopicV1:             s.goodbyeRPCHandler,
			p2p.RPCBlocksByRangeTopicV2:       s.beaconBlocksByRangeRPCHandler, // Modified in Deneb
			p2p.RPCBlocksByRootTopicV2:        s.beaconBlocksRootRPCHandler,    // Modified in Deneb
			p2p.RPCPingTopicV1:                s.pingHandler,
			p2p.RPCMetaDataTopicV2:            s.metaDataHandler,
			p2p.RPCBlobSidecarsByRootTopicV1:  s.blobSidecarByRootRPCHandler,   // Added in Deneb
			p2p.RPCBlobSidecarsByRangeTopicV1: s.blobSidecarsByRangeRPCHandler, // Added in Deneb
		}, nil
	}

	// Capella: https://github.com/ethereum/consensus-specs/blob/master/specs/capella/p2p-interface.md#messages
	// Bellatrix: https://github.com/ethereum/consensus-specs/blob/master/specs/bellatrix/p2p-interface.md#messages
	// Altair: https://github.com/ethereum/consensus-specs/blob/master/specs/altair/p2p-interface.md#messages
	if forkIndex >= version.Altair {
		handler := map[string]rpcHandler{
			p2p.RPCStatusTopicV1:        s.statusRPCHandler,
			p2p.RPCGoodByeTopicV1:       s.goodbyeRPCHandler,
			p2p.RPCBlocksByRangeTopicV2: s.beaconBlocksByRangeRPCHandler, // Updated in Altair and modified in Bellatrix and Capella
			p2p.RPCBlocksByRootTopicV2:  s.beaconBlocksRootRPCHandler,    // Updated in Altair and modified in Bellatrix and Capella
			p2p.RPCPingTopicV1:          s.pingHandler,
			p2p.RPCMetaDataTopicV2:      s.metaDataHandler, // Updated in Altair
		}

		if features.Get().EnableLightClient {
			handler[p2p.RPCLightClientBootstrapTopicV1] = s.lightClientBootstrapRPCHandler
			handler[p2p.RPCLightClientUpdatesByRangeTopicV1] = s.lightClientUpdatesByRangeRPCHandler
			handler[p2p.RPCLightClientFinalityUpdateTopicV1] = s.lightClientFinalityUpdateRPCHandler
			handler[p2p.RPCLightClientOptimisticUpdateTopicV1] = s.lightClientOptimisticUpdateRPCHandler
		}

		return handler, nil
	}

	// PhaseO: https://github.com/ethereum/consensus-specs/blob/master/specs/phase0/p2p-interface.md#messages
	if forkIndex >= version.Phase0 {
		return map[string]rpcHandler{
			p2p.RPCStatusTopicV1:        s.statusRPCHandler,
			p2p.RPCGoodByeTopicV1:       s.goodbyeRPCHandler,
			p2p.RPCBlocksByRangeTopicV1: s.beaconBlocksByRangeRPCHandler,
			p2p.RPCBlocksByRootTopicV1:  s.beaconBlocksRootRPCHandler,
			p2p.RPCPingTopicV1:          s.pingHandler,
			p2p.RPCMetaDataTopicV1:      s.metaDataHandler,
		}, nil
	}

	return nil, errors.Errorf("RPC handler not found for fork index %d", forkIndex)
}

// addedRPCHandlerByTopic returns the RPC handlers that are added in the new map that are not present in the old map.
func addedRPCHandlerByTopic(previous, next map[string]rpcHandler) map[string]rpcHandler {
	added := make(map[string]rpcHandler, len(next))

	for topic, handler := range next {
		if _, ok := previous[topic]; !ok {
			added[topic] = handler
		}
	}

	return added
}

// removedTopics returns the topics that are removed in the new map that are not present in the old map.
func removedRPCTopics(previous, next map[string]rpcHandler) map[string]bool {
	removed := make(map[string]bool)

	for topic := range previous {
		if _, ok := next[topic]; !ok {
			removed[topic] = true
		}
	}

	return removed
}

// registerRPCHandlers for p2p RPC.
func (s *Service) registerRPCHandlers(nse params.NetworkScheduleEntry) error {
	if s.digestActionDone(nse.ForkDigest, registerRpcOnce) {
		return nil
	}
	// Get the RPC handlers for the current epoch.
	handlerByTopic, err := s.rpcHandlerByTopicFromFork(nse.VersionEnum)
	if err != nil {
		return errors.Wrap(err, "rpc handler by topic from epoch")
	}

	// Register the RPC handlers for the current epoch.
	for topic, handler := range handlerByTopic {
		s.registerRPC(topic, handler)
	}

	return nil
}

// registerRPC for a given topic with an expected protobuf message type.
func (s *Service) registerRPC(baseTopic string, handle rpcHandler) {
	topic := baseTopic + s.cfg.p2p.Encoding().ProtocolSuffix()
	log := log.WithField("topic", topic)
	s.cfg.p2p.SetStreamHandler(topic, func(stream network.Stream) {
		defer func() {
			if r := recover(); r != nil {
				log.WithField("error", r).
					WithField("recoveredAt", "registerRPC").
					WithField("stack", string(debug.Stack())).
					Error("Panic occurred")
			}
		}()

		ctx, cancel := context.WithTimeout(s.ctx, ttfbTimeout)
		defer cancel()

		conn := stream.Conn()
		remotePeer := conn.RemotePeer()

		// Resetting after closing is a no-op so defer a reset in case something goes wrong.
		// It's up to the handler to Close the stream (send an EOF) if
		// it successfully writes a response. We don't blindly call
		// Close here because we may have only written a partial
		// response.
		// About the special case for quic-v1, please see:
		// https://github.com/quic-go/quic-go/issues/3291
		defer func() {
			if strings.Contains(stream.Conn().RemoteMultiaddr().String(), "quic-v1") {
				time.Sleep(2 * time.Second)
			}

			_err := stream.Reset()
			_ = _err
		}()

		ctx, span := trace.StartSpan(ctx, "sync.rpc")
		defer span.End()
		span.SetAttributes(trace.StringAttribute("topic", topic))
		span.SetAttributes(trace.StringAttribute("peer", remotePeer.String()))
		log := log.WithFields(logrus.Fields{"peer": remotePeer.String(), "topic": string(stream.Protocol())})

		// Check before hand that peer is valid.
		if err := s.cfg.p2p.Peers().IsBad(remotePeer); err != nil {
			if err := s.sendGoodByeAndDisconnect(ctx, p2ptypes.GoodbyeCodeBanned, remotePeer); err != nil {
				log.WithError(err).Debug("Could not disconnect from peer")
			}
			return
		}
		// Validate request according to peer limits.
		if err := s.rateLimiter.validateRawRpcRequest(stream, 1); err != nil {
			log.WithError(err).Debug("Could not validate rpc request from peer")
			return
		}
		s.rateLimiter.addRawStream(stream)

		if err := stream.SetReadDeadline(time.Now().Add(ttfbTimeout)); err != nil {
			log.WithError(err).Debug("Could not set stream read deadline")
			return
		}

		base, ok := p2p.RPCTopicMappings[baseTopic]
		if !ok {
			log.Errorf("Could not retrieve base message for topic %s", baseTopic)
			return
		}
		t := reflect.TypeOf(base)
		// Copy Base
		base = reflect.New(t)

		// Increment message received counter.
		messageReceivedCounter.WithLabelValues(topic).Inc()

		// since some requests do not have any data in the payload, we
		// do not decode anything.
		topics := map[string]bool{
			p2p.RPCMetaDataTopicV1:                    true,
			p2p.RPCMetaDataTopicV2:                    true,
			p2p.RPCMetaDataTopicV3:                    true,
			p2p.RPCLightClientOptimisticUpdateTopicV1: true,
			p2p.RPCLightClientFinalityUpdateTopicV1:   true,
		}

		if topics[baseTopic] {
			if err := handle(ctx, base, stream); err != nil {
				messageFailedProcessingCounter.WithLabelValues(topic).Inc()
				if !errors.Is(err, p2ptypes.ErrWrongForkDigestVersion) {
					log.WithError(err).Debug("Could not handle p2p RPC")
				}
				tracing.AnnotateError(span, err)
			}
			return
		}

		// Given we have an input argument that can be pointer or the actual object, this gives us
		// a way to check for its reflect.Kind and based on the result, we can decode
		// accordingly.
		if t.Kind() == reflect.Ptr {
			msg, ok := reflect.New(t.Elem()).Interface().(ssz.Unmarshaler)
			if !ok {
				log.Errorf("message of %T does not support marshaller interface", msg)
				return
			}
			if err := s.cfg.p2p.Encoding().DecodeWithMaxLength(stream, msg); err != nil {
				logStreamErrors(err, topic)
				tracing.AnnotateError(span, err)
				s.downscorePeer(remotePeer, "registerRpcError")
				return
			}
			if err := handle(ctx, msg, stream); err != nil {
				messageFailedProcessingCounter.WithLabelValues(topic).Inc()
				if !errors.Is(err, p2ptypes.ErrWrongForkDigestVersion) {
					log.WithError(err).Debug("Could not handle p2p RPC")
				}
				tracing.AnnotateError(span, err)
			}
		} else {
			nTyp := reflect.New(t)
			msg, ok := nTyp.Interface().(ssz.Unmarshaler)
			if !ok {
				log.Errorf("message of %T does not support marshaller interface", msg)
				return
			}
			if err := s.cfg.p2p.Encoding().DecodeWithMaxLength(stream, msg); err != nil {
				logStreamErrors(err, topic)
				tracing.AnnotateError(span, err)
				s.downscorePeer(remotePeer, "registerRpcError")
				return
			}
			if err := handle(ctx, nTyp.Elem().Interface(), stream); err != nil {
				messageFailedProcessingCounter.WithLabelValues(topic).Inc()
				if !errors.Is(err, p2ptypes.ErrWrongForkDigestVersion) {
					log.WithError(err).Debug("Could not handle p2p RPC")
				}
				tracing.AnnotateError(span, err)
			}
		}
	})
	log.Debug("Registered new RPC handler")
}

func logStreamErrors(err error, topic string) {
	if isUnwantedError(err) {
		return
	}
	if strings.Contains(topic, p2p.RPCGoodByeTopicV1) {
		log.WithError(err).WithField("topic", topic).Trace("Could not decode goodbye stream message")
		return
	}
	log.WithError(err).WithField("topic", topic).Debug("Could not decode stream message")
}
