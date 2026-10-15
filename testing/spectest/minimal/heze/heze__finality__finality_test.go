package heze

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/heze/finality"
)

func TestMinimal_Heze_Finality(t *testing.T) {
	finality.RunFinalityTest(t, "minimal")
}
