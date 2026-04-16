package blocks

import (
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// TestCellsToVerifyFromPartialMessage_DoesNotMutateInput verifies that
// CellsToVerifyFromPartialMessage does not modify the caller's message slices.
// The current implementation re-slices message.PartialColumn and
// message.KzgProofs in place, which silently truncates the input.
func TestCellsToVerifyFromPartialMessage_DoesNotMutateInput(t *testing.T) {
	nCommitments := uint64(3)
	col, err := NewPartialDataColumn(
		[fieldparams.RootLength]byte{},
		&ethpb.SignedBeaconBlockHeader{
			Header: &ethpb.BeaconBlockHeader{
				ParentRoot: make([]byte, fieldparams.RootLength),
				StateRoot:  make([]byte, fieldparams.RootLength),
				BodyRoot:   make([]byte, fieldparams.RootLength),
			},
			Signature: make([]byte, fieldparams.BLSSignatureLength),
		},
		0,
		make([][]byte, nCommitments), // 3 commitments
		nil,
	)
	require.NoError(t, err)

	// The partial column has cell 0 already; cells 1 and 2 are missing.
	col.ExtendFromVerifiedCell(0, []byte{0x01}, []byte{0x02})

	// Build a message that offers cells 1 and 2.
	bitmap := bitfield.NewBitlist(nCommitments)
	bitmap.SetBitAt(1, true)
	bitmap.SetBitAt(2, true)

	msg := &ethpb.PartialDataColumnSidecar{
		CellsPresentBitmap: bitmap,
		PartialColumn:      [][]byte{{0x10}, {0x20}},
		KzgProofs:          [][]byte{{0xA0}, {0xB0}},
	}

	origColumnLen := len(msg.PartialColumn)
	origProofsLen := len(msg.KzgProofs)

	_, _, err = col.CellsToVerifyFromPartialMessage(msg)
	require.NoError(t, err)

	// Assert that the message's slices were NOT modified.
	if len(msg.PartialColumn) != origColumnLen {
		t.Errorf("CellsToVerifyFromPartialMessage mutated message.PartialColumn: len went from %d to %d",
			origColumnLen, len(msg.PartialColumn))
	}
	if len(msg.KzgProofs) != origProofsLen {
		t.Errorf("CellsToVerifyFromPartialMessage mutated message.KzgProofs: len went from %d to %d",
			origProofsLen, len(msg.KzgProofs))
	}
}
