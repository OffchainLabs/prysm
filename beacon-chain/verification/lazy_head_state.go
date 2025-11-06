package verification

import (
	"context"
	"sync"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/pkg/errors"
)

var errLazyProviderNotSet = errors.New("lazy head state provider not set")

type LazyHeadStateProvider struct {
	mu       sync.RWMutex
	provider HeadStateProvider
}

func NewLazyHeadStateProvider() *LazyHeadStateProvider {
	return &LazyHeadStateProvider{}
}

// HeadRoot delegates to the underlying provider's HeadRoot method.
func (p *LazyHeadStateProvider) HeadRoot(ctx context.Context) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.provider == nil {
		return nil, nil
	}
	return p.provider.HeadRoot(ctx)
}

// HeadSlot delegates to the underlying provider's HeadSlot method.
func (p *LazyHeadStateProvider) HeadSlot() primitives.Slot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.provider == nil {
		return 0
	}
	return p.provider.HeadSlot()
}

// HeadState delegates to the underlying provider's HeadState method.
func (p *LazyHeadStateProvider) HeadState(ctx context.Context) (state.BeaconState, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.provider == nil {
		return nil, errLazyProviderNotSet
	}
	return p.provider.HeadState(ctx)
}

// HeadStateReadOnly delegates to the underlying provider's HeadStateReadOnly method.
func (p *LazyHeadStateProvider) HeadStateReadOnly(ctx context.Context) (state.ReadOnlyBeaconState, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.provider == nil {
		return nil, errLazyProviderNotSet
	}
	return p.provider.HeadStateReadOnly(ctx)
}

// SetProvider sets the HeadStateProvider to be used by the SimplerLazyHeadStateProvider.
func (p *LazyHeadStateProvider) SetProvider(provider HeadStateProvider) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.provider = provider
}

var _ HeadStateProvider = &LazyHeadStateProvider{}
