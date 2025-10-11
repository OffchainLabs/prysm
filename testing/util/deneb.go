package util

import (
	"encoding/binary"
	"math/big"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	"github.com/OffchainLabs/prysm/v6/crypto/random"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	GoKZG "github.com/crate-crypto/go-kzg-4844"
	"github.com/ethereum/go-ethereum/common"
	gethTypes "github.com/ethereum/go-ethereum/core/types"
)

type DenebBlockGeneratorOption func(*denebBlockGenerator)

type denebBlockGenerator struct {
	parent   [32]byte
	slot     primitives.Slot
	nblobs   int
	sign     bool
	sk       bls.SecretKey
	proposer primitives.ValidatorIndex
	valRoot  []byte
	payload  *enginev1.ExecutionPayloadDeneb
}

func WithProposerSigning(idx primitives.ValidatorIndex, sk bls.SecretKey, valRoot []byte) DenebBlockGeneratorOption {
	return func(g *denebBlockGenerator) {
		g.sign = true
		g.proposer = idx
		g.sk = sk
		g.valRoot = valRoot
	}
}

func WithPayloadSetter(p *enginev1.ExecutionPayloadDeneb) DenebBlockGeneratorOption {
	return func(g *denebBlockGenerator) {
		g.payload = p
	}
}

func WithDenebSlot(slot primitives.Slot) DenebBlockGeneratorOption {
	return func(g *denebBlockGenerator) {
		g.slot = slot
	}
}

func GenerateTestDenebBlockWithSidecar(t *testing.T, parent [32]byte, slot primitives.Slot, nblobs int, opts ...DenebBlockGeneratorOption) (blocks.ROBlock, []blocks.ROBlob) {
	g := &denebBlockGenerator{
		parent: parent,
		slot:   slot,
		nblobs: nblobs,
	}
	for _, o := range opts {
		o(g)
	}

	if g.payload == nil {
		stateRoot := bytesutil.PadTo([]byte("stateRoot"), fieldparams.RootLength)
		ads := common.HexToAddress("095e7baea6a6c7c4c2dfeb977efac326af552d87")
		tx := gethTypes.NewTx(&gethTypes.LegacyTx{
			Nonce:    0,
			To:       &ads,
			Value:    big.NewInt(0),
			Gas:      0,
			GasPrice: big.NewInt(0),
			Data:     nil,
		})

		txs := []*gethTypes.Transaction{tx}
		encodedBinaryTxs := make([][]byte, 1)
		var err error
		encodedBinaryTxs[0], err = txs[0].MarshalBinary()
		require.NoError(t, err)
		blockHash := bytesutil.ToBytes32([]byte("foo"))
		logsBloom := bytesutil.PadTo([]byte("logs"), fieldparams.LogsBloomLength)
		receiptsRoot := bytesutil.PadTo([]byte("receiptsRoot"), fieldparams.RootLength)
		parentHash := bytesutil.PadTo([]byte("parentHash"), fieldparams.RootLength)
		g.payload = &enginev1.ExecutionPayloadDeneb{
			ParentHash:    parentHash,
			FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
			StateRoot:     stateRoot,
			ReceiptsRoot:  receiptsRoot,
			LogsBloom:     logsBloom,
			PrevRandao:    blockHash[:],
			BlockNumber:   0,
			GasLimit:      0,
			GasUsed:       0,
			Timestamp:     0,
			ExtraData:     make([]byte, 0),
			BaseFeePerGas: bytesutil.PadTo([]byte("baseFeePerGas"), fieldparams.RootLength),
			BlockHash:     blockHash[:],
			Transactions:  encodedBinaryTxs,
			Withdrawals:   make([]*enginev1.Withdrawal, 0),
			BlobGasUsed:   0,
			ExcessBlobGas: 0,
		}
	}

	block := NewBeaconBlockDeneb()
	block.Block.Body.ExecutionPayload = g.payload
	block.Block.Slot = g.slot
	block.Block.ParentRoot = g.parent[:]
	block.Block.ProposerIndex = g.proposer
	commitments := make([][48]byte, g.nblobs)
	block.Block.Body.BlobKzgCommitments = make([][]byte, g.nblobs)
	for i := range commitments {
		binary.LittleEndian.PutUint16(commitments[i][0:16], uint16(i))
		binary.LittleEndian.PutUint16(commitments[i][16:32], uint16(g.slot))
		block.Block.Body.BlobKzgCommitments[i] = commitments[i][:]
	}

	body, err := blocks.NewBeaconBlockBody(block.Block.Body)
	require.NoError(t, err)
	inclusion := make([][][]byte, len(commitments))
	for i := range commitments {
		proof, err := blocks.MerkleProofKZGCommitment(body, i)
		require.NoError(t, err)
		inclusion[i] = proof
	}
	if g.sign {
		epoch := slots.ToEpoch(block.Block.Slot)
		fork := params.ForkFromConfig(params.BeaconConfig(), epoch)
		domain := params.BeaconConfig().DomainBeaconProposer
		sig, err := signing.ComputeDomainAndSignWithoutState(fork, epoch, domain, g.valRoot, block.Block, g.sk)
		require.NoError(t, err)
		block.Signature = sig
	}

	root, err := block.Block.HashTreeRoot()
	require.NoError(t, err)

	sidecars := make([]blocks.ROBlob, len(commitments))
	sbb, err := blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)

	sh, err := sbb.Header()
	require.NoError(t, err)
	for i, c := range block.Block.Body.BlobKzgCommitments {
		sidecars[i] = GenerateTestDenebBlobSidecar(t, root, sh, i, c, inclusion[i])
	}

	rob, err := blocks.NewROBlock(sbb)
	require.NoError(t, err)
	return rob, sidecars
}

func GenerateTestDenebBlobSidecar(t *testing.T, root [32]byte, header *ethpb.SignedBeaconBlockHeader, index int, commitment []byte, incProof [][]byte) blocks.ROBlob {
	blob := make([]byte, fieldparams.BlobSize)
	binary.LittleEndian.PutUint64(blob, uint64(index))
	pb := &ethpb.BlobSidecar{
		SignedBlockHeader: header,
		Index:             uint64(index),
		Blob:              blob,
		KzgCommitment:     commitment,
		KzgProof:          commitment,
	}
	if len(incProof) == 0 {
		incProof = fakeEmptyProof(t, pb)
	}
	pb.CommitmentInclusionProof = incProof
	r, err := blocks.NewROBlobWithRoot(pb, root)
	require.NoError(t, err)
	return r
}

func fakeEmptyProof(_ *testing.T, _ *ethpb.BlobSidecar) [][]byte {
	r := make([][]byte, fieldparams.KzgCommitmentInclusionProofDepth)
	for i := range r {
		r[i] = make([]byte, fieldparams.RootLength)
	}
	return r
}

func ExtendBlocksPlusBlobs(t *testing.T, blks []blocks.ROBlock, size int) ([]blocks.ROBlock, []blocks.ROBlob) {
	deneb := params.BeaconConfig().DenebForkEpoch
	denebSlot := SlotAtEpoch(t, deneb)
	blobs := make([]blocks.ROBlob, 0)
	if len(blks) == 0 {
		blk, blb := GenerateTestDenebBlockWithSidecar(t, [32]byte{}, denebSlot, 6)
		blobs = append(blobs, blb...)
		blks = append(blks, blk)
	}

	for i := 0; i < size; i++ {
		prev := blks[len(blks)-1]
		blk, blb := GenerateTestDenebBlockWithSidecar(t, prev.Root(), prev.Block().Slot()+1, 6)
		blobs = append(blobs, blb...)
		blks = append(blks, blk)
	}

	return blks, blobs
}

func DeterministicRandomness(seed int64) [32]byte {
	return random.DeterministicRandomness(seed)
}

// Returns a serialized random field element in big-endian
func GetRandFieldElement(seed int64) [32]byte {
	return random.GetRandFieldElement(seed)
}

// Returns a random blob using the passed seed as entropy
func GetRandBlob(seed int64) GoKZG.Blob {
	return random.GetRandBlob(seed)
}
