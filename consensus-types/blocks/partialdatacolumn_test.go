package blocks

import (
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/libp2p/go-libp2p-pubsub/partialmessages"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
)

func testSignedHeader(validRoots bool, sigLen int) *ethpb.SignedBeaconBlockHeader {
	parentRoot := make([]byte, fieldparams.RootLength)
	stateRoot := make([]byte, fieldparams.RootLength)
	bodyRoot := make([]byte, fieldparams.RootLength)
	if !validRoots {
		parentRoot = []byte{1}
	}
	return &ethpb.SignedBeaconBlockHeader{
		Header: &ethpb.BeaconBlockHeader{
			ParentRoot: parentRoot,
			StateRoot:  stateRoot,
			BodyRoot:   bodyRoot,
		},
		Signature: make([]byte, sigLen),
	}
}

func sizedSlices(n, size int, start byte) [][]byte {
	out := make([][]byte, n)
	for i := range n {
		b := make([]byte, size)
		for j := range b {
			b[j] = start + byte(i)
		}
		out[i] = b
	}
	return out
}

func testBitlist(n uint64, set ...uint64) bitfield.Bitlist {
	bl := bitfield.NewBitlist(n)
	for _, idx := range set {
		bl.SetBitAt(idx, true)
	}
	return bl
}

func testPeerMeta(n uint64, available, requests []uint64) *ethpb.PartialDataColumnPartsMetadata {
	return &ethpb.PartialDataColumnPartsMetadata{
		Available: testBitlist(n, available...),
		Requests:  testBitlist(n, requests...),
	}
}

func allSet(n uint64) []uint64 {
	out := make([]uint64, n)
	for i := range n {
		out[i] = i
	}
	return out
}

func mustMarshalMeta(t *testing.T, meta *ethpb.PartialDataColumnPartsMetadata) partialmessages.PartsMetadata {
	t.Helper()
	out, err := marshalPartsMetadata(meta)
	require.NoError(t, err)
	return out
}

func mustNewPartialColumnWithSigLen(t *testing.T, n int, sigLen int, included ...uint64) *PartialDataColumn {
	t.Helper()
	pdc, err := NewPartialDataColumn(
		testSignedHeader(true, sigLen),
		7,
		sizedSlices(n, 48, 1),
		sizedSlices(4, 32, 90),
	)
	require.NoError(t, err)

	for _, idx := range included {
		pdc.Included.SetBitAt(idx, true)
		pdc.Column[idx] = sizedSlices(1, 2048, byte(idx+1))[0]
		pdc.KzgProofs[idx] = sizedSlices(1, 48, byte(idx+11))[0]
	}
	return &pdc
}

func mustNewPartialColumn(t *testing.T, n int, included ...uint64) *PartialDataColumn {
	t.Helper()
	return mustNewPartialColumnWithSigLen(t, n, fieldparams.BLSSignatureLength, included...)
}

func mustDecodeSidecar(t *testing.T, encoded []byte) *ethpb.PartialDataColumnSidecar {
	t.Helper()
	var msg ethpb.PartialDataColumnSidecar
	require.NoError(t, msg.UnmarshalSSZ(encoded))
	return &msg
}

func TestNewPartialDataColumn(t *testing.T) {
	tests := []struct {
		name        string
		header      *ethpb.SignedBeaconBlockHeader
		commitments [][]byte
		inclusion   [][]byte
		wantErr     string
	}{
		{
			name:        "nominal empty commitments",
			header:      testSignedHeader(true, fieldparams.BLSSignatureLength),
			commitments: nil,
			inclusion:   sizedSlices(4, 32, 10),
		},
		{
			name:        "nominal with commitments",
			header:      testSignedHeader(true, fieldparams.BLSSignatureLength),
			commitments: sizedSlices(3, 48, 10),
			inclusion:   sizedSlices(4, 32, 10),
		},
		{
			name:        "header hash tree root error",
			header:      testSignedHeader(false, fieldparams.BLSSignatureLength),
			commitments: sizedSlices(2, 48, 10),
			inclusion:   sizedSlices(4, 32, 10),
			wantErr:     "ParentRoot",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdc, err := NewPartialDataColumn(tt.header, 11, tt.commitments, tt.inclusion)
			if tt.wantErr != "" {
				require.ErrorContains(t, tt.wantErr, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, uint64(11), pdc.Index)
			require.Equal(t, len(tt.commitments), len(pdc.Column))
			require.Equal(t, len(tt.commitments), len(pdc.KzgProofs))
			require.Equal(t, uint64(len(tt.commitments)), pdc.Included.Len())
			require.Equal(t, uint64(0), pdc.Included.Count())
			require.Equal(t, fieldparams.RootLength+1, len(pdc.groupID))
			require.Equal(t, byte(0), pdc.groupID[0])

			root, rootErr := tt.header.Header.HashTreeRoot()
			require.NoError(t, rootErr)
			require.DeepEqual(t, root[:], pdc.groupID[1:])
		})
	}
}

func TestPartialDataColumn_ClearEagerPushSent(t *testing.T) {
	tests := []struct {
		name string
		m    map[peer.ID]struct{}
	}{
		{name: "already nil", m: nil},
		{name: "non-empty map", m: map[peer.ID]struct{}{peer.ID("p1"): {}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &PartialDataColumn{eagerPushSent: tt.m}
			p.ClearEagerPushSent()
			require.NotNil(t, p.eagerPushSent)
			require.Equal(t, 0, len(p.eagerPushSent))
		})
	}
}

func TestPartialDataColumn_newPartsMetadata(t *testing.T) {
	tests := []struct {
		name          string
		n             int
		includedBits  []uint64
		expectedAvail []uint64
	}{
		{name: "none included", n: 4, includedBits: nil, expectedAvail: nil},
		{name: "sparse included", n: 5, includedBits: []uint64{1, 4}, expectedAvail: []uint64{1, 4}},
		{name: "all included", n: 3, includedBits: []uint64{0, 1, 2}, expectedAvail: []uint64{0, 1, 2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := mustNewPartialColumn(t, tt.n, tt.includedBits...)
			meta := p.newPartsMetadata()
			require.Equal(t, uint64(tt.n), bitfield.Bitlist(meta.Available).Len())
			require.Equal(t, uint64(tt.n), bitfield.Bitlist(meta.Requests).Len())

			expected := testBitlist(uint64(tt.n), tt.expectedAvail...)
			require.DeepEqual(t, []byte(expected), []byte(meta.Available))

			for i := uint64(0); i < uint64(tt.n); i++ {
				require.Equal(t, true, bitfield.Bitlist(meta.Requests).BitAt(i))
			}
		})
	}
}

func TestNewPartsMetaWithNoAvailableAndNoRequests(t *testing.T) {
	tests := []struct {
		name string
		n    uint64
	}{
		{name: "zero", n: 0},
		{name: "non-zero", n: 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := NewPartsMetaWithNoAvailableAndNoRequests(tt.n)
			require.Equal(t, tt.n, bitfield.Bitlist(meta.Available).Len())
			require.Equal(t, uint64(0), bitfield.Bitlist(meta.Available).Count())
			require.Equal(t, tt.n, bitfield.Bitlist(meta.Requests).Len())
			require.Equal(t, uint64(0), bitfield.Bitlist(meta.Requests).Count())
		})
	}
}

func TestMarshalPartsMetadata(t *testing.T) {
	tests := []struct {
		name    string
		meta    *ethpb.PartialDataColumnPartsMetadata
		wantErr string
	}{
		{
			name: "valid",
			meta: &ethpb.PartialDataColumnPartsMetadata{
				Available: testBitlist(4, 1),
				Requests:  testBitlist(4, allSet(4)...),
			},
		},
		{
			name: "available too large",
			meta: &ethpb.PartialDataColumnPartsMetadata{
				Available: bitfield.NewBitlist(4096),
				Requests:  bitfield.NewBitlist(1),
			},
			wantErr: "Available",
		},
		{
			name: "requests too large",
			meta: &ethpb.PartialDataColumnPartsMetadata{
				Available: bitfield.NewBitlist(1),
				Requests:  bitfield.NewBitlist(4096),
			},
			wantErr: "Requests",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := marshalPartsMetadata(tt.meta)
			if tt.wantErr != "" {
				require.ErrorContains(t, tt.wantErr, err)
				return
			}
			require.NoError(t, err)
			parsed, parseErr := ParsePartsMetadata(out, 4)
			require.NoError(t, parseErr)
			require.Equal(t, uint64(4), bitfield.Bitlist(parsed.Available).Len())
		})
	}
}

func TestParsePartsMetadata(t *testing.T) {
	validMeta := mustMarshalMeta(t, &ethpb.PartialDataColumnPartsMetadata{
		Available: testBitlist(4, 1),
		Requests:  testBitlist(4, allSet(4)...),
	})

	requestMismatchMeta := mustMarshalMeta(t, &ethpb.PartialDataColumnPartsMetadata{
		Available: bitfield.NewBitlist(4),
		Requests:  bitfield.NewBitlist(3),
	})

	tests := []struct {
		name           string
		pm             partialmessages.PartsMetadata
		expectedLength uint64
		wantErr        string
	}{
		{
			name:           "valid",
			pm:             validMeta,
			expectedLength: 4,
		},
		{
			name:           "invalid ssz",
			pm:             partialmessages.PartsMetadata{1, 2, 3},
			expectedLength: 4,
			wantErr:        "size",
		},
		{
			name:           "available length mismatch",
			pm:             validMeta,
			expectedLength: 3,
			wantErr:        "invalid parts metadata length",
		},
		{
			name:           "requests length mismatch",
			pm:             requestMismatchMeta,
			expectedLength: 4,
			wantErr:        "invalid parts metadata length",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, err := ParsePartsMetadata(tt.pm, tt.expectedLength)
			if tt.wantErr != "" {
				require.ErrorContains(t, tt.wantErr, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expectedLength, bitfield.Bitlist(meta.Available).Len())
			require.Equal(t, tt.expectedLength, bitfield.Bitlist(meta.Requests).Len())
		})
	}
}

func TestPartialDataColumn_PartsMetadata(t *testing.T) {
	tests := []struct {
		name       string
		p          *PartialDataColumn
		expectedN  uint64
		expectErr  string
		availCount uint64
	}{
		{
			name:       "nominal",
			p:          mustNewPartialColumn(t, 4, 1, 2),
			expectedN:  4,
			availCount: 2,
		},
		{
			name: "marshal error due max bitlist size",
			p: &PartialDataColumn{
				DataColumnSidecar: &ethpb.DataColumnSidecar{
					KzgCommitments: make([][]byte, 4096),
				},
				Included: bitfield.NewBitlist(4096),
			},
			expectErr: "Available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, err := tt.p.PartsMetadata()
			if tt.expectErr != "" {
				require.ErrorContains(t, tt.expectErr, err)
				return
			}
			require.NoError(t, err)
			parsed, parseErr := ParsePartsMetadata(meta, tt.expectedN)
			require.NoError(t, parseErr)
			require.Equal(t, tt.availCount, bitfield.Bitlist(parsed.Available).Count())
			require.Equal(t, tt.expectedN, bitfield.Bitlist(parsed.Requests).Count())
		})
	}
}

func TestPartialDataColumn_cellsToSendForPeer(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "metadata length mismatch",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 4, 0)
				_, _, err := p.cellsToSendForPeer(testPeerMeta(3, nil, allSet(3)))
				require.ErrorContains(t, "peer metadata bitmap length mismatch", err)
			},
		},
		{
			name: "no cells to send",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 3, 1)
				encoded, sent, err := p.cellsToSendForPeer(testPeerMeta(3, []uint64{1}, allSet(3)))
				require.NoError(t, err)
				require.IsNil(t, encoded)
				require.IsNil(t, sent)
			},
		},
		{
			name: "sends only requested and missing cells",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 4, 0, 1, 3)
				encoded, sent, err := p.cellsToSendForPeer(testPeerMeta(4, []uint64{1}, allSet(4)))
				require.NoError(t, err)
				require.NotNil(t, encoded)
				require.NotNil(t, sent)
				require.Equal(t, true, sent.BitAt(0))
				require.Equal(t, false, sent.BitAt(1))
				require.Equal(t, true, sent.BitAt(3))

				msg := mustDecodeSidecar(t, encoded)
				require.Equal(t, 2, len(msg.PartialColumn))
				require.Equal(t, 2, len(msg.KzgProofs))
				require.Equal(t, true, bitfield.Bitlist(msg.CellsPresentBitmap).BitAt(0))
				require.Equal(t, false, bitfield.Bitlist(msg.CellsPresentBitmap).BitAt(1))
				require.Equal(t, true, bitfield.Bitlist(msg.CellsPresentBitmap).BitAt(3))
			},
		},
		{
			name: "marshal fails with invalid cell length",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 1, 0)
				p.Column[0] = []byte{1}
				_, _, err := p.cellsToSendForPeer(testPeerMeta(1, nil, []uint64{0}))
				require.ErrorContains(t, "PartialColumn", err)
			},
		},
		{
			name: "marshal fails with invalid proof length",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 1, 0)
				p.KzgProofs[0] = []byte{1}
				_, _, err := p.cellsToSendForPeer(testPeerMeta(1, nil, []uint64{0}))
				require.ErrorContains(t, "KzgProofs", err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestPartialDataColumn_eagerPushBytes(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "nominal",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 3, 0)
				encoded, err := p.eagerPushBytes()
				require.NoError(t, err)
				msg := mustDecodeSidecar(t, encoded)
				require.Equal(t, 1, len(msg.Header))
				require.Equal(t, 0, len(msg.PartialColumn))
				require.Equal(t, 0, len(msg.KzgProofs))
				require.Equal(t, uint64(3), bitfield.Bitlist(msg.CellsPresentBitmap).Len())
				require.Equal(t, uint64(0), bitfield.Bitlist(msg.CellsPresentBitmap).Count())
			},
		},
		{
			name: "invalid commitment size",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 2, 0)
				p.KzgCommitments[0] = []byte{1}
				_, err := p.eagerPushBytes()
				require.ErrorContains(t, "KzgCommitments", err)
			},
		},
		{
			name: "invalid inclusion proof vector length",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 2, 0)
				p.KzgCommitmentsInclusionProof = p.KzgCommitmentsInclusionProof[:3]
				_, err := p.eagerPushBytes()
				require.ErrorContains(t, "KzgCommitmentsInclusionProof", err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestMergeAvailableIntoPartsMetadata(t *testing.T) {
	tests := []struct {
		name      string
		base      *ethpb.PartialDataColumnPartsMetadata
		add       bitfield.Bitlist
		expectErr string
	}{
		{
			name:      "nil base",
			base:      nil,
			add:       bitfield.NewBitlist(2),
			expectErr: "base is nil",
		},
		{
			name: "available length mismatch",
			base: &ethpb.PartialDataColumnPartsMetadata{
				Available: bitfield.NewBitlist(4),
				Requests:  bitfield.NewBitlist(4),
			},
			add:       bitfield.NewBitlist(3),
			expectErr: "available length mismatch",
		},
		{
			name: "requests length mismatch",
			base: &ethpb.PartialDataColumnPartsMetadata{
				Available: bitfield.NewBitlist(4),
				Requests:  bitfield.NewBitlist(3),
			},
			add:       bitfield.NewBitlist(4),
			expectErr: "requests length mismatch",
		},
		{
			name: "successfully merges",
			base: &ethpb.PartialDataColumnPartsMetadata{
				Available: testBitlist(4, 1),
				Requests:  testBitlist(4, allSet(4)...),
			},
			add: testBitlist(4, 2),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := MergeAvailableIntoPartsMetadata(tt.base, tt.add)
			if tt.expectErr != "" {
				require.ErrorContains(t, tt.expectErr, err)
				return
			}

			require.NoError(t, err)
			meta, parseErr := ParsePartsMetadata(out, tt.add.Len())
			require.NoError(t, parseErr)
			require.Equal(t, false, bitfield.Bitlist(meta.Available).BitAt(0))
			require.Equal(t, true, bitfield.Bitlist(meta.Available).BitAt(1))
			require.Equal(t, true, bitfield.Bitlist(meta.Available).BitAt(2))
			require.Equal(t, false, bitfield.Bitlist(meta.Available).BitAt(3))
			// Verify that MergeAvailableIntoPartsMetadata mutates its base argument.
			require.Equal(t, true, bitfield.Bitlist(tt.base.Available).BitAt(2))
		})
	}
}

func TestPartialDataColumn_updateReceivedStateOutgoing(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, p *PartialDataColumn)
	}{
		{
			name: "nil receivedMeta",
			run: func(t *testing.T, p *PartialDataColumn) {
				_, err := p.updateReceivedStateOutgoing(nil, testBitlist(4, 1))
				require.ErrorContains(t, "receivedMeta is empty", err)
			},
		},
		{
			name: "invalid receivedMeta parse",
			run: func(t *testing.T, p *PartialDataColumn) {
				_, err := p.updateReceivedStateOutgoing(partialmessages.PartsMetadata{1, 2}, testBitlist(4, 1))
				require.NotNil(t, err)
			},
		},
		{
			name: "cellsSent length mismatch",
			run: func(t *testing.T, p *PartialDataColumn) {
				recvd := mustMarshalMeta(t, testPeerMeta(4, nil, allSet(4)))
				_, err := p.updateReceivedStateOutgoing(recvd, testBitlist(3, 1))
				require.ErrorContains(t, "available length mismatch", err)
			},
		},
		{
			name: "success",
			run: func(t *testing.T, p *PartialDataColumn) {
				recvd := mustMarshalMeta(t, &ethpb.PartialDataColumnPartsMetadata{
					Available: testBitlist(4, 1),
					Requests:  testBitlist(4, allSet(4)...),
				})
				out, err := p.updateReceivedStateOutgoing(recvd, testBitlist(4, 3))
				require.NoError(t, err)
				parsed, parseErr := ParsePartsMetadata(out, 4)
				require.NoError(t, parseErr)
				require.Equal(t, true, bitfield.Bitlist(parsed.Available).BitAt(1))
				require.Equal(t, true, bitfield.Bitlist(parsed.Available).BitAt(3))
				require.Equal(t, uint64(4), bitfield.Bitlist(parsed.Requests).Count())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := mustNewPartialColumn(t, 4, 0, 1)
			tt.run(t, p)
		})
	}
}

func TestPartialDataColumn_ForPeer(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "eager push first time for peer",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 2, 0)
				initial := partialmessages.PeerState{}
				next, encoded, meta, err := p.ForPeer(peer.ID("peer-a"), true, initial)
				require.NoError(t, err)
				require.NotNil(t, encoded)
				require.IsNil(t, meta)
				require.IsNil(t, next.RecvdState)
				require.IsNil(t, next.SentState)
				_, seen := p.eagerPushSent[peer.ID("peer-a")]
				require.Equal(t, true, seen)
				decoded := mustDecodeSidecar(t, encoded)
				require.Equal(t, 1, len(decoded.Header))
				require.Equal(t, 0, len(decoded.PartialColumn))
			},
		},
		{
			name: "eager push not repeated",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 2, 0)
				_, _, _, err := p.ForPeer(peer.ID("peer-a"), true, partialmessages.PeerState{})
				require.NoError(t, err)
				next, encoded, meta, err := p.ForPeer(peer.ID("peer-a"), true, partialmessages.PeerState{})
				require.NoError(t, err)
				require.IsNil(t, encoded)
				require.IsNil(t, meta)
				require.IsNil(t, next.RecvdState)
				require.IsNil(t, next.SentState)
			},
		},
		{
			name: "requested false sends only parts metadata",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 3, 0)
				next, encoded, meta, err := p.ForPeer(peer.ID("peer-a"), false, partialmessages.PeerState{})
				require.NoError(t, err)
				require.IsNil(t, encoded)
				require.NotNil(t, meta)
				require.NotNil(t, next.SentState)
				_, ok := next.SentState.(partialmessages.PartsMetadata)
				require.Equal(t, true, ok)
			},
		},
		{
			name: "invalid SentState type",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 3, 0)
				_, _, _, err := p.ForPeer(peer.ID("peer-a"), false, partialmessages.PeerState{SentState: "bad"})
				require.ErrorContains(t, "SentState is not PartsMetadata", err)
			},
		},
		{
			name: "invalid RecvdState type",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 3, 0)
				_, _, _, err := p.ForPeer(peer.ID("peer-a"), false, partialmessages.PeerState{RecvdState: "bad"})
				require.ErrorContains(t, "RecvdState is not PartsMetadata", err)
			},
		},
		{
			name: "invalid received metadata parse",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 3, 0)
				_, _, _, err := p.ForPeer(peer.ID("peer-a"), true, partialmessages.PeerState{
					RecvdState: partialmessages.PartsMetadata{1, 2},
				})
				require.ErrorContains(t, "size", err)
			},
		},
		{
			name: "requested true sends missing cells and updates recvd state",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 4, 0, 2)
				initialMeta := mustMarshalMeta(t, testPeerMeta(4, nil, allSet(4)))
				next, encoded, meta, err := p.ForPeer(peer.ID("peer-a"), true, partialmessages.PeerState{
					RecvdState: initialMeta,
				})
				require.NoError(t, err)
				require.NotNil(t, encoded)
				require.NotNil(t, meta)

				sentMeta, parseSentErr := ParsePartsMetadata(meta, 4)
				require.NoError(t, parseSentErr)
				require.Equal(t, uint64(2), bitfield.Bitlist(sentMeta.Available).Count())
				require.Equal(t, true, bitfield.Bitlist(sentMeta.Available).BitAt(0))
				require.Equal(t, true, bitfield.Bitlist(sentMeta.Available).BitAt(2))
				require.Equal(t, uint64(4), bitfield.Bitlist(sentMeta.Requests).Count())

				sentState, ok := next.SentState.(partialmessages.PartsMetadata)
				require.Equal(t, true, ok)
				require.DeepEqual(t, []byte(meta), []byte(sentState))

				msg := mustDecodeSidecar(t, encoded)
				require.Equal(t, 2, len(msg.PartialColumn))
				require.Equal(t, true, bitfield.Bitlist(msg.CellsPresentBitmap).BitAt(0))
				require.Equal(t, true, bitfield.Bitlist(msg.CellsPresentBitmap).BitAt(2))
				recvdOut, parseErr := ParsePartsMetadata(next.RecvdState.(partialmessages.PartsMetadata), 4)
				require.NoError(t, parseErr)
				require.Equal(t, true, bitfield.Bitlist(recvdOut.Available).BitAt(0))
				require.Equal(t, true, bitfield.Bitlist(recvdOut.Available).BitAt(2))
			},
		},
		{
			name: "requested true with no missing cells",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 3, 1)
				recvd := mustMarshalMeta(t, &ethpb.PartialDataColumnPartsMetadata{
					Available: testBitlist(3, 1),
					Requests:  testBitlist(3, allSet(3)...),
				})
				next, encoded, _, err := p.ForPeer(peer.ID("peer-a"), true, partialmessages.PeerState{
					RecvdState: recvd,
				})
				require.NoError(t, err)
				require.IsNil(t, encoded)
				require.DeepEqual(t, []byte(recvd), []byte(next.RecvdState.(partialmessages.PartsMetadata)))
			},
		},
		{
			name: "does not resend unchanged metadata",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 3, 1)
				myMeta, err := p.PartsMetadata()
				require.NoError(t, err)
				next, encoded, meta, err := p.ForPeer(peer.ID("peer-a"), false, partialmessages.PeerState{
					SentState: myMeta,
				})
				require.NoError(t, err)
				require.IsNil(t, encoded)
				require.IsNil(t, meta)
				require.DeepEqual(t, []byte(myMeta), []byte(next.SentState.(partialmessages.PartsMetadata)))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestPartialDataColumn_CellsToVerifyFromPartialMessage(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "empty bitmap",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 3, 0)
				indices, bundles, err := p.CellsToVerifyFromPartialMessage(&ethpb.PartialDataColumnSidecar{
					CellsPresentBitmap: bitfield.NewBitlist(0),
				})
				require.NoError(t, err)
				require.IsNil(t, indices)
				require.IsNil(t, bundles)
			},
		},
		{
			name: "proofs count mismatch",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 3, 0)
				_, _, err := p.CellsToVerifyFromPartialMessage(&ethpb.PartialDataColumnSidecar{
					CellsPresentBitmap: testBitlist(3, 1),
					PartialColumn:      [][]byte{{1}},
					KzgProofs:          nil,
				})
				require.ErrorContains(t, "Missing KZG proofs", err)
			},
		},
		{
			name: "cells count mismatch",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 3, 0)
				_, _, err := p.CellsToVerifyFromPartialMessage(&ethpb.PartialDataColumnSidecar{
					CellsPresentBitmap: testBitlist(3, 1),
					PartialColumn:      nil,
					KzgProofs:          [][]byte{{1}},
				})
				require.ErrorContains(t, "Missing cells", err)
			},
		},
		{
			name: "wrong bitmap length",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 4, 0)
				_, _, err := p.CellsToVerifyFromPartialMessage(&ethpb.PartialDataColumnSidecar{
					CellsPresentBitmap: testBitlist(3, 1),
					PartialColumn:      [][]byte{{1}},
					KzgProofs:          [][]byte{{2}},
				})
				require.ErrorContains(t, "wrong bitmap length", err)
			},
		},
		{
			name: "returns only unknown cells in bitmap order",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 4, 1)
				msg := &ethpb.PartialDataColumnSidecar{
					CellsPresentBitmap: testBitlist(4, 0, 1, 3),
					PartialColumn:      [][]byte{{0xA}, {0xB}, {0xC}},
					KzgProofs:          [][]byte{{0x1}, {0x2}, {0x3}},
				}
				indices, bundles, err := p.CellsToVerifyFromPartialMessage(msg)
				require.NoError(t, err)
				require.DeepEqual(t, []uint64{0, 3}, indices)
				require.Equal(t, 2, len(bundles))
				require.Equal(t, p.Index, bundles[0].ColumnIndex)
				require.DeepEqual(t, []byte{0xA}, bundles[0].Cell)
				require.DeepEqual(t, []byte{0xC}, bundles[1].Cell)
				require.DeepEqual(t, p.KzgCommitments[0], bundles[0].Commitment)
				require.DeepEqual(t, p.KzgCommitments[3], bundles[1].Commitment)
			},
		},
		{
			name: "all cells already known",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 2, 0, 1)
				msg := &ethpb.PartialDataColumnSidecar{
					CellsPresentBitmap: testBitlist(2, 0, 1),
					PartialColumn:      [][]byte{{0xA}, {0xB}},
					KzgProofs:          [][]byte{{0x1}, {0x2}},
				}
				indices, bundles, err := p.CellsToVerifyFromPartialMessage(msg)
				require.NoError(t, err)
				require.Equal(t, 0, len(indices))
				require.Equal(t, 0, len(bundles))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestPartialDataColumn_ExtendFromVerifiedCell(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "already present cell is not overwritten",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 2, 1)
				oldCell := p.Column[1]
				ok := p.ExtendFromVerifiedCell(1, []byte{9}, []byte{8})
				require.Equal(t, false, ok)
				require.DeepEqual(t, oldCell, p.Column[1])
			},
		},
		{
			name: "new cell extends data",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 2, 1)
				ok := p.ExtendFromVerifiedCell(0, []byte{9}, []byte{8})
				require.Equal(t, true, ok)
				require.Equal(t, true, p.Included.BitAt(0))
				require.DeepEqual(t, []byte{9}, p.Column[0])
				require.DeepEqual(t, []byte{8}, p.KzgProofs[0])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestPartialDataColumn_ExtendFromVerifiedCells(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "mismatched cellIndices and cells panics",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 2)
				defer func() {
					require.NotNil(t, recover())
				}()
				p.ExtendFromVerifiedCells(
					[]uint64{0},
					[]CellProofBundle{
						{ColumnIndex: p.Index, Cell: []byte{1}, Proof: []byte{2}},
						{ColumnIndex: p.Index, Cell: []byte{3}, Proof: []byte{4}},
					},
				)
			},
		},
		{
			name: "all new cells",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 3)
				ok := p.ExtendFromVerifiedCells(
					[]uint64{0, 2},
					[]CellProofBundle{
						{ColumnIndex: p.Index, Cell: []byte{1}, Proof: []byte{2}},
						{ColumnIndex: p.Index, Cell: []byte{3}, Proof: []byte{4}},
					},
				)
				require.Equal(t, true, ok)
				require.Equal(t, true, p.Included.BitAt(0))
				require.Equal(t, true, p.Included.BitAt(2))
			},
		},
		{
			name: "all duplicate cells",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 2, 1)
				ok := p.ExtendFromVerifiedCells(
					[]uint64{1},
					[]CellProofBundle{{ColumnIndex: p.Index, Cell: []byte{7}, Proof: []byte{8}}},
				)
				require.Equal(t, false, ok)
			},
		},
		{
			name: "invalid column index first",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 2)
				ok := p.ExtendFromVerifiedCells(
					[]uint64{0},
					[]CellProofBundle{{ColumnIndex: p.Index + 1, Cell: []byte{1}, Proof: []byte{2}}},
				)
				require.Equal(t, false, ok)
				require.Equal(t, uint64(0), p.Included.Count())
			},
		},
		{
			name: "invalid column index after partial extension",
			run: func(t *testing.T) {
				p := mustNewPartialColumn(t, 3)
				ok := p.ExtendFromVerifiedCells(
					[]uint64{0, 1},
					[]CellProofBundle{
						{ColumnIndex: p.Index, Cell: []byte{1}, Proof: []byte{2}},
						{ColumnIndex: p.Index + 1, Cell: []byte{3}, Proof: []byte{4}},
					},
				)
				require.Equal(t, false, ok)
				require.Equal(t, true, p.Included.BitAt(0))
				require.Equal(t, false, p.Included.BitAt(1))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestPartialDataColumn_Complete(t *testing.T) {
	tests := []struct {
		name   string
		p      *PartialDataColumn
		wantOK bool
	}{
		{
			name:   "incomplete",
			p:      mustNewPartialColumn(t, 2, 0),
			wantOK: false,
		},
		{
			name:   "complete valid data column",
			p:      mustNewPartialColumn(t, 2, 0, 1),
			wantOK: true,
		},
		{
			name:   "complete but invalid signed header",
			p:      mustNewPartialColumnWithSigLen(t, 2, 0, 0, 1),
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.p.Complete(logrus.New())
			require.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				require.NotNil(t, got.DataColumnSidecar)
			}
		})
	}
}
