package rlnc

import (
	"errors"

	"github.com/OffchainLabs/prysm/v6/crypto/rand"
	ristretto "github.com/gtank/ristretto255"
)

// Committer is a structure that holds the Ristretto generators.
type Committer struct {
	generators []*ristretto.Element
}

// newCommitter creates a new committer with the number of generators.
// TODO: read the generators from the config file.
func newCommitter(n uint) *Committer {
	generators := make([]*ristretto.Element, n)
	for i := range generators {
		generators[i] = randomElement()
		if generators[i] == nil {
			return nil
		}
	}
	return &Committer{generators}
}

func (c *Committer) commit(scalars []*ristretto.Scalar) (*ristretto.Element, error) {
	if len(scalars) > len(c.generators) {
		return nil, errors.New("too many scalars")
	}
	result := &ristretto.Element{}
	return result.VarTimeMultiScalarMult(scalars, c.generators[:len(scalars)]), nil
}

func (c *Committer) num() int {
	return len(c.generators)
}

func randomElement() (ret *ristretto.Element) {
	buf := make([]byte, 64)
	gen := rand.NewGenerator()
	_, err := gen.Read(buf)
	if err != nil {
		return nil
	}
	ret = &ristretto.Element{}
	ret.FromUniformBytes(buf)
	return
}
