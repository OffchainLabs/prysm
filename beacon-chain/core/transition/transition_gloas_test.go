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
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
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
			ParentBlockHash:        make([]byte, 32),
			ParentBlockRoot:        make([]byte, 32),
			BlockHash:              make([]byte, 32),
			PrevRandao:             make([]byte, 32),
			FeeRecipient:           make([]byte, 20),
			BlobKzgCommitmentsRoot: make([]byte, 32),
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
