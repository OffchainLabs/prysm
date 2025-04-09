package validators_test

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/validators"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

func TestSlashingParamsPerVersion_NoErrors(t *testing.T) {
	for _, v := range version.All() {
		_, _, _, err := validators.SlashingParamsPerVersion(v)
		if err != nil {
			// If this test is failing, you need to add a case for the version in slashingParamsPerVersion.
			t.Errorf("Error occurred for version %d: %v", v, err)
		}
	}
}
