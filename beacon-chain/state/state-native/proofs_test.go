package state_native_test

import (
	"testing"

	statenative "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestBeaconStateMerkleProofs_phase0_notsupported(t *testing.T) {
	ctx := t.Context()
	st, _ := util.DeterministicGenesisState(t, 256)
	t.Run("current sync committee", func(t *testing.T) {
		_, err := st.CurrentSyncCommitteeProof(ctx)
		require.ErrorContains(t, "not supported", err)
	})
	t.Run("next sync committee", func(t *testing.T) {
		_, err := st.NextSyncCommitteeProof(ctx)
		require.ErrorContains(t, "not supported", err)
	})
}
func TestBeaconStateMerkleProofs_altair(t *testing.T) {
	ctx := t.Context()
	altair, err := util.NewBeaconStateAltair()
	require.NoError(t, err)
	htr, err := altair.HashTreeRoot(ctx)
	require.NoError(t, err)
	results := []string{
		"0x173669ae8794c057def63b20372114a628abb029354a2ef50d7a1aaa9a3dab4a",
		"0xe8facaa9be1c488207092f135ca6159f7998f313459b4198f46a9433f8b346e6",
		"0x0a7910590f2a08faa740a5c40e919722b80a786d18d146318309926a6b2ab95e",
		"0xc78009fdf07fc56a11f122370658a353aaa542ed63e44c4bc15ff4cd105ab33c",
		"0x4616e1d9312a92eb228e8cd5483fa1fca64d99781d62129bc53718d194b98c45",
	}
	t.Run("current sync committee", func(t *testing.T) {
		cscp, err := altair.CurrentSyncCommitteeProof(ctx)
		require.NoError(t, err)
		require.Equal(t, 5, len(cscp))
		for i, bytes := range cscp {
			require.Equal(t, results[i], hexutil.Encode(bytes))
		}
	})
	t.Run("next sync committee", func(t *testing.T) {
		nscp, err := altair.NextSyncCommitteeProof(ctx)
		require.NoError(t, err)
		require.Equal(t, 5, len(nscp))
		for i, bytes := range nscp {
			require.Equal(t, results[i], hexutil.Encode(bytes))
		}
	})
	t.Run("finalized root", func(t *testing.T) {
		finalizedRoot := altair.FinalizedCheckpoint().Root
		proof, err := altair.FinalizedRootProof(ctx)
		require.NoError(t, err)
		gIndex := statenative.FinalizedRootGeneralizedIndex()
		valid := trie.VerifyMerkleProof(htr[:], finalizedRoot, gIndex, proof)
		require.Equal(t, true, valid)
	})
	t.Run("recomputes root on dirty fields", func(t *testing.T) {
		currentRoot, err := altair.HashTreeRoot(ctx)
		require.NoError(t, err)
		cpt := altair.FinalizedCheckpoint()
		require.NoError(t, err)

		// Edit the checkpoint.
		cpt.Epoch = 100
		require.NoError(t, altair.SetFinalizedCheckpoint(cpt))

		// Produce a proof for the finalized root.
		proof, err := altair.FinalizedRootProof(ctx)
		require.NoError(t, err)

		// We expect the previous step to have triggered
		// a recomputation of dirty fields in the beacon state, resulting
		// in a new hash tree root as the finalized checkpoint had previously
		// changed and should have been marked as a dirty state field.
		// The proof validity should be false for the old root, but true for the new.
		finalizedRoot := altair.FinalizedCheckpoint().Root
		gIndex := statenative.FinalizedRootGeneralizedIndex()
		valid := trie.VerifyMerkleProof(currentRoot[:], finalizedRoot, gIndex, proof)
		require.Equal(t, false, valid)

		newRoot, err := altair.HashTreeRoot(ctx)
		require.NoError(t, err)

		valid = trie.VerifyMerkleProof(newRoot[:], finalizedRoot, gIndex, proof)
		require.Equal(t, true, valid)
	})
}

func TestBeaconStateMerkleProofs_bellatrix(t *testing.T) {
	ctx := t.Context()
	bellatrix, err := util.NewBeaconStateBellatrix()
	require.NoError(t, err)
	htr, err := bellatrix.HashTreeRoot(ctx)
	require.NoError(t, err)
	results := []string{
		"0x173669ae8794c057def63b20372114a628abb029354a2ef50d7a1aaa9a3dab4a",
		"0xe8facaa9be1c488207092f135ca6159f7998f313459b4198f46a9433f8b346e6",
		"0x0a7910590f2a08faa740a5c40e919722b80a786d18d146318309926a6b2ab95e",
		"0xa83dc5a6222b6e5d5f11115ec4ba4035512c060e74908c56ebc25ad74dd25c18",
		"0x4616e1d9312a92eb228e8cd5483fa1fca64d99781d62129bc53718d194b98c45",
	}
	t.Run("current sync committee", func(t *testing.T) {
		cscp, err := bellatrix.CurrentSyncCommitteeProof(ctx)
		require.NoError(t, err)
		require.Equal(t, 5, len(cscp))
		for i, bytes := range cscp {
			require.Equal(t, results[i], hexutil.Encode(bytes))
		}
	})
	t.Run("next sync committee", func(t *testing.T) {
		nscp, err := bellatrix.NextSyncCommitteeProof(ctx)
		require.NoError(t, err)
		require.Equal(t, 5, len(nscp))
		for i, bytes := range nscp {
			require.Equal(t, results[i], hexutil.Encode(bytes))
		}
	})
	t.Run("finalized root", func(t *testing.T) {
		finalizedRoot := bellatrix.FinalizedCheckpoint().Root
		proof, err := bellatrix.FinalizedRootProof(ctx)
		require.NoError(t, err)
		gIndex := statenative.FinalizedRootGeneralizedIndex()
		valid := trie.VerifyMerkleProof(htr[:], finalizedRoot, gIndex, proof)
		require.Equal(t, true, valid)
	})
	t.Run("recomputes root on dirty fields", func(t *testing.T) {
		currentRoot, err := bellatrix.HashTreeRoot(ctx)
		require.NoError(t, err)
		cpt := bellatrix.FinalizedCheckpoint()
		require.NoError(t, err)

		// Edit the checkpoint.
		cpt.Epoch = 100
		require.NoError(t, bellatrix.SetFinalizedCheckpoint(cpt))

		// Produce a proof for the finalized root.
		proof, err := bellatrix.FinalizedRootProof(ctx)
		require.NoError(t, err)

		// We expect the previous step to have triggered
		// a recomputation of dirty fields in the beacon state, resulting
		// in a new hash tree root as the finalized checkpoint had previously
		// changed and should have been marked as a dirty state field.
		// The proof validity should be false for the old root, but true for the new.
		finalizedRoot := bellatrix.FinalizedCheckpoint().Root
		gIndex := statenative.FinalizedRootGeneralizedIndex()
		valid := trie.VerifyMerkleProof(currentRoot[:], finalizedRoot, gIndex, proof)
		require.Equal(t, false, valid)

		newRoot, err := bellatrix.HashTreeRoot(ctx)
		require.NoError(t, err)

		valid = trie.VerifyMerkleProof(newRoot[:], finalizedRoot, gIndex, proof)
		require.Equal(t, true, valid)
	})
}

func TestBeaconStateMerkleProofs_electra_generalized(t *testing.T) {
	ctx := t.Context()
	electra, err := util.NewBeaconStateElectra()
	require.NoError(t, err)
	htr, err := electra.HashTreeRoot(ctx)
	require.NoError(t, err)
	t.Run("validators", func(t *testing.T) {
		validatorsRoot, err := stateutil.ValidatorRegistryRoot(electra.Validators())
		require.NoError(t, err)
		proof, err := electra.ProofByFieldIndex(ctx, types.Validators)
		require.NoError(t, err)
		gIndex := uint64(75) // Post-Electra: generalized index for field "validators" is 75.
		valid := trie.VerifyMerkleProof(htr[:], validatorsRoot[:], gIndex, proof)
		require.Equal(t, true, valid)
	})
	t.Run("pending deposits", func(t *testing.T) {
		pds, err := electra.PendingDeposits()
		require.NoError(t, err)
		pbdRoot, err := stateutil.PendingDepositsRoot(pds)
		require.NoError(t, err)
		proof, err := electra.ProofByFieldIndex(ctx, types.PendingDeposits)
		require.NoError(t, err)
		gIndex := uint64(98) // Post-Electra: generalized index for field "pending_deposits" is 98.
		valid := trie.VerifyMerkleProof(htr[:], pbdRoot[:], gIndex, proof)
		require.Equal(t, true, valid)
	})

}

// func TestBeaconStateMerkleProofs_electra_generalizedIndices(t *testing.T) {
// 	ctx := t.Context()
// 	electra, err := util.NewBeaconStateElectra(func(s *ethpb.BeaconStateElectra) error {
// 		s.Fork.PreviousVersion = []byte{0x00, 0x00, 0x10, 0x00}
// 		s.Fork.CurrentVersion = []byte{0x00, 0x00, 0x10, 0x00}
// 		s.Fork.Epoch = 2
// 		s.Slot = 1000
// 		s.Validators = make([]*ethpb.Validator, 128)
// 		for i := 0; i < 128; i++ {
// 			s.Validators[i] = &ethpb.Validator{
// 				WithdrawalCredentials: []byte{byte(i)},
// 			}
// 		}
// 		s.Balances = make([]uint64, 128)
// 		for i := 0; i < 128; i++ {
// 			s.Balances[i] = uint64(i * 1000)
// 		}
// 		return nil
// 	})
// 	require.NoError(t, err)
// 	htr, err := electra.HashTreeRoot(ctx)
// 	require.NoError(t, err)
// 	t.Run("genesis_time", func(t *testing.T) {
// 		gIndex := uint64(64) // Post-Electra: generalized index for field "genesis_time".
// 		require.NoError(t, err)
// 		proof, err := electra.ProofByGeneralizedIndex(ctx, []uint64{gIndex})
// 		require.NoError(t, err)
// 		genesisTime := electra.GenesisTime()
// 		genesisTimeBytes := make([]byte, 8)
// 		binary.LittleEndian.PutUint64(genesisTimeBytes, uint64(genesisTime.UnixMilli()))
// 		valid := trie.VerifyMerkleProof(htr[:], genesisTimeBytes, gIndex, proof)
// 		require.Equal(t, true, valid)
// 	})
// 	t.Run("genesis_validators_root", func(t *testing.T) {
// 		gIndex := uint64(65) // Post-Electra: generalized index for field "validators" is 82463372083200.
// 		require.NoError(t, err)
// 		proof, err := electra.ProofByGeneralizedIndex(ctx, []uint64{gIndex})
// 		require.NoError(t, err)
// 		valid := trie.VerifyMerkleProof(htr[:], electra.GenesisValidatorsRoot(), gIndex, proof)
// 		require.Equal(t, true, valid)
// 	})
// 	t.Run("slot", func(t *testing.T) {
// 		gIndex := uint64(66) // Post-Electra: generalized index for field "slot"
// 		require.NoError(t, err)
// 		proof, err := electra.ProofByGeneralizedIndex(ctx, []uint64{gIndex})
// 		require.NoError(t, err)
// 		slot := electra.Slot()
// 		slotBytes := make([]byte, 8)
// 		binary.LittleEndian.PutUint64(slotBytes, uint64(slot))
// 		valid := trie.VerifyMerkleProof(htr[:], slotBytes, gIndex, proof)
// 		require.Equal(t, true, valid)
// 	})
// 	t.Run("fork.previous_version", func(t *testing.T) {
// 		gIndex := uint64(268) // Post-Electra: generalized index for field "fork.previous_version" is 82463372083200.
// 		require.NoError(t, err)
// 		proof, err := electra.ProofByGeneralizedIndex(ctx, []uint64{gIndex})
// 		require.NoError(t, err)
// 		valid := trie.VerifyMerkleProof(htr[:], electra.Fork().PreviousVersion, gIndex, proof)
// 		require.Equal(t, true, valid)
// 	})
// 	t.Run("fork.current_version", func(t *testing.T) {
// 		gIndex := uint64(269) // Post-Electra: generalized index for field "fork.previous_version" is 82463372083200.
// 		require.NoError(t, err)
// 		proof, err := electra.ProofByGeneralizedIndex(ctx, []uint64{gIndex})
// 		require.NoError(t, err)
// 		valid := trie.VerifyMerkleProof(htr[:], electra.Fork().CurrentVersion, gIndex, proof)
// 		require.Equal(t, true, valid)
// 	})
// 	t.Run("fork.epoch", func(t *testing.T) {
// 		gIndex := uint64(270) // Post-Electra: generalized index for field "fork.epoch" is 82463372083200.
// 		require.NoError(t, err)
// 		proof, err := electra.ProofByGeneralizedIndex(ctx, []uint64{gIndex})
// 		require.NoError(t, err)
// 		epoch := electra.Fork().Epoch
// 		epochBytes := make([]byte, 8)
// 		binary.LittleEndian.PutUint64(epochBytes, uint64(epoch))
// 		valid := trie.VerifyMerkleProof(htr[:], epochBytes, gIndex, proof)
// 		require.Equal(t, true, valid)
// 	})
// 	t.Run("block_roots[0]", func(t *testing.T) {
// 		gIndex := uint64(565248) // Post-Electra: generalized index for field "block_roots" is 82463372083200.
// 		require.NoError(t, err)
// 		proof, err := electra.ProofByGeneralizedIndex(ctx, []uint64{gIndex})
// 		require.NoError(t, err)
// 		valid := trie.VerifyMerkleProof(htr[:], electra.BlockRoots()[0], gIndex, proof)
// 		require.Equal(t, true, valid)
// 	})
// 	t.Run("state_roots[0]", func(t *testing.T) {
// 		gIndex := uint64(573440) // Post-Electra: generalized index for field "state_roots" is 82463372083200.
// 		require.NoError(t, err)
// 		proof, err := electra.ProofByGeneralizedIndex(ctx, []uint64{gIndex})
// 		require.NoError(t, err)
// 		valid := trie.VerifyMerkleProof(htr[:], electra.StateRoots()[0], gIndex, proof)
// 		require.Equal(t, true, valid)
// 	})
// 	// t.Run("historical_roots[0]", func(t *testing.T) {
// 	// 	gIndex := uint64(2382364672) // Post-Electra: generalized index for field "historical_roots" is 82463372083200.
// 	// 	require.NoError(t, err)
// 	// 	proof, err := electra.ProofByGeneralizedIndex(ctx, []uint64{gIndex})
// 	// 	require.NoError(t, err)
// 	// 	valid := trie.VerifyMerkleProof(htr[:], electra.HistoricalRoots()[0], gIndex, proof)
// 	// 	require.Equal(t, true, valid)
// 	// })

// 	t.Run("eth1Data.deposit_root", func(t *testing.T) {
// 		gIndex := uint64(288) // Post-Electra: generalized index for field "eth1data.deposit_root" is 82463372083200.
// 		require.NoError(t, err)
// 		proof, err := electra.ProofByGeneralizedIndex(ctx, []uint64{gIndex})
// 		require.NoError(t, err)
// 		valid := trie.VerifyMerkleProof(htr[:], electra.Eth1Data().DepositRoot, gIndex, proof)
// 		require.Equal(t, true, valid)
// 	})
// 	t.Run("eth1Data.deposit_count", func(t *testing.T) {
// 		gIndex := uint64(289) // Post-Electra: generalized index for field "eth1data.deposit_count" is 82463372083200.
// 		require.NoError(t, err)
// 		proof, err := electra.ProofByGeneralizedIndex(ctx, []uint64{gIndex})
// 		require.NoError(t, err)
// 		depositCount := make([]byte, 8)
// 		binary.LittleEndian.PutUint64(depositCount, electra.Eth1Data().DepositCount)
// 		valid := trie.VerifyMerkleProof(htr[:], depositCount, gIndex, proof)
// 		require.Equal(t, true, valid)
// 	})
// 	t.Run("eth1Data.block_hash", func(t *testing.T) {
// 		gIndex := uint64(290) // Post-Electra: generalized index for field "eth1data.block_hash" is 82463372083200.
// 		require.NoError(t, err)
// 		proof, err := electra.ProofByGeneralizedIndex(ctx, []uint64{gIndex})
// 		require.NoError(t, err)
// 		valid := trie.VerifyMerkleProof(htr[:], electra.Eth1Data().BlockHash, gIndex, proof)
// 		require.Equal(t, true, valid)
// 	})
// 	// t.Run("eth1DataVotes[0]", func(t *testing.T) {
// 	// 	gIndex := uint64(1196032) // Post-Electra: generalized index for field "eth1dataVotes[0]" is 82463372083200.
// 	// 	require.NoError(t, err)
// 	// 	proof, err := electra.ProofByGeneralizedIndex(ctx, []uint64{gIndex})
// 	// 	require.NoError(t, err)
// 	// 	votes := electra.Eth1DataVotes()[0].DepositRoot
// 	// 	valid := trie.VerifyMerkleProof(htr[:], votes, gIndex, proof)
// 	// 	require.Equal(t, true, valid)
// 	// })
// 	t.Run("eth1_deposit_index", func(t *testing.T) {
// 		gIndex := uint64(74) // Post-Electra: generalized index for field "eth1data.deposit_index" is 82463372083200.
// 		require.NoError(t, err)
// 		proof, err := electra.ProofByGeneralizedIndex(ctx, []uint64{gIndex})
// 		require.NoError(t, err)
// 		eth1DepositIndex := make([]byte, 8)
// 		binary.LittleEndian.PutUint64(eth1DepositIndex, electra.Eth1DepositIndex())
// 		valid := trie.VerifyMerkleProof(htr[:], eth1DepositIndex, gIndex, proof)
// 		require.Equal(t, true, valid)
// 	})
// 	// t.Run("validators[0]", func(t *testing.T) {
// 	// 	gIndex := uint64(1319413953331201) // Post-Electra: generalized index for field "validators[0]" is 82463372083200.
// 	// 	require.NoError(t, err)
// 	// 	proof, err := electra.ProofByGeneralizedIndex(ctx, []uint64{gIndex})
// 	// 	require.NoError(t, err)
// 	// 	wc := electra.Validators()[0].WithdrawalCredentials

// 	// 	valid := trie.VerifyMerkleProof(htr[:], wc, gIndex, proof)
// 	// 	require.Equal(t, true, valid)
// 	// })
// 	// t.Run("balances[0]", func(t *testing.T) {
// 	// 	gIndex := uint64(41781441855488) // Post-Electra: generalized index for field "balances[0]" is 82463372083200.
// 	// 	require.NoError(t, err)
// 	// 	proof, err := electra.ProofByGeneralizedIndex(ctx, []uint64{gIndex})
// 	// 	require.NoError(t, err)
// 	// 	balance := electra.Balances()[0]
// 	// 	balanceBytes := make([]byte, 8)
// 	// 	binary.LittleEndian.PutUint64(balanceBytes, balance)
// 	// 	valid := trie.VerifyMerkleProof(htr[:], balanceBytes, gIndex, proof)
// 	// 	require.Equal(t, true, valid)
// 	// })
// 	t.Run("randao_mixes[0]", func(t *testing.T) {
// 		electra.UpdateRandaoMixesAtIndex(0, [32]byte{0xAB, 0xCD, 0xEF})
// 		gIndex := uint64(5046272) // Post-Electra: generalized index for field "randao_mixes" is 82463372083200.
// 		require.NoError(t, err)
// 		proof, err := electra.ProofByGeneralizedIndex(ctx, []uint64{gIndex})
// 		require.NoError(t, err)
// 		valid := trie.VerifyMerkleProof(htr[:], electra.RandaoMixes()[0][:], gIndex, proof)
// 		require.Equal(t, true, valid)
// 	})
// 	t.Run("slashings[0]", func(t *testing.T) {
// 		gIndex := uint64(159744) // Post-Electra: generalized index for field "slashings[0]" is 82463372083200.
// 		require.NoError(t, err)
// 		proof, err := electra.ProofByGeneralizedIndex(ctx, []uint64{gIndex})
// 		require.NoError(t, err)
// 		slashings := make([]byte, 8)
// 		binary.LittleEndian.PutUint64(slashings, electra.Slashings()[0])
// 		valid := trie.VerifyMerkleProof(htr[:], slashings, gIndex, proof)
// 		require.Equal(t, true, valid)
// 	})
// 	t.Run("previous_epoch_participation", func(t *testing.T) {
// 		gIndex := uint64(79) // Post-Electra: generalized index for field "5428838662144[0]" is 82463372083200.
// 		require.NoError(t, err)
// 		proof, err := electra.ProofByGeneralizedIndex(ctx, []uint64{gIndex})
// 		require.NoError(t, err)
// 		previousEpochParticipation, err := electra.PreviousEpochParticipation()
// 		require.NoError(t, err)

// 		valid := trie.VerifyMerkleProof(htr[:], previousEpochParticipation, gIndex, proof)
// 		require.Equal(t, true, valid)
// 	})
// 	t.Run("pending_deposits[0].amount", func(t *testing.T) {
// 		gIndex := uint64(210453397506) // Post-Electra: generalized index for field "5428838662144[0]" is 82463372083200.
// 		require.NoError(t, err)
// 		proof, err := electra.ProofByGeneralizedIndex(ctx, []uint64{gIndex})
// 		require.NoError(t, err)
// 		aa, err := electra.PendingDeposits()
// 		amount := make([]byte, 8)
// 		binary.LittleEndian.PutUint64(amount, aa[0].Amount)
// 		valid := trie.VerifyMerkleProof(htr[:], amount, gIndex, proof)
// 		require.Equal(t, true, valid)
// 	})

// }
