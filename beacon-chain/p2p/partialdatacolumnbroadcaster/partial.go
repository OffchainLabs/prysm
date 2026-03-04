package partialdatacolumnbroadcaster

import (
	"bytes"
	"log/slog"
	"regexp"
	"strconv"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p-pubsub/partialmessages"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// TODOs:
// different eager push strategies:
// - no eager push
// - full column eager push
//   - With debouncing - some factor of RTT
// - eager push missing cells

const TTLInSlots = 3
const maxConcurrentValidators = 128
const maxConcurrentHeaderHandlers = 128

var dataColumnTopicRegex = regexp.MustCompile(`data_column_sidecar_(\d+)`)

func extractColumnIndexFromTopic(topic string) (uint64, error) {
	matches := dataColumnTopicRegex.FindStringSubmatch(topic)
	if len(matches) < 2 {
		return 0, errors.New("could not extract column index from topic")
	}
	return strconv.ParseUint(matches[1], 10, 64)
}

// HeaderValidator validates a PartialDataColumnHeader.
// Returns (reject, err) where:
//   - reject=true, err!=nil: REJECT - peer should be penalized
//   - reject=false, err!=nil: IGNORE - don't penalize, just ignore
//   - reject=false, err=nil: valid header
type HeaderValidator func(header *ethpb.PartialDataColumnHeader) (reject bool, err error)
type HeaderHandler func(header *ethpb.PartialDataColumnHeader, groupID string)
type ColumnValidator func(cells []blocks.CellProofBundle) error
type partialColumnPubSub interface {
	PeerFeedback(topic string, peer peer.ID, kind pubsub.PeerFeedbackKind) error
	PublishPartialMessage(topic string, partialMessage partialmessages.Message, opts partialmessages.PublishOptions) error
}

type PartialColumnBroadcaster struct {
	ps   partialColumnPubSub
	stop chan struct{}

	validateHeader HeaderValidator
	validateColumn ColumnValidator
	handleColumn   SubHandler
	handleHeader   HeaderHandler

	// map groupID -> bool to signal when getBlobs has been called
	getBlobsCalled map[string]bool

	// map topic -> *pubsub.Topic
	topics map[string]*pubsub.Topic

	concurrentValidatorSemaphore     chan struct{}
	concurrentHeaderHandlerSemaphore chan struct{}

	// map topic -> map[groupID]PartialColumn
	partialMsgStore map[string]map[string]*blocks.PartialDataColumn

	groupTTL map[string]int8

	// validHeaderCache caches validated headers by group ID (works across topics)
	validHeaderCache map[string]*ethpb.PartialDataColumnHeader

	incomingReq chan request
}

type requestKind uint8

const (
	requestKindPublish requestKind = iota
	requestKindSubscribe
	requestKindUnsubscribe
	requestKindGossipForPeer
	requestKindHandleIncomingRPC
	requestKindCellsValidated
)

type request struct {
	kind           requestKind
	cellsValidated *cellsValidated
	response       chan error
	unsub          unsubscribe
	incomingRPC    rpcWithFrom
	sub            subscribe
	publish        publish
	gossipForPeer  gossipForPeer
}

type publish struct {
	topic          string
	c              blocks.PartialDataColumn
	getBlobsCalled bool
	publishRespCh  chan publishResponse
}

type publishResponse struct {
	err             error
	columnCompleted bool
}

type subscribe struct {
	t *pubsub.Topic
}

type unsubscribe struct {
	topic string
}

type rpcWithFrom struct {
	*pubsub_pb.PartialMessagesExtension
	from    peer.ID
	message *ethpb.PartialDataColumnSidecar
}

type cellsValidated struct {
	validationTook time.Duration
	topic          string
	group          []byte
	cellIndices    []uint64
	cells          []blocks.CellProofBundle
}

type gossipForPeer struct {
	topic             string
	groupID           string
	remote            peer.ID
	peerState         partialmessages.PeerState
	gossipForPeerResp chan gossipForPeerResponse
}

type gossipForPeerResponse struct {
	nextPeerState       partialmessages.PeerState
	encodedMsg          []byte
	partsMetadataToSend partialmessages.PartsMetadata
	err                 error
}

func NewBroadcaster() *PartialColumnBroadcaster {
	return &PartialColumnBroadcaster{
		topics:           make(map[string]*pubsub.Topic),
		partialMsgStore:  make(map[string]map[string]*blocks.PartialDataColumn),
		groupTTL:         make(map[string]int8),
		validHeaderCache: make(map[string]*ethpb.PartialDataColumnHeader),
		getBlobsCalled:   make(map[string]bool),
		// GossipSub sends the messages to this channel. The buffer should be
		// big enough to avoid dropping messages. We don't want to block the gossipsub event loop for this.
		incomingReq: make(chan request, 128*16),

		concurrentValidatorSemaphore:     make(chan struct{}, maxConcurrentValidators),
		concurrentHeaderHandlerSemaphore: make(chan struct{}, maxConcurrentHeaderHandlers),
	}
}

// AppendPubSubOpts adds the necessary pubsub options to enable partial messages.
func (p *PartialColumnBroadcaster) AppendPubSubOpts(opts []pubsub.Option) []pubsub.Option {
	opts = append(opts,
		pubsub.WithPartialMessagesExtension(&partialmessages.PartialMessagesExtension{
			Logger: slog.Default().With("package", "beacon-chain/p2p/partialdatacolumnbroadcaster"),
			GossipForPeer: func(topic string, groupID string, remote peer.ID, peerState partialmessages.PeerState) (partialmessages.PeerState, []byte, partialmessages.PartsMetadata, error) {
				respCh := make(chan gossipForPeerResponse, 1)
				p.incomingReq <- request{
					kind: requestKindGossipForPeer,
					gossipForPeer: gossipForPeer{
						topic:             topic,
						groupID:           groupID,
						remote:            remote,
						peerState:         peerState,
						gossipForPeerResp: respCh,
					},
				}
				select {
				case resp := <-respCh:
					return resp.nextPeerState, resp.encodedMsg, resp.partsMetadataToSend, resp.err
				case <-p.stop:
					return peerState, nil, nil, errors.New("broadcaster stopped")
				}
			},
			OnIncomingRPC: func(from peer.ID, peerState partialmessages.PeerState, rpc *pubsub_pb.PartialMessagesExtension) (partialmessages.PeerState, error) {
				if rpc == nil {
					return peerState, errors.New("rpc is nil")
				}
				nextPeerState, message, err := updatePeerStateFromIncomingRPC(peerState, rpc)
				if err != nil {
					return peerState, err
				}
				select {
				case p.incomingReq <- request{
					kind:        requestKindHandleIncomingRPC,
					incomingRPC: rpcWithFrom{rpc, from, message},
				}:
				default:
					log.WithField("rpc", rpc).Warn("Dropping incoming partial RPC")
					return nextPeerState, errors.New("incomingReq channel is full, dropping RPC")
				}
				return nextPeerState, nil
			},
		}),
		func(ps *pubsub.PubSub) error {
			p.ps = ps
			return nil
		},
	)
	return opts
}

// Start starts the event loop of the PartialColumnBroadcaster.
// It accepts the required validator and handler functions, returning an error if any is nil.
// The event loop is launched in a goroutine.
func (p *PartialColumnBroadcaster) Start(
	validateHeader HeaderValidator,
	validateColumn ColumnValidator,
	handleColumn SubHandler,
	handleHeader HeaderHandler,
) error {
	if validateHeader == nil {
		return errors.New("no header validator provided")
	}
	if handleHeader == nil {
		return errors.New("no header handler provided")
	}
	if validateColumn == nil {
		return errors.New("no column validator provided")
	}
	if handleColumn == nil {
		return errors.New("no column handler provided")
	}
	p.validateHeader = validateHeader
	p.validateColumn = validateColumn
	p.handleColumn = handleColumn
	p.handleHeader = handleHeader
	p.stop = make(chan struct{})
	go p.loop()
	return nil
}

func (p *PartialColumnBroadcaster) loop() {
	cleanup := time.NewTicker(time.Second * time.Duration(params.BeaconConfig().SecondsPerSlot))
	defer cleanup.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-cleanup.C:
			for groupID, ttl := range p.groupTTL {
				if ttl > 0 {
					p.groupTTL[groupID] = ttl - 1
					continue
				}

				delete(p.groupTTL, groupID)
				delete(p.validHeaderCache, groupID)
				delete(p.getBlobsCalled, groupID)
				for topic, msgStore := range p.partialMsgStore {
					delete(msgStore, groupID)
					if len(msgStore) == 0 {
						delete(p.partialMsgStore, topic)
					}
				}
			}
		case req := <-p.incomingReq:
			switch req.kind {
			case requestKindPublish:
				var pr publishResponse
				pr.columnCompleted, pr.err = p.publish(req.publish.topic, req.publish.c, req.publish.getBlobsCalled)
				req.publish.publishRespCh <- pr
			case requestKindSubscribe:
				req.response <- p.subscribe(req.sub.t)
			case requestKindUnsubscribe:
				req.response <- p.unsubscribe(req.unsub.topic)
			case requestKindGossipForPeer:
				nextPeerState, encodedMsg, partsMetadataToSend, err := p.handleGossipForPeer(req.gossipForPeer)
				req.gossipForPeer.gossipForPeerResp <- gossipForPeerResponse{
					nextPeerState:       nextPeerState,
					encodedMsg:          encodedMsg,
					partsMetadataToSend: partsMetadataToSend,
					err:                 err,
				}
			case requestKindHandleIncomingRPC:
				err := p.handleIncomingRPC(req.incomingRPC)
				if err != nil {
					log.WithError(err).Error("Failed to handle incoming partial RPC")
				}
			case requestKindCellsValidated:
				err := p.handleCellsValidated(req.cellsValidated)
				if err != nil {
					log.WithError(err).Error("Failed to handle cells validated")
				}
			default:
				log.WithField("kind", req.kind).Error("Unknown request kind")
			}
		}
	}
}

func (p *PartialColumnBroadcaster) getDataColumn(topic string, group []byte) *blocks.PartialDataColumn {
	topicStore, ok := p.partialMsgStore[topic]
	if !ok {
		return nil
	}
	msg, ok := topicStore[string(group)]
	if !ok {
		return nil
	}
	return msg
}

func (p *PartialColumnBroadcaster) handleGossipForPeer(req gossipForPeer) (partialmessages.PeerState, []byte, partialmessages.PartsMetadata, error) {
	topicStore, ok := p.partialMsgStore[req.topic]
	if !ok {
		return req.peerState, nil, nil, errors.New("not tracking topic for group")
	}
	partialColumn, ok := topicStore[req.groupID]
	if !ok || partialColumn == nil {
		return req.peerState, nil, nil, errors.New("not tracking topic for group")
	}
	// we're not requesting a message here as this will be used to emit gossip. So, we pass requested message as false.
	return partialColumn.ForPeer(req.remote, false, req.peerState)
}

func parsePartsMetadataFromPeerState(state any, expectedLength uint64) (*ethpb.PartialDataColumnPartsMetadata, error) {
	if state == nil {
		return blocks.NewPartsMetaWithNoAvailableAndNoRequests(expectedLength), nil
	}
	meta, ok := state.(*ethpb.PartialDataColumnPartsMetadata)
	if !ok {
		return nil, errors.New("state is not *PartialDataColumnPartsMetadata")
	}
	return meta, nil
}

func updatePeerStateFromIncomingRPC(peerState partialmessages.PeerState, rpc *pubsub_pb.PartialMessagesExtension) (partialmessages.PeerState,
	*ethpb.PartialDataColumnSidecar, error) {
	peerState = blocks.ClonePeerState(peerState)
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

		if peerState.RecvdState == nil {
			peerState.RecvdState = &incomingMeta
		} else {
			existingMeta, ok := peerState.RecvdState.(*ethpb.PartialDataColumnPartsMetadata)
			if !ok {
				return peerState, nil, errors.New("recvdState is not *PartialDataColumnPartsMetadata")
			}
			existingMeta.Requests = incomingMeta.Requests
			var err error
			peerState.RecvdState, err = blocks.MergeAvailableIntoPartsMetadata(existingMeta, incomingMeta.Available)
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

	var message ethpb.PartialDataColumnSidecar
	if err := message.UnmarshalSSZ(rpc.PartialMessage); err != nil {
		return peerState, nil, errors.Wrap(err, "failed to unmarshal partial message data")
	}
	if len(message.CellsPresentBitmap) == 0 {
		return peerState, &message, nil
	}

	nKzgCommitments := message.CellsPresentBitmap.Len()
	if nKzgCommitments == 0 {
		return peerState, nil, errors.New("length of cells present bitmap is 0")
	}

	// only update RecvdState using the incoming partial message if the peer did not send us their parts metadata
	if !hasIncomingPartsMetadata {
		recievedMeta, err := parsePartsMetadataFromPeerState(peerState.RecvdState, nKzgCommitments)
		if err != nil {
			return peerState, nil, errors.Wrap(err, "received")
		}
		recvdState, err := blocks.MergeAvailableIntoPartsMetadata(recievedMeta, message.CellsPresentBitmap)
		if err != nil {
			return peerState, nil, err
		}
		peerState.RecvdState = recvdState
	}

	sentMeta, err := parsePartsMetadataFromPeerState(peerState.SentState, nKzgCommitments)
	if err != nil {
		return peerState, nil, errors.Wrap(err, "sent")
	}

	sentState, err := blocks.MergeAvailableIntoPartsMetadata(sentMeta, message.CellsPresentBitmap)
	if err != nil {
		return peerState, nil, err
	}
	peerState.SentState = sentState

	return peerState, &message, nil
}

func (p *PartialColumnBroadcaster) handleIncomingRPC(rpcWithFrom rpcWithFrom) error {
	if p.ps == nil {
		return errors.New("pubsub not initialized")
	}

	message := rpcWithFrom.message
	hasMessage := message != nil

	topicID := rpcWithFrom.GetTopicID()
	groupID := rpcWithFrom.GroupID
	ourDataColumn := p.getDataColumn(topicID, groupID)
	var shouldRepublish bool

	if ourDataColumn == nil && hasMessage {
		var header *ethpb.PartialDataColumnHeader
		// Check cache first for this group
		if cachedHeader, ok := p.validHeaderCache[string(groupID)]; ok {
			header = cachedHeader
		} else {
			// We haven't seen this group before. Check if we have a valid header.
			if len(message.Header) == 0 {
				log.Debug("No partial column found and no header in message, ignoring")
				return nil
			}

			header = message.Header[0]
			reject, err := p.validateHeader(header)
			if err != nil {
				log.WithError(err).WithField("reject", reject).Debug("Header validation failed")
				if reject {
					// REJECT case: penalize the peer
					_ = p.ps.PeerFeedback(topicID, rpcWithFrom.from, pubsub.PeerFeedbackInvalidMessage)
				}
				// Both REJECT and IGNORE: don't process further
				return nil
			}
			// Cache the valid header
			p.validHeaderCache[string(groupID)] = header

			select {
			case p.concurrentHeaderHandlerSemaphore <- struct{}{}:
				go func() {
					defer func() {
						<-p.concurrentHeaderHandlerSemaphore
					}()
					p.handleHeader(header, string(groupID))
				}()
			default:
				log.WithFields(logrus.Fields{
					"topic": topicID,
					"group": groupID,
				}).Warn("Dropping header handler, max concurrent header handlers reached")
			}
		}

		columnIndex, err := extractColumnIndexFromTopic(topicID)
		if err != nil {
			return err
		}

		newColumn, err := blocks.NewPartialDataColumn(
			header.SignedBlockHeader,
			columnIndex,
			header.KzgCommitments,
			header.KzgCommitmentsInclusionProof,
		)
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"topic":          topicID,
				"columnIndex":    columnIndex,
				"numCommitments": len(header.KzgCommitments),
			}).Error("Failed to create partial data column from header")
			return err
		}

		// Save to store
		topicStore, ok := p.partialMsgStore[topicID]
		if !ok {
			topicStore = make(map[string]*blocks.PartialDataColumn)
			p.partialMsgStore[topicID] = topicStore
		}
		topicStore[string(newColumn.GroupID())] = &newColumn
		p.groupTTL[string(newColumn.GroupID())] = TTLInSlots

		ourDataColumn = &newColumn
		shouldRepublish = true
	}

	if ourDataColumn == nil {
		// We don't have a partial column for this. Can happen if we got cells
		// without a header.
		return nil
	}

	logger := log.WithFields(logrus.Fields{
		"from":  rpcWithFrom.from,
		"topic": topicID,
		"group": groupID,
	})

	if hasMessage {
		// TODO: is there any penalty we want to consider for giving us data we didn't request?
		// Note that we need to be careful around race conditions and eager data.
		// Also note that protobufs by design allow extra data that we don't parse.
		// Marco's thoughts. No, we don't need to do anything else here.
		cellIndices, cellsToVerify, err := ourDataColumn.CellsToVerifyFromPartialMessage(message)
		if err != nil {
			return err
		}
		// Track cells received via partial message
		if len(cellIndices) > 0 {
			columnIndexStr := strconv.FormatUint(ourDataColumn.Index, 10)
			partialMessageCellsReceivedTotal.WithLabelValues(columnIndexStr).Add(float64(len(cellIndices)))
		}
		if len(cellsToVerify) > 0 {
			p.concurrentValidatorSemaphore <- struct{}{}
			go func() {
				defer func() {
					<-p.concurrentValidatorSemaphore
				}()
				start := time.Now()
				err := p.validateColumn(cellsToVerify)
				if err != nil {
					logger.WithError(err).Error("Failed to validate cells")
					_ = p.ps.PeerFeedback(topicID, rpcWithFrom.from, pubsub.PeerFeedbackInvalidMessage)
					return
				}
				_ = p.ps.PeerFeedback(topicID, rpcWithFrom.from, pubsub.PeerFeedbackUsefulMessage)
				p.incomingReq <- request{
					kind: requestKindCellsValidated,
					cellsValidated: &cellsValidated{
						validationTook: time.Since(start),
						topic:          topicID,
						group:          groupID,
						cells:          cellsToVerify,
						cellIndices:    cellIndices,
					},
				}
			}()
		}
	}

	getBlobsCalled := p.getBlobsCalled[string(groupID)]
	if !getBlobsCalled {
		log.WithFields(logrus.Fields{"topic": topicID, "group": groupID}).Debug("GetBlobs not called, skipping republish")
		return nil
	}

	peerMeta := rpcWithFrom.PartsMetadata
	myMeta, err := ourDataColumn.PartsMetadata()
	if err != nil {
		return err
	}
	if !shouldRepublish && len(peerMeta) > 0 && !bytes.Equal(peerMeta, myMeta) {
		// Either we have something they don't or vice versa
		shouldRepublish = true
		logger.Debug("republishing due to parts metadata difference")
	}

	if shouldRepublish {
		err := p.ps.PublishPartialMessage(topicID, ourDataColumn, partialmessages.PublishOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *PartialColumnBroadcaster) handleCellsValidated(cells *cellsValidated) error {
	ourDataColumn := p.getDataColumn(cells.topic, cells.group)
	if ourDataColumn == nil {
		return errors.New("data column not found for verified cells")
	}
	extended := ourDataColumn.ExtendFromVerifiedCells(cells.cellIndices, cells.cells)
	log.WithFields(logrus.Fields{"duration": cells.validationTook, "extended": extended}).Debug("Extended partial message")

	columnIndexStr := strconv.FormatUint(ourDataColumn.Index, 10)
	if extended {
		// Track useful cells (cells that extended our data)
		partialMessageUsefulCellsTotal.WithLabelValues(columnIndexStr).Add(float64(len(cells.cells)))

		// TODO: we could use the heuristic here that if this data was
		// useful to us, it's likely useful to our peers and we should
		// republish eagerly

		if col, ok := ourDataColumn.Complete(); ok {
			log.WithFields(logrus.Fields{"topic": cells.topic, "group": cells.group}).Info("Completed partial column")
			if p.handleColumn != nil {
				go p.handleColumn(cells.topic, col)
			}
		} else {
			log.WithFields(logrus.Fields{"topic": cells.topic, "group": cells.group}).Info("Extended partial column")
		}

		getBlobsCalled := p.getBlobsCalled[string(ourDataColumn.GroupID())]
		if !getBlobsCalled {
			log.WithFields(logrus.Fields{"topic": cells.topic, "group": cells.group}).Debug("GetBlobs not called, skipping republish")
			return nil
		}

		err := p.ps.PublishPartialMessage(cells.topic, ourDataColumn, partialmessages.PublishOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *PartialColumnBroadcaster) Stop() {
	if p.stop != nil {
		close(p.stop)
	}
}

// Publish publishes the partial column.
func (p *PartialColumnBroadcaster) Publish(topic string, c blocks.PartialDataColumn, getBlobsCalled bool) (bool, error) {
	if p.ps == nil {
		return false, errors.New("pubsub not initialized")
	}
	respCh := make(chan publishResponse, 1)
	p.incomingReq <- request{
		kind: requestKindPublish,
		publish: publish{
			topic:          topic,
			c:              c,
			getBlobsCalled: getBlobsCalled,
			publishRespCh:  respCh,
		},
	}
	resp := <-respCh
	return resp.columnCompleted, resp.err
}

func (p *PartialColumnBroadcaster) publish(topic string, c blocks.PartialDataColumn, getBlobsCalled bool) (bool, error) {
	var columnCompleted bool
	groupIDBytes := c.GroupID()
	topicStore, ok := p.partialMsgStore[topic]
	if !ok {
		topicStore = make(map[string]*blocks.PartialDataColumn)
		p.partialMsgStore[topic] = topicStore
	}

	existing := topicStore[string(groupIDBytes)]
	if existing != nil {
		var extended bool
		// Extend the existing column with cells being published here.
		// The existing column may already contain cells received from peers. We must not overwrite it.
		for i := range c.Included.Len() {
			if c.Included.BitAt(i) {
				if existing.ExtendFromVerifiedCell(uint64(i), c.Column[i], c.KzgProofs[i]) {
					extended = true
				}
			}
		}
		if extended {
			if col, ok := existing.Complete(); ok {
				log.WithFields(logrus.Fields{"topic": topic, "group": existing.GroupID()}).Info("Completed partial column")
				if p.handleColumn != nil {
					columnCompleted = true
					go p.handleColumn(topic, col)
				}
			}
		}
	} else {
		topicStore[string(groupIDBytes)] = &c
		existing = &c
	}

	p.groupTTL[string(groupIDBytes)] = TTLInSlots

	err := p.ps.PublishPartialMessage(topic, existing, partialmessages.PublishOptions{})
	if err == nil {
		p.getBlobsCalled[string(groupIDBytes)] = getBlobsCalled
	}
	return columnCompleted, err
}

type SubHandler func(topic string, col blocks.VerifiedRODataColumn)

func (p *PartialColumnBroadcaster) Subscribe(t *pubsub.Topic) error {
	respCh := make(chan error)
	p.incomingReq <- request{
		kind: requestKindSubscribe,
		sub: subscribe{
			t: t,
		},
		response: respCh,
	}
	return <-respCh
}
func (p *PartialColumnBroadcaster) subscribe(t *pubsub.Topic) error {
	topic := t.String()
	if _, ok := p.topics[topic]; ok {
		return errors.New("already subscribed")
	}

	p.topics[topic] = t
	return nil
}

func (p *PartialColumnBroadcaster) Unsubscribe(topic string) error {
	respCh := make(chan error)
	p.incomingReq <- request{
		kind: requestKindUnsubscribe,
		unsub: unsubscribe{
			topic: topic,
		},
		response: respCh,
	}
	return <-respCh
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
