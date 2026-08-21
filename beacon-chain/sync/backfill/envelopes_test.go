package backfill

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/iface"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/kv"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/proto/dbval"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/interop"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
)

// envChain is a linked chain of Gloas blocks with consistent bids, plus the key material needed
// to build and sign envelopes for them.
type envChain struct {
	blks       []blocks.ROBlock
	sks        []bls.SecretKey
	keys       [][fieldparams.BLSPubkeyLength]byte
	builderSks []bls.SecretKey
	builders   []*ethpb.Builder
	vr         []byte
	domain     *domainCache
}

type envChainCfg struct {
	start primitives.Slot
	n     int
	// withheld marks block indices whose payload is withheld: the child block's bid then
	// commits to the last revealed hash instead of the parent's block hash.
	withheld map[int]bool
	// builderAt returns the bid builder index for block i; nil means every block is self-built.
	builderAt func(i int) primitives.BuilderIndex
}

func testEnvHash(tag string, i int) [32]byte {
	return bytesutil.ToBytes32([]byte(fmt.Sprintf("%s-%d", tag, i)))
}

func makeEnvChain(t *testing.T, cfg envChainCfg) *envChain {
	t.Helper()
	sks, pks, err := interop.DeterministicallyGenerateKeys(0, uint64(cfg.n))
	require.NoError(t, err)
	builderSks, builderPks, err := interop.DeterministicallyGenerateKeys(1000, 4)
	require.NoError(t, err)
	keys := make([][fieldparams.BLSPubkeyLength]byte, len(pks))
	for i := range pks {
		keys[i] = bytesutil.ToBytes48(pks[i].Marshal())
	}
	builders := make([]*ethpb.Builder, len(builderPks))
	for i := range builderPks {
		builders[i] = &ethpb.Builder{Pubkey: builderPks[i].Marshal(), DepositEpoch: 0}
	}
	vr := bytesutil.PadTo([]byte("envelope-test-root"), 32)
	dc, err := newDomainCache(vr, params.BeaconConfig().DomainBeaconBuilder)
	require.NoError(t, err)

	reqRoot, err := (&enginev1.ExecutionRequestsGloas{}).HashTreeRoot()
	require.NoError(t, err)

	c := &envChain{sks: sks, keys: keys, builderSks: builderSks, builders: builders, vr: vr, domain: dc}
	prevRoot := [32]byte{}
	prevEffHash := testEnvHash("genesis-el", 0)
	for i := 0; i < cfg.n; i++ {
		bh := testEnvHash("block-hash", i)
		blk := util.NewBeaconBlockGloas()
		blk.Block.Slot = cfg.start + primitives.Slot(i)
		blk.Block.ProposerIndex = primitives.ValidatorIndex(i)
		blk.Block.ParentRoot = bytesutil.SafeCopyBytes(prevRoot[:])
		bid := blk.Block.Body.SignedExecutionPayloadBid.Message
		bid.Slot = blk.Block.Slot
		bid.BuilderIndex = params.BeaconConfig().BuilderIndexSelfBuild
		if cfg.builderAt != nil {
			bid.BuilderIndex = cfg.builderAt(i)
		}
		bid.BlockHash = bh[:]
		bid.ParentBlockHash = bytesutil.SafeCopyBytes(prevEffHash[:])
		bid.ParentBlockRoot = bytesutil.SafeCopyBytes(prevRoot[:])
		bid.ExecutionRequestsRoot = bytesutil.SafeCopyBytes(reqRoot[:])
		sb, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		rob, err := blocks.NewROBlock(sb)
		require.NoError(t, err)
		c.blks = append(c.blks, rob)
		prevRoot = rob.Root()
		if !cfg.withheld[i] {
			prevEffHash = bh
		}
	}
	return c
}

func testGloasPayload(slot primitives.Slot, blockHash, parentHash []byte) *enginev1.ExecutionPayloadGloas {
	return &enginev1.ExecutionPayloadGloas{
		ParentHash:    parentHash,
		FeeRecipient:  make([]byte, 20),
		StateRoot:     make([]byte, 32),
		ReceiptsRoot:  make([]byte, 32),
		LogsBloom:     make([]byte, 256),
		PrevRandao:    make([]byte, 32),
		BlockNumber:   uint64(slot),
		BaseFeePerGas: make([]byte, 32),
		BlockHash:     blockHash,
		SlotNumber:    slot,
	}
}

// envelope builds a signed envelope for block i, signed by signer over the builder domain for
// the block's epoch. Passing a nil signer selects the correct signer for the bid's builder index.
func (c *envChain) envelope(t *testing.T, i int, signer bls.SecretKey) *ethpb.SignedExecutionPayloadEnvelope {
	t.Helper()
	b := c.blks[i]
	bid, err := b.Block().Body().SignedExecutionPayloadBid()
	require.NoError(t, err)
	root := b.Root()
	parentRoot := b.Block().ParentRoot()
	msg := &ethpb.ExecutionPayloadEnvelope{
		Payload:               testGloasPayload(b.Block().Slot(), bid.Message.BlockHash, bid.Message.ParentBlockHash),
		ExecutionRequests:     &enginev1.ExecutionRequestsGloas{},
		BuilderIndex:          bid.Message.BuilderIndex,
		BeaconBlockRoot:       root[:],
		ParentBeaconBlockRoot: parentRoot[:],
	}
	if signer == nil {
		if bid.Message.BuilderIndex == params.BeaconConfig().BuilderIndexSelfBuild {
			signer = c.sks[b.Block().ProposerIndex()]
		} else {
			signer = c.builderSks[bid.Message.BuilderIndex]
		}
	}
	dom, err := c.domain.forEpoch(slots.ToEpoch(b.Block().Slot()))
	require.NoError(t, err)
	sr, err := signing.ComputeSigningRoot(msg, dom)
	require.NoError(t, err)
	return &ethpb.SignedExecutionPayloadEnvelope{Message: msg, Signature: signer.Sign(sr[:]).Marshal()}
}

func (c *envChain) reconPayloads(t *testing.T, idx ...int) map[[32]byte]*enginev1.ExecutionPayloadGloas {
	t.Helper()
	out := make(map[[32]byte]*enginev1.ExecutionPayloadGloas)
	for _, i := range idx {
		b := c.blks[i]
		bid, err := b.Block().Body().SignedExecutionPayloadBid()
		require.NoError(t, err)
		out[bytesutil.ToBytes32(bid.Message.BlockHash)] = testGloasPayload(b.Block().Slot(), bid.Message.BlockHash, bid.Message.ParentBlockHash)
	}
	return out
}

type mockReconstructor struct {
	payloads map[[32]byte]*enginev1.ExecutionPayloadGloas
	err      error
	// failBatched mimics the real engine client, whose batched call fails as a whole when any
	// one requested body is unavailable; single-hash calls still succeed.
	failBatched bool
	calls       int
}

func (m *mockReconstructor) ReconstructFullGloasExecutionPayloadsByHash(_ context.Context, hashes [][32]byte) (map[[32]byte]*enginev1.ExecutionPayloadGloas, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	out := make(map[[32]byte]*enginev1.ExecutionPayloadGloas, len(hashes))
	for _, h := range hashes {
		p, ok := m.payloads[h]
		if !ok {
			if m.failBatched && len(hashes) > 1 {
				return nil, errors.New("payload bodies unavailable")
			}
			continue
		}
		out[h] = p
	}
	return out, nil
}

type downscoreRecorder struct {
	calls []string
}

func (d *downscoreRecorder) fn(_ peer.ID, reason string, _ error) {
	d.calls = append(d.calls, reason)
}

type scriptedFetcher struct {
	reqs      []*ethpb.ExecutionPayloadEnvelopesByRangeRequest
	responses [][]*ethpb.SignedExecutionPayloadEnvelope
	errs      []error
}

func (f *scriptedFetcher) fetch(_ context.Context, _ peer.ID, req *ethpb.ExecutionPayloadEnvelopesByRangeRequest) ([]*ethpb.SignedExecutionPayloadEnvelope, error) {
	i := len(f.reqs)
	f.reqs = append(f.reqs, req)
	var err error
	if i < len(f.errs) {
		err = f.errs[i]
	}
	var resp []*ethpb.SignedExecutionPayloadEnvelope
	if i < len(f.responses) {
		resp = f.responses[i]
	}
	return resp, err
}

func wideEnvWindow() das.CurrentNeeds {
	return das.CurrentNeeds{Env: das.NeedSpan{Begin: 1, End: primitives.Slot(1 << 40)}}
}

func testEnvSyncCfg(t *testing.T, c *envChain, recon EnvelopeReconstructor, ds *downscoreRecorder) *envelopeSyncCfg {
	t.Helper()
	v, err := newEnvelopeVerifier(c.vr, c.keys, c.builders)
	require.NoError(t, err)
	return &envelopeSyncCfg{
		verifier:      v,
		reconstructor: recon,
		hasEnvelope:   func(context.Context, [32]byte) bool { return false },
		currentNeeds:  wideEnvWindow,
		downscore:     ds.fn,
		maxAttempts:   3,
		attemptBudget: time.Hour,
		pace:          time.Nanosecond,
		localDelay:    time.Millisecond,
	}
}

func TestEnvelopeExpectations(t *testing.T) {
	ctx := t.Context()
	t.Run("revealed slots expect, withheld slots do not", func(t *testing.T) {
		// Blocks 0 and 2 revealed (committed by children 1 and 3), block 1 withheld.
		// Block 3 is the batch tail with an unclassified child.
		c := makeEnvChain(t, envChainCfg{start: 10, n: 4, withheld: map[int]bool{1: true}})
		es, err := newEnvelopeSync(ctx, c.blks, testEnvSyncCfg(t, c, &mockReconstructor{}, &downscoreRecorder{}))
		require.NoError(t, err)
		require.Equal(t, 3, es.unresolved())
		require.NotNil(t, es.pending[10])
		require.IsNil(t, es.pending[11]) // withheld expects nothing
		require.NotNil(t, es.pending[12])
		require.NotNil(t, es.pending[13]) // tail stays pending for opportunistic fetch
		require.Equal(t, true, es.pending[10].required)
		require.Equal(t, false, es.pending[13].required)
		require.NotNil(t, es.tail)
	})
	t.Run("already stored slots expect nothing", func(t *testing.T) {
		c := makeEnvChain(t, envChainCfg{start: 10, n: 3})
		cfg := testEnvSyncCfg(t, c, &mockReconstructor{}, &downscoreRecorder{})
		stored := c.blks[0].Root()
		cfg.hasEnvelope = func(_ context.Context, root [32]byte) bool { return root == stored }
		es, err := newEnvelopeSync(ctx, c.blks, cfg)
		require.NoError(t, err)
		require.IsNil(t, es.pending[10])
		require.NotNil(t, es.pending[11])
	})
	t.Run("slots outside the Env window expect nothing", func(t *testing.T) {
		c := makeEnvChain(t, envChainCfg{start: 10, n: 3})
		cfg := testEnvSyncCfg(t, c, &mockReconstructor{}, &downscoreRecorder{})
		cfg.currentNeeds = func() das.CurrentNeeds { return das.CurrentNeeds{Env: das.NeedSpan{Begin: 12, End: 100}} }
		es, err := newEnvelopeSync(ctx, c.blks, cfg)
		require.NoError(t, err)
		require.Equal(t, 1, es.unresolved())
		require.NotNil(t, es.pending[12])
	})
	t.Run("pre-Gloas blocks expect nothing", func(t *testing.T) {
		blks, _, _, _ := testBlocksWithKeys(t, 3, 0, make([]byte, 32))
		c := makeEnvChain(t, envChainCfg{start: 10, n: 1})
		es, err := newEnvelopeSync(ctx, blks, testEnvSyncCfg(t, c, &mockReconstructor{}, &downscoreRecorder{}))
		require.NoError(t, err)
		require.Equal(t, 0, es.unresolved())
	})
	t.Run("fullness classification works without column or blob work", func(t *testing.T) {
		// Blob-less Gloas blocks with empty Blob/Col windows still produce expectations.
		c := makeEnvChain(t, envChainCfg{start: 10, n: 3})
		cfg := testEnvSyncCfg(t, c, &mockReconstructor{}, &downscoreRecorder{})
		cfg.currentNeeds = func() das.CurrentNeeds {
			return das.CurrentNeeds{Env: das.NeedSpan{Begin: 1, End: 1 << 40}} // Blob and Col spans empty
		}
		es, err := newEnvelopeSync(ctx, c.blks, cfg)
		require.NoError(t, err)
		require.Equal(t, 3, es.unresolved())
	})
}

func TestEnvelopeExpectationsUnverifiable(t *testing.T) {
	ctx := t.Context()
	t.Run("reused builder index signs historical envelope - skipped not accepted", func(t *testing.T) {
		// The registry snapshot occupant of index 0 deposited after the envelope's epoch, so the
		// occupant at snapshot time is not the builder that signed at the envelope's slot. The
		// slot must be skipped without a request, even though the occupant's signature would
		// verify against the snapshot key.
		c := makeEnvChain(t, envChainCfg{start: 100, n: 4, builderAt: func(int) primitives.BuilderIndex { return 0 }})
		c.builders[0].DepositEpoch = slots.ToEpoch(100) + 1
		batch, child := c.blks[:3], c.blks[3]
		cfg := testEnvSyncCfg(t, c, &mockReconstructor{}, &downscoreRecorder{})
		es, err := newEnvelopeSync(ctx, batch, cfg)
		require.NoError(t, err)
		require.Equal(t, 0, es.unresolved())
		require.Equal(t, 2, len(es.skips[envSkipSigUnverifiable]))
		// The unverifiable tail is not fetched; its skip is recorded once classified revealed.
		require.NotNil(t, es.tail)
		require.Equal(t, envSkipSigUnverifiable, es.tail.skipReason)
		// No expectations means no pages, so nothing is ever requested from a peer.
		f := &scriptedFetcher{}
		es.fetchPass(ctx, "peer-a", f.fetch)
		require.Equal(t, 0, len(f.reqs))
		mdb := &mockBackfillDB{}
		st := testFinalizeStore(t, mdb, batch[len(batch)-1], child)
		es.finalize(ctx, st.store, bytesutil.ToBytes32(st.status().LowRoot))
		require.Equal(t, 3, len(es.skips[envSkipSigUnverifiable]))
		require.Equal(t, 0, len(mdb.envelopes))
	})
	t.Run("missing builder index is unverifiable", func(t *testing.T) {
		c := makeEnvChain(t, envChainCfg{start: 100, n: 2, builderAt: func(int) primitives.BuilderIndex { return 99 }})
		es, err := newEnvelopeSync(ctx, c.blks, testEnvSyncCfg(t, c, &mockReconstructor{}, &downscoreRecorder{}))
		require.NoError(t, err)
		require.Equal(t, 0, es.unresolved())
		require.Equal(t, 1, len(es.skips[envSkipSigUnverifiable]))
		require.NotNil(t, es.tail)
		require.Equal(t, envSkipSigUnverifiable, es.tail.skipReason)
	})
	t.Run("withheld unverifiable tail records no skip", func(t *testing.T) {
		// Slot 102's payload is withheld and its builder is unknown: after classification it
		// expects nothing, so no skip may be recorded for it.
		c := makeEnvChain(t, envChainCfg{start: 100, n: 4, withheld: map[int]bool{2: true}, builderAt: func(i int) primitives.BuilderIndex {
			if i == 2 {
				return 99
			}
			return primitives.BuilderIndex(params.BeaconConfig().BuilderIndexSelfBuild)
		}})
		batch, child := c.blks[:3], c.blks[3]
		recon := &mockReconstructor{payloads: c.reconPayloads(t, 0, 1)}
		es, err := newEnvelopeSync(ctx, batch, testEnvSyncCfg(t, c, recon, &downscoreRecorder{}))
		require.NoError(t, err)
		require.NotNil(t, es.tail)
		mdb := &mockBackfillDB{}
		st := testFinalizeStore(t, mdb, batch[len(batch)-1], child)
		es.finalize(ctx, st.store, bytesutil.ToBytes32(st.status().LowRoot))
		require.Equal(t, 0, len(es.skips[envSkipSigUnverifiable]))
		require.Equal(t, 0, len(es.skips[envSkipTailUnresolved]))
	})
	t.Run("builder deposited strictly before envelope epoch is verifiable", func(t *testing.T) {
		c := makeEnvChain(t, envChainCfg{start: 100, n: 2, builderAt: func(int) primitives.BuilderIndex { return 1 }})
		c.builders[1].DepositEpoch = slots.ToEpoch(100) - 1
		es, err := newEnvelopeSync(ctx, c.blks, testEnvSyncCfg(t, c, &mockReconstructor{}, &downscoreRecorder{}))
		require.NoError(t, err)
		require.Equal(t, 2, es.unresolved())
	})
	t.Run("builder deposited in the envelope epoch is unverifiable", func(t *testing.T) {
		// A builder is only active in epochs strictly after its deposit epoch, so a
		// deposit_epoch == envelope_epoch occupant could be a later reuse of the index.
		c := makeEnvChain(t, envChainCfg{start: 100, n: 2, builderAt: func(int) primitives.BuilderIndex { return 1 }})
		c.builders[1].DepositEpoch = slots.ToEpoch(100)
		es, err := newEnvelopeSync(ctx, c.blks, testEnvSyncCfg(t, c, &mockReconstructor{}, &downscoreRecorder{}))
		require.NoError(t, err)
		require.Equal(t, 0, es.unresolved())
		require.Equal(t, 1, len(es.skips[envSkipSigUnverifiable]))
	})
}

func TestEnvelopeTailClassificationAtBuild(t *testing.T) {
	ctx := t.Context()
	t.Run("boundary child says revealed - tail becomes required", func(t *testing.T) {
		c := makeEnvChain(t, envChainCfg{start: 10, n: 4})
		batch, child := c.blks[:3], c.blks[3]
		cfg := testEnvSyncCfg(t, c, &mockReconstructor{}, &downscoreRecorder{})
		cfg.boundaryChild = func(context.Context, [32]byte) (interfaces.ReadOnlyBeaconBlock, error) {
			return child.Block(), nil
		}
		es, err := newEnvelopeSync(ctx, batch, cfg)
		require.NoError(t, err)
		require.IsNil(t, es.tail)
		require.NotNil(t, es.pending[12])
		require.Equal(t, true, es.pending[12].required)
	})
	t.Run("boundary child says withheld - tail expects nothing", func(t *testing.T) {
		c := makeEnvChain(t, envChainCfg{start: 10, n: 4, withheld: map[int]bool{2: true}})
		batch, child := c.blks[:3], c.blks[3]
		cfg := testEnvSyncCfg(t, c, &mockReconstructor{}, &downscoreRecorder{})
		cfg.boundaryChild = func(context.Context, [32]byte) (interfaces.ReadOnlyBeaconBlock, error) {
			return child.Block(), nil
		}
		es, err := newEnvelopeSync(ctx, batch, cfg)
		require.NoError(t, err)
		require.IsNil(t, es.tail)
		require.IsNil(t, es.pending[12])
	})
	t.Run("boundary child unavailable - classification deferred", func(t *testing.T) {
		c := makeEnvChain(t, envChainCfg{start: 10, n: 3})
		cfg := testEnvSyncCfg(t, c, &mockReconstructor{}, &downscoreRecorder{})
		cfg.boundaryChild = func(context.Context, [32]byte) (interfaces.ReadOnlyBeaconBlock, error) {
			return nil, nil
		}
		es, err := newEnvelopeSync(ctx, c.blks, cfg)
		require.NoError(t, err)
		require.NotNil(t, es.tail)
		require.NotNil(t, es.pending[12])
		require.Equal(t, false, es.pending[12].required)
	})
}

func TestEnvelopeFetchAndVerify(t *testing.T) {
	ctx := t.Context()
	t.Run("self-build verifies with the historical proposer key", func(t *testing.T) {
		c := makeEnvChain(t, envChainCfg{start: 10, n: 3})
		recon := &mockReconstructor{payloads: c.reconPayloads(t, 0, 1, 2)}
		ds := &downscoreRecorder{}
		es, err := newEnvelopeSync(ctx, c.blks, testEnvSyncCfg(t, c, recon, ds))
		require.NoError(t, err)
		require.Equal(t, 3, es.unresolved())
		f := &scriptedFetcher{responses: [][]*ethpb.SignedExecutionPayloadEnvelope{{
			c.envelope(t, 0, nil), c.envelope(t, 1, nil), c.envelope(t, 2, nil),
		}}}
		es.fetchPass(ctx, "peer-a", f.fetch)
		require.Equal(t, 0, es.unresolved())
		require.Equal(t, 3, len(es.held))
		require.Equal(t, 0, len(ds.calls))
		// Held envelopes are blinded: the payload is reduced to its block hash.
		bid, err := c.blks[0].Block().Body().SignedExecutionPayloadBid()
		require.NoError(t, err)
		require.DeepEqual(t, bid.Message.BlockHash, es.held[10].blinded.Message.BlockHash)
		require.Equal(t, primitives.Slot(10), es.held[10].blinded.Message.Slot)
	})
	t.Run("self-build signed by a different validator key fails and rotates", func(t *testing.T) {
		// The signer stands in for the snapshot state's latest proposer: verification must use
		// the historical block's proposer key, so any other key must fail.
		c := makeEnvChain(t, envChainCfg{start: 10, n: 2})
		recon := &mockReconstructor{payloads: c.reconPayloads(t, 0, 1)}
		ds := &downscoreRecorder{}
		es, err := newEnvelopeSync(ctx, c.blks, testEnvSyncCfg(t, c, recon, ds))
		require.NoError(t, err)
		f := &scriptedFetcher{responses: [][]*ethpb.SignedExecutionPayloadEnvelope{
			{c.envelope(t, 0, c.sks[1])}, // wrong signer
			{c.envelope(t, 0, nil)},      // retry succeeds
		}}
		es.fetchPass(ctx, "peer-a", f.fetch)
		require.Equal(t, 1, len(ds.calls))
		require.NotNil(t, es.pending[10]) // aggregate failure leaves the page unresolved for another peer
		es.fetchPass(ctx, "peer-b", f.fetch)
		require.IsNil(t, es.pending[10])
		require.NotNil(t, es.held[10])
	})
	t.Run("registry builder key verifies", func(t *testing.T) {
		c := makeEnvChain(t, envChainCfg{start: 100, n: 2, builderAt: func(int) primitives.BuilderIndex { return 2 }})
		recon := &mockReconstructor{payloads: c.reconPayloads(t, 0, 1)}
		ds := &downscoreRecorder{}
		es, err := newEnvelopeSync(ctx, c.blks, testEnvSyncCfg(t, c, recon, ds))
		require.NoError(t, err)
		f := &scriptedFetcher{responses: [][]*ethpb.SignedExecutionPayloadEnvelope{{
			c.envelope(t, 0, nil), c.envelope(t, 1, nil),
		}}}
		es.fetchPass(ctx, "peer-a", f.fetch)
		require.Equal(t, 0, es.unresolved())
		require.Equal(t, 2, len(es.held))
		require.Equal(t, 0, len(ds.calls))
	})
	t.Run("builder domain lookup uses the fork schedule", func(t *testing.T) {
		// An envelope signed over the proposer domain must fail against the builder domain.
		c := makeEnvChain(t, envChainCfg{start: 10, n: 2})
		recon := &mockReconstructor{payloads: c.reconPayloads(t, 0, 1)}
		ds := &downscoreRecorder{}
		es, err := newEnvelopeSync(ctx, c.blks, testEnvSyncCfg(t, c, recon, ds))
		require.NoError(t, err)
		env := c.envelope(t, 0, nil)
		wrongDc, err := newDomainCache(c.vr, params.BeaconConfig().DomainBeaconProposer)
		require.NoError(t, err)
		dom, err := wrongDc.forEpoch(slots.ToEpoch(10))
		require.NoError(t, err)
		sr, err := signing.ComputeSigningRoot(env.Message, dom)
		require.NoError(t, err)
		env.Signature = c.sks[0].Sign(sr[:]).Marshal()
		f := &scriptedFetcher{responses: [][]*ethpb.SignedExecutionPayloadEnvelope{{env}}}
		es.fetchPass(ctx, "peer-a", f.fetch)
		require.Equal(t, 1, len(ds.calls))
		require.NotNil(t, es.pending[10])
	})
	t.Run("bid binding mismatch is a peer offense", func(t *testing.T) {
		c := makeEnvChain(t, envChainCfg{start: 10, n: 2})
		recon := &mockReconstructor{payloads: c.reconPayloads(t, 0, 1)}
		ds := &downscoreRecorder{}
		es, err := newEnvelopeSync(ctx, c.blks, testEnvSyncCfg(t, c, recon, ds))
		require.NoError(t, err)
		env := c.envelope(t, 0, nil)
		env.Message.Payload.BlockHash = bytesutil.PadTo([]byte("tampered"), 32)
		f := &scriptedFetcher{responses: [][]*ethpb.SignedExecutionPayloadEnvelope{{env}}}
		es.fetchPass(ctx, "peer-a", f.fetch)
		require.Equal(t, 1, len(ds.calls))
		require.NotNil(t, es.pending[10])
		require.Equal(t, 0, len(es.held))
	})
	t.Run("EL reconstruction failure skips the slot and is not a peer offense", func(t *testing.T) {
		c := makeEnvChain(t, envChainCfg{start: 10, n: 3})
		recon := &mockReconstructor{err: errors.New("EL down")}
		ds := &downscoreRecorder{}
		es, err := newEnvelopeSync(ctx, c.blks, testEnvSyncCfg(t, c, recon, ds))
		require.NoError(t, err)
		f := &scriptedFetcher{responses: [][]*ethpb.SignedExecutionPayloadEnvelope{{
			c.envelope(t, 0, nil), c.envelope(t, 1, nil),
		}}}
		es.fetchPass(ctx, "peer-a", f.fetch)
		// Bounded local retries of the batched call, then one isolation call per hash.
		require.Equal(t, envelopeLocalRetries+2, recon.calls)
		require.Equal(t, 2, len(es.skips[envSkipELFailed]))
		require.Equal(t, 0, len(ds.calls))
		// The tail (slot 12) got no envelope and no skip; it awaits import-time classification.
		require.IsNil(t, es.pending[10])
		require.IsNil(t, es.pending[11])
		require.NotNil(t, es.tail)
	})
	t.Run("one unavailable EL body does not discard the rest of the page", func(t *testing.T) {
		c := makeEnvChain(t, envChainCfg{start: 10, n: 4})
		// The body for slot 11 is unavailable; the real engine client fails the whole batched
		// call in that case, so the fallback must reconstruct the others individually.
		recon := &mockReconstructor{payloads: c.reconPayloads(t, 0, 2, 3), failBatched: true}
		ds := &downscoreRecorder{}
		es, err := newEnvelopeSync(ctx, c.blks, testEnvSyncCfg(t, c, recon, ds))
		require.NoError(t, err)
		f := &scriptedFetcher{responses: [][]*ethpb.SignedExecutionPayloadEnvelope{{
			c.envelope(t, 0, nil), c.envelope(t, 1, nil), c.envelope(t, 2, nil), c.envelope(t, 3, nil),
		}}}
		es.fetchPass(ctx, "peer-a", f.fetch)
		// Bounded batched retries plus one isolation call per hash.
		require.Equal(t, envelopeLocalRetries+4, recon.calls)
		require.NotNil(t, es.held[10])
		require.IsNil(t, es.held[11])
		require.NotNil(t, es.held[12])
		require.NotNil(t, es.held[13])
		require.Equal(t, 1, len(es.skips[envSkipELFailed]))
		require.Equal(t, primitives.Slot(11), es.skips[envSkipELFailed][0])
		require.Equal(t, 0, len(ds.calls))
	})
	t.Run("EL payload HTR mismatch after successful reconstruction is a peer offense", func(t *testing.T) {
		c := makeEnvChain(t, envChainCfg{start: 10, n: 2})
		payloads := c.reconPayloads(t, 0, 1)
		for _, p := range payloads {
			p.GasUsed++ // EL disagrees with the fetched payload bytes
		}
		recon := &mockReconstructor{payloads: payloads}
		ds := &downscoreRecorder{}
		es, err := newEnvelopeSync(ctx, c.blks, testEnvSyncCfg(t, c, recon, ds))
		require.NoError(t, err)
		f := &scriptedFetcher{responses: [][]*ethpb.SignedExecutionPayloadEnvelope{{c.envelope(t, 0, nil)}}}
		es.fetchPass(ctx, "peer-a", f.fetch)
		require.Equal(t, 1, len(ds.calls))
		require.NotNil(t, es.pending[10]) // retryable from another peer
		require.Equal(t, 0, len(es.skips[envSkipELFailed]))
	})
	t.Run("peer exhaustion takes a terminal skip after bounded attempts", func(t *testing.T) {
		c := makeEnvChain(t, envChainCfg{start: 10, n: 3})
		recon := &mockReconstructor{payloads: c.reconPayloads(t, 0, 1, 2)}
		ds := &downscoreRecorder{}
		es, err := newEnvelopeSync(ctx, c.blks, testEnvSyncCfg(t, c, recon, ds))
		require.NoError(t, err)
		f := &scriptedFetcher{} // always returns empty responses, which are protocol-legal
		for i := 0; i < envelopeMaxPageAttempts; i++ {
			require.Equal(t, true, es.unresolved() > 0)
			es.fetchPass(ctx, "peer-a", f.fetch)
		}
		require.Equal(t, envelopeMaxPageAttempts, len(f.reqs))
		require.Equal(t, 0, es.unresolved())
		require.Equal(t, 2, len(es.skips[envSkipPeerExhausted])) // required slots 10, 11
		require.Equal(t, 0, len(ds.calls))                       // empty responses are never downscored
		require.NotNil(t, es.tail)                               // tail classification deferred to import
	})
	t.Run("elapsed budget expires a page", func(t *testing.T) {
		c := makeEnvChain(t, envChainCfg{start: 10, n: 2})
		recon := &mockReconstructor{payloads: c.reconPayloads(t, 0, 1)}
		cfg := testEnvSyncCfg(t, c, recon, &downscoreRecorder{})
		cfg.attemptBudget = time.Millisecond
		es, err := newEnvelopeSync(ctx, c.blks, cfg)
		require.NoError(t, err)
		f := &scriptedFetcher{}
		es.fetchPass(ctx, "peer-a", f.fetch)
		require.Equal(t, 1, len(f.reqs))
		now := time.Now()
		es.now = func() time.Time { return now.Add(time.Hour) } // waiting does not extend the budget
		es.fetchPass(ctx, "peer-b", f.fetch)
		require.Equal(t, 1, len(f.reqs)) // no second attempt after expiry
		require.Equal(t, 0, es.unresolved())
		require.Equal(t, 1, len(es.skips[envSkipPeerExhausted]))
	})
	t.Run("unexpected slots in the response are ignored", func(t *testing.T) {
		c := makeEnvChain(t, envChainCfg{start: 10, n: 3, withheld: map[int]bool{1: true}})
		recon := &mockReconstructor{payloads: c.reconPayloads(t, 0, 1, 2)}
		ds := &downscoreRecorder{}
		es, err := newEnvelopeSync(ctx, c.blks, testEnvSyncCfg(t, c, recon, ds))
		require.NoError(t, err)
		// Slot 11 is withheld and expects nothing, but a malicious peer serves a leaked envelope.
		f := &scriptedFetcher{responses: [][]*ethpb.SignedExecutionPayloadEnvelope{{
			c.envelope(t, 0, nil), c.envelope(t, 1, nil),
		}}}
		es.fetchPass(ctx, "peer-a", f.fetch)
		require.NotNil(t, es.held[10])
		require.IsNil(t, es.held[11])
		require.Equal(t, 0, len(ds.calls))
	})
}

// testFinalizeStore builds a Store whose backfill status links the batch tail to the boundary child.
func testFinalizeStore(t *testing.T, mdb *mockBackfillDB, batchTail, child blocks.ROBlock) *Store {
	t.Helper()
	if mdb.blocks == nil {
		mdb.blocks = make(map[[32]byte]blocks.ROBlock)
	}
	mdb.blocks[child.Root()] = child
	tailRoot := batchTail.Root()
	return &Store{
		store: mdb,
		bs: &dbval.BackfillStatus{
			LowSlot:       uint64(child.Block().Slot()),
			LowRoot:       child.RootSlice(),
			LowParentRoot: tailRoot[:],
		},
	}
}

func TestEnvelopeFinalizeTailClassification(t *testing.T) {
	ctx := t.Context()
	setup := func(t *testing.T, withheldTail bool) (*envChain, *Store, *mockBackfillDB, *envelopeSync) {
		wh := map[int]bool{}
		if withheldTail {
			wh[2] = true
		}
		c := makeEnvChain(t, envChainCfg{start: 10, n: 4, withheld: wh})
		batch, child := c.blks[:3], c.blks[3]
		mdb := &mockBackfillDB{}
		st := testFinalizeStore(t, mdb, batch[len(batch)-1], child)
		recon := &mockReconstructor{payloads: c.reconPayloads(t, 0, 1, 2)}
		es, err := newEnvelopeSync(ctx, batch, testEnvSyncCfg(t, c, recon, &downscoreRecorder{}))
		require.NoError(t, err)
		require.NotNil(t, es.tail)
		return c, st, mdb, es
	}
	t.Run("revealed tail with held envelope is persisted", func(t *testing.T) {
		c, st, mdb, es := setup(t, false)
		f := &scriptedFetcher{responses: [][]*ethpb.SignedExecutionPayloadEnvelope{{
			c.envelope(t, 0, nil), c.envelope(t, 1, nil), c.envelope(t, 2, nil),
		}}}
		es.fetchPass(ctx, "peer-a", f.fetch)
		require.Equal(t, 3, len(es.held))
		es.finalize(ctx, st.store, bytesutil.ToBytes32(st.status().LowRoot))
		require.Equal(t, 3, len(mdb.envelopes))
		require.Equal(t, true, mdb.HasExecutionPayloadEnvelope(ctx, c.blks[2].Root()))
	})
	t.Run("revealed tail without held envelope is peer_exhausted", func(t *testing.T) {
		c, st, mdb, es := setup(t, false)
		f := &scriptedFetcher{responses: [][]*ethpb.SignedExecutionPayloadEnvelope{{
			c.envelope(t, 0, nil), c.envelope(t, 1, nil),
		}}}
		es.fetchPass(ctx, "peer-a", f.fetch)
		es.fetchPass(ctx, "peer-b", f.fetch)
		es.fetchPass(ctx, "peer-c", f.fetch)
		require.Equal(t, 0, es.unresolved())
		es.finalize(ctx, st.store, bytesutil.ToBytes32(st.status().LowRoot))
		require.Equal(t, 2, len(mdb.envelopes))
		require.Equal(t, false, mdb.HasExecutionPayloadEnvelope(ctx, c.blks[2].Root()))
		require.Equal(t, 1, len(es.skips[envSkipPeerExhausted]))
	})
	t.Run("withheld tail drops a leaked envelope", func(t *testing.T) {
		c, st, mdb, es := setup(t, true)
		// A malicious peer leaks a validly signed envelope for the withheld tail; it verifies
		// statelessly, but classification at import must drop it.
		f := &scriptedFetcher{responses: [][]*ethpb.SignedExecutionPayloadEnvelope{{
			c.envelope(t, 0, nil), c.envelope(t, 1, nil), c.envelope(t, 2, nil),
		}}}
		es.fetchPass(ctx, "peer-a", f.fetch)
		require.NotNil(t, es.held[12])
		es.finalize(ctx, st.store, bytesutil.ToBytes32(st.status().LowRoot))
		require.Equal(t, false, mdb.HasExecutionPayloadEnvelope(ctx, c.blks[2].Root()))
		require.Equal(t, 2, len(mdb.envelopes))
		require.Equal(t, 0, len(es.skips[envSkipTailUnresolved]))
	})
	t.Run("tail lookup failure takes bounded retries then tail_unresolved", func(t *testing.T) {
		c, st, mdb, es := setup(t, false)
		calls := 0
		mdb.block = func(context.Context, [32]byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
			calls++
			return nil, errors.New("db unavailable")
		}
		f := &scriptedFetcher{responses: [][]*ethpb.SignedExecutionPayloadEnvelope{{
			c.envelope(t, 0, nil), c.envelope(t, 1, nil), c.envelope(t, 2, nil),
		}}}
		es.fetchPass(ctx, "peer-a", f.fetch)
		es.finalize(ctx, st.store, bytesutil.ToBytes32(st.status().LowRoot))
		require.Equal(t, envelopeLocalRetries, calls)
		require.Equal(t, 1, len(es.skips[envSkipTailUnresolved]))
		// Non-tail envelopes still persist.
		require.Equal(t, 2, len(mdb.envelopes))
	})
}

func TestFillBackEnvelopeOutcomes(t *testing.T) {
	ctx := t.Context()
	setup := func(t *testing.T) (*envChain, *Store, *mockBackfillDB, *envelopeSync) {
		c := makeEnvChain(t, envChainCfg{start: 10, n: 4})
		batch, child := c.blks[:3], c.blks[3]
		mdb := &mockBackfillDB{}
		st := testFinalizeStore(t, mdb, batch[len(batch)-1], child)
		recon := &mockReconstructor{payloads: c.reconPayloads(t, 0, 1, 2)}
		es, err := newEnvelopeSync(ctx, batch, testEnvSyncCfg(t, c, recon, &downscoreRecorder{}))
		require.NoError(t, err)
		f := &scriptedFetcher{responses: [][]*ethpb.SignedExecutionPayloadEnvelope{{
			c.envelope(t, 0, nil), c.envelope(t, 1, nil), c.envelope(t, 2, nil),
		}}}
		es.fetchPass(ctx, "peer-a", f.fetch)
		require.Equal(t, 3, len(es.held))
		return c, st, mdb, es
	}
	t.Run("envelope db failure takes bounded retries then db_failed while blocks still import", func(t *testing.T) {
		c, st, mdb, es := setup(t)
		saves := 0
		mdb.saveBlindedEnvelope = func(context.Context, *ethpb.SignedBlindedExecutionPayloadEnvelope) (iface.EnvelopeSaveOutcome, error) {
			saves++
			return iface.EnvelopeSaveInserted, errors.New("disk full")
		}
		bs, err := st.fillBack(ctx, 20, c.blks[:3], &das.MockAvailabilityStore{}, es)
		require.NoError(t, err)
		require.Equal(t, envelopeLocalRetries*3, saves)
		require.Equal(t, 3, len(es.skips[envSkipDBFailed]))
		// Normal block import was still attempted and succeeded.
		require.Equal(t, uint64(10), bs.LowSlot)
		require.NotNil(t, mdb.blocks[c.blks[0].Root()])
	})
	t.Run("fillBack persists envelopes before block writes", func(t *testing.T) {
		c, st, mdb, es := setup(t)
		bs, err := st.fillBack(ctx, 20, c.blks[:3], &das.MockAvailabilityStore{}, es)
		require.NoError(t, err)
		require.Equal(t, uint64(10), bs.LowSlot)
		require.Equal(t, 3, len(mdb.envelopes))
	})
	t.Run("duplicate save is a byte-identical no-op and conflicting stored envelope is kept", func(t *testing.T) {
		c, st, mdb, es := setup(t)
		// Pre-store a conflicting envelope for slot 10 and a byte-identical one for slot 11.
		conflicting := kv.BlindEnvelope(c.envelope(t, 0, nil))
		conflicting.Message.BuilderIndex = 7
		_, err := mdb.SaveBlindedExecutionPayloadEnvelope(ctx, conflicting)
		require.NoError(t, err)
		_, err = mdb.SaveBlindedExecutionPayloadEnvelope(ctx, kv.BlindEnvelope(c.envelope(t, 1, nil)))
		require.NoError(t, err)
		_, err = st.fillBack(ctx, 20, c.blks[:3], &das.MockAvailabilityStore{}, es)
		require.NoError(t, err)
		// The conflicting entry was kept, the fetched copy dropped.
		stored := mdb.envelopes[c.blks[0].Root()]
		require.Equal(t, primitives.BuilderIndex(7), stored.Message.BuilderIndex)
		// All three slots have envelopes and no skips were recorded.
		require.Equal(t, 3, len(mdb.envelopes))
		require.Equal(t, 0, len(es.skips[envSkipDBFailed]))
	})
}

// TestBatchTransitionEnvelopes exercises the batch state machine routing through the envelope
// stage: a batch with unresolved envelope work returns to batchSyncEnvelopes (so the pool
// reassigns it, rotating peers), and proceeds to batchImportable once every slot is resolved.
func TestBatchTransitionEnvelopes(t *testing.T) {
	ctx := t.Context()
	c := makeEnvChain(t, envChainCfg{start: 10, n: 2})
	recon := &mockReconstructor{payloads: c.reconPayloads(t, 0, 1)}
	es, err := newEnvelopeSync(ctx, c.blks, testEnvSyncCfg(t, c, recon, &downscoreRecorder{}))
	require.NoError(t, err)
	require.Equal(t, 2, es.unresolved())

	b := batch{blocks: c.blks, columns: &columnSync{}, envelopes: es}
	b = b.transitionToNext()
	require.Equal(t, batchSyncEnvelopes, b.state)

	// A pass that resolves nothing keeps the batch in the envelope stage for another peer.
	empty := &scriptedFetcher{}
	b.envelopes.fetchPass(ctx, "peer-a", empty.fetch)
	b = b.transitionToNext()
	require.Equal(t, batchSyncEnvelopes, b.state)

	// Resolving everything moves the batch to importable.
	f := &scriptedFetcher{responses: [][]*ethpb.SignedExecutionPayloadEnvelope{{
		c.envelope(t, 0, nil), c.envelope(t, 1, nil),
	}}}
	b.envelopes.fetchPass(ctx, "peer-b", f.fetch)
	b = b.transitionToNext()
	require.Equal(t, batchImportable, b.state)

	// A batch with no envelope sync at all (stage disabled) is importable immediately.
	nb := batch{blocks: c.blks, columns: &columnSync{}}
	nb = nb.transitionToNext()
	require.Equal(t, batchImportable, nb.state)
}
