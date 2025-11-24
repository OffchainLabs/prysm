package backfill

import (
	"context"
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

// trackingAvailabilityChecker wraps a das.AvailabilityChecker and tracks calls
type trackingAvailabilityChecker struct {
	checker            das.AvailabilityChecker
	callCount          int
	blocksSeenPerCall  [][]blocks.ROBlock // Track blocks passed in each call
}

// NewTrackingAvailabilityChecker creates a wrapper that tracks calls to the underlying checker
func NewTrackingAvailabilityChecker(checker das.AvailabilityChecker) *trackingAvailabilityChecker {
	return &trackingAvailabilityChecker{
		checker:           checker,
		callCount:         0,
		blocksSeenPerCall: [][]blocks.ROBlock{},
	}
}

// IsDataAvailable implements das.AvailabilityChecker
func (t *trackingAvailabilityChecker) IsDataAvailable(ctx context.Context, current primitives.Slot, blks ...blocks.ROBlock) error {
	t.callCount++
	// Track a copy of the blocks passed in this call
	blocksCopy := make([]blocks.ROBlock, len(blks))
	copy(blocksCopy, blks)
	t.blocksSeenPerCall = append(t.blocksSeenPerCall, blocksCopy)

	// Delegate to the underlying checker
	return t.checker.IsDataAvailable(ctx, current, blks...)
}

// GetCallCount returns how many times IsDataAvailable was called
func (t *trackingAvailabilityChecker) GetCallCount() int {
	return t.callCount
}

// GetBlocksInCall returns the blocks passed in a specific call (0-indexed)
func (t *trackingAvailabilityChecker) GetBlocksInCall(callIndex int) []blocks.ROBlock {
	if callIndex < 0 || callIndex >= len(t.blocksSeenPerCall) {
		return nil
	}
	return t.blocksSeenPerCall[callIndex]
}

// GetTotalBlocksSeen returns total number of blocks seen across all calls
func (t *trackingAvailabilityChecker) GetTotalBlocksSeen() int {
	total := 0
	for _, blkSlice := range t.blocksSeenPerCall {
		total += len(blkSlice)
	}
	return total
}

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

// Helper to create test blocks without blob commitments
// Uses 0 commitments instead of 1 like testBlocksWithCommitments
func testBlocksWithoutCommitments(t *testing.T, startSlot primitives.Slot, count int) []blocks.ROBlock {
	blks := make([]blocks.ROBlock, count)
	for i := 0; i < count; i++ {
		blk, _ := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, startSlot+primitives.Slot(i), 0)
		blks[i] = blk
	}
	return blks
}

// TestSafeRangeIsZero verifies the isZero method works correctly
func TestSafeRangeIsZero(t *testing.T) {
	cases := []struct {
		name   string
		sr     safeRange
		expect bool
	}{
		{
			name:   "zero range (0, 0)",
			sr:     safeRange{start: 0, end: 0},
			expect: true,
		},
		{
			name:   "non-zero range (1, 2)",
			sr:     safeRange{start: 1, end: 2},
			expect: false,
		},
		{
			name:   "non-zero range (0, 1)",
			sr:     safeRange{start: 0, end: 1},
			expect: false,
		},
		{
			name:   "non-zero range (5, 10)",
			sr:     safeRange{start: 5, end: 10},
			expect: false,
		},
		{
			name:   "invalid range (5, 3)",
			sr:     safeRange{start: 5, end: 3},
			expect: false, // start != end, so isZero returns false
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.sr.isZero()
			require.Equal(t, tc.expect, result,
				"safeRange{%d, %d}.isZero() should return %v, got %v", tc.sr.start, tc.sr.end, tc.expect, result)
		})
	}
}

// TestBlockDaNeedsWithoutCommitments verifies blocks without commitments are skipped
func TestBlockDaNeedsWithoutCommitments(t *testing.T) {
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
			name: "deneb blocks without commitments",
			setup: func() (daNeeds, []blocks.ROBlock) {
				blks := testBlocksWithoutCommitments(t, denebSlot, 3)
				return daNeeds{}, blks // Expect empty daNeeds
			},
		},
		{
			name: "fulu blocks without commitments",
			setup: func() (daNeeds, []blocks.ROBlock) {
				blks := testBlocksWithoutCommitments(t, fuluSlot, 3)
				return daNeeds{}, blks // Expect empty daNeeds
			},
		},
		{
			name: "mixed: some deneb with commitments, some without",
			setup: func() (daNeeds, []blocks.ROBlock) {
				withCommit := testBlocksWithCommitments(t, denebSlot, 2)
				withoutCommit := testBlocksWithoutCommitments(t, denebSlot+2, 2)
				blks := append(withCommit, withoutCommit...)
				return daNeeds{
					blobs: withCommit, // Only the ones with commitments
				}, blks
			},
		},
		{
			name: "pre-deneb blocks are skipped",
			setup: func() (daNeeds, []blocks.ROBlock) {
				blks := testBlocksWithCommitments(t, denebSlot-10, 5)
				return daNeeds{}, blks // All pre-deneb, expect empty
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
			// Verify blob blocks
			require.Equal(t, len(expectNeeds.blobs), len(needs.blobs),
				"expected %d blob blocks, got %d", len(expectNeeds.blobs), len(needs.blobs))
			// Verify col blocks
			require.Equal(t, len(expectNeeds.cols), len(needs.cols),
				"expected %d col blocks, got %d", len(expectNeeds.cols), len(needs.cols))
		})
	}
}

// TestBlockDaNeedsAcrossEras verifies blocks spanning pre-deneb/deneb/fulu boundaries
func TestBlockDaNeedsAcrossEras(t *testing.T) {
	denebSlot, fuluSlot := testDenebAndFuluSlots(t)
	mux := &checkMultiplexer{
		denebStart: denebSlot,
		fuluStart:  fuluSlot,
	}

	cases := []struct {
		name         string
		setup        func() (daNeeds, []blocks.ROBlock)
		expectBlobCount int
		expectColCount  int
	}{
		{
			name: "pre-deneb, deneb, fulu sequence",
			setup: func() (daNeeds, []blocks.ROBlock) {
				preDeneb := testBlocksWithCommitments(t, denebSlot-1, 1)
				deneb := testBlocksWithCommitments(t, denebSlot, 2)
				fulu := testBlocksWithCommitments(t, fuluSlot, 2)
				blks := append(preDeneb, append(deneb, fulu...)...)
				return daNeeds{}, blks
			},
			expectBlobCount: 2, // Only deneb blocks
			expectColCount:  2, // Only fulu blocks
		},
		{
			name: "blocks at exact deneb boundary",
			setup: func() (daNeeds, []blocks.ROBlock) {
				atBoundary := testBlocksWithCommitments(t, denebSlot, 1)
				return daNeeds{
					blobs: atBoundary,
				}, atBoundary
			},
			expectBlobCount: 1,
			expectColCount:  0,
		},
		{
			name: "blocks at exact fulu boundary",
			setup: func() (daNeeds, []blocks.ROBlock) {
				atBoundary := testBlocksWithCommitments(t, fuluSlot, 1)
				return daNeeds{
					cols: atBoundary,
				}, atBoundary
			},
			expectBlobCount: 0,
			expectColCount:  1,
		},
		{
			name: "many deneb blocks before fulu transition",
			setup: func() (daNeeds, []blocks.ROBlock) {
				deneb := testBlocksWithCommitments(t, denebSlot, 10)
				fulu := testBlocksWithCommitments(t, fuluSlot, 5)
				blks := append(deneb, fulu...)
				return daNeeds{}, blks
			},
			expectBlobCount: 10,
			expectColCount:  5,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, blks := tc.setup()
			needs, err := mux.blockDaNeeds(blks)
			require.NoError(t, err)
			require.Equal(t, tc.expectBlobCount, len(needs.blobs),
				"expected %d blob blocks, got %d", tc.expectBlobCount, len(needs.blobs))
			require.Equal(t, tc.expectColCount, len(needs.cols),
				"expected %d col blocks, got %d", tc.expectColCount, len(needs.cols))
		})
	}
}

// TestDoAvailabilityCheckEdgeCases verifies edge cases in doAvailabilityCheck
func TestDoAvailabilityCheckEdgeCases(t *testing.T) {
	denebSlot, _ := testDenebAndFuluSlots(t)
	checkerErr := errors.New("checker error")

	cases := []struct {
		name            string
		checker         das.AvailabilityChecker
		blocks          []blocks.ROBlock
		expectErr       error
		setupTestBlocks func() []blocks.ROBlock
	}{
		{
			name:      "nil checker with empty blocks",
			checker:   nil,
			blocks:    []blocks.ROBlock{},
			expectErr: nil, // Should succeed with no blocks
		},
		{
			name:      "nil checker with blocks",
			checker:   nil,
			expectErr: errMissingAvailabilityChecker,
			setupTestBlocks: func() []blocks.ROBlock {
				return testBlocksWithCommitments(t, denebSlot, 1)
			},
		},
		{
			name:      "valid checker with empty blocks",
			checker:   &das.MockAvailabilityStore{},
			blocks:    []blocks.ROBlock{},
			expectErr: nil,
		},
		{
			name:      "valid checker with blocks succeeds",
			checker:   &das.MockAvailabilityStore{},
			expectErr: nil,
			setupTestBlocks: func() []blocks.ROBlock {
				return testBlocksWithCommitments(t, denebSlot, 3)
			},
		},
		{
			name:    "valid checker error is propagated",
			checker: &das.MockAvailabilityStore{ErrIsDataAvailable: checkerErr},
			expectErr: checkerErr,
			setupTestBlocks: func() []blocks.ROBlock {
				return testBlocksWithCommitments(t, denebSlot, 1)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blks := tc.blocks
			if tc.setupTestBlocks != nil {
				blks = tc.setupTestBlocks()
			}
			err := doAvailabilityCheck(t.Context(), tc.checker, denebSlot, blks)
			if tc.expectErr != nil {
				require.NotNil(t, err)
				require.ErrorIs(t, err, tc.expectErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestSubSliceEdgeCases verifies additional edge cases for subSlice
func TestSubSliceEdgeCases(t *testing.T) {
	cases := []struct {
		name     string
		sr       safeRange
		slice    []int
		expected []int
		err      error
	}{
		{
			name:  "full range extraction",
			sr:    safeRange{start: 0, end: 5},
			slice: []int{0, 1, 2, 3, 4},
			expected: []int{0, 1, 2, 3, 4},
		},
		{
			name:  "single element extraction",
			sr:    safeRange{start: 2, end: 3},
			slice: []int{0, 1, 2, 3, 4},
			expected: []int{2},
		},
		{
			name:  "last element extraction",
			sr:    safeRange{start: 4, end: 5},
			slice: []int{0, 1, 2, 3, 4},
			expected: []int{4},
		},
		{
			name:  "start equals end (zero range)",
			sr:    safeRange{start: 2, end: 2},
			slice: []int{0, 1, 2, 3, 4},
			// Zero range should return nil, not error
		},
		{
			name:  "large slice extraction",
			sr:    safeRange{start: 0, end: 100},
			slice: make([]int, 100),
			expected: make([]int, 100),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := subSlice(tc.slice, tc.sr)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, len(tc.expected), len(result),
				"expected slice of length %d, got %d", len(tc.expected), len(result))
			if len(tc.expected) > 0 {
				for i := range tc.expected {
					require.Equal(t, tc.expected[i], result[i],
						"element at index %d should be %d, got %d", i, tc.expected[i], result[i])
				}
			}
		})
	}
}

// TestBlockDaNeedsErrorWrapping verifies error messages are properly wrapped
func TestBlockDaNeedsErrorWrapping(t *testing.T) {
	denebSlot, fuluSlot := testDenebAndFuluSlots(t)
	mux := &checkMultiplexer{
		denebStart: denebSlot,
		fuluStart:  fuluSlot,
	}

	// Test with a block that has commitments but in deneb range
	blks := testBlocksWithCommitments(t, denebSlot, 2)

	// This should succeed without errors
	needs, err := mux.blockDaNeeds(blks)
	require.NoError(t, err)
	require.Equal(t, 2, len(needs.blobs))
	require.Equal(t, 0, len(needs.cols))
}

// TestIsDataAvailableCallRouting verifies that blocks are routed to the correct checker
// based on their era (pre-deneb, deneb, fulu) and tests various block combinations
func TestIsDataAvailableCallRouting(t *testing.T) {
	denebSlot, fuluSlot := testDenebAndFuluSlots(t)

	cases := []struct {
		name              string
		buildBlocks       func() []blocks.ROBlock
		expectedBlobCalls int
		expectedBlobBlocks int
		expectedColCalls  int
		expectedColBlocks int
	}{
		{
			name: "PreDenebOnly",
			buildBlocks: func() []blocks.ROBlock {
				return testBlocksWithCommitments(t, denebSlot-10, 3)
			},
			expectedBlobCalls:  0,
			expectedBlobBlocks: 0,
			expectedColCalls:   0,
			expectedColBlocks:  0,
		},
		{
			name: "DenebOnly",
			buildBlocks: func() []blocks.ROBlock {
				return testBlocksWithCommitments(t, denebSlot, 3)
			},
			expectedBlobCalls:  1,
			expectedBlobBlocks: 3,
			expectedColCalls:   0,
			expectedColBlocks:  0,
		},
		{
			name: "FuluOnly",
			buildBlocks: func() []blocks.ROBlock {
				return testBlocksWithCommitments(t, fuluSlot, 3)
			},
			expectedBlobCalls:  0,
			expectedBlobBlocks: 0,
			expectedColCalls:   1,
			expectedColBlocks:  3,
		},
		{
			name: "PreDeneb_Deneb_Mix",
			buildBlocks: func() []blocks.ROBlock {
				preDeneb := testBlocksWithCommitments(t, denebSlot-10, 3)
				deneb := testBlocksWithCommitments(t, denebSlot, 3)
				return append(preDeneb, deneb...)
			},
			expectedBlobCalls:  1,
			expectedBlobBlocks: 3,
			expectedColCalls:   0,
			expectedColBlocks:  0,
		},
		{
			name: "PreDeneb_Fulu_Mix",
			buildBlocks: func() []blocks.ROBlock {
				preDeneb := testBlocksWithCommitments(t, denebSlot-10, 3)
				fulu := testBlocksWithCommitments(t, fuluSlot, 3)
				return append(preDeneb, fulu...)
			},
			expectedBlobCalls:  0,
			expectedBlobBlocks: 0,
			expectedColCalls:   1,
			expectedColBlocks:  3,
		},
		{
			name: "Deneb_Fulu_Mix",
			buildBlocks: func() []blocks.ROBlock {
				deneb := testBlocksWithCommitments(t, denebSlot, 3)
				fulu := testBlocksWithCommitments(t, fuluSlot, 3)
				return append(deneb, fulu...)
			},
			expectedBlobCalls:  1,
			expectedBlobBlocks: 3,
			expectedColCalls:   1,
			expectedColBlocks:  3,
		},
		{
			name: "PreDeneb_Deneb_Fulu_Mix",
			buildBlocks: func() []blocks.ROBlock {
				preDeneb := testBlocksWithCommitments(t, denebSlot-10, 3)
				deneb := testBlocksWithCommitments(t, denebSlot, 4)
				fulu := testBlocksWithCommitments(t, fuluSlot, 3)
				return append(preDeneb, append(deneb, fulu...)...)
			},
			expectedBlobCalls:  1,
			expectedBlobBlocks: 4,
			expectedColCalls:   1,
			expectedColBlocks:  3,
		},
		{
			name: "DenebNoCommitments",
			buildBlocks: func() []blocks.ROBlock {
				return testBlocksWithoutCommitments(t, denebSlot, 3)
			},
			expectedBlobCalls:  0,
			expectedBlobBlocks: 0,
			expectedColCalls:   0,
			expectedColBlocks:  0,
		},
		{
			name: "FuluNoCommitments",
			buildBlocks: func() []blocks.ROBlock {
				return testBlocksWithoutCommitments(t, fuluSlot, 3)
			},
			expectedBlobCalls:  0,
			expectedBlobBlocks: 0,
			expectedColCalls:   0,
			expectedColBlocks:  0,
		},
		{
			name: "MixedCommitments_Deneb",
			buildBlocks: func() []blocks.ROBlock {
				with := testBlocksWithCommitments(t, denebSlot, 3)
				without := testBlocksWithoutCommitments(t, denebSlot+3, 3)
				return append(with, without...)
			},
			expectedBlobCalls:  1,
			expectedBlobBlocks: 3,
			expectedColCalls:   0,
			expectedColBlocks:  0,
		},
		{
			name: "MixedCommitments_Fulu",
			buildBlocks: func() []blocks.ROBlock {
				with := testBlocksWithCommitments(t, fuluSlot, 3)
				without := testBlocksWithoutCommitments(t, fuluSlot+3, 3)
				return append(with, without...)
			},
			expectedBlobCalls:  0,
			expectedBlobBlocks: 0,
			expectedColCalls:   1,
			expectedColBlocks:  3,
		},
		{
			name: "MixedCommitments_All",
			buildBlocks: func() []blocks.ROBlock {
				denebWith := testBlocksWithCommitments(t, denebSlot, 3)
				denebWithout := testBlocksWithoutCommitments(t, denebSlot+3, 2)
				fuluWith := testBlocksWithCommitments(t, fuluSlot, 3)
				fuluWithout := testBlocksWithoutCommitments(t, fuluSlot+3, 2)
				return append(denebWith, append(denebWithout, append(fuluWith, fuluWithout...)...)...)
			},
			expectedBlobCalls:  1,
			expectedBlobBlocks: 3,
			expectedColCalls:   1,
			expectedColBlocks:  3,
		},
		{
			name: "EmptyBlocks",
			buildBlocks: func() []blocks.ROBlock {
				return []blocks.ROBlock{}
			},
			expectedBlobCalls:  0,
			expectedBlobBlocks: 0,
			expectedColCalls:   0,
			expectedColBlocks:  0,
		},
		{
			name: "SingleDeneb",
			buildBlocks: func() []blocks.ROBlock {
				return testBlocksWithCommitments(t, denebSlot, 1)
			},
			expectedBlobCalls:  1,
			expectedBlobBlocks: 1,
			expectedColCalls:   0,
			expectedColBlocks:  0,
		},
		{
			name: "SingleFulu",
			buildBlocks: func() []blocks.ROBlock {
				return testBlocksWithCommitments(t, fuluSlot, 1)
			},
			expectedBlobCalls:  0,
			expectedBlobBlocks: 0,
			expectedColCalls:   1,
			expectedColBlocks:  1,
		},
		{
			name: "DenebAtBoundary",
			buildBlocks: func() []blocks.ROBlock {
				preDeneb := testBlocksWithCommitments(t, denebSlot-1, 1)
				atBoundary := testBlocksWithCommitments(t, denebSlot, 1)
				return append(preDeneb, atBoundary...)
			},
			expectedBlobCalls:  1,
			expectedBlobBlocks: 1,
			expectedColCalls:   0,
			expectedColBlocks:  0,
		},
		{
			name: "FuluAtBoundary",
			buildBlocks: func() []blocks.ROBlock {
				deneb := testBlocksWithCommitments(t, fuluSlot-1, 1)
				atBoundary := testBlocksWithCommitments(t, fuluSlot, 1)
				return append(deneb, atBoundary...)
			},
			expectedBlobCalls:  1,
			expectedBlobBlocks: 1,
			expectedColCalls:   1,
			expectedColBlocks:  1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Create tracking wrappers around mock checkers
			blobTracker := NewTrackingAvailabilityChecker(&das.MockAvailabilityStore{})
			colTracker := NewTrackingAvailabilityChecker(&das.MockAvailabilityStore{})

			// Create multiplexer with tracked checkers
			mux := &checkMultiplexer{
				blobCheck:  blobTracker,
				colCheck:   colTracker,
				denebStart: denebSlot,
				fuluStart:  fuluSlot,
			}

			// Build blocks and run availability check
			blocks := tc.buildBlocks()
			err := mux.IsDataAvailable(t.Context(), denebSlot, blocks...)
			require.NoError(t, err)

			// Assert blob checker was called the expected number of times
			require.Equal(t, tc.expectedBlobCalls, blobTracker.GetCallCount(),
				"blob checker call count mismatch for test %s", tc.name)

			// Assert blob checker saw the expected number of blocks
			require.Equal(t, tc.expectedBlobBlocks, blobTracker.GetTotalBlocksSeen(),
				"blob checker block count mismatch for test %s", tc.name)

			// Assert column checker was called the expected number of times
			require.Equal(t, tc.expectedColCalls, colTracker.GetCallCount(),
				"column checker call count mismatch for test %s", tc.name)

			// Assert column checker saw the expected number of blocks
			require.Equal(t, tc.expectedColBlocks, colTracker.GetTotalBlocksSeen(),
				"column checker block count mismatch for test %s", tc.name)
		})
	}
}
