package heze

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/heze/operations"
)

func TestMinimal_Heze_Operations_ParentExecutionPayload(t *testing.T) {
	operations.RunParentExecutionPayloadTest(t, "minimal")
}
