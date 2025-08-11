package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/wrapper"
	pb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1/metadata"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/go-bitfield"
)

// metaDataHandler reads the incoming metadata RPC request from the peer.
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
		case version.Altair, version.Fulu:
			// If the stream version corresponds to Phase 0 but our metadata
			// corresponds to Altair or Fulu, convert our metadata to the Phase 0 one.
			metadata = wrapper.WrappedMetadataV0(
				&pb.MetaDataV0{
					Attnets:   metadata.AttnetsBitfield(),
					SeqNumber: metadata.SequenceNumber(),
				})
		}

	case p2p.SchemaVersionV2:
		switch metadataVersion {
		case version.Phase0:
			// If the stream version corresponds to Altair but our metadata
			// corresponds to Phase 0, convert our metadata to the Altair one,
			// and use a zeroed syncnets bitfield.
			metadata = wrapper.WrappedMetadataV1(
				&pb.MetaDataV1{
					Attnets:   metadata.AttnetsBitfield(),
					SeqNumber: metadata.SequenceNumber(),
					Syncnets:  bitfield.Bitvector4{byte(0x00)},
				})
		case version.Fulu:
			// If the stream version corresponds to Altair but our metadata
			// corresponds to Fulu, convert our metadata to the Altair one.
			metadata = wrapper.WrappedMetadataV1(
				&pb.MetaDataV1{
					Attnets:   metadata.AttnetsBitfield(),
					SeqNumber: metadata.SequenceNumber(),
					Syncnets:  metadata.SyncnetsBitfield(),
				})
		}

	case p2p.SchemaVersionV3:
		switch metadataVersion {
		case version.Phase0:
			// If the stream version corresponds to Fulu but our metadata
			// corresponds to Phase 0, convert our metadata to the Fulu one,
			// and use a zeroed syncnets bitfield and custody group count.
			metadata = wrapper.WrappedMetadataV2(
				&pb.MetaDataV2{
					Attnets:           metadata.AttnetsBitfield(),
					SeqNumber:         metadata.SequenceNumber(),
					Syncnets:          bitfield.Bitvector4{byte(0x00)},
					CustodyGroupCount: 0,
				})
		case version.Altair:
			// If the stream version corresponds to Fulu but our metadata
			// corresponds to Altair, convert our metadata to the Fulu one and
			// use a zeroed custody group count.
			metadata = wrapper.WrappedMetadataV2(
				&pb.MetaDataV2{
					Attnets:           metadata.AttnetsBitfield(),
					SeqNumber:         metadata.SequenceNumber(),
					Syncnets:          metadata.SyncnetsBitfield(),
					CustodyGroupCount: 0,
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

	closeStreamAndWait(stream, log)
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

	defer closeStreamAndWait(stream, log)

	// Read the METADATA response from the peer.
	code, errMsg, err := ReadStatusCode(stream, s.cfg.p2p.Encoding())
	if err != nil {
		s.downscorePeer(peerID, "MetadataReadStatusCodeError")
		return nil, errors.Wrap(err, "read status code")
	}

	if code != 0 {
		s.downscorePeer(peerID, "NonNullMetadataReadStatusCode")
		return nil, errors.New(errMsg)
	}

	digest := params.ForkDigest(currentEpoch)
	// Instantiate zero value of the metadata.
	msg, err := extractDataTypeFromTypeMap(types.MetaDataMap, digest[:], s.cfg.clock)
	if err != nil {
		return nil, errors.Wrap(err, "extract data type from type map")
	}

	// Defensive check to ensure valid objects are being sent.
	var topicVersion string
	switch msg.Version() {
	case version.Phase0:
		topicVersion = p2p.SchemaVersionV1
	case version.Altair:
		topicVersion = p2p.SchemaVersionV2
	case version.Fulu:
		topicVersion = p2p.SchemaVersionV3
	}

	// Validate the version of the topic.
	if err := validateVersion(topicVersion, stream); err != nil {
		return nil, err
	}

	// Decode the metadata from the peer.
	if err := s.cfg.p2p.Encoding().DecodeWithMaxLength(stream, msg); err != nil {
		s.downscorePeer(peerID, "MetadataDecodeError")
		return nil, errors.Wrap(err, "decode with max length")
	}

	return msg, nil
}
