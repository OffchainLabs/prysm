package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/gloas/sanity"
)

func TestMainnet_Gloas_Sanity_Slots(t *testing.T) {
	t.Skip("gloas spec tests disabled until https://github.com/OffchainLabs/prysm/pull/16658")
	sanity.RunSlotProcessingTests(t, "mainnet")
}
