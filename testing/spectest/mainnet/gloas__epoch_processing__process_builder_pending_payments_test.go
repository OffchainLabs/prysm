package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/gloas/epoch_processing"
)

func TestMainnet_Gloas_EpochProcessing_ProcessBuilderPendingPayments(t *testing.T) {
	epoch_processing.RunBuilderPendingPaymentsTests(t, "mainnet")
}
