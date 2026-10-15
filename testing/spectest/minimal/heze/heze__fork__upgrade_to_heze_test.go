package heze

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/heze/fork"
)

func TestMinimal_UpgradeToHeze(t *testing.T) {
	fork.RunUpgradeToHeze(t, "minimal")
}
