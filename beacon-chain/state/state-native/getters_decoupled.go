package state_native

import (
	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

func (b *BeaconState) JustifiedCheckpointDecoupled() (*ethpb.CheckpointDecoupled, error) {
	if b.version < version.Decoupled {
		return nil, errNotSupported("JustifiedCheckpointDecoupled", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.justifiedCheckpointDecoupled, nil
}

func (b *BeaconState) FinalizedCheckpointDecoupled() (*ethpb.CheckpointDecoupled, error) {
	if b.version < version.Decoupled {
		return nil, errNotSupported("FinalizedCheckpointDecoupled", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.finalizedCheckpointDecoupled, nil
}

func (b *BeaconState) AvailableCommitteeWindow() ([]*ethpb.AvailableCommittee, error) {
	if b.version < version.Decoupled {
		return nil, errNotSupported("AvailableCommitteeWindow", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.availableCommitteeWindow, nil
}

func (b *BeaconState) JustifiedHeight() (primitives.Height, error) {
	if b.version < version.Decoupled {
		return 0, errNotSupported("JustifiedHeight", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.justifiedHeight, nil
}

func (b *BeaconState) FinalizedHeight() (primitives.Height, error) {
	if b.version < version.Decoupled {
		return 0, errNotSupported("FinalizedHeight", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.finalizedHeight, nil
}

func (b *BeaconState) CurrentHeight() (primitives.Height, error) {
	if b.version < version.Decoupled {
		return 0, errNotSupported("CurrentHeight", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.currentHeight, nil
}

func (b *BeaconState) CurrentHeightNonjustifiable() (bool, error) {
	if b.version < version.Decoupled {
		return false, errNotSupported("CurrentHeightNonjustifiable", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.currentHeightNonjustifiable, nil
}

func (b *BeaconState) CurrentHeightTarget() (*ethpb.CheckpointDecoupled, error) {
	if b.version < version.Decoupled {
		return nil, errNotSupported("CurrentHeightTarget", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.currentHeightTarget, nil
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

func (b *BeaconState) FinalityParticipation() (bitfield.Bitlist, error) {
	if b.version < version.Decoupled {
		return nil, errNotSupported("FinalityParticipation", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.finalityParticipation, nil
}

func (b *BeaconState) BuilderPendingPaymentsDecoupled() ([]*ethpb.BuilderPendingPaymentDecoupled, error) {
	if b.version < version.Decoupled {
		return nil, errNotSupported("BuilderPendingPaymentsDecoupled", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.builderPendingPaymentsDecoupled, nil
}
