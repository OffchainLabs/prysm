package driver

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestParseGoMinorVersion(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"go1.21.5", 21},
		{"go1.21", 21},
		{"go1.22rc1", 22},
		{"go1.20.0", 20},
		{"go1.100.1", 100},
		{"devel +abc123", 0},
		{"go1.", 0},
		{"go1", 0},
		{"", 0},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			require.Equal(t, c.want, parseGoMinorVersion(c.in))
		})
	}
}
