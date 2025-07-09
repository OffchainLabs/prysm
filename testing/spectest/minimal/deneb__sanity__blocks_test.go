package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/deneb/sanity"
)

func TestMinimal_Deneb_Sanity_Blocks(t *testing.T) {
	sanity.RunBlockProcessingTest(t, "minimal", "sanity/blocks/pyspec_tests")
}
