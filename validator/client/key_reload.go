package client

import (
	"context"

	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
)

// HandleKeyReload makes sure the validator keeps operating correctly after a change to the underlying keys.
// It is also responsible for logging out information about the new state of keys.
func (v *validator) HandleKeyReload(ctx context.Context, currentKeys [][fieldparams.BLSPubkeyLength]byte) (bool, error) {
	ctx, span := trace.StartSpan(ctx, "validator.HandleKeyReload")
	defer span.End()
	if err := v.updateValidatorStatusCache(ctx, currentKeys); err != nil {
		return false, err
	}

	return v.checkAndLogValidatorStatus(), nil
}
