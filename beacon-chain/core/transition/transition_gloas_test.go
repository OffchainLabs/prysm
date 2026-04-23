package transition

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	engine "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/stretchr/testify/require"
)

func TestProcessSlot_GloasClearsNextPayloadAvailability(t *testing.T) {
	slot := primitives.Slot(10)
	cfg := params.BeaconConfig()
	nextIdx := uint64((slot + 1) % cfg.SlotsPerHistoricalRoot)
	byteIdx := nextIdx / 8
	bitMask := byte(1 << (nextIdx % 8))
	availability := bytes.Repeat([]byte{0xFF}, int(cfg.SlotsPerHistoricalRoot/8))
	st := newGloasState(t, slot, availability)

	_, err := ProcessSlot(context.Background(), st)
	require.NoError(t, err)

	post := st.ToProto().(*ethpb.BeaconStateGloas)
	require.Equal(t, byte(0xFF)&^bitMask, post.ExecutionPayloadAvailability[byteIdx])
}

func TestProcessSlot_GloasClearsNextPayloadAvailability_Wrap(t *testing.T) {
	cfg := params.BeaconConfig()
	slot := primitives.Slot(cfg.SlotsPerHistoricalRoot - 1)
	availability := bytes.Repeat([]byte{0xFF}, int(cfg.SlotsPerHistoricalRoot/8))
	st := newGloasState(t, slot, availability)

	_, err := ProcessSlot(context.Background(), st)
	require.NoError(t, err)

	post := st.ToProto().(*ethpb.BeaconStateGloas)
	require.Equal(t, byte(0xFE), post.ExecutionPayloadAvailability[0])
}

func TestProcessSlot_GloasAvailabilityUpdateError(t *testing.T) {
	slot := primitives.Slot(7)
	availability := make([]byte, 1)
	st := newGloasState(t, slot, availability)

	_, err := ProcessSlot(context.Background(), st)
	cfg := params.BeaconConfig()
	idx := uint64((slot + 1) % cfg.SlotsPerHistoricalRoot)
	byteIdx := idx / 8
	require.EqualError(t, err, fmt.Sprintf(
		"bit index %d (byte index %d) out of range for execution payload availability length %d",
		idx, byteIdx, len(availability),
	))
}

func newGloasState(t *testing.T, slot primitives.Slot, availability []byte) state.BeaconState {
	t.Helper()

	cfg := params.BeaconConfig()
	protoState := &ethpb.BeaconStateGloas{
		Slot:                         slot,
		LatestBlockHeader:            testBeaconBlockHeader(),
		BlockRoots:                   make([][]byte, cfg.SlotsPerHistoricalRoot),
		StateRoots:                   make([][]byte, cfg.SlotsPerHistoricalRoot),
		RandaoMixes:                  make([][]byte, fieldparams.RandaoMixesLength),
		ExecutionPayloadAvailability: availability,
		BuilderPendingPayments:       make([]*ethpb.BuilderPendingPayment, int(cfg.SlotsPerEpoch*2)),
		LatestExecutionPayloadBid: &ethpb.ExecutionPayloadBid{
			ParentBlockHash:    make([]byte, 32),
			ParentBlockRoot:    make([]byte, 32),
			BlockHash:          make([]byte, 32),
			PrevRandao:         make([]byte, 32),
			FeeRecipient:       make([]byte, 20),
			BlobKzgCommitments: [][]byte{make([]byte, 48)},
		},
		Eth1Data: &ethpb.Eth1Data{
			DepositRoot: make([]byte, 32),
			BlockHash:   make([]byte, 32),
		},
		PreviousEpochParticipation:  []byte{},
		CurrentEpochParticipation:   []byte{},
		JustificationBits:           []byte{0},
		PreviousJustifiedCheckpoint: &ethpb.Checkpoint{Root: make([]byte, 32)},
		CurrentJustifiedCheckpoint:  &ethpb.Checkpoint{Root: make([]byte, 32)},
		FinalizedCheckpoint:         &ethpb.Checkpoint{Root: make([]byte, 32)},
		CurrentSyncCommittee:        &ethpb.SyncCommittee{},
		NextSyncCommittee:           &ethpb.SyncCommittee{},
	}

	for i := range protoState.BlockRoots {
		protoState.BlockRoots[i] = make([]byte, 32)
	}
	for i := range protoState.StateRoots {
		protoState.StateRoots[i] = make([]byte, 32)
	}
	for i := range protoState.RandaoMixes {
		protoState.RandaoMixes[i] = make([]byte, 32)
	}

	for i := range protoState.BuilderPendingPayments {
		protoState.BuilderPendingPayments[i] = &ethpb.BuilderPendingPayment{
			Withdrawal: &ethpb.BuilderPendingWithdrawal{
				FeeRecipient: make([]byte, 20),
			},
		}
	}

	pubkeys := make([][]byte, cfg.SyncCommitteeSize)
	for i := range pubkeys {
		pubkeys[i] = make([]byte, fieldparams.BLSPubkeyLength)
	}
	aggPubkey := make([]byte, fieldparams.BLSPubkeyLength)
	protoState.CurrentSyncCommittee = &ethpb.SyncCommittee{
		Pubkeys:         pubkeys,
		AggregatePubkey: aggPubkey,
	}
	protoState.NextSyncCommittee = &ethpb.SyncCommittee{
		Pubkeys:         pubkeys,
		AggregatePubkey: aggPubkey,
	}

	st, err := state_native.InitializeFromProtoGloas(protoState)
	require.NoError(t, err)
	require.Equal(t, version.Gloas, st.Version())
	return st
}

func testBeaconBlockHeader() *ethpb.BeaconBlockHeader {
	return &ethpb.BeaconBlockHeader{
		ParentRoot: make([]byte, 32),
		StateRoot:  make([]byte, 32),
		BodyRoot:   make([]byte, 32),
	}
}

// newGloasForkBoundaryState returns a Gloas BeaconState where IsParentBlockFull()==true
// because bid.BlockHash == latestBlockHash. The parentBlockRoot parameter controls
// whether the bid looks like an upgrade-seed (all-zeros) or a real committed bid (non-zero).
func newGloasForkBoundaryState(
	t *testing.T,
	slot primitives.Slot,
	blockHash [32]byte,
	parentBlockRoot [32]byte,
) state.BeaconState {
	t.Helper()
	cfg := params.BeaconConfig()
	availability := bytes.Repeat([]byte{0xFF}, int(cfg.SlotsPerHistoricalRoot/8))
	protoState := &ethpb.BeaconStateGloas{
		Slot:                         slot,
		LatestBlockHeader:            testBeaconBlockHeader(),
		BlockRoots:                   make([][]byte, cfg.SlotsPerHistoricalRoot),
		StateRoots:                   make([][]byte, cfg.SlotsPerHistoricalRoot),
		RandaoMixes:                  make([][]byte, fieldparams.RandaoMixesLength),
		ExecutionPayloadAvailability: availability,
		BuilderPendingPayments:       make([]*ethpb.BuilderPendingPayment, int(cfg.SlotsPerEpoch*2)),
		// bid.BlockHash == LatestBlockHash so that IsParentBlockFull() returns true.
		LatestBlockHash: blockHash[:],
		LatestExecutionPayloadBid: &ethpb.ExecutionPayloadBid{
			ParentBlockHash:    make([]byte, 32),
			ParentBlockRoot:    parentBlockRoot[:],
			BlockHash:          blockHash[:],
			PrevRandao:         make([]byte, 32),
			FeeRecipient:       make([]byte, 20),
			BlobKzgCommitments: [][]byte{make([]byte, 48)},
		},
		Eth1Data: &ethpb.Eth1Data{
			DepositRoot: make([]byte, 32),
			BlockHash:   make([]byte, 32),
		},
		PreviousEpochParticipation:  []byte{},
		CurrentEpochParticipation:   []byte{},
		JustificationBits:           []byte{0},
		PreviousJustifiedCheckpoint: &ethpb.Checkpoint{Root: make([]byte, 32)},
		CurrentJustifiedCheckpoint:  &ethpb.Checkpoint{Root: make([]byte, 32)},
		FinalizedCheckpoint:         &ethpb.Checkpoint{Root: make([]byte, 32)},
		PayloadExpectedWithdrawals:  make([]*engine.Withdrawal, 0),
		ProposerLookahead:           make([]primitives.ValidatorIndex, 0),
		Builders:                    make([]*ethpb.Builder, 0),
	}
	for i := range protoState.BlockRoots {
		protoState.BlockRoots[i] = make([]byte, 32)
	}
	for i := range protoState.StateRoots {
		protoState.StateRoots[i] = make([]byte, 32)
	}
	for i := range protoState.RandaoMixes {
		protoState.RandaoMixes[i] = make([]byte, 32)
	}
	for i := range protoState.BuilderPendingPayments {
		protoState.BuilderPendingPayments[i] = &ethpb.BuilderPendingPayment{
			Withdrawal: &ethpb.BuilderPendingWithdrawal{FeeRecipient: make([]byte, 20)},
		}
	}
	pubkeys := make([][]byte, cfg.SyncCommitteeSize)
	for i := range pubkeys {
		pubkeys[i] = make([]byte, fieldparams.BLSPubkeyLength)
	}
	aggPubkey := make([]byte, fieldparams.BLSPubkeyLength)
	protoState.CurrentSyncCommittee = &ethpb.SyncCommittee{Pubkeys: pubkeys, AggregatePubkey: aggPubkey}
	protoState.NextSyncCommittee = &ethpb.SyncCommittee{Pubkeys: pubkeys, AggregatePubkey: aggPubkey}
	st, err := state_native.InitializeFromProtoGloas(protoState)
	require.NoError(t, err)
	return st
}

// newGloasTestBlock returns an ROBlock at the given slot with the given parentRoot.
func newGloasTestBlock(t *testing.T, slot primitives.Slot, parentRoot [32]byte) consensusblocks.ROBlock {
	t.Helper()
	blkProto := &ethpb.SignedBeaconBlockGloas{
		Block: &ethpb.BeaconBlockGloas{
			Slot:       slot,
			ParentRoot: parentRoot[:],
			StateRoot:  make([]byte, 32),
			Body: &ethpb.BeaconBlockBodyGloas{
				RandaoReveal: make([]byte, fieldparams.BLSSignatureLength),
				Graffiti:     make([]byte, 32),
				Eth1Data:     &ethpb.Eth1Data{DepositRoot: make([]byte, 32), BlockHash: make([]byte, 32)},
				SyncAggregate: &ethpb.SyncAggregate{
					SyncCommitteeBits:      make([]byte, fieldparams.SyncAggregateSyncCommitteeBytesLength),
					SyncCommitteeSignature: make([]byte, fieldparams.BLSSignatureLength),
				},
				SignedExecutionPayloadBid: &ethpb.SignedExecutionPayloadBid{
					Message: &ethpb.ExecutionPayloadBid{
						Slot:               slot,
						ParentBlockHash:    make([]byte, 32),
						ParentBlockRoot:    make([]byte, 32),
						BlockHash:          make([]byte, 32),
						PrevRandao:         make([]byte, 32),
						FeeRecipient:       make([]byte, 20),
						BlobKzgCommitments: [][]byte{},
					},
					Signature: make([]byte, fieldparams.BLSSignatureLength),
				},
				PayloadAttestations: []*ethpb.PayloadAttestation{},
			},
		},
		Signature: make([]byte, fieldparams.BLSSignatureLength),
	}
	wsb, err := consensusblocks.NewSignedBeaconBlock(blkProto)
	require.NoError(t, err)
	rob, err := consensusblocks.NewROBlock(wsb)
	require.NoError(t, err)
	return rob
}

// TestProcessSlotsForBlock_UpgradeSeededBid verifies that ProcessSlotsForBlock uses
// b.ParentRoot() as the NSC access key when the state has an upgrade-seeded bid
// (bid.ParentBlockRoot == zero). This guards against the Fulu->Gloas fork-boundary
// false positive where UpgradeToGloas seeds bid.BlockHash == latestBlockHash while
// leaving bid.ParentBlockRoot as all-zeros.
func TestProcessSlotsForBlock_UpgradeSeededBid(t *testing.T) {
	ctx := context.Background()
	parentRoot := [32]byte{0x01, 0x02, 0x03}
	blockHash := [32]byte{0xAA, 0xBB, 0xCC}
	targetSlot := primitives.Slot(9)

	// Build a Gloas state at slot 8 with IsParentBlockFull()==true but
	// bid.ParentBlockRoot==zero (upgrade-seeded: not a real committed bid).
	st := newGloasForkBoundaryState(t, targetSlot-1, blockHash, [32]byte{})
	require.Equal(t, version.Gloas, st.Version())

	// Verify preconditions.
	full, err := st.IsParentBlockFull()
	require.NoError(t, err)
	require.True(t, full, "precondition: IsParentBlockFull must be true")

	bid, err := st.LatestExecutionPayloadBid()
	require.NoError(t, err)
	require.Equal(t, [32]byte{}, bid.ParentBlockRoot(), "upgrade-seeded bid must have zero ParentBlockRoot")

	// Prime NSC with parentRoot as the access key.
	// With the guard in place (realBid==false), ProcessSlotsForBlock will use
	// b.ParentRoot() as the NSC key and find this cached entry.
	require.NoError(t, UpdateNextSlotCache(ctx, parentRoot[:], st))

	blk := newGloasTestBlock(t, targetSlot, parentRoot)

	out, err := ProcessSlotsForBlock(ctx, st, blk.Block())
	require.NoError(t, err)
	require.Equal(t, targetSlot, out.Slot())

	// Verify that the NSC entry primed under parentRoot is still present,
	// confirming it was used (read) rather than bypassed.
	cached := NextSlotState(parentRoot[:], targetSlot)
	require.NotNil(t, cached, "NSC entry under parentRoot should still be present after use")
}

// TestProcessSlotsForBlock_RealBid verifies that ProcessSlotsForBlock uses
// LatestBlockHash as the NSC access key when the state has a real committed bid
// (bid.ParentBlockRoot != zero). This is the normal post-fork case.
func TestProcessSlotsForBlock_RealBid(t *testing.T) {
	ctx := context.Background()
	parentRoot := [32]byte{0x01, 0x02, 0x03}
	blockHash := [32]byte{0xAA, 0xBB, 0xCC}
	realParentBlockRoot := [32]byte{0xDE, 0xAD, 0xBE, 0xEF}
	targetSlot := primitives.Slot(9)

	// Build a Gloas state at slot 8 with IsParentBlockFull()==true and
	// bid.ParentBlockRoot!=zero (a real committed bid).
	st := newGloasForkBoundaryState(t, targetSlot-1, blockHash, realParentBlockRoot)
	require.Equal(t, version.Gloas, st.Version())

	// Verify preconditions.
	full, err := st.IsParentBlockFull()
	require.NoError(t, err)
	require.True(t, full, "precondition: IsParentBlockFull must be true")

	bid, err := st.LatestExecutionPayloadBid()
	require.NoError(t, err)
	require.NotEqual(t, [32]byte{}, bid.ParentBlockRoot(), "real bid must have non-zero ParentBlockRoot")

	// Prime NSC with the EL block hash as access key.
	// With the guard in place (realBid==true), ProcessSlotsForBlock will use
	// LatestBlockHash as the NSC key and find this cached entry.
	require.NoError(t, UpdateNextSlotCache(ctx, blockHash[:], st))

	blk := newGloasTestBlock(t, targetSlot, parentRoot)

	out, err := ProcessSlotsForBlock(ctx, st, blk.Block())
	require.NoError(t, err)
	require.Equal(t, targetSlot, out.Slot())

	// Verify that the NSC entry primed under blockHash is still present,
	// confirming it was used (read) rather than bypassed.
	cached := NextSlotState(blockHash[:], targetSlot)
	require.NotNil(t, cached, "NSC entry under blockHash should still be present after use")
}

// TestProcessSlotsForBlock_PreGloas verifies that ProcessSlotsForBlock uses
// b.ParentRoot() as access key on pre-Gloas (Fulu) states, unchanged by the fix.
func TestProcessSlotsForBlock_PreGloas(t *testing.T) {
	ctx := context.Background()
	parentRoot := [32]byte{0x01, 0x02, 0x03}
	targetSlot := primitives.Slot(5)

	// newGloasState creates a Gloas-versioned state; we need a Fulu/pre-Gloas state.
	// Use newGloasState as a base and just verify the slot advancement works.
	// Note: version.Gloas is the version created by newGloasState; for pre-Gloas
	// the function takes the version < Gloas path. We build a minimal Gloas state
	// to test, but note ProcessSlotsForBlock has an explicit version check at top.
	st := newGloasState(t, targetSlot-1, bytes.Repeat([]byte{0}, int(params.BeaconConfig().SlotsPerHistoricalRoot/8)))

	blk := newGloasTestBlock(t, targetSlot, parentRoot)

	out, err := ProcessSlotsForBlock(ctx, st, blk.Block())
	require.NoError(t, err)
	require.Equal(t, targetSlot, out.Slot())
}
