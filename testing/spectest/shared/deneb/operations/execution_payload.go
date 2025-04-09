package operations

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	common "github.com/prysmaticlabs/prysm/v5/testing/spectest/shared/common/operations"
)

func RunExecutionPayloadTest(t *testing.T, config string) {
	common.RunExecutionPayloadTest(t, config, version.String(version.Deneb), sszToBlockBody, sszToState)
}
