package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/gloas/operations"
)

func TestMainnet_Gloas_Operations_Consolidation(t *testing.T) {
	operations.RunConsolidationTest(t, "mainnet")
}
