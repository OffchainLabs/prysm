package fieldtrie

import (
	"runtime"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestProgressiveFieldTrie_Build(t *testing.T) {
	t.Run("validators", func(t *testing.T) {
		for _, count := range []int{0, 1, 2, 5, 6, 21, 22, 85, 86} {
			validators := progressiveTestValidators(count)
			fieldTrie, err := NewFieldTrieWithMode(
				types.Validators,
				types.CompositeArray,
				MerkleModeProgressive,
				validators,
				fieldparams.ValidatorRegistryLimit,
				0,
			)
			require.NoError(t, err)
			root, err := fieldTrie.TrieRoot()
			require.NoError(t, err)
			expected, err := stateutil.ValidatorRegistryRootProgressive(validators)
			require.NoError(t, err)
			require.Equal(t, expected, root)
		}
	})

	t.Run("balances", func(t *testing.T) {
		for _, count := range []int{0, 1, 4, 5, 20, 21, 84, 85} {
			balances := progressiveTestBalances(count)
			fieldTrie, err := NewFieldTrieWithMode(
				types.Balances,
				types.CompressedArray,
				MerkleModeProgressive,
				balances,
				stateutil.ValidatorLimitForBalancesChunks(),
				0,
			)
			require.NoError(t, err)
			root, err := fieldTrie.TrieRoot()
			require.NoError(t, err)
			expected, err := stateutil.Uint64ListRootProgressive(balances)
			require.NoError(t, err)
			require.Equal(t, expected, root)
		}
	})
}

func TestProgressiveFieldTrie_RecomputeOwned(t *testing.T) {
	t.Run("validator", func(t *testing.T) {
		validators := progressiveTestValidators(86)
		fieldTrie := newProgressiveTestFieldTrie(t, types.Validators, types.CompositeArray, validators)

		validators[42].EffectiveBalance++
		returned, root, err := fieldTrie.RecomputeTrie([]uint64{42}, validators)
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
}

func TestProgressiveFieldTrie_AppendBoundaries(t *testing.T) {
	t.Run("validators", func(t *testing.T) {
		for _, initialCount := range []int{0, 1, 5, 21, 85} {
			validators := progressiveTestValidators(initialCount)
			fieldTrie := newProgressiveTestFieldTrie(t, types.Validators, types.CompositeArray, validators)

			validators = append(validators, progressiveTestValidators(1)[0])
			validators[len(validators)-1].EffectiveBalance = uint64(len(validators) * 1000)
			returned, root, err := fieldTrie.RecomputeTrie([]uint64{uint64(len(validators) - 1)}, validators)
			require.NoError(t, err)
			requireProgressiveFreshRoot(t, returned, validators, root)
		}
	})

	t.Run("balances new chunks", func(t *testing.T) {
		for _, initialChunkCount := range []int{1, 5, 21, 85} {
			balances := progressiveTestBalances(initialChunkCount * 4)
			fieldTrie := newProgressiveTestFieldTrie(t, types.Balances, types.CompressedArray, balances)

			balances = append(balances, uint64(len(balances)+1))
			returned, root, err := fieldTrie.RecomputeTrie([]uint64{uint64(len(balances) - 1)}, balances)
			require.NoError(t, err)
			requireProgressiveFreshRoot(t, returned, balances, root)
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
}

func TestProgressiveFieldTrie_CopyOnWriteAndPromotion(t *testing.T) {
	validators := progressiveTestValidators(86)
	fieldTrie := newProgressiveTestFieldTrie(t, types.Validators, types.CompositeArray, validators)
	originalRoot, err := fieldTrie.TrieRoot()
	require.NoError(t, err)

	copied := fieldTrie.CopyTrie()
	fieldTrie.promotionThreshold = 1
	copied.promotionThreshold = 1

	validators[1].EffectiveBalance++
	overlay, overlayRoot, err := fieldTrie.RecomputeTrie([]uint64{1}, validators)
	require.NoError(t, err)
	require.Equal(t, fieldTrie, overlay.base)
	requireProgressiveFreshRoot(t, overlay, validators, overlayRoot)

	copyRoot, err := copied.TrieRoot()
	require.NoError(t, err)
	require.Equal(t, originalRoot, copyRoot)

	validators[6].EffectiveBalance++
	overlay, overlayRoot, err = overlay.RecomputeTrie([]uint64{6}, validators)
	require.NoError(t, err)
	requireProgressiveFreshRoot(t, overlay, validators, overlayRoot)

	validators[22].EffectiveBalance++
	promoted, promotedRoot, err := overlay.RecomputeTrie([]uint64{22}, validators)
	require.NoError(t, err)
	require.Equal(t, true, promoted.base == nil)
	requireProgressiveFreshRoot(t, promoted, validators, promotedRoot)
}

func TestProgressiveFieldTrie_CopyOnWriteAppendBoundary(t *testing.T) {
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
}

func TestProgressiveFieldTrie_Rebuild(t *testing.T) {
	validators := progressiveTestValidators(22)
	fieldTrie := newProgressiveTestFieldTrie(t, types.Validators, types.CompositeArray, validators)
	copied := fieldTrie.CopyTrie()
	_ = copied

	validators[0].EffectiveBalance++
	overlay, _, err := fieldTrie.RecomputeTrie([]uint64{0}, validators)
	require.NoError(t, err)
	runtime.KeepAlive(copied)
	require.Equal(t, true, overlay.base != nil)

	replacement := progressiveTestValidators(86)
	rebuilt, root, err := overlay.RecomputeTrie(nil, replacement)
	require.NoError(t, err)
	require.Equal(t, true, rebuilt.base == nil)
	requireProgressiveFreshRoot(t, rebuilt, replacement, root)
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
