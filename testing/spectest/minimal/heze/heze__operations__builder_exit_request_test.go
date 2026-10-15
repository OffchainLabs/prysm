package heze

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/shared/heze/operations"
)

func TestMinimal_Heze_Operations_BuilderExitRequest(t *testing.T) {
	operations.RunBuilderExitRequestTest(t, "minimal")
}
