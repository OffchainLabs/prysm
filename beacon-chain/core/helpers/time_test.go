package helpers_test

import (
	"context"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
)

func TestSleep(t *testing.T) {
	t.Run("context cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		helpers.Sleep(ctx, 1*time.Hour)
	})

	t.Run("Nominal", func(t *testing.T) {
		helpers.Sleep(t.Context(), 0)
	})
}
