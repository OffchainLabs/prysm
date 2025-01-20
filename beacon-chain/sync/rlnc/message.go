package rlnc

import (
	"github.com/OffchainLabs/prysm/v6/crypto/rand"
	ristretto "github.com/gtank/ristretto255"
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
