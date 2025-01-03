package util

import (
	"encoding/binary"
	"math"
	"math/big"
	"testing"

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
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

// HackElectraMaxuint is helpful for tests that need to set up cases where the electra fork has passed.
// We have unit tests that assert our config matches the upstream config, where the next fork is always
// set to MaxUint64 until the fork epoch is formally set. This creates an issue for tests that want to
// work with slots that are defined to be after electra because converting the max epoch to a slot leads
// to multiplication overflow.
// Monkey patching tests with this function is the simplest workaround in these cases.
func HackElectraMaxuint(t *testing.T) func() {
	bc := params.MainnetConfig().Copy()
	bc.ElectraForkEpoch = math.MaxUint32
	undo, err := params.SetActiveWithUndo(bc)
	require.NoError(t, err)
	return func() {
		require.NoError(t, undo())
	}
}

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

func WithElectraProposerSigning(idx primitives.ValidatorIndex, sk bls.SecretKey, valRoot []byte) ElectraBlockGeneratorOption {
	return func(g *electraBlockGenerator) {
		g.sign = true
		g.proposer = idx
		g.sk = sk
		g.valRoot = valRoot
	}
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
