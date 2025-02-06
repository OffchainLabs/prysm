package operations

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v6/runtime/version"
	common "github.com/prysmaticlabs/prysm/v6/testing/spectest/shared/common/operations"
)

func RunExecutionPayloadTest(t *testing.T, config string) {
	common.RunExecutionPayloadTest(t, config, version.String(version.Electra), sszToBlockBody, sszToState)
}
