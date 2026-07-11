package internal

import (
	"context"
	"errors"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/assert"
)

func TestRequestStatus(t *testing.T) {
	assert.Equal(t, "timeout", requestStatus(context.DeadlineExceeded))
	assert.Equal(t, "error", requestStatus(errors.New("failed")))
}
