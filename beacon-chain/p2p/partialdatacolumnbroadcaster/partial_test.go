package partialdatacolumnbroadcaster

import (
	"bytes"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	libp2p "github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/peer"
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
}

type callbackRecorder struct {
	validateColumnCallCh chan []blocks.CellProofBundle
	handleHeaderCallCh   chan headerHandlerCall
	validateHeaderCallCh chan *ethpb.PartialDataColumnHeader
	handleColumnCallCh   chan columnHandlerCall

	validateColumnErr    error
	validateHeaderErr    error
	validateHeaderReject bool
}

func newCallbackRecorder(callBuffer int, validateHeaderReject bool, validateColumnErr, validateHeaderErr error) *callbackRecorder {
	return &callbackRecorder{
		validateColumnCallCh: make(chan []blocks.CellProofBundle, callBuffer),
		handleHeaderCallCh:   make(chan headerHandlerCall, callBuffer),
		validateHeaderCallCh: make(chan *ethpb.PartialDataColumnHeader, callBuffer),
		handleColumnCallCh:   make(chan columnHandlerCall, callBuffer),
		validateHeaderReject: validateHeaderReject,
		validateHeaderErr:    validateHeaderErr,
		validateColumnErr:    validateColumnErr,
	}
}

func (r *callbackRecorder) ValidateHeader(header *ethpb.PartialDataColumnHeader) (bool, error) {
	r.validateHeaderCallCh <- header
	return r.validateHeaderReject, r.validateHeaderErr
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

	actual := m.publishedPartialColumns

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
	m.peerFeedbackCalls = append(m.peerFeedbackCalls, peerFeedbackCall{
		peerID: id,
		topic:  topic,
		kind:   kind,
	})
	return m.peerFeedbackErr
}

func (m *mockPubSub) publishPartialCol(topic string, groupID []byte, col *blocks.PartialDataColumn) error {
	m.publishedPartialColumns = append(m.publishedPartialColumns, publishedColumn{col, topic})
	retErr := m.publishPartialMessageErr
	return retErr
}

func newBroadcasterHarness(t *testing.T, ps *mockPubSub) *broadcasterHarness {
	t.Helper()
	broadcaster := NewBroadcaster()
	broadcaster.peerFeedback = ps.peerFeedback
	broadcaster.publishPartialCol = ps.publishPartialCol

	return &broadcasterHarness{
		t:           t,
		broadcaster: broadcaster,
	}
}

func (h *broadcasterHarness) start(cr *callbackRecorder) {
	h.t.Helper()

	err := h.broadcaster.Start(
		cr.ValidateHeader,
		cr.ValidateColumn,
		cr.HandleColumn,
		cr.HandleHeader,
	)
	require.NoError(h.t, err)
}

func (h *broadcasterHarness) Stop() {
	h.t.Helper()
	h.broadcaster.Stop()
}

func createPartialColumn(t *testing.T, nCells uint64, cells map[uint64][]byte) *blocks.PartialDataColumn {
	t.Helper()

	commitments := make([][]byte, nCells)
	for i := range nCells {
		commitments[i] = []byte{byte(i + 1)}
	}

	c, err := blocks.NewPartialDataColumn(
		&ethpb.SignedBeaconBlockHeader{
			Header: &ethpb.BeaconBlockHeader{
				ParentRoot: make([]byte, 32),
				StateRoot:  make([]byte, 32),
				BodyRoot:   make([]byte, 32),
			},
			Signature: []byte{1},
		},
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

func buildIncomingRPC(topic string, group []byte, message *ethpb.PartialDataColumnSidecar, partsMetadata []byte) rpcWithFrom {
	topicCopy := topic
	return rpcWithFrom{
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
		if len(ps.peerFeedbackCalls) >= expected {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("expected at least %d peer feedback calls, got %d", expected, len(ps.peerFeedbackCalls))
		case <-ticker.C:
		}
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
			originalPeerState := blocks.ClonePeerState(peerState)

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
		inputRPC                   rpcWithFrom
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
		expectPublish            bool
		expectHeaderValidateCall bool
		expectedStoreColumn      func(t *testing.T) *blocks.PartialDataColumn
		setup                    func(t *testing.T, b *PartialColumnBroadcaster) testSetup
		expectPeerFeedback       pubsub.PeerFeedbackKind
		expectedErrContains      string
		validateHeaderErr        error
		validateColumnErr        error
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
			expectHeaderValidateCall: true,
			expectHeaderHandleCall:   true,
		},
		{
			name: "existing column with incoming cells calls validateColumn and enqueues cellsValidated request",
			setup: func(t *testing.T, b *PartialColumnBroadcaster) testSetup {
				existing := createPartialColumn(t, 3, map[uint64][]byte{
					0: {0x11},
				})
				group := existing.GroupID()
				b.partialMsgStore[validTopic] = map[string]*blocks.PartialDataColumn{
					string(group): existing,
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
				b.partialMsgStore[validTopic] = map[string]*blocks.PartialDataColumn{
					string(group): existing,
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
				b.partialMsgStore[validTopic] = map[string]*blocks.PartialDataColumn{
					string(group): existing,
				}
				b.getBlobsCalled[string(group)] = true
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := newMockPubSub(nil, nil)
			recorder := newCallbackRecorder(8, tt.validateHeaderReject, tt.validateColumnErr, tt.validateHeaderErr)
			h := newBroadcasterHarness(t, ps)

			h.broadcaster.validateHeader = recorder.ValidateHeader
			h.broadcaster.validateColumn = recorder.ValidateColumn
			h.broadcaster.handleHeader = recorder.HandleHeader

			setup := tt.setup(t, h.broadcaster)
			err := h.broadcaster.handleIncomingRPC(setup.inputRPC)

			if tt.expectedErrContains != "" {
				require.ErrorContains(t, tt.expectedErrContains, err)
			} else {
				require.NoError(t, err)
			}

			if tt.expectHeaderValidateCall {
				select {
				case call := <-recorder.validateHeaderCallCh:
					require.DeepEqual(t, setup.expectedHeader.KzgCommitments, call.KzgCommitments)
					require.DeepEqual(t, setup.expectedHeader.KzgCommitmentsInclusionProof, call.KzgCommitmentsInclusionProof)
					require.DeepEqual(t, setup.expectedHeader.SignedBlockHeader, call.SignedBlockHeader)
				case <-t.Context().Done():
					t.Fatalf("header validation call not received")
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
				require.Equal(t, tt.expectPeerFeedback, ps.peerFeedbackCalls[0].kind)
				require.Equal(t, setup.inputRPC.from, ps.peerFeedbackCalls[0].peerID)
			} else {
				require.Equal(t, 0, len(ps.peerFeedbackCalls))
			}

			stored := h.broadcaster.getDataColumn(setup.inputRPC.GetTopicID(), setup.inputRPC.GroupID)
			if tt.expectPublish {
				require.NotNil(t, stored)
				ps.assertPartialColumnsPublished(t, setup.inputRPC.GetTopicID(), []*blocks.PartialDataColumn{stored})
			} else {
				require.Equal(t, 0, len(ps.publishedPartialColumns))
			}

			if tt.expectedStoreColumn != nil {
				assertPartialColumnsEqual(t, tt.expectedStoreColumn(t), stored)
			}
		})
	}
}

func TestPartialColumnBroadcaster_handleCellsValidated(t *testing.T) {
	const topic = "/eth2/abcd1234/data_column_sidecar_12/ssz_snappy"

	type testSetup struct {
		column         *blocks.PartialDataColumn
		group          []byte
		getBlobsCalled bool
	}

	tests := []struct {
		wantErrContains  string
		name             string
		publishErr       error
		setup            func(t *testing.T) testSetup
		validatedCells   map[uint64][]byte
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
			name: "duplicate validated cells do not extend and do not publish",
			setup: func(t *testing.T) testSetup {
				c := createPartialColumn(t, 3, map[uint64][]byte{
					0: {0x10},
				})
				return testSetup{
					column:         c,
					group:          c.GroupID(),
					getBlobsCalled: true,
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
					column:         c,
					group:          c.GroupID(),
					getBlobsCalled: false,
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
					column:         c,
					group:          c.GroupID(),
					getBlobsCalled: true,
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
					column:         c,
					group:          c.GroupID(),
					getBlobsCalled: true,
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
					column:         c,
					group:          c.GroupID(),
					getBlobsCalled: false,
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
					column:         c,
					group:          c.GroupID(),
					getBlobsCalled: true,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := newMockPubSub(tt.publishErr, nil)
			recorder := newCallbackRecorder(2, false, nil, nil)
			h := newBroadcasterHarness(t, ps)

			setup := tt.setup(t)
			if setup.column != nil {
				h.broadcaster.partialMsgStore[topic] = map[string]*blocks.PartialDataColumn{
					string(setup.group): setup.column,
				}
				h.broadcaster.getBlobsCalled[string(setup.group)] = setup.getBlobsCalled
			}
			h.broadcaster.handleColumn = recorder.HandleColumn

			var cellIndices []uint64
			var cells []blocks.CellProofBundle
			if setup.column != nil {
				cellIndices, cells = buildValidatedCells(setup.column.Index, tt.validatedCells)
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
				require.Equal(t, 0, len(ps.publishedPartialColumns))
			}

			if tt.expectHandle {
				select {
				case call := <-recorder.handleColumnCallCh:
					require.Equal(t, topic, call.topic)
					require.Equal(t, true, len(call.column.Column) > 0)
				case <-t.Context().Done():
					t.Fatalf("handle column call not received")
				}
			}
		})
	}
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
		expectHandleColumn  bool
		expectedErrContains string
		publishErr          error
		name                string
		existingColumn      func(t *testing.T) *blocks.PartialDataColumn
		publishColumn       func(t *testing.T) *blocks.PartialDataColumn
		expectedStoreColumn func(t *testing.T) *blocks.PartialDataColumn
	}{
		{
			name:                "new group stores and publishes",
			publishColumn:       column1,
			expectedStoreColumn: column1,
		},
		{
			name:                "publish error is returned to caller",
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
			name:               "existing extends to complete and invokes handleColumn",
			expectHandleColumn: true,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			column := tt.publishColumn(t)
			groupID := string(column.GroupID())

			ps := newMockPubSub(tt.publishErr, nil)
			recorder := newCallbackRecorder(1, false, nil, nil)

			h := newBroadcasterHarness(t, ps)
			if tt.existingColumn != nil {
				existing := tt.existingColumn(t)
				h.broadcaster.partialMsgStore[topic] = map[string]*blocks.PartialDataColumn{
					groupID: existing,
				}
			}

			h.start(recorder)
			defer h.Stop()

			err := h.broadcaster.Publish(func(yield func(string, blocks.PartialDataColumn) bool) {
				yield(topic, *column)
			})
			if tt.expectedErrContains != "" {
				require.ErrorContains(t, tt.expectedErrContains, err)
			} else {
				require.NoError(t, err)
			}

			stored := h.broadcaster.getDataColumn(topic, column.GroupID())
			expectedStored := tt.expectedStoreColumn(t)
			assertPartialColumnsEqual(t, expectedStored, stored)
			ps.assertPartialColumnsPublished(t, topic, []*blocks.PartialDataColumn{expectedStored})

			getBlobs := h.broadcaster.getBlobsCalled[groupID]
			// getBlobs is only updated if err == nil
			require.Equal(t, err == nil, getBlobs)

			if tt.expectHandleColumn {
				select {
				case call := <-recorder.handleColumnCallCh:
					require.Equal(t, topic, call.topic)
					require.DeepEqual(t, expectedStored.Column, call.column.Column)
				case <-t.Context().Done():
					t.Fatalf("handle column call not received")
				}
			} else {
				select {
				case <-recorder.handleColumnCallCh:
					t.Fatal("handle column should not be called")
				default:
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

			err := h.broadcaster.Subscribe(topic)
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
				h.broadcaster.partialMsgStore[topicName] = map[string]*blocks.PartialDataColumn{
					"group1": createPartialColumn(t, 2, map[uint64][]byte{0: {0x10}}),
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

			err := h.broadcaster.Unsubscribe(topicName)
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
