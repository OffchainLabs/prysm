package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/phase0/sanity"
)

func TestMinimal_Phase0_Sanity_Blocks(t *testing.T) {
	sanity.RunBlockProcessingTest(t, "minimal", "sanity/blocks/pyspec_tests")
}
