package kv

import (
	"bytes"
	"fmt"
	"slices"
	"testing"

	"github.com/OffchainLabs/methodical-ssz/ssz"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/consensus-types/hdiff"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/golang/snappy"
	"go.etcd.io/bbolt"
)

func TestGetBaseAndDiffChain_CachedAncestors(t *testing.T) {
	for _, offset := range []primitives.Slot{0, 32, 1<<40 + 32, ^primitives.Slot(0) - 511} {
		t.Run(fmt.Sprintf("offset=%d", offset), func(t *testing.T) {
			db, states := setupStateDiffReadTree(t, offset, []int{9, 8, 7, 6, 5}, []primitives.Slot{0, 256, 288, 384, 448, 480})
			warm := db.stateDiffCache
			storedDiffs := make(map[primitives.Slot]hdiff.HdiffBytes)
			for rel := range states {
				if rel == 0 {
					continue
				}
				diff, err := db.getDiff(computeLevel(uint64(offset), offset+rel), uint64(offset+rel))
				require.NoError(t, err)
				storedDiffs[rel] = diff
			}
			for _, tt := range []struct {
				name      string
				target    primitives.Slot
				prepare   func(*testing.T, *Store)
				wantBase  primitives.Slot
				wantDiffs []primitives.Slot
				wantReads int
			}{
				{name: "nearest ancestor", target: 480, wantBase: 448, wantDiffs: []primitives.Slot{480}, wantReads: 1},
				{name: "cached target at first level", target: 256, wantBase: 256},
				{name: "cached target at middle level", target: 384, wantBase: 384},
				{name: "cached target at deepest cached level", target: 448, wantBase: 448},
				{name: "full snapshot", target: 0},
				{name: "duplicate ancestor slots", target: 288, wantBase: 256, wantDiffs: []primitives.Slot{288}, wantReads: 1},
				{
					name: "middle ancestor", target: 480, wantBase: 384, wantDiffs: []primitives.Slot{448, 480}, wantReads: 2,
					prepare: func(_ *testing.T, db *Store) { db.stateDiffCache.anchors[3] = anchor{} },
				},
				{
					name: "first ancestor", target: 480, wantBase: 256, wantDiffs: []primitives.Slot{384, 448, 480}, wantReads: 3,
					prepare: func(_ *testing.T, db *Store) { clear(db.stateDiffCache.anchors[2:]) },
				},
				{
					name: "only root cached", target: 480, wantDiffs: []primitives.Slot{256, 384, 448, 480}, wantReads: 4,
					prepare: func(_ *testing.T, db *Store) { clear(db.stateDiffCache.anchors[1:]) },
				},
				{
					name: "empty cache", target: 480, wantDiffs: []primitives.Slot{256, 384, 448, 480}, wantReads: 5,
					prepare: func(_ *testing.T, db *Store) { db.stateDiffCache.clearAnchors() },
				},
				{
					name: "empty level flags with stored diffs", target: 480, wantDiffs: []primitives.Slot{256, 384, 448, 480}, wantReads: 4,
					prepare: func(_ *testing.T, db *Store) { clear(db.stateDiffCache.levelsWithData[1:]) },
				},
				{
					name: "nil cache", target: 480, wantDiffs: []primitives.Slot{256, 384, 448, 480}, wantReads: 5,
					prepare: func(_ *testing.T, db *Store) { db.stateDiffCache = nil },
				},
				{
					name: "older anchor", target: 480, wantBase: 384, wantDiffs: []primitives.Slot{448, 480}, wantReads: 2,
					prepare: func(t *testing.T, db *Store) {
						st := states[448].Copy()
						require.NoError(t, st.SetSlot(offset+320))
						require.NoError(t, db.stateDiffCache.setAnchor(3, st))
					},
				},
				{
					name: "newer anchor", target: 480, wantBase: 384, wantDiffs: []primitives.Slot{448, 480}, wantReads: 2,
					prepare: func(t *testing.T, db *Store) {
						st := states[448].Copy()
						require.NoError(t, st.SetSlot(offset+449))
						require.NoError(t, db.stateDiffCache.setAnchor(3, st))
					},
				},
				{
					name: "anchors at wrong levels", target: 480, wantBase: 256, wantDiffs: []primitives.Slot{384, 448, 480}, wantReads: 3,
					prepare: func(_ *testing.T, db *Store) {
						c := db.stateDiffCache
						c.anchors[2], c.anchors[3] = c.anchors[3], c.anchors[2]
					},
				},
				{
					name: "empty matching anchor", target: 480, wantBase: 384, wantDiffs: []primitives.Slot{448, 480}, wantReads: 2,
					prepare: func(_ *testing.T, db *Store) { db.stateDiffCache.anchors[3].data = nil },
				},
				{
					name: "corrupt compressed anchor", target: 480, wantBase: 384, wantDiffs: []primitives.Slot{448, 480}, wantReads: 2,
					prepare: func(_ *testing.T, db *Store) { db.stateDiffCache.anchors[3].data = []byte{0xff} },
				},
				{
					name: "invalid anchor SSZ", target: 480, wantBase: 384, wantDiffs: []primitives.Slot{448, 480}, wantReads: 2,
					prepare: func(_ *testing.T, db *Store) { db.stateDiffCache.anchors[3].data = snappy.Encode(nil, altairKey) },
				},
				{
					name: "all anchors corrupt", target: 480, wantDiffs: []primitives.Slot{256, 384, 448, 480}, wantReads: 5,
					prepare: func(_ *testing.T, db *Store) {
						for i := range db.stateDiffCache.anchors {
							db.stateDiffCache.anchors[i].data = []byte{0xff}
						}
					},
				},
			} {
				t.Run(tt.name, func(t *testing.T) {
					db.stateDiffCache = &stateDiffCache{
						anchors: slices.Clone(warm.anchors), levelsWithData: slices.Clone(warm.levelsWithData), offset: uint64(offset),
					}
					if tt.prepare != nil {
						tt.prepare(t, db)
					}
					before := db.db.Stats().TxN
					base, chain, err := db.getBaseAndDiffChain(uint64(offset), offset+tt.target)
					require.NoError(t, err)
					require.Equal(t, tt.wantReads, db.db.Stats().TxN-before)
					require.Equal(t, offset+tt.wantBase, base.Slot())
					require.DeepSSZEqual(t, states[tt.wantBase].ToProto(), base.ToProto())
					require.Equal(t, len(tt.wantDiffs), len(chain))
					for i, rel := range tt.wantDiffs {
						require.DeepEqual(t, storedDiffs[rel], chain[i])
						base, err = hdiff.ApplyDiff(t.Context(), base, chain[i])
						require.NoError(t, err)
					}
					assertStateDiffRead(t, states[tt.target], base)
					if db.stateDiffCache == nil {
						return
					}

					got, err := db.stateByDiff(t.Context(), offset+tt.target)
					require.NoError(t, err)
					assertStateDiffRead(t, states[tt.target], got)
					require.NoError(t, got.UpdateBalancesAtIndex(0, 123))
					require.NoError(t, got.SetSlot(0))
					got, err = db.stateByDiff(t.Context(), offset+tt.target)
					require.NoError(t, err)
					assertStateDiffRead(t, states[tt.target], got)
				})
			}
		})
	}
}

func TestGetBaseAndDiffChain_TreeBoundaries(t *testing.T) {
	for _, tt := range []struct {
		name      string
		exponents []int
		slots     []primitives.Slot
		target    primitives.Slot
		wantBase  primitives.Slot
		wantDiffs int
	}{
		{name: "single exponent", exponents: []int{5}, slots: []primitives.Slot{0, 32}, target: 32, wantBase: 32},
		{name: "two levels with uncached leaf", exponents: []int{7, 5}, slots: []primitives.Slot{0, 32, 96}, target: 96, wantDiffs: 1},
		{name: "next full snapshot", exponents: []int{9, 8, 7, 6, 5}, slots: []primitives.Slot{0, 256, 384, 448, 480, 512}, target: 512, wantBase: 512},
		{name: "leaf in next tree", exponents: []int{9, 8, 7, 6, 5}, slots: []primitives.Slot{0, 256, 384, 448, 480, 512, 768, 800}, target: 800, wantBase: 768, wantDiffs: 1},
		{name: "old tree with newer cached anchors", exponents: []int{9, 8, 7, 6, 5}, slots: []primitives.Slot{0, 256, 384, 448, 480, 512, 768, 800}, target: 480, wantDiffs: 4},
	} {
		t.Run(tt.name, func(t *testing.T) {
			const offset = primitives.Slot(32)
			db, states := setupStateDiffReadTree(t, offset, tt.exponents, tt.slots)
			base, chain, err := db.getBaseAndDiffChain(uint64(offset), offset+tt.target)
			require.NoError(t, err)
			require.Equal(t, offset+tt.wantBase, base.Slot())
			require.Equal(t, tt.wantDiffs, len(chain))
			got, err := db.stateByDiff(t.Context(), offset+tt.target)
			require.NoError(t, err)
			assertStateDiffRead(t, states[tt.target], got)
		})
	}

	for _, tt := range []struct {
		name        string
		missing     []primitives.Slot
		strayAnchor bool
		wantMissing bool
	}{
		{name: "cached descendant bypasses empty intermediate levels", missing: []primitives.Slot{256, 384}},
		{name: "only leaf level has diffs", missing: []primitives.Slot{256, 384, 448}, wantMissing: true},
		{name: "ignore anchors for levels without data", missing: []primitives.Slot{256, 384}, strayAnchor: true, wantMissing: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			const offset = primitives.Slot(32)
			// Save valid diffs before removing their ancestors to simulate incomplete history.
			db, states := setupStateDiffReadTree(t, offset, []int{9, 8, 7, 6, 5}, []primitives.Slot{0, 256, 384, 448, 480})
			require.NoError(t, db.db.Update(func(tx *bbolt.Tx) error {
				bucket := tx.Bucket(stateDiffBucket)
				for _, rel := range tt.missing {
					level := computeLevel(uint64(offset), offset+rel)
					for _, suffix := range []string{stateSuffix, validatorSuffix, balancesSuffix} {
						if err := bucket.Delete(append(makeKeyForStateDiffTree(level, uint64(offset+rel)), suffix...)); err != nil {
							return err
						}
					}
				}
				return nil
			}))
			for _, rel := range tt.missing {
				level := computeLevel(uint64(offset), offset+rel)
				db.stateDiffCache.anchors[level] = anchor{}
				db.stateDiffCache.levelsWithData[level] = false
			}
			if tt.strayAnchor {
				db.stateDiffCache.anchors[3] = anchor{}
				stray := states[0].Copy()
				require.NoError(t, stray.SetSlot(offset+384))
				require.NoError(t, stray.UpdateBalancesAtIndex(0, 123))
				require.NoError(t, db.stateDiffCache.setAnchor(2, stray))
				require.Equal(t, false, db.stateDiffCache.levelHasData(2))
			}

			base, chain, err := db.getBaseAndDiffChain(uint64(offset), offset+480)
			if tt.wantMissing {
				require.ErrorIs(t, err, ErrNotFoundState)
				require.ErrorContains(t, "level 1 slot 288", err)
				require.IsNil(t, base)
				require.IsNil(t, chain)
			} else {
				require.NoError(t, err)
				require.Equal(t, offset+448, base.Slot())
				require.Equal(t, 1, len(chain))
				got, err := db.stateByDiff(t.Context(), offset+480)
				require.NoError(t, err)
				assertStateDiffRead(t, states[480], got)
				// Without the cached descendant, the missing ancestors are required again.
				db.stateDiffCache.clearAnchors()
			}
			got, err := db.stateByDiff(t.Context(), offset+480)
			require.ErrorIs(t, err, ErrNotFoundState)
			require.ErrorContains(t, "level 1 slot 288", err)
			require.IsNil(t, got)
		})
	}
}

func TestGetBaseAndDiffChain_BypassesCachedPrefix(t *testing.T) {
	db, states := setupStateDiffReadTree(t, 32, []int{9, 8, 7, 6, 5}, []primitives.Slot{0, 256, 384, 448, 480})
	db.stateDiffCache.anchors[0] = anchor{}
	require.NoError(t, db.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if err := bucket.Delete(makeKeyForStateDiffTree(0, 32)); err != nil {
			return err
		}
		for level, rel := range []uint64{256, 384, 448} {
			for _, suffix := range []string{stateSuffix, validatorSuffix, balancesSuffix} {
				if err := bucket.Delete(append(makeKeyForStateDiffTree(level+1, 32+rel), suffix...)); err != nil {
					return err
				}
			}
		}
		return nil
	}))
	for _, rel := range []primitives.Slot{448, 480} {
		t.Run(fmt.Sprintf("target=%d", rel), func(t *testing.T) {
			before := db.db.Stats().TxN
			got, err := db.stateByDiff(t.Context(), 32+rel)
			require.NoError(t, err)
			wantReads := 0
			if rel == 480 {
				wantReads = 1
			}
			require.Equal(t, wantReads, db.db.Stats().TxN-before)
			assertStateDiffRead(t, states[rel], got)
		})
	}
}

func TestGetBaseAndDiffChain_Errors(t *testing.T) {
	for _, tt := range []struct {
		name    string
		target  primitives.Slot
		prepare func(*testing.T, *Store)
		wantErr string
	}{
		{name: "before offset", target: 31, wantErr: ErrSlotBeforeOffset.Error()},
		{name: "not in tree", target: 32 + 481, wantErr: "slot not in tree"},
		{name: "missing next snapshot", target: 32 + 512, wantErr: errSnapshotNotFound.Error()},
		{
			name: "missing bucket", target: 32 + 480, wantErr: bbolt.ErrBucketNotFound.Error(),
			prepare: func(t *testing.T, db *Store) {
				db.stateDiffCache.clearAnchors()
				deleteStateDiffBucket(t, db)
			},
		},
		{
			name: "missing root without usable anchor", target: 32 + 480, wantErr: errSnapshotNotFound.Error(),
			prepare: func(t *testing.T, db *Store) {
				db.stateDiffCache.clearAnchors()
				require.NoError(t, db.db.Update(func(tx *bbolt.Tx) error {
					return tx.Bucket(stateDiffBucket).Delete(makeKeyForStateDiffTree(0, 32))
				}))
			},
		},
		{
			name: "corrupt root without usable anchor", target: 32 + 480, wantErr: "snappy",
			prepare: func(t *testing.T, db *Store) {
				db.stateDiffCache.clearAnchors()
				require.NoError(t, db.db.Update(func(tx *bbolt.Tx) error {
					return tx.Bucket(stateDiffBucket).Put(makeKeyForStateDiffTree(0, 32), []byte{0xff})
				}))
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := setupStateDiffReadTree(t, 32, []int{9, 8, 7, 6, 5}, []primitives.Slot{0, 256, 384, 448, 480})
			if tt.prepare != nil {
				tt.prepare(t, db)
			}
			base, chain, err := db.getBaseAndDiffChain(32, tt.target)
			require.ErrorContains(t, tt.wantErr, err)
			require.IsNil(t, base)
			require.IsNil(t, chain)
			got, err := db.stateByDiff(t.Context(), tt.target)
			require.ErrorContains(t, tt.wantErr, err)
			require.IsNil(t, got)
		})
	}

	for _, suffix := range []string{stateSuffix, validatorSuffix, balancesSuffix} {
		for _, corrupt := range []bool{false, true} {
			t.Run(fmt.Sprintf("required suffix=%s/corrupt=%t", suffix, corrupt), func(t *testing.T) {
				db, _ := setupStateDiffReadTree(t, 32, []int{9, 8, 7, 6, 5}, []primitives.Slot{0, 256, 384, 448, 480})
				require.NoError(t, db.db.Update(func(tx *bbolt.Tx) error {
					bucket := tx.Bucket(stateDiffBucket)
					key := append(makeKeyForStateDiffTree(4, 32+480), suffix...)
					if corrupt {
						return bucket.Put(key, []byte{0xff})
					}
					return bucket.Delete(key)
				}))
				base, chain, err := db.getBaseAndDiffChain(32, 32+480)
				if corrupt {
					require.NoError(t, err)
					require.Equal(t, primitives.Slot(32+448), base.Slot())
					require.Equal(t, 1, len(chain))
				} else {
					require.ErrorContains(t, "diff not found", err)
					require.IsNil(t, base)
					require.IsNil(t, chain)
				}
				got, err := db.stateByDiff(t.Context(), 32+480)
				require.NotNil(t, err)
				require.IsNil(t, got)
			})
		}
	}
}

func TestGetBaseAndDiffChain_ForkTransitions(t *testing.T) {
	forks := version.AllIncludingUnreleased()
	for i, fork := range forks {
		t.Run(version.String(fork), func(t *testing.T) {
			original := flags.Get()
			setStateDiffExponents([]int{8, 7, 6, 5})
			t.Cleanup(func() { flags.Init(original) })
			db := setupDB(t)
			require.NoError(t, setOffsetInDB(db, 32))
			nextFork := forks[min(i+1, len(forks)-1)]
			base, _ := createState(t, 32, fork)
			first, _ := createState(t, 32+128, fork)
			middle, _ := createState(t, 32+192, nextFork)
			target, _ := createState(t, 32+224, nextFork)
			if fork == version.Fulu {
				// Initializing the Gloas PTC window requires active validators.
				base, _ = util.DeterministicGenesisStateFulu(t, 64)
				require.NoError(t, base.SetSlot(32))
				first = base.Copy()
				require.NoError(t, first.SetSlot(32+128))
				var err error
				middle, err = gloas.UpgradeToGloas(t.Context(), first.Copy())
				require.NoError(t, err)
				require.NoError(t, middle.SetSlot(32+192))
				target = middle.Copy()
				require.NoError(t, target.SetSlot(32+224))
			}
			for _, st := range []state.BeaconState{base, first, middle, target} {
				require.NoError(t, db.saveStateByDiff(t.Context(), st))
			}
			warm := db.stateDiffCache
			for _, tt := range []struct {
				name      string
				clearFrom int
				wantBase  state.BeaconState
				wantDiffs int
			}{
				{name: "anchor after upgrade", clearFrom: -1, wantBase: middle, wantDiffs: 1},
				{name: "anchor before upgrade", clearFrom: 2, wantBase: first, wantDiffs: 2},
				{name: "cold reconstruction", clearFrom: 0, wantBase: base, wantDiffs: 3},
			} {
				t.Run(tt.name, func(t *testing.T) {
					db.stateDiffCache = &stateDiffCache{
						anchors: slices.Clone(warm.anchors), levelsWithData: slices.Clone(warm.levelsWithData), offset: 32,
					}
					if tt.clearFrom >= 0 {
						clear(db.stateDiffCache.anchors[tt.clearFrom:])
					}
					gotBase, chain, err := db.getBaseAndDiffChain(32, target.Slot())
					require.NoError(t, err)
					require.Equal(t, tt.wantBase.Slot(), gotBase.Slot())
					require.Equal(t, tt.wantBase.Version(), gotBase.Version())
					require.Equal(t, tt.wantDiffs, len(chain))
					got, err := db.stateByDiff(t.Context(), target.Slot())
					require.NoError(t, err)
					assertStateDiffRead(t, target, got)
				})
			}
		})
	}
}

func TestGetBaseAndDiffChain_ConcurrentCacheClear(t *testing.T) {
	db, states := setupStateDiffReadTree(t, 32, []int{9, 8, 7, 6, 5}, []primitives.Slot{0, 256, 384, 448, 480})
	wantRoot, err := states[480].HashTreeRoot(t.Context())
	require.NoError(t, err)
	errCh := make(chan error, 2)
	go func() {
		for range 10 {
			db.stateDiffCache.clearAnchors()
			for level, rel := range []primitives.Slot{256, 384, 448} {
				if err := db.stateDiffCache.setAnchor(level+1, states[rel]); err != nil {
					errCh <- err
					return
				}
			}
		}
		errCh <- nil
	}()
	go func() {
		for range 10 {
			got, err := db.stateByDiff(t.Context(), 32+480)
			if err != nil {
				errCh <- err
				return
			}
			root, err := got.HashTreeRoot(t.Context())
			if err != nil {
				errCh <- err
				return
			}
			if root != wantRoot {
				errCh <- fmt.Errorf("reconstructed root %x, want %x", root, wantRoot)
				return
			}
		}
		errCh <- nil
	}()
	err1, err2 := <-errCh, <-errCh
	require.NoError(t, err1)
	require.NoError(t, err2)
}

func setupStateDiffReadTree(t *testing.T, offset primitives.Slot, exponents []int, relativeSlots []primitives.Slot) (*Store, map[primitives.Slot]state.BeaconState) {
	t.Helper()
	original := flags.Get()
	setStateDiffExponents(exponents)
	t.Cleanup(func() { flags.Init(original) })
	db := setupDB(t)
	require.NoError(t, setOffsetInDB(db, uint64(offset)))
	st, _ := util.DeterministicGenesisStateAltair(t, 8)
	states := make(map[primitives.Slot]state.BeaconState, len(relativeSlots))
	for i, rel := range relativeSlots {
		st = st.Copy()
		require.NoError(t, st.SetSlot(offset+rel))
		require.NoError(t, st.UpdateBalancesAtIndex(0, uint64(32_000_000_000+i*37)))
		val, err := st.ValidatorAtIndex(0)
		require.NoError(t, err)
		val.EffectiveBalance -= uint64(i)
		require.NoError(t, st.UpdateValidatorAtIndex(0, val))
		scores, err := st.InactivityScores()
		require.NoError(t, err)
		scores[0] = uint64(i)
		require.NoError(t, st.SetInactivityScores(scores))
		require.NoError(t, st.UpdateSlashingsAtIndex(0, uint64(i*i)))
		require.NoError(t, db.saveStateByDiff(t.Context(), st))
		states[rel] = st
	}
	return db, states
}

func assertStateDiffRead(t *testing.T, want, got state.BeaconState) {
	t.Helper()
	require.NotNil(t, got)
	wantSSZ, err := want.MarshalSSZ()
	require.NoError(t, err)
	gotSSZ, err := got.MarshalSSZ()
	require.NoError(t, err)
	require.DeepEqual(t, wantSSZ, gotSSZ)
	wantRoot, err := want.HashTreeRoot(t.Context())
	require.NoError(t, err)
	gotRoot, err := got.HashTreeRoot(t.Context())
	require.NoError(t, err)
	require.Equal(t, wantRoot, gotRoot)
}

func TestEncodeStateWithKey(t *testing.T) {
	st, err := util.NewBeaconStateElectra()
	require.NoError(t, err)
	pb, ok := st.ToProtoUnsafe().(ssz.Marshaler)
	require.Equal(t, true, ok)

	t.Run("marshals into the prefixed buffer", func(t *testing.T) {
		// A regrow would mean SizeSSZ under-reported, silently bringing the extra copy back.
		buf := make([]byte, len(ElectraKey), len(ElectraKey)+pb.SizeSSZ())
		copy(buf, ElectraKey)
		out, err := pb.MarshalSSZTo(buf)
		require.NoError(t, err)
		require.Equal(t, true, &buf[0] == &out[0], "MarshalSSZTo reallocated the buffer")
		require.Equal(t, cap(buf), len(out))
	})

	t.Run("allocates only the buffer and the snappy output", func(t *testing.T) {
		// Ensures memory allocations are as expected.
		allocs := testing.AllocsPerRun(5, func() {
			if _, err := encodeProtoWithKey(version.Electra, pb); err != nil {
				t.Fatal(err)
			}
		})

		// Allocations: 1 for the prefixed SSZ buffer, 1 for the snappy output.
		require.Equal(t, 2.0, allocs)
	})

	t.Run("round trip", func(t *testing.T) {
		stateBytes, err := st.MarshalSSZ()
		require.NoError(t, err)
		want, err := addKey(version.Electra, stateBytes)
		require.NoError(t, err)

		got, err := encodeStateWithKey(st)
		require.NoError(t, err)
		decoded, err := snappy.Decode(nil, got)
		require.NoError(t, err)
		require.DeepEqual(t, want, decoded)
	})
}

func TestMakeKeyForStateDiffTree_KeyLength(t *testing.T) {
	// Existing databases store state diff keys at this exact length. Changing
	// it would make all persisted keys unreadable on restart.
	key := makeKeyForStateDiffTree(0, 0)
	require.Equal(t, 16, len(key))

	key = makeKeyForStateDiffTree(3, 1<<40)
	require.Equal(t, 16, len(key))
}

func TestIsStateDiffTreeKey(t *testing.T) {
	setStateDiffExponents([]int{7, 5})

	// A tree key with a level byte, a slot, and zero padding, then the same with an entry suffix.
	treeKey := makeKeyForStateDiffTree(1, 320)
	suffixedKey := append(bytes.Clone(treeKey), stateSuffix...)

	// A key that is shaped like a tree key up to its padding, which a tree key never sets.
	paddedKey := bytes.Clone(treeKey)
	paddedKey[stateDiffTreeKeySlotEnd] = 'x'

	tests := []struct {
		name string
		key  []byte
		want bool
	}{
		{name: "tree key", key: treeKey, want: true},
		{name: "suffixed tree key", key: suffixedKey, want: true},
		{name: "offset metadata key", key: offsetKey, want: false},
		{name: "exponents metadata key", key: exponentsKey, want: false},
		{name: "metadata key longer than a tree key", key: []byte("a-long-metadata-key-here"), want: false},
		{name: "level byte out of range", key: append([]byte("m"), make([]byte, stateDiffTreeKeyLength)...), want: false},
		{name: "non-zero padding", key: paddedKey, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isStateDiffTreeKey(tt.key))
		})
	}
}

func TestStateKeyByVersion_AllVersions(t *testing.T) {
	for _, v := range version.All() {
		t.Run(version.String(v), func(t *testing.T) {
			key, ok := stateKeyByVersion[v]
			require.Equal(t, true, ok)
			require.NotEqual(t, 0, len(key))
		})
	}
}

func BenchmarkEncodeProtoWithKey(b *testing.B) {
	st, err := util.NewBeaconStateElectra()
	require.NoError(b, err)
	pb := st.ToProtoUnsafe().(ssz.Marshaler)

	b.Run("append-copy", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			raw, err := pb.MarshalSSZ()
			require.NoError(b, err)
			_ = snappy.Encode(nil, append(ElectraKey, raw...))
		}
	})
	b.Run("marshal-into-key", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := encodeProtoWithKey(version.Electra, pb)
			require.NoError(b, err)
		}
	})
}

// addKey is the reference encoding: version key followed by the raw SSZ bytes.
func addKey(v int, bytes []byte) ([]byte, error) {
	key, ok := stateKeyByVersion[v]
	if !ok {
		return nil, fmt.Errorf("no state key for fork %s", version.String(v))
	}
	return append(append([]byte{}, key...), bytes...), nil
}
