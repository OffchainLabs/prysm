package util

import (
	"encoding/binary"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	gethTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/kzg"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/network/forks"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

type FuluBlockGeneratorOption func(*fuluBlockGenerator)

type fuluBlockGenerator struct {
	parent   [32]byte
	slot     primitives.Slot
	nblobs   int
	sign     bool
	sk       bls.SecretKey
	proposer primitives.ValidatorIndex
	valRoot  []byte
	payload  *enginev1.ExecutionPayloadDeneb
}

func WithFuluProposerSigning(idx primitives.ValidatorIndex, sk bls.SecretKey, valRoot []byte) FuluBlockGeneratorOption {
	return func(g *fuluBlockGenerator) {
		g.sign = true
		g.proposer = idx
		g.sk = sk
		g.valRoot = valRoot
	}
}

func WithFuluPayload(p *enginev1.ExecutionPayloadDeneb) FuluBlockGeneratorOption {
	return func(g *fuluBlockGenerator) {
		g.payload = p
	}
}

func GenerateTestFuluBlockWithSidecars(
	t *testing.T,
	parent [32]byte,
	slot primitives.Slot,
	nblobs int,
	opts ...FuluBlockGeneratorOption,
) (
	blocks.ROBlock,
	[]blocks.RODataColumn,
) {
	g := &fuluBlockGenerator{
		parent: parent,
		slot:   slot,
		nblobs: nblobs,
	}
	for _, o := range opts {
		o(g)
	}

	if g.payload == nil {
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
		g.payload = &enginev1.ExecutionPayloadDeneb{
			ParentHash:    bytesutil.PadTo([]byte("parentHash"), fieldparams.RootLength),
			FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
			StateRoot:     bytesutil.PadTo([]byte("stateRoot"), fieldparams.RootLength),
			ReceiptsRoot:  bytesutil.PadTo([]byte("receiptsRoot"), fieldparams.RootLength),
			LogsBloom:     bytesutil.PadTo([]byte("logs"), fieldparams.LogsBloomLength),
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

	block := NewBeaconBlockFulu()
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

	sbb, err := blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)

	sh, err := sbb.Header()
	require.NoError(t, err)
	blobs := make([]kzg.Blob, nblobs)
	for i, comt := range block.Block.Body.BlobKzgCommitments {
		blobs[i] = kzg.Blob(GenerateTestDenebBlobSidecar(t, root, sh, i, comt, inclusion[i]).Blob)
	}
	sidecars := make([]blocks.RODataColumn, 0, params.BeaconConfig().NumberOfColumns)
	cellsAndProofs := GenerateCellsAndProofs(t, blobs)
	dataColumns, err := peerdas.DataColumnSidecars(sbb, cellsAndProofs)
	require.NoError(t, err)
	for _, dataColumn := range dataColumns {
		sidecar, err := blocks.NewRODataColumnWithRoot(dataColumn, root)
		require.NoError(t, err)

		sidecars = append(sidecars, sidecar)
	}

	rob, err := blocks.NewROBlockWithRoot(sbb, root)
	require.NoError(t, err)

	return rob, sidecars
}

func GenerateCellsAndProofs(t *testing.T, blobs []kzg.Blob) []kzg.CellsAndProofs {
	cellsAndProofs := make([]kzg.CellsAndProofs, len(blobs))
	for i := range blobs {
		cp, err := kzg.ComputeCellsAndKZGProofs(&blobs[i])
		require.NoError(t, err)
		cellsAndProofs[i] = cp
	}
	return cellsAndProofs
}
