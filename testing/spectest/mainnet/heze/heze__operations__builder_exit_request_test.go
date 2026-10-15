package heze

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/heze/operations"
)

func TestMainnet_Heze_Operations_BuilderExitRequest(t *testing.T) {
	operations.RunBuilderExitRequestTest(t, "mainnet")
}
