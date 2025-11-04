package verification

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
)

// HeadStateGetter is a function type that retrieves a HeadStateProvider.
// This allows for lazy initialization of the head state provider.
type HeadStateGetter func() (HeadStateProvider, error)

// LazyHeadStateProvider wraps a HeadStateGetter to provide lazy access to head state methods.
// This is useful when the underlying head state provider (e.g., blockchain service) is not
// available at construction time but will be available when methods are called.
type LazyHeadStateProvider struct {
	getter HeadStateGetter
}

var _ HeadStateProvider = &LazyHeadStateProvider{}

// NewLazyHeadStateProvider creates a new LazyHeadStateProvider that uses the provided
// getter function to lazily retrieve the underlying HeadStateProvider.
func NewLazyHeadStateProvider(getter HeadStateGetter) *LazyHeadStateProvider {
	return &LazyHeadStateProvider{getter: getter}
}

// HeadRoot delegates to the underlying head state provider's HeadRoot method.
func (l *LazyHeadStateProvider) HeadRoot(ctx context.Context) ([]byte, error) {
	hsp, err := l.getter()
	if err != nil {
		return nil, err
	}
	return hsp.HeadRoot(ctx)
}

// HeadSlot delegates to the underlying head state provider's HeadSlot method.
func (l *LazyHeadStateProvider) HeadSlot() primitives.Slot {
	hsp, err := l.getter()
	if err != nil {
		return 0
	}
	return hsp.HeadSlot()
}

// HeadState delegates to the underlying head state provider's HeadState method.
func (l *LazyHeadStateProvider) HeadState(ctx context.Context) (state.BeaconState, error) {
	hsp, err := l.getter()
	if err != nil {
		return nil, err
	}
	return hsp.HeadState(ctx)
}

// HeadStateReadOnly delegates to the underlying head state provider's HeadStateReadOnly method.
func (l *LazyHeadStateProvider) HeadStateReadOnly(ctx context.Context) (state.ReadOnlyBeaconState, error) {
	hsp, err := l.getter()
	if err != nil {
		return nil, err
	}
	return hsp.HeadStateReadOnly(ctx)
}
