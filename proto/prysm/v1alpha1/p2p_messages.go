package eth

import (
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
)

// The MaxSizeSSZ methods below report the maximum possible SSZ-encoded size of
// each RPC message type. The p2p encoder validates the length prefix declared
// by a remote peer against this bound before allocating the receive buffer.
// For fixed-size SSZ types, SizeSSZ is a constant and already is that bound.

// MaxSizeSSZ returns the maximum size of the type when SSZ-encoded.
func (c *Status) MaxSizeSSZ() int {
	return c.SizeSSZ()
}

// MaxSizeSSZ returns the maximum size of the type when SSZ-encoded.
func (c *StatusV2) MaxSizeSSZ() int {
	return c.SizeSSZ()
}

// MaxSizeSSZ returns the maximum size of the type when SSZ-encoded.
func (c *BeaconBlocksByRangeRequest) MaxSizeSSZ() int {
	return c.SizeSSZ()
}

// MaxSizeSSZ returns the maximum size of the type when SSZ-encoded.
func (c *BlobSidecarsByRangeRequest) MaxSizeSSZ() int {
	return c.SizeSSZ()
}

// MaxSizeSSZ returns the maximum size of the type when SSZ-encoded.
func (c *ExecutionPayloadEnvelopesByRangeRequest) MaxSizeSSZ() int {
	return c.SizeSSZ()
}

// MaxSizeSSZ returns the maximum size of the type when SSZ-encoded:
// StartSlot (8) and Count (8), plus the Columns list offset (4) and at most
// NumberOfColumns column indices (8 each).
func (c *DataColumnSidecarsByRangeRequest) MaxSizeSSZ() int {
	return 20 + fieldparams.NumberOfColumns*8
}
