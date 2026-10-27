package eth

import (
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
)

// MaxSizeSSZ returns the fixed SSZ size.
func (c *Status) MaxSizeSSZ() int {
	return c.SizeSSZ()
}

// MaxSizeSSZ returns the fixed SSZ size.
func (c *StatusV2) MaxSizeSSZ() int {
	return c.SizeSSZ()
}

// MaxSizeSSZ returns the fixed SSZ size.
func (c *BeaconBlocksByRangeRequest) MaxSizeSSZ() int {
	return c.SizeSSZ()
}

// MaxSizeSSZ returns the fixed SSZ size.
func (c *BlobSidecarsByRangeRequest) MaxSizeSSZ() int {
	return c.SizeSSZ()
}

// MaxSizeSSZ returns the fixed SSZ size.
func (c *ExecutionPayloadEnvelopesByRangeRequest) MaxSizeSSZ() int {
	return c.SizeSSZ()
}

// MaxSizeSSZ includes the maximum number of column indices.
func (c *DataColumnSidecarsByRangeRequest) MaxSizeSSZ() int {
	fixedSize := (&DataColumnSidecarsByRangeRequest{}).SizeSSZ()
	return fixedSize + fieldparams.NumberOfColumns*8
}
