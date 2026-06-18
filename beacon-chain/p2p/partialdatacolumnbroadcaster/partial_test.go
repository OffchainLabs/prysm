package partialdatacolumnbroadcaster

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	libp2p "github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
)

type headerHandlerCall struct {
	header  *ethpb.PartialDataColumnHeader
	groupID string
}

type columnHandlerCall struct {
	topic  string
	column blocks.VerifiedRODataColumn
}

type broadcasterHarness struct {
	t           *testing.T
	broadcaster *PartialColumnBroadcaster
	cancel      context.CancelFunc
}

type callbackRecorder struct {
	validateColumnCallCh                   chan []blocks.CellProofBundle
	handleHeaderCallCh                     chan headerHandlerCall
	partialVerifierFromHeaderCallCh        chan *blocks.PartialDataColumn
	partialVerifierFromTrustedColumnCallCh chan *blocks.PartialDataColumn
	handleColumnCallCh                     chan columnHandlerCall

	validateColumnErr                error
	partialVerifierFromHeaderErr     error
	partialVerifierFromHeaderReject  bool
	partialVerifierFromTrustedColErr error
}

func newCallbackRecorder(callBuffer int, validateHeaderReject bool, validateColumnErr, validateHeaderErr error) *callbackRecorder {
	return &callbackRecorder{
		validateColumnCallCh:                   make(chan []blocks.CellProofBundle, callBuffer),
		handleHeaderCallCh:                     make(chan headerHandlerCall, callBuffer),
		partialVerifierFromHeaderCallCh:        make(chan *blocks.PartialDataColumn, callBuffer),
		partialVerifierFromTrustedColumnCallCh: make(chan *blocks.PartialDataColumn, callBuffer),
		handleColumnCallCh:                     make(chan columnHandlerCall, callBuffer),
		partialVerifierFromHeaderReject:        validateHeaderReject,
		partialVerifierFromHeaderErr:           validateHeaderErr,
		validateColumnErr:                      validateColumnErr,
	}
}

func (r *callbackRecorder) PartialVerifierFromHeader(col *blocks.PartialDataColumn) (*verification.PartialColumnVerifier, pubsub.ValidationResult, error) {
	r.partialVerifierFromHeaderCallCh <- col
	result := pubsub.ValidationAccept
	if r.partialVerifierFromHeaderErr != nil {
		result = pubsub.ValidationIgnore
		if r.partialVerifierFromHeaderReject {
			result = pubsub.ValidationReject
		}
	}
	return newMockPartialVerifier(col), result, r.partialVerifierFromHeaderErr
}

func (r *callbackRecorder) PartialVerifierFromTrustedColumn(col *blocks.PartialDataColumn) (*verification.PartialColumnVerifier, error) {
	r.partialVerifierFromTrustedColumnCallCh <- col
	return newMockPartialVerifier(col), r.partialVerifierFromTrustedColErr
}

func (r *callbackRecorder) ValidateColumn(cells []blocks.CellProofBundle) error {
	r.validateColumnCallCh <- cells
	return r.validateColumnErr
}

func (r *callbackRecorder) HandleColumn(topic string, col blocks.VerifiedRODataColumn) {
	r.handleColumnCallCh <- columnHandlerCall{topic: topic, column: col}
}

func (r *callbackRecorder) HandleHeader(header *ethpb.PartialDataColumnHeader, groupID string) {
	r.handleHeaderCallCh <- headerHandlerCall{header: header, groupID: groupID}
}

type peerFeedbackCall struct {
	peerID peer.ID
	topic  string
	kind   pubsub.PeerFeedbackKind
}

type publishedColumn struct {
	published *blocks.PartialDataColumn
	topic     string
}

type mockPubSub struct {
	mu                       sync.Mutex
	publishedPartialColumns  []publishedColumn
	peerFeedbackCalls        []peerFeedbackCall
	publishPartialMessageErr error
	peerFeedbackErr          error
}

func newMockPubSub(publisherr, peerFeebackErr error) *mockPubSub {
	return &mockPubSub{
		publishedPartialColumns:  make([]publishedColumn, 0, 8),
		peerFeedbackCalls:        make([]peerFeedbackCall, 0, 8),
		peerFeedbackErr:          peerFeebackErr,
		publishPartialMessageErr: publisherr,
	}
}

func (m *mockPubSub) assertPartialColumnsPublished(t *testing.T, topic string, expected []*blocks.PartialDataColumn) {
	t.Helper()

	actual := m.publishedColumnsSnapshot()

	require.Equal(t, len(expected), len(actual))
	if len(expected) == 0 {
		return
	}
	for i := range expected {
		require.Equal(t, topic, actual[i].topic)

		assertPublishedPartialColumnMatches(t, expected[i], actual[i].published)
	}
}

func (m *mockPubSub) peerFeedback(topic string, id peer.ID, kind pubsub.PeerFeedbackKind) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.peerFeedbackCalls = append(m.peerFeedbackCalls, peerFeedbackCall{
		peerID: id,
		topic:  topic,
		kind:   kind,
	})
	return m.peerFeedbackErr
}

func (m *mockPubSub) publishPartialCol(topic string, groupID []byte, col *blocks.PartialDataColumn) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishedPartialColumns = append(m.publishedPartialColumns, publishedColumn{col, topic})
	retErr := m.publishPartialMessageErr
	return retErr
}

func (m *mockPubSub) peerFeedbackCallsSnapshot() []peerFeedbackCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.peerFeedbackCalls)
}

func (m *mockPubSub) publishedColumnsSnapshot() []publishedColumn {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.publishedPartialColumns)
}

func (m *mockPubSub) peerFeedbackCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.peerFeedbackCalls)
}

func (m *mockPubSub) publishedColumnCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.publishedPartialColumns)
}

func newBroadcasterHarness(t *testing.T, ps *mockPubSub) *broadcasterHarness {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	broadcaster := NewBroadcaster(ctx, logrus.New())
	broadcaster.peerFeedback = ps.peerFeedback
	broadcaster.publishPartialCol = ps.publishPartialCol

	return &broadcasterHarness{
		t:           t,
		broadcaster: broadcaster,
		cancel:      cancel,
	}
}

func (h *broadcasterHarness) start(cr *callbackRecorder) {
	h.t.Helper()
	go h.broadcaster.Start(cr)
}

// Stop stops the broadcaster's event loop by cancelling its context.
func (h *broadcasterHarness) Stop() {
	h.t.Helper()
	h.cancel()
}

func createPartialColumn(t *testing.T, nCells uint64, cells map[uint64][]byte) *blocks.PartialDataColumn {
	t.Helper()

	commitments := make([][]byte, nCells)
	for i := range nCells {
		commitments[i] = []byte{byte(i + 1)}
	}

	header := &ethpb.SignedBeaconBlockHeader{
		Header: &ethpb.BeaconBlockHeader{
			ParentRoot: make([]byte, 32),
			StateRoot:  make([]byte, 32),
			BodyRoot:   make([]byte, 32),
		},
		Signature: []byte{1},
	}
	root, err := header.Header.HashTreeRoot()
	require.NoError(t, err)

	c, err := blocks.NewPartialDataColumn(
		root,
		header,
		12,
		commitments,
		nil,
	)
	require.NoError(t, err)

	for idx, cell := range cells {
		ok := c.ExtendFromVerifiedCell(
			idx,
			slices.Clone(cell),
			[]byte{0xC0 + byte(idx)},
		)
		require.Equal(t, true, ok)
	}

	return &c
}

func assertPublishedPartialColumnMatches(t *testing.T, expected *blocks.PartialDataColumn, actual *blocks.PartialDataColumn) {
	t.Helper()

	require.NotNil(t, expected)
	require.NotNil(t, actual)
	require.DeepEqual(t, expected.GroupID(), actual.GroupID())
	require.Equal(t, true, bytes.Equal(expected.Included, actual.Included))
	require.DeepEqual(t, expected.Column, actual.Column)
}

func assertPartialColumnsEqual(t *testing.T, expected *blocks.PartialDataColumn, actual *blocks.PartialDataColumn) {
	t.Helper()

	require.NotNil(t, expected)
	require.NotNil(t, actual)

	expectedRoot, err := expected.SignedBlockHeader.Header.HashTreeRoot()
	require.NoError(t, err)
	actualRoot, err := actual.SignedBlockHeader.Header.HashTreeRoot()
	require.NoError(t, err)

	require.DeepEqual(t, expectedRoot, actualRoot)
	require.DeepEqual(t, expected.GroupID(), actual.GroupID())
	require.DeepEqual(t, expected.Included, actual.Included)
	require.DeepEqual(t, expected.Column, actual.Column)
}

func newMockPartialVerifier(col *blocks.PartialDataColumn) *verification.PartialColumnVerifier {
	mv := &verification.MockDataColumnsVerifier{}
	ro, err := blocks.NewRODataColumn(col.DataColumnSidecar)
	if err == nil {
		mv.AppendRODataColumns(ro)
	}
	return verification.NewPartialColumnVerifier(mv, col)
}

func newMarkedVerifier(col *blocks.PartialDataColumn) *verification.PartialColumnVerifier {
	return newMockPartialVerifier(col)
}

func newMockPartialVerifierWithValidFieldsErr(col *blocks.PartialDataColumn, validFieldsErr error) *verification.PartialColumnVerifier {
	mv := &verification.MockDataColumnsVerifier{ErrValidFields: validFieldsErr}
	ro, err := blocks.NewRODataColumn(col.DataColumnSidecar)
	if err == nil {
		mv.AppendRODataColumns(ro)
	}
	return verification.NewPartialColumnVerifier(mv, col)
}

func testBitlist(n uint64, set ...uint64) bitfield.Bitlist {
	bl := bitfield.NewBitlist(n)
	for _, idx := range set {
		bl.SetBitAt(idx, true)
	}
	return bl
}

func testPartsMetadata(availableLen uint64, availableSet []uint64, requestsSet []uint64) *ethpb.PartialDataColumnPartsMetadata {
	return &ethpb.PartialDataColumnPartsMetadata{
		Available: testBitlist(availableLen, availableSet...),
		Requests:  testBitlist(availableLen, requestsSet...),
	}
}

func testPartsMetadataCustom(availableLen uint64, availableSet []uint64, requestLen uint64, requestsSet []uint64) *ethpb.PartialDataColumnPartsMetadata {
	return &ethpb.PartialDataColumnPartsMetadata{
		Available: testBitlist(availableLen, availableSet...),
		Requests:  testBitlist(requestLen, requestsSet...),
	}
}

func mustMarshalPartsMetadata(t *testing.T, meta *ethpb.PartialDataColumnPartsMetadata) []byte {
	t.Helper()
	b, err := meta.MarshalSSZ()
	require.NoError(t, err)
	return b
}

func mustMarshalSidecar(t *testing.T, cellsPresent bitfield.Bitlist) []byte {
	t.Helper()
	b, err := (&ethpb.PartialDataColumnSidecar{
		CellsPresentBitmap: cellsPresent,
	}).MarshalSSZ()
	require.NoError(t, err)
	return b
}

func assertPeerStatePartsMetadata(t *testing.T, got, want *ethpb.PartialDataColumnPartsMetadata) {
	t.Helper()
	if want == nil {
		require.Equal(t, want, got)
		return
	}

	require.NotNil(t, got)
	require.DeepEqual(t, want.Available, got.Available)
	require.DeepEqual(t, want.Requests, got.Requests)
}

func assertBitlistEqual(t *testing.T, got, want bitfield.Bitlist) {
	t.Helper()
	require.Equal(t, len(want), len(got))
	require.Equal(t, want.Len(), got.Len())
	for i := uint64(0); i < want.Len(); i++ {
		require.Equal(t, want.BitAt(i), got.BitAt(i))
	}
}

func buildValidatedCells(columnIndex uint64, cellsByIndex map[uint64][]byte) ([]uint64, []blocks.CellProofBundle) {
	indices := make([]uint64, 0, len(cellsByIndex))
	for idx := range cellsByIndex {
		indices = append(indices, idx)
	}
	slices.Sort(indices)

	cells := make([]blocks.CellProofBundle, 0, len(indices))
	for _, idx := range indices {
		cells = append(cells, blocks.CellProofBundle{
			ColumnIndex: columnIndex,
			Cell:        slices.Clone(cellsByIndex[idx]),
			Proof:       []byte{0xE0 + byte(idx)},
		})
	}

	return indices, cells
}

func buildHeaderFromColumn(c *blocks.PartialDataColumn) *ethpb.PartialDataColumnHeader {
	return &ethpb.PartialDataColumnHeader{
		SignedBlockHeader:            c.SignedBlockHeader,
		KzgCommitments:               c.KzgCommitments,
		KzgCommitmentsInclusionProof: c.KzgCommitmentsInclusionProof,
	}
}

func buildHeaderOnlySidecar(c *blocks.PartialDataColumn) *ethpb.PartialDataColumnSidecar {
	return &ethpb.PartialDataColumnSidecar{
		CellsPresentBitmap: testBitlist(uint64(len(c.KzgCommitments))),
		Header:             []*ethpb.PartialDataColumnHeader{buildHeaderFromColumn(c)},
	}
}

func buildSidecarWithCells(nCells uint64, cellsByIndex map[uint64][]byte) *ethpb.PartialDataColumnSidecar {
	indices := make([]uint64, 0, len(cellsByIndex))
	for idx := range cellsByIndex {
		indices = append(indices, idx)
	}
	slices.Sort(indices)

	msg := &ethpb.PartialDataColumnSidecar{
		CellsPresentBitmap: testBitlist(nCells, indices...),
		PartialColumn:      make([][]byte, 0, len(indices)),
		KzgProofs:          make([][]byte, 0, len(indices)),
	}
	for _, idx := range indices {
		msg.PartialColumn = append(msg.PartialColumn, slices.Clone(cellsByIndex[idx]))
		msg.KzgProofs = append(msg.KzgProofs, []byte{0xB0 + byte(idx)})
	}
	return msg
}

func buildIncomingRPC(topic string, group []byte, message *ethpb.PartialDataColumnSidecar, partsMetadata []byte) incomingPartialRPC {
	topicCopy := topic
	return incomingPartialRPC{
		PartialMessagesExtension: &pubsub_pb.PartialMessagesExtension{
			TopicID:        &topicCopy,
			GroupID:        slices.Clone(group),
			PartsMetadata:  slices.Clone(partsMetadata),
			PartialMessage: nil,
		},
		from:    peer.ID("peer-a"),
		message: message,
	}
}

func buildExpectedCellsToVerify(c *blocks.PartialDataColumn, cellsByIndex map[uint64][]byte) ([]uint64, []blocks.CellProofBundle) {
	indices := make([]uint64, 0, len(cellsByIndex))
	for idx := range cellsByIndex {
		indices = append(indices, idx)
	}
	slices.Sort(indices)

	cells := make([]blocks.CellProofBundle, 0, len(indices))
	for _, idx := range indices {
		cells = append(cells, blocks.CellProofBundle{
			ColumnIndex: c.Index,
			Commitment:  slices.Clone(c.KzgCommitments[idx]),
			Cell:        slices.Clone(cellsByIndex[idx]),
			Proof:       []byte{0xB0 + byte(idx)},
		})
	}

	return indices, cells
}

func assertCellProofBundlesEqual(t *testing.T, expected, actual []blocks.CellProofBundle) {
	t.Helper()
	require.Equal(t, len(expected), len(actual))
	for i := range expected {
		require.Equal(t, expected[i].ColumnIndex, actual[i].ColumnIndex)
		require.DeepEqual(t, expected[i].Commitment, actual[i].Commitment)
		require.DeepEqual(t, expected[i].Cell, actual[i].Cell)
		require.DeepEqual(t, expected[i].Proof, actual[i].Proof)
	}
}

func assertCellsValidatedEqual(t *testing.T, expected, actual *cellsValidated) {
	t.Helper()
	require.NotNil(t, expected)
	require.NotNil(t, actual)
	require.Equal(t, expected.topic, actual.topic)
	require.DeepEqual(t, expected.group, actual.group)
	require.DeepEqual(t, expected.cellIndices, actual.cellIndices)
	assertCellProofBundlesEqual(t, expected.cells, actual.cells)
}

func waitForPeerFeedbackCalls(t *testing.T, ps *mockPubSub, expected int) {
	t.Helper()
	deadline := time.NewTimer(500 * time.Millisecond)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		if ps.peerFeedbackCallCount() >= expected {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("expected at least %d peer feedback calls, got %d", expected, ps.peerFeedbackCallCount())
		case <-ticker.C:
		}
	}
}

func TestExtractColumnIndexFromTopic(t *testing.T) {
	tests := []struct {
		name            string
		topic           string
		expectedIndex   uint64
		wantErrContains string
	}{
		{
			name:          "valid topic extracts column index",
			topic:         "/eth2/abcd1234/data_column_sidecar_12/ssz_snappy",
			expectedIndex: 12,
		},
		{
			name:            "topic without data_column_sidecar prefix returns error",
			topic:           "/eth2/abcd1234/beacon_block/ssz_snappy",
			wantErrContains: "could not extract column index from topic",
		},
		{
			name:          "column index zero",
			topic:         "/eth2/abcd1234/data_column_sidecar_0/ssz_snappy",
			expectedIndex: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index, err := extractColumnIndexFromTopic(tt.topic)
			if tt.wantErrContains != "" {
				require.ErrorContains(t, tt.wantErrContains, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedIndex, index)
			}
		})
	}
}

func TestUpdatePeerStateFromIncomingRPC(t *testing.T) {
	tests := []struct {
		wantErrContains      string
		name                 string
		inputPeerState       func() blocks.PartialDataColumnPeerState
		inputRPC             func(t *testing.T) *pubsub_pb.PartialMessagesExtension
		expectedRecvdState   func() *ethpb.PartialDataColumnPartsMetadata
		expectedSentState    func() *ethpb.PartialDataColumnPartsMetadata
		expectedMesageBitmap bitfield.Bitlist
		expectedMessage      bool
	}{
		{
			name: "incoming parts metadata only initializes recvd state",
			inputPeerState: func() blocks.PartialDataColumnPeerState {
				return blocks.PartialDataColumnPeerState{}
			},
			inputRPC: func(t *testing.T) *pubsub_pb.PartialMessagesExtension {
				return &pubsub_pb.PartialMessagesExtension{
					PartsMetadata: mustMarshalPartsMetadata(t, testPartsMetadata(4, []uint64{1, 3}, []uint64{0, 2})),
				}
			},
			expectedRecvdState: func() *ethpb.PartialDataColumnPartsMetadata {
				return testPartsMetadata(4, []uint64{1, 3}, []uint64{0, 2})
			},
		},
		{
			name: "incoming parts metadata merges with existing recvd state available and overwrites requests and does not update sent state",
			inputPeerState: func() blocks.PartialDataColumnPeerState {
				return blocks.PartialDataColumnPeerState{
					Recvd: testPartsMetadata(4, []uint64{0}, []uint64{0}),
					Sent:  testPartsMetadata(4, []uint64{3}, []uint64{1}),
				}
			},
			inputRPC: func(t *testing.T) *pubsub_pb.PartialMessagesExtension {
				return &pubsub_pb.PartialMessagesExtension{
					PartsMetadata: mustMarshalPartsMetadata(t, testPartsMetadata(4, []uint64{2}, []uint64{2, 3})),
				}
			},
			expectedRecvdState: func() *ethpb.PartialDataColumnPartsMetadata {
				return testPartsMetadata(4, []uint64{0, 2}, []uint64{2, 3})
			},
			expectedSentState: func() *ethpb.PartialDataColumnPartsMetadata {
				return testPartsMetadata(4, []uint64{3}, []uint64{1})
			},
		},
		{
			name: "partial message  updates recvd and sent states when existing peer state is empty",
			inputPeerState: func() blocks.PartialDataColumnPeerState {
				return blocks.PartialDataColumnPeerState{}
			},
			inputRPC: func(t *testing.T) *pubsub_pb.PartialMessagesExtension {
				return &pubsub_pb.PartialMessagesExtension{
					PartialMessage: mustMarshalSidecar(t, testBitlist(4, 1, 3)),
				}
			},
			expectedMessage:      true,
			expectedMesageBitmap: testBitlist(4, 1, 3),
			expectedRecvdState: func() *ethpb.PartialDataColumnPartsMetadata {
				return testPartsMetadata(4, []uint64{1, 3}, nil)
			},
			expectedSentState: func() *ethpb.PartialDataColumnPartsMetadata {
				return testPartsMetadata(4, []uint64{1, 3}, nil)
			},
		},
		{
			name: "incoming parts metadata with message skips recvd update from message and updates sent",
			inputPeerState: func() blocks.PartialDataColumnPeerState {
				return blocks.PartialDataColumnPeerState{
					Sent: testPartsMetadata(4, []uint64{0, 1}, nil),
				}
			},
			inputRPC: func(t *testing.T) *pubsub_pb.PartialMessagesExtension {
				return &pubsub_pb.PartialMessagesExtension{
					PartsMetadata:  mustMarshalPartsMetadata(t, testPartsMetadata(4, []uint64{2}, []uint64{1})),
					PartialMessage: mustMarshalSidecar(t, testBitlist(4, 0, 3)),
				}
			},
			expectedMessage:      true,
			expectedMesageBitmap: testBitlist(4, 0, 3),
			expectedRecvdState: func() *ethpb.PartialDataColumnPartsMetadata {
				return testPartsMetadata(4, []uint64{2}, []uint64{1})
			},
			expectedSentState: func() *ethpb.PartialDataColumnPartsMetadata {
				return testPartsMetadata(4, []uint64{0, 1, 3}, nil)
			},
		},
		{
			name: "message with empty cells bitmap bytes returns decode error",
			inputPeerState: func() blocks.PartialDataColumnPeerState {
				return blocks.PartialDataColumnPeerState{}
			},
			inputRPC: func(t *testing.T) *pubsub_pb.PartialMessagesExtension {
				return &pubsub_pb.PartialMessagesExtension{
					PartialMessage: mustMarshalSidecar(t, nil),
				}
			},
			wantErrContains: "failed to unmarshal partial message data",
		},
		{
			name: "invalid incoming parts metadata bytes return error",
			inputPeerState: func() blocks.PartialDataColumnPeerState {
				return blocks.PartialDataColumnPeerState{}
			},
			inputRPC: func(_ *testing.T) *pubsub_pb.PartialMessagesExtension {
				return &pubsub_pb.PartialMessagesExtension{
					PartsMetadata: []byte{0x01, 0x02},
				}
			},
			wantErrContains: "failed to unmarshal incoming parts metadata",
		},
		{
			name: "incoming parts metadata with zero-length availability returns error",
			inputPeerState: func() blocks.PartialDataColumnPeerState {
				return blocks.PartialDataColumnPeerState{}
			},
			inputRPC: func(t *testing.T) *pubsub_pb.PartialMessagesExtension {
				return &pubsub_pb.PartialMessagesExtension{
					PartsMetadata: mustMarshalPartsMetadata(t, testPartsMetadata(0, nil, nil)),
				}
			},
			wantErrContains: "incoming parts metadata has 0 length availability",
		},
		{
			name: "incoming parts metadata merge length mismatch returns wrapped error",
			inputPeerState: func() blocks.PartialDataColumnPeerState {
				return blocks.PartialDataColumnPeerState{
					Recvd: testPartsMetadataCustom(3, []uint64{0}, 3, []uint64{0}),
				}
			},
			inputRPC: func(t *testing.T) *pubsub_pb.PartialMessagesExtension {
				return &pubsub_pb.PartialMessagesExtension{
					PartsMetadata: mustMarshalPartsMetadata(t, testPartsMetadataCustom(3, []uint64{1}, 4, []uint64{1})),
				}
			},
			wantErrContains: "failed to merge available cells into recvdState parts metadata",
		},
		{
			name: "invalid partial message bytes return error",
			inputPeerState: func() blocks.PartialDataColumnPeerState {
				return blocks.PartialDataColumnPeerState{}
			},
			inputRPC: func(_ *testing.T) *pubsub_pb.PartialMessagesExtension {
				return &pubsub_pb.PartialMessagesExtension{
					PartialMessage: []byte{0x01, 0x02},
				}
			},
			wantErrContains: "failed to unmarshal partial message data",
		},
		{
			name: "partial message with non-empty bitmap bytes but zero logical length returns error",
			inputPeerState: func() blocks.PartialDataColumnPeerState {
				return blocks.PartialDataColumnPeerState{}
			},
			inputRPC: func(t *testing.T) *pubsub_pb.PartialMessagesExtension {
				return &pubsub_pb.PartialMessagesExtension{
					PartialMessage: mustMarshalSidecar(t, testBitlist(0)),
				}
			},
			wantErrContains: "length of cells present bitmap is 0",
		},
		{
			name: "partial message sent merge length mismatch returns error",
			inputPeerState: func() blocks.PartialDataColumnPeerState {
				return blocks.PartialDataColumnPeerState{
					Sent: testPartsMetadataCustom(3, []uint64{0}, 4, []uint64{0}),
				}
			},
			inputRPC: func(t *testing.T) *pubsub_pb.PartialMessagesExtension {
				return &pubsub_pb.PartialMessagesExtension{
					PartialMessage: mustMarshalSidecar(t, testBitlist(3, 1)),
				}
			},
			wantErrContains: "requests length mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			peerState := tt.inputPeerState()
			originalPeerState := peerState.Clone()

			nextPeerState, msg, err := updatePeerStateFromIncomingRPC(peerState, tt.inputRPC(t))

			// updatePeerStateFromIncomingRPC must not mutate the input peerState.
			require.DeepEqual(t, originalPeerState.Recvd, peerState.Recvd)
			require.DeepEqual(t, originalPeerState.Sent, peerState.Sent)

			if tt.wantErrContains != "" {
				require.ErrorContains(t, tt.wantErrContains, err)
				return
			}

			require.NoError(t, err)
			if tt.expectedMessage {
				require.NotNil(t, msg)
				assertBitlistEqual(t, msg.CellsPresentBitmap, tt.expectedMesageBitmap)
			} else {
				require.IsNil(t, msg)
			}

			var wantRecvd *ethpb.PartialDataColumnPartsMetadata
			if tt.expectedRecvdState != nil {
				wantRecvd = tt.expectedRecvdState()
			}
			var wantSent *ethpb.PartialDataColumnPartsMetadata
			if tt.expectedSentState != nil {
				wantSent = tt.expectedSentState()
			}
			assertPeerStatePartsMetadata(t, nextPeerState.Recvd, wantRecvd)
			assertPeerStatePartsMetadata(t, nextPeerState.Sent, wantSent)
		})
	}
}

func TestPartialColumnBroadcaster_handleIncomingRPC(t *testing.T) {
	const (
		validTopic   = "/eth2/abcd1234/data_column_sidecar_12/ssz_snappy"
		invalidTopic = "/eth2/abcd1234/not_a_data_column_topic/ssz_snappy"
	)

	type testSetup struct {
		inputRPC                   incomingPartialRPC
		expectedGroupID            string
		expectedHeader             *ethpb.PartialDataColumnHeader
		expectedValidateColumnCall []blocks.CellProofBundle
		expectedCellsValidatedReq  *cellsValidated
	}

	tests := []struct {
		validateHeaderReject     bool
		expectCellsValidatedReq  bool
		expectPeerFeedbackCall   bool
		expectHeaderHandleCall   bool
		expectValidateColumnCall bool
		expectTrustedColumnCall  bool
		expectPublish            bool
		expectHeaderValidateCall bool
		expectedStoreColumn      func(t *testing.T) *blocks.PartialDataColumn
		setup                    func(t *testing.T, b *PartialColumnBroadcaster) testSetup
		expectPeerFeedback       pubsub.PeerFeedbackKind
		expectedErrContains      string
		validateHeaderErr        error
		validateColumnErr        error
		trustedColErr            error
		name                     string
	}{
		{
			name:                "pubsub not initialized returns error",
			expectedErrContains: "pubsub not initialized",
			setup: func(t *testing.T, b *PartialColumnBroadcaster) testSetup {
				b.publishPartialCol = nil
				group := []byte("missing-group")
				return testSetup{
					inputRPC: buildIncomingRPC(validTopic, group, nil, nil),
				}
			},
		},
		{
			name: "missing header for unknown group is ignored",
			setup: func(t *testing.T, _ *PartialColumnBroadcaster) testSetup {
				group := []byte("unknown-group")
				msg := &ethpb.PartialDataColumnSidecar{
					CellsPresentBitmap: testBitlist(2),
				}
				return testSetup{
					inputRPC: buildIncomingRPC(validTopic, group, msg, nil),
				}
			},
		},
		{
			name:                 "header validation reject reports invalid peer feedback",
			validateHeaderErr:    errors.New("bad header"),
			validateHeaderReject: true,
			setup: func(t *testing.T, _ *PartialColumnBroadcaster) testSetup {
				col := createPartialColumn(t, 2, nil)
				group := col.GroupID()
				return testSetup{
					inputRPC:        buildIncomingRPC(validTopic, group, buildHeaderOnlySidecar(col), nil),
					expectedGroupID: string(group),
					expectedHeader:  buildHeaderFromColumn(col),
				}
			},
			expectHeaderValidateCall: true,
			expectPeerFeedbackCall:   true,
			expectPeerFeedback:       pubsub.PeerFeedbackInvalidMessage,
		},
		{
			name:                "group ID mismatch rejects peer and returns error",
			expectedErrContains: "group ID mismatch",
			setup: func(t *testing.T, _ *PartialColumnBroadcaster) testSetup {
				col := createPartialColumn(t, 2, nil)
				wrongGroup := []byte("wrong-group-id")
				return testSetup{
					inputRPC: buildIncomingRPC(validTopic, wrongGroup, buildHeaderOnlySidecar(col), nil),
				}
			},
			expectPeerFeedbackCall: true,
			expectPeerFeedback:     pubsub.PeerFeedbackInvalidMessage,
		},
		{
			name:              "header validation ignore does not report peer feedback",
			validateHeaderErr: errors.New("ignore header"),
			setup: func(t *testing.T, _ *PartialColumnBroadcaster) testSetup {
				col := createPartialColumn(t, 2, nil)
				group := col.GroupID()
				return testSetup{
					inputRPC:        buildIncomingRPC(validTopic, group, buildHeaderOnlySidecar(col), nil),
					expectedGroupID: string(group),
					expectedHeader:  buildHeaderFromColumn(col),
				}
			},
			expectHeaderValidateCall: true,
		},
		{
			name: "header-only message creates and stores partial column and calls header handler",
			setup: func(t *testing.T, _ *PartialColumnBroadcaster) testSetup {
				col := createPartialColumn(t, 3, nil)
				group := col.GroupID()
				return testSetup{
					inputRPC:        buildIncomingRPC(validTopic, group, buildHeaderOnlySidecar(col), nil),
					expectedGroupID: string(group),
					expectedHeader:  buildHeaderFromColumn(col),
				}
			},
			expectHeaderValidateCall: true,
			expectHeaderHandleCall:   true,
			expectedStoreColumn: func(t *testing.T) *blocks.PartialDataColumn {
				return createPartialColumn(t, 3, nil)
			},
		},
		{
			name:                "invalid topic returns error for new group with header",
			expectedErrContains: "could not extract column index from topic",
			setup: func(t *testing.T, _ *PartialColumnBroadcaster) testSetup {
				col := createPartialColumn(t, 2, nil)
				group := col.GroupID()
				return testSetup{
					inputRPC:        buildIncomingRPC(invalidTopic, group, buildHeaderOnlySidecar(col), nil),
					expectedGroupID: string(group),
					expectedHeader:  buildHeaderFromColumn(col),
				}
			},
			expectHeaderHandleCall: false,
		},
		{
			name: "existing column with incoming cells calls validateColumn and enqueues cellsValidated request",
			setup: func(t *testing.T, b *PartialColumnBroadcaster) testSetup {
				existing := createPartialColumn(t, 3, map[uint64][]byte{
					0: {0x11},
				})
				group := existing.GroupID()
				b.partialMsgStore[validTopic] = map[string]*verification.PartialColumnVerifier{
					string(group): newMarkedVerifier(existing),
				}
				msg := buildSidecarWithCells(3, map[uint64][]byte{
					1: {0x22},
				})
				cellIndices, cellsToVerify := buildExpectedCellsToVerify(existing, map[uint64][]byte{
					1: {0x22},
				})
				return testSetup{
					inputRPC:                   buildIncomingRPC(validTopic, group, msg, nil),
					expectedValidateColumnCall: cellsToVerify,
					expectedCellsValidatedReq: &cellsValidated{
						topic:       validTopic,
						group:       slices.Clone(group),
						cellIndices: cellIndices,
						cells:       cellsToVerify,
					},
				}
			},
			expectValidateColumnCall: true,
			expectCellsValidatedReq:  true,
			expectPeerFeedbackCall:   true,
			expectPeerFeedback:       pubsub.PeerFeedbackUsefulMessage,
			expectedStoreColumn: func(t *testing.T) *blocks.PartialDataColumn {
				return createPartialColumn(t, 3, map[uint64][]byte{
					0: {0x11},
				})
			},
		},
		{
			name:              "validateColumn failure reports invalid peer feedback",
			validateColumnErr: errors.New("invalid cells"),
			setup: func(t *testing.T, b *PartialColumnBroadcaster) testSetup {
				existing := createPartialColumn(t, 3, map[uint64][]byte{
					0: {0x33},
				})
				group := existing.GroupID()
				b.partialMsgStore[validTopic] = map[string]*verification.PartialColumnVerifier{
					string(group): newMarkedVerifier(existing),
				}
				msg := buildSidecarWithCells(3, map[uint64][]byte{
					1: {0x44},
				})
				_, cellsToVerify := buildExpectedCellsToVerify(existing, map[uint64][]byte{
					1: {0x44},
				})
				return testSetup{
					inputRPC:                   buildIncomingRPC(validTopic, group, msg, nil),
					expectedValidateColumnCall: cellsToVerify,
				}
			},
			expectValidateColumnCall: true,
			expectPeerFeedbackCall:   true,
			expectPeerFeedback:       pubsub.PeerFeedbackInvalidMessage,
			expectedStoreColumn: func(t *testing.T) *blocks.PartialDataColumn {
				return createPartialColumn(t, 3, map[uint64][]byte{
					0: {0x33},
				})
			},
		},
		{
			name: "parts metadata difference republishes when getBlobs was called",
			setup: func(t *testing.T, b *PartialColumnBroadcaster) testSetup {
				existing := createPartialColumn(t, 3, map[uint64][]byte{
					0: {0x55},
				})
				group := existing.GroupID()
				b.partialMsgStore[validTopic] = map[string]*verification.PartialColumnVerifier{
					string(group): newMarkedVerifier(existing),
				}
				b.partialMsgStore[validTopic][string(group)].Column.Published = true
				return testSetup{
					inputRPC: buildIncomingRPC(validTopic, group, nil, []byte{0x01, 0x02}),
				}
			},
			expectPublish: true,
			expectedStoreColumn: func(t *testing.T) *blocks.PartialDataColumn {
				return createPartialColumn(t, 3, map[uint64][]byte{
					0: {0x55},
				})
			},
		},
		{
			name: "cached header builds verifier via trusted column and processes incoming cells",
			setup: func(t *testing.T, b *PartialColumnBroadcaster) testSetup {
				// Header already validated for this group, e.g. seen earlier on another column's topic.
				col := createPartialColumn(t, 3, nil)
				group := col.GroupID()
				b.validHeaderCache[string(group)] = buildHeaderFromColumn(col)
				msg := buildSidecarWithCells(3, map[uint64][]byte{
					1: {0x22},
				})
				cellIndices, cellsToVerify := buildExpectedCellsToVerify(col, map[uint64][]byte{
					1: {0x22},
				})
				return testSetup{
					inputRPC:                   buildIncomingRPC(validTopic, group, msg, nil),
					expectedValidateColumnCall: cellsToVerify,
					expectedCellsValidatedReq: &cellsValidated{
						topic:       validTopic,
						group:       slices.Clone(group),
						cellIndices: cellIndices,
						cells:       cellsToVerify,
					},
				}
			},
			expectTrustedColumnCall:  true,
			expectValidateColumnCall: true,
			expectCellsValidatedReq:  true,
			expectPeerFeedbackCall:   true,
			expectPeerFeedback:       pubsub.PeerFeedbackUsefulMessage,
			expectedStoreColumn: func(t *testing.T) *blocks.PartialDataColumn {
				return createPartialColumn(t, 3, nil)
			},
		},
		{
			name:                "cached header trusted column error returns error",
			expectedErrContains: "partial verifier from trusted column",
			trustedColErr:       errors.New("trusted boom"),
			setup: func(t *testing.T, b *PartialColumnBroadcaster) testSetup {
				col := createPartialColumn(t, 2, nil)
				group := col.GroupID()
				b.validHeaderCache[string(group)] = buildHeaderFromColumn(col)
				msg := buildSidecarWithCells(2, map[uint64][]byte{
					0: {0x11},
				})
				return testSetup{
					inputRPC: buildIncomingRPC(validTopic, group, msg, nil),
				}
			},
			expectTrustedColumnCall: true,
		},
		{
			name:                "header with nil signed block header rejects peer and returns error",
			expectedErrContains: "header is missing signed block header or header",
			setup: func(t *testing.T, _ *PartialColumnBroadcaster) testSetup {
				col := createPartialColumn(t, 2, nil)
				group := col.GroupID()
				msg := &ethpb.PartialDataColumnSidecar{
					CellsPresentBitmap: testBitlist(2),
					Header: []*ethpb.PartialDataColumnHeader{{
						SignedBlockHeader: nil,
						KzgCommitments:    col.KzgCommitments,
					}},
				}
				return testSetup{
					inputRPC: buildIncomingRPC(validTopic, group, msg, nil),
				}
			},
			expectPeerFeedbackCall: true,
			expectPeerFeedback:     pubsub.PeerFeedbackInvalidMessage,
		},
		{
			name:                "header with nil block header field rejects peer and returns error",
			expectedErrContains: "header is missing signed block header or header",
			setup: func(t *testing.T, _ *PartialColumnBroadcaster) testSetup {
				col := createPartialColumn(t, 2, nil)
				group := col.GroupID()
				msg := &ethpb.PartialDataColumnSidecar{
					CellsPresentBitmap: testBitlist(2),
					Header: []*ethpb.PartialDataColumnHeader{{
						SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{Header: nil, Signature: []byte{1}},
						KzgCommitments:    col.KzgCommitments,
					}},
				}
				return testSetup{
					inputRPC: buildIncomingRPC(validTopic, group, msg, nil),
				}
			},
			expectPeerFeedbackCall: true,
			expectPeerFeedback:     pubsub.PeerFeedbackInvalidMessage,
		},
		{
			name: "no message and no verifier for unknown group is ignored",
			setup: func(t *testing.T, _ *PartialColumnBroadcaster) testSetup {
				group := createPartialColumn(t, 2, nil).GroupID()
				// Only parts metadata, no partial message: nothing to build a verifier from.
				return testSetup{
					inputRPC: buildIncomingRPC(validTopic, group, nil, []byte{0x01, 0x02}),
				}
			},
		},
		{
			name:                "incoming cells with mismatched bitmap length returns error",
			expectedErrContains: "cells to verify from partial message",
			setup: func(t *testing.T, b *PartialColumnBroadcaster) testSetup {
				existing := createPartialColumn(t, 3, map[uint64][]byte{
					0: {0x11},
				})
				group := existing.GroupID()
				b.partialMsgStore[validTopic] = map[string]*verification.PartialColumnVerifier{
					string(group): newMarkedVerifier(existing),
				}
				// Message bitmap length (2) disagrees with our column's commitment count (3).
				msg := buildSidecarWithCells(2, map[uint64][]byte{
					0: {0x22},
				})
				return testSetup{
					inputRPC: buildIncomingRPC(validTopic, group, msg, nil),
				}
			},
			expectedStoreColumn: func(t *testing.T) *blocks.PartialDataColumn {
				return createPartialColumn(t, 3, map[uint64][]byte{
					0: {0x11},
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := newMockPubSub(nil, nil)
			recorder := newCallbackRecorder(8, tt.validateHeaderReject, tt.validateColumnErr, tt.validateHeaderErr)
			recorder.partialVerifierFromTrustedColErr = tt.trustedColErr
			h := newBroadcasterHarness(t, ps)

			h.broadcaster.callbacks = recorder

			setup := tt.setup(t, h.broadcaster)

			h.broadcaster.topics[setup.inputRPC.GetTopicID()] = nil
			err := h.broadcaster.handleIncomingRPC(setup.inputRPC)

			if tt.expectedErrContains != "" {
				require.ErrorContains(t, tt.expectedErrContains, err)
			} else {
				require.NoError(t, err)
			}

			if tt.expectHeaderValidateCall {
				select {
				case call := <-recorder.partialVerifierFromHeaderCallCh:
					require.DeepEqual(t, setup.expectedHeader.KzgCommitments, call.KzgCommitments)
					require.DeepEqual(t, setup.expectedHeader.KzgCommitmentsInclusionProof, call.KzgCommitmentsInclusionProof)
					require.DeepEqual(t, setup.expectedHeader.SignedBlockHeader, call.SignedBlockHeader)
				case <-t.Context().Done():
					t.Fatalf("header validation call not received")
				}
			}

			if tt.expectTrustedColumnCall {
				select {
				case call := <-recorder.partialVerifierFromTrustedColumnCallCh:
					require.NotNil(t, call)
				case <-t.Context().Done():
					t.Fatalf("trusted column call not received")
				}
			}

			if tt.expectHeaderHandleCall {
				select {
				case call := <-recorder.handleHeaderCallCh:
					if setup.expectedHeader != nil {
						require.DeepEqual(t, setup.expectedHeader.KzgCommitments, call.header.KzgCommitments)
						require.DeepEqual(t, setup.expectedHeader.KzgCommitmentsInclusionProof, call.header.KzgCommitmentsInclusionProof)
						require.DeepEqual(t, setup.expectedHeader.SignedBlockHeader, call.header.SignedBlockHeader)
					}
					require.Equal(t, setup.expectedGroupID, call.groupID)
				case <-t.Context().Done():
					t.Fatalf("header handler call not received")
				}
			}

			if tt.expectValidateColumnCall {
				select {
				case call := <-recorder.validateColumnCallCh:
					assertCellProofBundlesEqual(t, setup.expectedValidateColumnCall, call)
				case <-t.Context().Done():
					t.Fatalf("validateColumn call not received")
				}
			}

			if tt.expectCellsValidatedReq {
				select {
				case req := <-h.broadcaster.incomingReq:
					require.Equal(t, requestKindCellsValidated, req.kind)
					require.NotNil(t, req.cellsValidated)
					assertCellsValidatedEqual(t, setup.expectedCellsValidatedReq, req.cellsValidated)
				case <-t.Context().Done():
					t.Fatalf("cells validated request not enqueued")
				}
			}

			if tt.expectPeerFeedbackCall {
				waitForPeerFeedbackCalls(t, ps, 1)
				feedbackCalls := ps.peerFeedbackCallsSnapshot()
				require.Equal(t, tt.expectPeerFeedback, feedbackCalls[0].kind)
				require.Equal(t, setup.inputRPC.from, feedbackCalls[0].peerID)
			} else {
				require.Equal(t, 0, ps.peerFeedbackCallCount())
			}

			stored := h.broadcaster.getDataColumn(setup.inputRPC.GetTopicID(), setup.inputRPC.GroupID)
			if tt.expectPublish {
				require.NotNil(t, stored)
				ps.assertPartialColumnsPublished(t, setup.inputRPC.GetTopicID(), []*blocks.PartialDataColumn{stored})
			} else {
				require.Equal(t, 0, ps.publishedColumnCount())
			}

			if tt.expectedStoreColumn != nil {
				assertPartialColumnsEqual(t, tt.expectedStoreColumn(t), stored)
			}
		})
	}
}

// Regression test for the validator-semaphore deadlock: when every validator slot is in
// flight, handlePartialCells must shed the work rather than block the loop goroutine on the
// semaphore send (a blocking send there deadlocks with the validators that report results
// back via the blocking enqueue). It must also still fall through to republishColumn.
func TestPartialColumnBroadcaster_handleIncomingRPC_dropsValidationWhenSaturated(t *testing.T) {
	const validTopic = "/eth2/abcd1234/data_column_sidecar_12/ssz_snappy"

	ps := newMockPubSub(nil, nil)
	recorder := newCallbackRecorder(8, false, nil, nil)
	h := newBroadcasterHarness(t, ps)
	h.broadcaster.callbacks = recorder
	h.broadcaster.topics[validTopic] = nil

	// Existing, already-published column so republish can fire.
	existing := createPartialColumn(t, 3, map[uint64][]byte{0: {0x11}})
	group := existing.GroupID()
	h.broadcaster.partialMsgStore[validTopic] = map[string]*verification.PartialColumnVerifier{
		string(group): newMarkedVerifier(existing),
	}
	h.broadcaster.partialMsgStore[validTopic][string(group)].Column.Published = true

	// Incoming message carries a new cell (so cellsToVerify is non-empty) and differing parts
	// metadata (so republish should fire even though we drop the validation).
	msg := buildSidecarWithCells(3, map[uint64][]byte{1: {0x22}})
	rpc := buildIncomingRPC(validTopic, group, msg, []byte{0x01, 0x02})

	// Saturate the validator semaphore so the acquire must take the default (drop) path.
	for range cap(h.broadcaster.concurrentValidatorSemaphore) {
		h.broadcaster.concurrentValidatorSemaphore <- struct{}{}
	}

	// Must not block: a blocking send on the saturated semaphore would deadlock the loop.
	done := make(chan error, 1)
	go func() { done <- h.broadcaster.handleIncomingRPC(rpc) }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("handleIncomingRPC blocked on a saturated validator semaphore (deadlock regression)")
	}

	// Validation was shed, not run, and no cellsValidated result was enqueued.
	require.Equal(t, 0, len(recorder.validateColumnCallCh))
	require.Equal(t, 0, len(h.broadcaster.incomingReq))

	// Crucially, we still fell through to republish (unlike returning an error, which skips it).
	ps.assertPartialColumnsPublished(t, validTopic, []*blocks.PartialDataColumn{
		h.broadcaster.getDataColumn(validTopic, group),
	})
}

func TestPartialColumnBroadcaster_handleIncomingRPC_ignoresUnsubscribedTopic(t *testing.T) {
	ps := newMockPubSub(nil, nil)
	recorder := newCallbackRecorder(8, false, nil, nil)
	h := newBroadcasterHarness(t, ps)
	h.broadcaster.callbacks = recorder

	col := createPartialColumn(t, 2, nil)
	group := col.GroupID()
	// In-bounds, well-formed topic, but intentionally NOT registered in
	// h.broadcaster.topics, i.e. we are not subscribed to it.
	const topic = "/eth2/abcd1234/data_column_sidecar_12/ssz_snappy"

	err := h.broadcaster.handleIncomingRPC(buildIncomingRPC(topic, group, buildHeaderOnlySidecar(col), nil))
	require.NoError(t, err)

	require.IsNil(t, h.broadcaster.getDataColumn(topic, group))
	require.Equal(t, 0, ps.publishedColumnCount())
	require.Equal(t, 0, ps.peerFeedbackCallCount())
	require.Equal(t, 0, len(h.broadcaster.incomingReq))
}

func TestPartialColumnBroadcaster_onIncomingRPC_inputValidation(t *testing.T) {
	const from = peer.ID("peer-a")
	validGroup := createPartialColumn(t, 2, nil).GroupID()

	tests := []struct {
		name           string
		topic          string
		group          []byte // defaults to a valid-length group when nil
		expectedErr    string // defaults to "invalid topic ID" when reject and empty
		nilRPC         bool
		expectReject   bool
		expectEnqueued bool
	}{
		{
			name:           "in-bounds topic is accepted and enqueued",
			topic:          "/eth2/abcd1234/data_column_sidecar_0/ssz_snappy",
			expectReject:   false,
			expectEnqueued: true,
		},
		{
			name:           "nil rpc is ignored",
			nilRPC:         true,
			expectReject:   false,
			expectEnqueued: false,
		},
		{
			name:           "invalid group ID length is rejected",
			topic:          "/eth2/abcd1234/data_column_sidecar_0/ssz_snappy",
			group:          []byte("too-short"),
			expectedErr:    "invalid group ID length",
			expectReject:   true,
			expectEnqueued: false,
		},
		{
			name:           "column index at NumberOfColumns is rejected",
			topic:          "/eth2/abcd1234/data_column_sidecar_128/ssz_snappy",
			expectReject:   true,
			expectEnqueued: false,
		},
		{
			name:           "column index far out of bounds is rejected",
			topic:          "/eth2/abcd1234/data_column_sidecar_999999/ssz_snappy",
			expectReject:   true,
			expectEnqueued: false,
		},
		{
			name:           "topic without a column index is rejected",
			topic:          "/eth2/abcd1234/not_a_data_column_topic/ssz_snappy",
			expectReject:   true,
			expectEnqueued: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := newMockPubSub(nil, nil)
			h := newBroadcasterHarness(t, ps)

			var rpc *pubsub_pb.PartialMessagesExtension
			if !tt.nilRPC {
				group := validGroup
				if tt.group != nil {
					group = tt.group
				}
				topic := tt.topic
				rpc = &pubsub_pb.PartialMessagesExtension{
					TopicID: &topic,
					GroupID: slices.Clone(group),
				}
			}
			peerStates := map[peer.ID]blocks.PartialDataColumnPeerState{}

			err := h.broadcaster.onIncomingRPC(from, peerStates, rpc)

			if tt.expectReject {
				wantErr := tt.expectedErr
				if wantErr == "" {
					wantErr = "invalid topic ID"
				}
				require.ErrorContains(t, wantErr, err)
				waitForPeerFeedbackCalls(t, ps, 1)
				feedback := ps.peerFeedbackCallsSnapshot()
				require.Equal(t, 1, len(feedback))
				require.Equal(t, pubsub.PeerFeedbackInvalidMessage, feedback[0].kind)
				require.Equal(t, from, feedback[0].peerID)
				require.Equal(t, tt.topic, feedback[0].topic)
			} else {
				require.NoError(t, err)
				require.Equal(t, 0, ps.peerFeedbackCallCount())
			}

			enqueued := len(h.broadcaster.incomingReq)
			if tt.expectEnqueued {
				require.Equal(t, 1, enqueued)
			} else {
				require.Equal(t, 0, enqueued)
			}
		})
	}
}

func TestPartialColumnBroadcaster_onIncomingRPC_dropsWhenQueueFull(t *testing.T) {
	const from = peer.ID("peer-a")
	topic := "/eth2/abcd1234/data_column_sidecar_0/ssz_snappy"

	ps := newMockPubSub(nil, nil)
	h := newBroadcasterHarness(t, ps)
	fillRequestQueue(h.broadcaster)

	rpc := &pubsub_pb.PartialMessagesExtension{
		TopicID:       &topic,
		GroupID:       slices.Clone(createPartialColumn(t, 2, nil).GroupID()),
		PartsMetadata: mustMarshalPartsMetadata(t, testPartsMetadata(2, []uint64{0}, nil)),
	}
	peerStates := map[peer.ID]blocks.PartialDataColumnPeerState{}

	err := h.broadcaster.onIncomingRPC(from, peerStates, rpc)
	require.ErrorContains(t, "incomingReq channel is full", err)
	// The peer state update is discarded along with the dropped RPC.
	require.Equal(t, 0, len(peerStates))
	require.Equal(t, 0, ps.peerFeedbackCallCount())
}

func TestPartialColumnBroadcaster_handleCellsValidated(t *testing.T) {
	const topic = "/eth2/abcd1234/data_column_sidecar_12/ssz_snappy"

	type testSetup struct {
		column    *blocks.PartialDataColumn
		group     []byte
		published bool
	}

	tests := []struct {
		wantErrContains  string
		name             string
		publishErr       error
		validFieldsErr   error
		setup            func(t *testing.T) testSetup
		validatedCells   map[uint64][]byte
		wrongColumnIndex bool
		expectPublish    bool
		expectHandle     bool
		expectedStoreCol func(t *testing.T) *blocks.PartialDataColumn
	}{
		{
			name: "missing data column returns error",
			setup: func(_ *testing.T) testSetup {
				return testSetup{
					group: []byte("missing-group"),
				}
			},
			validatedCells:  map[uint64][]byte{0: {0xA0}},
			wantErrContains: "data column not found for verified cells",
		},
		{
			name: "cell bundle with wrong column index returns error",
			setup: func(t *testing.T) testSetup {
				c := createPartialColumn(t, 3, map[uint64][]byte{
					0: {0x70},
				})
				return testSetup{
					column:    c,
					group:     c.GroupID(),
					published: true,
				}
			},
			validatedCells: map[uint64][]byte{
				1: {0x71},
			},
			wrongColumnIndex: true,
			wantErrContains:  "cell bundle has wrong column index",
			expectedStoreCol: func(t *testing.T) *blocks.PartialDataColumn {
				return createPartialColumn(t, 3, map[uint64][]byte{
					0: {0x70},
				})
			},
		},
		{
			name: "duplicate validated cells do not extend and do not publish",
			setup: func(t *testing.T) testSetup {
				c := createPartialColumn(t, 3, map[uint64][]byte{
					0: {0x10},
				})
				return testSetup{
					column:    c,
					group:     c.GroupID(),
					published: true,
				}
			},
			validatedCells: map[uint64][]byte{
				0: {0x10},
			},
			expectedStoreCol: func(t *testing.T) *blocks.PartialDataColumn {
				return createPartialColumn(t, 3, map[uint64][]byte{
					0: {0x10},
				})
			},
		},
		{
			name: "extends incomplete column and skips publish when getBlobs not called",
			setup: func(t *testing.T) testSetup {
				c := createPartialColumn(t, 4, map[uint64][]byte{
					0: {0x20},
				})
				return testSetup{
					column:    c,
					group:     c.GroupID(),
					published: false,
				}
			},
			validatedCells: map[uint64][]byte{
				2: {0xC2},
			},
			expectedStoreCol: func(t *testing.T) *blocks.PartialDataColumn {
				return createPartialColumn(t, 4, map[uint64][]byte{
					0: {0x20},
					2: {0xC2},
				})
			},
		},
		{
			name: "extends incomplete column and publishes when getBlobs called",
			setup: func(t *testing.T) testSetup {
				c := createPartialColumn(t, 4, map[uint64][]byte{
					0: {0x30},
				})
				return testSetup{
					column:    c,
					group:     c.GroupID(),
					published: true,
				}
			},
			validatedCells: map[uint64][]byte{
				2: {0xD2},
			},
			expectPublish: true,
			expectedStoreCol: func(t *testing.T) *blocks.PartialDataColumn {
				return createPartialColumn(t, 4, map[uint64][]byte{
					0: {0x30},
					2: {0xD2},
				})
			},
		},
		{
			name:       "publish error is returned when extension triggers republish",
			publishErr: errors.New("publish failed"),
			setup: func(t *testing.T) testSetup {
				c := createPartialColumn(t, 4, map[uint64][]byte{
					0: {0x40},
				})
				return testSetup{
					column:    c,
					group:     c.GroupID(),
					published: true,
				}
			},
			validatedCells: map[uint64][]byte{
				1: {0xE1},
			},
			expectPublish:   true,
			wantErrContains: "publish failed",
			expectedStoreCol: func(t *testing.T) *blocks.PartialDataColumn {
				return createPartialColumn(t, 4, map[uint64][]byte{
					0: {0x40},
					1: {0xE1},
				})
			},
		},
		{
			name: "extends to complete and invokes handleColumn without publish when getBlobs not called",
			setup: func(t *testing.T) testSetup {
				c := createPartialColumn(t, 2, map[uint64][]byte{
					0: {0x50},
				})
				return testSetup{
					column:    c,
					group:     c.GroupID(),
					published: false,
				}
			},
			validatedCells: map[uint64][]byte{
				1: {0xF1},
			},
			expectHandle: true,
			expectedStoreCol: func(t *testing.T) *blocks.PartialDataColumn {
				return createPartialColumn(t, 2, map[uint64][]byte{
					0: {0x50},
					1: {0xF1},
				})
			},
		},
		{
			name: "extends to complete, invokes handleColumn, and publishes when getBlobs called",
			setup: func(t *testing.T) testSetup {
				c := createPartialColumn(t, 2, map[uint64][]byte{
					0: {0x60},
				})
				return testSetup{
					column:    c,
					group:     c.GroupID(),
					published: true,
				}
			},
			validatedCells: map[uint64][]byte{
				1: {0xA1},
			},
			expectPublish: true,
			expectHandle:  true,
			expectedStoreCol: func(t *testing.T) *blocks.PartialDataColumn {
				return createPartialColumn(t, 2, map[uint64][]byte{
					0: {0x60},
					1: {0xA1},
				})
			},
		},
		{
			// The final cell completes the column, so Complete() runs the failing ValidFields check.
			name:           "complete error is returned when ValidFields fails",
			validFieldsErr: errors.New("invalid fields"),
			setup: func(t *testing.T) testSetup {
				c := createPartialColumn(t, 2, map[uint64][]byte{
					0: {0x80},
				})
				return testSetup{
					column:    c,
					group:     c.GroupID(),
					published: true,
				}
			},
			validatedCells: map[uint64][]byte{
				1: {0x81},
			},
			wantErrContains: "complete partial column verifier",
			expectedStoreCol: func(t *testing.T) *blocks.PartialDataColumn {
				// The cell was extended before Complete() failed, so it stays stored.
				return createPartialColumn(t, 2, map[uint64][]byte{
					0: {0x80},
					1: {0x81},
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := newMockPubSub(tt.publishErr, nil)
			recorder := newCallbackRecorder(2, false, nil, nil)
			h := newBroadcasterHarness(t, ps)

			setup := tt.setup(t)
			if setup.column != nil {
				verifier := newMarkedVerifier(setup.column)
				if tt.validFieldsErr != nil {
					verifier = newMockPartialVerifierWithValidFieldsErr(setup.column, tt.validFieldsErr)
				}
				h.broadcaster.partialMsgStore[topic] = map[string]*verification.PartialColumnVerifier{
					string(setup.group): verifier,
				}
				h.broadcaster.partialMsgStore[topic][string(setup.group)].Column.Published = setup.published
			}
			h.broadcaster.callbacks = recorder

			var cellIndices []uint64
			var cells []blocks.CellProofBundle
			if setup.column != nil {
				columnIndex := setup.column.Index
				if tt.wrongColumnIndex {
					columnIndex = setup.column.Index + 1
				}
				cellIndices, cells = buildValidatedCells(columnIndex, tt.validatedCells)
			} else {
				cellIndices, cells = buildValidatedCells(12, tt.validatedCells)
			}
			err := h.broadcaster.handleCellsValidated(&cellsValidated{
				validationTook: 5,
				topic:          topic,
				group:          setup.group,
				cellIndices:    cellIndices,
				cells:          cells,
			})

			if tt.wantErrContains != "" {
				require.ErrorContains(t, tt.wantErrContains, err)
			} else {
				require.NoError(t, err)
			}

			stored := h.broadcaster.getDataColumn(topic, setup.group)
			if tt.expectedStoreCol != nil {
				assertPartialColumnsEqual(t, tt.expectedStoreCol(t), stored)
			} else {
				require.IsNil(t, stored)
			}

			if tt.expectPublish {
				ps.assertPartialColumnsPublished(t, topic, []*blocks.PartialDataColumn{stored})

			} else {
				require.Equal(t, 0, ps.publishedColumnCount())
			}

			if tt.expectHandle {
				select {
				case call := <-recorder.handleColumnCallCh:
					require.Equal(t, topic, call.topic)
					require.Equal(t, true, len(call.column.Column()) > 0)
				case <-t.Context().Done():
					t.Fatalf("handle column call not received")
				}
			}
		})
	}
}

func TestPartialColumnBroadcaster_loopDispatchesIncomingRPCAndCellsValidated(t *testing.T) {
	const topic = "/eth2/abcd1234/data_column_sidecar_12/ssz_snappy"

	ps := newMockPubSub(nil, nil)
	recorder := newCallbackRecorder(8, false, nil, nil)
	h := newBroadcasterHarness(t, ps)
	h.broadcaster.topics[topic] = nil

	// Existing column missing only cell 1, so the incoming validated cell completes it.
	existing := createPartialColumn(t, 2, map[uint64][]byte{0: {0x11}})
	group := existing.GroupID()
	h.broadcaster.partialMsgStore[topic] = map[string]*verification.PartialColumnVerifier{
		string(group): newMarkedVerifier(existing),
	}

	h.start(recorder)
	defer h.Stop()

	msg := buildSidecarWithCells(2, map[uint64][]byte{1: {0x22}})
	req, err := h.broadcaster.enqueue(t.Context(), requestKindHandleIncomingRPC, requestValues{
		incomingRPC: buildIncomingRPC(topic, group, msg, nil),
	})
	require.NoError(t, err)
	require.NoError(t, recvResponse(t, req))

	// HandleColumn firing proves the loop dispatched the cellsValidated request to completion.
	select {
	case call := <-recorder.handleColumnCallCh:
		require.Equal(t, topic, call.topic)
	case <-t.Context().Done():
		t.Fatalf("handle column call not received")
	}

	assertPartialColumnsEqual(t,
		createPartialColumn(t, 2, map[uint64][]byte{0: {0x11}, 1: {0x22}}),
		h.broadcaster.getDataColumn(topic, group))
}

func TestPartialColumnBroadcaster_Publish(t *testing.T) {
	const topic = "/eth2/abcd1234/data_column_sidecar_12/ssz_snappy"
	pc := func(nCells uint64, cells map[uint64][]byte) func(t *testing.T) *blocks.PartialDataColumn {
		return func(t *testing.T) *blocks.PartialDataColumn {
			return createPartialColumn(t, nCells, cells)
		}
	}
	column1 := pc(3, map[uint64][]byte{
		0: {0x10},
	})

	tests := []struct {
		expectTrustedCall   bool
		expectedErrContains string
		publishErr          error
		trustedColErr       error
		name                string
		existingColumn      func(t *testing.T) *blocks.PartialDataColumn
		publishColumn       func(t *testing.T) *blocks.PartialDataColumn
		expectedStoreColumn func(t *testing.T) *blocks.PartialDataColumn
	}{
		{
			name:                "new group stores and publishes",
			expectTrustedCall:   true,
			publishColumn:       column1,
			expectedStoreColumn: column1,
		},
		{
			name:                "publish error is returned to caller",
			expectTrustedCall:   true,
			publishErr:          errors.New("publish failed"),
			expectedErrContains: "publish failed",
			publishColumn:       column1,
			expectedStoreColumn: column1,
		},
		{
			name: "existing duplicate cells are not overwritten",
			existingColumn: pc(3, map[uint64][]byte{
				0: {0x20},
			}),
			publishColumn: pc(3, map[uint64][]byte{
				1: {0x90},
			}),
			expectedStoreColumn: pc(3, map[uint64][]byte{
				0: {0x20},
				1: {0x90},
			}),
		},
		{
			name: "existing extends with new cells and remains incomplete",
			existingColumn: pc(4, map[uint64][]byte{
				0: {0x30},
			}),
			publishColumn: pc(4, map[uint64][]byte{
				1: {0xA0},
				2: {0xA1},
			}),
			expectedStoreColumn: pc(4, map[uint64][]byte{
				0: {0x30},
				1: {0xA0},
				2: {0xA1},
			}),
		},
		{
			name: "existing extends to complete and invokes handleColumn",
			existingColumn: pc(2, map[uint64][]byte{
				0: {0x40},
			}),
			publishColumn: pc(2, map[uint64][]byte{
				1: {0xB1},
			}),
			expectedStoreColumn: pc(2, map[uint64][]byte{
				0: {0x40},
				1: {0xB1},
			}),
		},
		{
			name:          "column with no KZG commitments is skipped",
			publishColumn: pc(0, nil),
		},
		{
			name:                "trusted verifier error is aggregated and returned",
			publishColumn:       column1,
			trustedColErr:       errors.New("trusted boom"),
			expectedErrContains: "partial verifier from trusted column",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			column := tt.publishColumn(t)
			groupID := string(column.GroupID())

			ps := newMockPubSub(tt.publishErr, nil)
			recorder := newCallbackRecorder(1, false, nil, nil)
			recorder.partialVerifierFromTrustedColErr = tt.trustedColErr

			h := newBroadcasterHarness(t, ps)
			if tt.existingColumn != nil {
				existing := tt.existingColumn(t)
				h.broadcaster.partialMsgStore[topic] = map[string]*verification.PartialColumnVerifier{
					groupID: newMarkedVerifier(existing),
				}
			}

			h.start(recorder)
			defer h.Stop()

			err := h.broadcaster.Publish(t.Context(), func(yield func(string, blocks.PartialDataColumn) bool) {
				yield(topic, *column)
			})
			if tt.expectedErrContains != "" {
				require.ErrorContains(t, tt.expectedErrContains, err)
			} else {
				require.NoError(t, err)
			}

			stored := h.broadcaster.getDataColumn(topic, column.GroupID())
			if tt.expectedStoreColumn == nil {
				// Column was skipped or never stored (no commitments / verifier error).
				require.IsNil(t, stored)
				require.Equal(t, 0, ps.publishedColumnCount())
				return
			}
			expectedStored := tt.expectedStoreColumn(t)
			assertPartialColumnsEqual(t, expectedStored, stored)
			ps.assertPartialColumnsPublished(t, topic, []*blocks.PartialDataColumn{expectedStored})

			// Published is only set to true if err == nil
			require.Equal(t, err == nil, stored.Published)

			if tt.expectTrustedCall {
				select {
				case call := <-recorder.partialVerifierFromTrustedColumnCallCh:
					assertPartialColumnsEqual(t, column, call)
				case <-t.Context().Done():
					t.Fatalf("trusted partial verifier call not received")
				}
			}

		})
	}
}

func newTestTopic(t *testing.T, name string) *pubsub.Topic {
	t.Helper()
	h, err := libp2p.New(libp2p.NoListenAddrs)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })

	ps, err := pubsub.NewFloodSub(t.Context(), h)
	require.NoError(t, err)

	topic, err := ps.Join(name)
	require.NoError(t, err)

	return topic
}

func TestPartialColumnBroadcaster_Subscribe(t *testing.T) {
	const topicName = "/eth2/abcd1234/data_column_sidecar_1/ssz_snappy"

	tests := []struct {
		name             string
		preExistingTopic bool
		expectedErr      string
	}{
		{
			name: "new topic subscribes successfully",
		},
		{
			name:             "already subscribed topic returns error",
			preExistingTopic: true,
			expectedErr:      "already subscribed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			topic := newTestTopic(t, topicName)

			ps := newMockPubSub(nil, nil)
			recorder := newCallbackRecorder(1, false, nil, nil)
			h := newBroadcasterHarness(t, ps)

			if tt.preExistingTopic {
				h.broadcaster.topics[topicName] = topic
			}

			h.start(recorder)
			defer h.Stop()

			err := h.broadcaster.Subscribe(t.Context(), topic)
			if tt.expectedErr != "" {
				require.ErrorContains(t, tt.expectedErr, err)
			} else {
				require.NoError(t, err)
				stored, ok := h.broadcaster.topics[topicName]
				require.Equal(t, true, ok)
				require.Equal(t, topic, stored)
			}
		})
	}
}

func TestPartialColumnBroadcaster_Unsubscribe(t *testing.T) {
	const topicName = "/eth2/abcd1234/data_column_sidecar_1/ssz_snappy"

	tests := []struct {
		name              string
		setupTopic        bool
		setupPartialStore bool
		expectedErr       string
	}{
		{
			name:              "succeeds and cleans up topic and partial message store",
			setupTopic:        true,
			setupPartialStore: true,
		},
		{
			name:        "unknown topic returns error",
			expectedErr: "topic not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := newMockPubSub(nil, nil)
			recorder := newCallbackRecorder(1, false, nil, nil)
			h := newBroadcasterHarness(t, ps)

			if tt.setupTopic {
				topic := newTestTopic(t, topicName)
				h.broadcaster.topics[topicName] = topic
			}
			if tt.setupPartialStore {
				h.broadcaster.partialMsgStore[topicName] = map[string]*verification.PartialColumnVerifier{
					"group1": verification.NewPartialColumnVerifier(
						&verification.MockDataColumnsVerifier{},
						createPartialColumn(t, 2, map[uint64][]byte{0: {0x10}}),
					),
				}
			}

			// Assert state exists before unsubscribe.
			if tt.setupTopic {
				_, ok := h.broadcaster.topics[topicName]
				require.Equal(t, true, ok)
			}
			if tt.setupPartialStore {
				_, ok := h.broadcaster.partialMsgStore[topicName]
				require.Equal(t, true, ok)
			}

			h.start(recorder)
			defer h.Stop()

			err := h.broadcaster.Unsubscribe(t.Context(), topicName)
			if tt.expectedErr != "" {
				require.ErrorContains(t, tt.expectedErr, err)
			} else {
				require.NoError(t, err)
				_, topicExists := h.broadcaster.topics[topicName]
				require.Equal(t, false, topicExists)
				_, storeExists := h.broadcaster.partialMsgStore[topicName]
				require.Equal(t, false, storeExists)
			}
		})
	}
}

func TestPartialColumnBroadcaster_reportPeerFeedbackAsync(t *testing.T) {
	const (
		topic = "/eth2/abcd1234/data_column_sidecar_3/ssz_snappy"
		from  = peer.ID("peer-x")
	)

	tests := []struct {
		name       string
		saturate   bool
		expectCall bool
	}{
		{
			name:       "delivers feedback when semaphore has capacity",
			expectCall: true,
		},
		{
			name:     "drops feedback when semaphore is saturated",
			saturate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := newMockPubSub(nil, nil)
			h := newBroadcasterHarness(t, ps)

			if tt.saturate {
				for range cap(h.broadcaster.peerFeedbackSemaphore) {
					h.broadcaster.peerFeedbackSemaphore <- struct{}{}
				}
			}

			h.broadcaster.reportPeerFeedbackAsync(topic, from, pubsub.PeerFeedbackInvalidMessage)

			if tt.expectCall {
				require.Eventually(t, func() bool {
					return ps.peerFeedbackCallCount() == 1
				}, 2*time.Second, 5*time.Millisecond)
				calls := ps.peerFeedbackCallsSnapshot()
				require.Equal(t, pubsub.PeerFeedbackInvalidMessage, calls[0].kind)
				require.Equal(t, from, calls[0].peerID)
				require.Equal(t, topic, calls[0].topic)
			} else {
				// The drop path is synchronous, so no feedback is ever sent.
				require.Equal(t, 0, ps.peerFeedbackCallCount())
			}
		})
	}
}

func TestPartialColumnBroadcaster_handleHeader(t *testing.T) {
	const topic = "/eth2/abcd1234/data_column_sidecar_12/ssz_snappy"

	tests := []struct {
		name       string
		saturate   bool
		expectCall bool
	}{
		{
			name:       "caches header and invokes handler",
			expectCall: true,
		},
		{
			name:     "caches header but drops handler when saturated",
			saturate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := newMockPubSub(nil, nil)
			recorder := newCallbackRecorder(1, false, nil, nil)
			h := newBroadcasterHarness(t, ps)
			h.broadcaster.callbacks = recorder

			col := createPartialColumn(t, 2, nil)
			header := buildHeaderFromColumn(col)
			rpc := buildIncomingRPC(topic, col.GroupID(), nil, nil)

			if tt.saturate {
				for range cap(h.broadcaster.concurrentHeaderHandlerSemaphore) {
					h.broadcaster.concurrentHeaderHandlerSemaphore <- struct{}{}
				}
			}

			h.broadcaster.handleHeader(rpc, header)

			// The header is cached synchronously regardless of handler dispatch.
			cached, ok := h.broadcaster.validHeaderCache[string(col.GroupID())]
			require.Equal(t, true, ok)
			require.Equal(t, true, header == cached)

			if tt.expectCall {
				require.Eventually(t, func() bool {
					return len(recorder.handleHeaderCallCh) == 1
				}, 2*time.Second, 5*time.Millisecond)
				call := <-recorder.handleHeaderCallCh
				require.Equal(t, string(col.GroupID()), call.groupID)
				require.DeepEqual(t, header.KzgCommitments, call.header.KzgCommitments)
			} else {
				// The drop path is synchronous, so the handler is never invoked.
				require.Equal(t, 0, len(recorder.handleHeaderCallCh))
			}
		})
	}
}

func TestPartialColumnBroadcaster_gossip(t *testing.T) {
	const topic = "/eth2/abcd1234/data_column_sidecar_12/ssz_snappy"

	tests := []struct {
		name string
		// setup primes the broadcaster and returns the group ID to gossip.
		setup         func(t *testing.T, b *PartialColumnBroadcaster) []byte
		expectPublish bool
	}{
		{
			name: "unknown topic is a no-op",
			setup: func(t *testing.T, _ *PartialColumnBroadcaster) []byte {
				return createPartialColumn(t, 2, nil).GroupID()
			},
		},
		{
			name: "unknown group is a no-op",
			setup: func(_ *testing.T, b *PartialColumnBroadcaster) []byte {
				b.partialMsgStore[topic] = map[string]*verification.PartialColumnVerifier{}
				return []byte("missing-group")
			},
		},
		{
			name: "column with no included cells is a no-op",
			setup: func(t *testing.T, b *PartialColumnBroadcaster) []byte {
				col := createPartialColumn(t, 2, nil)
				col.Published = true
				b.partialMsgStore[topic] = map[string]*verification.PartialColumnVerifier{
					string(col.GroupID()): newMarkedVerifier(col),
				}
				return col.GroupID()
			},
		},
		{
			name: "unpublished column is a no-op",
			setup: func(t *testing.T, b *PartialColumnBroadcaster) []byte {
				col := createPartialColumn(t, 2, map[uint64][]byte{0: {0x10}})
				col.Published = false
				b.partialMsgStore[topic] = map[string]*verification.PartialColumnVerifier{
					string(col.GroupID()): newMarkedVerifier(col),
				}
				return col.GroupID()
			},
		},
		{
			name: "published column with cells is gossiped",
			setup: func(t *testing.T, b *PartialColumnBroadcaster) []byte {
				col := createPartialColumn(t, 2, map[uint64][]byte{0: {0x10}})
				col.Published = true
				b.partialMsgStore[topic] = map[string]*verification.PartialColumnVerifier{
					string(col.GroupID()): newMarkedVerifier(col),
				}
				return col.GroupID()
			},
			expectPublish: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := newMockPubSub(nil, nil)
			h := newBroadcasterHarness(t, ps)

			group := tt.setup(t, h.broadcaster)

			h.broadcaster.gossip(topic, group)

			if tt.expectPublish {
				ps.assertPartialColumnsPublished(t, topic, []*blocks.PartialDataColumn{
					h.broadcaster.getDataColumn(topic, group),
				})
			} else {
				require.Equal(t, 0, ps.publishedColumnCount())
			}
		})
	}
}

// recvResponse blocks until the request's response channel yields, returning the error.
func recvResponse(t *testing.T, req request) error {
	t.Helper()
	var got error
	received := false
	require.Eventually(t, func() bool {
		if received {
			return true
		}
		select {
		case err := <-req.response:
			got = err
			received = true
			return true
		default:
			return false
		}
	}, 2*time.Second, 5*time.Millisecond)
	return got
}

func TestPartialColumnBroadcaster_loopRequestHandling(t *testing.T) {
	const topic = "/eth2/abcd1234/data_column_sidecar_12/ssz_snappy"

	tests := []struct {
		name            string
		kind            requestKind
		reqCtxCancelled bool
		expectedErr     string
	}{
		{
			name:            "request with cancelled context is skipped",
			kind:            requestKindPublish,
			reqCtxCancelled: true,
			expectedErr:     "context canceled",
		},
		{
			name:        "unknown request kind returns error",
			kind:        requestKind(255),
			expectedErr: "unknown request kind",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := newMockPubSub(nil, nil)
			recorder := newCallbackRecorder(1, false, nil, nil)
			h := newBroadcasterHarness(t, ps)

			reqCtx := context.Background()
			if tt.reqCtxCancelled {
				cctx, ccancel := context.WithCancel(context.Background())
				ccancel()
				reqCtx = cctx
			}

			// A publish that would store and publish a column if the loop processed it.
			col := createPartialColumn(t, 2, map[uint64][]byte{0: {0x10}})
			req := newRequest(reqCtx, tt.kind, requestValues{
				publish: publish{
					topicsAndColumns: func(yield func(string, blocks.PartialDataColumn) bool) {
						yield(topic, *col)
					},
				},
			})
			h.broadcaster.incomingReq <- req

			h.start(recorder)
			defer h.Stop()

			err := recvResponse(t, req)
			require.ErrorContains(t, tt.expectedErr, err)
			require.Equal(t, 0, ps.publishedColumnCount())
		})
	}
}

func TestPartialColumnBroadcaster_loopDrainsOnShutdown(t *testing.T) {
	ps := newMockPubSub(nil, nil)
	recorder := newCallbackRecorder(1, false, nil, nil)
	h := newBroadcasterHarness(t, ps)
	h.broadcaster.callbacks = recorder

	// Enough buffered requests that the shutdown drain is overwhelmingly likely to observe some.
	const n = 500
	reqs := make([]request, 0, n)
	for range n {
		req := newRequest(context.Background(), requestKindGossip, requestValues{
			gossip: gossip{topic: "unsubscribed-topic", groupID: []byte("g")},
		})
		h.broadcaster.incomingReq <- req
		reqs = append(reqs, req)
	}

	// Cancel before starting the loop so buffered requests hit the shutdown drain.
	h.Stop()
	go h.broadcaster.loop()

	got := make([]bool, n)
	errs := make([]error, n)
	require.Eventually(t, func() bool {
		for i := range reqs {
			if got[i] {
				continue
			}
			select {
			case err := <-reqs[i].response:
				errs[i] = err
				got[i] = true
			default:
			}
		}
		for i := range got {
			if !got[i] {
				return false
			}
		}
		return true
	}, 5*time.Second, 5*time.Millisecond)

	stopped := 0
	for _, err := range errs {
		if errors.Is(err, errPartialBroadcasterStopped) {
			stopped++
		}
	}
	require.Equal(t, true, stopped > 0)
}

func TestPartialColumnBroadcaster_evictExpiredGroups(t *testing.T) {
	const topic = "/eth2/abcd1234/data_column_sidecar_12/ssz_snappy"

	t.Run("decrements positive ttl and keeps state", func(t *testing.T) {
		ps := newMockPubSub(nil, nil)
		b := newBroadcasterHarness(t, ps).broadcaster
		const group = "group-live"
		b.groupTTL[group] = 2
		b.validHeaderCache[group] = &ethpb.PartialDataColumnHeader{}
		b.headerSentCache[group] = map[peer.ID]bool{}
		b.partialMsgStore[topic] = map[string]*verification.PartialColumnVerifier{
			group: newMarkedVerifier(createPartialColumn(t, 2, map[uint64][]byte{0: {0x10}})),
		}

		b.evictExpiredGroups()

		require.Equal(t, int8(1), b.groupTTL[group])
		_, headerOK := b.validHeaderCache[group]
		require.Equal(t, true, headerOK)
		_, sentOK := b.headerSentCache[group]
		require.Equal(t, true, sentOK)
		_, storeOK := b.partialMsgStore[topic][group]
		require.Equal(t, true, storeOK)
	})

	t.Run("evicts expired group and cleans all caches and empty topic", func(t *testing.T) {
		ps := newMockPubSub(nil, nil)
		b := newBroadcasterHarness(t, ps).broadcaster
		const group = "group-expired"
		b.groupTTL[group] = 0
		b.validHeaderCache[group] = &ethpb.PartialDataColumnHeader{}
		b.headerSentCache[group] = map[peer.ID]bool{}
		b.partialMsgStore[topic] = map[string]*verification.PartialColumnVerifier{
			group: newMarkedVerifier(createPartialColumn(t, 2, map[uint64][]byte{0: {0x10}})),
		}

		b.evictExpiredGroups()

		_, ttlOK := b.groupTTL[group]
		require.Equal(t, false, ttlOK)
		_, headerOK := b.validHeaderCache[group]
		require.Equal(t, false, headerOK)
		_, sentOK := b.headerSentCache[group]
		require.Equal(t, false, sentOK)
		// The topic's only group was removed, so the topic entry is dropped too.
		_, topicOK := b.partialMsgStore[topic]
		require.Equal(t, false, topicOK)
	})

	t.Run("evicting one group leaves other groups on the same topic", func(t *testing.T) {
		ps := newMockPubSub(nil, nil)
		b := newBroadcasterHarness(t, ps).broadcaster
		const live, expired = "group-live", "group-expired"
		b.groupTTL[live] = 1
		b.groupTTL[expired] = 0
		b.partialMsgStore[topic] = map[string]*verification.PartialColumnVerifier{
			live:    newMarkedVerifier(createPartialColumn(t, 2, map[uint64][]byte{0: {0x10}})),
			expired: newMarkedVerifier(createPartialColumn(t, 2, map[uint64][]byte{0: {0x20}})),
		}

		b.evictExpiredGroups()

		_, expiredOK := b.partialMsgStore[topic][expired]
		require.Equal(t, false, expiredOK)
		_, liveOK := b.partialMsgStore[topic][live]
		require.Equal(t, true, liveOK)
		// The topic remains because the live group is still tracked.
		_, topicOK := b.partialMsgStore[topic]
		require.Equal(t, true, topicOK)
	})
}

// fillRequestQueue saturates the request channel so enqueue can no longer accept requests.
func fillRequestQueue(b *PartialColumnBroadcaster) {
	for range cap(b.incomingReq) {
		b.incomingReq <- request{}
	}
}

func TestPartialColumnBroadcaster_requestEnqueueStopped(t *testing.T) {
	const topicName = "/eth2/abcd1234/data_column_sidecar_1/ssz_snappy"
	col := createPartialColumn(t, 2, map[uint64][]byte{0: {0x10}})

	tests := []struct {
		name string
		call func(ctx context.Context, b *PartialColumnBroadcaster, topic *pubsub.Topic) error
	}{
		{
			name: "Publish",
			call: func(ctx context.Context, b *PartialColumnBroadcaster, topic *pubsub.Topic) error {
				return b.Publish(ctx, func(yield func(string, blocks.PartialDataColumn) bool) {
					yield(topic.String(), *col)
				})
			},
		},
		{
			name: "Subscribe",
			call: func(ctx context.Context, b *PartialColumnBroadcaster, topic *pubsub.Topic) error {
				return b.Subscribe(ctx, topic)
			},
		},
		{
			name: "Unsubscribe",
			call: func(ctx context.Context, b *PartialColumnBroadcaster, topic *pubsub.Topic) error {
				return b.Unsubscribe(ctx, topic.String())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := newMockPubSub(nil, nil)
			h := newBroadcasterHarness(t, ps)
			topic := newTestTopic(t, topicName)

			// With a full queue and a stopped broadcaster, p.ctx.Done() is enqueue's only ready case.
			fillRequestQueue(h.broadcaster)
			h.Stop()

			err := tt.call(context.Background(), h.broadcaster, topic)
			require.ErrorIs(t, err, errPartialBroadcasterStopped)
		})
	}
}

// Verifies AppendPubSubOpts wires the peer feedback and partial publish hooks at pubsub construction.
func TestPartialColumnBroadcaster_AppendPubSubOpts(t *testing.T) {
	host, err := libp2p.New(libp2p.NoListenAddrs)
	require.NoError(t, err)
	t.Cleanup(func() { _ = host.Close() })

	b := NewBroadcaster(t.Context(), logrus.New())
	opts := b.AppendPubSubOpts(nil)
	require.Equal(t, 2, len(opts))

	_, err = pubsub.NewGossipSub(t.Context(), host, opts...)
	require.NoError(t, err)
	require.NotNil(t, b.peerFeedback)
	require.NotNil(t, b.publishPartialCol)
}

func TestPartialColumnBroadcaster_Publish_pubsubNotInitialized(t *testing.T) {
	ps := newMockPubSub(nil, nil)
	h := newBroadcasterHarness(t, ps)
	h.broadcaster.publishPartialCol = nil

	err := h.broadcaster.Publish(context.Background(), func(_ func(string, blocks.PartialDataColumn) bool) {})
	require.ErrorContains(t, "pubsub not initialized", err)
}

// Verifies republishColumn surfaces a PartsMetadata() marshal failure.
func TestPartialColumnBroadcaster_republishColumn_partsMetadataError(t *testing.T) {
	const validTopic = "/eth2/abcd1234/data_column_sidecar_12/ssz_snappy"

	ps := newMockPubSub(nil, nil)
	h := newBroadcasterHarness(t, ps)

	// 40000 commitments -> Available bitlist of 5001 bytes, exceeding the 4096-byte cap.
	col := createPartialColumn(t, 40000, nil)
	col.Published = true
	rpc := buildIncomingRPC(validTopic, col.GroupID(), nil, nil)

	err := h.broadcaster.republishColumn(col, rpc, false)
	require.ErrorContains(t, "parts metadata", err)
	require.Equal(t, 0, ps.publishedColumnCount())
}

// Verifies eager pushes and skipped republishes are aggregated per group and
// flushed as one log line with pretty index ranges.
func TestPartialColumnBroadcaster_flushAggregatedLogs(t *testing.T) {
	var buf bytes.Buffer
	logger := &logrus.Logger{
		Out:       &buf,
		Formatter: &logrus.TextFormatter{DisableTimestamp: true},
		Level:     logrus.DebugLevel,
	}
	b := NewBroadcaster(t.Context(), logger)

	groupID := []byte{0, 1, 2, 3}
	for i := range uint64(4) {
		b.recordEagerPush(groupID, i, "peer-a")
		b.recordRepublishSkip(groupID, i)
	}
	b.recordEagerPush(groupID, 9, "peer-b")
	b.recordRepublishSkip(groupID, 9)

	b.flushAggregatedLogs()

	out := buf.String()
	require.StringContains(t, "Eager pushed partial data columns", out)
	require.StringContains(t, "Columns not published, skipping republish", out)
	require.StringContains(t, "0-3,9", out)
	require.StringContains(t, "peers=2", out)
	require.Equal(t, 0, len(b.eagerPushed))
	require.Equal(t, 0, len(b.republishSkipped))

	// Nothing accumulated, nothing logged.
	buf.Reset()
	b.flushAggregatedLogs()
	require.Equal(t, "", buf.String())
}
