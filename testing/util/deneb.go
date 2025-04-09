package util

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"math/big"
	"testing"

	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	GoKZG "github.com/crate-crypto/go-kzg-4844"
	"github.com/ethereum/go-ethereum/common"
	gethTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/network/forks"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/sirupsen/logrus"
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
		schedule := forks.NewOrderedSchedule(params.BeaconConfig())
		version, err := schedule.VersionForEpoch(epoch)
		require.NoError(t, err)
		fork, err := schedule.ForkFromVersion(version)
		require.NoError(t, err)
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
	blobs := make([]blocks.ROBlob, 0)
	if len(blks) == 0 {
		blk, blb := GenerateTestDenebBlockWithSidecar(t, [32]byte{}, 0, 6)
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

func deterministicRandomness(seed int64) [32]byte {
	// Converts an int64 to a byte slice
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, seed)
	if err != nil {
		logrus.WithError(err).Error("Failed to write int64 to bytes buffer")
		return [32]byte{}
	}
	bytes := buf.Bytes()

	return sha256.Sum256(bytes)
}

// Returns a serialized random field element in big-endian
func GetRandFieldElement(seed int64) [32]byte {
	bytes := deterministicRandomness(seed)
	var r fr.Element
	r.SetBytes(bytes[:])

	return GoKZG.SerializeScalar(r)
}

// Returns a random blob using the passed seed as entropy
func GetRandBlob(seed int64) GoKZG.Blob {
	var blob GoKZG.Blob
	bytesPerBlob := GoKZG.ScalarsPerBlob * GoKZG.SerializedScalarSize
	for i := 0; i < bytesPerBlob; i += GoKZG.SerializedScalarSize {
		fieldElementBytes := GetRandFieldElement(seed + int64(i))
		copy(blob[i:i+GoKZG.SerializedScalarSize], fieldElementBytes[:])
	}
	return blob
}
