package das

import (
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// TestNeedSpanAt tests the needSpan.at() method for range checking.
func TestNeedSpanAt(t *testing.T) {
	cases := []struct {
		name     string
		span     NeedSpan
		slots    []primitives.Slot
		expected bool
	}{
		{
			name:     "within bounds",
			span:     NeedSpan{Begin: 100, End: 200},
			slots:    []primitives.Slot{101, 150, 199},
			expected: true,
		},
		{
			name:     "before begin / at end boundary (exclusive)",
			span:     NeedSpan{Begin: 100, End: 200},
			slots:    []primitives.Slot{99, 200, 201},
			expected: false,
		},
		{
			name:     "empty span (begin == end)",
			span:     NeedSpan{Begin: 100, End: 100},
			slots:    []primitives.Slot{100},
			expected: false,
		},
		{
			name:     "slot 0 with span starting at 0",
			span:     NeedSpan{Begin: 0, End: 100},
			slots:    []primitives.Slot{0},
			expected: true,
		},
	}

	for _, tc := range cases {
		for _, sl := range tc.slots {
			t.Run(fmt.Sprintf("%s at slot %d, ", tc.name, sl), func(t *testing.T) {
				result := tc.span.At(sl)
				require.Equal(t, tc.expected, result)
			})
		}
	}
}

// TestSyncEpochOffset tests the syncEpochOffset helper function.
func TestSyncEpochOffset(t *testing.T) {
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch

	cases := []struct {
		name     string
		current  primitives.Slot
		subtract primitives.Epoch
		expected primitives.Slot
	}{
		{
			name:     "typical offset - 5 epochs back",
			current:  primitives.Slot(10000),
			subtract: 5,
			expected: primitives.Slot(10000 - 5*slotsPerEpoch),
		},
		{
			name:     "zero subtract returns current",
			current:  primitives.Slot(5000),
			subtract: 0,
			expected: primitives.Slot(5000),
		},
		{
			name:     "subtract 1 epoch from mid-range slot",
			current:  primitives.Slot(1000),
			subtract: 1,
			expected: primitives.Slot(1000 - slotsPerEpoch),
		},
		{
			name:     "offset equals current - underflow protection",
			current:  primitives.Slot(slotsPerEpoch),
			subtract: 1,
			expected: 1,
		},
		{
			name:     "offset exceeds current - underflow protection",
			current:  primitives.Slot(50),
			subtract: 1000,
			expected: 1,
		},
		{
			name:     "current very close to 0",
			current:  primitives.Slot(10),
			subtract: 1,
			expected: 1,
		},
		{
			name:     "subtract MaxSafeEpoch",
			current:  primitives.Slot(1000000),
			subtract: slots.MaxSafeEpoch(),
			expected: 1, // underflow protection
		},
		{
			name:     "result exactly at slot 1",
			current:  primitives.Slot(1 + slotsPerEpoch),
			subtract: 1,
			expected: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := syncEpochOffset(tc.current, tc.subtract)
			require.Equal(t, tc.expected, result)
		})
	}
}

// TestSyncNeedsInitialize tests the syncNeeds.initialize() method.
func TestSyncNeedsInitialize(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	minBlobEpochs := params.BeaconConfig().MinEpochsForBlobsSidecarsRequest
	minColEpochs := params.BeaconConfig().MinEpochsForDataColumnSidecarsRequest
	denebSlot := slots.UnsafeEpochStart(params.BeaconConfig().DenebForkEpoch)
	fuluSlot := slots.UnsafeEpochStart(params.BeaconConfig().FuluForkEpoch)
	minSlots := slots.UnsafeEpochStart(primitives.Epoch(params.BeaconConfig().MinEpochsForBlockRequests))

	currentSlot := primitives.Slot(10000)
	currentFunc := func() primitives.Slot { return currentSlot }

	cases := []struct {
		invalidOldestFlag bool
		expectValidOldest bool
		oldestSlotFlagPtr *primitives.Slot
		blobRetentionFlag primitives.Epoch
		expectedBlob      primitives.Epoch
		expectedCol       primitives.Epoch
		name              string
		input             SyncNeeds
		current           func() primitives.Slot
	}{
		{
			name:              "basic initialization with no flags",
			expectValidOldest: false,
			expectedBlob:      minBlobEpochs,
			expectedCol:       minColEpochs,
			blobRetentionFlag: 0,
		},
		{
			name:              "blob retention flag less than spec minimum",
			blobRetentionFlag: minBlobEpochs - 1,
			expectValidOldest: false,
			expectedBlob:      minBlobEpochs,
			expectedCol:       minColEpochs,
		},
		{
			name:              "blob retention flag greater than spec minimum",
			blobRetentionFlag: minBlobEpochs + 10,
			expectValidOldest: false,
			expectedBlob:      minBlobEpochs + 10,
			expectedCol:       minBlobEpochs + 10,
		},
		{
			name:              "oldestSlotFlagPtr is nil",
			blobRetentionFlag: 0,
			oldestSlotFlagPtr: nil,
			expectValidOldest: false,
			expectedBlob:      minBlobEpochs,
			expectedCol:       minColEpochs,
		},
		{
			name:              "valid oldestSlotFlagPtr (earlier than spec minimum)",
			blobRetentionFlag: 0,
			oldestSlotFlagPtr: &denebSlot,
			expectValidOldest: true,
			expectedBlob:      minBlobEpochs,
			expectedCol:       minColEpochs,
			current: func() primitives.Slot {
				return fuluSlot + minSlots
			},
		},
		{
			name:              "invalid oldestSlotFlagPtr (later than spec minimum)",
			blobRetentionFlag: 0,
			oldestSlotFlagPtr: func() *primitives.Slot {
				// Make it way past the spec minimum
				slot := currentSlot - primitives.Slot(params.BeaconConfig().MinEpochsForBlockRequests-1)*slotsPerEpoch
				return &slot
			}(),
			expectValidOldest: false,
			expectedBlob:      minBlobEpochs,
			expectedCol:       minColEpochs,
			invalidOldestFlag: true,
		},
		{
			name:              "oldestSlotFlagPtr at boundary (exactly at spec minimum)",
			blobRetentionFlag: 0,
			oldestSlotFlagPtr: func() *primitives.Slot {
				slot := currentSlot - primitives.Slot(params.BeaconConfig().MinEpochsForBlockRequests)*slotsPerEpoch
				return &slot
			}(),
			expectValidOldest: false,
			expectedBlob:      minBlobEpochs,
			expectedCol:       minColEpochs,
			invalidOldestFlag: true,
		},
		{
			name:              "both blob retention flag and oldest slot set",
			blobRetentionFlag: minBlobEpochs + 5,
			current: func() primitives.Slot {
				return fuluSlot + minSlots
			},
			oldestSlotFlagPtr: func() *primitives.Slot {
				slot := primitives.Slot(100)
				return &slot
			}(),
			expectValidOldest: true,
			expectedBlob:      minBlobEpochs + 5,
			expectedCol:       minBlobEpochs + 5,
		},
		{
			name:              "zero blob retention uses spec minimum",
			blobRetentionFlag: 0,
			expectValidOldest: false,
			expectedBlob:      minBlobEpochs,
			expectedCol:       minColEpochs,
		},
		{
			name:              "large blob retention value",
			blobRetentionFlag: 5000,
			expectValidOldest: false,
			expectedBlob:      5000,
			expectedCol:       5000,
		},
		{
			name:              "regression for deneb start",
			blobRetentionFlag: 8212500,
			expectValidOldest: true,
			oldestSlotFlagPtr: &denebSlot,
			current: func() primitives.Slot {
				return fuluSlot + minSlots
			},
			expectedBlob: 8212500,
			expectedCol:  8212500,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.current == nil {
				tc.current = currentFunc
			}
			result, err := NewSyncNeeds(tc.current, tc.oldestSlotFlagPtr, tc.blobRetentionFlag)
			require.NoError(t, err)

			// Check retention calculations
			require.Equal(t, tc.expectedBlob, result.blobRetention)
			require.Equal(t, tc.expectedCol, result.colRetention)

			if tc.invalidOldestFlag {
				require.IsNil(t, result.validOldestSlotPtr)
			} else {
				require.Equal(t, tc.oldestSlotFlagPtr, result.validOldestSlotPtr)
			}

			// Check blockRetention is always spec minimum
			require.Equal(t, primitives.Epoch(params.BeaconConfig().MinEpochsForBlockRequests), result.blockRetention)
		})
	}
}

// TestSyncNeedsBlockSpan tests the syncNeeds.blockSpan() method.
func TestSyncNeedsBlockSpan(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	minBlockEpochs := params.BeaconConfig().MinEpochsForBlockRequests

	cases := []struct {
		name           string
		validOldest    *primitives.Slot
		blockRetention primitives.Epoch
		current        primitives.Slot
		expectedBegin  primitives.Slot
		expectedEnd    primitives.Slot
	}{
		{
			name:           "with validOldestSlotPtr set",
			validOldest:    func() *primitives.Slot { s := primitives.Slot(500); return &s }(),
			blockRetention: primitives.Epoch(minBlockEpochs),
			current:        10000,
			expectedBegin:  500,
			expectedEnd:    10000,
		},
		{
			name:           "without validOldestSlotPtr (nil)",
			validOldest:    nil,
			blockRetention: primitives.Epoch(minBlockEpochs),
			current:        10000,
			expectedBegin:  syncEpochOffset(10000, primitives.Epoch(minBlockEpochs)),
			expectedEnd:    10000,
		},
		{
			name:           "very low current slot",
			validOldest:    nil,
			blockRetention: primitives.Epoch(minBlockEpochs),
			current:        100,
			expectedBegin:  1, // underflow protection
			expectedEnd:    100,
		},
		{
			name:           "very high current slot",
			validOldest:    nil,
			blockRetention: primitives.Epoch(minBlockEpochs),
			current:        1000000,
			expectedBegin:  syncEpochOffset(1000000, primitives.Epoch(minBlockEpochs)),
			expectedEnd:    1000000,
		},
		{
			name:           "validOldestSlotPtr at boundary value",
			validOldest:    func() *primitives.Slot { s := primitives.Slot(1); return &s }(),
			blockRetention: primitives.Epoch(minBlockEpochs),
			current:        5000,
			expectedBegin:  1,
			expectedEnd:    5000,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sn := SyncNeeds{
				validOldestSlotPtr: tc.validOldest,
				blockRetention:     tc.blockRetention,
			}
			result := sn.blockSpan(tc.current)
			require.Equal(t, tc.expectedBegin, result.Begin)
			require.Equal(t, tc.expectedEnd, result.End)
		})
	}
}

// TestSyncNeedsCurrently tests the syncNeeds.currently() method.
func TestSyncNeedsCurrently(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch

	denebSlot := primitives.Slot(1000)
	fuluSlot := primitives.Slot(2000)

	cases := []struct {
		name           string
		current        primitives.Slot
		blobRetention  primitives.Epoch
		colRetention   primitives.Epoch
		blockRetention primitives.Epoch
		validOldest    *primitives.Slot
		// Expected block span
		expectBlockBegin primitives.Slot
		expectBlockEnd   primitives.Slot
		// Expected blob span
		expectBlobBegin primitives.Slot
		expectBlobEnd   primitives.Slot
		// Expected column span
		expectColBegin primitives.Slot
		expectColEnd   primitives.Slot
	}{
		{
			name:             "pre-Deneb - only blocks needed",
			current:          500,
			blobRetention:    10,
			colRetention:     10,
			blockRetention:   5,
			validOldest:      nil,
			expectBlockBegin: syncEpochOffset(500, 5),
			expectBlockEnd:   500,
			expectBlobBegin:  denebSlot, // adjusted to deneb
			expectBlobEnd:    fuluSlot,
			expectColBegin:   fuluSlot, // adjusted to fulu
			expectColEnd:     500,
		},
		{
			name:             "between Deneb and Fulu - blocks and blobs needed",
			current:          1500,
			blobRetention:    10,
			colRetention:     10,
			blockRetention:   5,
			validOldest:      nil,
			expectBlockBegin: syncEpochOffset(1500, 5),
			expectBlockEnd:   1500,
			expectBlobBegin:  max(syncEpochOffset(1500, 10), denebSlot),
			expectBlobEnd:    fuluSlot,
			expectColBegin:   fuluSlot, // adjusted to fulu
			expectColEnd:     1500,
		},
		{
			name:             "post-Fulu - all resources needed",
			current:          3000,
			blobRetention:    10,
			colRetention:     10,
			blockRetention:   5,
			validOldest:      nil,
			expectBlockBegin: syncEpochOffset(3000, 5),
			expectBlockEnd:   3000,
			expectBlobBegin:  max(syncEpochOffset(3000, 10), denebSlot),
			expectBlobEnd:    fuluSlot,
			expectColBegin:   max(syncEpochOffset(3000, 10), fuluSlot),
			expectColEnd:     3000,
		},
		{
			name:             "exactly at Deneb boundary",
			current:          denebSlot,
			blobRetention:    10,
			colRetention:     10,
			blockRetention:   5,
			validOldest:      nil,
			expectBlockBegin: syncEpochOffset(denebSlot, 5),
			expectBlockEnd:   denebSlot,
			expectBlobBegin:  denebSlot,
			expectBlobEnd:    fuluSlot,
			expectColBegin:   fuluSlot,
			expectColEnd:     denebSlot,
		},
		{
			name:             "exactly at Fulu boundary",
			current:          fuluSlot,
			blobRetention:    10,
			colRetention:     10,
			blockRetention:   5,
			validOldest:      nil,
			expectBlockBegin: syncEpochOffset(fuluSlot, 5),
			expectBlockEnd:   fuluSlot,
			expectBlobBegin:  max(syncEpochOffset(fuluSlot, 10), denebSlot),
			expectBlobEnd:    fuluSlot,
			expectColBegin:   fuluSlot,
			expectColEnd:     fuluSlot,
		},
		{
			name:             "small retention periods",
			current:          5000,
			blobRetention:    1,
			colRetention:     2,
			blockRetention:   1,
			validOldest:      nil,
			expectBlockBegin: syncEpochOffset(5000, 1),
			expectBlockEnd:   5000,
			expectBlobBegin:  max(syncEpochOffset(5000, 1), denebSlot),
			expectBlobEnd:    fuluSlot,
			expectColBegin:   max(syncEpochOffset(5000, 2), fuluSlot),
			expectColEnd:     5000,
		},
		{
			name:             "large retention periods",
			current:          10000,
			blobRetention:    100,
			colRetention:     100,
			blockRetention:   50,
			validOldest:      nil,
			expectBlockBegin: syncEpochOffset(10000, 50),
			expectBlockEnd:   10000,
			expectBlobBegin:  max(syncEpochOffset(10000, 100), denebSlot),
			expectBlobEnd:    fuluSlot,
			expectColBegin:   max(syncEpochOffset(10000, 100), fuluSlot),
			expectColEnd:     10000,
		},
		{
			name:             "with validOldestSlotPtr for blocks",
			current:          8000,
			blobRetention:    10,
			colRetention:     10,
			blockRetention:   5,
			validOldest:      func() *primitives.Slot { s := primitives.Slot(100); return &s }(),
			expectBlockBegin: 100,
			expectBlockEnd:   8000,
			expectBlobBegin:  max(syncEpochOffset(8000, 10), denebSlot),
			expectBlobEnd:    fuluSlot,
			expectColBegin:   max(syncEpochOffset(8000, 10), fuluSlot),
			expectColEnd:     8000,
		},
		{
			name:             "retention approaching current slot",
			current:          primitives.Slot(2000 + 5*slotsPerEpoch),
			blobRetention:    5,
			colRetention:     5,
			blockRetention:   3,
			validOldest:      nil,
			expectBlockBegin: syncEpochOffset(primitives.Slot(2000+5*slotsPerEpoch), 3),
			expectBlockEnd:   primitives.Slot(2000 + 5*slotsPerEpoch),
			expectBlobBegin:  max(syncEpochOffset(primitives.Slot(2000+5*slotsPerEpoch), 5), denebSlot),
			expectBlobEnd:    fuluSlot,
			expectColBegin:   max(syncEpochOffset(primitives.Slot(2000+5*slotsPerEpoch), 5), fuluSlot),
			expectColEnd:     primitives.Slot(2000 + 5*slotsPerEpoch),
		},
		{
			name:             "current just after Deneb",
			current:          denebSlot + 10,
			blobRetention:    10,
			colRetention:     10,
			blockRetention:   5,
			validOldest:      nil,
			expectBlockBegin: syncEpochOffset(denebSlot+10, 5),
			expectBlockEnd:   denebSlot + 10,
			expectBlobBegin:  denebSlot,
			expectBlobEnd:    fuluSlot,
			expectColBegin:   fuluSlot,
			expectColEnd:     denebSlot + 10,
		},
		{
			name:             "current just after Fulu",
			current:          fuluSlot + 10,
			blobRetention:    10,
			colRetention:     10,
			blockRetention:   5,
			validOldest:      nil,
			expectBlockBegin: syncEpochOffset(fuluSlot+10, 5),
			expectBlockEnd:   fuluSlot + 10,
			expectBlobBegin:  max(syncEpochOffset(fuluSlot+10, 10), denebSlot),
			expectBlobEnd:    fuluSlot,
			expectColBegin:   fuluSlot,
			expectColEnd:     fuluSlot + 10,
		},
		{
			name:             "blob retention would start before Deneb",
			current:          denebSlot + primitives.Slot(5*slotsPerEpoch),
			blobRetention:    100, // very large retention
			colRetention:     10,
			blockRetention:   5,
			validOldest:      nil,
			expectBlockBegin: syncEpochOffset(denebSlot+primitives.Slot(5*slotsPerEpoch), 5),
			expectBlockEnd:   denebSlot + primitives.Slot(5*slotsPerEpoch),
			expectBlobBegin:  denebSlot, // clamped to deneb
			expectBlobEnd:    fuluSlot,
			expectColBegin:   fuluSlot,
			expectColEnd:     denebSlot + primitives.Slot(5*slotsPerEpoch),
		},
		{
			name:             "column retention would start before Fulu",
			current:          fuluSlot + primitives.Slot(5*slotsPerEpoch),
			blobRetention:    10,
			colRetention:     100, // very large retention
			blockRetention:   5,
			validOldest:      nil,
			expectBlockBegin: syncEpochOffset(fuluSlot+primitives.Slot(5*slotsPerEpoch), 5),
			expectBlockEnd:   fuluSlot + primitives.Slot(5*slotsPerEpoch),
			expectBlobBegin:  max(syncEpochOffset(fuluSlot+primitives.Slot(5*slotsPerEpoch), 10), denebSlot),
			expectBlobEnd:    fuluSlot,
			expectColBegin:   fuluSlot, // clamped to fulu
			expectColEnd:     fuluSlot + primitives.Slot(5*slotsPerEpoch),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sn := SyncNeeds{
				current:            func() primitives.Slot { return tc.current },
				deneb:              denebSlot,
				fulu:               fuluSlot,
				validOldestSlotPtr: tc.validOldest,
				blockRetention:     tc.blockRetention,
				blobRetention:      tc.blobRetention,
				colRetention:       tc.colRetention,
			}

			result := sn.Currently()

			// Verify block span
			require.Equal(t, tc.expectBlockBegin, result.Block.Begin,
				"block.begin mismatch")
			require.Equal(t, tc.expectBlockEnd, result.Block.End,
				"block.end mismatch")

			// Verify blob span
			require.Equal(t, tc.expectBlobBegin, result.Blob.Begin,
				"blob.begin mismatch")
			require.Equal(t, tc.expectBlobEnd, result.Blob.End,
				"blob.end mismatch")

			// Verify column span
			require.Equal(t, tc.expectColBegin, result.Col.Begin,
				"col.begin mismatch")
			require.Equal(t, tc.expectColEnd, result.Col.End,
				"col.end mismatch")
		})
	}
}

// TestCurrentNeedsIntegration verifies the complete currentNeeds workflow.
func TestCurrentNeedsIntegration(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	denebSlot := primitives.Slot(1000)
	fuluSlot := primitives.Slot(2000)

	cases := []struct {
		name          string
		current       primitives.Slot
		blobRetention primitives.Epoch
		colRetention  primitives.Epoch
		testSlots     []primitives.Slot
		expectBlockAt []bool
		expectBlobAt  []bool
		expectColAt   []bool
	}{
		{
			name:          "pre-Deneb slot - only blocks",
			current:       500,
			blobRetention: 10,
			colRetention:  10,
			testSlots:     []primitives.Slot{100, 250, 499, 500, 1000, 2000},
			expectBlockAt: []bool{true, true, true, false, false, false},
			expectBlobAt:  []bool{false, false, false, false, true, false},
			expectColAt:   []bool{false, false, false, false, false, false},
		},
		{
			name:          "between Deneb and Fulu - blocks and blobs",
			current:       1500,
			blobRetention: 10,
			colRetention:  10,
			testSlots:     []primitives.Slot{500, 1000, 1200, 1499, 1500, 2000},
			expectBlockAt: []bool{true, true, true, true, false, false},
			expectBlobAt:  []bool{false, false, true, true, true, false},
			expectColAt:   []bool{false, false, false, false, false, false},
		},
		{
			name:          "post-Fulu - all resources",
			current:       3000,
			blobRetention: 10,
			colRetention:  10,
			testSlots:     []primitives.Slot{1000, 1500, 2000, 2500, 2999, 3000},
			expectBlockAt: []bool{true, true, true, true, true, false},
			expectBlobAt:  []bool{false, false, false, false, false, false},
			expectColAt:   []bool{false, false, false, false, true, false},
		},
		{
			name:          "at Deneb boundary",
			current:       denebSlot,
			blobRetention: 5,
			colRetention:  5,
			testSlots:     []primitives.Slot{500, 999, 1000, 1500, 2000},
			expectBlockAt: []bool{true, true, false, false, false},
			expectBlobAt:  []bool{false, false, true, true, false},
			expectColAt:   []bool{false, false, false, false, false},
		},
		{
			name:          "at Fulu boundary",
			current:       fuluSlot,
			blobRetention: 5,
			colRetention:  5,
			testSlots:     []primitives.Slot{1000, 1500, 1999, 2000, 2001},
			expectBlockAt: []bool{true, true, true, false, false},
			expectBlobAt:  []bool{false, false, true, false, false},
			expectColAt:   []bool{false, false, false, false, false},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sn := SyncNeeds{
				current:        func() primitives.Slot { return tc.current },
				deneb:          denebSlot,
				fulu:           fuluSlot,
				blockRetention: 100,
				blobRetention:  tc.blobRetention,
				colRetention:   tc.colRetention,
			}

			cn := sn.Currently()

			// Verify block.end == current
			require.Equal(t, tc.current, cn.Block.End, "block.end should equal current")

			// Verify blob.end == fulu
			require.Equal(t, fuluSlot, cn.Blob.End, "blob.end should equal fulu")

			// Verify col.end == current
			require.Equal(t, tc.current, cn.Col.End, "col.end should equal current")

			// Test each slot
			for i, slot := range tc.testSlots {
				require.Equal(t, tc.expectBlockAt[i], cn.Block.At(slot),
					"block.at(%d) mismatch at index %d", slot, i)
				require.Equal(t, tc.expectBlobAt[i], cn.Blob.At(slot),
					"blob.at(%d) mismatch at index %d", slot, i)
				require.Equal(t, tc.expectColAt[i], cn.Col.At(slot),
					"col.at(%d) mismatch at index %d", slot, i)
			}
		})
	}
}
