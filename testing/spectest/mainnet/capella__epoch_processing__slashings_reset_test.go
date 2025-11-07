package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/capella/epoch_processing"
)

func TestMainnet_Capella_EpochProcessing_SlashingsReset(t *testing.T) {
	epoch_processing.RunSlashingsResetTests(t, "mainnet")
}
