package time

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestPrettyMinMaxAverage(t *testing.T) {
	require.Equal(t, "[min: 1ms, avg: 2.5ms, max: 4s]", PrettyMinMaxAverage(time.Millisecond, 4*time.Second, 2500*time.Microsecond))
}
