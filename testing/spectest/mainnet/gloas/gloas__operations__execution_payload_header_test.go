package gloas

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/gloas/operations"
)

func TestMainnet_Gloas_Operations_ExecutionPayloadBid(t *testing.T) {
	operations.RunExecutionPayloadBidTest(t, "mainnet")
}
