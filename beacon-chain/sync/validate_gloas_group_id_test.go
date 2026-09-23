package sync

import (
	"testing"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
)

func newGloasGroupCallbacks(t *testing.T, chain blockchainService) *partialColumnCallbacks {
	return &partialColumnCallbacks{service: &Service{
		cfg: &config{chain: chain},
		ctx: t.Context(),
	}}
}

func TestValidateGloasGroupID(t *testing.T) {
	root := [32]byte{'r', 'o', 'o', 't'}
	database := dbtest.SetupDB(t)
	seenChain := func() *mock.ChainService {
		return &mock.ChainService{DB: database, InitSyncBlockRoots: map[[32]byte]bool{root: true}}
	}

	t.Run("ignored when the chain service is unavailable", func(t *testing.T) {
		c := newGloasGroupCallbacks(t, nil)
		require.Equal(t, pubsub.ValidationIgnore, c.ValidateGloasGroupID(5, root))
	})

	t.Run("ignored when the block for the root has not been seen", func(t *testing.T) {
		// A mock chain with no DB reports HasBlock == false for every root.
		c := newGloasGroupCallbacks(t, &mock.ChainService{})
		require.Equal(t, pubsub.ValidationIgnore, c.ValidateGloasGroupID(5, root))
	})

	t.Run("ignored when the seen block is absent from forkchoice", func(t *testing.T) {
		// HasBlock reports true, but RecentBlockSlot errors because the block is not in forkchoice.
		chain := seenChain()
		chain.RecentBlockSlotErr = errors.New("block not in forkchoice")
		c := newGloasGroupCallbacks(t, chain)
		require.Equal(t, pubsub.ValidationIgnore, c.ValidateGloasGroupID(5, root))
	})

	t.Run("rejected when the block slot does not match the group slot", func(t *testing.T) {
		// The seen block is at slot 5, but the group claims slot 6.
		chain := seenChain()
		chain.BlockSlot = 5
		c := newGloasGroupCallbacks(t, chain)
		require.Equal(t, pubsub.ValidationReject, c.ValidateGloasGroupID(6, root))
	})

	t.Run("accepted when the block slot matches the group slot", func(t *testing.T) {
		chain := seenChain()
		chain.BlockSlot = 5
		c := newGloasGroupCallbacks(t, chain)
		require.Equal(t, pubsub.ValidationAccept, c.ValidateGloasGroupID(5, root))
	})
}
