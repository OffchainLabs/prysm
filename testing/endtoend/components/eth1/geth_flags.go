package eth1

import "slices"

const rpcTxFeeCapFlag = "--rpc.txfeecap=0"

func withUnlimitedRPCTxFeeCap(args []string) []string {
	if slices.Contains(args, rpcTxFeeCapFlag) {
		return args
	}
	return append(args, rpcTxFeeCapFlag)
}
