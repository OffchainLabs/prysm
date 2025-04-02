package eth_test

import (
	"testing"

	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

func TestCopyEip7521Types_Fuzz(t *testing.T) {
	fuzzCopies(t, &eth.PendingDeposit{})
	fuzzCopies(t, &eth.PendingPartialWithdrawal{})
	fuzzCopies(t, &eth.PendingConsolidation{})
}
