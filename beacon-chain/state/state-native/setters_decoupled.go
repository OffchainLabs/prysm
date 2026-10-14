package state_native

import (
	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

func (b *BeaconState) SetHeight(val primitives.Height) error {
	if b.version < version.Decoupled {
		return errNotSupported("SetHeight", b.version)
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	b.height = val
	b.markFieldAsDirty(types.Height)
	return nil
}

func (b *BeaconState) SetEntry(val *ethpb.CheckpointDecoupled) error {
	if b.version < version.Decoupled {
		return errNotSupported("SetEntry", b.version)
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	b.entry = val
	b.markFieldAsDirty(types.Entry)
	return nil
}

func (b *BeaconState) SetTargetParticipation(val bitfield.Bitlist) error {
	if b.version < version.Decoupled {
		return errNotSupported("SetTargetParticipation", b.version)
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	b.targetParticipation = val
	b.markFieldAsDirty(types.TargetParticipation)
	return nil
}

func (b *BeaconState) SetProgress(val bitfield.Bitlist) error {
	if b.version < version.Decoupled {
		return errNotSupported("SetProgress", b.version)
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	b.progress = val
	b.markFieldAsDirty(types.Progress)
	return nil
}

func (b *BeaconState) SetJustifiedHeight(val primitives.Height) error {
	if b.version < version.Decoupled {
		return errNotSupported("SetJustifiedHeight", b.version)
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	b.justifiedHeight = val
	b.markFieldAsDirty(types.JustifiedHeight)
	return nil
}

func (b *BeaconState) SetJustified(val *ethpb.CheckpointDecoupled) error {
	if b.version < version.Decoupled {
		return errNotSupported("SetJustified", b.version)
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	b.justified = val
	b.markFieldAsDirty(types.Justified)
	return nil
}

func (b *BeaconState) SetFinalizedHeight(val primitives.Height) error {
	if b.version < version.Decoupled {
		return errNotSupported("SetFinalizedHeight", b.version)
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	b.finalizedHeight = val
	b.markFieldAsDirty(types.FinalizedHeight)
	return nil
}

func (b *BeaconState) SetFinalized(val *ethpb.CheckpointDecoupled) error {
	if b.version < version.Decoupled {
		return errNotSupported("SetFinalized", b.version)
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	b.finalized = val
	b.markFieldAsDirty(types.Finalized)
	return nil
}

func (b *BeaconState) SetPendingFinality(val []*ethpb.PendingFinality) error {
	if b.version < version.Decoupled {
		return errNotSupported("SetPendingFinality", b.version)
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	b.pendingFinality = val
	b.markFieldAsDirty(types.PendingFinality)
	return nil
}
