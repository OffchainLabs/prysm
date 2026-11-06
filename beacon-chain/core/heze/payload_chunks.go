package heze

import (
	"bytes"
	"crypto/sha256"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

// EIP-8411 payload chunking.
//
// The serialized ExecutionPayloadEnvelope is split into
// TOTAL_EXECUTION_PAYLOAD_CHUNKS slices. Each Merkle leaf is sha256 over the
// raw chunk bytes (NOT the SSZ hash tree root of the ByteList), and
// payload_chunks_root is the plain depth-6 merkleization of the 64 leaves
// (Vector[Bytes32, 64], no length mix-in). No padding is applied: the chunk
// size is ceil(len/64) of the actual serialization, the final non-empty chunk
// may be shorter, and trailing chunks may be empty.

var (
	errChunkTooLarge      = errors.New("serialized envelope exceeds MAX_PAYLOAD_SIZE")
	errChunkSizeTooLarge  = errors.New("payload chunk exceeds MAX_PAYLOAD_CHUNK_SIZE")
	errChunkIndexTooLarge = errors.New("payload chunk index >= TOTAL_EXECUTION_PAYLOAD_CHUNKS")
	errChunkBadProofLen   = errors.New("payload chunk inclusion proof has wrong length")
	errChunkProofInvalid  = errors.New("payload chunk inclusion proof is invalid")
	errChunkSetIncomplete = errors.New("payload chunk set is incomplete")
	errEnvelopeRoundTrip  = errors.New("reconstructed envelope does not re-serialize to the committed bytes")
)

// ComputePayloadChunks splits serialized envelope bytes into the fixed number
// of chunks per EIP-8411.
func ComputePayloadChunks(data []byte) ([][]byte, error) {
	n := fieldparams.TotalExecutionPayloadChunks
	chunkSize := (len(data) + n - 1) / n
	if chunkSize > fieldparams.MaxPayloadChunkSize {
		return nil, errChunkTooLarge
	}
	chunks := make([][]byte, n)
	for i := range n {
		start := min(i*chunkSize, len(data))
		end := min(start+chunkSize, len(data))
		chunks[i] = data[start:end]
	}
	return chunks, nil
}

// payloadChunkLayers builds the full Merkle tree over sha256(chunk) leaves.
// layers[0] holds the 64 leaves and layers[ChunkInclusionProofDepth] the root.
func payloadChunkLayers(chunks [][]byte) [][][32]byte {
	depth := fieldparams.ChunkInclusionProofDepth
	layers := make([][][32]byte, depth+1)
	leaves := make([][32]byte, len(chunks))
	for i, c := range chunks {
		leaves[i] = sha256.Sum256(c)
	}
	layers[0] = leaves
	for d := range depth {
		prev := layers[d]
		next := make([][32]byte, len(prev)/2)
		for i := range next {
			next[i] = sha256.Sum256(append(prev[2*i][:], prev[2*i+1][:]...))
		}
		layers[d+1] = next
	}
	return layers
}

// ComputePayloadChunksRoot returns hash_tree_root(Vector[Bytes32, 64]) over the
// sha256 leaves of the chunk set.
func ComputePayloadChunksRoot(chunks [][]byte) ([32]byte, error) {
	if len(chunks) != fieldparams.TotalExecutionPayloadChunks {
		return [32]byte{}, errChunkSetIncomplete
	}
	layers := payloadChunkLayers(chunks)
	return layers[fieldparams.ChunkInclusionProofDepth][0], nil
}

// PayloadChunksRootForEnvelope serializes the envelope and returns its chunk
// commitment; this is the value the builder places in the bid.
func PayloadChunksRootForEnvelope(envelope *ethpb.ExecutionPayloadEnvelopeHeze) ([32]byte, error) {
	data, err := envelope.MarshalSSZ()
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "could not marshal envelope")
	}
	chunks, err := ComputePayloadChunks(data)
	if err != nil {
		return [32]byte{}, err
	}
	return ComputePayloadChunksRoot(chunks)
}

// BuildPayloadChunkMessages serializes the envelope and assembles all 64
// PayloadChunk gossip messages with their inclusion proofs. The slot and
// beacon block root are unauthenticated routing hints; the root may be zero at
// bid time and stamped later via StampPayloadChunkHints.
func BuildPayloadChunkMessages(envelope *ethpb.ExecutionPayloadEnvelopeHeze, slot primitives.Slot, beaconBlockRoot [32]byte) ([]*ethpb.PayloadChunk, [32]byte, error) {
	data, err := envelope.MarshalSSZ()
	if err != nil {
		return nil, [32]byte{}, errors.Wrap(err, "could not marshal envelope")
	}
	chunks, err := ComputePayloadChunks(data)
	if err != nil {
		return nil, [32]byte{}, err
	}
	layers := payloadChunkLayers(chunks)
	root := layers[fieldparams.ChunkInclusionProofDepth][0]

	msgs := make([]*ethpb.PayloadChunk, len(chunks))
	for i, c := range chunks {
		proof := make([][]byte, fieldparams.ChunkInclusionProofDepth)
		idx := i
		for d := range fieldparams.ChunkInclusionProofDepth {
			sibling := layers[d][idx^1]
			proof[d] = bytes.Clone(sibling[:])
			idx >>= 1
		}
		msgs[i] = &ethpb.PayloadChunk{
			PayloadChunksRoot:   bytes.Clone(root[:]),
			Index:               uint64(i),
			Chunk:               bytes.Clone(c),
			ChunkInclusionProof: proof,
			Slot:                slot,
			BeaconBlockRoot:     bytes.Clone(beaconBlockRoot[:]),
		}
	}
	return msgs, root, nil
}

// StampPayloadChunkHints fills the unauthenticated routing hints on a
// precomputed chunk set once the owning block is known.
func StampPayloadChunkHints(msgs []*ethpb.PayloadChunk, slot primitives.Slot, beaconBlockRoot [32]byte) {
	for _, m := range msgs {
		if m == nil {
			continue
		}
		m.Slot = slot
		m.BeaconBlockRoot = bytes.Clone(beaconBlockRoot[:])
	}
}

// VerifyPayloadChunkInclusionProof checks the depth-6 Merkle branch binding
// sha256(chunk.Chunk) at chunk.Index to chunk.PayloadChunksRoot. The index
// bound is security critical: the branch walk reads only the low
// CHUNK_INCLUSION_PROOF_DEPTH bits, so an unbounded index would allow replay
// at index + k*TOTAL_EXECUTION_PAYLOAD_CHUNKS.
//
// The size bound is likewise security critical and cannot be delegated to SSZ.
// The generated PayloadChunk.UnmarshalSSZ enforces no upper bound on the chunk
// byte list (only MarshalSSZ does), and the Merkle leaf is a plain
// sha256(chunk) over whatever bytes arrived, so an oversized chunk both decodes
// and proof-verifies. Without this check a builder could commit to a chunk set
// whose pieces each exceed MAX_PAYLOAD_CHUNK_SIZE and every node on the network
// would accept and buffer TOTAL_EXECUTION_PAYLOAD_CHUNKS of them.
func VerifyPayloadChunkInclusionProof(chunk *ethpb.PayloadChunk) error {
	if chunk == nil {
		return errors.New("nil payload chunk")
	}
	if chunk.Index >= fieldparams.TotalExecutionPayloadChunks {
		return errChunkIndexTooLarge
	}
	if len(chunk.Chunk) > fieldparams.MaxPayloadChunkSize {
		return errChunkSizeTooLarge
	}
	if len(chunk.ChunkInclusionProof) != fieldparams.ChunkInclusionProofDepth {
		return errChunkBadProofLen
	}
	node := sha256.Sum256(chunk.Chunk)
	for d, sibling := range chunk.ChunkInclusionProof {
		if len(sibling) != 32 {
			return errChunkBadProofLen
		}
		if chunk.Index>>uint(d)&1 == 1 {
			node = sha256.Sum256(append(bytes.Clone(sibling), node[:]...))
		} else {
			node = sha256.Sum256(append(node[:], sibling...))
		}
	}
	if !bytes.Equal(node[:], chunk.PayloadChunksRoot) {
		return errChunkProofInvalid
	}
	return nil
}

// ReconstructPayloadEnvelope concatenates a complete, proof-verified chunk set
// in index order and strictly decodes the Heze envelope. A round-trip
// re-serialization guard rejects any decoder leniency: the committed byte
// string must be exactly what the envelope re-serializes to.
func ReconstructPayloadEnvelope(chunks [][]byte) (*ethpb.ExecutionPayloadEnvelopeHeze, error) {
	if len(chunks) != fieldparams.TotalExecutionPayloadChunks {
		return nil, errChunkSetIncomplete
	}
	size := 0
	for _, c := range chunks {
		size += len(c)
	}
	data := make([]byte, 0, size)
	for _, c := range chunks {
		data = append(data, c...)
	}
	envelope := &ethpb.ExecutionPayloadEnvelopeHeze{}
	if err := envelope.UnmarshalSSZ(data); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal reconstructed envelope")
	}
	reserialized, err := envelope.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrap(err, "could not re-marshal reconstructed envelope")
	}
	if !bytes.Equal(reserialized, data) {
		return nil, errEnvelopeRoundTrip
	}
	return envelope, nil
}

// IsChunkAuthenticatedEnvelope reports whether the signed envelope carries the
// all-zero signature sentinel marking an envelope reconstructed from
// proof-verified payload chunks (EIP-8411). Such envelopes are authenticated
// against the bid's chunk commitment instead of a BLS signature.
func IsChunkAuthenticatedEnvelope(signedEnvelope interfaces.ROSignedExecutionPayloadEnvelope) bool {
	return signedEnvelope.Signature() == [fieldparams.BLSSignatureLength]byte{}
}

// VerifyEnvelopeChunkCommitment authenticates a chunk-reconstructed Heze
// envelope by recomputing its chunk commitment and comparing it against the
// payload_chunks_root the state's committed bid signed over. Every chunk was
// individually proof-verified against that root on gossip, so a matching
// recomputation proves the envelope bytes are exactly the ones the builder
// committed to.
func VerifyEnvelopeChunkCommitment(st state.BeaconState, signedEnvelope interfaces.ROSignedExecutionPayloadEnvelope) error {
	signed, ok := signedEnvelope.Proto().(*ethpb.SignedExecutionPayloadEnvelopeHeze)
	if !ok {
		return errors.New("chunk-authenticated envelope is not a Heze envelope")
	}
	root, err := PayloadChunksRootForEnvelope(signed.Message)
	if err != nil {
		return errors.Wrap(err, "could not compute envelope chunk commitment")
	}
	bid, err := st.LatestExecutionPayloadBid()
	if err != nil {
		return errors.Wrap(err, "could not get latest execution payload bid")
	}
	if bid == nil {
		return errors.New("latest execution payload bid is nil")
	}
	if root != bid.PayloadChunksRoot() {
		return errors.New("envelope chunk commitment does not match the committed bid payload_chunks_root")
	}
	return nil
}
