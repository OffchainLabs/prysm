package partialdatacolumnbroadcaster

import (
	"bytes"
	"context"
	stderrors "errors"
	"fmt"
	"iter"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/internal/logrusadapter"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p-pubsub/partialmessages"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const TTLInSlots = 3

const logPackage = "beacon-chain/p2p/partialdatacolumnbroadcaster"

var errInvalidHeader = errors.New("invalid header")

const dataColumnSidecarPrefix = "data_column_sidecar_"

func extractColumnIndexFromTopic(topic string) (uint64, error) {
	idx := strings.Index(topic, dataColumnSidecarPrefix)
	if idx == -1 {
		return 0, errors.New("could not extract column index from topic")
	}
	sub := topic[idx+len(dataColumnSidecarPrefix):]
	end := strings.Index(sub, "/")
	if end != -1 {
		sub = sub[:end]
	}
	return strconv.ParseUint(sub, 10, 64)
}

// ColumnCallbacks is the interface that the broadcaster uses to validate and handle
// partial data column headers and cells.
type ColumnCallbacks interface {
	// PartialVerifierFromHeader builds and validates a partial column from a new header.
	// Returns (verifier, result, err) where:
	//   - ValidationReject, err!=nil: peer should be penalized
	//   - ValidationIgnore, err!=nil: don't penalize, just ignore
	//   - ValidationAccept, err=nil: valid verifier
	PartialVerifierFromHeader(col *blocks.PartialDataColumn) (verifier *verification.PartialColumnVerifier, result pubsub.ValidationResult, err error)
	// PartialVerifierFromTrustedColumn creates a verifier from a previously validated column.
	PartialVerifierFromTrustedColumn(col *blocks.PartialDataColumn) (*verification.PartialColumnVerifier, error)
	// ValidateColumn validates the KZG proofs of the given cells.
	ValidateColumn(cells []blocks.CellProofBundle) error
	// HandleColumn is called when a partial column has been fully reconstructed.
	HandleColumn(topic string, col blocks.VerifiedRODataColumn)
	// HandleHeader is called when a new partial data column header is first validated.
	HandleHeader(header *ethpb.PartialDataColumnHeader, groupID string)
}

// Broadcaster is the behaviour of the partial data column broadcaster used by the rest of the node.
type Broadcaster interface {
	Start(callbacks ColumnCallbacks)
	Publish(ctx context.Context, topicsAndColumns iter.Seq2[string, blocks.PartialDataColumn]) error
	AppendPubSubOpts(opts []pubsub.Option) []pubsub.Option
	Subscribe(ctx context.Context, t *pubsub.Topic) error
	Unsubscribe(ctx context.Context, topic string) error
}

var _ Broadcaster = (*PartialColumnBroadcaster)(nil)

type PartialColumnBroadcaster struct {
	logger *logrus.Entry

	ctx context.Context

	peerFeedback      func(topic string, peer peer.ID, kind pubsub.PeerFeedbackKind) error
	publishPartialCol func(topic string, groupID []byte, col *blocks.PartialDataColumn) error
	callbacks         ColumnCallbacks
	// map topic -> *pubsub.Topic
	topics                           map[string]*pubsub.Topic
	peerFeedbackSemaphore            chan struct{}
	concurrentValidatorSemaphore     chan struct{}
	concurrentHeaderHandlerSemaphore chan struct{}
	// map topic -> map[groupID]PartialColumnVerifier
	partialMsgStore map[string]map[string]*verification.PartialColumnVerifier
	groupTTL        map[string]int8
	// validHeaderCache caches validated headers by group ID (works across topics)
	validHeaderCache map[string]*ethpb.PartialDataColumnHeader
	// map groupID -> map[peer.ID]bool
	headerSentCache  map[string]map[peer.ID]bool
	incomingReq      chan request
	eagerPushed      map[string]*eagerPushAgg
	republishSkipped map[string]map[uint64]bool
}

type eagerPushAgg struct {
	indices map[uint64]bool
	peers   map[peer.ID]bool
}

type requestKind uint8

const (
	requestKindPublish requestKind = iota
	requestKindSubscribe
	requestKindUnsubscribe
	requestKindGossip
	requestKindHandleIncomingRPC
	requestKindCellsValidated
)

func (k requestKind) String() string {
	switch k {
	case requestKindPublish:
		return "publish"
	case requestKindSubscribe:
		return "subscribe"
	case requestKindUnsubscribe:
		return "unsubscribe"
	case requestKindGossip:
		return "gossip"
	case requestKindHandleIncomingRPC:
		return "handle_incoming_rpc"
	case requestKindCellsValidated:
		return "cells_validated"
	default:
		return "unknown"
	}
}

type requestValues struct {
	cellsValidated *cellsValidated
	unsub          unsubscribe
	incomingRPC    incomingPartialRPC
	sub            subscribe
	publish        publish
	gossip         gossip
}

type request struct {
	requestValues
	ctx      context.Context
	kind     requestKind
	response chan error
}

func newRequest(ctx context.Context, kind requestKind, v requestValues) request {
	return request{
		requestValues: v,
		ctx:           ctx,
		kind:          kind,
		response:      make(chan error, 1),
	}
}

// finish sends the result to the caller waiting on the response channel.
func (r request) finish(err error) {
	r.response <- err
}

// enqueue creates and enqueues a request, blocking until it is accepted.
// Returns an error if the broadcaster has stopped or the context has been cancelled.
// A nil ctx is permitted for fire-and-forget requests that have no cancellation.
func (p *PartialColumnBroadcaster) enqueue(ctx context.Context, kind requestKind, v requestValues) (request, error) {
	req := newRequest(ctx, kind, v)
	select {
	case p.incomingReq <- req:
		return req, nil
	case <-p.ctx.Done():
		return req, errPartialBroadcasterStopped
	case <-ctx.Done():
		return req, ctx.Err()
	}
}

// tryEnqueue creates and enqueues a request without blocking.
// Returns false if the request channel is full.
func (p *PartialColumnBroadcaster) tryEnqueue(kind requestKind, v requestValues) (request, bool) {
	req := newRequest(p.ctx, kind, v)
	select {
	case p.incomingReq <- req:
		return req, true
	default:
		return req, false
	}
}

// waitForResponse blocks until the request has been processed and returns the result.
// If the request's context is cancelled before a response arrives, it returns the context error.
func (r request) waitForResponse() error {
	select {
	case err := <-r.response:
		return err
	case <-r.ctx.Done():
		return r.ctx.Err()
	}
}

type publish struct {
	topicsAndColumns iter.Seq2[string, blocks.PartialDataColumn]
}

type subscribe struct {
	t *pubsub.Topic
}

type unsubscribe struct {
	topic string
}

type incomingPartialRPC struct {
	*pubsub_pb.PartialMessagesExtension
	from    peer.ID
	message *ethpb.PartialDataColumnSidecar
	isGloas bool
}

func (r incomingPartialRPC) logFields() logrus.Fields {
	return logrus.Fields{
		"from":  r.from,
		"topic": r.GetTopicID(),
		"group": fmt.Sprintf("%#x", r.GroupID),
	}
}

type cellsValidated struct {
	validationTook time.Duration
	topic          string
	group          []byte
	cellIndices    []uint64
	cells          []blocks.CellProofBundle
}

func (c *cellsValidated) logFields() logrus.Fields {
	return logrus.Fields{
		"topic": c.topic,
		"group": fmt.Sprintf("%#x", c.group),
	}
}

// gossip is used when we are republishing our PartialDataColumn to gossip peers.
type gossip struct {
	topic   string
	groupID []byte
}

func NewBroadcaster(ctx context.Context, logger *logrus.Logger) *PartialColumnBroadcaster {
	concurrency := params.BeaconConfig().DataColumnSidecarSubnetCount
	return &PartialColumnBroadcaster{
		ctx:              ctx,
		topics:           make(map[string]*pubsub.Topic),
		partialMsgStore:  make(map[string]map[string]*verification.PartialColumnVerifier),
		groupTTL:         make(map[string]int8),
		validHeaderCache: make(map[string]*ethpb.PartialDataColumnHeader),
		headerSentCache:  make(map[string]map[peer.ID]bool),
		eagerPushed:      make(map[string]*eagerPushAgg),
		republishSkipped: make(map[string]map[uint64]bool),

		// GossipSub sends the messages to this channel. The buffer should be
		// big enough to avoid dropping messages. We don't want to block the gossipsub event loop for this.
		incomingReq: make(chan request, 128*16),
		logger:      logger.WithField("package", logPackage),

		peerFeedbackSemaphore:            make(chan struct{}, concurrency),
		concurrentValidatorSemaphore:     make(chan struct{}, concurrency),
		concurrentHeaderHandlerSemaphore: make(chan struct{}, concurrency),
	}
}

// onEmitGossip enqueues a gossip request for the broadcaster's event loop.
func (p *PartialColumnBroadcaster) onEmitGossip(topic string, groupID []byte, _ []peer.ID, _ map[peer.ID]blocks.PartialDataColumnPeerState) {
	// Drop gossip emission if we have too many pending requests.
	p.tryEnqueue(requestKindGossip, requestValues{
		gossip: gossip{
			topic:   topic,
			groupID: groupID,
		},
	})
}

// onIncomingRPC processes an incoming partial message RPC by updating peer state
// and enqueuing the message for the broadcaster's event loop.
func (p *PartialColumnBroadcaster) onIncomingRPC(from peer.ID, peerStates map[peer.ID]blocks.PartialDataColumnPeerState, rpc *pubsub_pb.PartialMessagesExtension) error {
	if rpc == nil {
		return nil
	}

	// Parse the group ID to detect the fork (Fulu 0x00||root, 33B; Gloas 0x01||SSZ(groupID), 41B).
	// This validates the version byte, length, and (for Gloas) the SSZ encoding in one place.
	isGloas, _, _, err := blocks.ParsePartialColumnGroupID(rpc.GetGroupID())
	if err != nil {
		p.logger.WithError(err).WithFields(logrus.Fields{
			"peer":  from,
			"topic": rpc.GetTopicID(),
			"got":   len(rpc.GetGroupID()),
		}).Debug("Invalid group ID")
		p.reportPeerFeedbackAsync(rpc.GetTopicID(), from, pubsub.PeerFeedbackInvalidMessage)
		return errors.Wrap(err, "parse partial column group id")
	}

	columnIndex, err := extractColumnIndexFromTopic(rpc.GetTopicID())
	if err != nil || columnIndex >= fieldparams.NumberOfColumns {
		p.logger.WithError(err).WithFields(logrus.Fields{
			"peer":        from,
			"topic":       rpc.GetTopicID(),
			"columnIndex": columnIndex,
			"maxColumns":  fieldparams.NumberOfColumns,
		}).Debug("Invalid topic ID: column index missing or out of bounds")
		p.reportPeerFeedbackAsync(rpc.GetTopicID(), from, pubsub.PeerFeedbackInvalidMessage)
		return errors.Errorf("invalid topic ID %q: column index missing or out of bounds", rpc.GetTopicID())
	}

	nextPeerState, message, err := updatePeerStateFromIncomingRPC(peerStates[from], rpc, isGloas)
	if err != nil {
		return errors.Wrap(err, "update peer state from incoming rpc")
	}
	_, ok := p.tryEnqueue(requestKindHandleIncomingRPC, requestValues{
		incomingRPC: incomingPartialRPC{rpc, from, message, isGloas},
	})
	if !ok {
		p.logger.WithFields(logrus.Fields{
			"peer":  from,
			"topic": rpc.GetTopicID(),
			"group": fmt.Sprintf("%#x", rpc.GetGroupID()),
		}).Warn("Dropping incoming partial RPC")
		return errors.New("incomingReq channel is full, dropping RPC")
	}
	peerStates[from] = nextPeerState
	return nil
}

func (p *PartialColumnBroadcaster) reportPeerFeedbackAsync(topic string, from peer.ID, kind pubsub.PeerFeedbackKind) {
	select {
	case p.peerFeedbackSemaphore <- struct{}{}:
		go func() {
			defer func() { <-p.peerFeedbackSemaphore }()
			// return early if the context is done (e.g. the broadcaster is shutting down) as gossipsub loop
			// might already be exiting
			if p.ctx.Err() != nil {
				return
			}
			_ = p.peerFeedback(topic, from, kind)
		}()
	default:
		p.logger.WithFields(logrus.Fields{
			"peer":  from,
			"topic": topic,
		}).Warn("Peer feedback semaphore saturated, dropping feedback")
	}
}

// AppendPubSubOpts adds the necessary pubsub options to enable partial messages.
func (p *PartialColumnBroadcaster) AppendPubSubOpts(opts []pubsub.Option) []pubsub.Option {
	slogger := slog.New(logrusadapter.Handler{Logger: p.logger.Logger}).With("package", logPackage)
	opts = append(opts,
		pubsub.WithPartialMessagesExtension(&partialmessages.PartialMessagesExtension[blocks.PartialDataColumnPeerState]{
			Logger:        slogger,
			OnEmitGossip:  p.onEmitGossip,
			OnIncomingRPC: p.onIncomingRPC,
		}),
		func(ps *pubsub.PubSub) error {
			p.peerFeedback = ps.PeerFeedback
			p.publishPartialCol = func(topic string, groupID []byte, col *blocks.PartialDataColumn) error {
				if _, ok := p.headerSentCache[string(groupID)]; !ok {
					p.headerSentCache[string(groupID)] = make(map[peer.ID]bool)
				}
				onEagerPush := func(remote peer.ID) {
					p.recordEagerPush(groupID, col.Index(), remote)
				}
				return pubsub.PublishPartial(ps, topic, groupID, col.PublishActionsFn(p.headerSentCache[string(groupID)], onEagerPush))
			}
			return nil
		},
	)
	return opts
}

// Start starts the event loop of the PartialColumnBroadcaster.
// It accepts the required validator and handler functions, returning an error if any is nil.
// Note: The event loop is blocking and so the broadcaster should be started in a goroutine.
func (p *PartialColumnBroadcaster) Start(callbacks ColumnCallbacks) {
	p.callbacks = callbacks
	p.loop()
}

var (
	errPartialBroadcasterStopped = errors.New("partial column broadcaster stopped")
	errUnknownRequestKind        = errors.New("unknown request kind")
)

func (p *PartialColumnBroadcaster) loop() {
	cleanup := time.NewTicker(time.Second * time.Duration(params.BeaconConfig().SecondsPerSlot))
	for {
		select {
		case req := <-p.incomingReq:
			// This check enables the requester to cancel the request by cancelling the given context.
			if req.ctx.Err() != nil {
				p.logger.WithError(req.ctx.Err()).WithField("kind", req.kind.String()).
					Debug("Context canceled for PartialColumnBroadcaster event.") // Debug log level to avoid log storm at node shutdown.
				req.finish(req.ctx.Err())
				continue
			}
			var err error
			switch req.kind {
			case requestKindPublish:
				err = p.publish(req.publish.topicsAndColumns)
			case requestKindSubscribe:
				err = p.subscribe(req.sub.t)
			case requestKindUnsubscribe:
				err = p.unsubscribe(req.unsub.topic)
			case requestKindGossip:
				p.gossip(req.gossip.topic, req.gossip.groupID)
			case requestKindHandleIncomingRPC:
				err = p.handleIncomingRPC(req.incomingRPC)
			case requestKindCellsValidated:
				err = p.handleCellsValidated(req.cellsValidated)
			default:
				err = errUnknownRequestKind
			}
			if err != nil {
				p.logger.WithField("kind", req.kind.String()).WithError(err).
					Error("Failure handling PartialColumnBroadcaster event.")
				err = errors.Wrap(err, "partial column broadcaster "+req.kind.String()+" event")
			}
			req.finish(err)
		case <-p.ctx.Done():
			// Drain remaining requests before exiting the loop.
			for {
				select {
				case req := <-p.incomingReq:
					req.finish(errPartialBroadcasterStopped)
				default:
					return
				}
			}
		case <-cleanup.C:
			p.flushAggregatedLogs()
			p.evictExpiredGroups()
		}
	}
}

func (p *PartialColumnBroadcaster) recordEagerPush(groupID []byte, columnIndex uint64, remote peer.ID) {
	agg, ok := p.eagerPushed[string(groupID)]
	if !ok {
		agg = &eagerPushAgg{indices: make(map[uint64]bool), peers: make(map[peer.ID]bool)}
		p.eagerPushed[string(groupID)] = agg
	}
	agg.indices[columnIndex] = true
	agg.peers[remote] = true
}

func (p *PartialColumnBroadcaster) recordRepublishSkip(groupID []byte, columnIndex uint64) {
	indices, ok := p.republishSkipped[string(groupID)]
	if !ok {
		indices = make(map[uint64]bool)
		p.republishSkipped[string(groupID)] = indices
	}
	indices[columnIndex] = true
}

func (p *PartialColumnBroadcaster) flushAggregatedLogs() {
	for groupID, agg := range p.eagerPushed {
		p.logger.WithFields(logrus.Fields{
			"group":   fmt.Sprintf("%#x", groupID),
			"count":   len(agg.indices),
			"indices": helpers.SortedPrettySliceFromMap(agg.indices),
			"peers":   len(agg.peers),
		}).Debug("Eager pushed partial data columns")
		delete(p.eagerPushed, groupID)
	}
	for groupID, indices := range p.republishSkipped {
		p.logger.WithFields(logrus.Fields{
			"group":   fmt.Sprintf("%#x", groupID),
			"count":   len(indices),
			"indices": helpers.SortedPrettySliceFromMap(indices),
		}).Debug("Columns not published, skipping republish")
		delete(p.republishSkipped, groupID)
	}
}

func (p *PartialColumnBroadcaster) evictExpiredGroups() {
	for groupID, ttl := range p.groupTTL {
		if ttl > 0 {
			p.groupTTL[groupID] = ttl - 1
			continue
		}

		delete(p.groupTTL, groupID)
		delete(p.validHeaderCache, groupID)
		delete(p.headerSentCache, groupID)
		for topic, msgStore := range p.partialMsgStore {
			delete(msgStore, groupID)
			if len(msgStore) == 0 {
				delete(p.partialMsgStore, topic)
			}
		}
	}
}

func (p *PartialColumnBroadcaster) getPartialVerifier(topic string, group []byte) *verification.PartialColumnVerifier {
	topicStore, ok := p.partialMsgStore[topic]
	if !ok {
		return nil
	}
	verifier, ok := topicStore[string(group)]
	if !ok {
		return nil
	}
	return verifier
}

func (p *PartialColumnBroadcaster) getDataColumn(topic string, group []byte) *blocks.PartialDataColumn {
	verifier := p.getPartialVerifier(topic, group)
	if verifier == nil {
		return nil
	}
	return verifier.Column
}

func decodePartsMetadataFromPeerState(state *ethpb.PartialDataColumnPartsMetadata, expectedLength uint64) (*ethpb.PartialDataColumnPartsMetadata, error) {
	if state == nil {
		return blocks.NewPartsMetaWithNoAvailableAndNoRequests(expectedLength), nil
	}
	return state, nil
}

func updatePeerStateFromIncomingRPC(peerState blocks.PartialDataColumnPeerState, rpc *pubsub_pb.PartialMessagesExtension, isGloas bool) (blocks.PartialDataColumnPeerState,
	*ethpb.PartialDataColumnSidecar, error) {
	peerState = peerState.Clone()
	hasIncomingPartsMetadata := len(rpc.PartsMetadata) > 0
	hasMessage := len(rpc.PartialMessage) > 0

	if hasIncomingPartsMetadata {
		var incomingMeta ethpb.PartialDataColumnPartsMetadata
		if err := incomingMeta.UnmarshalSSZ(rpc.PartsMetadata); err != nil {
			return peerState, nil, errors.Wrap(err, "failed to unmarshal incoming parts metadata")
		}
		if incomingMeta.Available.Len() == 0 {
			return peerState, nil, errors.New("incoming parts metadata has 0 length availability")
		}

		if peerState.Recvd == nil {
			peerState.Recvd = &incomingMeta
		} else {
			if peerState.Recvd.Requests.Len() != incomingMeta.Requests.Len() {
				return peerState, nil, errors.New("failed to merge available cells into recvdState parts metadata. requests length mismatch")
			}
			peerState.Recvd.Requests = incomingMeta.Requests
			var err error
			peerState.Recvd.Available, err = peerState.Recvd.Available.Or(incomingMeta.Available)
			if err != nil {
				return peerState, nil, errors.Wrap(err, "failed to merge available cells into recvdState parts metadata")
			}
		}
	}

	// we've already handled the update to the peer state based on the incoming parts metadata,
	// so we can return early if there's no message to process.
	if !hasMessage {
		return peerState, nil, nil
	}

	message, err := blocks.DecodePartialColumnSidecar(rpc.PartialMessage, isGloas)
	if err != nil {
		return peerState, nil, errors.Wrap(err, "failed to unmarshal partial message data")
	}
	if len(message.CellsPresentBitmap) == 0 {
		return peerState, message, nil
	}

	nKzgCommitments := message.CellsPresentBitmap.Len()
	if nKzgCommitments == 0 {
		return peerState, nil, errors.New("length of cells present bitmap is 0")
	}

	// only update RecvdState using the incoming partial message if the peer did not send us their parts metadata
	if !hasIncomingPartsMetadata {
		recievedMeta, err := decodePartsMetadataFromPeerState(peerState.Recvd, nKzgCommitments)
		if err != nil {
			return peerState, nil, errors.Wrap(err, "received")
		}
		recvdState, err := blocks.MergeAvailableIntoPartsMetadata(recievedMeta, message.CellsPresentBitmap)
		if err != nil {
			return peerState, nil, errors.Wrap(err, "merge available cells into received parts metadata")
		}
		peerState.Recvd = recvdState
	}

	sentMeta, err := decodePartsMetadataFromPeerState(peerState.Sent, nKzgCommitments)
	if err != nil {
		return peerState, nil, errors.Wrap(err, "sent")
	}

	sentState, err := blocks.MergeAvailableIntoPartsMetadata(sentMeta, message.CellsPresentBitmap)
	if err != nil {
		return peerState, nil, errors.Wrap(err, "merge available cells into sent parts metadata")
	}
	peerState.Sent = sentState

	return peerState, message, nil
}

func (p *PartialColumnBroadcaster) handleIncomingRPC(rpc incomingPartialRPC) error {
	if p.peerFeedback == nil || p.publishPartialCol == nil {
		return errors.New("pubsub not initialized")
	}

	topicID := rpc.GetTopicID()
	// Only act on partial messages for topics we are currently subscribed to.
	// The topic ID is peer-controlled, so this prevents a peer from making us
	// allocate verifier/header state for columns we never asked for.
	if _, subscribed := p.topics[topicID]; !subscribed {
		p.logger.WithFields(rpc.logFields()).Debug("Ignoring partial message for unsubscribed topic")
		return nil
	}

	message := rpc.message
	hasMessage := message != nil

	groupID := rpc.GroupID
	ourVerifier := p.getPartialVerifier(topicID, groupID)
	var shouldRepublish bool

	if ourVerifier == nil && hasMessage {
		if rpc.isGloas {
			// Gloas has no header to seed a verifier from; we can only verify incoming cells once
			// we have published locally (block + bid commitments). Never buffer unsolicited cells.
			p.logger.WithFields(rpc.logFields()).Debug("Dropping Gloas partial cells: no local verifier")
			return nil
		}
		header, headerWasCached := p.getHeader(groupID, message)
		if header == nil {
			return nil
		}

		// downscore peer if invalid header
		if header.SignedBlockHeader == nil || header.SignedBlockHeader.Header == nil {
			p.logger.WithFields(rpc.logFields()).Debug("Header is missing signed block header or header")
			_ = p.peerFeedback(topicID, rpc.from, pubsub.PeerFeedbackInvalidMessage)
			return errors.New("header is missing signed block header or header")
		}

		// downscore peer if invalid header
		root, err := header.SignedBlockHeader.Header.HashTreeRoot()
		if err != nil {
			p.logger.WithFields(rpc.logFields()).WithError(err).Debug("Failed to get root from header")
			_ = p.peerFeedback(topicID, rpc.from, pubsub.PeerFeedbackInvalidMessage)
			return errors.Wrap(err, "failed to get root from header")
		}

		columnIndex, err := extractColumnIndexFromTopic(topicID)
		if err != nil {
			return errors.Wrap(err, "extract column index from topic")
		}

		verifier, err := p.makeVerifierFromHeader(root, header, columnIndex, headerWasCached, rpc)
		if err != nil {
			if err == errInvalidHeader {
				return nil
			}
			return errors.Wrap(err, "make verifier from header")
		}

		if !headerWasCached {
			p.logger.WithFields(rpc.logFields()).Debug("Handling header as it was previously not cached for this group")
			p.handleHeader(rpc, header)
		}

		// Save to store
		topicStore, ok := p.partialMsgStore[topicID]
		if !ok {
			topicStore = make(map[string]*verification.PartialColumnVerifier)
			p.partialMsgStore[topicID] = topicStore
		}
		topicStore[string(groupID)] = verifier
		p.groupTTL[string(groupID)] = TTLInSlots

		ourVerifier = verifier
		shouldRepublish = true
	}

	if ourVerifier == nil {
		// We don't have a partial column for this. Can happen if we got cells
		// without a header.
		return nil
	}
	ourDataColumn := ourVerifier.Column

	if hasMessage {
		err := p.handlePartialCells(ourDataColumn, message, rpc)
		if err != nil {
			return errors.Wrap(err, "handle partial cells")
		}
	}

	return p.republishColumn(ourDataColumn, rpc, shouldRepublish)
}

func (p *PartialColumnBroadcaster) makeVerifierFromHeader(root [fieldparams.RootLength]byte, header *ethpb.PartialDataColumnHeader, columnIndex uint64,
	headerWasCached bool, rpc incomingPartialRPC) (*verification.PartialColumnVerifier, error) {
	topicID := rpc.GetTopicID()

	if len(header.KzgCommitments) == 0 {
		p.logger.WithFields(rpc.logFields()).Debug("Ignoring partial column header with no KZG commitments")
		return nil, errInvalidHeader
	}

	newColumn, err := blocks.NewPartialDataColumn(
		root,
		header.SignedBlockHeader,
		columnIndex,
		header.KzgCommitments,
		header.KzgCommitmentsInclusionProof,
	)
	if err != nil {
		p.logger.WithError(err).WithFields(logrus.Fields{
			"topic":          topicID,
			"columnIndex":    columnIndex,
			"numCommitments": len(header.KzgCommitments),
		}).Error("Failed to create partial data column from header")
		return nil, errors.Wrap(err, "new partial data column")
	}

	if !bytes.Equal(newColumn.GroupID(), rpc.GroupID) {
		p.logger.WithFields(rpc.logFields()).Error("Group ID mismatch")
		// REJECT case: penalize the peer
		_ = p.peerFeedback(topicID, rpc.from, pubsub.PeerFeedbackInvalidMessage)
		return nil, errors.New("group ID mismatch")
	}

	if headerWasCached {
		verifier, err := p.callbacks.PartialVerifierFromTrustedColumn(&newColumn)
		if err != nil {
			p.logger.WithError(err).WithFields(logrus.Fields{
				"topic":          topicID,
				"columnIndex":    columnIndex,
				"numCommitments": len(header.KzgCommitments),
			}).Error("Failed to create partial column verifier from header")
			return nil, errors.Wrap(err, "partial verifier from trusted column")
		}
		return verifier, nil
	}
	verifier, result, err := p.callbacks.PartialVerifierFromHeader(&newColumn)
	if err != nil {
		p.logger.WithError(err).WithFields(rpc.logFields()).WithField("result", result).Debug("Partial column header validation failed")
		if result == pubsub.ValidationReject {
			// REJECT case: penalize the peer
			_ = p.peerFeedback(topicID, rpc.from, pubsub.PeerFeedbackInvalidMessage)
		}
		// Both REJECT and IGNORE: don't process further
		return nil, errInvalidHeader
	}
	return verifier, nil
}

func (p *PartialColumnBroadcaster) getHeader(groupID []byte, message *ethpb.PartialDataColumnSidecar) (*ethpb.PartialDataColumnHeader, bool) {
	if cachedHeader, ok := p.validHeaderCache[string(groupID)]; ok {
		return cachedHeader, true
	} else {
		// We haven't seen this group before. Check if we have a valid header.
		if len(message.Header) == 0 {
			p.logger.Debug("No partial column found and no header in message, ignoring")
			return nil, false
		}

		return message.Header[0], false
	}
}

func (p *PartialColumnBroadcaster) republishColumn(ourDataColumn *blocks.PartialDataColumn, rpc incomingPartialRPC,
	shouldRepublish bool) error {
	if !ourDataColumn.Published {
		p.recordRepublishSkip(rpc.GroupID, ourDataColumn.Index())
		return nil
	}

	topicId := rpc.GetTopicID()

	peerMeta := rpc.PartsMetadata
	myMeta, err := ourDataColumn.PartsMetadata()
	if err != nil {
		return errors.Wrap(err, "parts metadata")
	}
	if !shouldRepublish && len(peerMeta) > 0 && !bytes.Equal(peerMeta, myMeta) {
		// Either we have something they don't or vice versa
		shouldRepublish = true
	}

	if shouldRepublish {
		err := p.publishPartialCol(topicId, ourDataColumn.GroupID(), ourDataColumn)
		if err != nil {
			return errors.Wrap(err, "publish partial column")
		}
	}
	return nil
}

func (p *PartialColumnBroadcaster) handlePartialCells(ourDataColumn *blocks.PartialDataColumn, message *ethpb.PartialDataColumnSidecar,
	rpc incomingPartialRPC) error {
	topicId := rpc.GetTopicID()

	cellIndices, cellsToVerify, err := ourDataColumn.CellsToVerifyFromPartialMessage(message)
	if err != nil {
		return errors.Wrap(err, "cells to verify from partial message")
	}
	// Track cells received via partial message
	if len(cellIndices) > 0 {
		columnIndexStr := strconv.FormatUint(ourDataColumn.Index(), 10)
		partialMessageCellsReceivedTotal.WithLabelValues(columnIndexStr).Add(float64(len(cellIndices)))
	}
	if len(cellsToVerify) > 0 {
		select {
		case p.concurrentValidatorSemaphore <- struct{}{}:
			go func() {
				defer func() {
					<-p.concurrentValidatorSemaphore
				}()
				start := time.Now()
				err := p.callbacks.ValidateColumn(cellsToVerify)
				if err != nil {
					p.logger.WithError(err).WithFields(rpc.logFields()).Error("Failed to validate cells")
					_ = p.peerFeedback(topicId, rpc.from, pubsub.PeerFeedbackInvalidMessage)
					return
				}
				_ = p.peerFeedback(topicId, rpc.from, pubsub.PeerFeedbackUsefulMessage)
				_, _ = p.enqueue(p.ctx, requestKindCellsValidated, requestValues{
					cellsValidated: &cellsValidated{
						validationTook: time.Since(start),
						topic:          topicId,
						group:          ourDataColumn.GroupID(),
						cells:          cellsToVerify,
						cellIndices:    cellIndices,
					},
				})
			}()
		default:
			columnIndexStr := strconv.FormatUint(ourDataColumn.Index(), 10)
			partialMessageValidationsDroppedTotal.WithLabelValues(columnIndexStr).Add(float64(len(cellsToVerify)))
			p.logger.WithFields(rpc.logFields()).Warn("Validator semaphore saturated, dropping cell validation")
		}
	}
	return nil
}

func (p *PartialColumnBroadcaster) handleHeader(rpc incomingPartialRPC, header *ethpb.PartialDataColumnHeader) {
	groupID := rpc.GroupID
	// Cache the valid header.
	p.validHeaderCache[string(groupID)] = header

	select {
	case p.concurrentHeaderHandlerSemaphore <- struct{}{}:
		go func() {
			p.callbacks.HandleHeader(header, string(groupID))
			<-p.concurrentHeaderHandlerSemaphore
		}()
	default:
		p.logger.WithFields(rpc.logFields()).Warn("Dropping header handler, max concurrent header handlers reached")
	}

}

func (p *PartialColumnBroadcaster) handleCellsValidated(cells *cellsValidated) error {
	ourVerifier := p.getPartialVerifier(cells.topic, cells.group)
	if ourVerifier == nil {
		return errors.New("data column not found for verified cells")
	}
	ourDataColumn := ourVerifier.Column
	var extended bool
	for i, bundle := range cells.cells {
		if bundle.ColumnIndex != ourDataColumn.Index() {
			return errors.New("cell bundle has wrong column index")
		}
		if ourVerifier.ExtendFromVerifiedCell(cells.cellIndices[i], bundle.Cell, bundle.Proof) {
			extended = true
		}
	}

	columnIndexStr := strconv.FormatUint(ourDataColumn.Index(), 10)
	if extended {
		// Track useful cells (cells that extended our data)
		partialMessageUsefulCellsTotal.WithLabelValues(columnIndexStr).Add(float64(len(cells.cells)))

		col, ok, err := ourVerifier.Complete()
		if err != nil {
			p.logger.WithError(err).WithFields(cells.logFields()).Error("Failed to complete partial column verifier")
			return errors.Wrap(err, "complete partial column verifier")
		}
		if ok {
			go p.callbacks.HandleColumn(cells.topic, col)
		}

		if !ourDataColumn.Published {
			p.recordRepublishSkip(cells.group, ourDataColumn.Index())
			return nil
		}

		err = p.publishPartialCol(cells.topic, ourDataColumn.GroupID(), ourDataColumn)
		if err != nil {
			return errors.Wrap(err, "publish partial column")
		}
	}
	return nil
}

// Publish publishes partial columns for the given topics.
func (p *PartialColumnBroadcaster) Publish(ctx context.Context, topicsAndColumns iter.Seq2[string, blocks.PartialDataColumn]) error {
	if p.peerFeedback == nil || p.publishPartialCol == nil {
		return errors.New("pubsub not initialized")
	}
	req, err := p.enqueue(ctx, requestKindPublish, requestValues{
		publish: publish{
			topicsAndColumns: topicsAndColumns,
		},
	})
	if err != nil {
		return err
	}
	return req.waitForResponse()
}

func (p *PartialColumnBroadcaster) gossip(topic string, groupID []byte) {
	topicStore, ok := p.partialMsgStore[topic]
	if !ok {
		return
	}
	existing := topicStore[string(groupID)]
	if existing == nil {
		return
	}
	if existing.Column.Included.Count() == 0 {
		// Nothing useful here
		return
	}
	if !existing.Column.Published {
		return
	}
	err := p.publishPartialCol(topic, existing.Column.GroupID(), existing.Column)
	if err != nil {
		p.logger.WithFields(logrus.Fields{"err": err}).Warn("Failed to publish gossip")
	}
}

func (p *PartialColumnBroadcaster) publish(topicsAndColumns iter.Seq2[string, blocks.PartialDataColumn]) error {
	var aggErr error
	for topic, partialCol := range topicsAndColumns {
		if partialCol.KzgCommitmentCount() == 0 {
			p.logger.WithFields(logrus.Fields{
				"topic": topic,
			}).Debug("Skipping publish for column with no KZG commitments")
			continue
		}
		groupIDBytes := partialCol.GroupID()
		topicStore, ok := p.partialMsgStore[topic]
		if !ok {
			topicStore = make(map[string]*verification.PartialColumnVerifier)
			p.partialMsgStore[topic] = topicStore
		}
		verifier := p.getPartialVerifier(topic, groupIDBytes)
		if verifier == nil {
			var err error
			verifier, err = p.callbacks.PartialVerifierFromTrustedColumn(&partialCol)
			if err != nil {
				aggErr = stderrors.Join(aggErr, errors.Wrap(err, "partial verifier from trusted column"))
				continue
			}
			topicStore[string(groupIDBytes)] = verifier
		} else {
			if requests, ok := partialCol.PartsRequests(); ok {
				if err := verifier.Column.SetPartsRequests(requests); err != nil {
					aggErr = stderrors.Join(aggErr, errors.Wrap(err, "set parts requests"))
					continue
				}
			} else {
				verifier.Column.ClearPartsRequests()
			}
			var extended bool
			for i := range partialCol.Included.Len() {
				if partialCol.Included.BitAt(i) {
					if verifier.ExtendFromVerifiedCell(uint64(i), partialCol.Column()[i], partialCol.KzgProofs()[i]) {
						extended = true
					}
				}
			}
			if extended {
				// A column completed by this merge never reaches handleCellsValidated, so hand it to the callback here.
				col, ok, err := verifier.Complete()
				if err != nil {
					aggErr = stderrors.Join(aggErr, errors.Wrap(err, "complete partial column verifier"))
					continue
				}
				if ok {
					go p.callbacks.HandleColumn(topic, col)
				}
			}
		}
		ourColummn := verifier.Column

		p.groupTTL[string(groupIDBytes)] = TTLInSlots
		err := p.publishPartialCol(topic, ourColummn.GroupID(), ourColummn)
		if err == nil {
			ourColummn.Published = true
		} else {
			aggErr = stderrors.Join(aggErr, errors.Wrap(err, "publish partial column"))
		}
	}
	return aggErr
}

func (p *PartialColumnBroadcaster) Subscribe(ctx context.Context, t *pubsub.Topic) error {
	req, err := p.enqueue(ctx, requestKindSubscribe, requestValues{
		sub: subscribe{
			t: t,
		},
	})
	if err != nil {
		return err
	}
	return req.waitForResponse()
}

func (p *PartialColumnBroadcaster) subscribe(t *pubsub.Topic) error {
	topic := t.String()
	if _, ok := p.topics[topic]; ok {
		return errors.New("already subscribed")
	}

	p.topics[topic] = t
	return nil
}

func (p *PartialColumnBroadcaster) Unsubscribe(ctx context.Context, topic string) error {
	req, err := p.enqueue(ctx, requestKindUnsubscribe, requestValues{
		unsub: unsubscribe{
			topic: topic,
		},
	})
	if err != nil {
		return err
	}
	return req.waitForResponse()
}
func (p *PartialColumnBroadcaster) unsubscribe(topic string) error {
	t, ok := p.topics[topic]
	if !ok {
		return errors.New("topic not found")
	}
	delete(p.topics, topic)
	delete(p.partialMsgStore, topic)
	return t.Close()
}
