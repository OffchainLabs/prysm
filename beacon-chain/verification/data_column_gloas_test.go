package verification

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"google.golang.org/protobuf/proto"
)

func testGloasDataColumnFixture(t *testing.T) (blocks.RODataColumn, interfaces.SignedBeaconBlock) {
	t.Helper()

	_, roSidecars, _ := util.GenerateTestFuluBlockWithSidecars(t, 1, util.WithSlot(1))
	require.Equal(t, true, len(roSidecars) > 0)

	base := roSidecars[0]
	bid := util.GenerateTestSignedExecutionPayloadBid(base.Slot())
	bid.Message.BlobKzgCommitments = base.KzgCommitments

	pb := util.NewBeaconBlockGloas()
	pb.Block.Slot = base.Slot()
	pb.Block.ProposerIndex = base.ProposerIndex()
	pb.Block.ParentRoot = bytes.Clone(base.SignedBlockHeader.Header.ParentRoot)
	pb.Block.StateRoot = bytes.Clone(base.SignedBlockHeader.Header.StateRoot)
	pb.Block.Body.SignedExecutionPayloadBid = bid

	signedBlock, err := blocks.NewSignedBeaconBlock(pb)
	require.NoError(t, err)

	header, err := signedBlock.Header()
	require.NoError(t, err)

	sidecar := proto.Clone(base.DataColumnSidecar).(*ethpb.DataColumnSidecar)
	sidecar.SignedBlockHeader = header
	sidecar.KzgCommitments = [][]byte{bytes.Repeat([]byte{0x42}, len(base.KzgCommitments[0]))}

	roDataColumn, err := blocks.NewRODataColumn(sidecar)
	require.NoError(t, err)

	return roDataColumn, signedBlock
}

func TestVerifyDataColumnSidecarSlotMatchesBlockGloas(t *testing.T) {
	roDataColumn, signedBlock := testGloasDataColumnFixture(t)
	verifier := NewGloasDataColumnVerifier(roDataColumn, signedBlock.Block(), GossipDataColumnSidecarRequirementsGloas)

	require.NoError(t, verifier.VerifyDataColumnSidecarSlotMatchesBlockGloas())

	sidecar := proto.Clone(roDataColumn.DataColumnSidecar).(*ethpb.DataColumnSidecar)
	sidecar.SignedBlockHeader.Header.Slot++
	wrongSlot, err := blocks.NewRODataColumn(sidecar)
	require.NoError(t, err)

	verifier = NewGloasDataColumnVerifier(wrongSlot, signedBlock.Block(), GossipDataColumnSidecarRequirementsGloas)
	err = verifier.VerifyDataColumnSidecarSlotMatchesBlockGloas()
	require.ErrorContains(t, "slot does not match block slot", err)
}

func TestVerifyDataColumnSidecarGloas(t *testing.T) {
	roDataColumn, signedBlock := testGloasDataColumnFixture(t)
	verifier := NewGloasDataColumnVerifier(roDataColumn, signedBlock.Block(), GossipDataColumnSidecarRequirementsGloas)

	require.NoError(t, verifier.VerifyDataColumnSidecarGloas())
	require.NoError(t, verifier.VerifyDataColumnSidecarKzgProofsGloas())

	sidecar := proto.Clone(roDataColumn.DataColumnSidecar).(*ethpb.DataColumnSidecar)
	sidecar.KzgProofs = nil
	noProofs, err := blocks.NewRODataColumn(sidecar)
	require.NoError(t, err)

	sidecar = proto.Clone(roDataColumn.DataColumnSidecar).(*ethpb.DataColumnSidecar)
	sidecar.Column = nil
	emptyColumn, err := blocks.NewRODataColumn(sidecar)
	require.NoError(t, err)
	verifier = NewGloasDataColumnVerifier(emptyColumn, signedBlock.Block(), GossipDataColumnSidecarRequirementsGloas)
	err = verifier.VerifyDataColumnSidecarGloas()
	require.ErrorIs(t, err, peerdas.ErrNoKzgCommitments)

	verifier = NewGloasDataColumnVerifier(noProofs, signedBlock.Block(), GossipDataColumnSidecarRequirementsGloas)
	err = verifier.VerifyDataColumnSidecarGloas()
	require.ErrorIs(t, err, peerdas.ErrMismatchLength)
}
