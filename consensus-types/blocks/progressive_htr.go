package blocks

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	encodingssz "github.com/OffchainLabs/prysm/v7/encoding/ssz"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	fastssz "github.com/prysmaticlabs/fastssz"
)

func progressiveSSZEnabledForVersion(v int) bool {
	return v >= version.Gloas && features.Get().EnableProgressiveSSZ
}

func progressiveSSZEnabledForSlot(slot primitives.Slot) bool {
	return slots.ToEpoch(slot) >= params.BeaconConfig().GloasForkEpoch && features.Get().EnableProgressiveSSZ
}

func hashTreeRootForVersion(v int, rooter encodingssz.Hashable) ([32]byte, error) {
	if progressiveSSZEnabledForVersion(v) {
		if progressive, ok := rooter.(encodingssz.ProgressiveHashable); ok {
			return progressive.HashTreeRootProgressive()
		}
	}
	return rooter.HashTreeRoot()
}

func signingRootForSlot(slot primitives.Slot, rooter fastssz.HashRoot, domain []byte) ([32]byte, error) {
	if progressiveSSZEnabledForSlot(slot) {
		return signing.ComputeSigningRootProgressive(rooter, domain)
	}
	return signing.ComputeSigningRoot(rooter, domain)
}
