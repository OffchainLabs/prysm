package cache

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

// RegistrationCache is used to store the cached results of an Validator Registration request.
// beacon api /eth/v1/validator/register_validator
type RegistrationCache struct {
	indexToRegistration map[primitives.ValidatorIndex]*ethpb.ValidatorRegistrationV1
	lock                sync.RWMutex
}

// NewRegistrationCache initializes the map and underlying cache.
func NewRegistrationCache() *RegistrationCache {
	return &RegistrationCache{
		indexToRegistration: make(map[primitives.ValidatorIndex]*ethpb.ValidatorRegistrationV1),
		lock:                sync.RWMutex{},
	}
}

// RegistrationByIndex returns the registration by index in the cache and also removes items in the cache if expired.
func (regCache *RegistrationCache) RegistrationByIndex(id primitives.ValidatorIndex) (*ethpb.ValidatorRegistrationV1, error) {
	regCache.lock.RLock()
	v, ok := regCache.indexToRegistration[id]
	if !ok {
		regCache.lock.RUnlock()
		return nil, errors.Wrapf(ErrNotFoundRegistration, "validator id %d", id)
	}
	regCache.lock.RUnlock()
	return v, nil
}

// UpdateIndexToRegisteredMap adds or updates values in the cache based on the argument.
func (regCache *RegistrationCache) UpdateIndexToRegisteredMap(ctx context.Context, m map[primitives.ValidatorIndex]*ethpb.ValidatorRegistrationV1) {
	_, span := trace.StartSpan(ctx, "RegistrationCache.UpdateIndexToRegisteredMap")
	defer span.End()
	regCache.lock.Lock()
	defer regCache.lock.Unlock()
	for key, value := range m {
		regCache.indexToRegistration[key] = &ethpb.ValidatorRegistrationV1{
			Pubkey:       bytesutil.SafeCopyBytes(value.Pubkey),
			FeeRecipient: bytesutil.SafeCopyBytes(value.FeeRecipient),
			GasLimit:     value.GasLimit,
			Timestamp:    value.Timestamp,
		}
	}
}
