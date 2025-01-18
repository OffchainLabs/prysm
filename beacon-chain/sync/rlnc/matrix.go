package rlnc

import (
	"errors"

	ristretto "github.com/gtank/ristretto255"
)

var zeroVector = ristretto.NewScalar()

func scalarLC(coeffs []*ristretto.Scalar, data [][]*ristretto.Scalar) (ret []*ristretto.Scalar, err error) {
	if len(coeffs) != len(data) {
		return nil, errors.New("different number of coefficients and vectors")
	}
	if len(data) == 0 {
		return nil, nil
	}
	prod := ristretto.Scalar{}
	ret = make([]*ristretto.Scalar, len(data[0]))
	for i := range ret {
		ret[i] = ristretto.NewScalar()
		for j, c := range coeffs {
			ret[i].Add(ret[i], prod.Multiply(c, data[j][i]))
		}
	}
	return
}

// echelon is a struct that holds the echelon form of a matrix of coefficients and the
// corresponding transformation matrix to get it to that form.
type echelon struct {
	coefficients [][]*ristretto.Scalar
	triangular   [][]*ristretto.Scalar
	transform    [][]*ristretto.Scalar
}

// newEchelon returns a new echelon struct.
func newEchelon(size int) *echelon {
	transform := make([][]*ristretto.Scalar, size)
	for i := range transform {
		transform[i] = make([]*ristretto.Scalar, size)
		for j := range transform[i] {
			if j != i {
				transform[i][j] = ristretto.NewScalar()
			} else {
				transform[i][i] = scalarOne()
			}
		}
	}
	return &echelon{
		coefficients: make([][]*ristretto.Scalar, 0),
		triangular:   make([][]*ristretto.Scalar, 0),
		transform:    transform,
	}
}

func identityMatrix(size int) [][]*ristretto.Scalar {
	coefficients := make([][]*ristretto.Scalar, size)
	for i := range coefficients {
		coefficients[i] = make([]*ristretto.Scalar, size)
		for j := range coefficients[i] {
			if j == i {
				coefficients[i][i] = scalarOne()
			} else {
				coefficients[i][j] = ristretto.NewScalar()
			}
		}
	}
	return coefficients
}

// newIdentityEchelon returns a new echelon struct with the identity coefficients.
func newIdentityEchelon(size int) *echelon {
	return &echelon{
		coefficients: identityMatrix(size),
		triangular:   identityMatrix(size),
		transform:    identityMatrix(size),
	}
}

func (e *echelon) isFull() bool {
	if len(e.coefficients) == 0 {
		return false
	}
	return len(e.coefficients) == len(e.coefficients[0])
}

// copyVector returns a copy of a vector.
func copyVector(v []*ristretto.Scalar) []*ristretto.Scalar {
	ret := make([]*ristretto.Scalar, len(v))
	for i, s := range v {
		ret[i] = ristretto.NewScalar()
		ret[i].Decode(s.Encode(nil))
	}
	return ret
}

// firstEntry returns the index of the first non-zero entry in a vector. It returns -1 if the vector is all zeros.
func firstEntry(v []*ristretto.Scalar) int {
	for i, s := range v {
		if s.Equal(zeroVector) == 0 {
			return i
		}
	}
	return -1
}

// isZeroVector returns true if the vector is all zeros.
func isZeroVector(v []*ristretto.Scalar) bool {
	return firstEntry(v) == -1
}

func (e *echelon) addRow(row []*ristretto.Scalar) bool {
	// do not add malformed rows. This assumes transform is never nil.
	if len(row) != len(e.transform[0]) {
		return false
	}
	if isZeroVector(row) {
		return false
	}
	// do not add anything if we have the full data.
	if e.isFull() {
		return false
	}
	// Add any incoming row if we are empty.
	currentSize := len(e.coefficients)
	if currentSize == 0 {
		e.coefficients = append(e.coefficients, row)
		e.triangular = append(e.triangular, row)
		return true
	}

	// currentSize is the index we are about to add.
	tr := copyVector(e.transform[currentSize])
	newEchelonRow := copyVector(row)
	i := 0
	for ; i < currentSize; i++ {
		j := firstEntry(e.triangular[i])
		k := firstEntry(newEchelonRow)
		if k == -1 {
			return false
		}
		if j < k {
			continue
		}
		if j > k {
			break
		}
		pivot := *e.triangular[i][j]
		f := *newEchelonRow[j]
		for l := range newEchelonRow {
			newEchelonRow[l].Multiply(newEchelonRow[l], &pivot)
			newEchelonRow[l].Subtract(newEchelonRow[l], ristretto.NewScalar().Multiply(&f, e.triangular[i][l]))
			tr[l].Multiply(tr[l], &pivot)
			tr[l].Subtract(tr[l], ristretto.NewScalar().Multiply(&f, e.transform[i][l]))
		}
	}
	if isZeroVector(newEchelonRow) {
		return false
	}
	e.triangular = append(e.triangular[:i], append([][]*ristretto.Scalar{newEchelonRow}, e.triangular[i:]...)...)
	e.coefficients = append(e.coefficients, row)
	if i < currentSize {
		e.transform = append(e.transform[:currentSize], e.transform[currentSize+1:]...)
		e.transform = append(e.transform[:i], append([][]*ristretto.Scalar{tr}, e.transform[i:]...)...)
		return true
	}
	e.transform[i] = tr
	return true
}
