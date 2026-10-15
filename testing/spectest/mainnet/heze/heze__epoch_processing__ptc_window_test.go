package heze

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/heze/epoch_processing"
)

func TestMainnet_Heze_EpochProcessing_PTCWindow(t *testing.T) {
	epoch_processing.RunPTCWindowTests(t, "mainnet")
}
