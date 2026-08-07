package httputil

import (
	"errors"
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/assert"
)

func TestDefaultJsonError_Is(t *testing.T) {
	err := &DefaultJsonError{Code: 404, Message: "not found"}

	t.Run("matches another DefaultJsonError with the same code", func(t *testing.T) {
		// The message is ignored; only the code is compared.
		assert.Equal(t, true, err.Is(&DefaultJsonError{Code: 404, Message: "other"}))
	})

	t.Run("does not match a different code", func(t *testing.T) {
		assert.Equal(t, false, err.Is(&DefaultJsonError{Code: 500}))
	})

	t.Run("does not match a non-DefaultJsonError", func(t *testing.T) {
		assert.Equal(t, false, err.Is(errors.New("plain error")))
	})

	t.Run("matches through a wrapped error via errors.Is", func(t *testing.T) {
		wrapped := fmt.Errorf("context: %w", &DefaultJsonError{Code: 404})
		assert.Equal(t, true, errors.Is(wrapped, &DefaultJsonError{Code: 404}))
		assert.Equal(t, false, errors.Is(wrapped, &DefaultJsonError{Code: 400}))
	})
}
