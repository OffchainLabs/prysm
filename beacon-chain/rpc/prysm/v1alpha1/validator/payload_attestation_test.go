package validator

import (
	"context"
	"testing"

	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	payloadattestation "github.com/OffchainLabs/prysm/v7/beacon-chain/operations/payloadattestation"
	p2pmock "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"google.golang.org/protobuf/types/known/emptypb"
)

type payloadAttestationBlockReceiver struct {
	*chainMock.ChainService
	received bool
}

func (r *payloadAttestationBlockReceiver) ReceivePayloadAttestationMessage(_ context.Context, _ *ethpb.PayloadAttestationMessage) error {
	r.received = true
	return nil
}

func TestPayloadAttestationData_OK(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	slot := primitives.Slot(7)
	root := bytesutil.PadTo([]byte{0xAA}, 32)
	chain := &chainMock.ChainService{
		Slot: &slot,
		Root: root,
		MockCanonicalRoots: map[primitives.Slot][32]byte{
			slot: bytesutil.ToBytes32(root),
		},
		MockCanonicalFull: map[primitives.Slot]bool{
			slot: false,
		},
	}
	vs := &Server{
		SyncChecker:       &mockSync.Sync{IsSyncing: false},
		TimeFetcher:       chain,
		HeadFetcher:       chain,
		ForkchoiceFetcher: chain,
		CoreService:       &core.Service{GenesisTimeFetcher: chain, ForkchoiceFetcher: chain, HeadFetcher: chain},
	}

	resp, err := vs.PayloadAttestationData(t.Context(), &ethpb.PayloadAttestationDataRequest{Slot: slot})
	require.NoError(t, err)
	require.DeepEqual(t, root, resp.BeaconBlockRoot)
	assert.Equal(t, slot, resp.Slot)
	assert.Equal(t, false, resp.PayloadPresent)
	assert.Equal(t, false, resp.BlobDataAvailable)
}

func TestSubmitPayloadAttestation_OK(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	slot := primitives.Slot(0)
	root := bytesutil.PadTo([]byte{0xBB}, 32)
	st, _ := util.DeterministicGenesisStateGloas(t, 64)
	ptc, err := st.PayloadCommitteeReadOnly(slot)
	require.NoError(t, err)
	require.NotEqual(t, 0, len(ptc))

	chain := &chainMock.ChainService{
		Slot:      &slot,
		State:     st,
		BlockSlot: slot,
	}
	p2p := &p2pmock.MockBroadcaster{}
	receiver := &payloadAttestationBlockReceiver{ChainService: chain}

	vs := &Server{
		SyncChecker:                &mockSync.Sync{IsSyncing: false},
		TimeFetcher:                chain,
		HeadFetcher:                chain,
		ForkchoiceFetcher:          chain,
		P2P:                        p2p,
		BlockReceiver:              receiver,
		PayloadAttestationReceiver: receiver,
		PayloadAttestationPool:     payloadattestation.NewPool(),
		OperationNotifier:          chain.OperationNotifier(),
	}

	msg := &ethpb.PayloadAttestationMessage{
		ValidatorIndex: ptc[0],
		Data: &ethpb.PayloadAttestationData{
			BeaconBlockRoot: root,
			Slot:            slot,
		},
		Signature: make([]byte, 96),
	}

	resp, err := vs.SubmitPayloadAttestation(t.Context(), msg)
	require.NoError(t, err)
	require.DeepEqual(t, &emptypb.Empty{}, resp)
	assert.Equal(t, true, p2p.BroadcastCalled.Load())
	assert.Equal(t, true, receiver.received)
}

func TestSubmitPayloadAttestation_Syncing(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	slot := primitives.Slot(12)
	root := bytesutil.PadTo([]byte{0xCC}, 32)
	chain := &chainMock.ChainService{
		Slot:      &slot,
		BlockSlot: slot,
	}
	vs := &Server{
		SyncChecker:                &mockSync.Sync{IsSyncing: true},
		TimeFetcher:                chain,
		ForkchoiceFetcher:          chain,
		P2P:                        &p2pmock.MockBroadcaster{},
		BlockReceiver:              chain,
		PayloadAttestationReceiver: chain,
	}

	msg := &ethpb.PayloadAttestationMessage{
		ValidatorIndex: 1,
		Data: &ethpb.PayloadAttestationData{
			BeaconBlockRoot: root,
			Slot:            slot,
		},
		Signature: make([]byte, 96),
	}
	_, err := vs.SubmitPayloadAttestation(t.Context(), msg)
	require.ErrorContains(t, "not ready to respond", err)
}
