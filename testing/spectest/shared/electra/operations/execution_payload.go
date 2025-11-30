package operations

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/runtime/version"
	common "github.com/OffchainLabs/prysm/v7/testing/spectest/shared/common/operations"
)

func RunExecutionPayloadTest(t *testing.T, config string) {
	common.RunExecutionPayloadTest(t, config, version.String(version.Electra), sszToBlockBody, sszToState)
}
