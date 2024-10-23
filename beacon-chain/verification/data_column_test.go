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

func TestDataColumnsIndexInBounds(t *testing.T) {
	testCases := []struct {
		name         string
		columnsIndex uint64
		isError      bool
	}{
		{
			name:         "column index in bounds",
			columnsIndex: 0,
			isError:      false,
		},
		{
			name:         "column index out of bounds",
			columnsIndex: fieldparams.NumberOfColumns,
			isError:      true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			const (
				columnSlot = 0
				blobCount  = 1
			)

			parentRoot := [32]byte{}
			initializer := Initializer{}

			_, columns := util.GenerateTestDenebBlockWithColumns(t, parentRoot, columnSlot, blobCount)
			for _, column := range columns {
				column.ColumnIndex = tc.columnsIndex
			}

			verifier := initializer.NewDataColumnsVerifier(columns, GossipColumnSidecarRequirements)

			err := verifier.DataColumnsIndexInBounds()
			require.Equal(t, true, verifier.results.executed(RequireDataColumnIndexInBounds))

			if tc.isError {
				require.ErrorIs(t, err, ErrColumnIndexInvalid)
				require.NotNil(t, verifier.results.result(RequireDataColumnIndexInBounds))
				return
			}

			require.NoError(t, err)
			require.NoError(t, verifier.results.result(RequireDataColumnIndexInBounds))
		})
	}
}

func TestNotFromFutureSlot(t *testing.T) {
	maximumGossipClockDisparity := params.BeaconConfig().MaximumGossipClockDisparityDuration()

	testCases := []struct {
		name                    string
		currentSlot, columnSlot primitives.Slot
		timeBeforeCurrentSlot   time.Duration
		isError                 bool
	}{
		{
			name:                  "column slot == current slot",
			currentSlot:           42,
			columnSlot:            42,
			timeBeforeCurrentSlot: 0,
			isError:               false,
		},
		{
			name:                  "within maximum gossip clock disparity",
			currentSlot:           42,
			columnSlot:            42,
			timeBeforeCurrentSlot: maximumGossipClockDisparity / 2,
			isError:               false,
		},
		{
			name:                  "outside maximum gossip clock disparity",
			currentSlot:           42,
			columnSlot:            42,
			timeBeforeCurrentSlot: maximumGossipClockDisparity * 2,
			isError:               true,
		},
		{
			name:                  "too far in the future",
			currentSlot:           10,
			columnSlot:            42,
			timeBeforeCurrentSlot: 0,
			isError:               true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			const blobCount = 1

			now := time.Now()
			secondsPerSlot := time.Duration(params.BeaconConfig().SecondsPerSlot)
			genesis := now.Add(-time.Duration(tc.currentSlot) * secondsPerSlot * time.Second)

			clock := startup.NewClock(
				genesis,
				[fieldparams.RootLength]byte{},
				startup.WithNower(func() time.Time {
					return now.Add(-tc.timeBeforeCurrentSlot)
				}),
			)

			parentRoot := [fieldparams.RootLength]byte{}
			initializer := Initializer{shared: &sharedResources{clock: clock}}

			_, columns := util.GenerateTestDenebBlockWithColumns(t, parentRoot, tc.columnSlot, blobCount)
			verifier := initializer.NewDataColumnsVerifier(columns, GossipColumnSidecarRequirements)

			err := verifier.NotFromFutureSlot()
			require.Equal(t, true, verifier.results.executed(RequireNotFromFutureSlot))

			if tc.isError {
				require.ErrorIs(t, err, ErrFromFutureSlot)
				require.NotNil(t, verifier.results.result(RequireNotFromFutureSlot))
				return
			}

			require.NoError(t, err)
			require.NoError(t, verifier.results.result(RequireNotFromFutureSlot))
		})
	}
}

func TestColumnSlotAboveFinalized(t *testing.T) {
	testCases := []struct {
		name                      string
		finalizedSlot, columnSlot primitives.Slot
		isErr                     bool
	}{
		{
			name:          "finalized epoch < column epoch",
			finalizedSlot: 10,
			columnSlot:    96,
			isErr:         false,
		},
		{
			name:          "finalized slot < column slot (same epoch)",
			finalizedSlot: 32,
			columnSlot:    33,
			isErr:         false,
		},
		{
			name:          "finalized slot == column slot",
			finalizedSlot: 64,
			columnSlot:    64,
			isErr:         true,
		},
		{
			name:          "finalized epoch > column epoch",
			finalizedSlot: 32,
			columnSlot:    31,
			isErr:         true,
		},
	}
	for _, tc := range testCases {
		const blobCount = 1

		t.Run(tc.name, func(t *testing.T) {
			finalizedCheckpoint := func() *forkchoicetypes.Checkpoint {
				return &forkchoicetypes.Checkpoint{
					Epoch: slots.ToEpoch(tc.finalizedSlot),
					Root:  [fieldparams.RootLength]byte{},
				}
			}

			parentRoot := [fieldparams.RootLength]byte{}
			initializer := &Initializer{shared: &sharedResources{
				fc: &mockForkchoicer{FinalizedCheckpointCB: finalizedCheckpoint},
			}}

			_, columns := util.GenerateTestDenebBlockWithColumns(t, parentRoot, tc.columnSlot, blobCount)

			v := initializer.NewDataColumnsVerifier(columns, GossipColumnSidecarRequirements)

			err := v.SlotAboveFinalized()
			require.Equal(t, true, v.results.executed(RequireSlotAboveFinalized))

			if tc.isErr {
				require.ErrorIs(t, err, ErrSlotNotAfterFinalized)
				require.NotNil(t, v.results.result(RequireSlotAboveFinalized))
				return
			}

			require.NoError(t, err)
			require.NoError(t, v.results.result(RequireSlotAboveFinalized))
		})
	}
}

func TestValidProposerSignature(t *testing.T) {
	const (
		columnSlot = 0
		blobCount  = 1
	)

	parentRoot := [fieldparams.RootLength]byte{}
	validator := &ethpb.Validator{}

	_, columns := util.GenerateTestDenebBlockWithColumns(t, parentRoot, columnSlot, blobCount)
	firstColumn := columns[0]

	// The signature data does not depend on the data column itself, so we can use the first one.
	expectedSignatureData := columnToSignatureData(firstColumn)

	testCases := []struct {
		isError         bool
		vscbShouldError bool
		svcbReturn      bool
		stateByRooter   StateByRooter
		vscbError       error
		svcbError       error
		name            string
	}{
		{
			name:            "cache hit - success",
			svcbReturn:      true,
			svcbError:       nil,
			vscbShouldError: true,
			vscbError:       nil,
			stateByRooter:   &mockStateByRooter{sbr: sbrErrorIfCalled(t)},
			isError:         false,
		},
		{
			name:            "cache hit - error",
			svcbReturn:      true,
			svcbError:       errors.New("derp"),
			vscbShouldError: true,
			vscbError:       nil,
			stateByRooter:   &mockStateByRooter{sbr: sbrErrorIfCalled(t)},
			isError:         true,
		},
		{
			name:            "cache miss - success",
			svcbReturn:      false,
			svcbError:       nil,
			vscbShouldError: false,
			vscbError:       nil,
			stateByRooter:   sbrForValOverride(firstColumn.ProposerIndex(), validator),
			isError:         false,
		},
		{
			name:            "cache miss - state not found",
			svcbReturn:      false,
			svcbError:       nil,
			vscbShouldError: false,
			vscbError:       nil,
			stateByRooter:   sbrNotFound(t, expectedSignatureData.Parent),
			isError:         true,
		},
		{
			name:            "cache miss - signature failure",
			svcbReturn:      false,
			svcbError:       nil,
			vscbShouldError: false,
			vscbError:       errors.New("signature, not so good!"),
			stateByRooter:   sbrForValOverride(firstColumn.ProposerIndex(), validator),
			isError:         true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			signatureCache := &mockSignatureCache{
				svcb: func(signatureData SignatureData) (bool, error) {
					if signatureData != expectedSignatureData {
						t.Error("Did not see expected SignatureData")
					}
					return tc.svcbReturn, tc.svcbError
				},
				vscb: func(signatureData SignatureData, _ ValidatorAtIndexer) (err error) {
					if tc.vscbShouldError {
						t.Error("VerifySignature should not be called if the result is cached")
						return nil
					}

					if expectedSignatureData != signatureData {
						t.Error("unexpected signature data")
					}

					return tc.vscbError
				},
			}

			initializer := Initializer{
				shared: &sharedResources{
					sc: signatureCache,
					sr: tc.stateByRooter,
				},
			}

			verifier := initializer.NewDataColumnsVerifier(columns, GossipColumnSidecarRequirements)
			err := verifier.ValidProposerSignature(context.Background())
			require.Equal(t, true, verifier.results.executed(RequireValidProposerSignature))

			if tc.isError {
				require.ErrorIs(t, err, ErrInvalidProposerSignature)
				require.NotNil(t, verifier.results.result(RequireValidProposerSignature))
				return
			}

			require.NoError(t, err)
			require.NoError(t, verifier.results.result(RequireValidProposerSignature))
		})
	}
}

func TestDataColumnsSidecarParentSeen(t *testing.T) {
	const (
		columnSlot = 0
		blobCount  = 1
	)

	parentRoot := [fieldparams.RootLength]byte{}

	_, columns := util.GenerateTestDenebBlockWithColumns(t, parentRoot, columnSlot, blobCount)
	firstColumn := columns[0]

	fcHas := &mockForkchoicer{
		HasNodeCB: func(parent [fieldparams.RootLength]byte) bool {
			if parent != firstColumn.ParentRoot() {
				t.Error("forkchoice.HasNode called with unexpected parent root")
			}

			return true
		},
	}

	fcLacks := &mockForkchoicer{
		HasNodeCB: func(parent [fieldparams.RootLength]byte) bool {
			if parent != firstColumn.ParentRoot() {
				t.Error("forkchoice.HasNode called with unexpected parent root")
			}

			return false
		},
	}

	testCases := []struct {
		name        string
		forkChoicer Forkchoicer
		parentSeen  func([fieldparams.RootLength]byte) bool
		isError     bool
	}{
		{
			name:        "happy path",
			forkChoicer: fcHas,
			parentSeen:  nil,
			isError:     false,
		},
		{
			name:        "HasNode false, no badParent cb, expected error",
			forkChoicer: fcLacks,
			parentSeen:  nil,
			isError:     true,
		},
		{
			name:        "HasNode false, badParent true",
			forkChoicer: fcLacks,
			parentSeen:  badParentCb(t, firstColumn.ParentRoot(), true),
			isError:     false,
		},
		{
			name:        "HasNode false, badParent false",
			forkChoicer: fcLacks,
			parentSeen:  badParentCb(t, firstColumn.ParentRoot(), false),
			isError:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			initializer := Initializer{shared: &sharedResources{fc: tc.forkChoicer}}
			verifier := initializer.NewDataColumnsVerifier(columns, GossipColumnSidecarRequirements)
			err := verifier.SidecarParentSeen(tc.parentSeen)
			require.Equal(t, true, verifier.results.executed(RequireSidecarParentSeen))

			if tc.isError {
				require.ErrorIs(t, err, ErrSidecarParentNotSeen)
				require.NotNil(t, verifier.results.result(RequireSidecarParentSeen))
				return
			}

			require.NoError(t, err)
			require.NoError(t, verifier.results.result(RequireSidecarParentSeen))
		})
	}
}

func TestDataColumnsSidecarParentValid(t *testing.T) {
	testCases := []struct {
		name              string
		badParentCbReturn bool
		isError           bool
	}{
		{
			name:              "parent valid",
			badParentCbReturn: false,
			isError:           false,
		},
		{
			name:              "parent not valid",
			badParentCbReturn: true,
			isError:           true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			const (
				columnSlot = 0
				blobCount  = 1
			)

			parentRoot := [fieldparams.RootLength]byte{}

			_, columns := util.GenerateTestDenebBlockWithColumns(t, parentRoot, columnSlot, blobCount)
			firstColumn := columns[0]

			initializer := Initializer{shared: &sharedResources{}}
			verifier := initializer.NewDataColumnsVerifier(columns, GossipColumnSidecarRequirements)
			err := verifier.SidecarParentValid(badParentCb(t, firstColumn.ParentRoot(), tc.badParentCbReturn))
			require.Equal(t, true, verifier.results.executed(RequireSidecarParentValid))

			if tc.isError {
				require.ErrorIs(t, err, ErrSidecarParentInvalid)
				require.NotNil(t, verifier.results.result(RequireSidecarParentValid))
				return
			}

			require.NoError(t, err)
			require.NoError(t, verifier.results.result(RequireSidecarParentValid))
		})
	}
}

func TestColumnSidecarParentSlotLower(t *testing.T) {
	_, columns := util.GenerateTestDenebBlockWithColumns(t, [32]byte{}, 1, 1)
	firstColumn := columns[0]

	cases := []struct {
		name                 string
		forkChoiceSlot       primitives.Slot
		forkChoiceError, err error
	}{
		{
			name:            "Not in forkchoice",
			forkChoiceError: errors.New("not in forkchoice"),
			err:             ErrSlotNotAfterParent,
		},
		{
			name:           "In forkchoice, slot lower",
			forkChoiceSlot: firstColumn.Slot() - 1,
		},
		{
			name:           "In forkchoice, slot equal",
			forkChoiceSlot: firstColumn.Slot(),
			err:            ErrSlotNotAfterParent,
		},
		{
			name:           "In forkchoice, slot higher",
			forkChoiceSlot: firstColumn.Slot() + 1,
			err:            ErrSlotNotAfterParent,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			initializer := Initializer{
				shared: &sharedResources{fc: &mockForkchoicer{
					SlotCB: func(r [32]byte) (primitives.Slot, error) {
						if firstColumn.ParentRoot() != r {
							t.Error("forkchoice.Slot called with unexpected parent root")
						}

						return c.forkChoiceSlot, c.forkChoiceError
					},
				}},
			}

			verifier := initializer.NewDataColumnsVerifier(columns, GossipColumnSidecarRequirements)
			err := verifier.SidecarParentSlotLower()
			require.Equal(t, true, verifier.results.executed(RequireSidecarParentSlotLower))

			if c.err == nil {
				require.NoError(t, err)
				require.NoError(t, verifier.results.result(RequireSidecarParentSlotLower))
				return
			}

			require.ErrorIs(t, err, c.err)
			require.NotNil(t, verifier.results.result(RequireSidecarParentSlotLower))
		})
	}
}

func TestDataColumnsSidecarDescendsFromFinalized(t *testing.T) {
	testCases := []struct {
		name            string
		hasNodeCBReturn bool
		isError         bool
	}{
		{
			name:            "Not canonical",
			hasNodeCBReturn: false,
			isError:         true,
		},
		{
			name:            "Canonical",
			hasNodeCBReturn: true,
			isError:         false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			const (
				columnSlot = 0
				blobCount  = 1
			)

			parentRoot := [fieldparams.RootLength]byte{}

			_, columns := util.GenerateTestDenebBlockWithColumns(t, parentRoot, columnSlot, blobCount)
			firstColumn := columns[0]

			initializer := Initializer{
				shared: &sharedResources{
					fc: &mockForkchoicer{
						HasNodeCB: func(r [fieldparams.RootLength]byte) bool {
							if firstColumn.ParentRoot() != r {
								t.Error("forkchoice.Slot called with unexpected parent root")
							}

							return tc.hasNodeCBReturn
						},
					},
				},
			}

			verifier := initializer.NewDataColumnsVerifier(columns, GossipColumnSidecarRequirements)
			err := verifier.SidecarDescendsFromFinalized()
			require.Equal(t, true, verifier.results.executed(RequireSidecarDescendsFromFinalized))

			if tc.isError {
				require.ErrorIs(t, err, ErrSidecarNotFinalizedDescendent)
				require.NotNil(t, verifier.results.result(RequireSidecarDescendsFromFinalized))
				return
			}

			require.NoError(t, err)
			require.NoError(t, verifier.results.result(RequireSidecarDescendsFromFinalized))
		})
	}
}

func TestDataColumnsSidecarInclusionProven(t *testing.T) {
	testCases := []struct {
		name     string
		alterate bool
		isError  bool
	}{
		{
			name:     "Inclusion proven",
			alterate: false,
			isError:  false,
		},
		{
			name:     "Inclusion not proven",
			alterate: true,
			isError:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			const (
				columnSlot = 0
				blobCount  = 1
			)

			parentRoot := [fieldparams.RootLength]byte{}
			_, columns := util.GenerateTestDenebBlockWithColumns(t, parentRoot, columnSlot, blobCount)
			if tc.alterate {
				firstColumn := columns[0]
				byte0 := firstColumn.SignedBlockHeader.Header.BodyRoot[0]
				firstColumn.SignedBlockHeader.Header.BodyRoot[0] = byte0 ^ 255
			}

			initializer := Initializer{}
			verifier := initializer.NewDataColumnsVerifier(columns, GossipColumnSidecarRequirements)
			err := verifier.SidecarInclusionProven()
			require.Equal(t, true, verifier.results.executed(RequireSidecarInclusionProven))

			if tc.isError {
				require.ErrorIs(t, err, ErrSidecarInclusionProofInvalid)
				require.NotNil(t, verifier.results.result(RequireSidecarInclusionProven))
				return
			}

			require.NoError(t, err)
			require.NoError(t, verifier.results.result(RequireSidecarInclusionProven))
		})
	}
}

func TestDataColumnsSidecarKzgProofVerified(t *testing.T) {
	testCases := []struct {
		isError                           bool
		verifyDataColumnsCommitmentReturn bool
		verifyDataColumnsCommitmentError  error
		name                              string
	}{
		{
			name:                              "KZG proof verified",
			verifyDataColumnsCommitmentReturn: true,
			verifyDataColumnsCommitmentError:  nil,
			isError:                           false,
		},
		{
			name:                              "KZG proof error",
			verifyDataColumnsCommitmentReturn: false,
			verifyDataColumnsCommitmentError:  errors.New("KZG proof error"),
			isError:                           true,
		},
		{
			name:                              "KZG proof not verified",
			verifyDataColumnsCommitmentReturn: false,
			verifyDataColumnsCommitmentError:  nil,
			isError:                           true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			const (
				columnSlot = 0
				blobCount  = 1
			)

			parentRoot := [fieldparams.RootLength]byte{}
			_, columns := util.GenerateTestDenebBlockWithColumns(t, parentRoot, columnSlot, blobCount)
			firstColumn := columns[0]

			verifyDataColumnsCommitment := func(roDataColumns []blocks.RODataColumn) (bool, error) {
				for _, roDataColumn := range roDataColumns {
					require.Equal(t, true, reflect.DeepEqual(firstColumn.KzgCommitments, roDataColumn.KzgCommitments))
				}

				return tc.verifyDataColumnsCommitmentReturn, tc.verifyDataColumnsCommitmentError
			}

			verifier := &RODataColumnsVerifier{
				results:                     newResults(),
				dataColumns:                 columns,
				verifyDataColumnsCommitment: verifyDataColumnsCommitment,
			}

			err := verifier.SidecarKzgProofVerified()
			require.Equal(t, true, verifier.results.executed(RequireSidecarKzgProofVerified))

			if tc.isError {
				require.ErrorIs(t, err, ErrSidecarKzgProofInvalid)
				require.NotNil(t, verifier.results.result(RequireSidecarKzgProofVerified))
				return
			}

			require.NoError(t, err)
			require.NoError(t, verifier.results.result(RequireSidecarKzgProofVerified))
		})
	}
}

func TestDataColumnsSidecarProposerExpected(t *testing.T) {
	const (
		columnSlot = 1
		blobCount  = 1
	)

	parentRoot := [fieldparams.RootLength]byte{}
	_, columns := util.GenerateTestDenebBlockWithColumns(t, parentRoot, columnSlot, blobCount)
	firstColumn := columns[0]

	_, newColumns := util.GenerateTestDenebBlockWithColumns(t, parentRoot, 2*params.BeaconConfig().SlotsPerEpoch, blobCount)
	firstNewColumn := newColumns[0]

	validator := &ethpb.Validator{}

	commonComputeProposerCB := func(_ context.Context, root [fieldparams.RootLength]byte, slot primitives.Slot, _ state.BeaconState) (primitives.ValidatorIndex, error) {
		require.Equal(t, firstColumn.ParentRoot(), root)
		require.Equal(t, firstColumn.Slot(), slot)
		return firstColumn.ProposerIndex(), nil
	}

	testCases := []struct {
		name          string
		stateByRooter StateByRooter
		proposerCache ProposerCache
		columns       []blocks.RODataColumn
		isError       bool
	}{
		{
			name:          "Cached, matches",
			stateByRooter: nil,
			proposerCache: &mockProposerCache{
				ProposerCB: pcReturnsIdx(firstColumn.ProposerIndex()),
			},
			columns: columns,
			isError: false,
		},
		{
			name:          "Cached, does not match",
			stateByRooter: nil,
			proposerCache: &mockProposerCache{
				ProposerCB: pcReturnsIdx(firstColumn.ProposerIndex() + 1),
			},
			columns: columns,
			isError: true,
		},
		{
			name:          "Not cached, state lookup failure",
			stateByRooter: sbrNotFound(t, firstColumn.ParentRoot()),
			proposerCache: &mockProposerCache{
				ProposerCB: pcReturnsNotFound(),
			},
			columns: columns,
			isError: true,
		},
		{
			name:          "Not cached, proposer matches",
			stateByRooter: sbrForValOverride(firstColumn.ProposerIndex(), validator),
			proposerCache: &mockProposerCache{
				ProposerCB:        pcReturnsNotFound(),
				ComputeProposerCB: commonComputeProposerCB,
			},
			columns: columns,
			isError: false,
		},
		{
			name:          "Not cached, proposer matches",
			stateByRooter: sbrForValOverride(firstColumn.ProposerIndex(), validator),
			proposerCache: &mockProposerCache{
				ProposerCB:        pcReturnsNotFound(),
				ComputeProposerCB: commonComputeProposerCB,
			},
			columns: columns,
			isError: false,
		},
		{
			name:          "Not cached, proposer matches for next epoch",
			stateByRooter: sbrForValOverride(firstNewColumn.ProposerIndex(), validator),
			proposerCache: &mockProposerCache{
				ProposerCB: pcReturnsNotFound(),
				ComputeProposerCB: func(_ context.Context, root [32]byte, slot primitives.Slot, _ state.BeaconState) (primitives.ValidatorIndex, error) {
					require.Equal(t, firstNewColumn.ParentRoot(), root)
					require.Equal(t, firstNewColumn.Slot(), slot)
					return firstColumn.ProposerIndex(), nil
				},
			},
			columns: newColumns,
			isError: false,
		},
		{
			name:          "Not cached, proposer does not match",
			stateByRooter: sbrForValOverride(firstColumn.ProposerIndex(), validator),
			proposerCache: &mockProposerCache{
				ProposerCB: pcReturnsNotFound(),
				ComputeProposerCB: func(_ context.Context, root [32]byte, slot primitives.Slot, _ state.BeaconState) (primitives.ValidatorIndex, error) {
					require.Equal(t, firstColumn.ParentRoot(), root)
					require.Equal(t, firstColumn.Slot(), slot)
					return firstColumn.ProposerIndex() + 1, nil
				},
			},
			columns: columns,
			isError: true,
		},
		{
			name:          "Not cached, ComputeProposer fails",
			stateByRooter: sbrForValOverride(firstColumn.ProposerIndex(), validator),
			proposerCache: &mockProposerCache{
				ProposerCB: pcReturnsNotFound(),
				ComputeProposerCB: func(_ context.Context, root [32]byte, slot primitives.Slot, _ state.BeaconState) (primitives.ValidatorIndex, error) {
					require.Equal(t, firstColumn.ParentRoot(), root)
					require.Equal(t, firstColumn.Slot(), slot)
					return 0, errors.New("ComputeProposer failed")
				},
			},
			columns: columns,
			isError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			initializer := Initializer{
				shared: &sharedResources{
					sr: tc.stateByRooter,
					pc: tc.proposerCache,
					fc: &mockForkchoicer{
						TargetRootForEpochCB: fcReturnsTargetRoot([fieldparams.RootLength]byte{}),
					},
				},
			}

			verifier := initializer.NewDataColumnsVerifier(tc.columns, GossipColumnSidecarRequirements)
			err := verifier.SidecarProposerExpected(context.Background())

			require.Equal(t, true, verifier.results.executed(RequireSidecarProposerExpected))

			if tc.isError {
				require.ErrorIs(t, err, ErrSidecarUnexpectedProposer)
				require.NotNil(t, verifier.results.result(RequireSidecarProposerExpected))
				return
			}

			require.NoError(t, err)
			require.NoError(t, verifier.results.result(RequireSidecarProposerExpected))
		})
	}
}

func TestColumnRequirementSatisfaction(t *testing.T) {
	const (
		columnSlot = 1
		blobCount  = 1
	)

	parentRoot := [fieldparams.RootLength]byte{}

	_, columns := util.GenerateTestDenebBlockWithColumns(t, parentRoot, columnSlot, blobCount)
	initializer := Initializer{}
	verifier := initializer.NewDataColumnsVerifier(columns, GossipColumnSidecarRequirements)

	_, err := verifier.VerifiedRODataColumns()
	require.ErrorIs(t, err, ErrColumnInvalid)

	var me VerificationMultiError
	ok := errors.As(err, &me)
	require.Equal(t, true, ok)
	fails := me.Failures()

	// We haven't performed any verification, so all the results should be this type.
	for _, v := range fails {
		require.ErrorIs(t, v, ErrMissingVerification)
	}

	// Satisfy everything through the backdoor and ensure we get the verified ro blob at the end.
	for _, r := range GossipColumnSidecarRequirements {
		verifier.results.record(r, nil)
	}

	require.Equal(t, true, verifier.results.allSatisfied())
	_, err = verifier.VerifiedRODataColumns()

	require.NoError(t, err)
}

func TestColumnSatisfyRequirement(t *testing.T) {
	const (
		columnSlot = 1
		blobCount  = 1
	)

	parentRoot := [fieldparams.RootLength]byte{}

	_, columns := util.GenerateTestDenebBlockWithColumns(t, parentRoot, columnSlot, blobCount)
	intializer := Initializer{}

	v := intializer.NewDataColumnsVerifier(columns, GossipColumnSidecarRequirements)
	require.Equal(t, false, v.results.executed(RequireDataColumnIndexInBounds))
	v.SatisfyRequirement(RequireDataColumnIndexInBounds)
	require.Equal(t, true, v.results.executed(RequireDataColumnIndexInBounds))
}
