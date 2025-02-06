package cache

import (
	"sync"

	"github.com/prysmaticlabs/prysm/v6/consensus-types/primitives"
)

type TrackedValidator struct {
	Active       bool
	FeeRecipient primitives.ExecutionAddress
	Index        primitives.ValidatorIndex
}

type TrackedValidatorsCache struct {
	sync.Mutex
	trackedValidators map[primitives.ValidatorIndex]TrackedValidator
}

func NewTrackedValidatorsCache() *TrackedValidatorsCache {
	return &TrackedValidatorsCache{
		trackedValidators: make(map[primitives.ValidatorIndex]TrackedValidator),
	}
}

func (t *TrackedValidatorsCache) Validator(index primitives.ValidatorIndex) (TrackedValidator, bool) {
	t.Lock()
	defer t.Unlock()
	val, ok := t.trackedValidators[index]
	return val, ok
}

func (t *TrackedValidatorsCache) Set(val TrackedValidator) {
	t.Lock()
	defer t.Unlock()
	t.trackedValidators[val.Index] = val
}

func (t *TrackedValidatorsCache) Prune() {
	t.Lock()
	defer t.Unlock()
	t.trackedValidators = make(map[primitives.ValidatorIndex]TrackedValidator)
}

func (t *TrackedValidatorsCache) Validating() bool {
	t.Lock()
	defer t.Unlock()
	return len(t.trackedValidators) > 0
}
