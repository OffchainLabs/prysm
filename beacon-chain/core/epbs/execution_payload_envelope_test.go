package epbs_test

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/epbs"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/OffchainLabs/prysm/v6/testing/util/random"
)

func TestProcessPayloadStateTransition(t *testing.T) {
	bh := [32]byte{'h'}
	p := random.ExecutionPayloadEnvelope(t)
	p.Payload.BlockHash = bh[:]
	p.ExecutionRequests = &enginev1.ExecutionRequests{}
	e, err := blocks.WrappedROExecutionPayloadEnvelope(p)
	require.NoError(t, err)
	validators := make([]*ethpb.Validator, 0)
	stpb := &ethpb.BeaconStateEPBS{Slot: 3, Validators: validators}
	st, err := state_native.InitializeFromProtoUnsafeEpbs(stpb)
	require.NoError(t, err)
	ctx := context.Background()

	lbh, err := st.LatestBlockHash()
	require.NoError(t, err)
	require.Equal(t, [32]byte{}, [32]byte(lbh))

	require.NoError(t, epbs.ProcessPayloadStateTransition(ctx, st, e))

	lbh, err = st.LatestBlockHash()
	require.NoError(t, err)
	require.Equal(t, bh, [32]byte(lbh))

	lfs, err := st.LatestFullSlot()
	require.NoError(t, err)
	require.Equal(t, lfs, st.Slot())
}

func Test_validateAgainstHeader(t *testing.T) {
	bh := [32]byte{'h'}
	payload := &enginev1.ExecutionPayloadDeneb{BlockHash: bh[:]}
	p := random.ExecutionPayloadEnvelope(t)
	p.Payload = payload
	e, err := blocks.WrappedROExecutionPayloadEnvelope(p)
	require.NoError(t, err)
	stpb := &ethpb.BeaconStateEPBS{Slot: 3}
	st, err := state_native.InitializeFromProtoUnsafeEpbs(stpb)
	require.NoError(t, err)
	ctx := context.Background()
	require.ErrorContains(t, "invalid nil latest block header", epbs.UpdateHeaderAndVerify(ctx, st, e))

	prest, _ := util.DeterministicGenesisStateEpbs(t, 64)
	br := [32]byte{'r'}
	p.BeaconBlockRoot = br[:]
	require.ErrorContains(t, "beacon block root does not match previous header", epbs.UpdateHeaderAndVerify(ctx, prest, e))

	header := prest.LatestBlockHeader()
	require.NoError(t, err)
	headerRoot, err := header.HashTreeRoot()
	require.NoError(t, err)
	p.BeaconBlockRoot = headerRoot[:]
	require.NoError(t, epbs.UpdateHeaderAndVerify(ctx, prest, e))
}

func Test_validateAgainstCommittedBid(t *testing.T) {
	payload := &enginev1.ExecutionPayloadDeneb{}
	p := random.ExecutionPayloadEnvelope(t)
	p.Payload = payload
	p.BuilderIndex = 1
	e, err := blocks.WrappedROExecutionPayloadEnvelope(p)
	require.NoError(t, err)
	h := &enginev1.ExecutionPayloadHeaderEPBS{}
	require.ErrorContains(t, "builder index does not match committed header", epbs.ValidateAgainstCommittedBid(h, e))

	h.BuilderIndex = 1
	p.BlobKzgCommitments = make([][]byte, 6)
	for i := range p.BlobKzgCommitments {
		p.BlobKzgCommitments[i] = make([]byte, 48)
	}
	h.BlobKzgCommitmentsRoot = make([]byte, 32)
	require.ErrorContains(t, "blob KZG commitments root does not match committed header", epbs.ValidateAgainstCommittedBid(h, e))

	root, err := e.BlobKzgCommitmentsRoot()
	require.NoError(t, err)
	h.BlobKzgCommitmentsRoot = root[:]
	require.NoError(t, epbs.ValidateAgainstCommittedBid(h, e))
}

func TestCheckPostStateRoot(t *testing.T) {
	payload := &enginev1.ExecutionPayloadDeneb{}
	p := random.ExecutionPayloadEnvelope(t)
	p.Payload = payload
	p.BuilderIndex = 1
	e, err := blocks.WrappedROExecutionPayloadEnvelope(p)
	require.NoError(t, err)
	ctx := context.Background()
	st, _ := util.DeterministicGenesisStateEpbs(t, 64)
	p.BeaconStateRoot = make([]byte, 32)
	require.ErrorContains(t, "state root mismatch", epbs.CheckPostStateRoot(ctx, st, e))
	root, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)
	p.BeaconStateRoot = root[:]
	require.NoError(t, epbs.CheckPostStateRoot(ctx, st, e))
}
