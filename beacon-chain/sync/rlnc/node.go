package rlnc

import (
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	ristretto "github.com/gtank/ristretto255"
	"github.com/sirupsen/logrus"
)

// rlncBlockSuffix is a byte that is added to the end of the block to mark it's end.
var rlncBlockSuffix = byte(0xfe)

// Node represents a node in the RLNC network. It keeps the data it holds as a matrix of scalars
// as well as the commitments to the data. The coefficients and a partial inversion of the corresponding
// matrix is kept in the echelon object. The committer keeps the trusted setup generators
type Node struct {
	chunks        [][]*ristretto.Scalar
	commitments   []*ristretto.Element
	echelon       *echelon
	committer     *Committer
	slot          primitives.Slot           // repeated but convenient for the validator client
	proposerIndex primitives.ValidatorIndex // repeated but convenient for the validator client
	parentRoot    []byte                    // repeated but convenient for the validator client
	signature     []byte                    // repeated but convenient for the validator client
}

func NewNode(committer *Committer, size uint) *Node {
	return &Node{
		chunks:    make([][]*ristretto.Scalar, 0),
		echelon:   newEchelon(size),
		committer: committer,
	}
}

func (n *Node) GetChunkedBlock(blk *ethpb.GenericSignedBeaconBlock) *ethpb.ChunkedBeaconBlock {
	chunks := make([]*ethpb.BeaconBlockChunkData, len(n.chunks))
	for i, c := range n.Data() {
		chunks[i] = &ethpb.BeaconBlockChunkData{
			Data: c,
		}
	}
	header := &ethpb.BeaconBlockChunkHeader{
		Slot:          n.Slot(),
		ProposerIndex: n.ProposerIndex(),
		ParentRoot:    n.ParentRoot(),
		Commitments:   n.Commitments(),
	}

	return &ethpb.ChunkedBeaconBlock{
		Header:    header,
		Chunks:    chunks,
		Signature: n.Signature(),
		Block:     blk,
	}
}

// NewSource creates a new node that holds all the data already chunked and committed.
// It is called by a broadcasting node starting the RLNC process.
func NewSource(committer *Committer, size uint, data []byte) (*Node, error) {
	chunks := blockToChunks(size, data)
	commitments, err := computeCommitments(committer, chunks)
	if err != nil {
		return nil, err
	}
	return &Node{
		chunks:      chunks,
		commitments: commitments,
		echelon:     newIdentityEchelon(size),
		committer:   committer,
	}, nil
}

// SetSlot sets the slot of the node.
func (n *Node) SetSlot(slot primitives.Slot) {
	n.slot = slot
}

// SetProposerIndex sets the proposer index of the node.
func (n *Node) SetProposerIndex(proposerIndex primitives.ValidatorIndex) {
	n.proposerIndex = proposerIndex
}

// SetSignature sets the signature of the node.
func (n *Node) SetSignature(signature []byte) {
	n.signature = signature
}

// SetParentRoot sets the parent root of the node.
func (n *Node) SetParentRoot(parentRoot []byte) {
	n.parentRoot = parentRoot
}

// Slot returns the slot of the node.
func (n *Node) Slot() primitives.Slot {
	return n.slot
}

// ProposerIndex returns the proposer index of the node.
func (n *Node) ProposerIndex() primitives.ValidatorIndex {
	return n.proposerIndex
}

// Signature returns the signature of the node.
func (n *Node) Signature() []byte {
	return n.signature
}

// ParentRoot returns the parent root of the node.
func (n *Node) ParentRoot() []byte {
	return n.parentRoot
}

// computeCommitments computes the commitments of the data in the node.
func computeCommitments(c *Committer, data [][]*ristretto.Scalar) (commitments []*ristretto.Element, err error) {
	if len(data) == 0 {
		return nil, nil
	}
	commitments = make([]*ristretto.Element, len(data))
	for i, d := range data {
		commitments[i], err = c.commit(d)
		if err != nil {
			return nil, err
		}
	}
	return commitments, nil
}

func blockToChunks(numChunks uint, data []byte) [][]*ristretto.Scalar {
	chunks := make([][]*ristretto.Scalar, numChunks)
	chunkSize := ((uint(len(data))+numChunks-1)/numChunks + 30) / 31 * 31

	for i := uint(0); i < numChunks; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if end > uint(len(data)) {
			end = uint(len(data))
		}
		// The last chunk can actually be fully zeroes.
		if start > uint(len(data)) {
			start = end
		}
		chunk := data[start:end]

		// Pad the chunk with zeroes if necessary
		if uint(len(chunk)) < chunkSize {
			paddedChunk := make([]byte, chunkSize)
			copy(paddedChunk, chunk)
			chunk = paddedChunk
		}
		chunks[i] = bytesToVector(chunk)
	}
	return chunks
}

// bytesToVector converts a byte slice to a vector of scalars. The input is expected to be a multiple of 31 bytes.
// We take the very naive approach of decoding each 31 bytes of data to a scalar of 32 bytes,
// this way we are guaranteed to get a valid little Endian encoding of the scalar.
// TODO: The actual network encoding should take care of adding 5 zero bits every 256 bits so that
// we can avoid copying memory every time here. This is a temporary solution.
func bytesToVector(data []byte) []*ristretto.Scalar {
	ret := make([]*ristretto.Scalar, len(data)/31)
	for i := 0; i < len(data); i += 31 {
		ret[i/31] = ristretto.NewScalar()
		if err := ret[i/31].Decode(append(data[i:i+31], byte(0))); err != nil {
			logrus.WithError(err).Error("failed to decode scalar")
		}
	}
	return ret
}

func (n *Node) generateRandomCoeffs() []*ristretto.Scalar {
	coeffs := make([]*ristretto.Scalar, len(n.chunks))
	for i := 0; i < len(n.chunks); i++ {
		coeffs[i] = randomScalar()
	}
	return coeffs
}

func (n *Node) chunkLC(scalars []*ristretto.Scalar) (chunk, error) {
	if len(scalars) != len(n.chunks) {
		return chunk{}, ErrInvalidSize
	}
	data, err := scalarLC(scalars, n.chunks)
	if err != nil {
		return chunk{}, err
	}
	coefficients, err := scalarLC(scalars, n.echelon.coefficients)
	if err != nil {
		return chunk{}, err
	}
	return chunk{
		data:         data,
		coefficients: coefficients,
	}, nil
}

func (n *Node) prepareMessage() (*message, error) {
	if len(n.chunks) == 0 {
		return nil, ErrNoData
	}
	scalars := n.generateRandomCoeffs()
	chunk, err := n.chunkLC(scalars)
	if err != nil {
		return nil, err
	}
	return &message{
		chunk:       chunk,
		commitments: n.commitments,
	}, nil
}

// checkExistingCommitments returns true if the commitments are the same as the ones in the node or the node didn't have any.
func (n *Node) checkExistingCommitments(c []*ristretto.Element) bool {
	if len(n.commitments) == 0 {
		return true
	}
	if len(c) != len(n.commitments) {
		return false
	}
	for i := 0; i < len(c); i++ {
		if c[i].Equal(n.commitments[i]) != 1 {
			return false
		}
	}
	return true
}

// checkExistingChunks checks that the incoming chunk has the same size as the preexisting ones if any.
func (n *Node) checkExistingChunks(c []*ristretto.Scalar) bool {
	if len(n.chunks) == 0 {
		return true
	}
	return len(c) == len(n.chunks[0])
}

func (n *Node) receive(message *message) error {
	if !n.checkExistingCommitments(message.commitments) {
		return ErrIncorrectCommitments
	}
	if !n.checkExistingChunks(message.chunk.data) {
		return ErrInvalidSize
	}
	if len(message.chunk.coefficients) != len(message.commitments) {
		return ErrInvalidSize
	}
	if !message.Verify(n.committer) {
		return ErrInvalidMessage
	}
	if !n.echelon.addRow(message.chunk.coefficients) {
		return ErrLinearlyDependentMessage
	}
	n.chunks = append(n.chunks, message.chunk.data)
	if len(n.commitments) == 0 {
		n.commitments = make([]*ristretto.Element, len(message.commitments))
		for i, c := range message.commitments {
			n.commitments[i] = &ristretto.Element{}
			*n.commitments[i] = *c
		}
	}
	return nil
}

// stripSuffix returns the slice before the last appearance of suffix, returns the whole data if suffix is not found
func stripSuffix(data []byte, suffix byte) []byte {
	for i := len(data) - 1; i > 0; i-- {
		if data[i] == suffix {
			return data[:i]
		}
	}
	return data
}

func (n *Node) decode() ([]byte, error) {
	inverse, err := n.echelon.inverse()
	if err != nil {
		return nil, err
	}
	ret := make([]byte, 0, len(n.chunks)*len(n.chunks[0])*31+1)
	prod := &ristretto.Scalar{}
	for i := 0; i < len(inverse); i++ {
		for k := 0; k < len(n.chunks[0]); k++ {
			entry := ristretto.NewScalar()
			for j := 0; j < len(inverse); j++ {
				prod = prod.Multiply(inverse[i][j], n.chunks[j][k])
				entry = entry.Add(entry, prod)
			}
			ret = entry.Encode(ret)[:len(ret)+31] // len(ret) is computed before the append.
		}
	}
	return stripSuffix(ret, rlncBlockSuffix), nil
}

// Data returns the data of the chunks in a node as serialized bytes
func (m *Node) Data() [][][]byte {
	ret := make([][][]byte, len(m.chunks))
	for j, c := range m.chunks {
		ret[j] = make([][]byte, len(c))
		for i, d := range c {
			ret[j][i] = d.Encode(nil)
		}
	}
	return ret
}

// Commitments returns the commitments of the chunks in a node as serialized bytes
func (m *Node) Commitments() [][]byte {
	ret := make([][]byte, len(m.commitments))
	for j, c := range m.commitments {
		ret[j] = c.Encode(nil)
	}
	return ret
}
