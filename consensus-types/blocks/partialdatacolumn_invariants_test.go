package blocks_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/libp2p/go-libp2p-pubsub/partialmessages"
	"github.com/libp2p/go-libp2p/core/peer"
)

type invariantChecker struct {
	t *testing.T
}

var _ partialmessages.InvariantChecker[*blocks.PartialDataColumn] = (*invariantChecker)(nil)

func (i *invariantChecker) MergePartsMetadata(left, right partialmessages.PartsMetadata) partialmessages.PartsMetadata {
	return partialmessages.MergeBitmap(left, right)
}

func (i *invariantChecker) SplitIntoParts(in *blocks.PartialDataColumn) ([]*blocks.PartialDataColumn, error) {
	var parts []*blocks.PartialDataColumn
	for idx := range in.Column {
		if !in.Included.BitAt(uint64(idx)) {
			continue
		}
		msg := i.EmptyMessage()
		msg.Included.SetBitAt(uint64(idx), true)
		msg.KzgCommitments = in.KzgCommitments
		msg.Column[idx] = in.Column[idx]
		msg.KzgProofs[idx] = in.KzgProofs[idx]
		parts = append(parts, msg)
	}
	return parts, nil
}

func (i *invariantChecker) FullMessage() (*blocks.PartialDataColumn, error) {
	blockRoot := []byte("test-block-root")
	numCells := 128
	commitments := make([][]byte, numCells)
	cells := make([][]byte, numCells)
	proofs := make([][]byte, numCells)

	for i := range numCells {
		for j := range commitments[i] {
			commitments[i][j] = byte(i)
		}
		cells[i] = make([]byte, 2048)
		fmt.Appendf(cells[i][:0], "cell %d", i)
		proofs[i] = make([]byte, 48)
		fmt.Appendf(proofs[i][:0], "proof %d", i)
	}

	roDC, _ := util.CreateTestVerifiedRoDataColumnSidecars(i.t, []util.DataColumnParam{
		{
			BodyRoot:       blockRoot[:],
			KzgCommitments: commitments,
			Column:         cells,
			KzgProofs:      proofs,
		},
	})

	c, err := blocks.NewPartialDataColumn(roDC[0].DataColumnSidecar.SignedBlockHeader, roDC[0].Index, roDC[0].KzgCommitments, roDC[0].KzgCommitmentsInclusionProof)
	return &c, err
}

func (i *invariantChecker) EmptyMessage() *blocks.PartialDataColumn {
	blockRoot := []byte("test-block-root")
	numCells := 128
	commitments := make([][]byte, numCells)
	cells := make([][]byte, numCells)
	proofs := make([][]byte, numCells)
	roDC, _ := util.CreateTestVerifiedRoDataColumnSidecars(i.t, []util.DataColumnParam{
		{
			BodyRoot:       blockRoot[:],
			KzgCommitments: commitments,
			Column:         cells,
			KzgProofs:      proofs,
		},
	})
	for i := range roDC[0].Column {
		// Clear these fields since this is an empty message
		roDC[0].Column[i] = nil
		roDC[0].KzgProofs[i] = nil
	}

	pc, err := blocks.NewPartialDataColumn(roDC[0].DataColumnSidecar.SignedBlockHeader, roDC[0].Index, roDC[0].KzgCommitments, roDC[0].KzgCommitmentsInclusionProof)
	if err != nil {
		panic(err)
	}
	return &pc
}

func (i *invariantChecker) ExtendFromBytes(a *blocks.PartialDataColumn, data []byte) (*blocks.PartialDataColumn, error) {
	var message ethpb.PartialDataColumnSidecar
	err := message.UnmarshalSSZ(data)
	if err != nil {
		return nil, err
	}
	cellIndices, bundle, err := a.CellsToVerifyFromPartialMessage(&message)
	if err != nil {
		return nil, err
	}
	// No validation happening here. Copy-pasters beware!
	_ = a.ExtendFromVerfifiedCells(cellIndices, bundle)
	return a, nil
}

func (i *invariantChecker) ShouldRequest(a *blocks.PartialDataColumn, from peer.ID, partsMetadata []byte) bool {
	peerHas := bitfield.Bitlist(partsMetadata)
	for i := range peerHas.Len() {
		if peerHas.BitAt(i) && !a.Included.BitAt(i) {
			return true
		}
	}
	return false
}

func (i *invariantChecker) Equal(a, b *blocks.PartialDataColumn) bool {
	if !bytes.Equal(a.GroupID(), b.GroupID()) {
		return false
	}
	if !bytes.Equal(a.Included, b.Included) {
		return false
	}
	if len(a.KzgCommitments) != len(b.KzgCommitments) {
		return false
	}
	for i := range a.KzgCommitments {
		if !bytes.Equal(a.KzgCommitments[i], b.KzgCommitments[i]) {
			return false
		}
	}
	if len(a.Column) != len(b.Column) {
		return false
	}
	for i := range a.Column {
		if !bytes.Equal(a.Column[i], b.Column[i]) {
			return false
		}
	}
	if len(a.KzgProofs) != len(b.KzgProofs) {
		return false
	}
	for i := range a.KzgProofs {
		if !bytes.Equal(a.KzgProofs[i], b.KzgProofs[i]) {
			return false
		}
	}
	return true
}

func TestDataColumnInvariants(t *testing.T) {
	partialmessages.TestPartialMessageInvariants(t, &invariantChecker{t})
}
