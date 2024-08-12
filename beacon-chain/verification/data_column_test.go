package verification

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/pkg/errors"
	forkchoicetypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/forkchoice/types"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/startup"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

func TestColumnIndexInBounds(t *testing.T) {
	ini := &Initializer{}
	_, cols := util.GenerateTestDenebBlockWithColumns(t, [32]byte{}, 0, 1)
	b := cols[0]
	// set Index to a value that is out of bounds
	v := ini.NewColumnVerifier(b, GossipColumnSidecarRequirements)
	require.NoError(t, v.DataColumnIndexInBounds())
	require.Equal(t, true, v.results.executed(RequireDataColumnIndexInBounds))
	require.NoError(t, v.results.result(RequireDataColumnIndexInBounds))

	b.ColumnIndex = fieldparams.NumberOfColumns
	v = ini.NewColumnVerifier(b, GossipColumnSidecarRequirements)
	require.ErrorIs(t, v.DataColumnIndexInBounds(), ErrColumnIndexInvalid)
	require.Equal(t, true, v.results.executed(RequireDataColumnIndexInBounds))
	require.NotNil(t, v.results.result(RequireDataColumnIndexInBounds))
}

func TestColumnSlotNotTooEarly(t *testing.T) {
	now := time.Now()
	// make genesis 1 slot in the past
	genesis := now.Add(-1 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second)

	_, columns := util.GenerateTestDenebBlockWithColumns(t, [32]byte{}, 0, 1)
	c := columns[0]
	// slot 1 should be 12 seconds after genesis
	c.SignedBlockHeader.Header.Slot = 1

	// This clock will give a current slot of 1 on the nose
	happyClock := startup.NewClock(genesis, [32]byte{}, startup.WithNower(func() time.Time { return now }))
	ini := Initializer{shared: &sharedResources{clock: happyClock}}
	v := ini.NewColumnVerifier(c, GossipColumnSidecarRequirements)
	require.NoError(t, v.NotFromFutureSlot())
	require.Equal(t, true, v.results.executed(RequireNotFromFutureSlot))
	require.NoError(t, v.results.result(RequireNotFromFutureSlot))

	// Since we have an early return for slots that are directly equal, give a time that is less than max disparity
	// but still in the previous slot.
	closeClock := startup.NewClock(genesis, [32]byte{}, startup.WithNower(func() time.Time { return now.Add(-1 * params.BeaconConfig().MaximumGossipClockDisparityDuration() / 2) }))
	ini = Initializer{shared: &sharedResources{clock: closeClock}}
	v = ini.NewColumnVerifier(c, GossipColumnSidecarRequirements)
	require.NoError(t, v.NotFromFutureSlot())

	// This clock will give a current slot of 0, with now coming more than max clock disparity before slot 1
	disparate := now.Add(-2 * params.BeaconConfig().MaximumGossipClockDisparityDuration())
	dispClock := startup.NewClock(genesis, [32]byte{}, startup.WithNower(func() time.Time { return disparate }))
	// Set up initializer to use the clock that will set now to a little to far before slot 1
	ini = Initializer{shared: &sharedResources{clock: dispClock}}
	v = ini.NewColumnVerifier(c, GossipColumnSidecarRequirements)
	require.ErrorIs(t, v.NotFromFutureSlot(), ErrFromFutureSlot)
	require.Equal(t, true, v.results.executed(RequireNotFromFutureSlot))
	require.NotNil(t, v.results.result(RequireNotFromFutureSlot))
}

func TestColumnSlotAboveFinalized(t *testing.T) {
	ini := &Initializer{shared: &sharedResources{}}
	cases := []struct {
		name          string
		slot          primitives.Slot
		finalizedSlot primitives.Slot
		err           error
	}{
		{
			name: "finalized epoch < column epoch",
			slot: 32,
		},
		{
			name: "finalized slot < column slot (same epoch)",
			slot: 31,
		},
		{
			name:          "finalized epoch > column epoch",
			finalizedSlot: 32,
			err:           ErrSlotNotAfterFinalized,
		},
		{
			name:          "finalized slot == column slot",
			slot:          35,
			finalizedSlot: 35,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			finalizedCB := func() *forkchoicetypes.Checkpoint {
				return &forkchoicetypes.Checkpoint{
					Epoch: slots.ToEpoch(c.finalizedSlot),
					Root:  [32]byte{},
				}
			}
			ini.shared.fc = &mockForkchoicer{FinalizedCheckpointCB: finalizedCB}
			_, columns := util.GenerateTestDenebBlockWithColumns(t, [32]byte{}, 0, 1)
			col := columns[0]
			col.SignedBlockHeader.Header.Slot = c.slot
			v := ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)
			err := v.SlotAboveFinalized()
			require.Equal(t, true, v.results.executed(RequireSlotAboveFinalized))
			if c.err == nil {
				require.NoError(t, err)
				require.NoError(t, v.results.result(RequireSlotAboveFinalized))
			} else {
				require.ErrorIs(t, err, c.err)
				require.NotNil(t, v.results.result(RequireSlotAboveFinalized))
			}
		})
	}
}

func TestDataColumnValidProposerSignature_Cached(t *testing.T) {
	ctx := context.Background()
	_, columns := util.GenerateTestDenebBlockWithColumns(t, [32]byte{}, 0, 1)
	col := columns[0]
	expectedSd := columnToSignatureData(col)
	sc := &mockSignatureCache{
		svcb: func(sig SignatureData) (bool, error) {
			if sig != expectedSd {
				t.Error("Did not see expected SignatureData")
			}
			return true, nil
		},
		vscb: func(sig SignatureData, v ValidatorAtIndexer) (err error) {
			t.Error("VerifySignature should not be called if the result is cached")
			return nil
		},
	}
	ini := Initializer{shared: &sharedResources{sc: sc, sr: &mockStateByRooter{sbr: sbrErrorIfCalled(t)}}}
	v := ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)
	require.NoError(t, v.ValidProposerSignature(ctx))
	require.Equal(t, true, v.results.executed(RequireValidProposerSignature))
	require.NoError(t, v.results.result(RequireValidProposerSignature))

	// simulate an error in the cache - indicating the previous verification failed
	sc.svcb = func(sig SignatureData) (bool, error) {
		if sig != expectedSd {
			t.Error("Did not see expected SignatureData")
		}
		return true, errors.New("derp")
	}
	ini = Initializer{shared: &sharedResources{sc: sc, sr: &mockStateByRooter{sbr: sbrErrorIfCalled(t)}}}
	v = ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)
	require.ErrorIs(t, v.ValidProposerSignature(ctx), ErrInvalidProposerSignature)
	require.Equal(t, true, v.results.executed(RequireValidProposerSignature))
	require.NotNil(t, v.results.result(RequireValidProposerSignature))
}

func TestColumnValidProposerSignature_CacheMiss(t *testing.T) {
	ctx := context.Background()
	_, columns := util.GenerateTestDenebBlockWithColumns(t, [32]byte{}, 0, 1)
	col := columns[0]
	expectedSd := columnToSignatureData(col)
	sc := &mockSignatureCache{
		svcb: func(sig SignatureData) (bool, error) {
			return false, nil
		},
		vscb: func(sig SignatureData, v ValidatorAtIndexer) (err error) {
			if expectedSd != sig {
				t.Error("unexpected signature data")
			}
			return nil
		},
	}
	ini := Initializer{shared: &sharedResources{sc: sc, sr: sbrForValOverride(col.ProposerIndex(), &ethpb.Validator{})}}
	v := ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)
	require.NoError(t, v.ValidProposerSignature(ctx))
	require.Equal(t, true, v.results.executed(RequireValidProposerSignature))
	require.NoError(t, v.results.result(RequireValidProposerSignature))

	// simulate state not found
	ini = Initializer{shared: &sharedResources{sc: sc, sr: sbrNotFound(t, expectedSd.Parent)}}
	v = ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)
	require.ErrorIs(t, v.ValidProposerSignature(ctx), ErrInvalidProposerSignature)
	require.Equal(t, true, v.results.executed(RequireValidProposerSignature))
	require.NotNil(t, v.results.result(RequireValidProposerSignature))

	// simulate successful state lookup, but sig failure
	sbr := sbrForValOverride(col.ProposerIndex(), &ethpb.Validator{})
	sc = &mockSignatureCache{
		svcb: sc.svcb,
		vscb: func(sig SignatureData, v ValidatorAtIndexer) (err error) {
			if expectedSd != sig {
				t.Error("unexpected signature data")
			}
			return errors.New("signature, not so good!")
		},
	}
	ini = Initializer{shared: &sharedResources{sc: sc, sr: sbr}}
	v = ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)

	// make sure all the histories are clean before calling the method
	// so we don't get polluted by previous usages
	require.Equal(t, false, sbr.calledForRoot[expectedSd.Parent])
	require.Equal(t, false, sc.svCalledForSig[expectedSd])
	require.Equal(t, false, sc.vsCalledForSig[expectedSd])

	// Here we're mainly checking that all the right interfaces get used in the unhappy path
	require.ErrorIs(t, v.ValidProposerSignature(ctx), ErrInvalidProposerSignature)
	require.Equal(t, true, sbr.calledForRoot[expectedSd.Parent])
	require.Equal(t, true, sc.svCalledForSig[expectedSd])
	require.Equal(t, true, sc.vsCalledForSig[expectedSd])
	require.Equal(t, true, v.results.executed(RequireValidProposerSignature))
	require.NotNil(t, v.results.result(RequireValidProposerSignature))
}

func TestColumnSidecarParentSeen(t *testing.T) {
	_, columns := util.GenerateTestDenebBlockWithColumns(t, [32]byte{}, 0, 1)
	col := columns[0]

	fcHas := &mockForkchoicer{
		HasNodeCB: func(parent [32]byte) bool {
			if parent != col.ParentRoot() {
				t.Error("forkchoice.HasNode called with unexpected parent root")
			}
			return true
		},
	}
	fcLacks := &mockForkchoicer{
		HasNodeCB: func(parent [32]byte) bool {
			if parent != col.ParentRoot() {
				t.Error("forkchoice.HasNode called with unexpected parent root")
			}
			return false
		},
	}

	t.Run("happy path", func(t *testing.T) {
		ini := Initializer{shared: &sharedResources{fc: fcHas}}
		v := ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)
		require.NoError(t, v.SidecarParentSeen(nil))
		require.Equal(t, true, v.results.executed(RequireSidecarParentSeen))
		require.NoError(t, v.results.result(RequireSidecarParentSeen))
	})
	t.Run("HasNode false, no badParent cb, expected error", func(t *testing.T) {
		ini := Initializer{shared: &sharedResources{fc: fcLacks}}
		v := ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)
		require.ErrorIs(t, v.SidecarParentSeen(nil), ErrSidecarParentNotSeen)
		require.Equal(t, true, v.results.executed(RequireSidecarParentSeen))
		require.NotNil(t, v.results.result(RequireSidecarParentSeen))
	})

	t.Run("HasNode false, badParent true", func(t *testing.T) {
		ini := Initializer{shared: &sharedResources{fc: fcLacks}}
		v := ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)
		require.NoError(t, v.SidecarParentSeen(badParentCb(t, col.ParentRoot(), true)))
		require.Equal(t, true, v.results.executed(RequireSidecarParentSeen))
		require.NoError(t, v.results.result(RequireSidecarParentSeen))
	})
	t.Run("HasNode false, badParent false", func(t *testing.T) {
		ini := Initializer{shared: &sharedResources{fc: fcLacks}}
		v := ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)
		require.ErrorIs(t, v.SidecarParentSeen(badParentCb(t, col.ParentRoot(), false)), ErrSidecarParentNotSeen)
		require.Equal(t, true, v.results.executed(RequireSidecarParentSeen))
		require.NotNil(t, v.results.result(RequireSidecarParentSeen))
	})
}

func TestColumnSidecarParentValid(t *testing.T) {
	_, columns := util.GenerateTestDenebBlockWithColumns(t, [32]byte{}, 0, 1)
	col := columns[0]
	t.Run("parent valid", func(t *testing.T) {
		ini := Initializer{shared: &sharedResources{}}
		v := ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)
		require.NoError(t, v.SidecarParentValid(badParentCb(t, col.ParentRoot(), false)))
		require.Equal(t, true, v.results.executed(RequireSidecarParentValid))
		require.NoError(t, v.results.result(RequireSidecarParentValid))
	})
	t.Run("parent not valid", func(t *testing.T) {
		ini := Initializer{shared: &sharedResources{}}
		v := ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)
		require.ErrorIs(t, v.SidecarParentValid(badParentCb(t, col.ParentRoot(), true)), ErrSidecarParentInvalid)
		require.Equal(t, true, v.results.executed(RequireSidecarParentValid))
		require.NotNil(t, v.results.result(RequireSidecarParentValid))
	})
}

func TestColumnSidecarParentSlotLower(t *testing.T) {
	_, columns := util.GenerateTestDenebBlockWithColumns(t, [32]byte{}, 1, 1)
	col := columns[0]
	cases := []struct {
		name   string
		fcSlot primitives.Slot
		fcErr  error
		err    error
	}{
		{
			name:  "not in fc",
			fcErr: errors.New("not in forkchoice"),
			err:   ErrSlotNotAfterParent,
		},
		{
			name:   "in fc, slot lower",
			fcSlot: col.Slot() - 1,
		},
		{
			name:   "in fc, slot equal",
			fcSlot: col.Slot(),
			err:    ErrSlotNotAfterParent,
		},
		{
			name:   "in fc, slot higher",
			fcSlot: col.Slot() + 1,
			err:    ErrSlotNotAfterParent,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ini := Initializer{shared: &sharedResources{fc: &mockForkchoicer{SlotCB: func(r [32]byte) (primitives.Slot, error) {
				if col.ParentRoot() != r {
					t.Error("forkchoice.Slot called with unexpected parent root")
				}
				return c.fcSlot, c.fcErr
			}}}}
			v := ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)
			err := v.SidecarParentSlotLower()
			require.Equal(t, true, v.results.executed(RequireSidecarParentSlotLower))
			if c.err == nil {
				require.NoError(t, err)
				require.NoError(t, v.results.result(RequireSidecarParentSlotLower))
			} else {
				require.ErrorIs(t, err, c.err)
				require.NotNil(t, v.results.result(RequireSidecarParentSlotLower))
			}
		})
	}
}

func TestColumnSidecarDescendsFromFinalized(t *testing.T) {
	_, columns := util.GenerateTestDenebBlockWithColumns(t, [32]byte{}, 0, 1)
	col := columns[0]
	t.Run("not canonical", func(t *testing.T) {
		ini := Initializer{shared: &sharedResources{fc: &mockForkchoicer{HasNodeCB: func(r [32]byte) bool {
			if col.ParentRoot() != r {
				t.Error("forkchoice.Slot called with unexpected parent root")
			}
			return false
		}}}}
		v := ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)
		require.ErrorIs(t, v.SidecarDescendsFromFinalized(), ErrSidecarNotFinalizedDescendent)
		require.Equal(t, true, v.results.executed(RequireSidecarDescendsFromFinalized))
		require.NotNil(t, v.results.result(RequireSidecarDescendsFromFinalized))
	})
	t.Run("canonical", func(t *testing.T) {
		ini := Initializer{shared: &sharedResources{fc: &mockForkchoicer{HasNodeCB: func(r [32]byte) bool {
			if col.ParentRoot() != r {
				t.Error("forkchoice.Slot called with unexpected parent root")
			}
			return true
		}}}}
		v := ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)
		require.NoError(t, v.SidecarDescendsFromFinalized())
		require.Equal(t, true, v.results.executed(RequireSidecarDescendsFromFinalized))
		require.NoError(t, v.results.result(RequireSidecarDescendsFromFinalized))
	})
}

func TestColumnSidecarInclusionProven(t *testing.T) {
	_, columns := util.GenerateTestDenebBlockWithColumns(t, [32]byte{}, 0, 1)
	col := columns[0]

	ini := Initializer{}
	v := ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)
	require.NoError(t, v.SidecarInclusionProven())
	require.Equal(t, true, v.results.executed(RequireSidecarInclusionProven))
	require.NoError(t, v.results.result(RequireSidecarInclusionProven))

	// Invert bits of the first byte of the body root to mess up the proof
	byte0 := col.SignedBlockHeader.Header.BodyRoot[0]
	col.SignedBlockHeader.Header.BodyRoot[0] = byte0 ^ 255
	v = ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)
	require.ErrorIs(t, v.SidecarInclusionProven(), ErrSidecarInclusionProofInvalid)
	require.Equal(t, true, v.results.executed(RequireSidecarInclusionProven))
	require.NotNil(t, v.results.result(RequireSidecarInclusionProven))
}

func TestColumnSidecarInclusionProvenElectra(t *testing.T) {
	_, columns := util.GenerateTestDenebBlockWithColumns(t, [32]byte{}, 0, 1)
	col := columns[0]

	ini := Initializer{}
	v := ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)
	require.NoError(t, v.SidecarInclusionProven())
	require.Equal(t, true, v.results.executed(RequireSidecarInclusionProven))
	require.NoError(t, v.results.result(RequireSidecarInclusionProven))

	// Invert bits of the first byte of the body root to mess up the proof
	byte0 := col.SignedBlockHeader.Header.BodyRoot[0]
	col.SignedBlockHeader.Header.BodyRoot[0] = byte0 ^ 255
	v = ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)
	require.ErrorIs(t, v.SidecarInclusionProven(), ErrSidecarInclusionProofInvalid)
	require.Equal(t, true, v.results.executed(RequireSidecarInclusionProven))
	require.NotNil(t, v.results.result(RequireSidecarInclusionProven))
}

func TestColumnSidecarKzgProofVerified(t *testing.T) {
	_, columns := util.GenerateTestDenebBlockWithColumns(t, [32]byte{}, 0, 1)
	col := columns[0]
	passes := func(vb blocks.RODataColumn) (bool, error) {
		require.Equal(t, true, reflect.DeepEqual(col.KzgCommitments, vb.KzgCommitments))
		return true, nil
	}
	v := &RODataColumnVerifier{verifyDataColumnCommitment: passes, results: newResults(), dataColumn: col}
	require.NoError(t, v.SidecarKzgProofVerified())
	require.Equal(t, true, v.results.executed(RequireSidecarKzgProofVerified))
	require.NoError(t, v.results.result(RequireSidecarKzgProofVerified))

	fails := func(vb blocks.RODataColumn) (bool, error) {
		require.Equal(t, true, reflect.DeepEqual(col.KzgCommitments, vb.KzgCommitments))
		return false, errors.New("bad blob")
	}
	v = &RODataColumnVerifier{results: newResults(), dataColumn: col, verifyDataColumnCommitment: fails}
	require.ErrorIs(t, v.SidecarKzgProofVerified(), ErrSidecarKzgProofInvalid)
	require.Equal(t, true, v.results.executed(RequireSidecarKzgProofVerified))
	require.NotNil(t, v.results.result(RequireSidecarKzgProofVerified))
}

func TestColumnSidecarProposerExpected(t *testing.T) {
	ctx := context.Background()
	_, columns := util.GenerateTestDenebBlockWithColumns(t, [32]byte{}, 1, 1)
	col := columns[0]
	t.Run("cached, matches", func(t *testing.T) {
		ini := Initializer{shared: &sharedResources{pc: &mockProposerCache{ProposerCB: pcReturnsIdx(col.ProposerIndex())}, fc: &mockForkchoicer{TargetRootForEpochCB: fcReturnsTargetRoot([32]byte{})}}}
		v := ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)
		require.NoError(t, v.SidecarProposerExpected(ctx))
		require.Equal(t, true, v.results.executed(RequireSidecarProposerExpected))
		require.NoError(t, v.results.result(RequireSidecarProposerExpected))
	})
	t.Run("cached, does not match", func(t *testing.T) {
		ini := Initializer{shared: &sharedResources{pc: &mockProposerCache{ProposerCB: pcReturnsIdx(col.ProposerIndex() + 1)}, fc: &mockForkchoicer{TargetRootForEpochCB: fcReturnsTargetRoot([32]byte{})}}}
		v := ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)
		require.ErrorIs(t, v.SidecarProposerExpected(ctx), ErrSidecarUnexpectedProposer)
		require.Equal(t, true, v.results.executed(RequireSidecarProposerExpected))
		require.NotNil(t, v.results.result(RequireSidecarProposerExpected))
	})
	t.Run("not cached, state lookup failure", func(t *testing.T) {
		ini := Initializer{shared: &sharedResources{sr: sbrNotFound(t, col.ParentRoot()), pc: &mockProposerCache{ProposerCB: pcReturnsNotFound()}, fc: &mockForkchoicer{TargetRootForEpochCB: fcReturnsTargetRoot([32]byte{})}}}
		v := ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)
		require.ErrorIs(t, v.SidecarProposerExpected(ctx), ErrSidecarUnexpectedProposer)
		require.Equal(t, true, v.results.executed(RequireSidecarProposerExpected))
		require.NotNil(t, v.results.result(RequireSidecarProposerExpected))
	})

	t.Run("not cached, proposer matches", func(t *testing.T) {
		pc := &mockProposerCache{
			ProposerCB: pcReturnsNotFound(),
			ComputeProposerCB: func(ctx context.Context, root [32]byte, slot primitives.Slot, pst state.BeaconState) (primitives.ValidatorIndex, error) {
				require.Equal(t, col.ParentRoot(), root)
				require.Equal(t, col.Slot(), slot)
				return col.ProposerIndex(), nil
			},
		}
		ini := Initializer{shared: &sharedResources{sr: sbrForValOverride(col.ProposerIndex(), &ethpb.Validator{}), pc: pc, fc: &mockForkchoicer{TargetRootForEpochCB: fcReturnsTargetRoot([32]byte{})}}}
		v := ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)
		require.NoError(t, v.SidecarProposerExpected(ctx))
		require.Equal(t, true, v.results.executed(RequireSidecarProposerExpected))
		require.NoError(t, v.results.result(RequireSidecarProposerExpected))
	})

	t.Run("not cached, proposer matches for next epoch", func(t *testing.T) {
		_, newCols := util.GenerateTestDenebBlockWithColumns(t, [32]byte{}, 2*params.BeaconConfig().SlotsPerEpoch, 1)

		newCol := newCols[0]
		pc := &mockProposerCache{
			ProposerCB: pcReturnsNotFound(),
			ComputeProposerCB: func(ctx context.Context, root [32]byte, slot primitives.Slot, pst state.BeaconState) (primitives.ValidatorIndex, error) {
				require.Equal(t, newCol.ParentRoot(), root)
				require.Equal(t, newCol.Slot(), slot)
				return col.ProposerIndex(), nil
			},
		}
		ini := Initializer{shared: &sharedResources{sr: sbrForValOverride(newCol.ProposerIndex(), &ethpb.Validator{}), pc: pc, fc: &mockForkchoicer{TargetRootForEpochCB: fcReturnsTargetRoot([32]byte{})}}}
		v := ini.NewColumnVerifier(newCol, GossipColumnSidecarRequirements)
		require.NoError(t, v.SidecarProposerExpected(ctx))
		require.Equal(t, true, v.results.executed(RequireSidecarProposerExpected))
		require.NoError(t, v.results.result(RequireSidecarProposerExpected))
	})
	t.Run("not cached, proposer does not match", func(t *testing.T) {
		pc := &mockProposerCache{
			ProposerCB: pcReturnsNotFound(),
			ComputeProposerCB: func(ctx context.Context, root [32]byte, slot primitives.Slot, pst state.BeaconState) (primitives.ValidatorIndex, error) {
				require.Equal(t, col.ParentRoot(), root)
				require.Equal(t, col.Slot(), slot)
				return col.ProposerIndex() + 1, nil
			},
		}
		ini := Initializer{shared: &sharedResources{sr: sbrForValOverride(col.ProposerIndex(), &ethpb.Validator{}), pc: pc, fc: &mockForkchoicer{TargetRootForEpochCB: fcReturnsTargetRoot([32]byte{})}}}
		v := ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)
		require.ErrorIs(t, v.SidecarProposerExpected(ctx), ErrSidecarUnexpectedProposer)
		require.Equal(t, true, v.results.executed(RequireSidecarProposerExpected))
		require.NotNil(t, v.results.result(RequireSidecarProposerExpected))
	})
	t.Run("not cached, ComputeProposer fails", func(t *testing.T) {
		pc := &mockProposerCache{
			ProposerCB: pcReturnsNotFound(),
			ComputeProposerCB: func(ctx context.Context, root [32]byte, slot primitives.Slot, pst state.BeaconState) (primitives.ValidatorIndex, error) {
				require.Equal(t, col.ParentRoot(), root)
				require.Equal(t, col.Slot(), slot)
				return 0, errors.New("ComputeProposer failed")
			},
		}
		ini := Initializer{shared: &sharedResources{sr: sbrForValOverride(col.ProposerIndex(), &ethpb.Validator{}), pc: pc, fc: &mockForkchoicer{TargetRootForEpochCB: fcReturnsTargetRoot([32]byte{})}}}
		v := ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)
		require.ErrorIs(t, v.SidecarProposerExpected(ctx), ErrSidecarUnexpectedProposer)
		require.Equal(t, true, v.results.executed(RequireSidecarProposerExpected))
		require.NotNil(t, v.results.result(RequireSidecarProposerExpected))
	})
}

func TestColumnRequirementSatisfaction(t *testing.T) {
	_, columns := util.GenerateTestDenebBlockWithColumns(t, [32]byte{}, 1, 1)
	col := columns[0]
	ini := Initializer{}
	v := ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)

	_, err := v.VerifiedRODataColumn()
	require.ErrorIs(t, err, ErrColumnInvalid)
	var me VerificationMultiError
	ok := errors.As(err, &me)
	require.Equal(t, true, ok)
	fails := me.Failures()
	// we haven't performed any verification, so all the results should be this type
	for _, v := range fails {
		require.ErrorIs(t, v, ErrMissingVerification)
	}

	// satisfy everything through the backdoor and ensure we get the verified ro blob at the end
	for _, r := range GossipColumnSidecarRequirements {
		v.results.record(r, nil)
	}
	require.Equal(t, true, v.results.allSatisfied())
	_, err = v.VerifiedRODataColumn()
	require.NoError(t, err)
}

func TestStateCaching(t *testing.T) {
	_, columns := util.GenerateTestDenebBlockWithColumns(t, [32]byte{}, 1, 1)
	col := columns[0]
	ini := Initializer{shared: &sharedResources{sr: sbrForValOverride(col.ProposerIndex(), &ethpb.Validator{})}}
	v := ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)
	_, err := v.parentState(context.Background())
	require.NoError(t, err)

	// Utilize the cached state.
	v.sr = nil
	_, err = v.parentState(context.Background())
	require.NoError(t, err)
}

func TestColumnSatisfyRequirement(t *testing.T) {
	_, columns := util.GenerateTestDenebBlockWithColumns(t, [32]byte{}, 1, 1)
	col := columns[0]
	ini := Initializer{}
	v := ini.NewColumnVerifier(col, GossipColumnSidecarRequirements)
	require.Equal(t, false, v.results.executed(RequireDataColumnIndexInBounds))

	v.SatisfyRequirement(RequireDataColumnIndexInBounds)
	require.Equal(t, true, v.results.executed(RequireDataColumnIndexInBounds))
}
