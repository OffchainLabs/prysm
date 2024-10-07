package epbs

import (
	"context"
	"testing"

	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/prysmaticlabs/prysm/v5/testing/util/random"
)

func TestProcessPayloadStateTransition(t *testing.T) {
	bh := [32]byte{'h'}
	p := random.ExecutionPayloadEnvelope(t)
	p.Payload.BlockHash = bh[:]
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

	require.NoError(t, processPayloadStateTransition(ctx, st, e))

	lbh, err = st.LatestBlockHash()
	require.NoError(t, err)
	require.Equal(t, bh, [32]byte(lbh))

	lfs, err := st.LatestFullSlot()
	require.NoError(t, err)
	require.Equal(t, lfs, st.Slot())
}

func Test_validateAgainstHeader(t *testing.T) {
	bh := [32]byte{'h'}
	payload := &enginev1.ExecutionPayloadElectra{BlockHash: bh[:]}
	p := random.ExecutionPayloadEnvelope(t)
	p.Payload = payload
	e, err := blocks.WrappedROExecutionPayloadEnvelope(p)
	require.NoError(t, err)
	stpb := &ethpb.BeaconStateEPBS{Slot: 3}
	st, err := state_native.InitializeFromProtoUnsafeEpbs(stpb)
	require.NoError(t, err)
	ctx := context.Background()
	require.ErrorContains(t, "invalid nil latest block header", validateAgainstHeader(ctx, st, e))

	prest, _ := util.DeterministicGenesisStateEpbs(t, 64)
	br := [32]byte{'r'}
	p.BeaconBlockRoot = br[:]
	require.ErrorContains(t, "beacon block root does not match previous header", validateAgainstHeader(ctx, prest, e))

	header := prest.LatestBlockHeader()
	require.NoError(t, err)
	headerRoot, err := header.HashTreeRoot()
	require.NoError(t, err)
	p.BeaconBlockRoot = headerRoot[:]
	require.NoError(t, validateAgainstHeader(ctx, prest, e))
}

func Test_validateAgainstCommittedBid(t *testing.T) {
	payload := &enginev1.ExecutionPayloadElectra{}
	p := random.ExecutionPayloadEnvelope(t)
	p.Payload = payload
	p.BuilderIndex = 1
	e, err := blocks.WrappedROExecutionPayloadEnvelope(p)
	require.NoError(t, err)
	h := &enginev1.ExecutionPayloadHeaderEPBS{}
	require.ErrorContains(t, "builder index does not match committed header", validateAgainstCommittedBid(h, e))

	h.BuilderIndex = 1
	p.BlobKzgCommitments = make([][]byte, 6)
	for i := range p.BlobKzgCommitments {
		p.BlobKzgCommitments[i] = make([]byte, 48)
	}
	h.BlobKzgCommitmentsRoot = make([]byte, 32)
	require.ErrorContains(t, "blob KZG commitments root does not match committed header", validateAgainstCommittedBid(h, e))

	root, err := e.BlobKzgCommitmentsRoot()
	require.NoError(t, err)
	h.BlobKzgCommitmentsRoot = root[:]
	require.NoError(t, validateAgainstCommittedBid(h, e))
}

func TestCheckPostStateRoot(t *testing.T) {
	payload := &enginev1.ExecutionPayloadElectra{}
	p := random.ExecutionPayloadEnvelope(t)
	p.Payload = payload
	p.BuilderIndex = 1
	e, err := blocks.WrappedROExecutionPayloadEnvelope(p)
	require.NoError(t, err)
	ctx := context.Background()
	st, _ := util.DeterministicGenesisStateEpbs(t, 64)
	p.StateRoot = make([]byte, 32)
	require.ErrorContains(t, "state root mismatch", checkPostStateRoot(ctx, st, e))
	root, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)
	p.StateRoot = root[:]
	require.NoError(t, checkPostStateRoot(ctx, st, e))
}
