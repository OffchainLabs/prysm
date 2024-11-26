package sync

import (
	"context"
	"fmt"
	"strings"

	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	p2ptypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/types"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/time"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

// pingHandler reads the incoming ping rpc message from the peer.
// If the peer's sequence number is higher than the one stored locally,
// a METADATA request is sent to the peer to retrieve and update the latest metadata.
// Note: This function is misnamed, as it performs more than just reading a ping message.
func (s *Service) pingHandler(_ context.Context, msg interface{}, stream libp2pcore.Stream) error {
	SetRPCStreamDeadlines(stream)

	// Convert the message to SSW Uint64 type.
	m, ok := msg.(*primitives.SSZUint64)
	if !ok {
		return fmt.Errorf("wrong message type for ping, got %T, wanted *uint64", msg)
	}

	// Validate the incoming request regarding rate limiting.
	if err := s.rateLimiter.validateRequest(stream, 1); err != nil {
		return errors.Wrap(err, "validate request")
	}

	s.rateLimiter.add(stream, 1)

	// Retrieve the peer ID.
	peerID := stream.Conn().RemotePeer()

	// Check if the peer sequence number is higher than the one we have in our store.
	valid, err := s.validateSequenceNum(*m, peerID)
	if err != nil {
		// Descore peer for giving us a bad sequence number.
		if errors.Is(err, p2ptypes.ErrInvalidSequenceNum) {
			s.cfg.p2p.Peers().Scorers().BadResponsesScorer().Increment(peerID)
			s.writeErrorResponseToStream(responseCodeInvalidRequest, p2ptypes.ErrInvalidSequenceNum.Error(), stream)
		}

		return errors.Wrap(err, "validate sequence number")
	}

	// We can already prepare a success response to the peer.
	if _, err := stream.Write([]byte{responseCodeSuccess}); err != nil {
		return errors.Wrap(err, "write response")
	}

	// Retrieve our own sequence number.
	seqNumber := s.cfg.p2p.MetadataSeq()

	// SSZ encode our sequence number.
	seqNumberSSZ := primitives.SSZUint64(seqNumber)

	// Send our sequence number back to the peer.
	if _, err := s.cfg.p2p.Encoding().EncodeWithMaxLength(stream, &seqNumberSSZ); err != nil {
		return err
	}

	closeStream(stream, log)

	if valid {
		// If the peer's sequence numberwas valid we're done.
		return nil
	}

	// The peer's sequence number was not valid. We ask the peer for its metadata.
	go func() {
		// Define a new context so the calling function doesn't cancel on us.
		ctx, cancel := context.WithTimeout(context.Background(), ttfbTimeout)
		defer cancel()

		// Send a METADATA request to the peer.
		peerMetadata, err := s.sendMetaDataRequest(ctx, peerID)
		if err != nil {
			// We cannot compare errors directly as the stream muxer error
			// type isn't compatible with the error we have, so a direct
			// equality checks fails.
			if !strings.Contains(err.Error(), p2ptypes.ErrIODeadline.Error()) {
				log.WithField("peer", peerID).WithError(err).Debug("Could not send metadata request")
			}

			return
		}

		// Update peer's metadata.
		s.cfg.p2p.Peers().SetMetadata(peerID, peerMetadata)
	}()

	return nil
}

// sendPingRequest first sends a PING request to the peer.
// If the peer responds with a sequence number higher than latest one for it we have in our store,
// then this function sends a METADATA request to the peer, and stores the metadata received.
// This function is actually poorly named, since it does more than just sending a ping request.
func (s *Service) sendPingRequest(ctx context.Context, peerID peer.ID) error {
	ctx, cancel := context.WithTimeout(ctx, respTimeout)
	defer cancel()

	// Get the current epoch.
	currentSlot := s.cfg.clock.CurrentSlot()
	currentEpoch := slots.ToEpoch(currentSlot)

	// SSZ encode our metadata sequence number.
	metadataSeq := s.cfg.p2p.MetadataSeq()
	encodedMetadataSeq := primitives.SSZUint64(metadataSeq)

	// Get the PING topic for the current epoch.
	topic, err := p2p.TopicFromMessage(p2p.PingMessageName, currentEpoch)
	if err != nil {
		return errors.Wrap(err, "topic from message")
	}

	// Send the PING request to the peer.
	stream, err := s.cfg.p2p.Send(ctx, &encodedMetadataSeq, topic, peerID)
	if err != nil {
		return errors.Wrap(err, "send ping request")
	}
	defer closeStream(stream, log)

	startTime := time.Now()

	// Read the response from the peer.
	code, errMsg, err := ReadStatusCode(stream, s.cfg.p2p.Encoding())
	if err != nil {
		return errors.Wrap(err, "read status code")
	}

	// Record the latency of the ping request for that peer.
	s.cfg.p2p.Host().Peerstore().RecordLatency(peerID, time.Now().Sub(startTime))

	// If the peer responded with an error, increment the bad responses scorer.
	if code != 0 {
		s.cfg.p2p.Peers().Scorers().BadResponsesScorer().Increment(peerID)
		return errors.Errorf("code: %d - %s", code, errMsg)
	}

	// Decode the sequence number from the peer.
	msg := new(primitives.SSZUint64)
	if err := s.cfg.p2p.Encoding().DecodeWithMaxLength(stream, msg); err != nil {
		return errors.Wrap(err, "decode sequence number")
	}

	// Determine if the peer's sequence number returned by the peer is higher than the one we have in our store.
	valid, err := s.validateSequenceNum(*msg, peerID)
	if err != nil {
		// Descore peer for giving us a bad sequence number.
		if errors.Is(err, p2ptypes.ErrInvalidSequenceNum) {
			s.cfg.p2p.Peers().Scorers().BadResponsesScorer().Increment(peerID)
		}

		return errors.Wrap(err, "validate sequence number")
	}

	// The sequence number have in our store for this peer is the same as the one returned by the peer, all good.
	if valid {
		return nil
	}

	// We need to send a METADATA request to the peer to get its latest metadata.
	md, err := s.sendMetaDataRequest(ctx, peerID)
	if err != nil {
		// do not increment bad responses, as its already done in the request method.
		return errors.Wrap(err, "send metadata request")
	}

	// Update the metadata for the peer.
	s.cfg.p2p.Peers().SetMetadata(peerID, md)

	return nil
}

// validateSequenceNum validates the peer's sequence number.
// - If the peer's sequence number is greater than the sequence number we have in our store for the peer, return false.
// - If the peer's sequence number is equal to the sequence number we have in our store for the peer, return true.
// - If the peer's sequence number is less than the sequence number we have in our store for the peer, return an error.
func (s *Service) validateSequenceNum(seq primitives.SSZUint64, id peer.ID) (bool, error) {
	// Retrieve the metadata for the peer we got in our store.
	md, err := s.cfg.p2p.Peers().Metadata(id)
	if err != nil {
		return false, errors.Wrap(err, "get metadata")
	}

	// If we have no metadata for the peer, return false.
	if md == nil || md.IsNil() {
		return false, nil
	}

	// The peer's sequence number must be less than or equal to the sequence number we have in our store.
	if md.SequenceNumber() > uint64(seq) {
		return false, p2ptypes.ErrInvalidSequenceNum
	}

	// Return true if the peer's sequence number is equal to the sequence number we have in our store.
	return md.SequenceNumber() == uint64(seq), nil
}
