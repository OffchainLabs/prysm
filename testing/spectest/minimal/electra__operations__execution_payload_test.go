package minimal

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/electra/operations"
)

func TestMinimal_Electra_Operations_PayloadExecution(t *testing.T) {
	operations.RunExecutionPayloadTest(t, "minimal")
}
