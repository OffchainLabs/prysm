package filesystem

import (
	"context"
	"testing"

	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/prysmaticlabs/prysm/v5/validator/db/common"
)

func TestStore_ProposalHistoryForPubKey(t *testing.T) {
	var slot uint64 = 42
	ctx := context.Background()

	for _, tt := range []struct {
		name                        string
		validatorSlashingProtection *ValidatorSlashingProtection
		expectedProposals           []*common.Proposal
	}{
		{
			name:                        "validatorSlashingProtection is nil",
			validatorSlashingProtection: nil,
			expectedProposals:           []*common.Proposal{},
		},
		{
			name:                        "validatorSlashingProtection.LatestSignedBlockSlot is nil",
			validatorSlashingProtection: &ValidatorSlashingProtection{LatestSignedBlockSlot: nil},
			expectedProposals:           []*common.Proposal{},
		},
		{
			name:                        "validatorSlashingProtection.LatestSignedBlockSlot is something",
			validatorSlashingProtection: &ValidatorSlashingProtection{LatestSignedBlockSlot: &slot},
			expectedProposals: []*common.Proposal{
				{
					Slot: primitives.Slot(slot),
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Get a database path.
			databasePath := t.TempDir()

			// Create a public key.
			pubkey := getPubKeys(t, 1)[0]

			// Create a new store.
			store, err := NewStore(databasePath, nil)
			require.NoError(t, err, "NewStore should not return an error")

			// Set the validator slashing protection.
			err = store.saveValidatorSlashingProtection(pubkey, tt.validatorSlashingProtection)
			require.NoError(t, err, "saveValidatorSlashingProtection should not return an error")

			// Get the proposal history for the public key.
			actualProposals, err := store.ProposalHistoryForPubKey(ctx, pubkey)
			require.NoError(t, err, "ProposalHistoryForPubKey should not return an error")
			require.DeepEqual(t, tt.expectedProposals, actualProposals, "ProposalHistoryForPubKey should return the expected proposals")
		})
	}
}

func TestStore_SaveProposalHistoryForSlot(t *testing.T) {
	var (
		slot41 uint64 = 41
		slot42 uint64 = 42
		slot43 uint64 = 43
	)

	ctx := context.Background()

	for _, tt := range []struct {
		name                                string
		initialValidatorSlashingProtection  *ValidatorSlashingProtection
		slot                                uint64
		expectedValidatorSlashingProtection ValidatorSlashingProtection
		expectedError                       string
	}{
		{
			name:                                "validatorSlashingProtection is nil",
			initialValidatorSlashingProtection:  nil,
			slot:                                slot42,
			expectedValidatorSlashingProtection: ValidatorSlashingProtection{LatestSignedBlockSlot: &slot42},
			expectedError:                       "",
		},
		{
			name:                                "validatorSlashingProtection.LatestSignedBlockSlot is nil",
			initialValidatorSlashingProtection:  &ValidatorSlashingProtection{LatestSignedBlockSlot: nil},
			slot:                                slot42,
			expectedValidatorSlashingProtection: ValidatorSlashingProtection{LatestSignedBlockSlot: &slot42},
			expectedError:                       "",
		},
		{
			name:                                "validatorSlashingProtection.LatestSignedBlockSlot is lower than the incoming slot",
			initialValidatorSlashingProtection:  &ValidatorSlashingProtection{LatestSignedBlockSlot: &slot42},
			slot:                                slot41,
			expectedValidatorSlashingProtection: ValidatorSlashingProtection{LatestSignedBlockSlot: &slot42},
			expectedError:                       "could not sign proposal with slot lower than or equal to recorded slot",
		},
		{
			name:                                "validatorSlashingProtection.LatestSignedBlockSlot is equal to the incoming slot",
			initialValidatorSlashingProtection:  &ValidatorSlashingProtection{LatestSignedBlockSlot: &slot42},
			slot:                                slot42,
			expectedValidatorSlashingProtection: ValidatorSlashingProtection{LatestSignedBlockSlot: &slot42},
			expectedError:                       "could not sign proposal with slot lower than or equal to recorded slot",
		},
		{
			name:                                "validatorSlashingProtection.LatestSignedBlockSlot is higher to the incoming slot",
			initialValidatorSlashingProtection:  &ValidatorSlashingProtection{LatestSignedBlockSlot: &slot42},
			slot:                                slot43,
			expectedValidatorSlashingProtection: ValidatorSlashingProtection{LatestSignedBlockSlot: &slot43},
			expectedError:                       "",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Get a database path.
			databasePath := t.TempDir()

			// Create a public key.
			pubkey := getPubKeys(t, 1)[0]

			// Create a new store.
			store, err := NewStore(databasePath, nil)
			require.NoError(t, err, "NewStore should not return an error")

			// Set the initial validator slashing protection.
			err = store.saveValidatorSlashingProtection(pubkey, tt.initialValidatorSlashingProtection)
			require.NoError(t, err, "saveValidatorSlashingProtection should not return an error")

			// Attempt to save the proposal history for the public key.
			err = store.SaveProposalHistoryForSlot(ctx, pubkey, primitives.Slot(tt.slot), nil)
			if len(tt.expectedError) > 0 {
				require.ErrorContains(t, tt.expectedError, err, "validatorSlashingProtection should return the expected error")
			} else {
				require.NoError(t, err, "SaveProposalHistoryForSlot should not return an error")
			}

			// Get the final validator slashing protection.
			actualValidatorSlashingProtection, err := store.validatorSlashingProtection(pubkey)
			require.NoError(t, err, "validatorSlashingProtection should not return an error")

			// Check the proposal history.
			require.DeepEqual(t, tt.expectedValidatorSlashingProtection, *actualValidatorSlashingProtection, "validatorSlashingProtection should be the expected one")
		})
	}
}

func TestStore_ProposedPublicKeys(t *testing.T) {
	// We get a database path
	databasePath := t.TempDir()

	// We create some pubkeys
	pubkeys := getPubKeys(t, 5)

	// We create a new store
	s, err := NewStore(databasePath, &Config{PubKeys: pubkeys})
	require.NoError(t, err, "NewStore should not return an error")

	// We check the public keys
	expected := pubkeys
	actual, err := s.ProposedPublicKeys(context.Background())
	require.NoError(t, err, "publicKeys should not return an error")

	// We cannot compare the slices directly because the order is not guaranteed,
	// so we compare sets instead.
	expectedSet := make(map[[fieldparams.BLSPubkeyLength]byte]bool)
	for _, pubkey := range expected {
		expectedSet[pubkey] = true
	}

	actualSet := make(map[[fieldparams.BLSPubkeyLength]byte]bool)
	for _, pubkey := range actual {
		actualSet[pubkey] = true
	}

	require.DeepEqual(t, expectedSet, actualSet)
}

func Test_slashableProposalCheck_PreventsLowerThanMinProposal(t *testing.T) {
	ctx := context.Background()

	// We get a database path
	databasePath := t.TempDir()

	// We create some pubkeys
	pubkeys := getPubKeys(t, 1)
	pubkey := pubkeys[0]

	// We create a new store
	s, err := NewStore(databasePath, &Config{PubKeys: pubkeys})
	require.NoError(t, err, "NewStore should not return an error")

	lowestSignedSlot := primitives.Slot(10)

	// We save a proposal at the lowest signed slot in the DB.
	err = s.SaveProposalHistoryForSlot(ctx, pubkey, lowestSignedSlot, []byte{1})
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
	err = s.SlashableProposalCheck(context.Background(), pubkey, wsb, [32]byte{4}, false, nil)
	require.ErrorContains(t, common.FailedBlockSignLocalErr, err)

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
	err = s.SlashableProposalCheck(context.Background(), pubkey, wsb, [32]byte{1}, false, nil)
	require.ErrorContains(t, common.FailedBlockSignLocalErr, err)

	// We expect the same block with a slot equal to the lowest
	// signed slot to fail validation if signing roots are different.
	wsb, err = blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	err = s.SlashableProposalCheck(context.Background(), pubkey, wsb, [32]byte{4}, false, nil)
	require.ErrorContains(t, common.FailedBlockSignLocalErr, err)

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
	err = s.SlashableProposalCheck(context.Background(), pubkey, wsb, [32]byte{3}, false, nil)
	require.NoError(t, err)
}

func Test_slashableProposalCheck(t *testing.T) {
	ctx := context.Background()

	// We get a database path
	databasePath := t.TempDir()

	// We create some pubkeys
	pubkeys := getPubKeys(t, 1)
	pubkey := pubkeys[0]

	// We create a new store
	s, err := NewStore(databasePath, &Config{PubKeys: pubkeys})
	require.NoError(t, err, "NewStore should not return an error")

	blk := util.HydrateSignedBeaconBlock(&ethpb.SignedBeaconBlock{
		Block: &ethpb.BeaconBlock{
			Slot:          10,
			ProposerIndex: 0,
			Body:          &ethpb.BeaconBlockBody{},
		},
		Signature: params.BeaconConfig().EmptySignature[:],
	})

	// We save a proposal at slot 1 as our lowest proposal.
	err = s.SaveProposalHistoryForSlot(ctx, pubkey, 1, []byte{1})
	require.NoError(t, err)

	// We save a proposal at slot 10 with a dummy signing root.
	dummySigningRoot := [32]byte{1}
	err = s.SaveProposalHistoryForSlot(ctx, pubkey, 10, dummySigningRoot[:])
	require.NoError(t, err)
	sBlock, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)

	// We expect the same block sent out should be slasahble.
	err = s.SlashableProposalCheck(context.Background(), pubkey, sBlock, dummySigningRoot, false, nil)
	require.ErrorContains(t, common.FailedBlockSignLocalErr, err)

	// We expect the same block sent out with a different signing root should be slashable.
	err = s.SlashableProposalCheck(context.Background(), pubkey, sBlock, [32]byte{2}, false, nil)
	require.ErrorContains(t, common.FailedBlockSignLocalErr, err)

	// We save a proposal at slot 11 with a nil signing root.
	blk.Block.Slot = 11
	sBlock, err = blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	err = s.SaveProposalHistoryForSlot(ctx, pubkey, blk.Block.Slot, nil)
	require.NoError(t, err)

	// We expect the same block sent out should return slashable error even
	// if we had a nil signing root stored in the database.
	err = s.SlashableProposalCheck(context.Background(), pubkey, sBlock, [32]byte{2}, false, nil)
	require.ErrorContains(t, common.FailedBlockSignLocalErr, err)

	// A block with a different slot for which we do not have a proposing history
	// should not be failing validation.
	blk.Block.Slot = 9
	sBlock, err = blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	err = s.SlashableProposalCheck(context.Background(), pubkey, sBlock, [32]byte{3}, false, nil)
	require.ErrorContains(t, common.FailedBlockSignLocalErr, err)
}

func Test_slashableProposalCheck_RemoteProtection(t *testing.T) {
	// We get a database path
	databasePath := t.TempDir()

	// We create some pubkeys
	pubkeys := getPubKeys(t, 1)
	pubkey := pubkeys[0]

	// We create a new store
	s, err := NewStore(databasePath, &Config{PubKeys: pubkeys})
	require.NoError(t, err, "NewStore should not return an error")

	blk := util.NewBeaconBlock()
	blk.Block.Slot = 10
	sBlock, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)

	err = s.SlashableProposalCheck(context.Background(), pubkey, sBlock, [32]byte{2}, false, nil)
	require.NoError(t, err, "Expected allowed block not to throw error")
}
