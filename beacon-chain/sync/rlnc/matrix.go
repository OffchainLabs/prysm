package rlnc

import (
	"errors"

	ristretto "github.com/gtank/ristretto255"
)

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
