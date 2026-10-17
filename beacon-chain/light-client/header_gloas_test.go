package light_client

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"google.golang.org/protobuf/proto"
)

func TestBlockToLightClientHeader_Gloas(t *testing.T) {
	for _, tt := range []struct {
		name  string
		block any
		index uint64
		depth int
	}{
		{"phase0", util.NewBeaconBlock(), 0, 0},
		{"altair", util.NewBeaconBlockAltair(), 0, 0},
		{"bellatrix", util.NewBeaconBlockBellatrix(), 0, 0},
		{"capella", util.NewBeaconBlockCapella(), 412, 8},
		{"deneb", util.NewBeaconBlockDeneb(), 812, 9},
		{"electra", util.NewBeaconBlockElectra(), 812, 9},
		{"fulu", util.NewBeaconBlockFulu(), 812, 9},
		{"gloas", util.NewBeaconBlockGloas(), 2856, 11},
	} {
		t.Run(tt.name, func(t *testing.T) {
			block, err := blocks.NewSignedBeaconBlock(tt.block)
			require.NoError(t, err)
			block.SetSlot(123)
			block.SetProposerIndex(45)
			block.SetParentRoot(gloasHeaderTestRoot(1))
			block.SetStateRoot(gloasHeaderTestRoot(2))
			block.SetGraffiti(gloasHeaderTestRoot(3))
			populateGloasHeaderTestExecution(t, block)
			before, err := block.MarshalSSZ()
			require.NoError(t, err)

			header, err := BlockToLightClientHeader(t.Context(), version.Gloas, block)
			require.NoError(t, err)
			require.Equal(t, version.Gloas, header.Version())
			require.Equal(t, primitives.Slot(123), header.Beacon().Slot)
			require.Equal(t, primitives.ValidatorIndex(45), header.Beacon().ProposerIndex)
			require.DeepEqual(t, gloasHeaderTestRoot(1), header.Beacon().ParentRoot)
			require.DeepEqual(t, gloasHeaderTestRoot(2), header.Beacon().StateRoot)
			bodyRoot, err := block.Block().Body().HashTreeRoot()
			require.NoError(t, err)
			require.DeepEqual(t, bodyRoot[:], header.Beacon().BodyRoot)
			blockRoot, err := block.Block().HashTreeRoot()
			require.NoError(t, err)
			headerRoot, err := header.Beacon().HashTreeRoot()
			require.NoError(t, err)
			require.Equal(t, blockRoot, headerRoot)
			hash, err := header.ExecutionBlockHash()
			require.NoError(t, err)
			branch, err := header.ExecutionBranchGloas()
			require.NoError(t, err)
			require.Equal(t, 11, len(branch))
			if tt.depth == 0 {
				require.Equal(t, [32]byte{}, hash)
				require.Equal(t, interfaces.LightClientExecutionBranchGloas{}, branch)
			} else {
				require.DeepEqual(t, gloasHeaderTestRoot(42), hash[:])
				padding := len(branch) - tt.depth
				for _, node := range branch[:padding] {
					require.Equal(t, [32]byte{}, node)
				}
				proof := make([][]byte, tt.depth)
				for i := range proof {
					proof[i] = branch[padding+i][:]
				}
				require.Equal(t, true, trie.VerifyMerkleProof(bodyRoot[:], hash[:], tt.index, proof))
				hash[0] ^= 1
				require.Equal(t, false, trie.VerifyMerkleProof(bodyRoot[:], hash[:], tt.index, proof))
			}

			encoded, err := header.MarshalSSZ()
			require.NoError(t, err)
			decoded := &pb.LightClientHeaderGloas{}
			require.NoError(t, decoded.UnmarshalSSZ(encoded))
			require.Equal(t, true, proto.Equal(header.Proto(), decoded))
			after, err := block.MarshalSSZ()
			require.NoError(t, err)
			require.DeepEqual(t, before, after)

			if block.Version() >= version.Bellatrix && block.Version() < version.Gloas {
				t.Run("blinded", func(t *testing.T) {
					blinded, err := block.ToBlinded()
					require.NoError(t, err)
					blindedHeader, err := BlockToLightClientHeader(t.Context(), version.Gloas, blinded)
					require.NoError(t, err)
					require.Equal(t, true, proto.Equal(header.Proto(), blindedHeader.Proto()))
				})
			}
		})
	}
}

func gloasHeaderTestRoot(value byte) []byte {
	root := make([]byte, 32)
	root[0] = value
	return root
}

func populateGloasHeaderTestExecution(t *testing.T, block interfaces.SignedBeaconBlock) {
	t.Helper()
	if block.Version() == version.Gloas {
		bid, err := block.Block().Body().SignedExecutionPayloadBid()
		require.NoError(t, err)
		bid.Message.ParentBlockHash = gloasHeaderTestRoot(42)
		bid.Message.BlockHash = gloasHeaderTestRoot(99)
		bid.Message.BlobKzgCommitments = [][]byte{make([]byte, 48)}
		bid.Message.GasLimit = 30_000_000
		bid.Signature[0] = 7
		return
	}
	if block.Version() < version.Bellatrix {
		return
	}
	execution, err := block.Block().Body().Execution()
	require.NoError(t, err)
	transactions := [][]byte{{1, 2, 3}, {4, 5}}
	withdrawals := []*enginev1.Withdrawal{{Index: 1, ValidatorIndex: 2, Address: make([]byte, 20), Amount: 3}}
	switch p := execution.Proto().(type) {
	case *enginev1.ExecutionPayload:
		p.BlockHash = gloasHeaderTestRoot(42)
	case *enginev1.ExecutionPayloadCapella:
		p.BlockHash = gloasHeaderTestRoot(42)
		p.ExtraData, p.Transactions, p.Withdrawals = []byte{9, 8}, transactions, withdrawals
	case *enginev1.ExecutionPayloadDeneb:
		p.BlockHash = gloasHeaderTestRoot(42)
		p.ExtraData, p.Transactions, p.Withdrawals = []byte{9, 8}, transactions, withdrawals
		p.BlobGasUsed, p.ExcessBlobGas = 17, 23
	default:
		t.Fatalf("unexpected execution type %T", p)
	}
}

func TestBlockToLightClientHeader_GloasErrors(t *testing.T) {
	t.Run("cannot downgrade Gloas block", func(t *testing.T) {
		block, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockGloas())
		require.NoError(t, err)
		_, err = BlockToLightClientHeader(t.Context(), version.Fulu, block)
		require.ErrorContains(t, "greater than attested block version", err)
	})
	t.Run("unsupported target version", func(t *testing.T) {
		block, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockGloas())
		require.NoError(t, err)
		_, err = BlockToLightClientHeader(t.Context(), version.Gloas+1, block)
		require.ErrorContains(t, "unsupported attested block version", err)
	})
	t.Run("invalid body", func(t *testing.T) {
		p := util.NewBeaconBlockGloas()
		p.Block.Body.SignedExecutionPayloadBid.Message.ParentBlockHash = []byte{1}
		block, err := blocks.NewSignedBeaconBlock(p)
		require.NoError(t, err)
		_, err = BlockToLightClientHeader(t.Context(), version.Gloas, block)
		require.ErrorContains(t, "could not make beacon block header", err)
	})
}

func TestMakeExecutionAndProofGloas_Errors(t *testing.T) {
	t.Run("missing bid", func(t *testing.T) {
		block, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockGloas())
		require.NoError(t, err)
		for _, bid := range []*pb.SignedExecutionPayloadBid{nil, {}} {
			require.NoError(t, block.SetSignedExecutionPayloadBid(bid))
			_, _, err = makeExecutionAndProofGloas(t.Context(), block)
			require.ErrorContains(t, "signed execution payload bid is nil", err)
		}
	})
	t.Run("invalid bid proof", func(t *testing.T) {
		p := util.NewBeaconBlockGloas()
		p.Block.Body.SignedExecutionPayloadBid.Signature = []byte{1}
		block, err := blocks.NewSignedBeaconBlock(p)
		require.NoError(t, err)
		_, _, err = makeExecutionAndProofGloas(t.Context(), block)
		require.ErrorContains(t, "could not get execution block hash proof", err)
	})
	t.Run("invalid legacy payload", func(t *testing.T) {
		p := util.NewBeaconBlockCapella()
		p.Block.Body.ExecutionPayload.BlockHash = []byte{1}
		block, err := blocks.NewSignedBeaconBlock(p)
		require.NoError(t, err)
		_, _, err = makeExecutionAndProofGloas(t.Context(), block)
		require.ErrorContains(t, "could not get execution payload proof", err)
	})
}
