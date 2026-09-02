package verification

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

func TestEnvelopeVerifier_VerifySlotAboveFinalized(t *testing.T) {
	root := bytesutil.ToBytes32(bytes.Repeat([]byte{0xAA}, 32))
	blockHash := bytesutil.ToBytes32(bytes.Repeat([]byte{0xBB}, 32))
	env := testSignedExecutionPayloadEnvelope(t, 1, 1, root, blockHash)
	wrapped, err := blocks.WrappedROSignedExecutionPayloadEnvelope(env)
	require.NoError(t, err)

	verifier := &EnvelopeVerifier{results: newResults(RequireEnvelopeSlotAboveFinalized), e: wrapped}
	require.ErrorIs(t, verifier.VerifySlotAboveFinalized(1), ErrEnvelopeSlotBeforeFinalized)

	verifier = &EnvelopeVerifier{results: newResults(RequireEnvelopeSlotAboveFinalized), e: wrapped}
	require.NoError(t, verifier.VerifySlotAboveFinalized(0))
}

func TestEnvelopeVerifier_VerifySlotMatchesBlock(t *testing.T) {
	root := bytesutil.ToBytes32(bytes.Repeat([]byte{0xAA}, 32))
	blockHash := bytesutil.ToBytes32(bytes.Repeat([]byte{0xBB}, 32))
	env := testSignedExecutionPayloadEnvelope(t, 2, 1, root, blockHash)
	wrapped, err := blocks.WrappedROSignedExecutionPayloadEnvelope(env)
	require.NoError(t, err)

	verifier := &EnvelopeVerifier{results: newResults(RequireEnvelopeSlotMatchesBlock), e: wrapped}
	require.ErrorIs(t, verifier.VerifySlotMatchesBlock(3), ErrEnvelopeSlotMismatch)

	verifier = &EnvelopeVerifier{results: newResults(RequireEnvelopeSlotMatchesBlock), e: wrapped}
	require.NoError(t, verifier.VerifySlotMatchesBlock(2))
}

func TestEnvelopeVerifier_VerifyBlockRootSeen(t *testing.T) {
	root := bytesutil.ToBytes32(bytes.Repeat([]byte{0xAA}, 32))
	blockHash := bytesutil.ToBytes32(bytes.Repeat([]byte{0xBB}, 32))
	env := testSignedExecutionPayloadEnvelope(t, 1, 1, root, blockHash)
	wrapped, err := blocks.WrappedROSignedExecutionPayloadEnvelope(env)
	require.NoError(t, err)

	verifier := &EnvelopeVerifier{results: newResults(RequireBlockRootSeen), e: wrapped}
	require.ErrorIs(t, verifier.VerifyBlockRootSeen(func([32]byte) bool { return false }), ErrEnvelopeBlockRootNotSeen)

	verifier = &EnvelopeVerifier{results: newResults(RequireBlockRootSeen), e: wrapped}
	require.NoError(t, verifier.VerifyBlockRootSeen(func([32]byte) bool { return true }))
}

func TestEnvelopeVerifier_VerifyBlockRootValid(t *testing.T) {
	root := bytesutil.ToBytes32(bytes.Repeat([]byte{0xAA}, 32))
	blockHash := bytesutil.ToBytes32(bytes.Repeat([]byte{0xBB}, 32))
	env := testSignedExecutionPayloadEnvelope(t, 1, 1, root, blockHash)
	wrapped, err := blocks.WrappedROSignedExecutionPayloadEnvelope(env)
	require.NoError(t, err)

	verifier := &EnvelopeVerifier{results: newResults(RequireBlockRootValid), e: wrapped}
	require.ErrorIs(t, verifier.VerifyBlockRootValid(func([32]byte) bool { return true }), ErrEnvelopeBlockRootInvalid)

	verifier = &EnvelopeVerifier{results: newResults(RequireBlockRootValid), e: wrapped}
	require.NoError(t, verifier.VerifyBlockRootValid(func([32]byte) bool { return false }))
}

func TestEnvelopeVerifier_VerifyBuilderValid(t *testing.T) {
	root := bytesutil.ToBytes32(bytes.Repeat([]byte{0xAA}, 32))
	blockHash := bytesutil.ToBytes32(bytes.Repeat([]byte{0xBB}, 32))
	env := testSignedExecutionPayloadEnvelope(t, 1, 1, root, blockHash)
	wrapped, err := blocks.WrappedROSignedExecutionPayloadEnvelope(env)
	require.NoError(t, err)

	badBid := testExecutionPayloadBid(t, 1, 2, blockHash)
	verifier := &EnvelopeVerifier{results: newResults(RequireBuilderValid), e: wrapped}
	require.ErrorIs(t, verifier.VerifyBuilderValid(badBid), ErrIncorrectEnvelopeBuilder)

	okBid := testExecutionPayloadBid(t, 1, 1, blockHash)
	verifier = &EnvelopeVerifier{results: newResults(RequireBuilderValid), e: wrapped}
	require.NoError(t, verifier.VerifyBuilderValid(okBid))
}

func TestEnvelopeVerifier_VerifyPayloadHash(t *testing.T) {
	root := bytesutil.ToBytes32(bytes.Repeat([]byte{0xAA}, 32))
	blockHash := bytesutil.ToBytes32(bytes.Repeat([]byte{0xBB}, 32))
	env := testSignedExecutionPayloadEnvelope(t, 1, 1, root, blockHash)
	wrapped, err := blocks.WrappedROSignedExecutionPayloadEnvelope(env)
	require.NoError(t, err)

	badHash := bytesutil.ToBytes32(bytes.Repeat([]byte{0xCC}, 32))
	badBid := testExecutionPayloadBid(t, 1, 1, badHash)
	verifier := &EnvelopeVerifier{results: newResults(RequirePayloadHashValid), e: wrapped}
	require.ErrorIs(t, verifier.VerifyPayloadHash(badBid), ErrIncorrectEnvelopeBlockHash)

	okBid := testExecutionPayloadBid(t, 1, 1, blockHash)
	verifier = &EnvelopeVerifier{results: newResults(RequirePayloadHashValid), e: wrapped}
	require.NoError(t, verifier.VerifyPayloadHash(okBid))
}

func TestEnvelopeVerifier_VerifyExecutionRequestsLimits(t *testing.T) {
	root := bytesutil.ToBytes32(bytes.Repeat([]byte{0xAA}, 32))
	blockHash := bytesutil.ToBytes32(bytes.Repeat([]byte{0xBB}, 32))
	cfg := params.BeaconConfig()

	newVerifier := func(t *testing.T, reqs *enginev1.ExecutionRequestsGloas) *EnvelopeVerifier {
		env := testSignedExecutionPayloadEnvelope(t, 1, 1, root, blockHash)
		env.Message.ExecutionRequests = reqs
		wrapped, err := blocks.WrappedROSignedExecutionPayloadEnvelope(env)
		require.NoError(t, err)
		return &EnvelopeVerifier{results: newResults(RequireExecutionRequestsLimitsValid), e: wrapped}
	}

	t.Run("at limits", func(t *testing.T) {
		reqs := &enginev1.ExecutionRequestsGloas{
			Withdrawals:     make([]*enginev1.WithdrawalRequest, cfg.MaxWithdrawalRequestsPerPayload),
			Consolidations:  make([]*enginev1.ConsolidationRequest, cfg.MaxConsolidationsRequestsPerPayload),
			BuilderDeposits: make([]*enginev1.BuilderDepositRequest, cfg.MaxBuilderDepositRequestsPerPayload),
			BuilderExits:    make([]*enginev1.BuilderExitRequest, cfg.MaxBuilderExitRequestsPerPayload),
		}
		require.NoError(t, newVerifier(t, reqs).VerifyExecutionRequestsLimits())
	})

	cases := []struct {
		name string
		reqs *enginev1.ExecutionRequestsGloas
		want string
	}{
		{
			name: "withdrawal requests over limit",
			reqs: &enginev1.ExecutionRequestsGloas{Withdrawals: make([]*enginev1.WithdrawalRequest, cfg.MaxWithdrawalRequestsPerPayload+1)},
			want: "too many withdrawal requests",
		},
		{
			name: "consolidation requests over limit",
			reqs: &enginev1.ExecutionRequestsGloas{Consolidations: make([]*enginev1.ConsolidationRequest, cfg.MaxConsolidationsRequestsPerPayload+1)},
			want: "too many consolidation requests",
		},
		{
			name: "builder deposit requests over limit",
			reqs: &enginev1.ExecutionRequestsGloas{BuilderDeposits: make([]*enginev1.BuilderDepositRequest, cfg.MaxBuilderDepositRequestsPerPayload+1)},
			want: "too many builder deposit requests",
		},
		{
			name: "builder exit requests over limit",
			reqs: &enginev1.ExecutionRequestsGloas{BuilderExits: make([]*enginev1.BuilderExitRequest, cfg.MaxBuilderExitRequestsPerPayload+1)},
			want: "too many builder exit requests",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.ErrorContains(t, tc.want, newVerifier(t, tc.reqs).VerifyExecutionRequestsLimits())
		})
	}

	// SSZ decoding does not bound the request lists, so the check must hold post-decode.
	t.Run("enforced on decoded envelope", func(t *testing.T) {
		env := testSignedExecutionPayloadEnvelope(t, 1, 1, root, blockHash)
		for range cfg.MaxBuilderDepositRequestsPerPayload + 1 {
			env.Message.ExecutionRequests.BuilderDeposits = append(env.Message.ExecutionRequests.BuilderDeposits, &enginev1.BuilderDepositRequest{
				Pubkey:                make([]byte, 48),
				WithdrawalCredentials: make([]byte, 32),
				Signature:             make([]byte, 96),
			})
		}
		encoded, err := env.MarshalSSZ()
		require.NoError(t, err)
		decoded := &ethpb.SignedExecutionPayloadEnvelope{}
		require.NoError(t, decoded.UnmarshalSSZ(encoded))
		require.Equal(t, cfg.MaxBuilderDepositRequestsPerPayload+1, uint64(len(decoded.Message.ExecutionRequests.BuilderDeposits)))
		wrapped, err := blocks.WrappedROSignedExecutionPayloadEnvelope(decoded)
		require.NoError(t, err)
		verifier := NewEnvelopeVerifier(wrapped, GossipExecutionPayloadEnvelopeRequirements)
		require.ErrorContains(t, "too many builder deposit requests", verifier.VerifyExecutionRequestsLimits())
	})
}

func TestEnvelopeVerifier_VerifyWithdrawalsLimit(t *testing.T) {
	root := bytesutil.ToBytes32(bytes.Repeat([]byte{0xAA}, 32))
	blockHash := bytesutil.ToBytes32(bytes.Repeat([]byte{0xBB}, 32))
	limit := params.BeaconConfig().MaxWithdrawalsPerPayload

	newVerifier := func(t *testing.T, withdrawals []*enginev1.Withdrawal) *EnvelopeVerifier {
		env := testSignedExecutionPayloadEnvelope(t, 1, 1, root, blockHash)
		env.Message.Payload.Withdrawals = withdrawals
		wrapped, err := blocks.WrappedROSignedExecutionPayloadEnvelope(env)
		require.NoError(t, err)
		return &EnvelopeVerifier{results: newResults(RequireWithdrawalsLimitValid), e: wrapped}
	}

	t.Run("at limit", func(t *testing.T) {
		require.NoError(t, newVerifier(t, make([]*enginev1.Withdrawal, limit)).VerifyWithdrawalsLimit())
	})

	t.Run("over limit", func(t *testing.T) {
		require.ErrorIs(t, newVerifier(t, make([]*enginev1.Withdrawal, limit+1)).VerifyWithdrawalsLimit(), ErrTooManyWithdrawals)
	})
}

func TestEnvelopeVerifier_VerifySignature_Builder(t *testing.T) {
	slot := primitives.Slot(1)
	root := bytesutil.ToBytes32(bytes.Repeat([]byte{0xAA}, 32))
	blockHash := bytesutil.ToBytes32(bytes.Repeat([]byte{0xBB}, 32))
	env := testSignedExecutionPayloadEnvelope(t, slot, 0, root, blockHash)

	sk, err := bls.RandKey()
	require.NoError(t, err)
	builderPubkey := sk.PublicKey().Marshal()

	st := newGloasState(t, slot, nil, nil, []*ethpb.Builder{{Pubkey: builderPubkey}})

	sig := signEnvelope(t, sk, env.Message, st.Fork(), st.GenesisValidatorsRoot(), slot)
	env.Signature = sig[:]
	wrapped, err := blocks.WrappedROSignedExecutionPayloadEnvelope(env)
	require.NoError(t, err)

	verifier := &EnvelopeVerifier{results: newResults(RequireBuilderSignatureValid), e: wrapped}
	require.NoError(t, verifier.VerifySignature(t.Context(), st))

	sk2, err := bls.RandKey()
	require.NoError(t, err)
	badSig := signEnvelope(t, sk2, env.Message, st.Fork(), st.GenesisValidatorsRoot(), slot)
	env.Signature = badSig[:]
	wrapped, err = blocks.WrappedROSignedExecutionPayloadEnvelope(env)
	require.NoError(t, err)
	verifier = &EnvelopeVerifier{results: newResults(RequireBuilderSignatureValid), e: wrapped}
	require.ErrorIs(t, verifier.VerifySignature(t.Context(), st), signing.ErrSigFailedToVerify)
}

func TestEnvelopeVerifier_VerifySignature_SelfBuild(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.FuluForkEpoch = 0
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	slot := primitives.Slot(2)
	root := bytesutil.ToBytes32(bytes.Repeat([]byte{0xAA}, 32))
	blockHash := bytesutil.ToBytes32(bytes.Repeat([]byte{0xBB}, 32))
	env := testSignedExecutionPayloadEnvelope(t, slot, params.BeaconConfig().BuilderIndexSelfBuild, root, blockHash)

	sk, err := bls.RandKey()
	require.NoError(t, err)
	validatorPubkey := sk.PublicKey().Marshal()

	validators := []*ethpb.Validator{{PublicKey: validatorPubkey}}
	balances := []uint64{0}
	st := newGloasState(t, slot, validators, balances, nil)

	sig := signEnvelope(t, sk, env.Message, st.Fork(), st.GenesisValidatorsRoot(), slot)
	env.Signature = sig[:]
	wrapped, err := blocks.WrappedROSignedExecutionPayloadEnvelope(env)
	require.NoError(t, err)

	verifier := &EnvelopeVerifier{results: newResults(RequireBuilderSignatureValid), e: wrapped}
	require.NoError(t, verifier.VerifySignature(t.Context(), st))
}

func testSignedExecutionPayloadEnvelope(t *testing.T, slot primitives.Slot, builderIdx primitives.BuilderIndex, root, blockHash [32]byte) *ethpb.SignedExecutionPayloadEnvelope {
	t.Helper()

	payload := &enginev1.ExecutionPayloadGloas{
		ParentHash:    bytes.Repeat([]byte{0x01}, 32),
		FeeRecipient:  bytes.Repeat([]byte{0x02}, 20),
		StateRoot:     bytes.Repeat([]byte{0x03}, 32),
		ReceiptsRoot:  bytes.Repeat([]byte{0x04}, 32),
		LogsBloom:     bytes.Repeat([]byte{0x05}, 256),
		PrevRandao:    bytes.Repeat([]byte{0x06}, 32),
		BlockNumber:   1,
		GasLimit:      2,
		GasUsed:       3,
		Timestamp:     4,
		BaseFeePerGas: bytes.Repeat([]byte{0x07}, 32),
		BlockHash:     blockHash[:],
		Transactions:  [][]byte{},
		Withdrawals:   []*enginev1.Withdrawal{},
		BlobGasUsed:   0,
		ExcessBlobGas: 0,
		SlotNumber:    slot,
	}

	return &ethpb.SignedExecutionPayloadEnvelope{
		Message: &ethpb.ExecutionPayloadEnvelope{
			Payload: payload,
			ExecutionRequests: &enginev1.ExecutionRequestsGloas{
				Deposits: []*enginev1.DepositRequest{},
			},
			BuilderIndex:          builderIdx,
			BeaconBlockRoot:       root[:],
			ParentBeaconBlockRoot: make([]byte, 32),
		},
		Signature: bytes.Repeat([]byte{0xCC}, 96),
	}
}

func testExecutionPayloadBid(t *testing.T, slot primitives.Slot, builderIdx primitives.BuilderIndex, blockHash [32]byte) interfaces.ROExecutionPayloadBid {
	t.Helper()

	signed := util.GenerateTestSignedExecutionPayloadBid(slot)
	signed.Message.BuilderIndex = builderIdx
	copy(signed.Message.BlockHash, blockHash[:])

	wrapped, err := blocks.WrappedROSignedExecutionPayloadBid(signed)
	require.NoError(t, err)
	bid, err := wrapped.Bid()
	require.NoError(t, err)
	return bid
}

func newGloasState(
	t *testing.T,
	slot primitives.Slot,
	validators []*ethpb.Validator,
	balances []uint64,
	builders []*ethpb.Builder,
) state.BeaconState {
	t.Helper()

	genesisRoot := bytes.Repeat([]byte{0x11}, 32)
	st, err := util.NewBeaconStateGloas(func(s *ethpb.BeaconStateGloas) error {
		s.Slot = slot
		s.GenesisValidatorsRoot = genesisRoot
		if validators != nil {
			s.Validators = validators
		}
		if balances != nil {
			s.Balances = balances
		}
		if s.LatestBlockHeader != nil {
			s.LatestBlockHeader.ProposerIndex = 0
		}
		if builders != nil {
			s.Builders = builders
		}
		return nil
	})
	require.NoError(t, err)
	return st
}

func signEnvelope(t *testing.T, sk bls.SecretKey, env *ethpb.ExecutionPayloadEnvelope, fork *ethpb.Fork, genesisRoot []byte, slot primitives.Slot) [96]byte {
	t.Helper()

	epoch := slots.ToEpoch(slot)
	domain, err := signing.Domain(fork, epoch, params.BeaconConfig().DomainBeaconBuilder, genesisRoot)
	require.NoError(t, err)
	root, err := signing.ComputeSigningRoot(env, domain)
	require.NoError(t, err)
	sig := sk.Sign(root[:]).Marshal()
	var out [96]byte
	copy(out[:], sig)
	return out
}
