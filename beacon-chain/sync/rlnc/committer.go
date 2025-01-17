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
	for i := range generators {
		generators[i] = randomElement()
		if generators[i] == nil {
			return nil
		}
	}
	return &committer{generators}
}

func (c *committer) commit(scalars []*ristretto.Scalar) (*ristretto.Element, error) {
	if len(scalars) > len(c.generators) {
		return nil, errors.New("too many scalars")
	}
	result := &ristretto.Element{}
	return result.VarTimeMultiScalarMult(scalars, c.generators[:len(scalars)]), nil
}

func (c *committer) num() int {
	return len(c.generators)
}

func randomElement() (ret *ristretto.Element) {
	buf := make([]byte, 64)
	_, err := rand.NewGenerator().Read(buf)
	if err != nil {
		return nil
	}
	ret = &ristretto.Element{}
	ret.FromUniformBytes(buf)
	return
}
