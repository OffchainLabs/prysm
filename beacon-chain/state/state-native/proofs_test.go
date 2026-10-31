package state_native_test

import (
	"slices"
	"sync"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	statenative "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
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

func TestLightClientGeneralizedIndicesForVersion(t *testing.T) {
	reset := features.InitWithReset(&features.Flags{})
	defer reset()

	type test struct {
		name       string
		version    int
		finalized  uint64
		current    uint64
		next       uint64
		committees bool
	}

	var tests []test

	for _, v := range version.AllIncludingUnreleased() {
		switch v {
		case version.Phase0:
			tests = append(tests, test{name: "phase0", version: v, finalized: 105})
		case version.Altair, version.Bellatrix, version.Capella, version.Deneb:
			tests = append(tests, test{name: version.String(v), version: v, finalized: 105, current: 54, next: 55, committees: true})
		case version.Electra, version.Fulu:
			tests = append(tests, test{name: version.String(v), version: v, finalized: 169, current: 86, next: 87, committees: true})
		case version.Gloas, version.Heze:
			tests = append(tests, test{name: version.String(v), version: v, finalized: 735, current: 2945, next: 2946, committees: true})
		default:
			t.Fatalf("unsupported version: %d", v)
		}
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			finalized, err := statenative.FinalizedRootGeneralizedIndexForVersion(test.version)
			require.NoError(t, err)
			require.Equal(t, test.finalized, finalized)

			current, currentErr := statenative.CurrentSyncCommitteeGeneralizedIndexForVersion(test.version)
			next, nextErr := statenative.NextSyncCommitteeGeneralizedIndexForVersion(test.version)
			if !test.committees {
				require.ErrorContains(t, "not supported", currentErr)
				require.ErrorContains(t, "not supported", nextErr)
				return
			}
			require.NoError(t, currentErr)
			require.NoError(t, nextErr)
			require.Equal(t, test.current, current)
			require.Equal(t, test.next, next)
		})
	}

	t.Run("unknown version", func(t *testing.T) {
		_, err := statenative.FinalizedRootGeneralizedIndexForVersion(-1)
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
		gIndex, err := statenative.FinalizedRootGeneralizedIndexForVersion(version.Altair)
		require.NoError(t, err)
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
		gIndex, err := statenative.FinalizedRootGeneralizedIndexForVersion(version.Altair)
		require.NoError(t, err)
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
		gIndex, err := statenative.FinalizedRootGeneralizedIndexForVersion(version.Bellatrix)
		require.NoError(t, err)
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
		gIndex, err := statenative.FinalizedRootGeneralizedIndexForVersion(version.Bellatrix)
		require.NoError(t, err)
		valid := trie.VerifyMerkleProof(currentRoot[:], finalizedRoot, gIndex, proof)
		require.Equal(t, false, valid)

		newRoot, err := bellatrix.HashTreeRoot(ctx)
		require.NoError(t, err)

		valid = trie.VerifyMerkleProof(newRoot[:], finalizedRoot, gIndex, proof)
		require.Equal(t, true, valid)
	})
}

func TestBeaconStateMerkleProofs_Gloas(t *testing.T) {
	ctx := t.Context()
	gloas, err := util.NewBeaconStateGloas()
	require.NoError(t, err)
	htr, err := gloas.HashTreeRoot(ctx)
	require.NoError(t, err)

	t.Run("current sync committee", func(t *testing.T) {
		committee, err := gloas.CurrentSyncCommittee()
		require.NoError(t, err)
		committeeRoot, err := committee.HashTreeRoot()
		require.NoError(t, err)
		proof, err := gloas.CurrentSyncCommitteeProof(ctx)
		require.NoError(t, err)
		require.Equal(t, 11, len(proof))
		gIndex, err := statenative.CurrentSyncCommitteeGeneralizedIndexForVersion(gloas.Version())
		require.NoError(t, err)
		require.Equal(t, true, trie.VerifyMerkleProof(htr[:], committeeRoot[:], gIndex, proof))
	})

	t.Run("next sync committee", func(t *testing.T) {
		committee, err := gloas.NextSyncCommittee()
		require.NoError(t, err)
		committeeRoot, err := committee.HashTreeRoot()
		require.NoError(t, err)
		proof, err := gloas.NextSyncCommitteeProof(ctx)
		require.NoError(t, err)
		require.Equal(t, 11, len(proof))
		gIndex, err := statenative.NextSyncCommitteeGeneralizedIndexForVersion(gloas.Version())
		require.NoError(t, err)
		require.Equal(t, true, trie.VerifyMerkleProof(htr[:], committeeRoot[:], gIndex, proof))
	})

	t.Run("finalized root", func(t *testing.T) {
		finalizedRoot := gloas.FinalizedCheckpoint().Root
		proof, err := gloas.FinalizedRootProof(ctx)
		require.NoError(t, err)
		require.Equal(t, 9, len(proof))
		gIndex, err := statenative.FinalizedRootGeneralizedIndexForVersion(gloas.Version())
		require.NoError(t, err)
		require.Equal(t, true, trie.VerifyMerkleProof(htr[:], finalizedRoot, gIndex, proof))
	})

	t.Run("recomputes dirty fields", func(t *testing.T) {
		checkpoint := gloas.FinalizedCheckpoint()
		checkpoint.Epoch = 100
		checkpoint.Root[0] = 1
		require.NoError(t, gloas.SetFinalizedCheckpoint(checkpoint))

		proof, err := gloas.FinalizedRootProof(ctx)
		require.NoError(t, err)
		newRoot, err := gloas.HashTreeRoot(ctx)
		require.NoError(t, err)
		finalizedRoot := gloas.FinalizedCheckpoint().Root
		gIndex, err := statenative.FinalizedRootGeneralizedIndexForVersion(gloas.Version())
		require.NoError(t, err)
		require.Equal(t, false, trie.VerifyMerkleProof(htr[:], finalizedRoot, gIndex, proof))
		require.Equal(t, true, trie.VerifyMerkleProof(newRoot[:], finalizedRoot, gIndex, proof))
	})
}

func TestFieldRoots(t *testing.T) {
	ctx := t.Context()

	t.Run("RootsMatchState", func(t *testing.T) {
		forEachFieldRootsState(t, func(t *testing.T, c fieldRootsCase, st state.BeaconState) {
			roots, err := st.FieldRoots(ctx)
			require.NoError(t, err)
			require.Equal(t, c.fieldCount, len(roots))
			if st.Version() >= version.Gloas {
				schema, ok := statenative.ProgressiveStateSchemaForVersion(st.Version())
				require.Equal(t, true, ok)
				require.Equal(t, len(schema.Fields()), len(roots))
			}
			require.DeepSSZEqual(t, freshFieldRoots(t, st), roots)
			want, err := st.HashTreeRoot(ctx)
			require.NoError(t, err)
			require.Equal(t, want, remerkleizeFieldRoots(t, st.Version(), roots))
		})
	})

	for _, c := range fieldRootsCases() {
		if c.name != "fulu" && c.name != "gloas" {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			t.Run("ReflectsMutations", func(t *testing.T) {
				st := c.newState(t)
				_, err := st.HashTreeRoot(ctx)
				require.NoError(t, err)
				before, err := st.FieldRoots(ctx)
				require.NoError(t, err)

				// One scalar field and two composite fields backed by field tries.
				require.NoError(t, st.SetSlot(st.Slot()+1))
				balance, err := st.BalanceAtIndex(3)
				require.NoError(t, err)
				require.NoError(t, st.UpdateBalancesAtIndex(3, balance+1))
				validator, err := st.ValidatorAtIndex(5)
				require.NoError(t, err)
				validator.EffectiveBalance -= 1_000_000_000
				require.NoError(t, st.UpdateValidatorAtIndex(5, validator))

				after, err := st.FieldRoots(ctx)
				require.NoError(t, err)
				require.DeepSSZEqual(t, freshFieldRoots(t, st), after)
				for _, f := range []types.FieldIndex{types.Slot, types.Balances, types.Validators} {
					i := fieldRootIndex(t, st.Version(), f)
					require.NotEqual(t, before[i], after[i], f.String())
				}
				want, err := st.HashTreeRoot(ctx)
				require.NoError(t, err)
				require.Equal(t, want, remerkleizeFieldRoots(t, st.Version(), after))

				again, err := st.FieldRoots(ctx)
				require.NoError(t, err)
				require.DeepSSZEqual(t, after, again)
			})

			t.Run("ReturnedSliceIsACopy", func(t *testing.T) {
				st := c.newState(t)
				_, err := st.HashTreeRoot(ctx)
				require.NoError(t, err)
				roots, err := st.FieldRoots(ctx)
				require.NoError(t, err)
				want := slices.Clone(roots)

				roots[0][0] ^= 0xff
				roots[len(roots)-1] = [32]byte{}

				again, err := st.FieldRoots(ctx)
				require.NoError(t, err)
				require.DeepSSZEqual(t, want, again)
			})

			t.Run("ConcurrentReadsAndWrites", func(t *testing.T) {
				st := c.newState(t)
				_, err := st.HashTreeRoot(ctx)
				require.NoError(t, err)

				const readers, iterations = 8, 20
				errs := make(chan error, (readers+2)*iterations)
				var wg sync.WaitGroup
				for range readers {
					wg.Go(func() {
						for range iterations {
							if _, err := st.FieldRoots(ctx); err != nil {
								errs <- err
							}
						}
					})
				}
				wg.Go(func() {
					for range iterations {
						if err := st.SetSlot(st.Slot() + 1); err != nil {
							errs <- err
						}
					}
				})
				wg.Go(func() {
					for i := range iterations {
						if err := st.UpdateBalancesAtIndex(primitives.ValidatorIndex(i), uint64(i)); err != nil {
							errs <- err
						}
					}
				})
				wg.Wait()
				close(errs)
				for err := range errs {
					require.NoError(t, err)
				}

				// Reads during the writes are not atomic with each other, so only the settled state is compared.
				roots, err := st.FieldRoots(ctx)
				require.NoError(t, err)
				require.DeepSSZEqual(t, freshFieldRoots(t, st), roots)
				want, err := st.HashTreeRoot(ctx)
				require.NoError(t, err)
				require.Equal(t, want, remerkleizeFieldRoots(t, st.Version(), roots))
			})
		})
	}
}

// fieldRootIndex returns the position of field f in the FieldRoots output for version v.
func fieldRootIndex(t *testing.T, v int, f types.FieldIndex) int {
	schema, progressive := statenative.ProgressiveStateSchemaForVersion(v)
	if !progressive {
		return f.RealPosition()
	}
	i := slices.Index(schema.Fields(), f)
	require.NotEqual(t, -1, i)
	return i
}

// forEachFieldRootsState runs fn on a fresh state per fork, once with a cold Merkle cache and once warmed by HashTreeRoot.
func forEachFieldRootsState(t *testing.T, fn func(t *testing.T, c fieldRootsCase, st state.BeaconState)) {
	for _, mode := range []string{"cold", "warm"} {
		t.Run(mode, func(t *testing.T) {
			for _, c := range fieldRootsCases() {
				t.Run(c.name, func(t *testing.T) {
					st := c.newState(t)
					if mode == "warm" {
						_, err := st.HashTreeRoot(t.Context())
						require.NoError(t, err)
					}
					fn(t, c, st)
				})
			}
		})
	}
}

// remerkleizeFieldRoots rebuilds the state root from its field roots the way an API consumer would.
func remerkleizeFieldRoots(t *testing.T, v int, roots [][32]byte) [32]byte {
	schema, progressive := statenative.ProgressiveStateSchemaForVersion(v)
	if !progressive {
		return ssz.MerkleizeVector(slices.Clone(roots), uint64(len(roots)))
	}
	root, err := ssz.ContainerRootProgressive(roots, schema.ActiveFields())
	require.NoError(t, err)
	return root
}

// freshFieldRoots recomputes the active field roots from scratch, bypassing the state's Merkle cache.
func freshFieldRoots(t *testing.T, st state.BeaconState) [][32]byte {
	native, ok := st.(*statenative.BeaconState)
	require.Equal(t, true, ok)
	raw, err := statenative.ComputeFieldRootsWithHasher(t.Context(), native)
	require.NoError(t, err)

	schema, progressive := statenative.ProgressiveStateSchemaForVersion(st.Version())
	if !progressive {
		roots := make([][32]byte, len(raw))
		for i := range raw {
			roots[i] = bytesutil.ToBytes32(raw[i])
		}
		return roots
	}
	fields := schema.Fields()
	roots := make([][32]byte, len(fields))
	for i, f := range fields {
		roots[i] = bytesutil.ToBytes32(raw[f.RealPosition()])
	}
	return roots
}

type fieldRootsCase struct {
	name       string
	newState   func(t *testing.T) state.BeaconState
	fieldCount int
}

// fieldRootsCases returns one populated state per fork with its active field count taken from the consensus specs.
func fieldRootsCases() []fieldRootsCase {
	const numValidators = 64
	return []fieldRootsCase{
		{name: "phase0", fieldCount: 21, newState: func(t *testing.T) state.BeaconState {
			st, _ := util.DeterministicGenesisState(t, numValidators)
			return st
		}},
		{name: "altair", fieldCount: 24, newState: func(t *testing.T) state.BeaconState {
			st, _ := util.DeterministicGenesisStateAltair(t, numValidators)
			return st
		}},
		{name: "bellatrix", fieldCount: 25, newState: func(t *testing.T) state.BeaconState {
			st, _ := util.DeterministicGenesisStateBellatrix(t, numValidators)
			return st
		}},
		{name: "capella", fieldCount: 28, newState: func(t *testing.T) state.BeaconState {
			st, _ := util.DeterministicGenesisStateCapella(t, numValidators)
			return st
		}},
		{name: "deneb", fieldCount: 28, newState: func(t *testing.T) state.BeaconState {
			st, _ := util.DeterministicGenesisStateDeneb(t, numValidators)
			return st
		}},
		{name: "electra", fieldCount: 37, newState: func(t *testing.T) state.BeaconState {
			st, _ := util.DeterministicGenesisStateElectra(t, numValidators)
			return st
		}},
		{name: "fulu", fieldCount: 38, newState: func(t *testing.T) state.BeaconState {
			st, _ := util.DeterministicGenesisStateFulu(t, numValidators)
			return st
		}},
		{name: "gloas", fieldCount: 46, newState: func(t *testing.T) state.BeaconState {
			st, _ := util.DeterministicGenesisStateGloas(t, numValidators)
			return st
		}},
	}
}
