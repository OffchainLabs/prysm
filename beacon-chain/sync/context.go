package sync

import (
	"encoding/binary"
	"io"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/pkg/errors"
)

// Specifies the fixed size context length.
const forkDigestLength = 4

// writes peer's current context for the expected payload to the stream.
func writeContextToStream(objCtx []byte, stream network.Stream) error {
	// The rpc context for our v2 methods is the fork-digest of
	// the relevant payload. We write the associated fork-digest(context)
	// into the stream for the payload.
	rpcCtx, err := expectRpcContext(stream)
	if err != nil {
		return err
	}
	// Exit early if an empty context is expected.
	if !rpcCtx {
		return nil
	}
	_, err = stream.Write(objCtx)
	return err
}

// reads any attached context-bytes to the payload.
func readContextFromStream(stream network.Stream) ([]byte, error) {
	hasCtx, err := expectRpcContext(stream)
	if err != nil {
		return nil, err
	}
	if !hasCtx {
		return []byte{}, nil
	}
	// Read context (fork-digest) from stream
	b := make([]byte, forkDigestLength)
	if _, err := io.ReadFull(stream, b); err != nil {
		return nil, err
	}
	return b, nil
}

func expectRpcContext(stream network.Stream) (bool, error) {
	_, message, version, err := p2p.TopicDeconstructor(string(stream.Protocol()))
	if err != nil {
		return false, err
	}
	// For backwards compatibility, we want to omit context bytes for certain v1 methods that were defined before
	// context bytes were introduced into the protocol.
	if version == p2p.SchemaVersionV1 && p2p.OmitContextBytesV1[message] {
		return false, nil
	}
	return true, nil
}

// Minimal interface for a stream with a protocol.
type withProtocol interface {
	Protocol() protocol.ID
}

// Validates that the rpc topic matches the provided version.
func validateVersion(version string, stream withProtocol) error {
	_, _, streamVersion, err := p2p.TopicDeconstructor(string(stream.Protocol()))
	if err != nil {
		return err
	}
	if streamVersion != version {
		return errors.Errorf("stream version of %s doesn't match provided version %s", streamVersion, version)
	}
	return nil
}

// ContextByteVersions is a mapping between expected values for context bytes
// and the runtime/version identifier they correspond to. This can be used to look up the type
// needed to unmarshal a wire-encoded value.
type ContextByteVersions map[[4]byte]int

// ContextByteVersionsForValRoot computes a mapping between all possible context bytes values
// and the runtime/version identifier for the corresponding fork, deriving fork digests using the
// provided genesis validators root and applying Fulu BPO mixing when applicable.
func ContextByteVersionsForValRoot(valRoot [32]byte) (ContextByteVersions, error) {
	entries := params.SortedNetworkScheduleEntries()
	m := make(ContextByteVersions, len(entries))
	for _, entry := range entries {
		fd := &ethpb.ForkData{
			CurrentVersion:        entry.ForkVersion[:],
			GenesisValidatorsRoot: valRoot[:],
		}
		root, err := fd.HashTreeRoot()
		if err != nil {
			return nil, err
		}
		var digest [4]byte
		copy(digest[:], root[:4])
		if entry.Epoch >= params.BeaconConfig().FuluForkEpoch {
			hb := make([]byte, 16)
			binary.LittleEndian.PutUint64(hb[0:8], uint64(entry.BPOEpoch))
			binary.LittleEndian.PutUint64(hb[8:], entry.MaxBlobsPerBlock)
			bpoHash := hash.Hash(hb)
			digest[0] ^= bpoHash[0]
			digest[1] ^= bpoHash[1]
			digest[2] ^= bpoHash[2]
			digest[3] ^= bpoHash[3]
		}
		m[digest] = entry.VersionEnum
	}
	return m, nil
}
