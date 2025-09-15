package util

import (
	"math/big"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/ethereum/go-ethereum/common"
	gethTypes "github.com/ethereum/go-ethereum/core/types"
)

type FuluBlockGeneratorOption func(*fuluBlockGenerator)

type fuluBlockGenerator struct {
	parent    [32]byte
	slot      primitives.Slot
	blobCount int
	sign      bool
	sk        bls.SecretKey
	proposer  primitives.ValidatorIndex
	valRoot   []byte
	payload   *enginev1.ExecutionPayloadDeneb
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

func WithParentRoot(root [fieldparams.RootLength]byte) FuluBlockGeneratorOption {
	return func(g *fuluBlockGenerator) {
		g.parent = root
	}
}

func WithSlot(slot primitives.Slot) FuluBlockGeneratorOption {
	return func(g *fuluBlockGenerator) {
		g.slot = slot
	}
}

func GenerateTestFuluBlockWithSidecars(t *testing.T, blobCount int, options ...FuluBlockGeneratorOption) (blocks.ROBlock, []blocks.RODataColumn, []blocks.VerifiedRODataColumn) {
	generator := &fuluBlockGenerator{blobCount: blobCount}

	for _, option := range options {
		option(generator)
	}

	if generator.payload == nil {
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

		generator.payload = &enginev1.ExecutionPayloadDeneb{
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
	block.Block.Body.ExecutionPayload = generator.payload
	block.Block.Slot = generator.slot
	block.Block.ParentRoot = generator.parent[:]
	block.Block.ProposerIndex = generator.proposer

	blobs := make([]kzg.Blob, 0, generator.blobCount)
	commitments := make([][]byte, 0, generator.blobCount)

	for i := range generator.blobCount {
		blob := kzg.Blob{uint8(i)}

		commitment, err := kzg.BlobToKZGCommitment(&blob)
		require.NoError(t, err)

		blobs = append(blobs, blob)
		commitments = append(commitments, commitment[:])
	}

	block.Block.Body.BlobKzgCommitments = commitments

	if generator.sign {
		epoch := slots.ToEpoch(block.Block.Slot)
		fork := params.ForkFromConfig(params.BeaconConfig(), epoch)

		domain := params.BeaconConfig().DomainBeaconProposer
		sig, err := signing.ComputeDomainAndSignWithoutState(fork, epoch, domain, generator.valRoot, block.Block, generator.sk)
		require.NoError(t, err)

		block.Signature = sig
	}

	root, err := block.Block.HashTreeRoot()
	require.NoError(t, err)

	signedBeaconBlock, err := blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)

	cellsAndProofs := GenerateCellsAndProofs(t, blobs)

	rob, err := blocks.NewROBlockWithRoot(signedBeaconBlock, root)
	require.NoError(t, err)
	roSidecars, err := peerdas.DataColumnSidecars(cellsAndProofs, peerdas.PopulateFromBlock(rob))
	require.NoError(t, err)

	verifiedRoSidecars := make([]blocks.VerifiedRODataColumn, 0, len(roSidecars))
	for _, roSidecar := range roSidecars {
		roVerifiedSidecar := blocks.NewVerifiedRODataColumn(roSidecar)

		roSidecars = append(roSidecars, roSidecar)
		verifiedRoSidecars = append(verifiedRoSidecars, roVerifiedSidecar)
	}

	roBlock, err := blocks.NewROBlockWithRoot(signedBeaconBlock, root)
	require.NoError(t, err)

	return roBlock, roSidecars, verifiedRoSidecars
}

func GenerateCellsAndProofs(t testing.TB, blobs []kzg.Blob) []kzg.CellsAndProofs {
	cellsAndProofs := make([]kzg.CellsAndProofs, len(blobs))
	for i := range blobs {
		cp, err := kzg.ComputeCellsAndKZGProofs(&blobs[i])
		require.NoError(t, err)
		cellsAndProofs[i] = cp
	}
	return cellsAndProofs
}
