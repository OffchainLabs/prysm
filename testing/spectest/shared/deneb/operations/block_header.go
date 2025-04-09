package operations

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	common "github.com/prysmaticlabs/prysm/v5/testing/spectest/shared/common/operations"
)

func RunBlockHeaderTest(t *testing.T, config string) {
	common.RunBlockHeaderTest(t, config, version.String(version.Deneb), sszToBlock, sszToState)
}
