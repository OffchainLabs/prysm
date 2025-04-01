package depositsnapshot

import (
	"context"
	"math/big"
	"testing"

	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
)

var _ PendingDepositsFetcher = (*Cache)(nil)

func TestInsertPendingDeposit_OK(t *testing.T) {
	dc := Cache{}
	dc.InsertPendingDeposit(context.Background(), &ethpb.Deposit{}, 111, 100, [32]byte{})

	assert.Equal(t, 1, len(dc.pendingDeposits), "deposit not inserted")
}

func TestInsertPendingDeposit_ignoresNilDeposit(t *testing.T) {
	dc := Cache{}
	dc.InsertPendingDeposit(context.Background(), nil /*deposit*/, 0 /*blockNum*/, 0, [32]byte{})

	assert.Equal(t, 0, len(dc.pendingDeposits))
}

func TestPendingDeposits_OK(t *testing.T) {
	dc := Cache{}

	dc.pendingDeposits = []*ethpb.DepositContainer{
		{Eth1BlockHeight: 2, Deposit: &ethpb.Deposit{Proof: [][]byte{[]byte("A")}}},
		{Eth1BlockHeight: 4, Deposit: &ethpb.Deposit{Proof: [][]byte{[]byte("B")}}},
		{Eth1BlockHeight: 6, Deposit: &ethpb.Deposit{Proof: [][]byte{[]byte("c")}}},
	}

	deposits := dc.PendingDeposits(context.Background(), big.NewInt(4))
	expected := []*ethpb.Deposit{
		{Proof: [][]byte{[]byte("A")}},
		{Proof: [][]byte{[]byte("B")}},
	}
	assert.DeepSSZEqual(t, expected, deposits)

	all := dc.PendingDeposits(context.Background(), nil)
	assert.Equal(t, len(dc.pendingDeposits), len(all), "PendingDeposits(ctx, nil) did not return all deposits")
}
