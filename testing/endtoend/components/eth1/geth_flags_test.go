package eth1

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestWithUnlimitedRPCTxFeeCapAppendsFlag(t *testing.T) {
	args := withUnlimitedRPCTxFeeCap([]string{"--http"})
	require.DeepEqual(t, []string{"--http", "--rpc.txfeecap=0"}, args)
}

func TestWithUnlimitedRPCTxFeeCapAvoidsDuplicates(t *testing.T) {
	args := withUnlimitedRPCTxFeeCap([]string{"--http", "--rpc.txfeecap=0"})
	require.DeepEqual(t, []string{"--http", "--rpc.txfeecap=0"}, args)
}
