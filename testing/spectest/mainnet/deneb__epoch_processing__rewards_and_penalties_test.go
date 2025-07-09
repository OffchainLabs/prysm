package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/deneb/epoch_processing"
)

func TestMainnet_Deneb_EpochProcessing_RewardsAndPenalties(t *testing.T) {
	epoch_processing.RunRewardsAndPenaltiesTests(t, "mainnet")
}
