package params

import (
	"math/bits"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

func (b *BeaconChainConfig) AttestationSubnetCountAtEpoch(epoch primitives.Epoch) uint64 {
	if epoch >= b.PQForkEpoch {
		return b.AttestationSubnetCountPQ
	}
	return b.AttestationSubnetCount
}

// Spec: compute_attestation_subnet_prefix_bits
func (b *BeaconChainConfig) AttestationSubnetPrefixBitsAtEpoch(epoch primitives.Epoch) uint64 {
	if epoch >= b.PQForkEpoch {
		return uint64(bits.Len64(b.AttestationSubnetCountPQ-1)) + b.AttestationSubnetExtraBits
	}
	return b.AttestationSubnetPrefixBits
}
