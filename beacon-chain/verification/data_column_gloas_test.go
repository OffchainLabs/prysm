package verification

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
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
	require.NoError(t, kzg.Start())

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

func TestCorrectSubnetGloas(t *testing.T) {
	const dataColumnSidecarSubTopic = "/data_column_sidecar_%d/"

	roDataColumn, signedBlock := testGloasDataColumnFixture(t)

	t.Run("lengths mismatch", func(t *testing.T) {
		verifier := NewGloasDataColumnVerifier(roDataColumn, signedBlock.Block(), GossipDataColumnSidecarRequirementsGloas)

		err := verifier.CorrectSubnet(dataColumnSidecarSubTopic, []string{})
		require.ErrorIs(t, err, errBadTopicLength)
	})

	t.Run("wrong topic", func(t *testing.T) {
		verifier := NewGloasDataColumnVerifier(roDataColumn, signedBlock.Block(), GossipDataColumnSidecarRequirementsGloas)

		wrongSubnet := (peerdas.ComputeSubnetForDataColumnSidecar(roDataColumn.Index) + 1) % 128
		err := verifier.CorrectSubnet(
			dataColumnSidecarSubTopic,
			[]string{fmt.Sprintf("/eth2/9dc47cc6/data_column_sidecar_%d/ssz_snappy", wrongSubnet)},
		)
		require.ErrorIs(t, err, errBadTopic)
	})

	t.Run("nominal", func(t *testing.T) {
		verifier := NewGloasDataColumnVerifier(roDataColumn, signedBlock.Block(), GossipDataColumnSidecarRequirementsGloas)

		subnet := peerdas.ComputeSubnetForDataColumnSidecar(roDataColumn.Index)
		err := verifier.CorrectSubnet(
			dataColumnSidecarSubTopic,
			[]string{fmt.Sprintf("/eth2/9dc47cc6/data_column_sidecar_%d/ssz_snappy", subnet)},
		)
		require.NoError(t, err)

		err = verifier.CorrectSubnet(
			dataColumnSidecarSubTopic,
			[]string{fmt.Sprintf("/eth2/9dc47cc6/data_column_sidecar_%d/ssz_snappy", subnet)},
		)
		require.NoError(t, err)
	})
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
