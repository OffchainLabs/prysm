package state_native

import (
	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

func (b *BeaconState) SetJustifiedCheckpointDecoupled(val *ethpb.CheckpointDecoupled) error {
	if b.version < version.Decoupled {
		return errNotSupported("SetJustifiedCheckpointDecoupled", b.version)
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	b.justifiedCheckpointDecoupled = val
	b.markFieldAsDirty(types.JustifiedCheckpointDecoupled)
	return nil
}

func (b *BeaconState) SetFinalizedCheckpointDecoupled(val *ethpb.CheckpointDecoupled) error {
	if b.version < version.Decoupled {
		return errNotSupported("SetFinalizedCheckpointDecoupled", b.version)
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	b.finalizedCheckpointDecoupled = val
	b.markFieldAsDirty(types.FinalizedCheckpointDecoupled)
	return nil
}

func (b *BeaconState) SetAvailableCommitteeWindow(val []*ethpb.AvailableCommittee) error {
	if b.version < version.Decoupled {
		return errNotSupported("SetAvailableCommitteeWindow", b.version)
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	b.sharedFieldReferences[types.AvailableCommitteeWindow].MinusRef()
	b.sharedFieldReferences[types.AvailableCommitteeWindow] = stateutil.NewRef(1)
	b.availableCommitteeWindow = val
	b.markFieldAsDirty(types.AvailableCommitteeWindow)
	b.rebuildTrie[types.AvailableCommitteeWindow] = true
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

func (b *BeaconState) SetCurrentHeight(val primitives.Height) error {
	if b.version < version.Decoupled {
		return errNotSupported("SetCurrentHeight", b.version)
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	b.currentHeight = val
	b.markFieldAsDirty(types.CurrentHeight)
	return nil
}

func (b *BeaconState) SetCurrentHeightNonjustifiable(val bool) error {
	if b.version < version.Decoupled {
		return errNotSupported("SetCurrentHeightNonjustifiable", b.version)
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	b.currentHeightNonjustifiable = val
	b.markFieldAsDirty(types.CurrentHeightNonjustifiable)
	return nil
}

func (b *BeaconState) SetCurrentHeightTarget(val *ethpb.CheckpointDecoupled) error {
	if b.version < version.Decoupled {
		return errNotSupported("SetCurrentHeightTarget", b.version)
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	b.currentHeightTarget = val
	b.markFieldAsDirty(types.CurrentHeightTarget)
	return nil
}

func (b *BeaconState) SetTargetParticipation(val bitfield.Bitlist) error {
	if b.version < version.Decoupled {
		return errNotSupported("SetTargetParticipation", b.version)
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	b.sharedFieldReferences[types.TargetParticipation].MinusRef()
	b.sharedFieldReferences[types.TargetParticipation] = stateutil.NewRef(1)
	b.targetParticipation = val
	b.markFieldAsDirty(types.TargetParticipation)
	b.rebuildTrie[types.TargetParticipation] = true
	return nil
}

func (b *BeaconState) SetProgress(val bitfield.Bitlist) error {
	if b.version < version.Decoupled {
		return errNotSupported("SetProgress", b.version)
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	b.sharedFieldReferences[types.Progress].MinusRef()
	b.sharedFieldReferences[types.Progress] = stateutil.NewRef(1)
	b.progress = val
	b.markFieldAsDirty(types.Progress)
	b.rebuildTrie[types.Progress] = true
	return nil
}

func (b *BeaconState) SetFinalityParticipation(val bitfield.Bitlist) error {
	if b.version < version.Decoupled {
		return errNotSupported("SetFinalityParticipation", b.version)
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	b.sharedFieldReferences[types.FinalityParticipation].MinusRef()
	b.sharedFieldReferences[types.FinalityParticipation] = stateutil.NewRef(1)
	b.finalityParticipation = val
	b.markFieldAsDirty(types.FinalityParticipation)
	b.rebuildTrie[types.FinalityParticipation] = true
	return nil
}

func (b *BeaconState) SetBuilderPendingPaymentsDecoupled(val []*ethpb.BuilderPendingPaymentDecoupled) error {
	if b.version < version.Decoupled {
		return errNotSupported("SetBuilderPendingPaymentsDecoupled", b.version)
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	b.builderPendingPaymentsDecoupled = val
	b.markFieldAsDirty(types.BuilderPendingPaymentsDecoupled)
	b.rebuildTrie[types.BuilderPendingPaymentsDecoupled] = true
	return nil
}
