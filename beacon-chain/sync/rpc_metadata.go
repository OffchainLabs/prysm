package sync

import (
	"context"

	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/go-bitfield"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/types"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/wrapper"
	"github.com/prysmaticlabs/prysm/v5/network/forks"
	pb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/metadata"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

// metaDataHandler reads the incoming metadata rpc request from the peer.
func (s *Service) metaDataHandler(_ context.Context, _ interface{}, stream libp2pcore.Stream) error {
	SetRPCStreamDeadlines(stream)

	// Validate the incoming request regarding rate limiting.
	if err := s.rateLimiter.validateRequest(stream, 1); err != nil {
		return errors.Wrap(err, "validate request")
	}

	s.rateLimiter.add(stream, 1)

	// Retrieve our metadata.
	metadata := s.cfg.p2p.Metadata()

	// Handle the case our metadata is nil.
	if metadata == nil || metadata.IsNil() {
		nilErr := errors.New("nil metadata stored for host")

		resp, err := s.generateErrorResponse(responseCodeServerError, types.ErrGeneric.Error())
		if err != nil {
			log.WithError(err).Debug("Could not generate a response error")
			return nilErr
		}

		if _, err := stream.Write(resp); err != nil {
			log.WithError(err).Debug("Could not write to stream")
		}

		return nilErr
	}

	// Get the stream version from the protocol.
	_, _, streamVersion, err := p2p.TopicDeconstructor(string(stream.Protocol()))
	if err != nil {
		wrappedErr := errors.Wrap(err, "topic deconstructor")

		resp, genErr := s.generateErrorResponse(responseCodeServerError, types.ErrGeneric.Error())
		if genErr != nil {
			log.WithError(genErr).Debug("Could not generate a response error")
			return wrappedErr
		}

		if _, wErr := stream.Write(resp); wErr != nil {
			log.WithError(wErr).Debug("Could not write to stream")
		}
		return wrappedErr
	}

	// Handle the case where the stream version is not recognized.
	metadataVersion := metadata.Version()
	switch streamVersion {
	case p2p.SchemaVersionV1:
		switch metadataVersion {
		case version.Altair, version.Deneb:
			metadata = wrapper.WrappedMetadataV0(
				&pb.MetaDataV0{
					Attnets:   metadata.AttnetsBitfield(),
					SeqNumber: metadata.SequenceNumber(),
				})
		}

	case p2p.SchemaVersionV2:
		switch metadataVersion {
		case version.Phase0:
			metadata = wrapper.WrappedMetadataV1(
				&pb.MetaDataV1{
					Attnets:   metadata.AttnetsBitfield(),
					SeqNumber: metadata.SequenceNumber(),
					Syncnets:  bitfield.Bitvector4{byte(0x00)},
				})
		case version.Deneb:
			metadata = wrapper.WrappedMetadataV1(
				&pb.MetaDataV1{
					Attnets:   metadata.AttnetsBitfield(),
					SeqNumber: metadata.SequenceNumber(),
					Syncnets:  metadata.SyncnetsBitfield(),
				})
		}
	}

	// Write the METADATA response into the stream.
	if _, err := stream.Write([]byte{responseCodeSuccess}); err != nil {
		return errors.Wrap(err, "write metadata response")
	}

	// Encode the metadata and write it to the stream.
	_, err = s.cfg.p2p.Encoding().EncodeWithMaxLength(stream, metadata)
	if err != nil {
		return errors.Wrap(err, "encode metadata")
	}

	closeStream(stream, log)
	return nil
}

// sendMetaDataRequest sends a METADATA request to the peer and return the response.
func (s *Service) sendMetaDataRequest(ctx context.Context, peerID peer.ID) (metadata.Metadata, error) {
	ctx, cancel := context.WithTimeout(ctx, respTimeout)
	defer cancel()

	// Compute the current epoch.
	currentSlot := s.cfg.clock.CurrentSlot()
	currentEpoch := slots.ToEpoch(currentSlot)

	// Compute the topic for the metadata request regarding the current epoch.
	topic, err := p2p.TopicFromMessage(p2p.MetadataMessageName, currentEpoch)
	if err != nil {
		return nil, errors.Wrap(err, "topic from message")
	}

	// Send the METADATA request to the peer.
	message := new(interface{})
	stream, err := s.cfg.p2p.Send(ctx, message, topic, peerID)
	if err != nil {
		return nil, errors.Wrap(err, "send metadata request")
	}

	defer closeStream(stream, log)

	// Read the METADATA response from the peer.
	code, errMsg, err := ReadStatusCode(stream, s.cfg.p2p.Encoding())
	if err != nil {
		s.cfg.p2p.Peers().Scorers().BadResponsesScorer().Increment(peerID)
		return nil, errors.Wrap(err, "read status code")
	}

	if code != 0 {
		s.cfg.p2p.Peers().Scorers().BadResponsesScorer().Increment(peerID)
		return nil, errors.New(errMsg)
	}

	// Get the genesis validators root.
	valRoot := s.cfg.clock.GenesisValidatorsRoot()

	// Get the fork digest from the current epoch and the genesis validators root.
	rpcCtx, err := forks.ForkDigestFromEpoch(currentEpoch, valRoot[:])
	if err != nil {
		return nil, errors.Wrap(err, "fork digest from epoch")
	}

	// Instantiate zero value of the metadata.
	msg, err := extractDataTypeFromTypeMap(types.MetaDataMap, rpcCtx[:], s.cfg.clock)
	if err != nil {
		return nil, errors.Wrap(err, "extract data type from type map")
	}

	// Defensive check to ensure valid objects are being sent.
	topicVersion := ""
	switch msg.Version() {
	case version.Phase0:
		topicVersion = p2p.SchemaVersionV1
	case version.Altair:
		topicVersion = p2p.SchemaVersionV2
	}

	// Validate the version of the topic.
	if err := validateVersion(topicVersion, stream); err != nil {
		return nil, err
	}

	// Decode the metadata from the peer.
	if err := s.cfg.p2p.Encoding().DecodeWithMaxLength(stream, msg); err != nil {
		s.cfg.p2p.Peers().Scorers().BadResponsesScorer().Increment(stream.Conn().RemotePeer())
		return nil, err
	}

	return msg, nil
}
