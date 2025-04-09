package slashings

import ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"

// IsSurround checks if an attestation, a, is surrounding
// another one, b, based on the Ethereum slashing conditions specified
// by @protolambda https://github.com/protolambda/eth2-surround#definition.
//
//	s: source
//	t: target
//
//	a surrounds b if: s_a < s_b and t_b < t_a
func IsSurround(a, b ethpb.IndexedAtt) bool {
	return a.GetData().Source.Epoch < b.GetData().Source.Epoch && b.GetData().Target.Epoch < a.GetData().Target.Epoch
}
