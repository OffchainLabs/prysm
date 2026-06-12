package blocks_test

import (
	"testing"

	consensus_types "github.com/OffchainLabs/prysm/v7/consensus-types"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	enginev2 "github.com/OffchainLabs/prysm/v7/proto/engine/v2"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestWrappedExecutionPayloadBodyFulu(t *testing.T) {
	tx := []byte{0xde, 0xad}
	wd := []*enginev1.Withdrawal{{Index: 7}}
	b, err := blocks.WrappedExecutionPayloadBodyFulu(&enginev2.ExecutionPayloadBodyFulu{
		Transactions: [][]byte{tx},
		Withdrawals:  wd,
	})
	require.NoError(t, err)
	require.Equal(t, false, b.IsNil())

	txs, err := b.Transactions()
	require.NoError(t, err)
	assert.DeepEqual(t, [][]byte{tx}, txs)
	gotWd, err := b.Withdrawals()
	require.NoError(t, err)
	assert.DeepEqual(t, wd, gotWd)

	// Fulu has no block access list.
	_, err = b.BlockAccessList()
	require.ErrorIs(t, err, consensus_types.ErrUnsupportedField)

	_, err = blocks.WrappedExecutionPayloadBodyFulu(nil)
	require.ErrorIs(t, err, consensus_types.ErrNilObjectWrapped)
}

func TestWrappedExecutionPayloadBodyGloas(t *testing.T) {
	tx := []byte{0xbe, 0xef}
	wd := []*enginev1.Withdrawal{{Index: 9}}
	bal := []byte{0x01, 0x02, 0x03}
	b, err := blocks.WrappedExecutionPayloadBodyGloas(&enginev2.ExecutionPayloadBodyGloas{
		Transactions:    [][]byte{tx},
		Withdrawals:     wd,
		BlockAccessList: bal,
	})
	require.NoError(t, err)
	require.Equal(t, false, b.IsNil())

	txs, err := b.Transactions()
	require.NoError(t, err)
	assert.DeepEqual(t, [][]byte{tx}, txs)
	gotWd, err := b.Withdrawals()
	require.NoError(t, err)
	assert.DeepEqual(t, wd, gotWd)

	// Gloas preserves the block access list (unlike Fulu).
	gotBAL, err := b.BlockAccessList()
	require.NoError(t, err)
	assert.DeepEqual(t, bal, gotBAL)

	_, err = blocks.WrappedExecutionPayloadBodyGloas(nil)
	require.ErrorIs(t, err, consensus_types.ErrNilObjectWrapped)
}

func TestWrappedExecutionPayloadBodyV1JSON(t *testing.T) {
	tx := hexutil.Bytes{0xde, 0xad}
	wd := []*enginev1.Withdrawal{{Index: 7}}
	b, err := blocks.WrappedExecutionPayloadBodyV1JSON(&enginev1.ExecutionPayloadBody{
		Transactions: []hexutil.Bytes{tx},
		Withdrawals:  wd,
	})
	require.NoError(t, err)
	require.Equal(t, false, b.IsNil())

	txs, err := b.Transactions()
	require.NoError(t, err)
	assert.DeepEqual(t, [][]byte{[]byte(tx)}, txs)
	gotWd, err := b.Withdrawals()
	require.NoError(t, err)
	assert.DeepEqual(t, wd, gotWd)

	// The V1 JSON body has no block access list.
	_, err = b.BlockAccessList()
	require.ErrorIs(t, err, consensus_types.ErrUnsupportedField)

	_, err = blocks.WrappedExecutionPayloadBodyV1JSON(nil)
	require.ErrorIs(t, err, consensus_types.ErrNilObjectWrapped)
}

func TestWrappedExecutionPayloadBodyV2JSON(t *testing.T) {
	tx := hexutil.Bytes{0xbe, 0xef}
	wd := []*enginev1.Withdrawal{{Index: 9}}
	bal := hexutil.Bytes{0x01, 0x02, 0x03}
	b, err := blocks.WrappedExecutionPayloadBodyV2JSON(&enginev1.ExecutionPayloadBodyV2{
		Transactions:    []hexutil.Bytes{tx},
		Withdrawals:     wd,
		BlockAccessList: &bal,
	})
	require.NoError(t, err)
	require.Equal(t, false, b.IsNil())

	txs, err := b.Transactions()
	require.NoError(t, err)
	assert.DeepEqual(t, [][]byte{[]byte(tx)}, txs)
	gotWd, err := b.Withdrawals()
	require.NoError(t, err)
	assert.DeepEqual(t, wd, gotWd)

	// The V2 JSON body carries the block access list.
	gotBAL, err := b.BlockAccessList()
	require.NoError(t, err)
	assert.DeepEqual(t, []byte(bal), gotBAL)

	// A nil block access list yields no error and no bytes.
	noBAL, err := blocks.WrappedExecutionPayloadBodyV2JSON(&enginev1.ExecutionPayloadBodyV2{})
	require.NoError(t, err)
	gotNil, err := noBAL.BlockAccessList()
	require.NoError(t, err)
	require.IsNil(t, gotNil)

	_, err = blocks.WrappedExecutionPayloadBodyV2JSON(nil)
	require.ErrorIs(t, err, consensus_types.ErrNilObjectWrapped)
}
