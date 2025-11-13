package operations

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/runtime/version"
	common "github.com/OffchainLabs/prysm/v7/testing/spectest/shared/common/operations"
)

func RunBlockHeaderTest(t *testing.T, config string) {
	common.RunBlockHeaderTest(t, config, version.String(version.Capella), sszToBlock, sszToState)
}
