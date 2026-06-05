package eth1

import (
	"math/big"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestDepositGasPriceUsesFloor(t *testing.T) {
	require.Equal(t, "1000000000000", depositGasPrice(big.NewInt(1e11)).String())
}

func TestDepositGasPriceBumpsSuggestedPrice(t *testing.T) {
	require.Equal(t, "3000000000000", depositGasPrice(big.NewInt(3e11)).String())
}
