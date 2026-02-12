package partialdatacolumnbroadcaster

import (
	"bytes"
	"log/slog"
	"regexp"
	"strconv"
	"time"

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

// TODOs:
// different eager push strategies:
// - no eager push
// - full column eager push
//   - With debouncing - some factor of RTT
// - eager push missing cells

const TTLInSlots = 3
const maxConcurrentValidators = 128
const headerHandledTimeout = time.Second * 1
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

type PartialColumnBroadcaster struct {
	logger *logrus.Logger

	ps   *pubsub.PubSub
	stop chan struct{}

	validateHeader HeaderValidator
	validateColumn ColumnValidator
	handleColumn   SubHandler
	handleHeader   HeaderHandler

	// map groupID -> bool to signal when header has been handled
	headerHandled map[string]bool

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
	requestKindHandleIncomingRPC
	requestKindCellsValidated
	requestKindHeaderHandled
)

type request struct {
	kind               requestKind
	response           chan error
	sub                subscribe
	unsub              unsubscribe
	publish            publish
	incomingRPC        rpcWithFrom
	cellsValidated     *cellsValidated
	headerHandledGroup string
}

type publish struct {
	topic string
	c     blocks.PartialDataColumn
}

type subscribe struct {
	t *pubsub.Topic
}

type unsubscribe struct {
	topic string
}

type rpcWithFrom struct {
	*pubsub_pb.PartialMessagesExtension
	from peer.ID
}

type cellsValidated struct {
	validationTook time.Duration
	topic          string
	group          []byte
	cellIndices    []uint64
	cells          []blocks.CellProofBundle
}

func NewBroadcaster(logger *logrus.Logger) *PartialColumnBroadcaster {
	return &PartialColumnBroadcaster{
		topics:           make(map[string]*pubsub.Topic),
		partialMsgStore:  make(map[string]map[string]*blocks.PartialDataColumn),
		groupTTL:         make(map[string]int8),
		validHeaderCache: make(map[string]*ethpb.PartialDataColumnHeader),
		headerHandled:    make(map[string]bool),
		// GossipSub sends the messages to this channel. The buffer should be
		// big enough to avoid dropping messages. We don't want to block the gossipsub event loop for this.
		incomingReq: make(chan request, 128*16),
		logger:      logger,

		concurrentValidatorSemaphore:     make(chan struct{}, maxConcurrentValidators),
		concurrentHeaderHandlerSemaphore: make(chan struct{}, maxConcurrentHeaderHandlers),
	}
}

// AppendPubSubOpts adds the necessary pubsub options to enable partial messages.
func (p *PartialColumnBroadcaster) AppendPubSubOpts(opts []pubsub.Option) []pubsub.Option {
	slogger := slog.New(logrusadapter.Handler{Logger: p.logger})
	opts = append(opts,
		pubsub.WithPartialMessagesExtension(&partialmessages.PartialMessagesExtension{
			Logger: slogger,
			MergePartsMetadata: func(topic string, left, right partialmessages.PartsMetadata) partialmessages.PartsMetadata {
				merged, err := blocks.MergePartsMetadata(left, right)
				if err != nil {
					p.logger.Warn("Failed to merge parts metadata", "err", err)
					return left
				}
				return merged
			},
			ValidateRPC: func(from peer.ID, rpc *pubsub_pb.PartialMessagesExtension) error {
				// TODO. Add some basic and fast sanity checks
				return nil
			},
			OnIncomingRPC: func(from peer.ID, rpc *pubsub_pb.PartialMessagesExtension) error {
				select {
				case p.incomingReq <- request{
					kind:        requestKindHandleIncomingRPC,
					incomingRPC: rpcWithFrom{rpc, from},
				}:
				default:
					p.logger.Warn("Dropping incoming partial RPC", "rpc", rpc)
				}
				return nil
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
				delete(p.headerHandled, groupID)
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
				req.response <- p.publish(req.publish.topic, req.publish.c)
			case requestKindSubscribe:
				req.response <- p.subscribe(req.sub.t)
			case requestKindUnsubscribe:
				req.response <- p.unsubscribe(req.unsub.topic)
			case requestKindHandleIncomingRPC:
				err := p.handleIncomingRPC(req.incomingRPC)
				if err != nil {
					p.logger.Error("Failed to handle incoming partial RPC", "err", err)
				}
			case requestKindCellsValidated:
				err := p.handleCellsValidated(req.cellsValidated)
				if err != nil {
					p.logger.Error("Failed to handle cells validated", "err", err)
				}
			case requestKindHeaderHandled:
				p.handleHeaderHandled(req.headerHandledGroup)
			default:
				p.logger.Error("Unknown request kind", "kind", req.kind)
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

func (p *PartialColumnBroadcaster) handleIncomingRPC(rpcWithFrom rpcWithFrom) error {
	if p.ps == nil {
		return errors.New("pubsub not initialized")
	}

	hasMessage := len(rpcWithFrom.PartialMessage) > 0

	var message ethpb.PartialDataColumnSidecar
	if hasMessage {
		err := message.UnmarshalSSZ(rpcWithFrom.PartialMessage)
		if err != nil {
			return errors.Wrap(err, "failed to unmarshal partial message data")
		}
	}

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
				p.logger.Debug("No partial column found and no header in message, ignoring")
				return nil
			}

			header = message.Header[0]
			reject, err := p.validateHeader(header)
			if err != nil {
				p.logger.Debug("Header validation failed", "err", err, "reject", reject)
				if reject {
					// REJECT case: penalize the peer
					_ = p.ps.PeerFeedback(topicID, rpcWithFrom.from, pubsub.PeerFeedbackInvalidMessage)
				}
				// Both REJECT and IGNORE: don't process further
				return nil
			}
			// Cache the valid header
			p.validHeaderCache[string(groupID)] = header
			p.headerHandled[string(groupID)] = false

			p.concurrentHeaderHandlerSemaphore <- struct{}{}
			go func() {
				defer func() {
					<-p.concurrentHeaderHandlerSemaphore
				}()
				p.handleHeader(header, string(groupID))
			}()
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
			p.logger.WithError(err).WithFields(logrus.Fields{
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

	logger := p.logger.WithFields(logrus.Fields{
		"from":  rpcWithFrom.from,
		"topic": topicID,
		"group": groupID,
	})

	if len(rpcWithFrom.PartialMessage) > 0 {
		// TODO: is there any penalty we want to consider for giving us data we didn't request?
		// Note that we need to be careful around race conditions and eager data.
		// Also note that protobufs by design allow extra data that we don't parse.
		// Marco's thoughts. No, we don't need to do anything else here.
		cellIndices, cellsToVerify, err := ourDataColumn.CellsToVerifyFromPartialMessage(&message)
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
					logger.Error("failed to validate cells", "err", err)
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

	if !shouldRepublish && len(rpcWithFrom.PartsMetadata) > 0 {
		peerMeta, err := blocks.ParsePartsMetadata(rpcWithFrom.PartsMetadata)
		ourMeta, err2 := blocks.ParsePartsMetadata(ourDataColumn.PartsMetadata())
		if err == nil && err2 == nil && !bytes.Equal(peerMeta.Available, ourMeta.Available) {
			// Either we have something they don't or vice versa
			shouldRepublish = true
			logger.Debug("republishing due to parts metadata difference")
		}
	}

	headerHandled, ok := p.headerHandled[string(groupID)]
	// we only want to skip republishing if the header is currently being handled but hasn't been handled yet.
	// If the header is NOT being handled at all, these incoming cells are likely in response to a previous publish we sent
	// (either when got a data column sidecar, a beacon block body or if we are a block proposer).
	if ok && !headerHandled {
		p.logger.Debug("Header not handled, skipping republish", "topic", topicID, "group", groupID)
		return nil
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
	extended := ourDataColumn.ExtendFromVerfifiedCells(cells.cellIndices, cells.cells)
	p.logger.Debug("Extended partial message", "duration", cells.validationTook, "extended", extended)

	columnIndexStr := strconv.FormatUint(ourDataColumn.Index, 10)
	if extended {
		// Track useful cells (cells that extended our data)
		partialMessageUsefulCellsTotal.WithLabelValues(columnIndexStr).Add(float64(len(cells.cells)))

		// TODO: we could use the heuristic here that if this data was
		// useful to us, it's likely useful to our peers and we should
		// republish eagerly

		if col, ok := ourDataColumn.Complete(p.logger); ok {
			p.logger.Info("Completed partial column", "topic", cells.topic, "group", cells.group)
			if p.handleColumn != nil {
				go p.handleColumn(cells.topic, col)
			}
		} else {
			p.logger.Info("Extended partial column", "topic", cells.topic, "group", cells.group)
		}

		headerHandled, ok := p.headerHandled[string(ourDataColumn.GroupID())]
		if ok && !headerHandled {
			p.logger.Debug("Header not handled, skipping republish", "topic", cells.topic, "group", cells.group)
			return nil
		}

		err := p.ps.PublishPartialMessage(cells.topic, ourDataColumn, partialmessages.PublishOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *PartialColumnBroadcaster) handleHeaderHandled(groupID string) {
	p.headerHandled[groupID] = true
	for topic, topicStore := range p.partialMsgStore {
		col, ok := topicStore[groupID]
		if !ok {
			continue
		}
		err := p.ps.PublishPartialMessage(topic, col, partialmessages.PublishOptions{})
		if err != nil {
			p.logger.WithError(err).Error("Failed to republish after header handled", "topic", topic, "groupID", groupID)
		}
	}
}

func (p *PartialColumnBroadcaster) Stop() {
	if p.stop != nil {
		close(p.stop)
	}
}

// HeaderHandled notifies the event loop that a header has been fully processed,
// triggering republishing of all columns in the store for the given groupID.
func (p *PartialColumnBroadcaster) HeaderHandled(groupID string) error {
	if p.ps == nil {
		return errors.New("pubsub not initialized")
	}
	p.incomingReq <- request{
		kind:               requestKindHeaderHandled,
		headerHandledGroup: groupID,
	}
	return nil
}

// Publish publishes the partial column.
func (p *PartialColumnBroadcaster) Publish(topic string, c blocks.PartialDataColumn) error {
	if p.ps == nil {
		return errors.New("pubsub not initialized")
	}
	respCh := make(chan error)
	p.incomingReq <- request{
		kind:     requestKindPublish,
		response: respCh,
		publish: publish{
			topic: topic,
			c:     c,
		},
	}
	return <-respCh
}

func (p *PartialColumnBroadcaster) publish(topic string, c blocks.PartialDataColumn) error {
	topicStore, ok := p.partialMsgStore[topic]
	if !ok {
		topicStore = make(map[string]*blocks.PartialDataColumn)
		p.partialMsgStore[topic] = topicStore
	}

	var extended bool
	existing := topicStore[string(c.GroupID())]
	if existing != nil {
		// Extend the existing column with cells being published here.
		// The existing column may already contain cells received from peers. We must not overwrite it.
		for i := range c.Included.Len() {
			if c.Included.BitAt(i) {
				extended = existing.ExtendFromVerfifiedCell(uint64(i), c.Column[i], c.KzgProofs[i])
			}
		}
		if extended {
			if col, ok := existing.Complete(p.logger); ok {
				p.logger.Info("Completed partial column", "topic", topic, "group", existing.GroupID())
				if p.handleColumn != nil {
					go p.handleColumn(topic, col)
				}
			}
		}
	} else {
		topicStore[string(c.GroupID())] = &c
		existing = &c
	}

	p.groupTTL[string(c.GroupID())] = TTLInSlots

	return p.ps.PublishPartialMessage(topic, existing, partialmessages.PublishOptions{})
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
