package operations

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/runtime/version"
	common "github.com/OffchainLabs/prysm/v7/testing/spectest/shared/common/operations"
)

func RunPayloadAttestationTest(t *testing.T, config string) {
	common.RunPayloadAttestationTest(t, config, version.String(version.Gloas), sszToState)
}
