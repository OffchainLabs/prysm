package hdiff

import (
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

// executionPayloadBidSSZ returns the fork-specific latest execution payload bid of the diff.
func (s *stateDiff) executionPayloadBidSSZ() interface {
	SizeSSZ() int
	MarshalSSZTo([]byte) ([]byte, error)
} {
	if s.targetVersion >= version.Heze {
		return s.latestExecutionPayloadBidHeze
	}
	return s.latestExecutionPayloadBid
}
