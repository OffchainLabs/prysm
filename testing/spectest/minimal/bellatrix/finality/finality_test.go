package finality

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v6/testing/spectest/shared/bellatrix/finality"
)

func TestMinimal_Bellatrix_Finality(t *testing.T) {
	finality.RunFinalityTest(t, "minimal")
}
