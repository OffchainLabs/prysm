package kv

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/prysmaticlabs/prysm/v5/validator/db/common"
)

func TestNewProposalHistoryForSlot_ReturnsNilIfNoHistory(t *testing.T) {
	valPubkey := [fieldparams.BLSPubkeyLength]byte{1, 2, 3}
	db := setupDB(t, [][fieldparams.BLSPubkeyLength]byte{})

	_, proposalExists, signingRootExists, err := db.ProposalHistoryForSlot(context.Background(), valPubkey, 0)
	require.NoError(t, err)
	assert.Equal(t, false, proposalExists)
	assert.Equal(t, false, signingRootExists)
}

func TestProposalHistoryForSlot_InitializesNewPubKeys(t *testing.T) {
	pubkeys := [][fieldparams.BLSPubkeyLength]byte{{30}, {25}, {20}}
	db := setupDB(t, pubkeys)

	for _, pub := range pubkeys {
		_, proposalExists, signingRootExists, err := db.ProposalHistoryForSlot(context.Background(), pub, 0)
		require.NoError(t, err)
		assert.Equal(t, false, proposalExists)
		assert.Equal(t, false, signingRootExists)
	}
}

func TestNewProposalHistoryForSlot_SigningRootNil(t *testing.T) {
	pubkey := [fieldparams.BLSPubkeyLength]byte{1, 2, 3}
	slot := primitives.Slot(2)

	db := setupDB(t, [][fieldparams.BLSPubkeyLength]byte{})

	err := db.SaveProposalHistoryForSlot(context.Background(), pubkey, slot, nil)
	require.NoError(t, err, "Saving proposal history failed: %v")

	_, proposalExists, signingRootExists, err := db.ProposalHistoryForSlot(context.Background(), pubkey, slot)
	require.NoError(t, err)
	assert.Equal(t, true, proposalExists)
	assert.Equal(t, false, signingRootExists)
}

func TestSaveProposalHistoryForSlot_OK(t *testing.T) {
	pubkey := [fieldparams.BLSPubkeyLength]byte{3}
	db := setupDB(t, [][fieldparams.BLSPubkeyLength]byte{pubkey})

	slot := primitives.Slot(2)

	err := db.SaveProposalHistoryForSlot(context.Background(), pubkey, slot, []byte{1})
	require.NoError(t, err, "Saving proposal history failed: %v")
	signingRoot, proposalExists, signingRootExists, err := db.ProposalHistoryForSlot(context.Background(), pubkey, slot)
	require.NoError(t, err, "Failed to get proposal history")
	assert.Equal(t, true, proposalExists)
	assert.Equal(t, true, signingRootExists)

	require.NotNil(t, signingRoot)
	require.DeepEqual(t, bytesutil.PadTo([]byte{1}, 32), signingRoot[:], "Expected DB to keep object the same")
}

func TestNewProposalHistoryForPubKey_ReturnsEmptyIfNoHistory(t *testing.T) {
	valPubkey := [fieldparams.BLSPubkeyLength]byte{1, 2, 3}
	db := setupDB(t, [][fieldparams.BLSPubkeyLength]byte{})

	proposalHistory, err := db.ProposalHistoryForPubKey(context.Background(), valPubkey)
	require.NoError(t, err)
	assert.DeepEqual(t, make([]*common.Proposal, 0), proposalHistory)
}

func TestSaveProposalHistoryForPubKey_OK(t *testing.T) {
	pubkey := [fieldparams.BLSPubkeyLength]byte{3}
	db := setupDB(t, [][fieldparams.BLSPubkeyLength]byte{pubkey})

	slot := primitives.Slot(2)

	root := [32]byte{1}
	err := db.SaveProposalHistoryForSlot(context.Background(), pubkey, slot, root[:])
	require.NoError(t, err, "Saving proposal history failed: %v")
	proposalHistory, err := db.ProposalHistoryForPubKey(context.Background(), pubkey)
	require.NoError(t, err, "Failed to get proposal history")

	require.NotNil(t, proposalHistory)
	want := []*common.Proposal{
		{
			Slot:        slot,
			SigningRoot: root[:],
		},
	}
	require.DeepEqual(t, want[0], proposalHistory[0])
}

func TestSaveProposalHistoryForSlot_Overwrites(t *testing.T) {
	pubkey := [fieldparams.BLSPubkeyLength]byte{0}
	tests := []struct {
		signingRoot []byte
	}{
		{
			signingRoot: bytesutil.PadTo([]byte{1}, 32),
		},
		{
			signingRoot: bytesutil.PadTo([]byte{2}, 32),
		},
		{
			signingRoot: bytesutil.PadTo([]byte{3}, 32),
		},
	}

	for _, tt := range tests {
		db := setupDB(t, [][fieldparams.BLSPubkeyLength]byte{pubkey})
		err := db.SaveProposalHistoryForSlot(context.Background(), pubkey, 0, tt.signingRoot)
		require.NoError(t, err, "Saving proposal history failed")
		proposalHistory, err := db.ProposalHistoryForPubKey(context.Background(), pubkey)
		require.NoError(t, err, "Failed to get proposal history")

		require.NotNil(t, proposalHistory)
		require.DeepEqual(t, tt.signingRoot, proposalHistory[0].SigningRoot, "Expected DB to keep object the same")
		require.NoError(t, db.Close(), "Failed to close database")
	}
}

func TestPruneProposalHistoryBySlot_OK(t *testing.T) {
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	wsPeriod := params.BeaconConfig().WeakSubjectivityPeriod
	pubKey := [fieldparams.BLSPubkeyLength]byte{0}
	tests := []struct {
		slots        []primitives.Slot
		storedSlots  []primitives.Slot
		removedSlots []primitives.Slot
	}{
		{
			// Go 2 epochs past pruning point.
			slots:        []primitives.Slot{slotsPerEpoch / 2, slotsPerEpoch*5 + 6, slotsPerEpoch.Mul(uint64(wsPeriod+3)) + 8},
			storedSlots:  []primitives.Slot{slotsPerEpoch*5 + 6, slotsPerEpoch.Mul(uint64(wsPeriod+3)) + 8},
			removedSlots: []primitives.Slot{slotsPerEpoch / 2},
		},
		{
			// Go 10 epochs past pruning point.
			slots: []primitives.Slot{
				slotsPerEpoch + 4,
				slotsPerEpoch * 2,
				slotsPerEpoch * 3,
				slotsPerEpoch * 4,
				slotsPerEpoch * 5,
				slotsPerEpoch.Mul(uint64(wsPeriod+10)) + 8,
			},
			storedSlots: []primitives.Slot{slotsPerEpoch.Mul(uint64(wsPeriod+10)) + 8},
			removedSlots: []primitives.Slot{
				slotsPerEpoch + 4,
				slotsPerEpoch * 2,
				slotsPerEpoch * 3,
				slotsPerEpoch * 4,
				slotsPerEpoch * 5,
			},
		},
		{
			// Prune none.
			slots:       []primitives.Slot{slotsPerEpoch + 4, slotsPerEpoch*2 + 3, slotsPerEpoch*3 + 4, slotsPerEpoch*4 + 3, slotsPerEpoch*5 + 3},
			storedSlots: []primitives.Slot{slotsPerEpoch + 4, slotsPerEpoch*2 + 3, slotsPerEpoch*3 + 4, slotsPerEpoch*4 + 3, slotsPerEpoch*5 + 3},
		},
	}
	signedRoot := bytesutil.PadTo([]byte{1}, 32)

	for _, tt := range tests {
		db := setupDB(t, [][fieldparams.BLSPubkeyLength]byte{pubKey})
		for _, slot := range tt.slots {
			err := db.SaveProposalHistoryForSlot(context.Background(), pubKey, slot, signedRoot)
			require.NoError(t, err, "Saving proposal history failed")
		}

		signingRootsBySlot := make(map[primitives.Slot][]byte)
		proposalHistory, err := db.ProposalHistoryForPubKey(context.Background(), pubKey)
		require.NoError(t, err)

		for _, hist := range proposalHistory {
			signingRootsBySlot[hist.Slot] = hist.SigningRoot
		}

		for _, slot := range tt.removedSlots {
			_, ok := signingRootsBySlot[slot]
			require.Equal(t, false, ok)
		}
		for _, slot := range tt.storedSlots {
			root, ok := signingRootsBySlot[slot]
			require.Equal(t, true, ok)
			require.DeepEqual(t, signedRoot, root, "Unexpected difference in bytes for epoch %d", slot)
		}
		require.NoError(t, db.Close(), "Failed to close database")
	}
}

func TestStore_ProposedPublicKeys(t *testing.T) {
	ctx := context.Background()
	validatorDB, err := NewKVStore(ctx, t.TempDir(), &Config{})
	require.NoError(t, err, "Failed to instantiate DB")
	t.Cleanup(func() {
		require.NoError(t, validatorDB.Close(), "Failed to close database")
		require.NoError(t, validatorDB.ClearDB(), "Failed to clear database")
	})

	keys, err := validatorDB.ProposedPublicKeys(ctx)
	require.NoError(t, err)
	assert.DeepEqual(t, make([][fieldparams.BLSPubkeyLength]byte, 0), keys)

	pubKey := [fieldparams.BLSPubkeyLength]byte{1}
	var dummyRoot [32]byte
	err = validatorDB.SaveProposalHistoryForSlot(ctx, pubKey, 1, dummyRoot[:])
	require.NoError(t, err)

	keys, err = validatorDB.ProposedPublicKeys(ctx)
	require.NoError(t, err)
	assert.DeepEqual(t, [][fieldparams.BLSPubkeyLength]byte{pubKey}, keys)
}

func TestStore_LowestSignedProposal(t *testing.T) {
	ctx := context.Background()
	pubkey := [fieldparams.BLSPubkeyLength]byte{3}
	var dummySigningRoot [32]byte
	validatorDB := setupDB(t, [][fieldparams.BLSPubkeyLength]byte{pubkey})

	_, exists, err := validatorDB.LowestSignedProposal(ctx, pubkey)
	require.NoError(t, err)
	require.Equal(t, false, exists)

	// We save our first proposal history.
	err = validatorDB.SaveProposalHistoryForSlot(ctx, pubkey, 2 /* slot */, dummySigningRoot[:])
	require.NoError(t, err)

	// We expect the lowest signed slot is what we just saved.
	slot, exists, err := validatorDB.LowestSignedProposal(ctx, pubkey)
	require.NoError(t, err)
	require.Equal(t, true, exists)
	assert.Equal(t, primitives.Slot(2), slot)

	// We save a higher proposal history.
	err = validatorDB.SaveProposalHistoryForSlot(ctx, pubkey, 3 /* slot */, dummySigningRoot[:])
	require.NoError(t, err)

	// We expect the lowest signed slot did not change.
	slot, exists, err = validatorDB.LowestSignedProposal(ctx, pubkey)
	require.NoError(t, err)
	require.Equal(t, true, exists)
	assert.Equal(t, primitives.Slot(2), slot)

	// We save a lower proposal history.
	err = validatorDB.SaveProposalHistoryForSlot(ctx, pubkey, 1 /* slot */, dummySigningRoot[:])
	require.NoError(t, err)

	// We expect the lowest signed slot indeed changed.
	slot, exists, err = validatorDB.LowestSignedProposal(ctx, pubkey)
	require.NoError(t, err)
	require.Equal(t, true, exists)
	assert.Equal(t, primitives.Slot(1), slot)
}

func TestStore_HighestSignedProposal(t *testing.T) {
	ctx := context.Background()
	pubkey := [fieldparams.BLSPubkeyLength]byte{3}
	var dummySigningRoot [32]byte
	validatorDB := setupDB(t, [][fieldparams.BLSPubkeyLength]byte{pubkey})

	_, exists, err := validatorDB.HighestSignedProposal(ctx, pubkey)
	require.NoError(t, err)
	require.Equal(t, false, exists)

	// We save our first proposal history.
	err = validatorDB.SaveProposalHistoryForSlot(ctx, pubkey, 2 /* slot */, dummySigningRoot[:])
	require.NoError(t, err)

	// We expect the highest signed slot is what we just saved.
	slot, exists, err := validatorDB.HighestSignedProposal(ctx, pubkey)
	require.NoError(t, err)
	require.Equal(t, true, exists)
	assert.Equal(t, primitives.Slot(2), slot)

	// We save a lower proposal history.
	err = validatorDB.SaveProposalHistoryForSlot(ctx, pubkey, 1 /* slot */, dummySigningRoot[:])
	require.NoError(t, err)

	// We expect the lowest signed slot did not change.
	slot, exists, err = validatorDB.HighestSignedProposal(ctx, pubkey)
	require.NoError(t, err)
	require.Equal(t, true, exists)
	assert.Equal(t, primitives.Slot(2), slot)

	// We save a higher proposal history.
	err = validatorDB.SaveProposalHistoryForSlot(ctx, pubkey, 3 /* slot */, dummySigningRoot[:])
	require.NoError(t, err)

	// We expect the highest signed slot indeed changed.
	slot, exists, err = validatorDB.HighestSignedProposal(ctx, pubkey)
	require.NoError(t, err)
	require.Equal(t, true, exists)
	assert.Equal(t, primitives.Slot(3), slot)
}

func Test_slashableProposalCheck_PreventsLowerThanMinProposal(t *testing.T) {
	ctx := context.Background()
	lowestSignedSlot := primitives.Slot(10)

	var pubkey [fieldparams.BLSPubkeyLength]byte
	pubkeyBytes, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
	require.NoError(t, err, "Failed to decode pubkey")
	copy(pubkey[:], pubkeyBytes)

	db := setupDB(t, [][fieldparams.BLSPubkeyLength]byte{pubkey})
	require.NoError(t, err)

	// We save a proposal at the lowest signed slot in the DB.
	err = db.SaveProposalHistoryForSlot(ctx, pubkey, lowestSignedSlot, []byte{1})
	require.NoError(t, err)

	// We expect the same block with a slot lower than the lowest
	// signed slot to fail validation.
	blk := &ethpb.SignedBeaconBlock{
		Block: &ethpb.BeaconBlock{
			Slot:          lowestSignedSlot - 1,
			ProposerIndex: 0,
			Body:          &ethpb.BeaconBlockBody{},
		},
		Signature: params.BeaconConfig().EmptySignature[:],
	}
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	err = db.SlashableProposalCheck(context.Background(), pubkey, wsb, [32]byte{4}, false, nil)
	require.ErrorContains(t, "could not sign block with slot < lowest signed", err)

	// We expect the same block with a slot equal to the lowest
	// signed slot to pass validation if signing roots are equal.
	blk = &ethpb.SignedBeaconBlock{
		Block: &ethpb.BeaconBlock{
			Slot:          lowestSignedSlot,
			ProposerIndex: 0,
			Body:          &ethpb.BeaconBlockBody{},
		},
		Signature: params.BeaconConfig().EmptySignature[:],
	}
	wsb, err = blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	err = db.SlashableProposalCheck(ctx, pubkey, wsb, [32]byte{1}, false, nil)
	require.NoError(t, err)

	// We expect the same block with a slot equal to the lowest
	// signed slot to fail validation if signing roots are different.
	wsb, err = blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	err = db.SlashableProposalCheck(ctx, pubkey, wsb, [32]byte{4}, false, nil)
	require.ErrorContains(t, "could not sign block with slot == lowest signed", err)

	// We expect the same block with a slot > than the lowest
	// signed slot to pass validation.
	blk = &ethpb.SignedBeaconBlock{
		Block: &ethpb.BeaconBlock{
			Slot:          lowestSignedSlot + 1,
			ProposerIndex: 0,
			Body:          &ethpb.BeaconBlockBody{},
		},
		Signature: params.BeaconConfig().EmptySignature[:],
	}

	wsb, err = blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	err = db.SlashableProposalCheck(ctx, pubkey, wsb, [32]byte{3}, false, nil)
	require.NoError(t, err)
}

func Test_slashableProposalCheck(t *testing.T) {
	ctx := context.Background()

	var pubkey [fieldparams.BLSPubkeyLength]byte
	pubkeyBytes, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
	require.NoError(t, err, "Failed to decode pubkey")
	copy(pubkey[:], pubkeyBytes)

	db := setupDB(t, [][fieldparams.BLSPubkeyLength]byte{pubkey})
	require.NoError(t, err)

	blk := util.HydrateSignedBeaconBlock(&ethpb.SignedBeaconBlock{
		Block: &ethpb.BeaconBlock{
			Slot:          10,
			ProposerIndex: 0,
			Body:          &ethpb.BeaconBlockBody{},
		},
		Signature: params.BeaconConfig().EmptySignature[:],
	})

	// We save a proposal at slot 1 as our lowest proposal.
	err = db.SaveProposalHistoryForSlot(ctx, pubkey, 1, []byte{1})
	require.NoError(t, err)

	// We save a proposal at slot 10 with a dummy signing root.
	dummySigningRoot := [32]byte{1}
	err = db.SaveProposalHistoryForSlot(ctx, pubkey, 10, dummySigningRoot[:])
	require.NoError(t, err)
	sBlock, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)

	err = db.SlashableProposalCheck(ctx, pubkey, sBlock, dummySigningRoot, false, nil)
	// We expect the same block sent out with the same root should not be slasahble.
	require.NoError(t, err)

	// We expect the same block sent out with a different signing root should be slashable.
	err = db.SlashableProposalCheck(ctx, pubkey, sBlock, [32]byte{2}, false, nil)
	require.ErrorContains(t, common.FailedBlockSignLocalErr, err)

	// We save a proposal at slot 11 with a nil signing root.
	blk.Block.Slot = 11
	sBlock, err = blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	err = db.SaveProposalHistoryForSlot(ctx, pubkey, blk.Block.Slot, nil)
	require.NoError(t, err)

	// We expect the same block sent out should return slashable error even
	// if we had a nil signing root stored in the database.
	err = db.SlashableProposalCheck(ctx, pubkey, sBlock, [32]byte{2}, false, nil)
	require.ErrorContains(t, common.FailedBlockSignLocalErr, err)

	// A block with a different slot for which we do not have a proposing history
	// should not be failing validation.
	blk.Block.Slot = 9
	sBlock, err = blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	err = db.SlashableProposalCheck(ctx, pubkey, sBlock, [32]byte{3}, false, nil)
	require.NoError(t, err, "Expected allowed block not to throw error")
}

func Test_slashableProposalCheck_RemoteProtection(t *testing.T) {
	var pubkey [fieldparams.BLSPubkeyLength]byte
	pubkeyBytes, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
	require.NoError(t, err, "Failed to decode pubkey")
	copy(pubkey[:], pubkeyBytes)

	db := setupDB(t, [][fieldparams.BLSPubkeyLength]byte{pubkey})
	require.NoError(t, err)

	blk := util.NewBeaconBlock()
	blk.Block.Slot = 10
	sBlock, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)

	err = db.SlashableProposalCheck(context.Background(), pubkey, sBlock, [32]byte{2}, false, nil)
	require.NoError(t, err, "Expected allowed block not to throw error")
}
