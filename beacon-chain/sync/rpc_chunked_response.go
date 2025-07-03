package sync

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/encoder"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/pkg/errors"
)

// chunkBlockWriter writes the given message as a chunked response to the given network
// stream.
// response_chunk  ::= <result> | <context-bytes> | <encoding-dependent-header> | <encoded-payload>
func (s *Service) chunkBlockWriter(stream libp2pcore.Stream, blk interfaces.ReadOnlySignedBeaconBlock) error {
	SetStreamWriteDeadline(stream, defaultWriteDuration)
	return WriteBlockChunk(stream, s.cfg.clock, s.cfg.p2p.Encoding(), blk)
}

// WriteBlockChunk writes block chunk object to stream.
// response_chunk  ::= <result> | <context-bytes> | <encoding-dependent-header> | <encoded-payload>
func WriteBlockChunk(stream libp2pcore.Stream, tor blockchain.TemporalOracle, encoding encoder.NetworkEncoding, blk interfaces.ReadOnlySignedBeaconBlock) error {
	if _, err := stream.Write([]byte{responseCodeSuccess}); err != nil {
		return err
	}

	digest := params.ForkDigest(slots.ToEpoch(blk.Block().Slot()))
	if err := writeContextToStream(digest[:], stream); err != nil {
		return err
	}
	_, err := encoding.EncodeWithMaxLength(stream, blk)
	return err
}

// ReadChunkedBlock handles each response chunk that is sent by the
// peer and converts it into a beacon block.
func ReadChunkedBlock(stream libp2pcore.Stream, tor blockchain.TemporalOracle, p2p p2p.EncodingProvider, isFirstChunk bool) (interfaces.ReadOnlySignedBeaconBlock, error) {
	// Handle deadlines differently for first chunk
	if isFirstChunk {
		return readFirstChunkedBlock(stream, tor, p2p)
	}

	return readResponseChunk(stream, tor, p2p)
}

// readFirstChunkedBlock reads the first chunked block and applies the appropriate deadlines to
// it.
func readFirstChunkedBlock(stream libp2pcore.Stream, tor blockchain.TemporalOracle, p2p p2p.EncodingProvider) (interfaces.ReadOnlySignedBeaconBlock, error) {
	code, errMsg, err := ReadStatusCode(stream, p2p.Encoding())
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, errors.New(errMsg)
	}
	rpcCtx, err := readContextFromStream(stream)
	if err != nil {
		return nil, err
	}
	blk, err := extractDataTypeFromTypeMap(types.BlockMap, rpcCtx, tor)
	if err != nil {
		return nil, err
	}
	err = p2p.Encoding().DecodeWithMaxLength(stream, blk)
	return blk, err
}

// readResponseChunk reads the response from the stream and decodes it into the
// provided message type.
func readResponseChunk(stream libp2pcore.Stream, tor blockchain.TemporalOracle, p2p p2p.EncodingProvider) (interfaces.ReadOnlySignedBeaconBlock, error) {
	SetStreamReadDeadline(stream, respTimeout)
	code, errMsg, err := readStatusCodeNoDeadline(stream, p2p.Encoding())
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, errors.New(errMsg)
	}
	// No-op for now with the rpc context.
	rpcCtx, err := readContextFromStream(stream)
	if err != nil {
		return nil, err
	}
	blk, err := extractDataTypeFromTypeMap(types.BlockMap, rpcCtx, tor)
	if err != nil {
		return nil, err
	}
	err = p2p.Encoding().DecodeWithMaxLength(stream, blk)
	return blk, err
}

// WriteBlobSidecarChunk writes blob chunk object to stream.
// response_chunk  ::= <result> | <context-bytes> | <encoding-dependent-header> | <encoded-payload>
func WriteBlobSidecarChunk(stream libp2pcore.Stream, tor blockchain.TemporalOracle, encoding encoder.NetworkEncoding, sidecar blocks.VerifiedROBlob) error {
	if _, err := stream.Write([]byte{responseCodeSuccess}); err != nil {
		return err
	}
	ctxBytes := params.ForkDigest(slots.ToEpoch(sidecar.Slot()))
	if err := writeContextToStream(ctxBytes[:], stream); err != nil {
		return err
	}
	_, err := encoding.EncodeWithMaxLength(stream, sidecar)
	return err
}

func WriteLightClientBootstrapChunk(stream libp2pcore.Stream, tor blockchain.TemporalOracle, encoding encoder.NetworkEncoding, bootstrap interfaces.LightClientBootstrap) error {
	if _, err := stream.Write([]byte{responseCodeSuccess}); err != nil {
		return err
	}

	digest := params.ForkDigest(slots.ToEpoch(bootstrap.Header().Beacon().Slot))
	if err := writeContextToStream(digest[:], stream); err != nil {
		return err
	}

	_, err := encoding.EncodeWithMaxLength(stream, bootstrap)
	return err
}

func WriteLightClientUpdateChunk(stream libp2pcore.Stream, tor blockchain.TemporalOracle, encoding encoder.NetworkEncoding, update interfaces.LightClientUpdate) error {
	if _, err := stream.Write([]byte{responseCodeSuccess}); err != nil {
		return err
	}

	digest := params.ForkDigest(slots.ToEpoch(update.AttestedHeader().Beacon().Slot))
	if err := writeContextToStream(digest[:], stream); err != nil {
		return err
	}
	_, err := encoding.EncodeWithMaxLength(stream, update)
	return err
}

func WriteLightClientOptimisticUpdateChunk(stream libp2pcore.Stream, tor blockchain.TemporalOracle, encoding encoder.NetworkEncoding, update interfaces.LightClientOptimisticUpdate) error {
	if _, err := stream.Write([]byte{responseCodeSuccess}); err != nil {
		return err
	}

	digest := params.ForkDigest(slots.ToEpoch(update.AttestedHeader().Beacon().Slot))

	if err := writeContextToStream(digest[:], stream); err != nil {
		return err
	}
	_, err := encoding.EncodeWithMaxLength(stream, update)
	return err
}

func WriteLightClientFinalityUpdateChunk(stream libp2pcore.Stream, tor blockchain.TemporalOracle, encoding encoder.NetworkEncoding, update interfaces.LightClientFinalityUpdate) error {
	if _, err := stream.Write([]byte{responseCodeSuccess}); err != nil {
		return err
	}

	digest := params.ForkDigest(slots.ToEpoch(update.AttestedHeader().Beacon().Slot))

	if err := writeContextToStream(digest[:], stream); err != nil {
		return err
	}
	_, err := encoding.EncodeWithMaxLength(stream, update)
	return err
}

// WriteDataColumnSidecarChunk writes data column chunk object to stream.
// response_chunk  ::= <result> | <context-bytes> | <encoding-dependent-header> | <encoded-payload>
func WriteDataColumnSidecarChunk(stream libp2pcore.Stream, tor blockchain.TemporalOracle, encoding encoder.NetworkEncoding, sidecar *ethpb.DataColumnSidecar) error {
	// Success response code.
	if _, err := stream.Write([]byte{responseCodeSuccess}); err != nil {
		return errors.Wrap(err, "stream write")
	}
	ctxBytes := params.ForkDigest(slots.ToEpoch(sidecar.SignedBlockHeader.Header.Slot))
	if err := writeContextToStream(ctxBytes[:], stream); err != nil {
		return errors.Wrap(err, "write context to stream")
	}

	// Sidecar.
	if _, err := encoding.EncodeWithMaxLength(stream, sidecar); err != nil {
		return errors.Wrap(err, "encode with max length")
	}

	return nil
}
