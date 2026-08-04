package fieldtrie

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// progressiveRootsEnabled turns on progressive merkleization so the
// stateutil reference helpers return the progressive form.
func progressiveRootsEnabled(t *testing.T) {
	t.Helper()
	reset := features.InitWithReset(&features.Flags{EnableProgressiveSSZ: true})
	t.Cleanup(reset)
}

func TestProgressiveFieldTrie(t *testing.T) {
	progressiveRootsEnabled(t)

	t.Run("build", func(t *testing.T) {
		t.Run("nil elements", func(t *testing.T) {
			data, err := buildProgressiveTrie(types.Validators, nil)
			require.NoError(t, err)
			if data != nil {
				t.Fatal("nil elements produced progressive trie data")
			}
		})

		t.Run("unsupported field", func(t *testing.T) {
			_, err := buildProgressiveTrie(types.GenesisTime, []uint64{1})
			require.ErrorContains(t, "field converters", err)
		})

		t.Run("validators", func(t *testing.T) {
			for _, count := range []int{0, 1, 2, 5, 6, 21, 22, 85, 86} {
				t.Run(fmt.Sprintf("count_%d", count), func(t *testing.T) {
					validators := progressiveTestValidators(count)
					fieldTrie := newProgressiveTestFieldTrie(t, types.Validators, types.CompositeArray, validators)
					root, err := fieldTrie.TrieRoot()
					require.NoError(t, err)
					expected, err := stateutil.ValidatorRegistryRoot(version.Gloas, validators)
					require.NoError(t, err)
					require.Equal(t, expected, root)
				})
			}
		})

		t.Run("balances", func(t *testing.T) {
			for _, count := range []int{0, 1, 4, 5, 20, 21, 84, 85} {
				t.Run(fmt.Sprintf("count_%d", count), func(t *testing.T) {
					balances := progressiveTestBalances(count)
					fieldTrie := newProgressiveTestFieldTrie(t, types.Balances, types.CompressedArray, balances)
					root, err := fieldTrie.TrieRoot()
					require.NoError(t, err)
					expected, err := stateutil.Uint64ListRoot(version.Gloas, balances)
					require.NoError(t, err)
					require.Equal(t, expected, root)
				})
			}
		})
	})

	t.Run("root", func(t *testing.T) {
		t.Run("invalid trie", func(t *testing.T) {
			fieldTrie := &FieldTrie{dataType: types.CompositeArray}
			_, err := fieldTrie.progressiveTrieRoot()
			require.ErrorIs(t, err, ErrInvalidFieldTrie)
		})

		t.Run("invalid length mixin", func(t *testing.T) {
			fieldTrie := &FieldTrie{
				dataType:        types.DataType(-1),
				progressiveData: &progressiveNodesData{spine: [][32]byte{{1}}},
			}
			_, err := fieldTrie.progressiveTrieRoot()
			require.ErrorContains(t, "root with mixin", err)
		})

		t.Run("overlay", func(t *testing.T) {
			validators := progressiveTestValidators(6)
			base := newProgressiveTestFieldTrie(t, types.Validators, types.CompositeArray, validators)
			copied := base.CopyTrie()

			validators[5].EffectiveBalance++
			validators[1].EffectiveBalance++
			overlay, recomputedRoot, err := base.RecomputeTrie([]uint64{5, 1}, validators)
			require.NoError(t, err)
			runtime.KeepAlive(copied)

			root, err := overlay.TrieRoot()
			require.NoError(t, err)
			require.Equal(t, recomputedRoot, root)
			requireProgressiveFreshRoot(t, overlay, validators, root)
		})
	})

	t.Run("recompute owned", func(t *testing.T) {
		t.Run("validator", func(t *testing.T) {
			validators := progressiveTestValidators(86)
			fieldTrie := newProgressiveTestFieldTrie(t, types.Validators, types.CompositeArray, validators)

			validators[42].EffectiveBalance++
			validators[2].EffectiveBalance++
			returned, root, err := fieldTrie.RecomputeTrie([]uint64{42, 2}, validators)
			require.NoError(t, err)
			require.Equal(t, fieldTrie, returned)
			requireProgressiveFreshRoot(t, returned, validators, root)
		})

		t.Run("balances share a chunk", func(t *testing.T) {
			balances := progressiveTestBalances(86)
			fieldTrie := newProgressiveTestFieldTrie(t, types.Balances, types.CompressedArray, balances)

			balances[40]++
			balances[41]++
			returned, root, err := fieldTrie.RecomputeTrie([]uint64{40, 41}, balances)
			require.NoError(t, err)
			require.Equal(t, fieldTrie, returned)
			requireProgressiveFreshRoot(t, returned, balances, root)
		})
	})

	t.Run("append", func(t *testing.T) {
		t.Run("validators", func(t *testing.T) {
			for _, initialCount := range []int{0, 1, 2, 5, 21, 85} {
				t.Run(fmt.Sprintf("initial_count_%d", initialCount), func(t *testing.T) {
					validators := progressiveTestValidators(initialCount)
					fieldTrie := newProgressiveTestFieldTrie(t, types.Validators, types.CompositeArray, validators)

					validators = append(validators, progressiveTestValidators(1)[0])
					validators[len(validators)-1].EffectiveBalance = uint64(len(validators) * 1000)
					returned, root, err := fieldTrie.RecomputeTrie([]uint64{uint64(len(validators) - 1)}, validators)
					require.NoError(t, err)
					requireProgressiveFreshRoot(t, returned, validators, root)
				})
			}
		})

		t.Run("balances new chunks", func(t *testing.T) {
			for _, initialChunkCount := range []int{1, 5, 21, 85} {
				t.Run(fmt.Sprintf("initial_chunks_%d", initialChunkCount), func(t *testing.T) {
					balances := progressiveTestBalances(initialChunkCount * 4)
					fieldTrie := newProgressiveTestFieldTrie(t, types.Balances, types.CompressedArray, balances)

					balances = append(balances, uint64(len(balances)+1))
					returned, root, err := fieldTrie.RecomputeTrie([]uint64{uint64(len(balances) - 1)}, balances)
					require.NoError(t, err)
					requireProgressiveFreshRoot(t, returned, balances, root)
				})
			}
		})

		t.Run("balance existing chunk updates length", func(t *testing.T) {
			balances := progressiveTestBalances(3)
			fieldTrie := newProgressiveTestFieldTrie(t, types.Balances, types.CompressedArray, balances)

			balances = append(balances, 4)
			returned, root, err := fieldTrie.RecomputeTrie([]uint64{3}, balances)
			require.NoError(t, err)
			requireProgressiveFreshRoot(t, returned, balances, root)
		})
	})

	t.Run("copy on write", func(t *testing.T) {
		t.Run("owned source", func(t *testing.T) {
			validators := progressiveTestValidators(22)
			fieldTrie := newProgressiveTestFieldTrie(t, types.Validators, types.CompositeArray, validators)
			originalRoot, err := fieldTrie.TrieRoot()
			require.NoError(t, err)
			copied := fieldTrie.CopyTrie()

			validators[1].EffectiveBalance++
			overlay, overlayRoot, err := fieldTrie.RecomputeTrie([]uint64{1}, validators)
			require.NoError(t, err)
			runtime.KeepAlive(copied)
			require.Equal(t, fieldTrie, overlay.base)
			requireProgressiveFreshRoot(t, overlay, validators, overlayRoot)

			copyRoot, err := copied.TrieRoot()
			require.NoError(t, err)
			require.Equal(t, originalRoot, copyRoot)
		})

		t.Run("overlay source", func(t *testing.T) {
			validators := progressiveTestValidators(22)
			fieldTrie := newProgressiveTestFieldTrie(t, types.Validators, types.CompositeArray, validators)
			baseCopy := fieldTrie.CopyTrie()

			validators[1].EffectiveBalance++
			overlay, _, err := fieldTrie.RecomputeTrie([]uint64{1}, validators)
			require.NoError(t, err)
			runtime.KeepAlive(baseCopy)

			overlayCopy := overlay.CopyTrie()
			overlayCopyRoot, err := overlayCopy.TrieRoot()
			require.NoError(t, err)
			validators[6].EffectiveBalance++
			forkedOverlay, root, err := overlay.RecomputeTrie([]uint64{6}, validators)
			require.NoError(t, err)

			if forkedOverlay.progressiveOverridesData == overlay.progressiveOverridesData {
				t.Fatal("forked overlay shares mutable override data with its source")
			}
			require.Equal(t, overlay.base, forkedOverlay.base)
			requireProgressiveFreshRoot(t, forkedOverlay, validators, root)

			unchangedCopyRoot, err := overlayCopy.TrieRoot()
			require.NoError(t, err)
			require.Equal(t, overlayCopyRoot, unchangedCopyRoot)
		})

		t.Run("append boundary", func(t *testing.T) {
			t.Run("validators", func(t *testing.T) {
				validators := progressiveTestValidators(85)
				fieldTrie := newProgressiveTestFieldTrie(t, types.Validators, types.CompositeArray, validators)
				originalRoot, err := fieldTrie.TrieRoot()
				require.NoError(t, err)
				copied := fieldTrie.CopyTrie()

				validators = append(validators, progressiveTestValidators(1)[0])
				validators[85].EffectiveBalance = 86_000
				overlay, root, err := fieldTrie.RecomputeTrie([]uint64{85}, validators)
				require.NoError(t, err)
				runtime.KeepAlive(copied)
				require.Equal(t, fieldTrie, overlay.base)
				requireProgressiveFreshRoot(t, overlay, validators, root)

				copiedRoot, err := copied.TrieRoot()
				require.NoError(t, err)
				require.Equal(t, originalRoot, copiedRoot)
			})

			t.Run("balances", func(t *testing.T) {
				balances := progressiveTestBalances(85 * 4)
				fieldTrie := newProgressiveTestFieldTrie(t, types.Balances, types.CompressedArray, balances)
				originalRoot, err := fieldTrie.TrieRoot()
				require.NoError(t, err)
				copied := fieldTrie.CopyTrie()

				balances = append(balances, 341_000)
				overlay, root, err := fieldTrie.RecomputeTrie([]uint64{340}, balances)
				require.NoError(t, err)
				runtime.KeepAlive(copied)
				require.Equal(t, fieldTrie, overlay.base)
				requireProgressiveFreshRoot(t, overlay, balances, root)

				copiedRoot, err := copied.TrieRoot()
				require.NoError(t, err)
				require.Equal(t, originalRoot, copiedRoot)
			})
		})
	})

	t.Run("promotion", func(t *testing.T) {
		t.Run("accumulated leaves", func(t *testing.T) {
			validators := progressiveTestValidators(86)
			fieldTrie := newProgressiveTestFieldTrie(t, types.Validators, types.CompositeArray, validators)
			copied := fieldTrie.CopyTrie()
			fieldTrie.promotionThreshold = 1

			validators[1].EffectiveBalance++
			overlay, overlayRoot, err := fieldTrie.RecomputeTrie([]uint64{1}, validators)
			require.NoError(t, err)
			runtime.KeepAlive(copied)
			requireProgressiveFreshRoot(t, overlay, validators, overlayRoot)

			validators[6].EffectiveBalance++
			overlay, overlayRoot, err = overlay.RecomputeTrie([]uint64{6}, validators)
			require.NoError(t, err)
			requireProgressiveFreshRoot(t, overlay, validators, overlayRoot)

			validators[22].EffectiveBalance++
			promoted, promotedRoot, err := overlay.RecomputeTrie([]uint64{22}, validators)
			require.NoError(t, err)
			if promoted.base != nil {
				t.Fatal("overlay was not promoted to owned storage")
			}
			requireProgressiveFreshRoot(t, promoted, validators, promotedRoot)
		})

		t.Run("single large update", func(t *testing.T) {
			validators := progressiveTestValidators(22)
			fieldTrie := newProgressiveTestFieldTrie(t, types.Validators, types.CompositeArray, validators)
			copied := fieldTrie.CopyTrie()
			fieldTrie.promotionThreshold = 1

			validators[1].EffectiveBalance++
			validators[6].EffectiveBalance++
			promoted, root, err := fieldTrie.RecomputeTrie([]uint64{1, 6}, validators)
			require.NoError(t, err)
			runtime.KeepAlive(copied)
			if promoted.base != nil {
				t.Fatal("large overlay update was not rebuilt into owned storage")
			}
			requireProgressiveFreshRoot(t, promoted, validators, root)
		})
	})

	t.Run("rebuild", func(t *testing.T) {
		t.Run("overlay", func(t *testing.T) {
			validators := progressiveTestValidators(22)
			fieldTrie := newProgressiveTestFieldTrie(t, types.Validators, types.CompositeArray, validators)
			copied := fieldTrie.CopyTrie()

			validators[0].EffectiveBalance++
			overlay, _, err := fieldTrie.RecomputeTrie([]uint64{0}, validators)
			require.NoError(t, err)
			runtime.KeepAlive(copied)
			if overlay.base == nil {
				t.Fatal("shared trie update did not create an overlay")
			}

			replacement := progressiveTestValidators(86)
			rebuilt, root, err := overlay.RecomputeTrie(nil, replacement)
			require.NoError(t, err)
			if rebuilt.base != nil {
				t.Fatal("full rebuild retained an overlay base")
			}
			requireProgressiveFreshRoot(t, rebuilt, replacement, root)
		})

		t.Run("unsupported field", func(t *testing.T) {
			fieldTrie := &FieldTrie{field: types.GenesisTime, dataType: types.BasicArray}
			_, err := fieldTrie.rebuildProgressiveFromScratch([]uint64{1})
			require.ErrorContains(t, "build progressive trie", err)
		})

		t.Run("nil elements", func(t *testing.T) {
			fieldTrie := newProgressiveTestFieldTrie(
				t,
				types.Validators,
				types.CompositeArray,
				progressiveTestValidators(1),
			)
			_, err := fieldTrie.rebuildProgressiveFromScratch(nil)
			require.ErrorIs(t, err, ErrEmptyFieldTrie)
		})
	})

	t.Run("errors", func(t *testing.T) {
		t.Run("invalid index", func(t *testing.T) {
			validators := progressiveTestValidators(1)
			fieldTrie := newProgressiveTestFieldTrie(t, types.Validators, types.CompositeArray, validators)
			returned, _, err := fieldTrie.RecomputeTrie([]uint64{fieldparams.ValidatorRegistryLimit}, validators)
			require.ErrorContains(t, "validate indices", err)
			require.Equal(t, fieldTrie, returned)
		})

		t.Run("owned compressed indices", func(t *testing.T) {
			fieldTrie := &FieldTrie{field: types.Validators, dataType: types.CompressedArray}
			_, err := fieldTrie.recomputeProgressiveOwned(progressiveTestValidators(1), []uint64{0})
			require.ErrorContains(t, "compressed indices to chunks", err)
		})

		t.Run("owned field converter", func(t *testing.T) {
			fieldTrie := &FieldTrie{field: types.GenesisTime, dataType: types.BasicArray}
			_, err := fieldTrie.recomputeProgressiveOwned([]uint64{1}, []uint64{0})
			require.ErrorContains(t, "field converters", err)
		})

		t.Run("owned length mixin", func(t *testing.T) {
			validators := progressiveTestValidators(1)
			fieldTrie := newProgressiveTestFieldTrie(t, types.Validators, types.CompositeArray, validators)
			fieldTrie.dataType = types.DataType(-1)
			validators[0].EffectiveBalance++
			_, err := fieldTrie.recomputeProgressiveOwned(validators, []uint64{0})
			require.ErrorContains(t, "root with mixin", err)
		})

		t.Run("overlay compressed indices", func(t *testing.T) {
			fieldTrie := &FieldTrie{field: types.Validators, dataType: types.CompressedArray}
			_, err := fieldTrie.recomputeProgressiveOverlay(progressiveTestValidators(1), []uint64{0})
			require.ErrorContains(t, "compressed indices to chunks", err)
		})

		t.Run("overlay field converter", func(t *testing.T) {
			fieldTrie := &FieldTrie{field: types.GenesisTime, dataType: types.BasicArray}
			_, err := fieldTrie.recomputeProgressiveOverlay([]uint64{1}, []uint64{0})
			require.ErrorContains(t, "field converters", err)
		})

		t.Run("overlay length mixin", func(t *testing.T) {
			validators := progressiveTestValidators(1)
			base := newProgressiveTestFieldTrie(t, types.Validators, types.CompositeArray, validators)
			overlay := base.fork()
			overlay.dataType = types.DataType(-1)
			validators[0].EffectiveBalance++
			_, err := overlay.recomputeProgressiveOverlay(validators, []uint64{0})
			require.ErrorContains(t, "root with mixin", err)
		})
	})
}

func TestProgressiveOverlayReads(t *testing.T) {
	progressiveRootsEnabled(t)
	validators := progressiveTestValidators(6)
	base := newProgressiveTestFieldTrie(t, types.Validators, types.CompositeArray, validators)
	overlay := base.fork()

	t.Run("node override", func(t *testing.T) {
		position := progressiveNodePosition{subtree: 0, level: 0, index: 0}
		override := [32]byte{99}
		overlay.progressiveOverridesData.nodes[position] = override
		require.Equal(t, override, overlay.readProgressiveOverlayNode(0, 0, 0))
	})

	t.Run("node base fallback", func(t *testing.T) {
		require.Equal(
			t,
			base.progressiveData.readNode(1, 0, 0),
			overlay.readProgressiveOverlayNode(1, 0, 0),
		)
	})

	t.Run("negative spine level", func(t *testing.T) {
		require.Equal(t, [32]byte{}, overlay.readProgressiveOverlaySpine(-1))
	})

	t.Run("spine override", func(t *testing.T) {
		override := [32]byte{88}
		overlay.progressiveOverridesData.spine[0] = override
		require.Equal(t, override, overlay.readProgressiveOverlaySpine(0))
	})

	t.Run("spine base fallback", func(t *testing.T) {
		require.Equal(t, base.progressiveData.spine[1], overlay.readProgressiveOverlaySpine(1))
	})

	t.Run("spine beyond base", func(t *testing.T) {
		require.Equal(t, [32]byte{}, overlay.readProgressiveOverlaySpine(len(base.progressiveData.spine)))
	})
}

func TestProgressiveNodesData(t *testing.T) {
	t.Run("root", func(t *testing.T) {
		t.Run("nil", func(t *testing.T) {
			var data *progressiveNodesData
			require.Equal(t, [32]byte{}, data.root())
		})

		t.Run("empty", func(t *testing.T) {
			data := &progressiveNodesData{}
			require.Equal(t, [32]byte{}, data.root())
		})

		t.Run("populated", func(t *testing.T) {
			want := [32]byte{1}
			data := &progressiveNodesData{spine: [][32]byte{want}}
			require.Equal(t, want, data.root())
		})
	})

	t.Run("read node", func(t *testing.T) {
		leaves := [][32]byte{{1}, {2}, {3}}
		subtree := buildProgressiveSubtree(leaves, 2)
		data := &progressiveNodesData{subtrees: []*progressiveSubtreeData{subtree, nil}}

		tests := []struct {
			name     string
			subtree  int
			level    uint64
			index    uint64
			expected [32]byte
		}{
			{name: "existing", subtree: 0, level: 0, index: 1, expected: leaves[1]},
			{name: "negative subtree", subtree: -1, level: 0, expected: trie.ZeroHashes[0]},
			{name: "subtree out of bounds", subtree: 2, level: 1, expected: trie.ZeroHashes[1]},
			{name: "nil subtree", subtree: 1, level: 2, expected: trie.ZeroHashes[2]},
			{name: "level out of bounds", subtree: 0, level: 3, expected: trie.ZeroHashes[3]},
			{name: "index out of bounds", subtree: 0, level: 0, index: 3, expected: trie.ZeroHashes[0]},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				require.Equal(t, test.expected, data.readNode(test.subtree, test.level, test.index))
			})
		}
	})

	t.Run("recompute spine", func(t *testing.T) {
		t.Run("negative start", func(t *testing.T) {
			data := &progressiveNodesData{spine: [][32]byte{{1}}}
			data.recomputeSpineFrom(-1)
			require.DeepEqual(t, [][32]byte{{1}}, data.spine)
		})

		t.Run("extends spine", func(t *testing.T) {
			first := buildProgressiveSubtree([][32]byte{{1}}, 0)
			second := buildProgressiveSubtree([][32]byte{{2}}, 2)
			data := &progressiveNodesData{subtrees: []*progressiveSubtreeData{first, second}}

			data.recomputeSpineFrom(1)

			secondSpine := progressiveTestHashPair(second.nodes[second.offsets[2]], [32]byte{})
			want := progressiveTestHashPair(first.nodes[first.offsets[0]], secondSpine)
			require.Equal(t, 2, len(data.spine))
			require.Equal(t, secondSpine, data.spine[1])
			require.Equal(t, want, data.spine[0])
		})

		t.Run("reuses successor", func(t *testing.T) {
			first := buildProgressiveSubtree([][32]byte{{1}}, 0)
			second := buildProgressiveSubtree([][32]byte{{2}}, 2)
			data := &progressiveNodesData{subtrees: []*progressiveSubtreeData{first, second}}
			data.recomputeSpineFrom(1)
			successor := data.spine[1]

			first.nodes[0] = [32]byte{3}
			data.recomputeSpineFrom(0)

			require.Equal(t, successor, data.spine[1])
			require.Equal(t, progressiveTestHashPair(first.nodes[0], successor), data.spine[0])
		})
	})

	t.Run("ensure leaf capacity", func(t *testing.T) {
		t.Run("adds subtree and spine", func(t *testing.T) {
			data := &progressiveNodesData{}
			data.ensureLeafCapacity(2, 1)
			require.Equal(t, 3, len(data.subtrees))
			require.Equal(t, 3, len(data.spine))
			require.Equal(t, uint64(2), data.subtrees[2].levelSize(0))
		})

		t.Run("existing capacity is unchanged", func(t *testing.T) {
			subtree := buildProgressiveSubtree([][32]byte{{1}, {2}}, 2)
			data := &progressiveNodesData{subtrees: []*progressiveSubtreeData{nil, subtree}}
			data.ensureLeafCapacity(1, 2)
			if data.subtrees[1] != subtree {
				t.Fatal("sufficient subtree capacity was reallocated")
			}
		})

		t.Run("grows existing subtree", func(t *testing.T) {
			leaf := [32]byte{1}
			subtree := buildProgressiveSubtree([][32]byte{leaf}, 2)
			oldRoot := subtree.nodes[subtree.offsets[2]]
			data := &progressiveNodesData{
				subtrees: []*progressiveSubtreeData{nil, subtree},
				spine:    make([][32]byte, 2),
			}

			data.ensureLeafCapacity(1, 2)

			grown := data.subtrees[1]
			if grown == subtree {
				t.Fatal("subtree was not reallocated")
			}
			require.Equal(t, uint64(3), grown.levelSize(0))
			require.Equal(t, leaf, grown.nodes[grown.offsets[0]])
			require.Equal(t, oldRoot, grown.nodes[grown.offsets[2]])
			require.Equal(t, trie.ZeroHashes[1], grown.nodes[grown.offsets[1]+1])
		})

		t.Run("adds headroom", func(t *testing.T) {
			data := &progressiveNodesData{}
			data.ensureLeafCapacity(3, 20)
			require.Equal(t, uint64(22), data.subtrees[3].levelSize(0))
		})

		t.Run("caps at subtree capacity", func(t *testing.T) {
			data := &progressiveNodesData{}
			data.ensureLeafCapacity(1, 100)
			require.Equal(t, progressiveSubtreeCapacity(1), data.subtrees[1].levelSize(0))
		})
	})

	t.Run("entry count", func(t *testing.T) {
		t.Run("nil", func(t *testing.T) {
			var data *progressiveNodesData
			require.Equal(t, 0, data.entryCount())
		})

		t.Run("subtrees and spine", func(t *testing.T) {
			data := &progressiveNodesData{
				subtrees: []*progressiveSubtreeData{
					{nodes: make([][32]byte, 3)},
					nil,
					{nodes: make([][32]byte, 5)},
				},
				spine: make([][32]byte, 3),
			}
			require.Equal(t, 11, data.entryCount())
		})
	})

	t.Run("update metrics guards", func(t *testing.T) {
		t.Run("nil receiver", func(t *testing.T) {
			var data *progressiveNodesData
			data.updateMetrics()
		})

		t.Run("nil metrics", func(t *testing.T) {
			data := &progressiveNodesData{}
			data.updateMetrics()
		})
	})
}

func TestProgressiveSubtreeData(t *testing.T) {
	t.Run("build", func(t *testing.T) {
		leaves := [][32]byte{{1}, {2}, {3}}
		subtree := buildProgressiveSubtree(leaves, 2)
		left := progressiveTestHashPair(leaves[0], leaves[1])
		right := progressiveTestHashPair(leaves[2], trie.ZeroHashes[0])
		wantRoot := progressiveTestHashPair(left, right)

		require.DeepEqual(t, []uint64{0, 3, 5, 6}, subtree.offsets)
		require.Equal(t, uint64(3), subtree.levelSize(0))
		require.Equal(t, uint64(2), subtree.levelSize(1))
		require.Equal(t, uint64(1), subtree.levelSize(2))
		require.Equal(t, wantRoot, subtree.nodes[subtree.offsets[2]])
	})

	t.Run("recompute branch", func(t *testing.T) {
		for _, index := range []uint64{0, 1, 2} {
			t.Run(fmt.Sprintf("index_%d", index), func(t *testing.T) {
				leaves := [][32]byte{{1}, {2}, {3}}
				subtree := buildProgressiveSubtree(leaves, 2)
				leaves[index][0] += 10
				subtree.nodes[subtree.offsets[0]+index] = leaves[index]

				subtree.recomputeBranch(index)

				fresh := buildProgressiveSubtree(leaves, 2)
				require.Equal(t, fresh.nodes[fresh.offsets[2]], subtree.nodes[subtree.offsets[2]])
			})
		}
	})
}

func TestProgressiveOverridesData(t *testing.T) {
	t.Run("copy", func(t *testing.T) {
		original := newProgressiveOverridesData(types.Validators)
		position := progressiveNodePosition{subtree: 1, level: 2, index: 3}
		original.nodes[position] = [32]byte{1}
		original.spine[1] = [32]byte{2}
		original.leaves[4] = true
		original.updateMetrics()

		copied := original.copy(types.Validators)
		if copied == original {
			t.Fatal("copy returned the source override data")
		}
		require.DeepEqual(t, original.nodes, copied.nodes)
		require.DeepEqual(t, original.spine, copied.spine)
		require.DeepEqual(t, original.leaves, copied.leaves)
		require.Equal(t, 2, copied.metrics.totalCount)
		require.Equal(t, 1, copied.metrics.leafCount)

		copied.nodes[position] = [32]byte{9}
		copied.spine[1] = [32]byte{9}
		copied.leaves[5] = true
		require.Equal(t, [32]byte{1}, original.nodes[position])
		require.Equal(t, [32]byte{2}, original.spine[1])
		require.Equal(t, false, original.leaves[5])
	})

	t.Run("update metrics", func(t *testing.T) {
		data := newProgressiveOverridesData(types.Balances)
		data.nodes[progressiveNodePosition{}] = [32]byte{1}
		data.spine[0] = [32]byte{2}
		data.leaves[0] = true
		data.updateMetrics()
		require.Equal(t, 2, data.metrics.totalCount)
		require.Equal(t, 1, data.metrics.leafCount)

		clear(data.nodes)
		clear(data.spine)
		clear(data.leaves)
		data.updateMetrics()
		require.Equal(t, 0, data.metrics.totalCount)
		require.Equal(t, 0, data.metrics.leafCount)
	})
}

func TestProgressiveHelpers(t *testing.T) {
	t.Run("subtree capacity", func(t *testing.T) {
		for level, want := range []uint64{1, 4, 16, 64, 256} {
			t.Run(fmt.Sprintf("level_%d", level), func(t *testing.T) {
				require.Equal(t, want, progressiveSubtreeCapacity(level))
			})
		}
	})

	t.Run("subtree depth", func(t *testing.T) {
		for level, want := range []uint64{0, 2, 4, 6, 8} {
			t.Run(fmt.Sprintf("level_%d", level), func(t *testing.T) {
				require.Equal(t, want, progressiveSubtreeDepth(level))
			})
		}
	})

	t.Run("subtree start", func(t *testing.T) {
		for level, want := range []uint64{0, 1, 5, 21, 85} {
			t.Run(fmt.Sprintf("level_%d", level), func(t *testing.T) {
				require.Equal(t, want, progressiveSubtreeStart(level))
			})
		}
	})

	t.Run("number of levels", func(t *testing.T) {
		tests := []struct {
			leaves uint64
			levels int
		}{
			{leaves: 0, levels: 0},
			{leaves: 1, levels: 1},
			{leaves: 2, levels: 2},
			{leaves: 5, levels: 2},
			{leaves: 6, levels: 3},
			{leaves: 21, levels: 3},
			{leaves: 22, levels: 4},
			{leaves: 85, levels: 4},
			{leaves: 86, levels: 5},
		}
		for _, test := range tests {
			t.Run(fmt.Sprintf("leaves_%d", test.leaves), func(t *testing.T) {
				require.Equal(t, test.levels, progressiveNumLevels(test.leaves))
			})
		}
	})

	t.Run("subtree for index", func(t *testing.T) {
		tests := []struct {
			globalIndex  uint64
			subtreeIndex int
			localIndex   uint64
		}{
			{globalIndex: 0, subtreeIndex: 0, localIndex: 0},
			{globalIndex: 1, subtreeIndex: 1, localIndex: 0},
			{globalIndex: 4, subtreeIndex: 1, localIndex: 3},
			{globalIndex: 5, subtreeIndex: 2, localIndex: 0},
			{globalIndex: 20, subtreeIndex: 2, localIndex: 15},
			{globalIndex: 21, subtreeIndex: 3, localIndex: 0},
			{globalIndex: 84, subtreeIndex: 3, localIndex: 63},
			{globalIndex: 85, subtreeIndex: 4, localIndex: 0},
		}
		for _, test := range tests {
			t.Run(fmt.Sprintf("index_%d", test.globalIndex), func(t *testing.T) {
				subtreeIndex, localIndex := progressiveSubtreeForIndex(test.globalIndex)
				require.Equal(t, test.subtreeIndex, subtreeIndex)
				require.Equal(t, test.localIndex, localIndex)
			})
		}
	})
}

func newProgressiveTestFieldTrie(t *testing.T, field types.FieldIndex, dataType types.DataType, elements any) *FieldTrie {
	t.Helper()
	length := uint64(fieldparams.ValidatorRegistryLimit)
	if field == types.Balances {
		length = stateutil.ValidatorLimitForBalancesChunks()
	}
	fieldTrie, err := NewFieldTrieWithMode(field, dataType, MerkleModeProgressive, elements, length, 0)
	require.NoError(t, err)
	return fieldTrie
}

func requireProgressiveFreshRoot(t *testing.T, fieldTrie *FieldTrie, elements any, root [32]byte) {
	t.Helper()
	fresh := newProgressiveTestFieldTrie(t, fieldTrie.field, fieldTrie.dataType, elements)
	freshRoot, err := fresh.TrieRoot()
	require.NoError(t, err)
	require.Equal(t, freshRoot, root)
}

func progressiveTestHashPair(left, right [32]byte) [32]byte {
	var pair [64]byte
	copy(pair[:32], left[:])
	copy(pair[32:], right[:])
	return hash.Hash(pair[:])
}

func progressiveTestValidators(count int) []stateutil.CompactValidator {
	validators := make([]stateutil.CompactValidator, count)
	for i := range validators {
		validators[i].PublicKey[0] = byte(i)
		validators[i].PublicKey[1] = byte(i >> 8)
		validators[i].EffectiveBalance = uint64(i+1) * 1000
	}
	return validators
}

func progressiveTestBalances(count int) []uint64 {
	balances := make([]uint64, count)
	for i := range balances {
		balances[i] = uint64(i+1) * 1000
	}
	return balances
}
