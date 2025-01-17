package rlnc

import (
	"testing"

	ristretto "github.com/gtank/ristretto255"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestMatrixMul(t *testing.T) {
	// chunks have length 3 and there are two chunks
	s11 := randomScalar()
	s12 := randomScalar()
	s13 := randomScalar()
	scalars1 := []*ristretto.Scalar{s11, s12, s13}

	s21 := randomScalar()
	s22 := randomScalar()
	s23 := randomScalar()
	scalars2 := []*ristretto.Scalar{s21, s22, s23}
	data := [][]*ristretto.Scalar{scalars1, scalars2}

	coeff1 := randomScalar()
	coeff2 := randomScalar()
	coeff3 := randomScalar()

	badCofficients := []*ristretto.Scalar{coeff1, coeff2, coeff3}
	coefficients := badCofficients[:2]

	// Bad number of coefficients
	lc, err := scalarLC(badCofficients, data)
	require.NotNil(t, err)
	require.IsNil(t, lc)

	lc, err = scalarLC(coefficients, data)
	require.NoError(t, err)
	require.Equal(t, len(scalars1), len(lc))
	require.NotNil(t, lc[0])

	require.Equal(t, 1, ristretto.NewScalar().Add(
		ristretto.NewScalar().Multiply(coeff1, s11),
		ristretto.NewScalar().Multiply(coeff2, s21)).Equal(lc[0]))

	require.Equal(t, 1, ristretto.NewScalar().Add(
		ristretto.NewScalar().Multiply(coeff1, s12),
		ristretto.NewScalar().Multiply(coeff2, s22)).Equal(lc[1]))

	require.Equal(t, 1, ristretto.NewScalar().Add(
		ristretto.NewScalar().Multiply(coeff1, s13),
		ristretto.NewScalar().Multiply(coeff2, s23)).Equal(lc[2]))
}
