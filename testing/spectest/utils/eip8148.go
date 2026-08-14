package utils

import "testing"

// SkipGloasEip8148Divergence skips a Gloas spec test that cannot pass while Prysm ships
// EIP-8148 as part of Gloas.
//
// EIP-8148 adds `validator_sweep_thresholds` to the BeaconState and `sweep_thresholds` to
// ExecutionRequests. Upstream those land in Heze, so the consensus-spec test vectors for
// Gloas are generated without them and every state root, SSZ round trip and block
// transition in the Gloas suite disagrees with this implementation by construction.
//
// TODO: remove this and re-enable the Gloas suite once EIP-8148 is scheduled upstream and
// the vectors carry the extra fields (i.e. when Prysm moves the feature to Heze).
func SkipGloasEip8148Divergence(t *testing.T) {
	t.Helper()
	t.Skip("Gloas spec vectors predate EIP-8148, which Prysm ships as part of Gloas; see utils.SkipGloasEip8148Divergence")
}
