package depositsnapshot

import (
	"context"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestPrunePendingDeposits_ZeroMerkleIndex(t *testing.T) {
	dc := Cache{}

	dc.pendingDeposits = []*ethpb.DepositContainer{
		{Eth1BlockHeight: 2, Index: 2},
		{Eth1BlockHeight: 4, Index: 4},
		{Eth1BlockHeight: 6, Index: 6},
		{Eth1BlockHeight: 8, Index: 8},
		{Eth1BlockHeight: 10, Index: 10},
		{Eth1BlockHeight: 12, Index: 12},
	}

	dc.PrunePendingDeposits(context.Background(), 0)
	expected := []*ethpb.DepositContainer{
		{Eth1BlockHeight: 2, Index: 2},
		{Eth1BlockHeight: 4, Index: 4},
		{Eth1BlockHeight: 6, Index: 6},
		{Eth1BlockHeight: 8, Index: 8},
		{Eth1BlockHeight: 10, Index: 10},
		{Eth1BlockHeight: 12, Index: 12},
	}
	assert.DeepEqual(t, expected, dc.pendingDeposits)
}

func TestPrunePendingDeposits_OK(t *testing.T) {
	dc := Cache{}

	dc.pendingDeposits = []*ethpb.DepositContainer{
		{Eth1BlockHeight: 2, Index: 2},
		{Eth1BlockHeight: 4, Index: 4},
		{Eth1BlockHeight: 6, Index: 6},
		{Eth1BlockHeight: 8, Index: 8},
		{Eth1BlockHeight: 10, Index: 10},
		{Eth1BlockHeight: 12, Index: 12},
	}

	dc.PrunePendingDeposits(context.Background(), 6)
	expected := []*ethpb.DepositContainer{
		{Eth1BlockHeight: 6, Index: 6},
		{Eth1BlockHeight: 8, Index: 8},
		{Eth1BlockHeight: 10, Index: 10},
		{Eth1BlockHeight: 12, Index: 12},
	}

	assert.DeepEqual(t, expected, dc.pendingDeposits)

	dc.pendingDeposits = []*ethpb.DepositContainer{
		{Eth1BlockHeight: 2, Index: 2},
		{Eth1BlockHeight: 4, Index: 4},
		{Eth1BlockHeight: 6, Index: 6},
		{Eth1BlockHeight: 8, Index: 8},
		{Eth1BlockHeight: 10, Index: 10},
		{Eth1BlockHeight: 12, Index: 12},
	}

	dc.PrunePendingDeposits(context.Background(), 10)
	expected = []*ethpb.DepositContainer{
		{Eth1BlockHeight: 10, Index: 10},
		{Eth1BlockHeight: 12, Index: 12},
	}

	assert.DeepEqual(t, expected, dc.pendingDeposits)
}

func TestPruneAllPendingDeposits(t *testing.T) {
	dc := Cache{}

	dc.pendingDeposits = []*ethpb.DepositContainer{
		{Eth1BlockHeight: 2, Index: 2},
		{Eth1BlockHeight: 4, Index: 4},
		{Eth1BlockHeight: 6, Index: 6},
		{Eth1BlockHeight: 8, Index: 8},
		{Eth1BlockHeight: 10, Index: 10},
		{Eth1BlockHeight: 12, Index: 12},
	}

	dc.PruneAllPendingDeposits(context.Background())
	expected := []*ethpb.DepositContainer{}

	assert.DeepEqual(t, expected, dc.pendingDeposits)
}

func TestPruneProofs_Ok(t *testing.T) {
	dc, err := New()
	require.NoError(t, err)

	deposits := []struct {
		blkNum  uint64
		deposit *ethpb.Deposit
		index   int64
	}{
		{
			blkNum: 0,
			deposit: &ethpb.Deposit{Proof: makeDepositProof(),
				Data: &ethpb.Deposit_Data{PublicKey: bytesutil.PadTo([]byte("pk0"), 48)}},
			index: 0,
		},
		{
			blkNum: 0,
			deposit: &ethpb.Deposit{Proof: makeDepositProof(),
				Data: &ethpb.Deposit_Data{PublicKey: bytesutil.PadTo([]byte("pk1"), 48)}},
			index: 1,
		},
		{
			blkNum: 0,
			deposit: &ethpb.Deposit{Proof: makeDepositProof(),
				Data: &ethpb.Deposit_Data{PublicKey: bytesutil.PadTo([]byte("pk2"), 48)}},
			index: 2,
		},
		{
			blkNum: 0,
			deposit: &ethpb.Deposit{Proof: makeDepositProof(),
				Data: &ethpb.Deposit_Data{PublicKey: bytesutil.PadTo([]byte("pk3"), 48)}},
			index: 3,
		},
	}

	for _, ins := range deposits {
		assert.NoError(t, dc.InsertDeposit(context.Background(), ins.deposit, ins.blkNum, ins.index, [32]byte{}))
	}

	require.NoError(t, dc.PruneProofs(context.Background(), 1))

	assert.DeepEqual(t, [][]byte(nil), dc.deposits[0].Deposit.Proof)
	assert.DeepEqual(t, [][]byte(nil), dc.deposits[1].Deposit.Proof)
	assert.NotNil(t, dc.deposits[2].Deposit.Proof)
	assert.NotNil(t, dc.deposits[3].Deposit.Proof)
}

func TestPruneProofs_SomeAlreadyPruned(t *testing.T) {
	dc, err := New()
	require.NoError(t, err)

	deposits := []struct {
		blkNum  uint64
		deposit *ethpb.Deposit
		index   int64
	}{
		{
			blkNum: 0,
			deposit: &ethpb.Deposit{Proof: nil, Data: &ethpb.Deposit_Data{
				PublicKey: bytesutil.PadTo([]byte("pk0"), 48)}},
			index: 0,
		},
		{
			blkNum: 0,
			deposit: &ethpb.Deposit{Proof: nil, Data: &ethpb.Deposit_Data{
				PublicKey: bytesutil.PadTo([]byte("pk1"), 48)}}, index: 1,
		},
		{
			blkNum:  0,
			deposit: &ethpb.Deposit{Proof: makeDepositProof(), Data: &ethpb.Deposit_Data{PublicKey: bytesutil.PadTo([]byte("pk2"), 48)}},
			index:   2,
		},
		{
			blkNum: 0,
			deposit: &ethpb.Deposit{Proof: makeDepositProof(),
				Data: &ethpb.Deposit_Data{PublicKey: bytesutil.PadTo([]byte("pk3"), 48)}},
			index: 3,
		},
	}

	for _, ins := range deposits {
		assert.NoError(t, dc.InsertDeposit(context.Background(), ins.deposit, ins.blkNum, ins.index, [32]byte{}))
	}

	require.NoError(t, dc.PruneProofs(context.Background(), 2))

	assert.DeepEqual(t, [][]byte(nil), dc.deposits[2].Deposit.Proof)
}

func TestPruneProofs_PruneAllWhenDepositIndexTooBig(t *testing.T) {
	dc, err := New()
	require.NoError(t, err)

	deposits := []struct {
		blkNum  uint64
		deposit *ethpb.Deposit
		index   int64
	}{
		{
			blkNum: 0,
			deposit: &ethpb.Deposit{Proof: makeDepositProof(),
				Data: &ethpb.Deposit_Data{PublicKey: bytesutil.PadTo([]byte("pk0"), 48)}},
			index: 0,
		},
		{
			blkNum: 0,
			deposit: &ethpb.Deposit{Proof: makeDepositProof(),
				Data: &ethpb.Deposit_Data{PublicKey: bytesutil.PadTo([]byte("pk1"), 48)}},
			index: 1,
		},
		{
			blkNum: 0,
			deposit: &ethpb.Deposit{Proof: makeDepositProof(),
				Data: &ethpb.Deposit_Data{PublicKey: bytesutil.PadTo([]byte("pk2"), 48)}},
			index: 2,
		},
		{
			blkNum: 0,
			deposit: &ethpb.Deposit{Proof: makeDepositProof(),
				Data: &ethpb.Deposit_Data{PublicKey: bytesutil.PadTo([]byte("pk3"), 48)}},
			index: 3,
		},
	}

	for _, ins := range deposits {
		assert.NoError(t, dc.InsertDeposit(context.Background(), ins.deposit, ins.blkNum, ins.index, [32]byte{}))
	}

	require.NoError(t, dc.PruneProofs(context.Background(), 99))

	assert.DeepEqual(t, [][]byte(nil), dc.deposits[0].Deposit.Proof)
	assert.DeepEqual(t, [][]byte(nil), dc.deposits[1].Deposit.Proof)
	assert.DeepEqual(t, [][]byte(nil), dc.deposits[2].Deposit.Proof)
	assert.DeepEqual(t, [][]byte(nil), dc.deposits[3].Deposit.Proof)
}

func TestPruneProofs_CorrectlyHandleLastIndex(t *testing.T) {
	dc, err := New()
	require.NoError(t, err)

	deposits := []struct {
		blkNum  uint64
		deposit *ethpb.Deposit
		index   int64
	}{
		{
			blkNum: 0,
			deposit: &ethpb.Deposit{Proof: makeDepositProof(),
				Data: &ethpb.Deposit_Data{PublicKey: bytesutil.PadTo([]byte("pk0"), 48)}},
			index: 0,
		},
		{
			blkNum: 0,
			deposit: &ethpb.Deposit{Proof: makeDepositProof(),
				Data: &ethpb.Deposit_Data{PublicKey: bytesutil.PadTo([]byte("pk1"), 48)}},
			index: 1,
		},
		{
			blkNum: 0,
			deposit: &ethpb.Deposit{Proof: makeDepositProof(),
				Data: &ethpb.Deposit_Data{PublicKey: bytesutil.PadTo([]byte("pk2"), 48)}},
			index: 2,
		},
		{
			blkNum: 0,
			deposit: &ethpb.Deposit{Proof: makeDepositProof(),
				Data: &ethpb.Deposit_Data{PublicKey: bytesutil.PadTo([]byte("pk3"), 48)}},
			index: 3,
		},
	}

	for _, ins := range deposits {
		assert.NoError(t, dc.InsertDeposit(context.Background(), ins.deposit, ins.blkNum, ins.index, [32]byte{}))
	}

	require.NoError(t, dc.PruneProofs(context.Background(), 4))

	assert.DeepEqual(t, [][]byte(nil), dc.deposits[0].Deposit.Proof)
	assert.DeepEqual(t, [][]byte(nil), dc.deposits[1].Deposit.Proof)
	assert.DeepEqual(t, [][]byte(nil), dc.deposits[2].Deposit.Proof)
	assert.DeepEqual(t, [][]byte(nil), dc.deposits[3].Deposit.Proof)
}

func TestPruneAllProofs(t *testing.T) {
	dc, err := New()
	require.NoError(t, err)

	deposits := []struct {
		blkNum  uint64
		deposit *ethpb.Deposit
		index   int64
	}{
		{
			blkNum: 0,
			deposit: &ethpb.Deposit{Proof: makeDepositProof(),
				Data: &ethpb.Deposit_Data{PublicKey: bytesutil.PadTo([]byte("pk0"), 48)}},
			index: 0,
		},
		{
			blkNum: 0,
			deposit: &ethpb.Deposit{Proof: makeDepositProof(),
				Data: &ethpb.Deposit_Data{PublicKey: bytesutil.PadTo([]byte("pk1"), 48)}},
			index: 1,
		},
		{
			blkNum: 0,
			deposit: &ethpb.Deposit{Proof: makeDepositProof(),
				Data: &ethpb.Deposit_Data{PublicKey: bytesutil.PadTo([]byte("pk2"), 48)}},
			index: 2,
		},
		{
			blkNum: 0,
			deposit: &ethpb.Deposit{Proof: makeDepositProof(),
				Data: &ethpb.Deposit_Data{PublicKey: bytesutil.PadTo([]byte("pk3"), 48)}},
			index: 3,
		},
	}

	for _, ins := range deposits {
		assert.NoError(t, dc.InsertDeposit(context.Background(), ins.deposit, ins.blkNum, ins.index, [32]byte{}))
	}

	dc.PruneAllProofs(context.Background())

	assert.DeepEqual(t, [][]byte(nil), dc.deposits[0].Deposit.Proof)
	assert.DeepEqual(t, [][]byte(nil), dc.deposits[1].Deposit.Proof)
	assert.DeepEqual(t, [][]byte(nil), dc.deposits[2].Deposit.Proof)
	assert.DeepEqual(t, [][]byte(nil), dc.deposits[3].Deposit.Proof)
}
