package execution

import (
	"math/big"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum"
	"github.com/holiman/uint256"
)

func TestHeaderByHash_NotFound(t *testing.T) {
	srv := &Service{}
	srv.rpcClient = RPCClientBad{}

	_, err := srv.HeaderByHash(t.Context(), [32]byte{})
	assert.Equal(t, ethereum.NotFound, err)
}

func TestHeaderByNumber_NotFound(t *testing.T) {
	srv := &Service{}
	srv.rpcClient = RPCClientBad{}

	_, err := srv.HeaderByNumber(t.Context(), big.NewInt(100))
	assert.Equal(t, ethereum.NotFound, err)
}

func Test_tDStringToUint256(t *testing.T) {
	i, err := tDStringToUint256("0x0")
	require.NoError(t, err)
	require.DeepEqual(t, uint256.NewInt(0), i)

	i, err = tDStringToUint256("0x10000")
	require.NoError(t, err)
	require.DeepEqual(t, uint256.NewInt(65536), i)

	_, err = tDStringToUint256("100")
	require.ErrorContains(t, "hex string without 0x prefix", err)

	_, err = tDStringToUint256("0xzzzzzz")
	require.ErrorContains(t, "invalid hex string", err)

	_, err = tDStringToUint256("0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF" +
		"FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")
	require.ErrorContains(t, "hex number > 256 bits", err)
}

func TestToBlockNumArg(t *testing.T) {
	tests := []struct {
		name   string
		number *big.Int
		want   string
	}{
		{
			name:   "genesis",
			number: big.NewInt(0),
			want:   "0x0",
		},
		{
			name:   "near genesis block",
			number: big.NewInt(300),
			want:   "0x12c",
		},
		{
			name:   "current block",
			number: big.NewInt(15838075),
			want:   "0xf1ab7b",
		},
		{
			name:   "far off block",
			number: big.NewInt(12032894823020),
			want:   "0xaf1a06bea6c",
		},
		{
			name:   "latest block",
			number: nil,
			want:   "latest",
		},
		{
			name:   "pending block",
			number: big.NewInt(-1),
			want:   "pending",
		},
		{
			name:   "finalized block",
			number: big.NewInt(-3),
			want:   "finalized",
		},
		{
			name:   "safe block",
			number: big.NewInt(-4),
			want:   "safe",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toBlockNumArg(tt.number); got != tt.want {
				t.Errorf("toBlockNumArg() = %v, want %v", got, tt.want)
			}
		})
	}
}
