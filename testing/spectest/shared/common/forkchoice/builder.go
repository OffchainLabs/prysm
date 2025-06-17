package forkchoice

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/execution"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/ethereum/go-ethereum/common"
)

type Builder struct {
	service  *blockchain.Service
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
	bvw := verification.NewInitializerWaiter(cw, fc, sg, verification.WithForkLookup(getFork))
	return &Builder{
		service:  service,
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
		require.DeepEqual(t, common.FromHex(c.Head.Root), r)
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
}
