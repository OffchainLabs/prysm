package fork

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v6/testing/spectest/shared/altair/fork"
)

func TestMinimal_Altair_UpgradeToAltair(t *testing.T) {
	fork.RunUpgradeToAltair(t, "minimal")
}
