package state

import "errors"

var (
	// ErrNilValidatorsInState returns when accessing validators in the state while the state has a
	// nil slice for the validators field.
	ErrNilValidatorsInState = errors.New("state has nil validator slice")
	// ErrNoPayloadCommitteeAvailable returns when the state cannot resolve the PTC for the requested slot.
	ErrNoPayloadCommitteeAvailable = errors.New("no payload committee available for slot")
)
