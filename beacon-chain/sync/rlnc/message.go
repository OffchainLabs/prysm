package rlnc

import (
	"github.com/OffchainLabs/prysm/v6/crypto/rand"
	ristretto "github.com/gtank/ristretto255"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/sirupsen/logrus"
)

type chunk struct {
	data         []*ristretto.Scalar
	coefficients []*ristretto.Scalar
}

type message struct {
	chunk       chunk
	commitments []*ristretto.Element
}

// NewMessage creates a new message from a chunk interface.
func newMessage(c interfaces.ReadOnlyBeaconBlockChunk) (*message, error) {
	data, err := dataToVector(c.Data())
	if err != nil {
		return nil, err
	}
	coefficients, err := dataToVector(c.Coefficients())
	if err != nil {
		return nil, err
	}
	chunk := chunk{
		data:         data,
		coefficients: coefficients,
	}
	commitments, err := dataToElements(c.Commitments())
	if err != nil {
		return nil, err
	}
	return &message{
		chunk:       chunk,
		commitments: commitments,
	}, nil
}

// Verify verifies that the message is compatible with the signed committmments
func (m *message) Verify(c *committer) bool {
	// We should get the same number of coefficients as commitments.
	if len(m.chunk.coefficients) != len(m.commitments) {
		return false
	}
	msm, err := c.commit(m.chunk.data)

	if err != nil {
		return false
	}

	if len(m.chunk.data) > c.num() {
		return false
	}
	com := ristretto.NewElement().VarTimeMultiScalarMult(m.chunk.coefficients, m.commitments)
	return msm.Equal(com) == 1
}

var scalarOneBytes = [32]byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}

func scalarOne() (ret *ristretto.Scalar) {
	ret = &ristretto.Scalar{}
	if err := ret.Decode(scalarOneBytes[:]); err != nil {
		logrus.WithError(err).Error("failed to decode scalar one")
	}
	return
}

func randomScalar() (ret *ristretto.Scalar) {
	buf := make([]byte, 64)
	_, err := rand.NewGenerator().Read(buf)
	if err != nil {
		return nil
	}
	ret = &ristretto.Scalar{}
	ret.FromUniformBytes(buf)
	return
}

// dataToVector converts a slice of scalars encoded as bytes to a slice of scalars.
func dataToVector(data [][]byte) ([]*ristretto.Scalar, error) {
	ret := make([]*ristretto.Scalar, len(data))
	for i, d := range data {
		if len(d) != 32 {
			return nil, ErrInvalidScalar
		}
		ret[i] = &ristretto.Scalar{}
		if err := ret[i].Decode(d); err != nil {
			return nil, ErrInvalidScalar
		}
	}
	return ret, nil
}

// dataToElements converts a slice of scalars encoded as bytes to a slice of elements.
func dataToElements(data [][]byte) ([]*ristretto.Element, error) {
	ret := make([]*ristretto.Element, len(data))
	for i, d := range data {
		if len(d) != 32 {
			return nil, ErrInvalidElement
		}
		ret[i] = &ristretto.Element{}
		if err := ret[i].Decode(d); err != nil {
			return nil, ErrInvalidElement
		}
	}
	return ret, nil
}
