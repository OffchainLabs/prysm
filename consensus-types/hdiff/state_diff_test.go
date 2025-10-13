package hdiff

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/golang/snappy"
	"github.com/pkg/errors"
)

var sourceFile = flag.String("source", "", "Path to the source file")
var targetFile = flag.String("target", "", "Path to the target file")

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

func Test_diffToState(t *testing.T) {
	source, _ := util.DeterministicGenesisStateElectra(t, 256)
	target := source.Copy()
	require.NoError(t, target.SetSlot(source.Slot()+1))
	hdiff, err := diffToState(source, target)
	require.NoError(t, err)
	require.Equal(t, hdiff.slot, target.Slot())
	require.Equal(t, hdiff.targetVersion, target.Version())
}

func Test_kmpIndex(t *testing.T) {
	intSlice := make([]*int, 10)
	for i := 0; i < len(intSlice); i++ {
		intSlice[i] = new(int)
		*intSlice[i] = i
	}
	integerEquals := func(a, b *int) bool {
		if a == nil && b == nil {
			return true
		}
		if a == nil || b == nil {
			return false
		}
		return *a == *b
	}
	t.Run("integer entries match", func(t *testing.T) {
		source := []*int{intSlice[0], intSlice[1], intSlice[2], intSlice[3], intSlice[4]}
		target := []*int{intSlice[2], intSlice[3], intSlice[4], intSlice[5], intSlice[6], intSlice[7], nil}
		target = append(target, source...)
		require.Equal(t, 2, kmpIndex(len(source), target, integerEquals))
	})
	t.Run("integer entries skipped", func(t *testing.T) {
		source := []*int{intSlice[0], intSlice[1], intSlice[2], intSlice[3], intSlice[4]}
		target := []*int{intSlice[2], intSlice[3], intSlice[4], intSlice[0], intSlice[5], nil}
		target = append(target, source...)
		require.Equal(t, 2, kmpIndex(len(source), target, integerEquals))
	})
	t.Run("integer entries repetitions", func(t *testing.T) {
		source := []*int{intSlice[0], intSlice[1], intSlice[0], intSlice[0], intSlice[0]}
		target := []*int{intSlice[0], intSlice[0], intSlice[1], intSlice[2], intSlice[5], nil}
		target = append(target, source...)
		require.Equal(t, 3, kmpIndex(len(source), target, integerEquals))
	})
	t.Run("integer entries no match", func(t *testing.T) {
		source := []*int{intSlice[0], intSlice[1], intSlice[2], intSlice[3]}
		target := []*int{intSlice[4], intSlice[5], intSlice[6], nil}
		target = append(target, source...)
		require.Equal(t, len(source), kmpIndex(len(source), target, integerEquals))
	})

}

func TestApplyDiff(t *testing.T) {
	source, keys := util.DeterministicGenesisStateElectra(t, 256)
	blk, err := util.GenerateFullBlockElectra(source, keys, util.DefaultBlockGenConfig(), 1)
	require.NoError(t, err)
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	ctx := t.Context()
	target, err := transition.ExecuteStateTransition(ctx, source, wsb)
	require.NoError(t, err)

	// Add non-trivial eth1Data, regression check
	depositRoot := make([]byte, fieldparams.RootLength)
	for i := range depositRoot {
		depositRoot[i] = byte(i + 42)
	}
	blockHash := make([]byte, fieldparams.RootLength)
	for i := range blockHash {
		blockHash[i] = byte(i + 100)
	}
	require.NoError(t, target.SetEth1Data(&ethpb.Eth1Data{
		DepositRoot:  depositRoot,
		DepositCount: 99999,
		BlockHash:    blockHash,
	}))

	hdiff, err := Diff(source, target)
	require.NoError(t, err)
	source, err = ApplyDiff(ctx, source, hdiff)
	require.NoError(t, err)
	require.DeepEqual(t, source, target)
}

func getMainnetStates() (state.BeaconState, state.BeaconState, error) {
	sourceBytes, err := os.ReadFile(*sourceFile)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to read source file")
	}
	targetBytes, err := os.ReadFile(*targetFile)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to read target file")
	}
	sourceProto := &ethpb.BeaconStateDeneb{}
	if err := sourceProto.UnmarshalSSZ(sourceBytes); err != nil {
		return nil, nil, errors.Wrap(err, "failed to unmarshal source proto")
	}
	source, err := state_native.InitializeFromProtoDeneb(sourceProto)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to initialize source state")
	}
	targetProto := &ethpb.BeaconStateElectra{}
	if err := targetProto.UnmarshalSSZ(targetBytes); err != nil {
		return nil, nil, errors.Wrap(err, "failed to unmarshal target proto")
	}
	target, err := state_native.InitializeFromProtoElectra(targetProto)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to initialize target state")
	}
	return source, target, nil
}

func TestApplyDiffMainnet(t *testing.T) {
	if *sourceFile == "" || *targetFile == "" {
		t.Skip("source and target files not provided")
	}
	source, target, err := getMainnetStates()
	require.NoError(t, err)
	hdiff, err := Diff(source, target)
	require.NoError(t, err)
	source, err = ApplyDiff(t.Context(), source, hdiff)
	require.NoError(t, err)
	sourceSSZ, err := source.MarshalSSZ()
	require.NoError(t, err)
	targetSSZ, err := target.MarshalSSZ()
	require.NoError(t, err)
	require.DeepEqual(t, sourceSSZ, targetSSZ)
	sVals := source.Validators()
	tVals := target.Validators()
	require.Equal(t, len(sVals), len(tVals))
	for i, v := range sVals {
		require.Equal(t, true, bytes.Equal(v.PublicKey, tVals[i].PublicKey))
		require.Equal(t, true, bytes.Equal(v.WithdrawalCredentials, tVals[i].WithdrawalCredentials))
		require.Equal(t, v.EffectiveBalance, tVals[i].EffectiveBalance)
		require.Equal(t, v.Slashed, tVals[i].Slashed)
		require.Equal(t, v.ActivationEligibilityEpoch, tVals[i].ActivationEligibilityEpoch)
		require.Equal(t, v.ActivationEpoch, tVals[i].ActivationEpoch)
		require.Equal(t, v.ExitEpoch, tVals[i].ExitEpoch)
		require.Equal(t, v.WithdrawableEpoch, tVals[i].WithdrawableEpoch)
	}
}

// Test_newHdiff tests the newHdiff function that deserializes HdiffBytes into hdiff struct
func Test_newHdiff(t *testing.T) {
	source, _ := util.DeterministicGenesisStateElectra(t, 32)
	target := source.Copy()
	require.NoError(t, target.SetSlot(source.Slot()+1))

	// Create a valid diff
	diffBytes, err := Diff(source, target)
	require.NoError(t, err)

	// Test successful deserialization
	hdiff, err := newHdiff(diffBytes)
	require.NoError(t, err)
	require.NotNil(t, hdiff)
	require.NotNil(t, hdiff.stateDiff)
	require.NotNil(t, hdiff.validatorDiffs)
	require.NotNil(t, hdiff.balancesDiff)
	require.Equal(t, target.Slot(), hdiff.stateDiff.slot)

	// Test with invalid state diff data
	invalidDiff := HdiffBytes{
		StateDiff:      []byte{0x01, 0x02}, // too small
		ValidatorDiffs: diffBytes.ValidatorDiffs,
		BalancesDiff:   diffBytes.BalancesDiff,
	}
	_, err = newHdiff(invalidDiff)
	require.ErrorContains(t, "failed to create state diff", err)

	// Test with invalid validator diff data
	invalidDiff = HdiffBytes{
		StateDiff:      diffBytes.StateDiff,
		ValidatorDiffs: []byte{0x01, 0x02}, // too small
		BalancesDiff:   diffBytes.BalancesDiff,
	}
	_, err = newHdiff(invalidDiff)
	require.ErrorContains(t, "failed to create validator diffs", err)

	// Test with invalid balances diff data
	invalidDiff = HdiffBytes{
		StateDiff:      diffBytes.StateDiff,
		ValidatorDiffs: diffBytes.ValidatorDiffs,
		BalancesDiff:   []byte{0x01, 0x02}, // too small
	}
	_, err = newHdiff(invalidDiff)
	require.ErrorContains(t, "failed to create balances diff", err)
}

// Test_diffInternal tests the internal diff computation logic
func Test_diffInternal(t *testing.T) {
	source, keys := util.DeterministicGenesisStateFulu(t, 32)
	target := source.Copy()

	t.Run("same state", func(t *testing.T) {
		hdiff, err := diffInternal(source, source)
		require.NoError(t, err)
		require.NotNil(t, hdiff)
		require.Equal(t, 0, len(hdiff.validatorDiffs))
		// Balance diff should have same length as validators but all zeros
		require.Equal(t, len(source.Balances()), len(hdiff.balancesDiff))
		for _, diff := range hdiff.balancesDiff {
			require.Equal(t, int64(0), diff)
		}
	})

	t.Run("slot change", func(t *testing.T) {
		require.NoError(t, target.SetSlot(source.Slot()+5))
		hdiff, err := diffInternal(source, target)
		require.NoError(t, err)
		require.NotNil(t, hdiff)
		require.Equal(t, target.Slot(), hdiff.stateDiff.slot)
		require.Equal(t, target.Version(), hdiff.stateDiff.targetVersion)
	})

	t.Run("lookahead change", func(t *testing.T) {
		proposerLookahead, err := source.ProposerLookahead()
		require.NoError(t, err)
		proposerLookahead[0] = proposerLookahead[0] + 1
		require.NoError(t, target.SetProposerLookahead(proposerLookahead))
		hdiff, err := diffInternal(source, target)
		require.NoError(t, err)
		require.NotNil(t, hdiff)
		require.Equal(t, len(proposerLookahead), len(hdiff.stateDiff.proposerLookahead))
		for i, v := range proposerLookahead {
			require.Equal(t, uint64(v), hdiff.stateDiff.proposerLookahead[i])
		}
	})

	t.Run("with block transition", func(t *testing.T) {
		blk, err := util.GenerateFullBlockFulu(source, keys, util.DefaultBlockGenConfig(), 1)
		require.NoError(t, err)
		wsb, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		ctx := t.Context()
		target, err := transition.ExecuteStateTransition(ctx, source, wsb)
		require.NoError(t, err)

		hdiff, err := diffInternal(source, target)
		require.NoError(t, err)
		require.NotNil(t, hdiff)
		require.Equal(t, target.Slot(), hdiff.stateDiff.slot)
		require.Equal(t, target.Version(), hdiff.stateDiff.targetVersion)
	})
}

// Test_validatorsEqual tests the validator comparison function
func Test_validatorsEqual(t *testing.T) {
	source, _ := util.DeterministicGenesisStateElectra(t, 32)

	t.Run("nil validators", func(t *testing.T) {
		require.Equal(t, true, validatorsEqual(nil, nil))
	})

	// Create two different states to test validator comparison
	target := source.Copy()
	targetVals := target.Validators()
	modifiedVal := &ethpb.Validator{
		PublicKey:                  targetVals[0].PublicKey,
		WithdrawalCredentials:      targetVals[0].WithdrawalCredentials,
		EffectiveBalance:           targetVals[0].EffectiveBalance,
		Slashed:                    targetVals[0].Slashed,
		ActivationEligibilityEpoch: targetVals[0].ActivationEligibilityEpoch,
		ActivationEpoch:            targetVals[0].ActivationEpoch,
		ExitEpoch:                  targetVals[0].ExitEpoch,
		WithdrawableEpoch:          targetVals[0].WithdrawableEpoch,
	}
	modifiedVal.Slashed = !targetVals[0].Slashed
	targetVals[0] = modifiedVal
	require.NoError(t, target.SetValidators(targetVals))

	// Test that different validators are detected as different
	sourceDiffs, err := diffToVals(source, target)
	require.NoError(t, err)
	require.NotEqual(t, 0, len(sourceDiffs), "Should detect validator differences")
}

// Test_updateToVersion tests the version upgrade functionality
func Test_updateToVersion(t *testing.T) {
	ctx := t.Context()

	t.Run("no upgrade needed", func(t *testing.T) {
		source, _ := util.DeterministicGenesisStateFulu(t, 32)
		targetVersion := source.Version()

		result, err := updateToVersion(ctx, source, targetVersion)
		require.NoError(t, err)
		require.Equal(t, targetVersion, result.Version())
		require.Equal(t, source.Slot(), result.Slot())
	})
	t.Run("upgrade to Fulu", func(t *testing.T) {
		source, _ := util.DeterministicGenesisStateElectra(t, 32)
		targetVersion := version.Fulu

		result, err := updateToVersion(ctx, source, targetVersion)
		require.NoError(t, err)
		require.Equal(t, targetVersion, result.Version())
		require.Equal(t, source.Slot(), result.Slot())
		lookahead, err := result.ProposerLookahead()
		require.NoError(t, err)
		require.Equal(t, 2*fieldparams.SlotsPerEpoch, len(lookahead))
	})
}

func TestApplyDiffMainnetComplete(t *testing.T) {
	if *sourceFile == "" || *targetFile == "" {
		t.Skip("source and target files not provided")
	}
	source, target, err := getMainnetStates()
	require.NoError(t, err)
	hdiff, err := Diff(source, target)
	require.NoError(t, err)
	source, err = ApplyDiff(t.Context(), source, hdiff)
	require.NoError(t, err)

	sBals := source.Balances()
	tBals := target.Balances()
	require.Equal(t, len(sBals), len(tBals))
	for i, v := range sBals {
		require.Equal(t, v, tBals[i], "i: %d", i)
	}

	sourceSSZ, err := source.MarshalSSZ()
	require.NoError(t, err)
	targetSSZ, err := target.MarshalSSZ()
	require.NoError(t, err)
	require.Equal(t, true, bytes.Equal(sourceSSZ, targetSSZ))
}

// Test_diffToVals tests validator diff computation
func Test_diffToVals(t *testing.T) {
	source, _ := util.DeterministicGenesisStateElectra(t, 32)
	target := source.Copy()

	t.Run("no validator changes", func(t *testing.T) {
		diffs, err := diffToVals(source, target)
		require.NoError(t, err)
		require.Equal(t, 0, len(diffs))
	})

	t.Run("validator slashed", func(t *testing.T) {
		vals := target.Validators()
		modifiedVal := &ethpb.Validator{
			PublicKey:                  vals[0].PublicKey,
			WithdrawalCredentials:      vals[0].WithdrawalCredentials,
			EffectiveBalance:           vals[0].EffectiveBalance,
			Slashed:                    vals[0].Slashed,
			ActivationEligibilityEpoch: vals[0].ActivationEligibilityEpoch,
			ActivationEpoch:            vals[0].ActivationEpoch,
			ExitEpoch:                  vals[0].ExitEpoch,
			WithdrawableEpoch:          vals[0].WithdrawableEpoch,
		}
		modifiedVal.Slashed = true
		vals[0] = modifiedVal
		require.NoError(t, target.SetValidators(vals))

		diffs, err := diffToVals(source, target)
		require.NoError(t, err)
		require.Equal(t, 1, len(diffs))
		require.Equal(t, uint32(0), diffs[0].index)
		require.Equal(t, true, diffs[0].Slashed)
	})

	t.Run("validator effective balance changed", func(t *testing.T) {
		vals := target.Validators()
		modifiedVal := &ethpb.Validator{
			PublicKey:                  vals[1].PublicKey,
			WithdrawalCredentials:      vals[1].WithdrawalCredentials,
			EffectiveBalance:           vals[1].EffectiveBalance,
			Slashed:                    vals[1].Slashed,
			ActivationEligibilityEpoch: vals[1].ActivationEligibilityEpoch,
			ActivationEpoch:            vals[1].ActivationEpoch,
			ExitEpoch:                  vals[1].ExitEpoch,
			WithdrawableEpoch:          vals[1].WithdrawableEpoch,
		}
		modifiedVal.EffectiveBalance = vals[1].EffectiveBalance + 1000
		vals[1] = modifiedVal
		require.NoError(t, target.SetValidators(vals))

		diffs, err := diffToVals(source, target)
		require.NoError(t, err)
		found := false
		for _, diff := range diffs {
			if diff.index == 1 {
				require.Equal(t, modifiedVal.EffectiveBalance, diff.EffectiveBalance)
				found = true
				break
			}
		}
		require.Equal(t, true, found)
	})
}

// Test_newValidatorDiffs tests validator diff deserialization
func Test_newValidatorDiffs(t *testing.T) {
	source, _ := util.DeterministicGenesisStateElectra(t, 32)
	target := source.Copy()

	// Modify a validator to create diffs
	vals := target.Validators()
	modifiedVal := &ethpb.Validator{
		PublicKey:                  vals[0].PublicKey,
		WithdrawalCredentials:      vals[0].WithdrawalCredentials,
		EffectiveBalance:           vals[0].EffectiveBalance,
		Slashed:                    vals[0].Slashed,
		ActivationEligibilityEpoch: vals[0].ActivationEligibilityEpoch,
		ActivationEpoch:            vals[0].ActivationEpoch,
		ExitEpoch:                  vals[0].ExitEpoch,
		WithdrawableEpoch:          vals[0].WithdrawableEpoch,
	}
	modifiedVal.Slashed = true
	vals[0] = modifiedVal
	require.NoError(t, target.SetValidators(vals))

	// Create diff and serialize
	originalDiffs, err := diffToVals(source, target)
	require.NoError(t, err)

	hdiffBytes, err := Diff(source, target)
	require.NoError(t, err)

	// Test deserialization
	deserializedDiffs, err := newValidatorDiffs(hdiffBytes.ValidatorDiffs)
	require.NoError(t, err)
	require.Equal(t, len(originalDiffs), len(deserializedDiffs))

	if len(originalDiffs) > 0 {
		require.Equal(t, originalDiffs[0].index, deserializedDiffs[0].index)
		require.Equal(t, originalDiffs[0].Slashed, deserializedDiffs[0].Slashed)
	}

	// Test with invalid data
	_, err = newValidatorDiffs([]byte{0x01, 0x02})
	require.NotNil(t, err)
}

// Test_applyValidatorDiff tests applying validator changes to state
func Test_applyValidatorDiff(t *testing.T) {
	source, _ := util.DeterministicGenesisStateElectra(t, 32)
	target := source.Copy()

	// Modify validators in target
	vals := target.Validators()
	modifiedVal := &ethpb.Validator{
		PublicKey:                  vals[0].PublicKey,
		WithdrawalCredentials:      vals[0].WithdrawalCredentials,
		EffectiveBalance:           vals[0].EffectiveBalance,
		Slashed:                    vals[0].Slashed,
		ActivationEligibilityEpoch: vals[0].ActivationEligibilityEpoch,
		ActivationEpoch:            vals[0].ActivationEpoch,
		ExitEpoch:                  vals[0].ExitEpoch,
		WithdrawableEpoch:          vals[0].WithdrawableEpoch,
	}
	modifiedVal.Slashed = true
	modifiedVal.EffectiveBalance = vals[0].EffectiveBalance + 1000
	vals[0] = modifiedVal
	require.NoError(t, target.SetValidators(vals))

	// Create validator diffs
	diffs, err := diffToVals(source, target)
	require.NoError(t, err)

	// Apply diffs to source
	result, err := applyValidatorDiff(source, diffs)
	require.NoError(t, err)

	// Verify result matches target
	resultVals := result.Validators()
	targetVals := target.Validators()
	require.Equal(t, len(targetVals), len(resultVals))

	for i, val := range resultVals {
		require.Equal(t, targetVals[i].Slashed, val.Slashed)
		require.Equal(t, targetVals[i].EffectiveBalance, val.EffectiveBalance)
	}
}

// Test_diffToBalances tests balance diff computation
func Test_diffToBalances(t *testing.T) {
	source, _ := util.DeterministicGenesisStateElectra(t, 32)
	target := source.Copy()

	t.Run("no balance changes", func(t *testing.T) {
		diffs, err := diffToBalances(source, target)
		require.NoError(t, err)
		// Balance diff should have same length as validators but all zeros
		require.Equal(t, len(source.Balances()), len(diffs))
		for _, diff := range diffs {
			require.Equal(t, int64(0), diff)
		}
	})

	t.Run("balance changes", func(t *testing.T) {
		bals := target.Balances()
		bals[0] += 1000
		bals[1] -= 500
		bals[5] += 2000
		require.NoError(t, target.SetBalances(bals))

		diffs, err := diffToBalances(source, target)
		require.NoError(t, err)

		// Should have diffs for changed balances only
		require.NotEqual(t, 0, len(diffs))

		// Apply diffs to verify correctness
		sourceBals := source.Balances()
		for i, diff := range diffs {
			if diff != 0 {
				sourceBals[i] += uint64(diff)
			}
		}

		targetBals := target.Balances()
		for i := 0; i < len(sourceBals); i++ {
			require.Equal(t, targetBals[i], sourceBals[i], "balance mismatch at index %d", i)
		}
	})
}

// Test_newBalancesDiff tests balance diff deserialization
func Test_newBalancesDiff(t *testing.T) {
	source, _ := util.DeterministicGenesisStateElectra(t, 32)
	target := source.Copy()

	// Modify balances to create diffs
	bals := target.Balances()
	bals[0] += 1000
	bals[1] -= 500
	require.NoError(t, target.SetBalances(bals))

	// Create diff and serialize
	originalDiffs, err := diffToBalances(source, target)
	require.NoError(t, err)

	hdiffBytes, err := Diff(source, target)
	require.NoError(t, err)

	// Test deserialization
	deserializedDiffs, err := newBalancesDiff(hdiffBytes.BalancesDiff)
	require.NoError(t, err)
	require.Equal(t, len(originalDiffs), len(deserializedDiffs))

	for i, diff := range originalDiffs {
		require.Equal(t, diff, deserializedDiffs[i])
	}

	// Test with invalid data
	_, err = newBalancesDiff([]byte{0x01, 0x02})
	require.NotNil(t, err)
}

// Test_applyBalancesDiff tests applying balance changes to state
func Test_applyBalancesDiff(t *testing.T) {
	source, _ := util.DeterministicGenesisStateElectra(t, 32)
	target := source.Copy()

	// Modify balances in target
	bals := target.Balances()
	bals[0] += 1000
	bals[1] -= 500
	bals[5] += 2000
	require.NoError(t, target.SetBalances(bals))

	// Create balance diffs
	diffs, err := diffToBalances(source, target)
	require.NoError(t, err)

	// Apply diffs to source
	result, err := applyBalancesDiff(source, diffs)
	require.NoError(t, err)

	// Verify result matches target
	resultBals := result.Balances()
	targetBals := target.Balances()
	require.Equal(t, len(targetBals), len(resultBals))

	for i, bal := range resultBals {
		require.Equal(t, targetBals[i], bal, "balance mismatch at index %d", i)
	}
}

// Test_newStateDiff tests state diff deserialization
func Test_newStateDiff(t *testing.T) {
	source, _ := util.DeterministicGenesisStateElectra(t, 32)
	target := source.Copy()
	require.NoError(t, target.SetSlot(source.Slot()+5))

	// Create diff and serialize
	hdiffBytes, err := Diff(source, target)
	require.NoError(t, err)

	// Test successful deserialization
	stateDiff, err := newStateDiff(hdiffBytes.StateDiff)
	require.NoError(t, err)
	require.NotNil(t, stateDiff)
	require.Equal(t, target.Slot(), stateDiff.slot)
	require.Equal(t, target.Version(), stateDiff.targetVersion)

	// Test with invalid data (too small)
	_, err = newStateDiff([]byte{0x01, 0x02})
	require.ErrorContains(t, "failed to decode snappy", err)

	// Test with valid snappy data but insufficient content (need 8 bytes for targetVersion)
	insuffData := []byte{0x01, 0x02, 0x03, 0x04} // only 4 bytes
	validSnappyButInsufficientData := snappy.Encode(nil, insuffData)
	_, err = newStateDiff(validSnappyButInsufficientData)
	require.ErrorContains(t, "data is too small", err)
}

// Test_applyStateDiff tests applying state changes
func Test_applyStateDiff(t *testing.T) {
	ctx := t.Context()
	source, _ := util.DeterministicGenesisStateElectra(t, 32)
	target := source.Copy()

	// Modify target state
	require.NoError(t, target.SetSlot(source.Slot()+5))

	// Create state diff
	stateDiff, err := diffToState(source, target)
	require.NoError(t, err)

	// Apply diff to source
	result, err := applyStateDiff(ctx, source, stateDiff)
	require.NoError(t, err)

	// Verify result matches target
	require.Equal(t, target.Slot(), result.Slot())
	require.Equal(t, target.Version(), result.Version())
}

// Test_computeLPS tests the LPS array computation for KMP algorithm
func Test_computeLPS(t *testing.T) {
	intSlice := make([]*int, 10)
	for i := 0; i < len(intSlice); i++ {
		intSlice[i] = new(int)
		*intSlice[i] = i
	}
	integerEquals := func(a, b *int) bool {
		if a == nil && b == nil {
			return true
		}
		if a == nil || b == nil {
			return false
		}
		return *a == *b
	}

	t.Run("simple pattern", func(t *testing.T) {
		pattern := []*int{intSlice[0], intSlice[1], intSlice[0]}
		lps := computeLPS(pattern, integerEquals)
		expected := []int{0, 0, 1}
		require.Equal(t, len(expected), len(lps))
		for i, exp := range expected {
			require.Equal(t, exp, lps[i])
		}
	})

	t.Run("repeating pattern", func(t *testing.T) {
		pattern := []*int{intSlice[0], intSlice[0], intSlice[0]}
		lps := computeLPS(pattern, integerEquals)
		expected := []int{0, 1, 2}
		require.Equal(t, len(expected), len(lps))
		for i, exp := range expected {
			require.Equal(t, exp, lps[i])
		}
	})

	t.Run("complex pattern", func(t *testing.T) {
		pattern := []*int{intSlice[0], intSlice[1], intSlice[0], intSlice[1], intSlice[0]}
		lps := computeLPS(pattern, integerEquals)
		expected := []int{0, 0, 1, 2, 3}
		require.Equal(t, len(expected), len(lps))
		for i, exp := range expected {
			require.Equal(t, exp, lps[i])
		}
	})

	t.Run("no repetition", func(t *testing.T) {
		pattern := []*int{intSlice[0], intSlice[1], intSlice[2], intSlice[3]}
		lps := computeLPS(pattern, integerEquals)
		expected := []int{0, 0, 0, 0}
		require.Equal(t, len(expected), len(lps))
		for i, exp := range expected {
			require.Equal(t, exp, lps[i])
		}
	})
}

// Test field-specific diff functions
func Test_diffJustificationBits(t *testing.T) {
	source, _ := util.DeterministicGenesisStateElectra(t, 32)

	// Test justification bits extraction
	bits := diffJustificationBits(source)
	sourceBits := source.JustificationBits()
	require.Equal(t, sourceBits[0], bits)
}

func Test_diffBlockRoots(t *testing.T) {
	source, _ := util.DeterministicGenesisStateElectra(t, 32)
	target := source.Copy()

	// Modify block roots in target
	blockRoots := target.BlockRoots()
	copy(blockRoots[0], []byte{0x01, 0x02, 0x03})
	copy(blockRoots[1], []byte{0x04, 0x05, 0x06})
	require.NoError(t, target.SetBlockRoots(blockRoots))

	// Create diff
	diff := &stateDiff{}
	diffBlockRoots(diff, source, target)

	// Verify diff contains changes
	require.NotEqual(t, [32]byte{}, diff.blockRoots[0])
	require.NotEqual(t, [32]byte{}, diff.blockRoots[1])
}

func Test_diffStateRoots(t *testing.T) {
	source, _ := util.DeterministicGenesisStateElectra(t, 32)
	target := source.Copy()

	// Modify state roots in target
	stateRoots := target.StateRoots()
	copy(stateRoots[0], []byte{0x01, 0x02, 0x03})
	copy(stateRoots[1], []byte{0x04, 0x05, 0x06})
	require.NoError(t, target.SetStateRoots(stateRoots))

	// Create diff
	diff := &stateDiff{}
	diffStateRoots(diff, source, target)

	// Verify diff contains changes
	require.NotEqual(t, [32]byte{}, diff.stateRoots[0])
	require.NotEqual(t, [32]byte{}, diff.stateRoots[1])
}

func Test_shouldAppendEth1DataVotes(t *testing.T) {
	// Test empty votes
	root1 := make([]byte, 32)
	root1[0] = 0x01
	require.Equal(t, true, shouldAppendEth1DataVotes([]*ethpb.Eth1Data{}, []*ethpb.Eth1Data{{BlockHash: root1}}))

	// Test appending to existing votes
	root2 := make([]byte, 32)
	root2[0] = 0x02
	sourceVotes := []*ethpb.Eth1Data{{BlockHash: root1}}
	targetVotes := []*ethpb.Eth1Data{{BlockHash: root1}, {BlockHash: root2}}
	require.Equal(t, true, shouldAppendEth1DataVotes(sourceVotes, targetVotes))

	// Test complete replacement
	root3 := make([]byte, 32)
	root3[0] = 0x03
	sourceVotes = []*ethpb.Eth1Data{{BlockHash: root1}, {BlockHash: root2}}
	targetVotes = []*ethpb.Eth1Data{{BlockHash: root3}}
	require.Equal(t, false, shouldAppendEth1DataVotes(sourceVotes, targetVotes))
}

// Test key serialization methods
func Test_stateDiff_serialize(t *testing.T) {
	source, _ := util.DeterministicGenesisStateElectra(t, 32)
	target := source.Copy()
	require.NoError(t, target.SetSlot(source.Slot()+5))

	// Create state diff
	stateDiff, err := diffToState(source, target)
	require.NoError(t, err)

	// Serialize
	serialized := stateDiff.serialize()
	require.Equal(t, true, len(serialized) > 0)

	// Verify it can be deserialized back (need to compress with snappy first)
	compressed := snappy.Encode(nil, serialized)
	deserializedDiff, err := newStateDiff(compressed)
	require.NoError(t, err)
	require.Equal(t, stateDiff.slot, deserializedDiff.slot)
	require.Equal(t, stateDiff.targetVersion, deserializedDiff.targetVersion)
}

func Test_hdiff_serialize(t *testing.T) {
	source, _ := util.DeterministicGenesisStateElectra(t, 32)
	target := source.Copy()
	require.NoError(t, target.SetSlot(source.Slot()+5))

	// Create hdiff
	hdiff, err := diffInternal(source, target)
	require.NoError(t, err)

	// Serialize
	serialized := hdiff.serialize()
	require.Equal(t, true, len(serialized.StateDiff) > 0)
	require.Equal(t, true, len(serialized.ValidatorDiffs) >= 0)
	require.Equal(t, true, len(serialized.BalancesDiff) >= 0)

	// Verify it can be deserialized back
	deserializedHdiff, err := newHdiff(serialized)
	require.NoError(t, err)
	require.Equal(t, hdiff.stateDiff.slot, deserializedHdiff.stateDiff.slot)
	require.Equal(t, hdiff.stateDiff.targetVersion, deserializedHdiff.stateDiff.targetVersion)
}

// Test some key read methods
func Test_readTargetVersion(t *testing.T) {
	diff := &stateDiff{}

	// Test successful read
	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, 5)
	err := diff.readTargetVersion(&data)
	require.NoError(t, err)
	require.Equal(t, 5, diff.targetVersion)
	require.Equal(t, 0, len(data))

	// Test insufficient data
	data = []byte{0x01, 0x02}
	err = diff.readTargetVersion(&data)
	require.ErrorContains(t, "targetVersion", err)
}

func Test_readSlot(t *testing.T) {
	diff := &stateDiff{}

	// Test successful read
	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, 100)
	err := diff.readSlot(&data)
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(100), diff.slot)
	require.Equal(t, 0, len(data))

	// Test insufficient data
	data = []byte{0x01, 0x02}
	err = diff.readSlot(&data)
	require.ErrorContains(t, "slot", err)
}

// Test a sample apply method
func Test_applySlashingsDiff(t *testing.T) {
	source, _ := util.DeterministicGenesisStateElectra(t, 32)

	// Create a diff with slashing changes
	diff := &stateDiff{}
	originalSlashings := source.Slashings()
	diff.slashings[0] = 1000 // Algebraic diff
	diff.slashings[1] = 500  // Algebraic diff (positive to avoid underflow)

	// Apply the diff
	err := applySlashingsDiff(source, diff)
	require.NoError(t, err)

	// Verify the changes were applied
	resultSlashings := source.Slashings()
	require.Equal(t, originalSlashings[0]+1000, resultSlashings[0])
	require.Equal(t, originalSlashings[1]+500, resultSlashings[1])
}

// Test readPendingAttestation utility
func Test_readPendingAttestation(t *testing.T) {
	// Test insufficient data
	data := []byte{0x01, 0x02}
	_, err := readPendingAttestation(&data)
	require.ErrorContains(t, "data is too small", err)
}

// Test readEth1Data - regression test for bug where indices were off by 1
func Test_readEth1Data(t *testing.T) {
	diff := &stateDiff{}

	// Test nil marker
	data := []byte{nilMarker}
	err := diff.readEth1Data(&data)
	require.NoError(t, err)
	require.IsNil(t, diff.eth1Data)
	require.Equal(t, 0, len(data))

	// Test successful read with actual data
	// Create test data: marker + depositRoot + depositCount + blockHash
	depositRoot := make([]byte, fieldparams.RootLength)
	for i := range depositRoot {
		depositRoot[i] = byte(i % 256)
	}
	blockHash := make([]byte, fieldparams.RootLength)
	for i := range blockHash {
		blockHash[i] = byte((i + 100) % 256)
	}
	depositCount := uint64(12345)

	data = []byte{notNilMarker}
	data = append(data, depositRoot...)
	countBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(countBytes, depositCount)
	data = append(data, countBytes...)
	data = append(data, blockHash...)

	diff = &stateDiff{}
	err = diff.readEth1Data(&data)
	require.NoError(t, err)
	require.NotNil(t, diff.eth1Data)
	require.DeepEqual(t, depositRoot, diff.eth1Data.DepositRoot)
	require.Equal(t, depositCount, diff.eth1Data.DepositCount)
	require.DeepEqual(t, blockHash, diff.eth1Data.BlockHash)
	require.Equal(t, 0, len(data))

	// Test insufficient data for marker
	data = []byte{}
	diff = &stateDiff{}
	err = diff.readEth1Data(&data)
	require.ErrorContains(t, "eth1Data", err)

	// Test insufficient data after marker
	data = []byte{notNilMarker}
	diff = &stateDiff{}
	err = diff.readEth1Data(&data)
	require.ErrorContains(t, "eth1Data", err)
}

func BenchmarkGetDiff(b *testing.B) {
	if *sourceFile == "" || *targetFile == "" {
		b.Skip("source and target files not provided")
	}
	source, target, err := getMainnetStates()
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hdiff, err := Diff(source, target)
		b.Log("Diff size:", len(hdiff.StateDiff)+len(hdiff.BalancesDiff)+len(hdiff.ValidatorDiffs))
		require.NoError(b, err)
	}
}

func BenchmarkApplyDiff(b *testing.B) {
	if *sourceFile == "" || *targetFile == "" {
		b.Skip("source and target files not provided")
	}
	source, target, err := getMainnetStates()
	require.NoError(b, err)
	hdiff, err := Diff(source, target)
	require.NoError(b, err)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		source, err = ApplyDiff(b.Context(), source, hdiff)
		require.NoError(b, err)
	}
}

// BenchmarkDiffCreation measures the time to create diffs of various sizes
func BenchmarkDiffCreation(b *testing.B) {
	sizes := []uint64{32, 64, 128, 256, 512, 1024}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("validators_%d", size), func(b *testing.B) {
			source, _ := util.DeterministicGenesisStateElectra(b, size)
			target := source.Copy()
			_ = target.SetSlot(source.Slot() + 1)

			// Modify some validators
			validators := target.Validators()
			for i := 0; i < int(size/10); i++ {
				if i < len(validators) {
					validators[i].EffectiveBalance += 1000
				}
			}
			_ = target.SetValidators(validators)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := Diff(source, target)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkDiffApplication measures the time to apply diffs
func BenchmarkDiffApplication(b *testing.B) {
	sizes := []uint64{32, 64, 128, 256, 512}
	ctx := b.Context()

	for _, size := range sizes {
		b.Run(fmt.Sprintf("validators_%d", size), func(b *testing.B) {
			source, _ := util.DeterministicGenesisStateElectra(b, size)
			target := source.Copy()
			_ = target.SetSlot(source.Slot() + 10)

			// Create diff once
			diff, err := Diff(source, target)
			if err != nil {
				b.Fatal(err)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Need fresh source for each iteration
				freshSource := source.Copy()
				_, err := ApplyDiff(ctx, freshSource, diff)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkSerialization measures serialization performance
func BenchmarkSerialization(b *testing.B) {
	source, _ := util.DeterministicGenesisStateElectra(b, 256)
	target := source.Copy()
	_ = target.SetSlot(source.Slot() + 5)

	hdiff, err := diffInternal(source, target)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = hdiff.serialize()
	}
}

// BenchmarkDeserialization measures deserialization performance
func BenchmarkDeserialization(b *testing.B) {
	source, _ := util.DeterministicGenesisStateElectra(b, 256)
	target := source.Copy()
	_ = target.SetSlot(source.Slot() + 5)

	// Create serialized diff
	diff, err := Diff(source, target)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := newHdiff(diff)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBalanceDiff measures balance diff computation
func BenchmarkBalanceDiff(b *testing.B) {
	sizes := []uint64{100, 500, 1000, 5000, 10000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("balances_%d", size), func(b *testing.B) {
			source, _ := util.DeterministicGenesisStateElectra(b, size)
			target := source.Copy()

			// Modify all balances
			balances := target.Balances()
			for i := range balances {
				balances[i] += uint64(i % 1000)
			}
			_ = target.SetBalances(balances)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := diffToBalances(source, target)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkValidatorDiff measures validator diff computation
func BenchmarkValidatorDiff(b *testing.B) {
	sizes := []uint64{100, 500, 1000, 2000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("validators_%d", size), func(b *testing.B) {
			source, _ := util.DeterministicGenesisStateElectra(b, size)
			target := source.Copy()

			// Modify some validators
			validators := target.Validators()
			for i := 0; i < int(size/10); i++ {
				if i < len(validators) {
					validators[i].EffectiveBalance += 1000
					validators[i].Slashed = true
				}
			}
			_ = target.SetValidators(validators)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := diffToVals(source, target)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkKMPAlgorithm measures KMP performance with different pattern sizes
func BenchmarkKMPAlgorithm(b *testing.B) {
	patternSizes := []int{10, 50, 100, 500}
	textSizes := []int{100, 500, 1000, 5000}

	for _, pSize := range patternSizes {
		for _, tSize := range textSizes {
			if pSize > tSize {
				continue
			}

			b.Run(fmt.Sprintf("pattern_%d_text_%d", pSize, tSize), func(b *testing.B) {
				// Create pattern and text
				pattern := make([]*int, pSize)
				for i := range pattern {
					val := i % 10
					pattern[i] = &val
				}

				text := make([]*int, tSize)
				for i := range text {
					val := i % 10
					text[i] = &val
				}

				// Add pattern to end of text
				text = append(text, pattern...)

				intEquals := func(a, b *int) bool {
					if a == nil && b == nil {
						return true
					}
					if a == nil || b == nil {
						return false
					}
					return *a == *b
				}

				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					_ = kmpIndex(len(pattern), text, intEquals)
				}
			})
		}
	}
}

// BenchmarkCompressionRatio measures compression effectiveness
func BenchmarkCompressionRatio(b *testing.B) {
	source, _ := util.DeterministicGenesisStateElectra(b, 512)
	target := source.Copy()
	_ = target.SetSlot(source.Slot() + 1)

	// Create different types of changes
	testCases := []struct {
		name     string
		modifier func(target state.BeaconState)
	}{
		{
			name: "minimal_change",
			modifier: func(target state.BeaconState) {
				// Just slot change, already done
			},
		},
		{
			name: "balance_changes",
			modifier: func(target state.BeaconState) {
				balances := target.Balances()
				for i := 0; i < 10; i++ {
					if i < len(balances) {
						balances[i] += 1000
					}
				}
				_ = target.SetBalances(balances)
			},
		},
		{
			name: "validator_changes",
			modifier: func(target state.BeaconState) {
				validators := target.Validators()
				for i := 0; i < 10; i++ {
					if i < len(validators) {
						validators[i].EffectiveBalance += 1000
					}
				}
				_ = target.SetValidators(validators)
			},
		},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			testTarget := target.Copy()
			tc.modifier(testTarget)

			// Get full state size
			fullStateSSZ, err := testTarget.MarshalSSZ()
			if err != nil {
				b.Fatal(err)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				diff, err := Diff(source, testTarget)
				if err != nil {
					b.Fatal(err)
				}

				diffSize := len(diff.StateDiff) + len(diff.ValidatorDiffs) + len(diff.BalancesDiff)

				// Report compression ratio in the first iteration
				if i == 0 {
					ratio := float64(len(fullStateSSZ)) / float64(diffSize)
					b.Logf("Compression ratio: %.2fx (full: %d bytes, diff: %d bytes)",
						ratio, len(fullStateSSZ), diffSize)
				}
			}
		})
	}
}

// BenchmarkMemoryUsage measures memory allocations
func BenchmarkMemoryUsage(b *testing.B) {
	source, _ := util.DeterministicGenesisStateElectra(b, 256)
	target := source.Copy()
	_ = target.SetSlot(source.Slot() + 10)

	// Modify some data
	validators := target.Validators()
	for i := 0; i < 25; i++ {
		if i < len(validators) {
			validators[i].EffectiveBalance += 1000
		}
	}
	_ = target.SetValidators(validators)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		diff, err := Diff(source, target)
		if err != nil {
			b.Fatal(err)
		}

		_, err = ApplyDiff(b.Context(), source.Copy(), diff)
		if err != nil {
			b.Fatal(err)
		}
	}
}
