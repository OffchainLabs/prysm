package rlnc

import (
	"crypto/rand"
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/require"
	ristretto "github.com/gtank/ristretto255"
)

func TestCommit(t *testing.T) {
	n := 5
	c := newCommitter(uint(n))
	require.NotNil(t, c)
	require.Equal(t, n, len(c.generators))

	scalars := make([]*ristretto.Scalar, n)
	randomBytes := make([]byte, 64)
	for i := range scalars {
		_, err := rand.Read(randomBytes)
		require.NoError(t, err)
		scalars[i] = &ristretto.Scalar{}
		scalars[i].FromUniformBytes(randomBytes)
	}

	msm := &ristretto.Element{}
	msm.VarTimeMultiScalarMult(scalars, c.generators)

	expected := ristretto.NewElement()
	summand := &ristretto.Element{}
	for i, scalar := range scalars {
		summand.ScalarMult(scalar, c.generators[i])
		expected.Add(expected, summand)
	}
	require.Equal(t, 1, expected.Equal(msm))

	committment, err := c.commit(scalars)
	require.NoError(t, err)
	require.Equal(t, 1, committment.Equal(msm))
}
