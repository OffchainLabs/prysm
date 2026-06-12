package blocks_test

import (
	"testing"

	consensus_types "github.com/OffchainLabs/prysm/v7/consensus-types"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	enginev2 "github.com/OffchainLabs/prysm/v7/proto/engine/v2"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
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
