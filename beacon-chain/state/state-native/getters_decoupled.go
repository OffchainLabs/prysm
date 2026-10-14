package state_native

import (
	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

func (b *BeaconState) Height() (primitives.Height, error) {
	if b.version < version.Decoupled {
		return 0, errNotSupported("Height", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.height, nil
}

func (b *BeaconState) Entry() (*ethpb.CheckpointDecoupled, error) {
	if b.version < version.Decoupled {
		return nil, errNotSupported("Entry", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.entry, nil
}

func (b *BeaconState) TargetParticipation() (bitfield.Bitlist, error) {
	if b.version < version.Decoupled {
		return nil, errNotSupported("TargetParticipation", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.targetParticipation, nil
}

func (b *BeaconState) Progress() (bitfield.Bitlist, error) {
	if b.version < version.Decoupled {
		return nil, errNotSupported("Progress", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.progress, nil
}

func (b *BeaconState) JustifiedHeight() (primitives.Height, error) {
	if b.version < version.Decoupled {
		return 0, errNotSupported("JustifiedHeight", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.justifiedHeight, nil
}

func (b *BeaconState) Justified() (*ethpb.CheckpointDecoupled, error) {
	if b.version < version.Decoupled {
		return nil, errNotSupported("Justified", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.justified, nil
}

func (b *BeaconState) FinalizedHeight() (primitives.Height, error) {
	if b.version < version.Decoupled {
		return 0, errNotSupported("FinalizedHeight", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.finalizedHeight, nil
}

func (b *BeaconState) Finalized() (*ethpb.CheckpointDecoupled, error) {
	if b.version < version.Decoupled {
		return nil, errNotSupported("Finalized", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.finalized, nil
}

func (b *BeaconState) PendingFinality() ([]*ethpb.PendingFinality, error) {
	if b.version < version.Decoupled {
		return nil, errNotSupported("PendingFinality", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.pendingFinality, nil
}
