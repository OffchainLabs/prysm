package cache

import "github.com/pkg/errors"

var (
	// ErrNilValueProvided for when we try to put a nil value in a cache.
	ErrNilValueProvided = errors.New("nil value provided on Put()")
	// ErrIncorrectType for when the state is of the incorrect type.
	ErrIncorrectType = errors.New("incorrect state type provided")
	// ErrNotFound for cache fetches that return a nil value.
	ErrNotFound = errors.New("not found in cache")
	// ErrNonExistingSyncCommitteeKey when sync committee key (root) does not exist in cache.
	ErrNonExistingSyncCommitteeKey   = errors.New("does not exist sync committee key")
	errNotSyncCommitteeIndexPosition = errors.New("not syncCommitteeIndexPosition struct")
	// ErrNotFoundRegistration when validator registration does not exist in cache.
	ErrNotFoundRegistration = errors.Wrap(ErrNotFound, "no validator registered")

	// ErrAlreadyInProgress appears when attempting to mark a cache as in progress while it is
	// already in progress. The client should handle this error and wait for the in progress
	// data to resolve via Get.
	ErrAlreadyInProgress = errors.New("already in progress")
)
