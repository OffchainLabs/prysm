package backfill

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

type mockChecker struct {
}

var mockAvailabilityFailure = errors.New("fake error from IsDataAvailable")
var mockColumnFailure = errors.Wrap(mockAvailabilityFailure, "column checker failure")
var mockBlobFailure = errors.Wrap(mockAvailabilityFailure, "blob checker failure")

func TestNewCheckMultiplexer(t *testing.T) {
	denebSlot, fuluSlot := testDenebAndFuluSlots(t)

	cases := []struct {
		name         string
		batch        func() batch
		setupChecker func(*checkMultiplexer)
		current      primitives.Slot
		err          error
	}{
		{
			name:  "no availability checkers, no blocks",
			batch: func() batch { return batch{} },
		},
		{
			name: "no blob availability checkers, deneb blocks",
			batch: func() batch {
				blks, _ := testBlobGen(t, denebSlot, 2)
				return batch{
					blocks: blks,
				}
			},
			setupChecker: func(m *checkMultiplexer) {
				// Provide a column checker which should be unused in this test.
				m.colCheck = &das.MockAvailabilityStore{}
			},
			err: errMissingAvailabilityChecker,
		},
		{
			name: "no column availability checker, fulu blocks",
			batch: func() batch {
				blks, _ := testBlobGen(t, fuluSlot, 2)
				return batch{
					blocks: blks,
				}
			},
			err: errMissingAvailabilityChecker,
			setupChecker: func(m *checkMultiplexer) {
				// Provide a blob checker which should be unused in this test.
				m.blobCheck = &das.MockAvailabilityStore{}
			},
		},
		{
			name: "has column availability checker, fulu blocks",
			batch: func() batch {
				blks, _ := testBlobGen(t, fuluSlot, 2)
				return batch{
					blocks: blks,
				}
			},
			setupChecker: func(m *checkMultiplexer) {
				// Provide a blob checker which should be unused in this test.
				m.colCheck = &das.MockAvailabilityStore{}
			},
		},
		{
			name: "has blob availability checker, deneb blocks",
			batch: func() batch {
				blks, _ := testBlobGen(t, denebSlot, 2)
				return batch{
					blocks: blks,
				}
			},
			setupChecker: func(m *checkMultiplexer) {
				// Provide a blob checker which should be unused in this test.
				m.blobCheck = &das.MockAvailabilityStore{}
			},
		},
		{
			name: "has blob but not col availability checker, deneb and fulu blocks",
			batch: func() batch {
				blks, _ := testBlobGen(t, fuluSlot-2, 4) // spans deneb and fulu
				return batch{
					blocks: blks,
				}
			},
			err: errMissingAvailabilityChecker, // fails because column store is not present
			setupChecker: func(m *checkMultiplexer) {
				m.blobCheck = &das.MockAvailabilityStore{}
			},
		},
		{
			name: "has col but not blob availability checker, deneb and fulu blocks",
			batch: func() batch {
				blks, _ := testBlobGen(t, fuluSlot-2, 4) // spans deneb and fulu
				return batch{
					blocks: blks,
				}
			},
			err: errMissingAvailabilityChecker, // fails because column store is not present
			setupChecker: func(m *checkMultiplexer) {
				m.colCheck = &das.MockAvailabilityStore{}
			},
		},
		{
			name: "both checkers, deneb and fulu blocks",
			batch: func() batch {
				blks, _ := testBlobGen(t, fuluSlot-2, 4) // spans deneb and fulu
				return batch{
					blocks: blks,
				}
			},
			setupChecker: func(m *checkMultiplexer) {
				m.blobCheck = &das.MockAvailabilityStore{}
				m.colCheck = &das.MockAvailabilityStore{}
			},
		},
		{
			name: "deneb checker fails, deneb and fulu blocks",
			batch: func() batch {
				blks, _ := testBlobGen(t, fuluSlot-2, 4) // spans deneb and fulu
				return batch{
					blocks: blks,
				}
			},
			err: mockBlobFailure,
			setupChecker: func(m *checkMultiplexer) {
				m.blobCheck = &das.MockAvailabilityStore{ErrIsDataAvailable: mockBlobFailure}
				m.colCheck = &das.MockAvailabilityStore{}
			},
		},
		{
			name: "fulu checker fails, deneb and fulu blocks",
			batch: func() batch {
				blks, _ := testBlobGen(t, fuluSlot-2, 4) // spans deneb and fulu
				return batch{
					blocks: blks,
				}
			},
			err: mockBlobFailure,
			setupChecker: func(m *checkMultiplexer) {
				m.blobCheck = &das.MockAvailabilityStore{}
				m.colCheck = &das.MockAvailabilityStore{ErrIsDataAvailable: mockBlobFailure}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := tc.batch()
			var checker *checkMultiplexer
			checker = newCheckMultiplexer(fuluSlot, denebSlot, b)
			if tc.setupChecker != nil {
				tc.setupChecker(checker)
			}
			err := checker.IsDataAvailable(t.Context(), tc.current, b.blocks...)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func testBlocksWithCommitments(t *testing.T, startSlot primitives.Slot, count int) []blocks.ROBlock {
	blks := make([]blocks.ROBlock, count)
	for i := 0; i < count; i++ {
		blk, _ := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, startSlot+primitives.Slot(i), 1)
		blks[i] = blk
	}
	return blks
}

func TestDaNeeds(t *testing.T) {
	denebSlot, fuluSlot := testDenebAndFuluSlots(t)
	mux := &checkMultiplexer{
		denebStart: denebSlot,
		fuluStart:  fuluSlot,
	}

	cases := []struct {
		name   string
		setup  func() (daNeeds, []blocks.ROBlock)
		expect daNeeds
		err    error
	}{
		{
			name: "empty range",
			setup: func() (daNeeds, []blocks.ROBlock) {
				return daNeeds{}, testBlocksWithCommitments(t, 10, 5)
			},
		},
		{
			name: "single deneb block",
			setup: func() (daNeeds, []blocks.ROBlock) {
				blks := testBlocksWithCommitments(t, denebSlot, 1)
				return daNeeds{
					blobs: []blocks.ROBlock{blks[0]},
				}, blks
			},
		},
		{
			name: "single fulu block",
			setup: func() (daNeeds, []blocks.ROBlock) {
				blks := testBlocksWithCommitments(t, fuluSlot, 1)
				return daNeeds{
					cols: []blocks.ROBlock{blks[0]},
				}, blks
			},
		},
		{
			name: "deneb range",
			setup: func() (daNeeds, []blocks.ROBlock) {
				blks := testBlocksWithCommitments(t, denebSlot, 3)
				return daNeeds{
					blobs: blks,
				}, blks
			},
		},
		{
			name: "one deneb one fulu",
			setup: func() (daNeeds, []blocks.ROBlock) {
				deneb := testBlocksWithCommitments(t, denebSlot, 1)
				fulu := testBlocksWithCommitments(t, fuluSlot, 1)
				return daNeeds{
					blobs: []blocks.ROBlock{deneb[0]},
					cols:  []blocks.ROBlock{fulu[0]},
				}, append(deneb, fulu...)
			},
		},
		{
			name: "deneb and fulu range",
			setup: func() (daNeeds, []blocks.ROBlock) {
				deneb := testBlocksWithCommitments(t, denebSlot, 3)
				fulu := testBlocksWithCommitments(t, fuluSlot, 3)
				return daNeeds{
					blobs: deneb,
					cols:  fulu,
				}, append(deneb, fulu...)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expectNeeds, blks := tc.setup()
			needs, err := mux.blockDaNeeds(blks)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
			} else {
				require.NoError(t, err)
			}
			expectBlob := make(map[[32]byte]struct{})
			for _, blk := range expectNeeds.blobs {
				expectBlob[blk.Root()] = struct{}{}
			}
			for _, blk := range needs.blobs {
				_, ok := expectBlob[blk.Root()]
				require.Equal(t, true, ok, "unexpected blob block root %#x", blk.Root())
				delete(expectBlob, blk.Root())
			}
			require.Equal(t, 0, len(expectBlob), "missing blob blocks")

			expectCol := make(map[[32]byte]struct{})
			for _, blk := range expectNeeds.cols {
				expectCol[blk.Root()] = struct{}{}
			}
			for _, blk := range needs.cols {
				_, ok := expectCol[blk.Root()]
				require.Equal(t, true, ok, "unexpected col block root %#x", blk.Root())
				delete(expectCol, blk.Root())
			}
			require.Equal(t, 0, len(expectCol), "missing col blocks")
		})
	}
}

func TestSafeRange(t *testing.T) {
	cases := []struct {
		name     string
		sr       safeRange
		err      error
		slice    []int
		expected []int
	}{
		{
			name:  "zero range",
			sr:    safeRange{},
			slice: []int{0, 1, 2},
		},
		{
			name:     "valid range",
			sr:       safeRange{start: 1, end: 3},
			expected: []int{1, 2},
			slice:    []int{0, 1, 2},
		},
		{
			name:  "start greater than end",
			sr:    safeRange{start: 3, end: 2},
			err:   errUnsafeRange,
			slice: []int{0, 1, 2},
		},
		{
			name:  "end out of bounds",
			sr:    safeRange{start: 1, end: 5},
			err:   errUnsafeRange,
			slice: []int{0, 1, 2},
		},
		{
			name:  "start out of bounds",
			sr:    safeRange{start: 5, end: 6},
			err:   errUnsafeRange,
			slice: []int{0, 1, 2},
		},
		{
			name:  "no error for empty slice",
			sr:    safeRange{start: 6, end: 5},
			slice: []int{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sub, err := subSlice(tc.slice, tc.sr)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
				return
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, len(tc.expected), len(sub))
			for i := range tc.expected {
				require.Equal(t, tc.expected[i], sub[i])
			}
		})
	}
}

func testDenebAndFuluSlots(t *testing.T) (primitives.Slot, primitives.Slot) {
	params.SetupTestConfigCleanup(t)
	denebEpoch := params.BeaconConfig().DenebForkEpoch
	if params.BeaconConfig().FuluForkEpoch == params.BeaconConfig().FarFutureEpoch {
		params.BeaconConfig().FuluForkEpoch = denebEpoch + 4096*2
	}
	fuluEpoch := params.BeaconConfig().FuluForkEpoch
	fuluSlot, err := slots.EpochStart(fuluEpoch)
	require.NoError(t, err)
	denebSlot, err := slots.EpochStart(denebEpoch)
	require.NoError(t, err)
	return denebSlot, fuluSlot
}
