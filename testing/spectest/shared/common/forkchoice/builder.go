package forkchoice

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/execution"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common"
)

type Builder struct {
	service  *blockchain.Service
	fc       forkchoice.ForkChoicer
	lastTick int64
	execMock *engineMock
	vwait    *verification.InitializerWaiter
}

func NewBuilder(t testing.TB, initialState state.BeaconState, initialBlock interfaces.ReadOnlySignedBeaconBlock) *Builder {
	execMock := &engineMock{
		powBlocks: make(map[[32]byte]*ethpb.PowBlock),
	}
	cw := startup.NewClockSynchronizer()
	service, sg, fc := startChainService(t, initialState, initialBlock, execMock, cw)
	// blob spectests use a weird Fork in the genesis beacon state that has different previous and current versions.
	// This trips up the lite fork lookup code in the blob verifier that figures out the fork
	// based on the slot of the block. So just for spectests we override that behavior and get the fork from the state
	// which matches the behavior of block verification.
	getFork := func(targetEpoch primitives.Epoch) (*ethpb.Fork, error) {
		return initialState.Fork(), nil
	}
	bvw := verification.NewInitializerWaiter(cw, fc, sg, service, verification.WithForkLookup(getFork))
	return &Builder{
		service:  service,
		fc:       fc,
		execMock: execMock,
		vwait:    bvw,
	}
}

// Tick resets the genesis time to now()-tick and adjusts the slot to the appropriate value.
func (bb *Builder) Tick(t testing.TB, tick int64) {
	bb.service.SetGenesisTime(time.Unix(time.Now().Unix()-tick, 0))
	lastSlot := uint64(bb.lastTick) / params.BeaconConfig().SecondsPerSlot
	currentSlot := uint64(tick) / params.BeaconConfig().SecondsPerSlot
	for lastSlot < currentSlot {
		lastSlot++
		bb.service.SetForkChoiceGenesisTime(time.Now().Add(-1 * time.Duration(params.BeaconConfig().SecondsPerSlot*lastSlot) * time.Second))
		require.NoError(t, bb.service.NewSlot(t.Context(), primitives.Slot(lastSlot)))
	}
	if tick > int64(params.BeaconConfig().SecondsPerSlot*lastSlot) {
		bb.service.SetForkChoiceGenesisTime(time.Now().Add(-1 * time.Duration(tick) * time.Second))
	}
	bb.lastTick = tick
}

// SetPayloadStatus sets the payload status that the engine will return
func (bb *Builder) SetPayloadStatus(resp *MockEngineResp) error {
	if resp == nil {
		return errors.New("invalid nil payload status")
	}
	if resp.LatestValidHash == nil {
		bb.execMock.latestValidHash = common.FromHex("0x0000000000000000000000000000000000000000000000000000000000000000")
	} else {
		bb.execMock.latestValidHash = common.FromHex(*resp.LatestValidHash)
	}
	if resp.Status == nil {
		return errors.New("invalid nil status")
	}
	switch *resp.Status {
	case "SYNCING":
		bb.execMock.payloadStatus = execution.ErrAcceptedSyncingPayloadStatus
	case "VALID":
		bb.execMock.payloadStatus = nil
	case "INVALID":
		bb.execMock.payloadStatus = execution.ErrInvalidPayloadStatus
	default:
		return errors.New("unknown payload status")
	}
	return nil
}

// block returns the block root.
func (bb *Builder) block(t testing.TB, b interfaces.ReadOnlySignedBeaconBlock) [32]byte {
	r, err := b.Block().HashTreeRoot()
	require.NoError(t, err)
	return r
}

// InvalidBlock receives the invalid block and notifies forkchoice.
func (bb *Builder) InvalidBlock(t testing.TB, b interfaces.ReadOnlySignedBeaconBlock) {
	r := bb.block(t, b)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	require.Equal(t, true, bb.service.ReceiveBlock(ctx, b, r, nil) != nil)
}

// ValidBlock receives the valid block and notifies forkchoice.
func (bb *Builder) ValidBlock(t testing.TB, b interfaces.ReadOnlySignedBeaconBlock) {
	r := bb.block(t, b)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	require.NoError(t, bb.service.ReceiveBlock(ctx, b, r, nil))
}

// PoWBlock receives the block and notifies a mocked execution engine.
func (bb *Builder) PoWBlock(pb *ethpb.PowBlock) {
	bb.execMock.powBlocks[bytesutil.ToBytes32(pb.BlockHash)] = pb
}

// Attestation receives the attestation and updates forkchoice.
func (bb *Builder) Attestation(t testing.TB, a ethpb.Att) {
	require.NoError(t, bb.service.OnAttestation(context.TODO(), a, params.BeaconConfig().MaximumGossipClockDisparityDuration()))
}

// AttesterSlashing receives an attester slashing and feeds it to forkchoice.
func (bb *Builder) AttesterSlashing(s *ethpb.AttesterSlashing) {
	slashings := []ethpb.AttSlashing{s}
	bb.service.InsertSlashingsToForkChoiceStore(context.TODO(), slashings)
}

// ExecutionPayloadEnvelope feeds a signed execution payload envelope to the chain
// service. When valid is false, an error is required from the receiver.
func (bb *Builder) ExecutionPayloadEnvelope(t testing.TB, env interfaces.ROSignedExecutionPayloadEnvelope, valid bool) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	err := bb.service.ReceiveExecutionPayloadEnvelope(ctx, env)
	if valid {
		require.NoError(t, err)
		return
	}
	require.Equal(t, true, err != nil)
}

// PayloadAttestation feeds a payload attestation message to the chain service.
// When valid is false, an error is required from the receiver.
func (bb *Builder) PayloadAttestation(t testing.TB, msg *ethpb.PayloadAttestationMessage, valid bool) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	err := bb.service.ReceivePayloadAttestationMessage(ctx, msg)
	if valid {
		require.NoError(t, err)
		return
	}
	require.Equal(t, true, err != nil)
}

// Check evaluates the fork choice results and compares them to the expected values.
func (bb *Builder) Check(t testing.TB, c *Check) {
	if c == nil {
		return
	}
	ctx := t.Context()
	require.NoError(t, bb.service.UpdateAndSaveHeadWithBalances(ctx))
	if c.Head != nil {
		r, err := bb.service.HeadRoot(ctx)
		require.NoError(t, err)
		wantedRoot := common.FromHex(c.Head.Root)
		require.Equal(t, true, bytes.Equal(wantedRoot, r), fmt.Sprintf("Roots differ. wanted %#x, got %#x", wantedRoot, r))
		require.Equal(t, primitives.Slot(c.Head.Slot), bb.service.HeadSlot())
	}
	if c.JustifiedCheckPoint != nil {
		cp := &ethpb.Checkpoint{
			Epoch: primitives.Epoch(c.JustifiedCheckPoint.Epoch),
			Root:  common.FromHex(c.JustifiedCheckPoint.Root),
		}
		got := bb.service.CurrentJustifiedCheckpt()
		require.DeepEqual(t, cp, got)
	}
	if c.FinalizedCheckPoint != nil {
		cp := &ethpb.Checkpoint{
			Epoch: primitives.Epoch(c.FinalizedCheckPoint.Epoch),
			Root:  common.FromHex(c.FinalizedCheckPoint.Root),
		}
		got := bb.service.FinalizedCheckpt()
		require.DeepSSZEqual(t, cp, got)
	}
	if c.ProposerBoostRoot != nil {
		want := fmt.Sprintf("%#x", common.FromHex(*c.ProposerBoostRoot))
		got := fmt.Sprintf("%#x", bb.service.ProposerBoost())
		require.Equal(t, want, got)
	}
	if c.GetProposerHead != nil {
		want := fmt.Sprintf("%#x", common.FromHex(*c.GetProposerHead))
		got := fmt.Sprintf("%#x", bb.service.GetProposerHead())
		require.Equal(t, want, got)
	}
	/* TODO: We need to mock the entire proposer system to be able to test this.
	if c.ShouldOverrideFCU != nil {
		require.DeepEqual(t, c.ShouldOverrideFCU.Result, bb.service.ShouldOverrideFCU())
	}
	*/
	if c.Time != nil {
		// The compliance-runner emits `time` in every checks block, asserting the
		// current fork-choice time in seconds since genesis. Our builder's lastTick
		// is exactly that value — it's the argument passed to the most recent Tick.
		require.Equal(t, int64(*c.Time), bb.lastTick)
	}
	if c.HeadPayloadStatus != nil {
		headRoot, err := bb.service.HeadRoot(ctx)
		require.NoError(t, err)
		var status int
		if bb.fc.HasFullNode(bytesutil.ToBytes32(headRoot)) {
			status = 1
		}
		require.Equal(t, *c.HeadPayloadStatus, status)
	}
	if c.ViableForHeadRootsAndWeights != nil {
		checkViableHeads(t, bb.fc, c.ViableForHeadRootsAndWeights)
	}
}

// checkViableHeads compares the expected {root, weight} multiset against the
// set reported by the forkchoicer's Tips + Weight accessors. Comparison is
// order-insensitive: both sides are keyed by root into maps before comparing
// key-by-key (require.DeepEqual routes through go-cmp which panics on maps).
func checkViableHeads(t testing.TB, fc forkchoice.ForkChoicer, expected []RootWeight) {
	tips, _ := fc.Tips()
	got := make(map[[32]byte]uint64, len(tips))
	for _, root := range tips {
		w, err := fc.Weight(root)
		require.NoError(t, err)
		got[root] = w
	}
	want := make(map[[32]byte]uint64, len(expected))
	for _, rw := range expected {
		want[bytesutil.ToBytes32(common.FromHex(rw.Root))] = rw.Weight
	}
	if len(want) != len(got) {
		t.Fatalf("viable_for_head count mismatch: want %d, got %d (want=%x got=%x)", len(want), len(got), want, got)
	}
	for root, w := range want {
		gw, ok := got[root]
		if !ok {
			t.Fatalf("viable_for_head missing root %#x (weight %d)", root, w)
		}
		if gw != w {
			t.Fatalf("viable_for_head weight mismatch at %#x: want %d, got %d", root, w, gw)
		}
	}
}
