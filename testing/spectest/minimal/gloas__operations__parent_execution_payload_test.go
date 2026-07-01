package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/gloas/operations"
)

func TestMinimal_Gloas_Operations_ParentExecutionPayload(t *testing.T) {
	operations.RunParentExecutionPayloadTest(t, "minimal")
}
