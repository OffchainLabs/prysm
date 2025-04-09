package kv

import (
	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/go-bitfield"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/attestation"
)

func (c *AttCaches) insertSeenBit(att ethpb.Att) error {
	id, err := attestation.NewId(att, attestation.Data)
	if err != nil {
		return errors.Wrap(err, "could not create attestation ID")
	}

	v, ok := c.seenAtt.Get(id.String())
	if ok {
		seenBits, ok := v.([]bitfield.Bitlist)
		if !ok {
			return errors.New("could not convert to bitlist type")
		}
		alreadyExists := false
		for _, bit := range seenBits {
			if c, err := bit.Contains(att.GetAggregationBits()); err != nil {
				return err
			} else if c {
				alreadyExists = true
				break
			}
		}
		if !alreadyExists {
			seenBits = append(seenBits, att.GetAggregationBits())
		}
		c.seenAtt.Set(id.String(), seenBits, cache.DefaultExpiration /* one epoch */)
		return nil
	}

	c.seenAtt.Set(id.String(), []bitfield.Bitlist{att.GetAggregationBits()}, cache.DefaultExpiration /* one epoch */)
	return nil
}

func (c *AttCaches) hasSeenBit(att ethpb.Att) (bool, error) {
	id, err := attestation.NewId(att, attestation.Data)
	if err != nil {
		return false, errors.Wrap(err, "could not create attestation ID")
	}

	v, ok := c.seenAtt.Get(id.String())
	if ok {
		seenBits, ok := v.([]bitfield.Bitlist)
		if !ok {
			return false, errors.New("could not convert to bitlist type")
		}
		for _, bit := range seenBits {
			if c, err := bit.Contains(att.GetAggregationBits()); err != nil {
				return false, err
			} else if c {
				return true, nil
			}
		}
	}
	return false, nil
}
