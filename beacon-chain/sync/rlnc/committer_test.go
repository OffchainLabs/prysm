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

	scalars := make([]*ristretto.Scalar, n+1)
	randomBytes := make([]byte, 64)
	for i := range scalars {
		_, err := rand.Read(randomBytes)
		require.NoError(t, err)
		scalars[i] = &ristretto.Scalar{}
		scalars[i].FromUniformBytes(randomBytes)
	}
	msm := &ristretto.Element{}
	msm.VarTimeMultiScalarMult(scalars[:n], c.generators)

	_, err := c.commit(scalars[:n-1])
	require.NoError(t, err)
	_, err = c.commit(scalars)
	require.NotNil(t, err)
	committment, err := c.commit(scalars[:n])
	require.NoError(t, err)
	require.Equal(t, 1, committment.Equal(msm))
}
