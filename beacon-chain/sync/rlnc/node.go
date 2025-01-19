package rlnc

import (
	ristretto "github.com/gtank/ristretto255"
)

// Node represents a node in the RLNC network. It keeps the data it holds as a matrix of scalars
// as well as the commitments to the data. The coefficients and a partial inversion of the corresponding
// matrix is kept in the echelon object. The committer keeps the trusted setup generators
type Node struct {
	chunks      [][]*ristretto.Scalar
	commitments []*ristretto.Element
	echelon     *echelon
	committer   *committer
}

func NewNode(committer *committer, size uint) *Node {
	return &Node{
		chunks:    make([][]*ristretto.Scalar, 0),
		echelon:   newEchelon(size),
		committer: committer,
	}
}

// NewSource creates a new node that holds all the data already chunked and committed.
// It is called by a broadcasting node starting the RLNC process.
func NewSource(committer *committer, size uint, data []byte) (*Node, error) {
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

// computeCommitments computes the commitments of the data in the node.
func computeCommitments(c *committer, data [][]*ristretto.Scalar) (commitments []*ristretto.Element, err error) {
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

// blockToChunks converts a block of data to size chunks of data.
func blockToChunks(size uint, data []byte) [][]*ristretto.Scalar {
	chunks := make([][]*ristretto.Scalar, size)
	chunkSize := ((uint(len(data))+size-1)/size + 30) / 31 * 31

	for i := uint(0); i < size; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if end > uint(len(data)) {
			end = uint(len(data))
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
			panic(err) // lint:nopanic -- TODO: Handle this error!
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
	return ret, nil
}
