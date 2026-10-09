package blockchain

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	mockExecution "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/proofengine"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

// setupExecutionProofService returns a service with EIP-8025 enabled, where the block A has a revealed
// payload inserted in forkchoice, and the block B builds on (A, full) without payload.
func setupExecutionProofService(t *testing.T, minProofs uint64) (*Service, [32]byte, [32]byte) {
	t.Helper()
	resetCfg := features.InitWithReset(&features.Flags{EnableExecutionProofs: true})
	t.Cleanup(resetCfg)

	service, _ := minimalTestService(t,
		WithPayloadIDCache(cache.NewPayloadIDCache()),
		WithExecutionEngineCaller(&mockExecution.EngineClient{}),
		WithMinExecutionProofs(minProofs),
	)

	// The blocks use their real roots, under which the DB stores them.
	baseA, blkA := testGloasState(t, 1, params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash)
	rootA, err := blkA.Block.HashTreeRoot()
	require.NoError(t, err)
	insertGloasBlock(t, service, baseA, blkA, rootA)
	envA, err := blocks.WrappedROExecutionPayloadEnvelope(&ethpb.ExecutionPayloadEnvelope{
		BeaconBlockRoot:       rootA[:],
		ParentBeaconBlockRoot: make([]byte, 32),
		Payload:               &enginev1.ExecutionPayloadGloas{BlockHash: make([]byte, 32), ParentHash: make([]byte, 32)},
	})
	require.NoError(t, err)
	require.NoError(t, service.InsertPayload(envA))

	baseB, blkB := testGloasState(t, 2, rootA, bytesutil.ToBytes32([]byte("hashB")))
	rootB, err := blkB.Block.HashTreeRoot()
	require.NoError(t, err)
	insertGloasBlock(t, service, baseB, blkB, rootB)

	return service, rootA, rootB
}

func requireOptimistic(t *testing.T, s *Service, root [32]byte, expected bool) {
	t.Helper()
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	optimistic, err := s.cfg.ForkChoiceStore.IsOptimistic(root)
	require.NoError(t, err)
	require.Equal(t, expected, optimistic)
}

func TestExecutionProof_ELValidationWithoutProofIsOptimistic(t *testing.T) {
	service, rootA, rootB := setupExecutionProofService(t, 1)
	ctx := t.Context()

	// The empty block B is the head.
	service.head = &head{root: rootB, optimistic: true}

	service.cfg.ForkChoiceStore.Lock()
	err := service.markPayloadExecutionValid(ctx, rootA)
	service.cfg.ForkChoiceStore.Unlock()
	require.NoError(t, err)
	requireOptimistic(t, service, rootA, true)
	requireOptimistic(t, service, rootB, true)
	requireHeadOptimistic(t, service, true)

	// The proof is the last missing piece: the payload of A, and the empty block B built on it, are now valid.
	require.NoError(t, service.ReceiveExecutionProof(ctx, rootA, ethpb.ProofTypeEthrexSP1))
	requireOptimistic(t, service, rootA, false)
	requireOptimistic(t, service, rootB, false)
	requireHeadOptimistic(t, service, false)
}

func requireHeadOptimistic(t *testing.T, s *Service, expected bool) {
	t.Helper()
	s.headLock.RLock()
	defer s.headLock.RUnlock()
	require.Equal(t, expected, s.head.optimistic)
}

func TestExecutionProof_ProofWithoutELValidationIsOptimistic(t *testing.T) {
	service, rootA, _ := setupExecutionProofService(t, 1)
	ctx := t.Context()

	require.NoError(t, service.ReceiveExecutionProof(ctx, rootA, ethpb.ProofTypeEthrexSP1))
	requireOptimistic(t, service, rootA, true)

	service.cfg.ForkChoiceStore.Lock()
	err := service.markPayloadExecutionValid(ctx, rootA)
	service.cfg.ForkChoiceStore.Unlock()
	require.NoError(t, err)
	requireOptimistic(t, service, rootA, false)
}

func TestExecutionProof_MinExecutionProofs(t *testing.T) {
	service, rootA, _ := setupExecutionProofService(t, 2)
	ctx := t.Context()

	service.cfg.ForkChoiceStore.Lock()
	err := service.markPayloadExecutionValid(ctx, rootA)
	service.cfg.ForkChoiceStore.Unlock()
	require.NoError(t, err)

	require.NoError(t, service.ReceiveExecutionProof(ctx, rootA, ethpb.ProofTypeEthrexSP1))
	requireOptimistic(t, service, rootA, true)

	// A second proof of the same type does not count.
	require.ErrorIs(t, service.ReceiveExecutionProof(ctx, rootA, ethpb.ProofTypeEthrexSP1), ErrExecutionProofTypeKnown)
	requireOptimistic(t, service, rootA, true)

	require.NoError(t, service.ReceiveExecutionProof(ctx, rootA, ethpb.ProofTypeEthrexZisk))
	requireOptimistic(t, service, rootA, false)
}

func TestSetPayloadValidIfReady_LogsTransitionOnly(t *testing.T) {
	logHook := logTest.NewGlobal()
	service, rootA, _ := setupExecutionProofService(t, 1)
	ctx := t.Context()

	service.cfg.ForkChoiceStore.Lock()
	err := service.markPayloadExecutionValid(ctx, rootA)
	service.cfg.ForkChoiceStore.Unlock()
	require.NoError(t, err)

	const msg = "Payload validated by the EL and execution proofs"

	// The first proof makes the optimistic payload valid.
	require.NoError(t, service.ReceiveExecutionProof(ctx, rootA, ethpb.ProofTypeEthrexSP1))
	requireOptimistic(t, service, rootA, false)
	require.LogsContain(t, logHook, msg)

	// A later proof of the already valid payload is not reported again.
	logHook.Reset()
	require.NoError(t, service.ReceiveExecutionProof(ctx, rootA, ethpb.ProofTypeEthrexZisk))
	require.LogsDoNotContain(t, logHook, msg)
}

func TestUpdateFinalized_ExecutionProofs(t *testing.T) {
	// B, built on the payload of A, is finalized. A, the oldest block, stands for genesis.
	finalize := func(t *testing.T, service *Service, rootA, root [32]byte) {
		t.Helper()
		require.NoError(t, service.cfg.BeaconDB.SaveGenesisBlockRoot(t.Context(), rootA))
		require.NoError(t, service.cfg.BeaconDB.SaveLastValidatedCheckpoint(t.Context(), &ethpb.Checkpoint{Epoch: 0, Root: rootA[:]}))
		service.cfg.ForkChoiceStore.Lock()
		defer service.cfg.ForkChoiceStore.Unlock()
		require.NoError(t, service.updateFinalized(t.Context(), &ethpb.Checkpoint{Epoch: 1, Root: root[:]}))
	}

	t.Run("EL validated payload needs no proof", func(t *testing.T) {
		service, rootA, rootB := setupExecutionProofService(t, 1)

		service.cfg.ForkChoiceStore.Lock()
		require.NoError(t, service.markPayloadExecutionValid(t.Context(), rootA))
		service.cfg.ForkChoiceStore.Unlock()
		requireOptimistic(t, service, rootA, true)

		finalize(t, service, rootA, rootB)
		requireOptimistic(t, service, rootA, false)
		requireOptimistic(t, service, rootB, false)

		cp, err := service.cfg.BeaconDB.LastValidatedCheckpoint(t.Context())
		require.NoError(t, err)
		require.DeepEqual(t, rootB[:], cp.Root)
	})

	t.Run("payload not validated by the EL stays optimistic", func(t *testing.T) {
		service, rootA, rootB := setupExecutionProofService(t, 1)

		finalize(t, service, rootA, rootB)
		requireOptimistic(t, service, rootB, true)

		cp, err := service.cfg.BeaconDB.LastValidatedCheckpoint(t.Context())
		require.NoError(t, err)
		require.DeepEqual(t, rootA[:], cp.Root)
	})
}

func TestNewPayloadRequestRoot(t *testing.T) {
	blockHash := bytesutil.ToBytes32([]byte("hash"))
	base, _ := testGloasState(t, 1, params.BeaconConfig().ZeroHash, blockHash)
	st, err := state_native.InitializeFromProtoUnsafeGloas(base)
	require.NoError(t, err)

	envelope := func(t *testing.T, blockHash [32]byte) interfaces.ROExecutionPayloadEnvelope {
		env, err := blocks.WrappedROExecutionPayloadEnvelope(&ethpb.ExecutionPayloadEnvelope{
			BeaconBlockRoot:       make([]byte, 32),
			ParentBeaconBlockRoot: make([]byte, 32),
			Payload: &enginev1.ExecutionPayloadGloas{
				ParentHash:    make([]byte, 32),
				FeeRecipient:  make([]byte, 20),
				StateRoot:     make([]byte, 32),
				ReceiptsRoot:  make([]byte, 32),
				LogsBloom:     make([]byte, 256),
				PrevRandao:    make([]byte, 32),
				BaseFeePerGas: make([]byte, 32),
				BlockHash:     blockHash[:],
			},
		})
		require.NoError(t, err)
		return env
	}

	t.Run("disabled", func(t *testing.T) {
		root, err := newPayloadRequestRoot(envelope(t, blockHash), st)
		require.NoError(t, err)
		require.Equal(t, true, root == nil)
	})

	resetCfg := features.InitWithReset(&features.Flags{EnableExecutionProofs: true})
	defer resetCfg()

	t.Run("envelope fulfils the bid", func(t *testing.T) {
		root, err := newPayloadRequestRoot(envelope(t, blockHash), st)
		require.NoError(t, err)
		require.Equal(t, false, root == nil)
	})

	t.Run("envelope of another bid", func(t *testing.T) {
		root, err := newPayloadRequestRoot(envelope(t, bytesutil.ToBytes32([]byte("other"))), st)
		require.ErrorContains(t, "does not match", err)
		require.Equal(t, true, root == nil)
	})
}

// fakeExecutionProofVerifier records the public input it verifies against, and returns err.
type fakeExecutionProofVerifier struct {
	publicInput []byte
	err         error
}

func (f *fakeExecutionProofVerifier) Verify(_ context.Context, _ ethpb.ProofType, _, publicInput []byte) error {
	f.publicInput = publicInput
	return f.err
}

func TestVerifyExecutionProof(t *testing.T) {
	ctx := context.Background()
	requestRoot := bytesutil.ToBytes32([]byte("request"))

	proof, err := blocks.NewROSignedExecutionProofEnvelope(&ethpb.SignedExecutionProofEnvelope{
		Message: &ethpb.ExecutionProofEnvelope{
			ProofData:       []byte{0x01},
			ProofType:       []byte{byte(ethpb.ProofTypeEthrexSP1)},
			BeaconBlockRoot: make([]byte, 32),
		},
		Signature: make([]byte, 96),
	})
	require.NoError(t, err)

	t.Run("valid", func(t *testing.T) {
		verifier := &fakeExecutionProofVerifier{}
		service := &Service{cfg: &config{ExecutionProofVerifier: verifier}}
		require.NoError(t, service.VerifyExecutionProof(ctx, requestRoot, proof))
		require.DeepEqual(t, proofengine.PublicInput(requestRoot), verifier.publicInput)
	})

	t.Run("invalid", func(t *testing.T) {
		service := &Service{cfg: &config{ExecutionProofVerifier: &fakeExecutionProofVerifier{err: proofengine.ErrProofInvalid}}}
		require.ErrorIs(t, service.VerifyExecutionProof(ctx, requestRoot, proof), ErrInvalidExecutionProof)
	})

	t.Run("no verifier", func(t *testing.T) {
		service := &Service{cfg: &config{}}
		require.ErrorIs(t, service.VerifyExecutionProof(ctx, requestRoot, proof), errNoExecutionProofVerifier)
	})
}
