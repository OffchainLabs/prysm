package utils

import (
	"fmt"
	"os"
	"testing"
)

// SkipGloasSuiteForEip8148 skips a whole Gloas spec test package. Call it from TestMain.
//
// Prysm ships EIP-8148 (custom sweep threshold) as part of Gloas. It adds
// `validator_sweep_thresholds` to the BeaconState and `sweep_thresholds` to
// ExecutionRequests. Upstream those land in a later fork, so the Gloas vectors are generated
// without them and every state root, SSZ round trip and block transition in the suite disagrees
// with this implementation by construction.
//
// TODO: Remove once the Gloas vectors carry the EIP-8148 fields, or once EIP-8148 moves to its
// own fork in Prysm.
func SkipGloasSuiteForEip8148(_ *testing.M) {
	fmt.Println("Skipping the Gloas spec tests: the vectors predate EIP-8148, which Prysm ships as part of Gloas")
	os.Exit(0)
}
