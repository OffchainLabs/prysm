package state_native

import (
	"github.com/OffchainLabs/methodical-ssz/ssz"
	"github.com/pkg/errors"
)

var errAssertionFailed = errors.New("failed to convert interface to state container")

func (b *BeaconState) MarshalSSZ() ([]byte, error) {
	c := b.ToContainer()

	s, ok := c.(ssz.Marshaler)
	if !ok {
		return nil, errAssertionFailed
	}
	return s.MarshalSSZ()
}
