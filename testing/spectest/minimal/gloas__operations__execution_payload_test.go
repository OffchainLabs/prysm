package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/gloas/operations"
)

func TestMinimal_Gloas_Operations_ExecutionPayloadEnvelope(t *testing.T) {
	t.Skip("gloas spec tests disabled until https://github.com/OffchainLabs/prysm/pull/16658")
	operations.RunExecutionPayloadTest(t, "minimal")
}
