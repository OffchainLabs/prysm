package blockchain

import (
	"fmt"
	"testing"

	coreblocks "github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"google.golang.org/protobuf/proto"
)

const gloasBatchTestGasLimit = 30_000_000

type gloasChainLink struct {
	block    consensusblocks.ROBlock
	envelope *ethpb.SignedExecutionPayloadEnvelope
	state    state.BeaconState
}

// gloasGenesisForBatch returns a Gloas genesis whose latest block header commits to the body of
// the genesis block the database will reconstruct, as a real Gloas genesis distribution does.
func gloasGenesisForBatch(t *testing.T) (state.BeaconState, []bls.SecretKey, [32]byte) {
	t.Helper()
	ctx := t.Context()
	genesisState, keys := util.DeterministicGenesisStateGloas(t, 64)
	genesisBlock, err := coreblocks.NewGenesisBlockForState(ctx, genesisState)
	require.NoError(t, err)
	bodyRoot, err := genesisBlock.Block().Body().HashTreeRoot()
	require.NoError(t, err)
	header := genesisState.LatestBlockHeader()
	header.BodyRoot = bodyRoot[:]
	require.NoError(t, genesisState.SetLatestBlockHeader(header))
	genesisBlock, err = coreblocks.NewGenesisBlockForState(ctx, genesisState)
	require.NoError(t, err)
	genesisRoot, err := genesisBlock.Block().HashTreeRoot()
	require.NoError(t, err)
	return genesisState, keys, genesisRoot
}

// gloasBlock builds a signed self-build block at slot with the envelope revealing its payload.
// parentFull selects whether the bid declares the parent payload delivered.
func gloasBlock(t *testing.T, parent state.BeaconState, parentRoot [32]byte, slot primitives.Slot, blockHash [32]byte, parentFull bool, keys []bls.SecretKey) gloasChainLink {
	t.Helper()
	ctx := t.Context()
	cfg := params.BeaconConfig()
	adv, err := transition.ProcessSlots(ctx, parent.Copy(), slot)
	require.NoError(t, err)
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, adv)
	require.NoError(t, err)
	epoch := slots.ToEpoch(slot)
	reveal, err := util.RandaoReveal(adv, epoch, keys)
	require.NoError(t, err)
	parentBlockHash, err := adv.LatestBlockHash()
	require.NoError(t, err)
	if parentFull {
		parentBid, err := adv.LatestExecutionPayloadBid()
		require.NoError(t, err)
		parentBlockHash = parentBid.BlockHash()
	}
	parentBlockRoot, err := helpers.BlockRootAtSlot(adv, slot-1)
	require.NoError(t, err)
	randaoMix, err := helpers.RandaoMix(adv, epoch)
	require.NoError(t, err)
	emptyRequestsRoot, err := enginev1.EmptyExecutionRequestsHashTreeRoot()
	require.NoError(t, err)

	block := util.HydrateBeaconBlockGloas(&ethpb.BeaconBlockGloas{
		Slot:          slot,
		ProposerIndex: proposerIdx,
		ParentRoot:    parentRoot[:],
		Body: &ethpb.BeaconBlockBodyGloas{
			RandaoReveal: reveal,
			SyncAggregate: &ethpb.SyncAggregate{
				SyncCommitteeBits:      make([]byte, fieldparams.SyncAggregateSyncCommitteeBytesLength),
				SyncCommitteeSignature: common.InfiniteSignature[:],
			},
			SignedExecutionPayloadBid: &ethpb.SignedExecutionPayloadBid{
				Message: &ethpb.ExecutionPayloadBid{
					ParentBlockHash:       parentBlockHash[:],
					ParentBlockRoot:       parentBlockRoot,
					BlockHash:             blockHash[:],
					FeeRecipient:          make([]byte, fieldparams.FeeRecipientLength),
					GasLimit:              gloasBatchTestGasLimit,
					BuilderIndex:          cfg.BuilderIndexSelfBuild,
					Slot:                  slot,
					PrevRandao:            randaoMix,
					ExecutionRequestsRoot: emptyRequestsRoot[:],
				},
				Signature: common.InfiniteSignature[:],
			},
		},
	})
	unsigned, err := consensusblocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockGloas{Block: block, Signature: make([]byte, fieldparams.BLSSignatureLength)})
	require.NoError(t, err)
	stateRoot, err := transition.CalculateStateRoot(ctx, parent.Copy(), unsigned)
	require.NoError(t, err)
	block.StateRoot = stateRoot[:]
	sig, err := signing.ComputeDomainAndSign(adv, epoch, block, cfg.DomainBeaconProposer, keys[proposerIdx])
	require.NoError(t, err)
	signed, err := consensusblocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockGloas{Block: block, Signature: sig})
	require.NoError(t, err)
	ro, err := consensusblocks.NewROBlock(signed)
	require.NoError(t, err)
	post, err := transition.ExecuteStateTransition(ctx, parent.Copy(), signed)
	require.NoError(t, err)

	withdrawals, err := post.PayloadExpectedWithdrawals()
	require.NoError(t, err)
	if withdrawals == nil {
		withdrawals = []*enginev1.Withdrawal{}
	}
	startTime, err := slots.StartTime(post.GenesisTime(), slot)
	require.NoError(t, err)
	root := ro.Root()
	envelope := &ethpb.ExecutionPayloadEnvelope{
		Payload: &enginev1.ExecutionPayloadGloas{
			ParentHash:    parentBlockHash[:],
			FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ReceiptsRoot:  make([]byte, fieldparams.RootLength),
			LogsBloom:     make([]byte, fieldparams.LogsBloomLength),
			PrevRandao:    randaoMix,
			BlockNumber:   uint64(slot),
			GasLimit:      gloasBatchTestGasLimit,
			Timestamp:     uint64(startTime.Unix()),
			ExtraData:     []byte{},
			BaseFeePerGas: make([]byte, fieldparams.RootLength),
			BlockHash:     blockHash[:],
			Transactions:  [][]byte{},
			Withdrawals:   withdrawals,
			SlotNumber:    slot,
		},
		ExecutionRequests:     &enginev1.ExecutionRequestsGloas{},
		BuilderIndex:          cfg.BuilderIndexSelfBuild,
		BeaconBlockRoot:       root[:],
		ParentBeaconBlockRoot: parentRoot[:],
	}
	signedEnvelope := signGloasEnvelope(t, post, envelope, keys[proposerIdx])
	wrapped, err := consensusblocks.WrappedROSignedExecutionPayloadEnvelope(signedEnvelope)
	require.NoError(t, err)
	require.NoError(t, gloas.VerifyExecutionPayloadEnvelope(ctx, post, wrapped))
	return gloasChainLink{block: ro, envelope: signedEnvelope, state: post}
}

func signGloasEnvelope(t *testing.T, st state.BeaconState, envelope *ethpb.ExecutionPayloadEnvelope, key bls.SecretKey) *ethpb.SignedExecutionPayloadEnvelope {
	t.Helper()
	epoch := slots.ToEpoch(envelope.Payload.SlotNumber)
	domain, err := signing.Domain(st.Fork(), epoch, params.BeaconConfig().DomainBeaconBuilder, st.GenesisValidatorsRoot())
	require.NoError(t, err)
	signingRoot, err := signing.ComputeSigningRoot(envelope, domain)
	require.NoError(t, err)
	return &ethpb.SignedExecutionPayloadEnvelope{Message: envelope, Signature: key.Sign(signingRoot[:]).Marshal()}
}

func wrapGloasEnvelopes(t *testing.T, envs ...*ethpb.SignedExecutionPayloadEnvelope) []interfaces.ROSignedExecutionPayloadEnvelope {
	t.Helper()
	out := make([]interfaces.ROSignedExecutionPayloadEnvelope, len(envs))
	for i, e := range envs {
		w, err := consensusblocks.WrappedROSignedExecutionPayloadEnvelope(e)
		require.NoError(t, err)
		out[i] = w
	}
	return out
}

func TestOnBlockBatchGloasEnvelopes(t *testing.T) {
	ctx := t.Context()
	genesisState, keys, genesisRoot := gloasGenesisForBatch(t)

	// a is imported on its own first; b0 builds on a's payload; b1Full and b1Empty are the two
	// successors of b0, differing only in whether they declare b0's payload delivered.
	a := gloasBlock(t, genesisState, genesisRoot, 1, bytesutil.ToBytes32([]byte("payload-a")), true, keys)
	b0 := gloasBlock(t, a.state, a.block.Root(), 2, bytesutil.ToBytes32([]byte("payload-0")), true, keys)
	b1Full := gloasBlock(t, b0.state, b0.block.Root(), 3, bytesutil.ToBytes32([]byte("payload-1")), true, keys)
	b1Empty := gloasBlock(t, b0.state, b0.block.Root(), 3, bytesutil.ToBytes32([]byte("payload-1")), false, keys)
	// b2 builds on b1Empty's empty variant, i.e. b1Empty's payload was withheld and never revealed.
	b2 := gloasBlock(t, b1Empty.state, b1Empty.block.Root(), 4, bytesutil.ToBytes32([]byte("payload-2")), false, keys)

	newService := func(t *testing.T) *Service {
		service, tr := minimalTestService(t)
		require.NoError(t, service.saveGenesisData(tr.ctx, genesisState))
		// Block a declares the genesis payload delivered, so genesis needs a full node.
		service.cfg.ForkChoiceStore.Lock()
		service.cfg.ForkChoiceStore.MarkFullNode(genesisRoot, 0)
		service.cfg.ForkChoiceStore.Unlock()
		require.NoError(t, service.onBlockBatch(tr.ctx, []consensusblocks.ROBlock{a.block}, nil, &das.MockAvailabilityStore{}))
		return service
	}
	withRoot := func(env *ethpb.SignedExecutionPayloadEnvelope, root [32]byte) *ethpb.SignedExecutionPayloadEnvelope {
		c := proto.Clone(env).(*ethpb.SignedExecutionPayloadEnvelope)
		c.Message.BeaconBlockRoot = root[:]
		return c
	}
	// Re-signs the envelope with a key that is not the block proposer's.
	wrongSigner := func(link gloasChainLink) *ethpb.SignedExecutionPayloadEnvelope {
		other := keys[(link.block.Block().ProposerIndex()+1)%primitives.ValidatorIndex(len(keys))]
		msg := proto.Clone(link.envelope.Message).(*ethpb.ExecutionPayloadEnvelope)
		return signGloasEnvelope(t, link.state, msg, other)
	}
	unknownRoot := bytesutil.ToBytes32([]byte("not-a-block-in-this-batch"))

	tests := []struct {
		name      string
		successor gloasChainLink
		extra     []gloasChainLink
		envelopes func() []*ethpb.SignedExecutionPayloadEnvelope
		wantErr   string
		wantFull  map[string]bool
	}{
		{
			name:      "withheld payload in the middle of the batch",
			successor: b1Empty,
			extra:     []gloasChainLink{b2},
			envelopes: func() []*ethpb.SignedExecutionPayloadEnvelope {
				return []*ethpb.SignedExecutionPayloadEnvelope{a.envelope, b0.envelope, b2.envelope}
			},
			wantFull: map[string]bool{"a": true, "b0": true, "b1": false, "b2": true},
		},
		{
			name:      "well formed stream with full successor",
			successor: b1Full,
			envelopes: func() []*ethpb.SignedExecutionPayloadEnvelope {
				return []*ethpb.SignedExecutionPayloadEnvelope{a.envelope, b0.envelope, b1Full.envelope}
			},
			wantFull: map[string]bool{"a": true, "b0": true, "b1": true},
		},
		{
			name:      "well formed stream with empty successor",
			successor: b1Empty,
			envelopes: func() []*ethpb.SignedExecutionPayloadEnvelope {
				return []*ethpb.SignedExecutionPayloadEnvelope{a.envelope, b0.envelope, b1Empty.envelope}
			},
			wantFull: map[string]bool{"a": true, "b0": true, "b1": true},
		},
		{
			name:      "wrong signer on the last envelope",
			successor: b1Full,
			envelopes: func() []*ethpb.SignedExecutionPayloadEnvelope {
				return []*ethpb.SignedExecutionPayloadEnvelope{a.envelope, b0.envelope, wrongSigner(b1Full)}
			},
			wantErr:  "signature verification failed",
			wantFull: map[string]bool{"b0": false, "b1": false},
		},
		{
			name:      "unknown root ahead of a wrong signer with empty successor",
			successor: b1Empty,
			envelopes: func() []*ethpb.SignedExecutionPayloadEnvelope {
				return []*ethpb.SignedExecutionPayloadEnvelope{a.envelope, withRoot(b0.envelope, unknownRoot), wrongSigner(b1Empty)}
			},
			wantErr:  errBatchEnvelopeMismatch.Error(),
			wantFull: map[string]bool{"b0": false, "b1": false},
		},
		{
			name:      "unknown root ahead of a wrong signer with full successor",
			successor: b1Full,
			envelopes: func() []*ethpb.SignedExecutionPayloadEnvelope {
				return []*ethpb.SignedExecutionPayloadEnvelope{a.envelope, withRoot(b0.envelope, unknownRoot), wrongSigner(b1Full)}
			},
			wantErr:  errBatchEnvelopeMismatch.Error(),
			wantFull: map[string]bool{"b0": false, "b1": false},
		},
		{
			name:      "parent envelope naming an unknown root",
			successor: b1Full,
			envelopes: func() []*ethpb.SignedExecutionPayloadEnvelope {
				return []*ethpb.SignedExecutionPayloadEnvelope{withRoot(a.envelope, unknownRoot), b0.envelope, b1Full.envelope}
			},
			wantErr:  errBatchEnvelopeMismatch.Error(),
			wantFull: map[string]bool{"b0": false, "b1": false},
		},
		{
			name:      "trailing envelope naming an unknown root",
			successor: b1Full,
			envelopes: func() []*ethpb.SignedExecutionPayloadEnvelope {
				return []*ethpb.SignedExecutionPayloadEnvelope{a.envelope, b0.envelope, withRoot(b1Full.envelope, unknownRoot)}
			},
			wantErr:  errBatchEnvelopeMismatch.Error(),
			wantFull: map[string]bool{"b0": false, "b1": false},
		},
		{
			name:      "two envelopes naming the same block",
			successor: b1Full,
			envelopes: func() []*ethpb.SignedExecutionPayloadEnvelope {
				return []*ethpb.SignedExecutionPayloadEnvelope{a.envelope, b0.envelope, withRoot(b1Full.envelope, b0.block.Root())}
			},
			wantErr:  errBatchEnvelopeMismatch.Error(),
			wantFull: map[string]bool{"b0": false, "b1": false},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service := newService(t)
			blks := []consensusblocks.ROBlock{b0.block, tc.successor.block}
			roots := map[string][32]byte{"a": a.block.Root(), "b0": b0.block.Root(), "b1": tc.successor.block.Root()}
			for i, link := range tc.extra {
				blks = append(blks, link.block)
				roots[fmt.Sprintf("b%d", i+2)] = link.block.Root()
			}
			err := service.onBlockBatch(ctx, blks, wrapGloasEnvelopes(t, tc.envelopes()...), &das.MockAvailabilityStore{})
			for name, want := range tc.wantFull {
				require.Equal(t, want, service.HasFullNode(roots[name]), "full node for %s", name)
			}
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, tc.wantErr, err)
			}
		})
	}
}
