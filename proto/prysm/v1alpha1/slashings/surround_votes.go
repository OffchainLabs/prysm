package slashings

import ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"

// IsSurround checks if an attestation, a, is surrounding
// another one, b, based on the Ethereum slashing conditions specified
// by @protolambda https://github.com/protolambda/eth2-surround#definition.
//
//	s: source
//	t: target
//
//	a surrounds b if: s_a < s_b and t_b < t_a
func IsSurround(a, b ethpb.IndexedAtt) bool {
	dataA := a.GetData()
	dataB := b.GetData()
	if dataA == nil || dataB == nil || dataA.Source == nil || dataB.Source == nil || dataA.Target == nil || dataB.Target == nil {
		return false
	}
	return dataA.Source.Epoch < dataB.Source.Epoch && dataB.Target.Epoch < dataA.Target.Epoch
}
