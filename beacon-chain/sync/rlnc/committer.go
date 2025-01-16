package rlnc

import (
	"errors"

	"github.com/OffchainLabs/prysm/v6/crypto/rand"
	ristretto "github.com/gtank/ristretto255"
)

// committer is a structure that holds the Ristretto generators.
type committer struct {
	generators []*ristretto.Element
}

// newCommitter creates a new committer with the number of generators.
// TODO: read the generators from the config file.
func newCommitter(n uint) *committer {
	generators := make([]*ristretto.Element, n)
	randomBytes := make([]byte, 64)
	for i := range generators {
		_, err := rand.NewGenerator().Read(randomBytes)
		if err != nil {
			return nil
		}
		generators[i] = &ristretto.Element{}
		generators[i].FromUniformBytes(randomBytes)
	}
	return &committer{generators}
}

func (c *committer) commit(scalars []*ristretto.Scalar) (*ristretto.Element, error) {
	if len(scalars) > len(c.generators) {
		return nil, errors.New("too many scalars")
	}
	result := &ristretto.Element{}
	return result.VarTimeMultiScalarMult(scalars, c.generators), nil
}
