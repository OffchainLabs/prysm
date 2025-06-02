package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/electra/epoch_processing"
)

func TestMainnet_Electra_EpochProcessing_PendingDeposits(t *testing.T) {
	epoch_processing.RunPendingDepositsTests(t, "mainnet")
}
