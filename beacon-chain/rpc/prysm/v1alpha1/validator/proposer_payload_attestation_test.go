package validator

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/payloadattestation"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestGetPayloadAttestations_BeforeGloasFork(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 10
	params.OverrideBeaconConfig(cfg)

	// State at slot 0 (epoch 0), which is before GloasForkEpoch 10.
	headState, err := util.NewBeaconStateGloas()
	require.NoError(t, err)
	require.NoError(t, headState.SetSlot(0))

	vs := &Server{}
	result := vs.getPayloadAttestations(t.Context(), headState, [32]byte{})
	require.Equal(t, true, result == nil)
}

func TestGetPayloadAttestations_AtGloasFork(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	headState, err := util.NewBeaconStateGloas()
	require.NoError(t, err)
	require.NoError(t, headState.SetSlot(0))

	vs := &Server{}
	result := vs.getPayloadAttestations(t.Context(), headState, [32]byte{})
	require.NotNil(t, result)
	require.Equal(t, 0, len(result))
}

func TestGetPayloadAttestations_FromPoolFiltersByParentAndPreviousSlot(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	headState, err := util.NewBeaconStateGloas()
	require.NoError(t, err)
	require.NoError(t, headState.SetSlot(10))
	parentRoot := bytesutil.PadTo([]byte{0xAA}, 32)
	header := headState.LatestBlockHeader()
	header.ParentRoot = parentRoot
	require.NoError(t, headState.SetLatestBlockHeader(header))

	pool := payloadattestation.NewPool()
	insert := func(slot primitives.Slot, root []byte, payloadPresent bool, idx uint64) {
		t.Helper()
		msg := &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 0,
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot:   root,
				Slot:              slot,
				PayloadPresent:    payloadPresent,
				BlobDataAvailable: payloadPresent,
			},
			Signature: bytesutil.PadTo([]byte{byte(idx + 1)}, 96),
		}
		require.NoError(t, pool.InsertPayloadAttestation(msg, idx))
	}

	// Matching entries: slot 9 and block root == parent_root.
	insert(9, parentRoot, false, 0)
	insert(9, parentRoot, true, 1)
	// Non-matching entries.
	insert(8, parentRoot, true, 0)                        // wrong slot
	insert(9, bytesutil.PadTo([]byte{0xBB}, 32), true, 1) // wrong root

	vs := &Server{PayloadAttestationPool: pool}
	result := vs.getPayloadAttestations(t.Context(), headState, bytesutil.ToBytes32(parentRoot))
	require.Equal(t, 2, len(result))
	for _, att := range result {
		require.Equal(t, uint64(9), uint64(att.Data.Slot))
		require.Equal(t, true, bytes.Equal(parentRoot, att.Data.BeaconBlockRoot))
	}
	// Deterministic ordering: false payload_present first.
	require.Equal(t, false, result[0].Data.PayloadPresent)
	require.Equal(t, true, result[1].Data.PayloadPresent)
}
