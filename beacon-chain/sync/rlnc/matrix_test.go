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

func Test_isZeroVector(t *testing.T) {
	zero := ristretto.NewScalar()
	nonZero := randomScalar()
	require.Equal(t, true, isZeroVector([]*ristretto.Scalar{zero, zero, zero}))
	require.Equal(t, false, isZeroVector([]*ristretto.Scalar{zero, zero, nonZero}))
}

func Test_scalarOne(t *testing.T) {
	scalar := scalarOne()
	other := randomScalar()
	third := ristretto.NewScalar()
	third.Multiply(scalar, other)
	require.Equal(t, 1, third.Equal(other))
}

func TestAddRow(t *testing.T) {
	e := newEchelon(3)
	row := []*ristretto.Scalar{randomScalar(), randomScalar()}
	require.Equal(t, false, e.addRow(row))
	require.Equal(t, 0, len(e.coefficients))

	row = append(row, randomScalar())
	require.Equal(t, true, e.addRow(row))
	require.Equal(t, 1, len(e.coefficients))

	require.Equal(t, false, e.addRow(row))
	require.Equal(t, 1, len(e.coefficients))

	row = []*ristretto.Scalar{randomScalar(), randomScalar(), randomScalar()}
	require.Equal(t, true, e.addRow(row))
	require.Equal(t, 2, len(e.coefficients))
	require.Equal(t, 1, e.triangular[1][0].Equal(ristretto.NewScalar()))

	row = []*ristretto.Scalar{randomScalar(), randomScalar(), randomScalar()}
	require.Equal(t, true, e.addRow(row))
	require.Equal(t, 3, len(e.coefficients))
	require.Equal(t, 1, e.triangular[2][0].Equal(ristretto.NewScalar()))
	require.Equal(t, 1, e.triangular[2][1].Equal(ristretto.NewScalar()))

	row = []*ristretto.Scalar{randomScalar(), randomScalar(), randomScalar()}
	require.Equal(t, false, e.addRow(row))

	// Check that the transform * coefficients = triangular
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			prod := ristretto.NewScalar()
			for k := 0; k < 3; k++ {
				prod = prod.Add(prod, ristretto.NewScalar().Multiply(e.transform[i][k], e.coefficients[k][j]))
			}
			require.Equal(t, 1, prod.Equal(e.triangular[i][j]))
		}
	}

}

func TestInverse(t *testing.T) {
	e := newEchelon(3)
	_, err := e.inverse()
	require.ErrorIs(t, ErrNoData, err)

	row := []*ristretto.Scalar{randomScalar(), randomScalar(), randomScalar()}
	require.Equal(t, true, e.addRow(row))
	row = []*ristretto.Scalar{randomScalar(), randomScalar(), randomScalar()}
	require.Equal(t, true, e.addRow(row))
	row = []*ristretto.Scalar{randomScalar(), randomScalar(), randomScalar()}
	require.Equal(t, true, e.addRow(row))

	inv, err := e.inverse()
	require.NoError(t, err)
	require.Equal(t, 3, len(inv))
	require.Equal(t, 3, len(inv[0]))
	require.Equal(t, 3, len(inv[1]))
	require.Equal(t, 3, len(inv[2]))

	// Check that the inverse * coefficients = identity
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			prod := ristretto.NewScalar()
			for k := 0; k < 3; k++ {
				prod = prod.Add(prod, ristretto.NewScalar().Multiply(inv[i][k], e.coefficients[k][j]))
			}
			if j != i {
				require.Equal(t, 1, prod.Equal(ristretto.NewScalar()))
			} else {
				require.Equal(t, 1, prod.Equal(scalarOne()))
			}
		}
	}
}
