package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/bellatrix/sanity"
)

func TestMainnet_Bellatrix_Sanity_Slots(t *testing.T) {
	sanity.RunSlotProcessingTests(t, "mainnet")
}
