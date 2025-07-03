package util

import (
	"math/big"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/ethereum/go-ethereum/common"
	gethTypes "github.com/ethereum/go-ethereum/core/types"
)

type ElectraBlockGeneratorOption func(*electraBlockGenerator)

type electraBlockGenerator struct {
	parent   [32]byte
	slot     primitives.Slot
	nblobs   int
	sign     bool
	sk       bls.SecretKey
	proposer primitives.ValidatorIndex
	valRoot  []byte
	payload  *enginev1.ExecutionPayloadDeneb
}

func WithElectraPayload(p *enginev1.ExecutionPayloadDeneb) ElectraBlockGeneratorOption {
	return func(g *electraBlockGenerator) {
		g.payload = p
	}
}

func GenerateTestElectraBlockWithSidecar(t *testing.T, parent [32]byte, slot primitives.Slot, nblobs int, opts ...ElectraBlockGeneratorOption) (blocks.ROBlock, []blocks.ROBlob) {
	g := &electraBlockGenerator{
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

	block := NewBeaconBlockElectra()
	block.Block.Body.ExecutionPayload = g.payload
	block.Block.Slot = g.slot
	block.Block.ParentRoot = g.parent[:]
	block.Block.ProposerIndex = g.proposer

	blobs := make([][]byte, 0, g.nblobs)
	commitments := make([][]byte, 0, g.nblobs)
	kzgProofs := make([][]byte, 0, g.nblobs)

	for i := range g.nblobs {
		blob := kzg.Blob{uint8(i)}

		commitment, err := kzg.BlobToKZGCommitment(&blob)
		require.NoError(t, err)

		kzgProof, err := kzg.ComputeBlobKZGProof(&blob, commitment)
		require.NoError(t, err)

		blobs = append(blobs, blob[:])
		commitments = append(commitments, commitment[:])
		kzgProofs = append(kzgProofs, kzgProof[:])
	}

	block.Block.Body.BlobKzgCommitments = commitments

	body, err := blocks.NewBeaconBlockBody(block.Block.Body)
	require.NoError(t, err)

	inclusionProofs := make([][][]byte, 0, g.nblobs)
	for i := range g.nblobs {
		inclusionProof, err := blocks.MerkleProofKZGCommitment(body, i)
		require.NoError(t, err)

		inclusionProofs = append(inclusionProofs, inclusionProof)
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

	sbb, err := blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)

	sh, err := sbb.Header()
	require.NoError(t, err)

	roSidecars := make([]blocks.ROBlob, 0, g.nblobs)
	for i := range g.nblobs {
		pbSidecar := ethpb.BlobSidecar{
			Index:                    uint64(i),
			Blob:                     blobs[i],
			KzgCommitment:            commitments[i],
			KzgProof:                 kzgProofs[i],
			SignedBlockHeader:        sh,
			CommitmentInclusionProof: inclusionProofs[i],
		}

		roSidecar, err := blocks.NewROBlobWithRoot(&pbSidecar, root)
		require.NoError(t, err)

		roSidecars = append(roSidecars, roSidecar)
	}

	rob, err := blocks.NewROBlock(sbb)
	require.NoError(t, err)
	return rob, roSidecars
}
