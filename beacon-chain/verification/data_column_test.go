package verification

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

func GenerateTestDataColumns(t *testing.T, parent [fieldparams.RootLength]byte, slot primitives.Slot, blobCount int) []blocks.RODataColumn {
	roBlock, roBlobs := util.GenerateTestDenebBlockWithSidecar(t, parent, slot, blobCount)
	blobs := make([]kzg.Blob, 0, len(roBlobs))
	for i := range roBlobs {
		blobs = append(blobs, kzg.Blob(roBlobs[i].Blob))
	}

	cellsPerBlob, proofsPerBlob := util.GenerateCellsAndProofs(t, blobs)
	roDataColumnSidecars, err := peerdas.DataColumnSidecars(cellsPerBlob, proofsPerBlob, peerdas.PopulateFromBlock(roBlock))
	require.NoError(t, err)

	return roDataColumnSidecars
}

func TestColumnSatisfyRequirement(t *testing.T) {
	const (
		columnSlot = 1
		blobCount  = 1
	)

	parentRoot := [fieldparams.RootLength]byte{}

	columns := GenerateTestDataColumns(t, parentRoot, columnSlot, blobCount)
	intializer := Initializer{}

	v := intializer.NewDataColumnsVerifier(columns, GossipDataColumnSidecarRequirements)
	require.Equal(t, false, v.results.executed(RequireValidProposerSignature))
	v.SatisfyRequirement(RequireValidProposerSignature)
	require.Equal(t, true, v.results.executed(RequireValidProposerSignature))
}

func TestValid(t *testing.T) {
	var initializer Initializer

	t.Run("one invalid column", func(t *testing.T) {
		columns := GenerateTestDataColumns(t, [fieldparams.RootLength]byte{}, 1, 1)
		verifier := initializer.NewDataColumnsVerifier(columns, GossipDataColumnSidecarRequirements)

		err := verifier.ValidFields()
		require.NotNil(t, err)
		require.NotNil(t, verifier.results.result(RequireValidFields))
	})

	t.Run("nominal", func(t *testing.T) {
		const maxBlobsPerBlock = 2

		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig()
		cfg.FuluForkEpoch = 0
		cfg.BlobSchedule = []params.BlobScheduleEntry{{Epoch: 0, MaxBlobsPerBlock: maxBlobsPerBlock}}
		params.OverrideBeaconConfig(cfg)

		columns := GenerateTestDataColumns(t, [fieldparams.RootLength]byte{}, 1, 1)
		verifier := initializer.NewDataColumnsVerifier(columns, GossipDataColumnSidecarRequirements)

		err := verifier.ValidFields()
		require.NoError(t, err)
		require.IsNil(t, verifier.results.result(RequireValidFields))

		err = verifier.ValidFields()
		require.NoError(t, err)
	})
}

func TestCorrectSubnet(t *testing.T) {
	const dataColumnSidecarSubTopic = "/data_column_sidecar_%d/"

	var initializer Initializer

	t.Run("lengths mismatch", func(t *testing.T) {
		columns := GenerateTestDataColumns(t, [fieldparams.RootLength]byte{}, 1, 1)
		verifier := initializer.NewDataColumnsVerifier(columns, GossipDataColumnSidecarRequirements)

		err := verifier.CorrectSubnet(dataColumnSidecarSubTopic, []string{})
		require.ErrorIs(t, err, errBadTopicLength)
		require.NotNil(t, verifier.results.result(RequireCorrectSubnet))
	})

	t.Run("wrong topic", func(t *testing.T) {
		columns := GenerateTestDataColumns(t, [fieldparams.RootLength]byte{}, 1, 1)
		verifier := initializer.NewDataColumnsVerifier(columns[:2], GossipDataColumnSidecarRequirements)

		err := verifier.CorrectSubnet(
			dataColumnSidecarSubTopic,
			[]string{
				"/eth2/9dc47cc6/data_column_sidecar_1/ssz_snappy",
				"/eth2/9dc47cc6/data_column_sidecar_0/ssz_snappy",
			})

		require.ErrorIs(t, err, errBadTopic)
		require.NotNil(t, verifier.results.result(RequireCorrectSubnet))
	})

	t.Run("nominal", func(t *testing.T) {
		subnets := []string{
			"/eth2/9dc47cc6/data_column_sidecar_0/ssz_snappy",
			"/eth2/9dc47cc6/data_column_sidecar_1",
		}

		columns := GenerateTestDataColumns(t, [fieldparams.RootLength]byte{}, 1, 1)
		verifier := initializer.NewDataColumnsVerifier(columns[:2], GossipDataColumnSidecarRequirements)

		err := verifier.CorrectSubnet(dataColumnSidecarSubTopic, subnets)
		require.NoError(t, err)
		require.IsNil(t, verifier.results.result(RequireCorrectSubnet))

		err = verifier.CorrectSubnet(dataColumnSidecarSubTopic, subnets)
		require.NoError(t, err)
	})
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

			columns := GenerateTestDataColumns(t, parentRoot, tc.columnSlot, blobCount)
			verifier := initializer.NewDataColumnsVerifier(columns, GossipDataColumnSidecarRequirements)

			err := verifier.NotFromFutureSlot()
			require.Equal(t, true, verifier.results.executed(RequireNotFromFutureSlot))

			if tc.isError {
				require.ErrorIs(t, err, errFromFutureSlot)
				require.NotNil(t, verifier.results.result(RequireNotFromFutureSlot))
				return
			}

			require.NoError(t, err)
			require.NoError(t, verifier.results.result(RequireNotFromFutureSlot))

			err = verifier.NotFromFutureSlot()
			require.NoError(t, err)
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

			columns := GenerateTestDataColumns(t, parentRoot, tc.columnSlot, blobCount)

			v := initializer.NewDataColumnsVerifier(columns, GossipDataColumnSidecarRequirements)

			err := v.SlotAboveFinalized()
			require.Equal(t, true, v.results.executed(RequireSlotAboveFinalized))

			if tc.isErr {
				require.ErrorIs(t, err, errSlotNotAfterFinalized)
				require.NotNil(t, v.results.result(RequireSlotAboveFinalized))
				return
			}

			require.NoError(t, err)
			require.NoError(t, v.results.result(RequireSlotAboveFinalized))

			err = v.SlotAboveFinalized()
			require.NoError(t, err)
		})
	}
}

func TestValidProposerSignature(t *testing.T) {
	const (
		columnSlot = 97
		blobCount  = 1
	)

	parentRoot := [fieldparams.RootLength]byte{}
	validator := &ethpb.Validator{}

	columns := GenerateTestDataColumns(t, parentRoot, columnSlot, blobCount)
	firstColumn := columns[0]

	// The signature data does not depend on the data column itself, so we can use the first one.
	expectedSignatureData, err := columnToSignatureData(firstColumn)
	require.NoError(t, err)

	// Create a proper Fulu state for verification.
	// We need enough validators to cover the proposer index.
	firstColumnPI, err := firstColumn.ProposerIndex()
	require.NoError(t, err)
	numValidators := max(uint64(firstColumnPI+1), 64)
	fuluState, _ := util.DeterministicGenesisStateFulu(t, numValidators)

	// Head state provider that returns the fuluState via HeadStateReadOnly path.
	headStateWithState := &mockHeadStateProvider{
		headRoot:          parentRoot[:],
		headSlot:          columnSlot,
		headStateReadOnly: fuluState,
	}

	// Head state provider that will fail (headStateReadOnly is nil).
	headStateNotFound := &mockHeadStateProvider{
		headRoot: parentRoot[:],
		headSlot: columnSlot,
	}

	testCases := []struct {
		isError           bool
		vscbShouldError   bool
		svcbReturn        bool
		stateByRooter     StateByRooter
		headStateProvider *mockHeadStateProvider
		vscbError         error
		svcbError         error
		name              string
	}{
		{
			name:              "cache hit - success",
			svcbReturn:        true,
			svcbError:         nil,
			vscbShouldError:   true,
			vscbError:         nil,
			stateByRooter:     &mockStateByRooter{sbr: sbrErrorIfCalled(t)},
			headStateProvider: headStateWithState,
			isError:           false,
		},
		{
			name:              "cache hit - error",
			svcbReturn:        true,
			svcbError:         errors.New("derp"),
			vscbShouldError:   true,
			vscbError:         nil,
			stateByRooter:     &mockStateByRooter{sbr: sbrErrorIfCalled(t)},
			headStateProvider: headStateWithState,
			isError:           true,
		},
		{
			name:              "cache miss - success",
			svcbReturn:        false,
			svcbError:         nil,
			vscbShouldError:   false,
			vscbError:         nil,
			stateByRooter:     sbrForValOverrideWithT(t, firstColumnPI, validator),
			headStateProvider: headStateWithState,
			isError:           false,
		},
		{
			name:              "cache miss - state not found",
			svcbReturn:        false,
			svcbError:         nil,
			vscbShouldError:   false,
			vscbError:         nil,
			stateByRooter:     sbrNotFound(t, expectedSignatureData.Parent),
			headStateProvider: headStateNotFound,
			isError:           true,
		},
		{
			name:              "cache miss - signature failure",
			svcbReturn:        false,
			svcbError:         nil,
			vscbShouldError:   false,
			vscbError:         errors.New("signature, not so good!"),
			stateByRooter:     sbrForValOverrideWithT(t, firstColumnPI, validator),
			headStateProvider: headStateWithState,
			isError:           true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			signatureCache := &mockSignatureCache{
				svcb: func(signatureData signatureData) (bool, error) {
					if signatureData != expectedSignatureData {
						t.Error("Did not see expected SignatureData")
					}
					return tc.svcbReturn, tc.svcbError
				},
				vscb: func(signatureData signatureData, _ validatorAtIndexer) (err error) {
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
					sc:  signatureCache,
					sr:  tc.stateByRooter,
					hsp: tc.headStateProvider,
					fc: &mockForkchoicer{
						DependentRootForEpochCB: fcReturnsDependentRoot(),
						TargetRootForEpochCB:    fcReturnsTargetRoot([fieldparams.RootLength]byte{}),
					},
				},
			}

			verifier := initializer.NewDataColumnsVerifier(columns, GossipDataColumnSidecarRequirements)
			err := verifier.ValidProposerSignature(t.Context())
			require.Equal(t, true, verifier.results.executed(RequireValidProposerSignature))

			if tc.isError {
				require.NotNil(t, err)
				require.NotNil(t, verifier.results.result(RequireValidProposerSignature))
				return
			}

			require.NoError(t, err)
			require.NoError(t, verifier.results.result(RequireValidProposerSignature))

			err = verifier.ValidProposerSignature(t.Context())
			require.NoError(t, err)
		})
	}
}

func TestDataColumnsSidecarParentSeen(t *testing.T) {
	const (
		columnSlot = 97
		blobCount  = 1
	)

	parentRoot := [fieldparams.RootLength]byte{}

	columns := GenerateTestDataColumns(t, parentRoot, columnSlot, blobCount)
	firstColumn := columns[0]

	firstColumnParent, err := firstColumn.ParentRoot()
	require.NoError(t, err)

	fcHas := &mockForkchoicer{
		HasNodeCB: func(parent [fieldparams.RootLength]byte) bool {
			if parent != firstColumnParent {
				t.Error("forkchoice.HasNode called with unexpected parent root")
			}

			return true
		},
	}

	fcLacks := &mockForkchoicer{
		HasNodeCB: func(parent [fieldparams.RootLength]byte) bool {
			if parent != firstColumnParent {
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
			parentSeen:  badParentCb(t, firstColumnParent, true),
			isError:     false,
		},
		{
			name:        "HasNode false, badParent false",
			forkChoicer: fcLacks,
			parentSeen:  badParentCb(t, firstColumnParent, false),
			isError:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			initializer := Initializer{shared: &sharedResources{fc: tc.forkChoicer}}
			verifier := initializer.NewDataColumnsVerifier(columns, GossipDataColumnSidecarRequirements)
			err := verifier.SidecarParentSeen(tc.parentSeen)
			require.Equal(t, true, verifier.results.executed(RequireSidecarParentSeen))

			if tc.isError {
				require.ErrorIs(t, err, errSidecarParentNotSeen)
				require.NotNil(t, verifier.results.result(RequireSidecarParentSeen))
				return
			}

			require.NoError(t, err)
			require.NoError(t, verifier.results.result(RequireSidecarParentSeen))

			err = verifier.SidecarParentSeen(tc.parentSeen)
			require.NoError(t, err)
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
				columnSlot = 97
				blobCount  = 1
			)

			parentRoot := [fieldparams.RootLength]byte{}

			columns := GenerateTestDataColumns(t, parentRoot, columnSlot, blobCount)
			firstColumn := columns[0]

			firstColumnParent, err := firstColumn.ParentRoot()
			require.NoError(t, err)

			initializer := Initializer{shared: &sharedResources{}}
			verifier := initializer.NewDataColumnsVerifier(columns, GossipDataColumnSidecarRequirements)
			err = verifier.SidecarParentValid(badParentCb(t, firstColumnParent, tc.badParentCbReturn))
			require.Equal(t, true, verifier.results.executed(RequireSidecarParentValid))

			if tc.isError {
				require.ErrorIs(t, err, errSidecarParentInvalid)
				require.NotNil(t, verifier.results.result(RequireSidecarParentValid))
				return
			}

			require.NoError(t, err)
			require.NoError(t, verifier.results.result(RequireSidecarParentValid))

			err = verifier.SidecarParentValid(badParentCb(t, firstColumnParent, tc.badParentCbReturn))
			require.NoError(t, err)
		})
	}
}

func TestColumnSidecarParentSlotLower(t *testing.T) {
	columns := GenerateTestDataColumns(t, [32]byte{}, 1, 1)
	firstColumn := columns[0]

	firstColumnParent, err := firstColumn.ParentRoot()
	require.NoError(t, err)

	cases := []struct {
		name                 string
		forkChoiceSlot       primitives.Slot
		forkChoiceError, err error
		errCheckValue        bool
	}{
		{
			name:            "Not in forkchoice",
			forkChoiceError: errors.New("not in forkchoice"),
			err:             errSlotNotAfterParent,
		},
		{
			name:           "In forkchoice, slot lower",
			forkChoiceSlot: firstColumn.Slot() - 1,
		},
		{
			name:           "In forkchoice, slot equal",
			forkChoiceSlot: firstColumn.Slot(),
			err:            errSlotNotAfterParent,
			errCheckValue:  true,
		},
		{
			name:           "In forkchoice, slot higher",
			forkChoiceSlot: firstColumn.Slot() + 1,
			err:            errSlotNotAfterParent,
			errCheckValue:  true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			initializer := Initializer{
				shared: &sharedResources{fc: &mockForkchoicer{
					SlotCB: func(r [32]byte) (primitives.Slot, error) {
						if firstColumnParent != r {
							t.Error("forkchoice.Slot called with unexpected parent root")
						}

						return c.forkChoiceSlot, c.forkChoiceError
					},
				}},
			}

			verifier := initializer.NewDataColumnsVerifier(columns, GossipDataColumnSidecarRequirements)
			err := verifier.SidecarParentSlotLower()
			require.Equal(t, true, verifier.results.executed(RequireSidecarParentSlotLower))

			if c.err == nil {
				require.NoError(t, err)
				require.NoError(t, verifier.results.result(RequireSidecarParentSlotLower))

				err = verifier.SidecarParentSlotLower()
				require.NoError(t, err)

				return
			}

			require.NotNil(t, err)
			require.NotNil(t, verifier.results.result(RequireSidecarParentSlotLower))

			if c.errCheckValue {
				require.ErrorIs(t, err, c.err)
			}
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
				columnSlot = 97
				blobCount  = 1
			)

			parentRoot := [fieldparams.RootLength]byte{}

			columns := GenerateTestDataColumns(t, parentRoot, columnSlot, blobCount)
			firstColumn := columns[0]

			firstColumnParent, err := firstColumn.ParentRoot()
			require.NoError(t, err)

			initializer := Initializer{
				shared: &sharedResources{
					fc: &mockForkchoicer{
						HasNodeCB: func(r [fieldparams.RootLength]byte) bool {
							if firstColumnParent != r {
								t.Error("forkchoice.Slot called with unexpected parent root")
							}

							return tc.hasNodeCBReturn
						},
					},
				},
			}

			verifier := initializer.NewDataColumnsVerifier(columns, GossipDataColumnSidecarRequirements)
			err = verifier.SidecarDescendsFromFinalized()
			require.Equal(t, true, verifier.results.executed(RequireSidecarDescendsFromFinalized))

			if tc.isError {
				require.ErrorIs(t, err, errSidecarNotFinalizedDescendent)
				require.NotNil(t, verifier.results.result(RequireSidecarDescendsFromFinalized))
				return
			}

			require.NoError(t, err)
			require.NoError(t, verifier.results.result(RequireSidecarDescendsFromFinalized))

			err = verifier.SidecarDescendsFromFinalized()
			require.NoError(t, err)
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
				columnSlot = 97
				blobCount  = 1
			)

			parentRoot := [fieldparams.RootLength]byte{}
			columns := GenerateTestDataColumns(t, parentRoot, columnSlot, blobCount)
			if tc.alterate {
				firstColumn := columns[0]
				sbh, err := firstColumn.SignedBlockHeader()
				require.NoError(t, err)
				byte0 := sbh.Header.BodyRoot[0]
				sbh.Header.BodyRoot[0] = byte0 ^ 255
			}

			initializer := Initializer{
				shared: &sharedResources{ic: newInclusionProofCache(1)},
			}
			verifier := initializer.NewDataColumnsVerifier(columns, GossipDataColumnSidecarRequirements)
			err := verifier.SidecarInclusionProven()
			require.Equal(t, true, verifier.results.executed(RequireSidecarInclusionProven))

			if tc.isError {
				require.ErrorIs(t, err, ErrSidecarInclusionProofInvalid)
				require.NotNil(t, verifier.results.result(RequireSidecarInclusionProven))
				return
			}

			require.NoError(t, err)
			require.NoError(t, verifier.results.result(RequireSidecarInclusionProven))

			err = verifier.SidecarInclusionProven()
			require.NoError(t, err)
		})
	}
}

func TestDataColumnsSidecarKzgProofVerified(t *testing.T) {
	testCases := []struct {
		isError                          bool
		verifyDataColumnsCommitmentError error
		name                             string
	}{
		{
			name:                             "KZG proof verified",
			verifyDataColumnsCommitmentError: nil,
			isError:                          false,
		},
		{
			name:                             "KZG proof not verified",
			verifyDataColumnsCommitmentError: errors.New("KZG proof error"),
			isError:                          true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			const (
				columnSlot = 97
				blobCount  = 1
			)

			parentRoot := [fieldparams.RootLength]byte{}
			columns := GenerateTestDataColumns(t, parentRoot, columnSlot, blobCount)
			firstColumn := columns[0]

			firstColumnCommitments, err := firstColumn.KzgCommitments()
			require.NoError(t, err)

			verifyDataColumnsCommitment := func(roDataColumns []blocks.RODataColumn) error {
				for _, roDataColumn := range roDataColumns {
					roCommitments, err := roDataColumn.KzgCommitments()
					require.NoError(t, err)
					require.Equal(t, true, reflect.DeepEqual(firstColumnCommitments, roCommitments))
				}

				return tc.verifyDataColumnsCommitmentError
			}

			verifier := &RODataColumnsVerifier{
				results:                     newResults(),
				dataColumns:                 columns,
				verifyDataColumnsCommitment: verifyDataColumnsCommitment,
			}

			err = verifier.SidecarKzgProofVerified()
			require.Equal(t, true, verifier.results.executed(RequireSidecarKzgProofVerified))

			if tc.isError {
				require.NotNil(t, err)
				require.NotNil(t, verifier.results.result(RequireSidecarKzgProofVerified))
				return
			}

			require.NoError(t, err)
			require.NoError(t, verifier.results.result(RequireSidecarKzgProofVerified))

			err = verifier.SidecarKzgProofVerified()
			require.NoError(t, err)
		})
	}
}

func TestDataColumnsSidecarProposerExpected(t *testing.T) {
	const (
		columnSlot = 1
		blobCount  = 1
	)

	ctx := t.Context()
	parentRoot := [fieldparams.RootLength]byte{}

	// Create a Fulu state to get the expected proposer from the lookahead.
	fuluState, _ := util.DeterministicGenesisStateFulu(t, 32)
	expectedProposer, err := fuluState.ProposerLookahead()
	require.NoError(t, err)
	expectedProposerIdx := primitives.ValidatorIndex(expectedProposer[columnSlot])

	// Generate data columns with the expected proposer index.
	matchingColumns := generateTestDataColumnsWithProposer(t, parentRoot, columnSlot, blobCount, expectedProposerIdx)
	// Generate data columns with wrong proposer index.
	wrongColumns := generateTestDataColumnsWithProposer(t, parentRoot, columnSlot, blobCount, expectedProposerIdx+1)

	t.Run("Proposer matches", func(t *testing.T) {
		initializer := Initializer{
			shared: &sharedResources{
				sr: sbrReturnsState(fuluState),
				hsp: &mockHeadStateProvider{
					headRoot:          parentRoot[:],
					headSlot:          columnSlot, // Same epoch so HeadStateReadOnly is used
					headStateReadOnly: fuluState,
				},
				fc: &mockForkchoicer{},
			},
		}

		verifier := initializer.NewDataColumnsVerifier(matchingColumns, GossipDataColumnSidecarRequirements)
		err := verifier.SidecarProposerExpected(ctx)
		require.NoError(t, err)
		require.Equal(t, true, verifier.results.executed(RequireSidecarProposerExpected))
		require.NoError(t, verifier.results.result(RequireSidecarProposerExpected))
	})

	t.Run("Proposer does not match", func(t *testing.T) {
		initializer := Initializer{
			shared: &sharedResources{
				sr: sbrReturnsState(fuluState),
				hsp: &mockHeadStateProvider{
					headRoot:          parentRoot[:],
					headSlot:          columnSlot, // Same epoch so HeadStateReadOnly is used
					headStateReadOnly: fuluState,
				},
				fc: &mockForkchoicer{},
			},
		}

		verifier := initializer.NewDataColumnsVerifier(wrongColumns, GossipDataColumnSidecarRequirements)
		err := verifier.SidecarProposerExpected(ctx)
		require.ErrorContains(t, errSidecarUnexpectedProposer.Error(), err)
		require.Equal(t, true, verifier.results.executed(RequireSidecarProposerExpected))
		require.NotNil(t, verifier.results.result(RequireSidecarProposerExpected))
	})

	t.Run("State lookup failure", func(t *testing.T) {
		columns := GenerateTestDataColumns(t, parentRoot, columnSlot, blobCount)
		col0Parent, err := columns[0].ParentRoot()
		require.NoError(t, err)
		initializer := Initializer{
			shared: &sharedResources{
				sr:  sbrNotFound(t, col0Parent),
				hsp: &mockHeadStateProvider{},
				fc:  &mockForkchoicer{},
			},
		}

		verifier := initializer.NewDataColumnsVerifier(columns, GossipDataColumnSidecarRequirements)
		err = verifier.SidecarProposerExpected(ctx)
		require.ErrorContains(t, "verifying state", err)
		require.Equal(t, true, verifier.results.executed(RequireSidecarProposerExpected))
		require.NotNil(t, verifier.results.result(RequireSidecarProposerExpected))
	})
}

func generateTestDataColumnsWithProposer(t *testing.T, parent [fieldparams.RootLength]byte, slot primitives.Slot, blobCount int, proposer primitives.ValidatorIndex) []blocks.RODataColumn {
	roBlock, roBlobs := util.GenerateTestDenebBlockWithSidecar(t, parent, slot, blobCount, util.WithProposer(proposer))
	blobs := make([]kzg.Blob, 0, len(roBlobs))
	for i := range roBlobs {
		blobs = append(blobs, kzg.Blob(roBlobs[i].Blob))
	}

	cellsPerBlob, proofsPerBlob := util.GenerateCellsAndProofs(t, blobs)
	roDataColumnSidecars, err := peerdas.DataColumnSidecars(cellsPerBlob, proofsPerBlob, peerdas.PopulateFromBlock(roBlock))
	require.NoError(t, err)

	return roDataColumnSidecars
}

func TestColumnRequirementSatisfaction(t *testing.T) {
	const (
		columnSlot = 1
		blobCount  = 1
	)

	parentRoot := [fieldparams.RootLength]byte{}

	columns := GenerateTestDataColumns(t, parentRoot, columnSlot, blobCount)
	initializer := Initializer{}
	verifier := initializer.NewDataColumnsVerifier(columns, GossipDataColumnSidecarRequirements)

	// We haven't performed any verification, VerifiedRODataColumns should error.
	_, err := verifier.VerifiedRODataColumns()
	require.ErrorIs(t, err, errColumnsInvalid)

	var me VerificationMultiError
	ok := errors.As(err, &me)
	require.Equal(t, true, ok)
	fails := me.Failures()

	// We haven't performed any verification, so all the results should be this type.
	for _, v := range fails {
		require.ErrorIs(t, v, ErrMissingVerification)
	}

	// Satisfy everything but the first requirement through the backdoor.
	for _, r := range GossipDataColumnSidecarRequirements[1:] {
		verifier.results.record(r, nil)
	}

	// One requirement is missing, VerifiedRODataColumns should still error.
	_, err = verifier.VerifiedRODataColumns()
	require.ErrorIs(t, err, errColumnsInvalid)

	// Now, satisfy the first requirement.
	verifier.results.record(GossipDataColumnSidecarRequirements[0], nil)

	// VerifiedRODataColumns should now succeed.
	require.Equal(t, true, verifier.results.allSatisfied())
	_, err = verifier.VerifiedRODataColumns()
	require.NoError(t, err)
}

func TestGetVerifyingStateEdgeCases(t *testing.T) {
	const (
		columnSlot = 97 // epoch 3
		blobCount  = 1
	)

	parentRoot := [fieldparams.RootLength]byte{}
	columns := GenerateTestDataColumns(t, parentRoot, columnSlot, blobCount)

	// Create a proper Fulu state for verification.
	col0PI, err := columns[0].ProposerIndex()
	require.NoError(t, err)
	numValidators := max(uint64(col0PI+1), 64)
	fuluState, _ := util.DeterministicGenesisStateFulu(t, numValidators)

	t.Run("different dependent roots - uses StateByRoot path", func(t *testing.T) {
		// Parent and head are on different forks with different dependent roots.
		// This forces the code to use TargetRootForEpoch -> StateByRoot path.
		signatureCache := &mockSignatureCache{
			svcb: func(signatureData signatureData) (bool, error) {
				return false, nil // Cache miss
			},
			vscb: func(signatureData signatureData, _ validatorAtIndexer) (err error) {
				return nil // Signature valid
			},
		}

		// StateByRoot will be called because dependent roots differ
		stateByRootCalled := false
		stateByRooter := &mockStateByRooter{
			sbr: func(_ context.Context, root [32]byte) (state.BeaconState, error) {
				stateByRootCalled = true
				return fuluState, nil
			},
		}

		initializer := Initializer{
			shared: &sharedResources{
				sc: signatureCache,
				sr: stateByRooter,
				hsp: &mockHeadStateProvider{
					headRoot: []byte{0xff}, // Different from parentRoot
					headSlot: columnSlot,
				},
				fc: &mockForkchoicer{
					// Return different roots for parent vs head to simulate different forks
					DependentRootForEpochCB: func(root [32]byte, epoch primitives.Epoch) ([32]byte, error) {
						return root, nil // Returns input, so parent [0...] != head [0xff...]
					},
					TargetRootForEpochCB: fcReturnsTargetRoot([fieldparams.RootLength]byte{}),
				},
			},
		}

		verifier := initializer.NewDataColumnsVerifier(columns, GossipDataColumnSidecarRequirements)
		err := verifier.ValidProposerSignature(t.Context())
		require.NoError(t, err)
		require.Equal(t, true, stateByRootCalled, "StateByRoot should be called when dependent roots differ")
	})

	t.Run("same dependent root head far ahead - uses head state with ProcessSlots", func(t *testing.T) {
		// Parent is ancestor of head on same chain, but head is in epoch 1 while column is in epoch 3.
		// headEpoch (1) + 1 < dataColumnEpoch (3), so ProcessSlots is called on head state.
		signatureCache := &mockSignatureCache{
			svcb: func(signatureData signatureData) (bool, error) {
				return false, nil // Cache miss
			},
			vscb: func(signatureData signatureData, _ validatorAtIndexer) (err error) {
				return nil // Signature valid
			},
		}

		headStateCalled := false
		initializer := Initializer{
			shared: &sharedResources{
				sc: signatureCache,
				sr: &mockStateByRooter{sbr: sbrErrorIfCalled(t)}, // Should not be called
				hsp: &mockHeadStateProvider{
					headRoot:          parentRoot[:],    // Same as parent
					headSlot:          32,               // Epoch 1
					headState:         fuluState.Copy(), // HeadState (not ReadOnly) for ProcessSlots
					headStateReadOnly: nil,              // Should not use ReadOnly path
				},
				fc: &mockForkchoicer{
					// Return same root for both to simulate same chain
					DependentRootForEpochCB: func(root [32]byte, epoch primitives.Epoch) ([32]byte, error) {
						return [32]byte{0xaa}, nil // Same for all inputs
					},
					TargetRootForEpochCB: fcReturnsTargetRoot([fieldparams.RootLength]byte{}),
				},
			},
		}

		// Wrap to detect HeadState call
		originalHsp := initializer.shared.hsp.(*mockHeadStateProvider)
		wrappedHsp := &mockHeadStateProvider{
			headRoot:  originalHsp.headRoot,
			headSlot:  originalHsp.headSlot,
			headState: originalHsp.headState,
		}
		initializer.shared.hsp = &headStateCallTracker{
			mockHeadStateProvider: wrappedHsp,
			headStateCalled:       &headStateCalled,
		}

		verifier := initializer.NewDataColumnsVerifier(columns, GossipDataColumnSidecarRequirements)
		err := verifier.ValidProposerSignature(t.Context())
		require.NoError(t, err)
		require.Equal(t, true, headStateCalled, "HeadState should be called when head is far ahead")
	})
}

// headStateCallTracker wraps mockHeadStateProvider to track HeadState calls.
type headStateCallTracker struct {
	*mockHeadStateProvider
	headStateCalled *bool
}

func (h *headStateCallTracker) HeadState(ctx context.Context) (state.BeaconState, error) {
	*h.headStateCalled = true
	return h.mockHeadStateProvider.HeadState(ctx)
}

func (h *headStateCallTracker) HeadRoot(ctx context.Context) ([]byte, error) {
	return h.mockHeadStateProvider.HeadRoot(ctx)
}

func (h *headStateCallTracker) HeadSlot() primitives.Slot {
	return h.mockHeadStateProvider.HeadSlot()
}

func (h *headStateCallTracker) HeadStateReadOnly(ctx context.Context) (state.ReadOnlyBeaconState, error) {
	return h.mockHeadStateProvider.HeadStateReadOnly(ctx)
}
