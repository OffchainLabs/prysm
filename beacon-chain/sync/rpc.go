package sync

import (
	"context"
	"reflect"
	"runtime/debug"
	"strings"
	"time"

	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	p2ptypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/types"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
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
	// Fulu: https://github.com/ethereum/consensus-specs/blob/dev/specs/fulu/p2p-interface.md#messages
	if forkIndex >= version.Fulu {
		return map[string]rpcHandler{
			p2p.RPCStatusTopicV1:                    s.statusRPCHandler,
			p2p.RPCGoodByeTopicV1:                   s.goodbyeRPCHandler,
			p2p.RPCBlocksByRangeTopicV2:             s.beaconBlocksByRangeRPCHandler,
			p2p.RPCBlocksByRootTopicV2:              s.beaconBlocksRootRPCHandler,
			p2p.RPCPingTopicV1:                      s.pingHandler,
			p2p.RPCMetaDataTopicV3:                  s.metaDataHandler, // Modified in Fulu
			p2p.RPCBlobSidecarsByRootTopicV1:        s.blobSidecarByRootRPCHandler,
			p2p.RPCBlobSidecarsByRangeTopicV1:       s.blobSidecarsByRangeRPCHandler,
			p2p.RPCDataColumnSidecarsByRootTopicV1:  s.dataColumnSidecarByRootRPCHandler,   // Added in Fulu
			p2p.RPCDataColumnSidecarsByRangeTopicV1: s.dataColumnSidecarsByRangeRPCHandler, // Added in Fulu
		}, nil
	}

	// Electra: https://github.com/ethereum/consensus-specs/blob/dev/specs/electra/p2p-interface.md#messages
	if forkIndex >= version.Electra {
		return map[string]rpcHandler{
			p2p.RPCStatusTopicV1:              s.statusRPCHandler,
			p2p.RPCGoodByeTopicV1:             s.goodbyeRPCHandler,
			p2p.RPCBlocksByRangeTopicV2:       s.beaconBlocksByRangeRPCHandler,
			p2p.RPCBlocksByRootTopicV2:        s.beaconBlocksRootRPCHandler,
			p2p.RPCPingTopicV1:                s.pingHandler,
			p2p.RPCMetaDataTopicV2:            s.metaDataHandler,
			p2p.RPCBlobSidecarsByRootTopicV1:  s.blobSidecarByRootRPCHandler,   // Modified in Electra
			p2p.RPCBlobSidecarsByRangeTopicV1: s.blobSidecarsByRangeRPCHandler, // Modified in Electra
		}, nil
	}

	// Deneb: https://github.com/ethereum/consensus-specs/blob/dev/specs/deneb/p2p-interface.md#messages
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

	// Capella: https://github.com/ethereum/consensus-specs/blob/dev/specs/capella/p2p-interface.md#messages
	// Bellatrix: https://github.com/ethereum/consensus-specs/blob/dev/specs/bellatrix/p2p-interface.md#messages
	// Altair: https://github.com/ethereum/consensus-specs/blob/dev/specs/altair/p2p-interface.md#messages
	if forkIndex >= version.Altair {
		return map[string]rpcHandler{
			p2p.RPCStatusTopicV1:        s.statusRPCHandler,
			p2p.RPCGoodByeTopicV1:       s.goodbyeRPCHandler,
			p2p.RPCBlocksByRangeTopicV2: s.beaconBlocksByRangeRPCHandler, // Updated in Altair and modified in Capella
			p2p.RPCBlocksByRootTopicV2:  s.beaconBlocksRootRPCHandler,    // Updated in Altair and modified in Capella
			p2p.RPCPingTopicV1:          s.pingHandler,
			p2p.RPCMetaDataTopicV2:      s.metaDataHandler, // Updated in Altair
		}, nil
	}

	// PhaseO: https://github.com/ethereum/consensus-specs/blob/dev/specs/phase0/p2p-interface.md#messages
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

// rpcHandlerByTopic returns the RPC handlers for a given epoch.
func (s *Service) rpcHandlerByTopicFromEpoch(epoch primitives.Epoch) (map[string]rpcHandler, error) {
	// Get the beacon config.
	beaconConfig := params.BeaconConfig()

	if epoch >= beaconConfig.FuluForkEpoch {
		return s.rpcHandlerByTopicFromFork(version.Fulu)
	}

	if epoch >= beaconConfig.ElectraForkEpoch {
		return s.rpcHandlerByTopicFromFork(version.Electra)
	}

	if epoch >= beaconConfig.DenebForkEpoch {
		return s.rpcHandlerByTopicFromFork(version.Deneb)
	}

	if epoch >= beaconConfig.CapellaForkEpoch {
		return s.rpcHandlerByTopicFromFork(version.Capella)
	}

	if epoch >= beaconConfig.BellatrixForkEpoch {
		return s.rpcHandlerByTopicFromFork(version.Bellatrix)
	}

	if epoch >= beaconConfig.AltairForkEpoch {
		return s.rpcHandlerByTopicFromFork(version.Altair)
	}

	return s.rpcHandlerByTopicFromFork(version.Phase0)
}

// addedRPCHandlerByTopic returns the RPC handlers that are added in the new map that are not present in the old map.
func addedRPCHandlerByTopic(previous, next map[string]rpcHandler) map[string]rpcHandler {
	added := make(map[string]rpcHandler)

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
func (s *Service) registerRPCHandlers() error {
	// Get the current epoch.
	currentSlot := s.cfg.clock.CurrentSlot()
	currentEpoch := slots.ToEpoch(currentSlot)

	// Get the RPC handlers for the current epoch.
	handlerByTopic, err := s.rpcHandlerByTopicFromEpoch(currentEpoch)
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
		log := log.WithField("peer", stream.Conn().RemotePeer().String()).WithField("topic", string(stream.Protocol()))

		// Check before hand that peer is valid.
		if err := s.cfg.p2p.Peers().IsBad(remotePeer); err != nil {
			if err := s.sendGoodByeAndDisconnect(ctx, p2ptypes.GoodbyeCodeBanned, remotePeer); err != nil {
				log.WithError(err).Debug("Could not disconnect from peer")
			}
			return
		}
		// Validate request according to peer limits.
		if err := s.rateLimiter.validateRawRpcRequest(stream); err != nil {
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

		// since metadata requests do not have any data in the payload, we
		// do not decode anything.
		if baseTopic == p2p.RPCMetaDataTopicV1 || baseTopic == p2p.RPCMetaDataTopicV2 || baseTopic == p2p.RPCMetaDataTopicV3 {
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
				s.cfg.p2p.Peers().Scorers().BadResponsesScorer().Increment(stream.Conn().RemotePeer())
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
				s.cfg.p2p.Peers().Scorers().BadResponsesScorer().Increment(stream.Conn().RemotePeer())
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
