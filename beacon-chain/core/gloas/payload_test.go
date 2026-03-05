package gloas

import (
	"bytes"
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"google.golang.org/protobuf/proto"
)

type payloadFixture struct {
	state       state.BeaconState
	signed      interfaces.ROSignedExecutionPayloadEnvelope
	signedProto *ethpb.SignedExecutionPayloadEnvelope
	envelope    *ethpb.ExecutionPayloadEnvelope
	payload     *enginev1.ExecutionPayloadDeneb
	slot        primitives.Slot
}

func buildPayloadFixture(t *testing.T, mutate func(payload *enginev1.ExecutionPayloadDeneb, bid *ethpb.ExecutionPayloadBid, envelope *ethpb.ExecutionPayloadEnvelope)) payloadFixture {
	t.Helper()

	cfg := params.BeaconConfig()
	slot := primitives.Slot(5)
	builderIdx := primitives.BuilderIndex(0)

	sk, err := bls.RandKey()
	require.NoError(t, err)
	pk := sk.PublicKey().Marshal()

	randao := bytes.Repeat([]byte{0xAA}, 32)
	parentHash := bytes.Repeat([]byte{0xBB}, 32)
	blockHash := bytes.Repeat([]byte{0xCC}, 32)

	withdrawals := []*enginev1.Withdrawal{
		{Index: 0, ValidatorIndex: 1, Address: bytes.Repeat([]byte{0x01}, 20), Amount: 0},
	}

	payload := &enginev1.ExecutionPayloadDeneb{
		ParentHash:    parentHash,
		FeeRecipient:  bytes.Repeat([]byte{0x01}, 20),
		StateRoot:     bytes.Repeat([]byte{0x02}, 32),
		ReceiptsRoot:  bytes.Repeat([]byte{0x03}, 32),
		LogsBloom:     bytes.Repeat([]byte{0x04}, 256),
		PrevRandao:    randao,
		BlockNumber:   1,
		GasLimit:      1,
		GasUsed:       0,
		Timestamp:     100,
		ExtraData:     []byte{},
		BaseFeePerGas: bytes.Repeat([]byte{0x05}, 32),
		BlockHash:     blockHash,
		Transactions:  [][]byte{},
		Withdrawals:   withdrawals,
		BlobGasUsed:   0,
		ExcessBlobGas: 0,
	}

	bid := &ethpb.ExecutionPayloadBid{
		ParentBlockHash:  parentHash,
		ParentBlockRoot:  bytes.Repeat([]byte{0xDD}, 32),
		BlockHash:        blockHash,
		PrevRandao:       randao,
		GasLimit:         1,
		BuilderIndex:     builderIdx,
		Slot:             slot,
		Value:            0,
		ExecutionPayment: 0,
		FeeRecipient:     bytes.Repeat([]byte{0xEE}, 20),
	}

	header := &ethpb.BeaconBlockHeader{
		Slot:       slot,
		ParentRoot: bytes.Repeat([]byte{0x11}, 32),
		StateRoot:  bytes.Repeat([]byte{0x22}, 32),
		BodyRoot:   bytes.Repeat([]byte{0x33}, 32),
	}
	headerRoot, err := header.HashTreeRoot()
	require.NoError(t, err)

	envelope := &ethpb.ExecutionPayloadEnvelope{
		Slot:              slot,
		BuilderIndex:      builderIdx,
		BeaconBlockRoot:   headerRoot[:],
		Payload:           payload,
		ExecutionRequests: &enginev1.ExecutionRequests{},
	}

	if mutate != nil {
		mutate(payload, bid, envelope)
	}

	genesisRoot := bytes.Repeat([]byte{0xAB}, 32)
	blockRoots := make([][]byte, cfg.SlotsPerHistoricalRoot)
	stateRoots := make([][]byte, cfg.SlotsPerHistoricalRoot)
	for i := range blockRoots {
		blockRoots[i] = bytes.Repeat([]byte{0x44}, 32)
		stateRoots[i] = bytes.Repeat([]byte{0x55}, 32)
	}
	randaoMixes := make([][]byte, cfg.EpochsPerHistoricalVector)
	for i := range randaoMixes {
		randaoMixes[i] = randao
	}

	withdrawalCreds := make([]byte, 32)
	withdrawalCreds[0] = cfg.ETH1AddressWithdrawalPrefixByte

	eth1Data := &ethpb.Eth1Data{
		DepositRoot:  bytes.Repeat([]byte{0x66}, 32),
		DepositCount: 0,
		BlockHash:    bytes.Repeat([]byte{0x77}, 32),
	}

	vals := []*ethpb.Validator{
		{
			PublicKey:             pk,
			WithdrawalCredentials: withdrawalCreds,
			EffectiveBalance:      cfg.MinActivationBalance + 1_000,
		},
	}
	balances := []uint64{cfg.MinActivationBalance + 1_000}

	payments := make([]*ethpb.BuilderPendingPayment, cfg.SlotsPerEpoch*2)
	for i := range payments {
		payments[i] = &ethpb.BuilderPendingPayment{
			Withdrawal: &ethpb.BuilderPendingWithdrawal{
				FeeRecipient: make([]byte, 20),
			},
		}
	}

	executionPayloadAvailability := make([]byte, cfg.SlotsPerHistoricalRoot/8)

	builders := make([]*ethpb.Builder, builderIdx+1)
	builders[builderIdx] = &ethpb.Builder{
		Pubkey:            pk,
		Version:           []byte{0},
		ExecutionAddress:  bytes.Repeat([]byte{0x09}, 20),
		Balance:           0,
		DepositEpoch:      0,
		WithdrawableEpoch: 0,
	}

	genesisTime := uint64(0)
	slotSeconds := cfg.SecondsPerSlot * uint64(slot)
	if payload.Timestamp > slotSeconds {
		genesisTime = payload.Timestamp - slotSeconds
	}

	stProto := &ethpb.BeaconStateGloas{
		Slot:                  slot,
		GenesisTime:           genesisTime,
		GenesisValidatorsRoot: genesisRoot,
		Fork: &ethpb.Fork{
			CurrentVersion:  bytes.Repeat([]byte{0x01}, 4),
			PreviousVersion: bytes.Repeat([]byte{0x01}, 4),
			Epoch:           0,
		},
		LatestBlockHeader:            header,
		BlockRoots:                   blockRoots,
		StateRoots:                   stateRoots,
		RandaoMixes:                  randaoMixes,
		Eth1Data:                     eth1Data,
		Validators:                   vals,
		Balances:                     balances,
		LatestBlockHash:              payload.ParentHash,
		LatestExecutionPayloadBid:    bid,
		BuilderPendingPayments:       payments,
		ExecutionPayloadAvailability: executionPayloadAvailability,
		BuilderPendingWithdrawals:    []*ethpb.BuilderPendingWithdrawal{},
		PayloadExpectedWithdrawals:   payload.Withdrawals,
		Builders:                     builders,
	}

	st, err := state_native.InitializeFromProtoGloas(stProto)
	require.NoError(t, err)

	expected := st.Copy()
	ctx := context.Background()
	require.NoError(t, processExecutionRequests(ctx, expected, envelope.ExecutionRequests))
	require.NoError(t, expected.QueueBuilderPayment())
	require.NoError(t, expected.SetExecutionPayloadAvailability(slot, true))
	var blockHashArr [32]byte
	copy(blockHashArr[:], payload.BlockHash)
	require.NoError(t, expected.SetLatestBlockHash(blockHashArr))
	expectedRoot, err := expected.HashTreeRoot(ctx)
	require.NoError(t, err)
	envelope.StateRoot = expectedRoot[:]

	epoch := slots.ToEpoch(slot)
	domain, err := signing.Domain(st.Fork(), epoch, cfg.DomainBeaconBuilder, st.GenesisValidatorsRoot())
	require.NoError(t, err)
	signingRoot, err := signing.ComputeSigningRoot(envelope, domain)
	require.NoError(t, err)
	signature := sk.Sign(signingRoot[:]).Marshal()

	signedProto := &ethpb.SignedExecutionPayloadEnvelope{
		Message:   envelope,
		Signature: signature,
	}
	signed, err := blocks.WrappedROSignedExecutionPayloadEnvelope(signedProto)
	require.NoError(t, err)

	return payloadFixture{
		state:       st,
		signed:      signed,
		signedProto: signedProto,
		envelope:    envelope,
		payload:     payload,
		slot:        slot,
	}
}

func TestProcessExecutionPayload_Success(t *testing.T) {
	fixture := buildPayloadFixture(t, nil)
	require.NoError(t, ProcessExecutionPayload(t.Context(), fixture.state, fixture.signed))

	latestHash, err := fixture.state.LatestBlockHash()
	require.NoError(t, err)
	var expectedHash [32]byte
	copy(expectedHash[:], fixture.payload.BlockHash)
	require.Equal(t, expectedHash, latestHash)

	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	paymentIndex := slotsPerEpoch + (fixture.slot % slotsPerEpoch)
	payments, err := fixture.state.BuilderPendingPayments()
	require.NoError(t, err)
	payment := payments[paymentIndex]
	require.NotNil(t, payment)
	require.Equal(t, primitives.Gwei(0), payment.Withdrawal.Amount)
}

func TestApplyExecutionPayloadStateMutations_UpdatesAvailabilityAndLatestHash(t *testing.T) {
	fixture := buildPayloadFixture(t, nil)

	newHash := [32]byte{}
	newHash[0] = 0x99

	require.NoError(t, ApplyExecutionPayloadStateMutations(t.Context(), fixture.state, fixture.envelope.ExecutionRequests, newHash))

	latestHash, err := fixture.state.LatestBlockHash()
	require.NoError(t, err)
	require.Equal(t, newHash, latestHash)

	available, err := fixture.state.ExecutionPayloadAvailability(fixture.slot)
	require.NoError(t, err)
	require.Equal(t, uint64(1), available)
}

func TestProcessExecutionPayload_PrevRandaoMismatch(t *testing.T) {
	fixture := buildPayloadFixture(t, func(_ *enginev1.ExecutionPayloadDeneb, bid *ethpb.ExecutionPayloadBid, _ *ethpb.ExecutionPayloadEnvelope) {
		bid.PrevRandao = bytes.Repeat([]byte{0xFF}, 32)
	})

	err := ProcessExecutionPayload(t.Context(), fixture.state, fixture.signed)
	require.ErrorContains(t, "prev randao", err)
}

func TestQueueBuilderPayment_ZeroAmountClearsSlot(t *testing.T) {
	fixture := buildPayloadFixture(t, nil)

	require.NoError(t, fixture.state.QueueBuilderPayment())

	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	paymentIndex := slotsPerEpoch + (fixture.slot % slotsPerEpoch)
	payments, err := fixture.state.BuilderPendingPayments()
	require.NoError(t, err)
	payment := payments[paymentIndex]
	require.NotNil(t, payment)
	require.Equal(t, primitives.Gwei(0), payment.Withdrawal.Amount)
}

func TestVerifyExecutionPayloadEnvelopeSignature(t *testing.T) {
	fixture := buildPayloadFixture(t, nil)

	t.Run("self build", func(t *testing.T) {
		proposerSk, err := bls.RandKey()
		require.NoError(t, err)
		proposerPk := proposerSk.PublicKey().Marshal()

		stPb, ok := fixture.state.ToProtoUnsafe().(*ethpb.BeaconStateGloas)
		require.Equal(t, true, ok)
		stPb = proto.Clone(stPb).(*ethpb.BeaconStateGloas)
		stPb.Validators[0].PublicKey = proposerPk
		st, err := state_native.InitializeFromProtoUnsafeGloas(stPb)
		require.NoError(t, err)

		msg := proto.Clone(fixture.signedProto.Message).(*ethpb.ExecutionPayloadEnvelope)
		msg.BuilderIndex = params.BeaconConfig().BuilderIndexSelfBuild

		epoch := slots.ToEpoch(msg.Slot)
		domain, err := signing.Domain(st.Fork(), epoch, params.BeaconConfig().DomainBeaconBuilder, st.GenesisValidatorsRoot())
		require.NoError(t, err)
		signingRoot, err := signing.ComputeSigningRoot(msg, domain)
		require.NoError(t, err)
		signature := proposerSk.Sign(signingRoot[:]).Marshal()

		signedProto := &ethpb.SignedExecutionPayloadEnvelope{
			Message:   msg,
			Signature: signature,
		}
		signed, err := blocks.WrappedROSignedExecutionPayloadEnvelope(signedProto)
		require.NoError(t, err)

		require.NoError(t, VerifyExecutionPayloadEnvelopeSignature(st, signed))
	})

	t.Run("builder", func(t *testing.T) {
		signed, err := blocks.WrappedROSignedExecutionPayloadEnvelope(fixture.signedProto)
		require.NoError(t, err)

		require.NoError(t, VerifyExecutionPayloadEnvelopeSignature(fixture.state, signed))
	})

	t.Run("invalid signature", func(t *testing.T) {
		t.Run("self build", func(t *testing.T) {
			proposerSk, err := bls.RandKey()
			require.NoError(t, err)
			proposerPk := proposerSk.PublicKey().Marshal()

			stPb, ok := fixture.state.ToProtoUnsafe().(*ethpb.BeaconStateGloas)
			require.Equal(t, true, ok)
			stPb = proto.Clone(stPb).(*ethpb.BeaconStateGloas)
			stPb.Validators[0].PublicKey = proposerPk
			st, err := state_native.InitializeFromProtoUnsafeGloas(stPb)
			require.NoError(t, err)

			msg := proto.Clone(fixture.signedProto.Message).(*ethpb.ExecutionPayloadEnvelope)
			msg.BuilderIndex = params.BeaconConfig().BuilderIndexSelfBuild

			signedProto := &ethpb.SignedExecutionPayloadEnvelope{
				Message:   msg,
				Signature: bytes.Repeat([]byte{0xFF}, 96),
			}
			badSigned, err := blocks.WrappedROSignedExecutionPayloadEnvelope(signedProto)
			require.NoError(t, err)

			err = VerifyExecutionPayloadEnvelopeSignature(st, badSigned)
			require.ErrorContains(t, "invalid signature format", err)
		})

		t.Run("builder", func(t *testing.T) {
			signedProto := &ethpb.SignedExecutionPayloadEnvelope{
				Message:   fixture.signedProto.Message,
				Signature: bytes.Repeat([]byte{0xFF}, 96),
			}
			badSigned, err := blocks.WrappedROSignedExecutionPayloadEnvelope(signedProto)
			require.NoError(t, err)

			err = VerifyExecutionPayloadEnvelopeSignature(fixture.state, badSigned)
			require.ErrorContains(t, "invalid signature format", err)
		})
	})
}
