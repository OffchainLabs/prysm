package sync

import (
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/encoder"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/types"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/network/forks"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
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
	var obtainedCtx []byte

	valRoot := tor.GenesisValidatorsRoot()
	switch blk.Version() {
	case version.Phase0:
		digest, err := forks.ForkDigestFromEpoch(params.BeaconConfig().GenesisEpoch, valRoot[:])
		if err != nil {
			return err
		}
		obtainedCtx = digest[:]
	case version.Altair:
		digest, err := forks.ForkDigestFromEpoch(params.BeaconConfig().AltairForkEpoch, valRoot[:])
		if err != nil {
			return err
		}
		obtainedCtx = digest[:]
	case version.Bellatrix:
		digest, err := forks.ForkDigestFromEpoch(params.BeaconConfig().BellatrixForkEpoch, valRoot[:])
		if err != nil {
			return err
		}
		obtainedCtx = digest[:]
	case version.Capella:
		digest, err := forks.ForkDigestFromEpoch(params.BeaconConfig().CapellaForkEpoch, valRoot[:])
		if err != nil {
			return err
		}
		obtainedCtx = digest[:]
	case version.Deneb:
		digest, err := forks.ForkDigestFromEpoch(params.BeaconConfig().DenebForkEpoch, valRoot[:])
		if err != nil {
			return err
		}
		obtainedCtx = digest[:]
	case version.Electra:
		digest, err := forks.ForkDigestFromEpoch(params.BeaconConfig().ElectraForkEpoch, valRoot[:])
		if err != nil {
			return err
		}
		obtainedCtx = digest[:]
	default:
		return errors.Wrapf(ErrUnrecognizedVersion, "block version %d is not recognized", blk.Version())
	}

	if err := writeContextToStream(obtainedCtx, stream); err != nil {
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
	valRoot := tor.GenesisValidatorsRoot()
	ctxBytes, err := forks.ForkDigestFromEpoch(slots.ToEpoch(sidecar.Slot()), valRoot[:])
	if err != nil {
		return err
	}

	if err := writeContextToStream(ctxBytes[:], stream); err != nil {
		return err
	}
	_, err = encoding.EncodeWithMaxLength(stream, sidecar)
	return err
}

// WriteDataColumnSidecarChunk writes data column chunk object to stream.
// response_chunk  ::= <result> | <context-bytes> | <encoding-dependent-header> | <encoded-payload>
func WriteDataColumnSidecarChunk(stream libp2pcore.Stream, tor blockchain.TemporalOracle, encoding encoder.NetworkEncoding, sidecar *ethpb.DataColumnSidecar) error {
	if _, err := stream.Write([]byte{responseCodeSuccess}); err != nil {
		return err
	}
	valRoot := tor.GenesisValidatorsRoot()
	ctxBytes, err := forks.ForkDigestFromEpoch(slots.ToEpoch(sidecar.SignedBlockHeader.Header.Slot), valRoot[:])
	if err != nil {
		return err
	}

	if err := writeContextToStream(ctxBytes[:], stream); err != nil {
		return err
	}
	_, err = encoding.EncodeWithMaxLength(stream, sidecar)
	return err
}
