package interop

import (
	"math/big"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestPremineGenesis_Electra(t *testing.T) {
	one := uint64(1)

	genesis := types.NewBlockWithHeader(&types.Header{
		Time:          uint64(time.Now().Unix()),
		Extra:         make([]byte, 32),
		BaseFee:       big.NewInt(1),
		ExcessBlobGas: &one,
		BlobGasUsed:   &one,
	})
	_, err := NewPreminedGenesis(t.Context(), time.Unix(int64(genesis.Time()), 0), 10, 10, version.Electra, genesis)
	require.NoError(t, err)
}
