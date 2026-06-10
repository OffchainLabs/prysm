package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/gloas/sanity"
)

func TestMinimal_Gloas_Random(t *testing.T) {
	sanity.RunBlockProcessingTest(t, "minimal", "random/random/pyspec_tests")
}
