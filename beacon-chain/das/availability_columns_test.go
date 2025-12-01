package das

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/pkg/errors"
)

func mockShouldRetain(current primitives.Epoch) RetentionChecker {
	return func(slot primitives.Slot) bool {
		return params.WithinDAPeriod(slots.ToEpoch(slot), current)
	}
}

var commitments = [][]byte{
	bytesutil.PadTo([]byte("a"), 48),
	bytesutil.PadTo([]byte("b"), 48),
	bytesutil.PadTo([]byte("c"), 48),
	bytesutil.PadTo([]byte("d"), 48),
}

func TestPersist(t *testing.T) {
	t.Run("no sidecars", func(t *testing.T) {
		dataColumnStorage := filesystem.NewEphemeralDataColumnStorage(t)
		lazilyPersistentStoreColumns := NewLazilyPersistentStoreColumn(dataColumnStorage, nil, enode.ID{}, 0, nil, mockShouldRetain(0))
		err := lazilyPersistentStoreColumns.Persist(0)
		require.NoError(t, err)
		require.Equal(t, 0, len(lazilyPersistentStoreColumns.cache.entries))
	})

	t.Run("outside DA period", func(t *testing.T) {
		dataColumnStorage := filesystem.NewEphemeralDataColumnStorage(t)

		dataColumnParamsByBlockRoot := []util.DataColumnParam{
			{Slot: 1, Index: 1},
		}

		var current primitives.Slot = 1_000_000
		sr := mockShouldRetain(slots.ToEpoch(current))
		roSidecars, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, dataColumnParamsByBlockRoot)
		lazilyPersistentStoreColumns := NewLazilyPersistentStoreColumn(dataColumnStorage, nil, enode.ID{}, 0, nil, sr)

		err := lazilyPersistentStoreColumns.Persist(current, roSidecars...)
		require.NoError(t, err)
		require.Equal(t, len(roSidecars), len(lazilyPersistentStoreColumns.cache.entries))
	})

	t.Run("nominal", func(t *testing.T) {
		const slot = 42
		store := filesystem.NewEphemeralDataColumnStorage(t)

		dataColumnParamsByBlockRoot := []util.DataColumnParam{
			{Slot: slot, Index: 1},
			{Slot: slot, Index: 5},
		}

		roSidecars, roDataColumns := util.CreateTestVerifiedRoDataColumnSidecars(t, dataColumnParamsByBlockRoot)
		avs := NewLazilyPersistentStoreColumn(store, nil, enode.ID{}, 0, nil, mockShouldRetain(slots.ToEpoch(slot)))

		err := avs.Persist(slot, roSidecars...)
		require.NoError(t, err)
		require.Equal(t, 1, len(avs.cache.entries))

		key := cacheKey{slot: slot, root: roDataColumns[0].BlockRoot()}
		entry, ok := avs.cache.entries[key]
		require.Equal(t, true, ok)
		summary := store.Summary(key.root)
		// A call to Persist does NOT save the sidecars to disk.
		require.Equal(t, uint64(0), summary.Count())
		require.Equal(t, len(roSidecars), len(entry.scs))

		idx1 := entry.scs[1]
		require.NotNil(t, idx1)
		require.DeepSSZEqual(t, roDataColumns[0].BlockRoot(), idx1.BlockRoot())
		idx5 := entry.scs[5]
		require.NotNil(t, idx5)
		require.DeepSSZEqual(t, roDataColumns[1].BlockRoot(), idx5.BlockRoot())

		for i, roDataColumn := range entry.scs {
			if map[uint64]bool{1: true, 5: true}[i] {
				continue
			}

			require.IsNil(t, roDataColumn)
		}
	})
}

func TestIsDataAvailable(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().FuluForkEpoch = params.BeaconConfig().ElectraForkEpoch + 4096*2
	newDataColumnsVerifier := func(dataColumnSidecars []blocks.RODataColumn, _ []verification.Requirement) verification.DataColumnsVerifier {
		return &mockDataColumnsVerifier{t: t, dataColumnSidecars: dataColumnSidecars}
	}

	ctx := t.Context()

	t.Run("without commitments", func(t *testing.T) {
		signedBeaconBlockFulu := util.NewBeaconBlockFulu()
		signedRoBlock := newSignedRoBlock(t, signedBeaconBlockFulu)

		dataColumnStorage := filesystem.NewEphemeralDataColumnStorage(t)
		lazilyPersistentStoreColumns := NewLazilyPersistentStoreColumn(dataColumnStorage, newDataColumnsVerifier, enode.ID{}, 0, nil, mockShouldRetain(0))

		err := lazilyPersistentStoreColumns.IsDataAvailable(ctx, 0, signedRoBlock)
		require.NoError(t, err)
	})

	t.Run("with commitments", func(t *testing.T) {
		signedBeaconBlockFulu := util.NewBeaconBlockFulu()
		signedBeaconBlockFulu.Block.Slot = primitives.Slot(params.BeaconConfig().FuluForkEpoch) * params.BeaconConfig().SlotsPerEpoch
		signedBeaconBlockFulu.Block.Body.BlobKzgCommitments = commitments
		signedRoBlock := newSignedRoBlock(t, signedBeaconBlockFulu)
		block := signedRoBlock.Block()
		slot := block.Slot()
		proposerIndex := block.ProposerIndex()
		parentRoot := block.ParentRoot()
		stateRoot := block.StateRoot()
		bodyRoot, err := block.Body().HashTreeRoot()
		require.NoError(t, err)

		root := signedRoBlock.Root()

		storage := filesystem.NewEphemeralDataColumnStorage(t)

		indices := []uint64{1, 17, 19, 42, 75, 87, 102, 117}
		avs := NewLazilyPersistentStoreColumn(storage, newDataColumnsVerifier, enode.ID{}, uint64(len(indices)), nil, mockShouldRetain(slots.ToEpoch(slot)))
		dcparams := make([]util.DataColumnParam, 0, len(indices))
		for _, index := range indices {
			dataColumnParams := util.DataColumnParam{
				Index:          index,
				KzgCommitments: commitments,

				Slot:          slot,
				ProposerIndex: proposerIndex,
				ParentRoot:    parentRoot[:],
				StateRoot:     stateRoot[:],
				BodyRoot:      bodyRoot[:],
			}

			dcparams = append(dcparams, dataColumnParams)
		}

		_, verifiedRoDataColumns := util.CreateTestVerifiedRoDataColumnSidecars(t, dcparams)

		key := keyFromBlock(signedRoBlock)
		entry := avs.cache.entry(key)
		defer avs.cache.delete(key)

		for _, verifiedRoDataColumn := range verifiedRoDataColumns {
			err := entry.stash(verifiedRoDataColumn.RODataColumn)
			require.NoError(t, err)
		}

		err = avs.IsDataAvailable(ctx, slot, signedRoBlock)
		require.NoError(t, err)

		actual, err := storage.Get(root, indices)
		require.NoError(t, err)

		//summary := storage.Summary(root)
		require.Equal(t, len(verifiedRoDataColumns), len(actual))
		//require.Equal(t, uint64(len(indices)), summary.Count())
		//require.DeepSSZEqual(t, verifiedRoDataColumns, actual)
	})
}

func TestRetentionWindow(t *testing.T) {
	windowSlots, err := slots.EpochEnd(params.BeaconConfig().MinEpochsForDataColumnSidecarsRequest)
	require.NoError(t, err)
	fuluSlot, err := slots.EpochStart(params.BeaconConfig().FuluForkEpoch)
	require.NoError(t, err)

	numberOfColumns := fieldparams.NumberOfColumns
	testCases := []struct {
		name        string
		commitments [][]byte
		block       func(*testing.T) blocks.ROBlock
		slot        primitives.Slot
		wantedCols  int
	}{
		{
			name: "Pre-Fulu block",
			block: func(t *testing.T) blocks.ROBlock {
				return newSignedRoBlock(t, util.NewBeaconBlockElectra())
			},
		},
		{
			name: "Commitments outside data availability window",
			block: func(t *testing.T) blocks.ROBlock {
				beaconBlockElectra := util.NewBeaconBlockElectra()

				// Block is from slot 0, "current slot" is window size +1 (so outside the window)
				beaconBlockElectra.Block.Body.BlobKzgCommitments = commitments

				return newSignedRoBlock(t, beaconBlockElectra)
			},
			slot: fuluSlot + windowSlots,
		},
		{
			name: "Commitments within data availability window",
			block: func(t *testing.T) blocks.ROBlock {
				signedBeaconBlockFulu := util.NewBeaconBlockFulu()
				signedBeaconBlockFulu.Block.Body.BlobKzgCommitments = commitments
				signedBeaconBlockFulu.Block.Slot = fuluSlot + windowSlots - 1

				return newSignedRoBlock(t, signedBeaconBlockFulu)
			},
			commitments: commitments,
			slot:        fuluSlot + windowSlots,
			wantedCols:  numberOfColumns,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			b := tc.block(t)
			s := NewLazilyPersistentStoreColumn(nil, nil, enode.ID{}, uint64(numberOfColumns), nil, mockShouldRetain(slots.ToEpoch(tc.slot)))

			indices, err := s.required(b)
			require.NoError(t, err)

			require.Equal(t, tc.wantedCols, len(indices))
		})
	}
}

func newSignedRoBlock(t *testing.T, signedBeaconBlock any) blocks.ROBlock {
	sb, err := blocks.NewSignedBeaconBlock(signedBeaconBlock)
	require.NoError(t, err)

	rb, err := blocks.NewROBlock(sb)
	require.NoError(t, err)

	return rb
}

type mockDataColumnsVerifier struct {
	t                                                                        *testing.T
	dataColumnSidecars                                                       []blocks.RODataColumn
	validCalled, SidecarInclusionProvenCalled, SidecarKzgProofVerifiedCalled bool
}

var _ verification.DataColumnsVerifier = &mockDataColumnsVerifier{}

func (m *mockDataColumnsVerifier) VerifiedRODataColumns() ([]blocks.VerifiedRODataColumn, error) {
	require.Equal(m.t, true, m.validCalled && m.SidecarInclusionProvenCalled && m.SidecarKzgProofVerifiedCalled)

	verifiedDataColumnSidecars := make([]blocks.VerifiedRODataColumn, 0, len(m.dataColumnSidecars))
	for _, dataColumnSidecar := range m.dataColumnSidecars {
		verifiedDataColumnSidecar := blocks.NewVerifiedRODataColumn(dataColumnSidecar)
		verifiedDataColumnSidecars = append(verifiedDataColumnSidecars, verifiedDataColumnSidecar)
	}

	return verifiedDataColumnSidecars, nil
}

func (m *mockDataColumnsVerifier) SatisfyRequirement(verification.Requirement) {}

func (m *mockDataColumnsVerifier) ValidFields() error {
	m.validCalled = true
	return nil
}

func (m *mockDataColumnsVerifier) CorrectSubnet(dataColumnSidecarSubTopic string, expectedTopics []string) error {
	return nil
}
func (m *mockDataColumnsVerifier) NotFromFutureSlot() error                         { return nil }
func (m *mockDataColumnsVerifier) SlotAboveFinalized() error                        { return nil }
func (m *mockDataColumnsVerifier) ValidProposerSignature(ctx context.Context) error { return nil }

func (m *mockDataColumnsVerifier) SidecarParentSeen(parentSeen func([fieldparams.RootLength]byte) bool) error {
	return nil
}

func (m *mockDataColumnsVerifier) SidecarParentValid(badParent func([fieldparams.RootLength]byte) bool) error {
	return nil
}

func (m *mockDataColumnsVerifier) SidecarParentSlotLower() error       { return nil }
func (m *mockDataColumnsVerifier) SidecarDescendsFromFinalized() error { return nil }

func (m *mockDataColumnsVerifier) SidecarInclusionProven() error {
	m.SidecarInclusionProvenCalled = true
	return nil
}

func (m *mockDataColumnsVerifier) SidecarKzgProofVerified() error {
	m.SidecarKzgProofVerifiedCalled = true
	return nil
}

func (m *mockDataColumnsVerifier) SidecarProposerExpected(ctx context.Context) error { return nil }

// Mock implementations for bisectVerification tests

// mockBisectionIterator simulates a BisectionIterator for testing.
type mockBisectionIterator struct {
	chunks           [][]blocks.RODataColumn
	chunkErrors      []error
	finalError       error
	chunkIndex       int
	nextCallCount    int
	onErrorCallCount int
	onErrorErrors    []error
}

func (m *mockBisectionIterator) Next() ([]blocks.RODataColumn, error) {
	if m.chunkIndex >= len(m.chunks) {
		return nil, io.EOF
	}
	chunk := m.chunks[m.chunkIndex]
	var err error
	if m.chunkIndex < len(m.chunkErrors) {
		err = m.chunkErrors[m.chunkIndex]
	}
	m.chunkIndex++
	m.nextCallCount++
	if err != nil {
		return chunk, err
	}
	return chunk, nil
}

func (m *mockBisectionIterator) OnError(err error) {
	m.onErrorCallCount++
	m.onErrorErrors = append(m.onErrorErrors, err)
}

func (m *mockBisectionIterator) Error() error {
	return m.finalError
}

// mockBisector simulates a Bisector for testing.
type mockBisector struct {
	shouldError bool
	bisectErr   error
	iterator    *mockBisectionIterator
}

func (m *mockBisector) Bisect(columns []blocks.RODataColumn) (BisectionIterator, error) {
	if m.shouldError {
		return nil, m.bisectErr
	}
	return m.iterator, nil
}

// testDataColumnsVerifier implements verification.DataColumnsVerifier for testing.
type testDataColumnsVerifier struct {
	t          *testing.T
	shouldFail bool
	columns    []blocks.RODataColumn
}

func (v *testDataColumnsVerifier) VerifiedRODataColumns() ([]blocks.VerifiedRODataColumn, error) {
	verified := make([]blocks.VerifiedRODataColumn, len(v.columns))
	for i, col := range v.columns {
		verified[i] = blocks.NewVerifiedRODataColumn(col)
	}
	return verified, nil
}

func (v *testDataColumnsVerifier) SatisfyRequirement(verification.Requirement) {}
func (v *testDataColumnsVerifier) ValidFields() error {
	if v.shouldFail {
		return errors.New("verification failed")
	}
	return nil
}
func (v *testDataColumnsVerifier) CorrectSubnet(string, []string) error         { return nil }
func (v *testDataColumnsVerifier) NotFromFutureSlot() error                     { return nil }
func (v *testDataColumnsVerifier) SlotAboveFinalized() error                    { return nil }
func (v *testDataColumnsVerifier) ValidProposerSignature(context.Context) error { return nil }
func (v *testDataColumnsVerifier) SidecarParentSeen(func([fieldparams.RootLength]byte) bool) error {
	return nil
}
func (v *testDataColumnsVerifier) SidecarParentValid(func([fieldparams.RootLength]byte) bool) error {
	return nil
}
func (v *testDataColumnsVerifier) SidecarParentSlotLower() error                 { return nil }
func (v *testDataColumnsVerifier) SidecarDescendsFromFinalized() error           { return nil }
func (v *testDataColumnsVerifier) SidecarInclusionProven() error                 { return nil }
func (v *testDataColumnsVerifier) SidecarKzgProofVerified() error                { return nil }
func (v *testDataColumnsVerifier) SidecarProposerExpected(context.Context) error { return nil }

// Helper function to create test data columns
func makeTestDataColumns(t *testing.T, count int, blockRoot [32]byte, startIndex uint64) []blocks.RODataColumn {
	columns := make([]blocks.RODataColumn, 0, count)
	for i := range count {
		params := util.DataColumnParam{
			Index:          startIndex + uint64(i),
			KzgCommitments: commitments,
			Slot:           primitives.Slot(params.BeaconConfig().FuluForkEpoch) * params.BeaconConfig().SlotsPerEpoch,
		}
		_, verifiedCols := util.CreateTestVerifiedRoDataColumnSidecars(t, []util.DataColumnParam{params})
		if len(verifiedCols) > 0 {
			columns = append(columns, verifiedCols[0].RODataColumn)
		}
	}
	return columns
}

// Helper function to create test verifier factory with failure pattern
func makeTestVerifierFactory(failurePattern []bool) verification.NewDataColumnsVerifier {
	callIndex := 0
	return func(cols []blocks.RODataColumn, _ []verification.Requirement) verification.DataColumnsVerifier {
		shouldFail := callIndex < len(failurePattern) && failurePattern[callIndex]
		callIndex++
		return &testDataColumnsVerifier{
			shouldFail: shouldFail,
			columns:    cols,
		}
	}
}

// TestBisectVerification tests the bisectVerification method with comprehensive table-driven test cases.
func TestBisectVerification(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().FuluForkEpoch = params.BeaconConfig().ElectraForkEpoch + 4096*2

	cases := []struct {
		expectedError              bool
		bisectorNil                bool
		expectedOnErrorCallCount   int
		expectedNextCallCount      int
		inputCount                 int
		iteratorFinalError         error
		bisectorError              error
		name                       string
		storedColumnIndices        []uint64
		verificationFailurePattern []bool
		chunkErrors                []error
		chunks                     [][]blocks.RODataColumn
	}{
		{
			name:                     "EmptyColumns",
			inputCount:               0,
			expectedError:            false,
			expectedNextCallCount:    0,
			expectedOnErrorCallCount: 0,
		},
		{
			name:                     "NilBisector",
			inputCount:               3,
			bisectorNil:              true,
			expectedError:            true,
			expectedNextCallCount:    0,
			expectedOnErrorCallCount: 0,
		},
		{
			name:                     "BisectError",
			inputCount:               5,
			bisectorError:            errors.New("bisect failed"),
			expectedError:            true,
			expectedNextCallCount:    0,
			expectedOnErrorCallCount: 0,
		},
		{
			name:                       "SingleChunkSuccess",
			inputCount:                 4,
			chunks:                     [][]blocks.RODataColumn{{}},
			verificationFailurePattern: []bool{false},
			expectedError:              false,
			expectedNextCallCount:      2,
			expectedOnErrorCallCount:   0,
		},
		{
			name:                       "SingleChunkFails",
			inputCount:                 4,
			chunks:                     [][]blocks.RODataColumn{{}},
			verificationFailurePattern: []bool{true},
			iteratorFinalError:         errors.New("chunk failed"),
			expectedError:              true,
			expectedNextCallCount:      2,
			expectedOnErrorCallCount:   1,
		},
		{
			name:                       "TwoChunks_BothPass",
			inputCount:                 8,
			chunks:                     [][]blocks.RODataColumn{{}, {}},
			verificationFailurePattern: []bool{false, false},
			expectedError:              false,
			expectedNextCallCount:      3,
			expectedOnErrorCallCount:   0,
		},
		{
			name:                       "TwoChunks_FirstFails",
			inputCount:                 8,
			chunks:                     [][]blocks.RODataColumn{{}, {}},
			verificationFailurePattern: []bool{true, false},
			iteratorFinalError:         errors.New("first failed"),
			expectedError:              true,
			expectedNextCallCount:      3,
			expectedOnErrorCallCount:   1,
		},
		{
			name:                       "TwoChunks_SecondFails",
			inputCount:                 8,
			chunks:                     [][]blocks.RODataColumn{{}, {}},
			verificationFailurePattern: []bool{false, true},
			iteratorFinalError:         errors.New("second failed"),
			expectedError:              true,
			expectedNextCallCount:      3,
			expectedOnErrorCallCount:   1,
		},
		{
			name:                       "TwoChunks_BothFail",
			inputCount:                 8,
			chunks:                     [][]blocks.RODataColumn{{}, {}},
			verificationFailurePattern: []bool{true, true},
			iteratorFinalError:         errors.New("both failed"),
			expectedError:              true,
			expectedNextCallCount:      3,
			expectedOnErrorCallCount:   2,
		},
		{
			name:                       "ManyChunks_AllPass",
			inputCount:                 16,
			chunks:                     [][]blocks.RODataColumn{{}, {}, {}, {}},
			verificationFailurePattern: []bool{false, false, false, false},
			expectedError:              false,
			expectedNextCallCount:      5,
			expectedOnErrorCallCount:   0,
		},
		{
			name:                       "ManyChunks_MixedFail",
			inputCount:                 16,
			chunks:                     [][]blocks.RODataColumn{{}, {}, {}, {}},
			verificationFailurePattern: []bool{false, true, false, true},
			iteratorFinalError:         errors.New("mixed failures"),
			expectedError:              true,
			expectedNextCallCount:      5,
			expectedOnErrorCallCount:   2,
		},
		{
			name:                       "FilterStoredColumns_PartialFilter",
			inputCount:                 6,
			chunks:                     [][]blocks.RODataColumn{{}},
			verificationFailurePattern: []bool{false},
			storedColumnIndices:        []uint64{1, 3},
			expectedError:              false,
			expectedNextCallCount:      2,
			expectedOnErrorCallCount:   0,
		},
		{
			name:                       "FilterStoredColumns_AllStored",
			inputCount:                 6,
			chunks:                     [][]blocks.RODataColumn{{}},
			verificationFailurePattern: []bool{false},
			storedColumnIndices:        []uint64{0, 1, 2, 3, 4, 5},
			expectedError:              false,
			expectedNextCallCount:      2,
			expectedOnErrorCallCount:   0,
		},
		{
			name:                       "FilterStoredColumns_MixedAccess",
			inputCount:                 10,
			chunks:                     [][]blocks.RODataColumn{{}},
			verificationFailurePattern: []bool{false},
			storedColumnIndices:        []uint64{1, 5, 9},
			expectedError:              false,
			expectedNextCallCount:      2,
			expectedOnErrorCallCount:   0,
		},
		{
			name:                       "IteratorNextError",
			inputCount:                 4,
			chunks:                     [][]blocks.RODataColumn{{}, {}},
			chunkErrors:                []error{nil, errors.New("next error")},
			verificationFailurePattern: []bool{false},
			expectedError:              true,
			expectedNextCallCount:      2,
			expectedOnErrorCallCount:   0,
		},
		{
			name:                       "IteratorNextEOF",
			inputCount:                 4,
			chunks:                     [][]blocks.RODataColumn{{}},
			verificationFailurePattern: []bool{false},
			expectedError:              false,
			expectedNextCallCount:      2,
			expectedOnErrorCallCount:   0,
		},
		{
			name:                       "LargeChunkSize",
			inputCount:                 128,
			chunks:                     [][]blocks.RODataColumn{{}},
			verificationFailurePattern: []bool{false},
			expectedError:              false,
			expectedNextCallCount:      2,
			expectedOnErrorCallCount:   0,
		},
		{
			name:                       "ManySmallChunks",
			inputCount:                 32,
			chunks:                     [][]blocks.RODataColumn{{}, {}, {}, {}, {}, {}, {}, {}},
			verificationFailurePattern: []bool{false, false, false, false, false, false, false, false},
			expectedError:              false,
			expectedNextCallCount:      9,
			expectedOnErrorCallCount:   0,
		},
		{
			name:                       "ChunkWithSomeStoredColumns",
			inputCount:                 6,
			chunks:                     [][]blocks.RODataColumn{{}},
			verificationFailurePattern: []bool{false},
			storedColumnIndices:        []uint64{0, 2, 4},
			expectedError:              false,
			expectedNextCallCount:      2,
			expectedOnErrorCallCount:   0,
		},
		{
			name:                       "OnErrorDoesNotStopIteration",
			inputCount:                 8,
			chunks:                     [][]blocks.RODataColumn{{}, {}},
			verificationFailurePattern: []bool{true, false},
			iteratorFinalError:         errors.New("first failed"),
			expectedError:              true,
			expectedNextCallCount:      3,
			expectedOnErrorCallCount:   1,
		},
		{
			name:                       "VerificationErrorWrapping",
			inputCount:                 4,
			chunks:                     [][]blocks.RODataColumn{{}},
			verificationFailurePattern: []bool{true},
			iteratorFinalError:         errors.New("verification failed"),
			expectedError:              true,
			expectedNextCallCount:      2,
			expectedOnErrorCallCount:   1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup storage
			var store *filesystem.DataColumnStorage
			if len(tc.storedColumnIndices) > 0 {
				mocker, s := filesystem.NewEphemeralDataColumnStorageWithMocker(t)
				blockRoot := [32]byte{1, 2, 3}
				slot := primitives.Slot(params.BeaconConfig().FuluForkEpoch) * params.BeaconConfig().SlotsPerEpoch
				require.NoError(t, mocker.CreateFakeIndices(blockRoot, slot, tc.storedColumnIndices...))
				store = s
			} else {
				store = filesystem.NewEphemeralDataColumnStorage(t)
			}

			// Create test columns
			blockRoot := [32]byte{1, 2, 3}
			columns := makeTestDataColumns(t, tc.inputCount, blockRoot, 0)

			// Setup iterator with chunks
			iterator := &mockBisectionIterator{
				chunks:      tc.chunks,
				chunkErrors: tc.chunkErrors,
				finalError:  tc.iteratorFinalError,
			}

			// Setup bisector
			var bisector Bisector
			if tc.bisectorNil || tc.inputCount == 0 {
				bisector = nil
			} else if tc.bisectorError != nil {
				bisector = &mockBisector{
					shouldError: true,
					bisectErr:   tc.bisectorError,
				}
			} else {
				bisector = &mockBisector{
					shouldError: false,
					iterator:    iterator,
				}
			}

			// Create store with verifier
			verifierFactory := makeTestVerifierFactory(tc.verificationFailurePattern)
			lazilyPersistentStore := &LazilyPersistentStoreColumn{
				store:                  store,
				cache:                  newDataColumnCache(),
				newDataColumnsVerifier: verifierFactory,
				custody:                &custodyRequirement{},
				bisector:               bisector,
			}

			// Execute
			err := lazilyPersistentStore.bisectVerification(columns)

			// Assert
			if tc.expectedError {
				require.NotNil(t, err)
			} else {
				require.NoError(t, err)
			}

			// Verify iterator interactions for non-error cases
			if tc.inputCount > 0 && bisector != nil && tc.bisectorError == nil && !tc.expectedError {
				require.NotEqual(t, 0, iterator.nextCallCount, "iterator Next() should have been called")
				require.Equal(t, tc.expectedOnErrorCallCount, iterator.onErrorCallCount, "OnError() call count mismatch")
			}
		})
	}
}

func allIndicesExcept(total int, excluded []uint64) []uint64 {
	excludeMap := make(map[uint64]bool)
	for _, idx := range excluded {
		excludeMap[idx] = true
	}

	var result []uint64
	for i := range total {
		if !excludeMap[uint64(i)] {
			result = append(result, uint64(i))
		}
	}
	return result
}

// TestColumnsNotStored tests the columnsNotStored method.
func TestColumnsNotStored(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().FuluForkEpoch = params.BeaconConfig().ElectraForkEpoch + 4096*2

	cases := []struct {
		name     string
		count    int
		stored   []uint64 // Column indices marked as stored
		expected []uint64 // Expected column indices in returned result
	}{
		// Empty cases
		{
			name:     "EmptyInput",
			count:    0,
			stored:   []uint64{},
			expected: []uint64{},
		},
		// Single element cases
		{
			name:     "SingleElement_NotStored",
			count:    1,
			stored:   []uint64{},
			expected: []uint64{0},
		},
		{
			name:     "SingleElement_Stored",
			count:    1,
			stored:   []uint64{0},
			expected: []uint64{},
		},
		// All not stored cases
		{
			name:     "AllNotStored_FiveElements",
			count:    5,
			stored:   []uint64{},
			expected: []uint64{0, 1, 2, 3, 4},
		},
		// All stored cases
		{
			name:     "AllStored",
			count:    5,
			stored:   []uint64{0, 1, 2, 3, 4},
			expected: []uint64{},
		},
		// Partial storage - beginning
		{
			name:     "StoredAtBeginning",
			count:    5,
			stored:   []uint64{0, 1},
			expected: []uint64{2, 3, 4},
		},
		// Partial storage - end
		{
			name:     "StoredAtEnd",
			count:    5,
			stored:   []uint64{3, 4},
			expected: []uint64{0, 1, 2},
		},
		// Partial storage - middle
		{
			name:     "StoredInMiddle",
			count:    5,
			stored:   []uint64{2},
			expected: []uint64{0, 1, 3, 4},
		},
		// Partial storage - scattered
		{
			name:     "StoredScattered",
			count:    8,
			stored:   []uint64{1, 3, 5},
			expected: []uint64{0, 2, 4, 6, 7},
		},
		// Alternating pattern
		{
			name:     "AlternatingPattern",
			count:    8,
			stored:   []uint64{0, 2, 4, 6},
			expected: []uint64{1, 3, 5, 7},
		},
		// Consecutive stored
		{
			name:     "ConsecutiveStored",
			count:    10,
			stored:   []uint64{3, 4, 5, 6},
			expected: []uint64{0, 1, 2, 7, 8, 9},
		},
		// Large slice cases
		{
			name:     "LargeSlice_NoStored",
			count:    64,
			stored:   []uint64{},
			expected: allIndicesExcept(64, []uint64{}),
		},
		{
			name:     "LargeSlice_SingleStored",
			count:    64,
			stored:   []uint64{32},
			expected: allIndicesExcept(64, []uint64{32}),
		},
	}

	slot := primitives.Slot(params.BeaconConfig().FuluForkEpoch) * params.BeaconConfig().SlotsPerEpoch

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Create test columns first to get the actual block root
			var columns []blocks.RODataColumn
			if tc.count > 0 {
				columns = makeTestDataColumns(t, tc.count, [32]byte{}, 0)
			}

			// Get the actual block root from the first column (if any)
			var blockRoot [32]byte
			if len(columns) > 0 {
				blockRoot = columns[0].BlockRoot()
			}

			// Setup storage
			var store *filesystem.DataColumnStorage
			if len(tc.stored) > 0 {
				mocker, s := filesystem.NewEphemeralDataColumnStorageWithMocker(t)
				require.NoError(t, mocker.CreateFakeIndices(blockRoot, slot, tc.stored...))
				store = s
			} else {
				store = filesystem.NewEphemeralDataColumnStorage(t)
			}

			// Create store instance
			lazilyPersistentStore := &LazilyPersistentStoreColumn{
				store: store,
			}

			// Execute
			result := lazilyPersistentStore.columnsNotStored(columns)

			// Assert count
			require.Equal(t, len(tc.expected), len(result),
				fmt.Sprintf("expected %d columns, got %d", len(tc.expected), len(result)))

			// Verify that no stored columns are in the result
			if len(tc.stored) > 0 {
				resultIndices := make(map[uint64]bool)
				for _, col := range result {
					resultIndices[col.Index] = true
				}
				for _, storedIdx := range tc.stored {
					require.Equal(t, false, resultIndices[storedIdx],
						fmt.Sprintf("stored column index %d should not be in result", storedIdx))
				}
			}

			// If expectedIndices is specified, verify the exact column indices in order
			if len(tc.expected) > 0 && len(tc.stored) == 0 {
				// Only check exact order for non-stored cases (where we know they stay in same order)
				for i, expectedIdx := range tc.expected {
					require.Equal(t, columns[expectedIdx].Index, result[i].Index,
						fmt.Sprintf("column %d: expected index %d, got %d", i, columns[expectedIdx].Index, result[i].Index))
				}
			}

			// Verify optimization: if nothing stored, should return original slice
			if len(tc.stored) == 0 && tc.count > 0 {
				require.Equal(t, &columns[0], &result[0],
					"when no columns stored, should return original slice (same pointer)")
			}

			// Verify optimization: if some stored, result should use in-place shifting
			if len(tc.stored) > 0 && len(tc.expected) > 0 && tc.count > 0 {
				require.Equal(t, cap(columns), cap(result),
					"result should be in-place shifted from original (same capacity)")
			}
		})
	}
}
