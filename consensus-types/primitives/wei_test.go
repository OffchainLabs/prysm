package primitives_test

import (
	"math"
	"math/big"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestWeiStringer(t *testing.T) {
	require.Equal(t, "0", primitives.WeiToBigInt(primitives.ZeroWei()).String())
	require.Equal(t, "1234", primitives.WeiToBigInt(primitives.Uint64ToWei(1234)).String())
	require.Equal(t, "18446744073709551615", primitives.WeiToBigInt(primitives.Uint64ToWei(math.MaxUint64)).String())
}

func TestWeiToGwei(t *testing.T) {
	tests := []struct {
		v    *big.Int
		want primitives.Gwei
	}{
		{big.NewInt(1e9 - 1), 0},
		{big.NewInt(1e9), 1},
		{big.NewInt(1e10), 10},
		{big.NewInt(239489233849348394), 239489233},
	}
	for _, tt := range tests {
		if got := primitives.WeiToGwei(tt.v); got != tt.want {
			t.Errorf("WeiToGwei() = %v, want %v", got, tt.want)
		}
	}
}

func TestWeiToGwei_CopyOk(t *testing.T) {
	v := big.NewInt(1e9)
	got := primitives.WeiToGwei(v)

	require.Equal(t, primitives.Gwei(1), got)
	require.Equal(t, big.NewInt(1e9).Uint64(), v.Uint64())
}

func TestZero(t *testing.T) {
	z := primitives.ZeroWei()
	require.Equal(t, 0, big.NewInt(0).Cmp(z))
}
