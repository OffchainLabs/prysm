package gloas_test

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

func TestUpgradeToGloas_Basic(t *testing.T) {
	st, _ := util.DeterministicGenesisStateFulu(t, params.BeaconConfig().MaxValidatorsPerCommittee)

	require.NoError(t, st.SetHistoricalRoots([][]byte{{1}}))

	lookaheadSize := int(params.BeaconConfig().MinSeedLookahead+1) * int(params.BeaconConfig().SlotsPerEpoch)
	lookahead := make([]primitives.ValidatorIndex, lookaheadSize)
	for i := range lookahead {
		lookahead[i] = primitives.ValidatorIndex(i)
	}
	require.NoError(t, st.SetProposerLookahead(lookahead))

	require.NoError(t, st.SetPendingPartialWithdrawals([]*ethpb.PendingPartialWithdrawal{{Index: 1, Amount: 2}}))
	require.NoError(t, st.SetPendingConsolidations([]*ethpb.PendingConsolidation{{SourceIndex: 3, TargetIndex: 4}}))

	blockHash := bytes.Repeat([]byte{0xAB}, 32)
	header := &enginev1.ExecutionPayloadHeaderDeneb{BlockHash: blockHash}
	wrappedHeader, err := consensusblocks.WrappedExecutionPayloadHeaderDeneb(header)
	require.NoError(t, err)
	require.NoError(t, st.SetLatestExecutionPayloadHeader(wrappedHeader))

	preForkState := st.Copy()
	mSt, err := gloas.UpgradeToGloas(t.Context(), st)
	require.NoError(t, err)

	require.Equal(t, preForkState.GenesisTime(), mSt.GenesisTime())
	require.DeepSSZEqual(t, preForkState.GenesisValidatorsRoot(), mSt.GenesisValidatorsRoot())
	require.Equal(t, preForkState.Slot(), mSt.Slot())

	require.DeepSSZEqual(t, &ethpb.Fork{
		PreviousVersion: st.Fork().CurrentVersion,
		CurrentVersion:  params.BeaconConfig().GloasForkVersion,
		Epoch:           time.CurrentEpoch(st),
	}, mSt.Fork())

	bid, err := mSt.LatestExecutionPayloadBid()
	require.NoError(t, err)
	wantBlockHash := [32]byte{}
	copy(wantBlockHash[:], blockHash)
	require.DeepSSZEqual(t, wantBlockHash, bid.BlockHash())
	require.DeepSSZEqual(t, [20]byte{}, bid.FeeRecipient())
	require.DeepSSZEqual(t, [32]byte{}, bid.ParentBlockHash())
	require.DeepSSZEqual(t, [32]byte{}, bid.ParentBlockRoot())
	require.DeepSSZEqual(t, [32]byte{}, bid.PrevRandao())

	latestBlockHash, err := mSt.LatestBlockHash()
	require.NoError(t, err)
	require.DeepSSZEqual(t, blockHash, latestBlockHash[:])

	pbState, ok := mSt.ToProtoUnsafe().(*ethpb.BeaconStateGloas)
	require.Equal(t, true, ok)

	expectedAvailLen := int((params.BeaconConfig().SlotsPerHistoricalRoot + 7) / 8)
	require.Equal(t, expectedAvailLen, len(pbState.ExecutionPayloadAvailability))
	for _, b := range pbState.ExecutionPayloadAvailability {
		require.Equal(t, byte(0xff), b)
	}

	require.Equal(t, 0, len(pbState.Builders))
	require.Equal(t, primitives.BuilderIndex(0), pbState.NextWithdrawalBuilderIndex)
	require.Equal(t, 0, len(pbState.BuilderPendingWithdrawals))
	require.Equal(t, 0, len(pbState.PayloadExpectedWithdrawals))

	require.Equal(t, int(params.BeaconConfig().SlotsPerEpoch*2), len(pbState.BuilderPendingPayments))
	for _, payment := range pbState.BuilderPendingPayments {
		require.NotNil(t, payment)
		require.NotNil(t, payment.Withdrawal)
		require.Equal(t, fieldparams.FeeRecipientLength, len(payment.Withdrawal.FeeRecipient))
	}

	ppw, err := mSt.PendingPartialWithdrawals()
	require.NoError(t, err)
	prePPW, err := preForkState.PendingPartialWithdrawals()
	require.NoError(t, err)
	require.DeepSSZEqual(t, prePPW, ppw)

	pc, err := mSt.PendingConsolidations()
	require.NoError(t, err)
	prePC, err := preForkState.PendingConsolidations()
	require.NoError(t, err)
	require.DeepSSZEqual(t, prePC, pc)
}

func TestUpgradeToGloas_OnboardsBuilderDeposit(t *testing.T) {
	st, _ := util.DeterministicGenesisStateFulu(t, params.BeaconConfig().MaxValidatorsPerCommittee)

	sk, err := bls.RandKey()
	require.NoError(t, err)
	builderCreds := builderWithdrawalCredentials(0xDD)
	amount := uint64(1234)
	depSlot := primitives.Slot(params.BeaconConfig().SlotsPerEpoch*2 + 3)
	deposit := newPendingDeposit(t, sk, builderCreds, amount, depSlot, true)

	require.NoError(t, st.SetPendingDeposits([]*ethpb.PendingDeposit{deposit}))

	mSt, err := gloas.UpgradeToGloas(t.Context(), st)
	require.NoError(t, err)

	pbState, ok := mSt.ToProtoUnsafe().(*ethpb.BeaconStateGloas)
	require.Equal(t, true, ok)

	require.Equal(t, 0, len(pbState.PendingDeposits))
	require.Equal(t, 1, len(pbState.Builders))

	builder := pbState.Builders[0]
	require.DeepSSZEqual(t, sk.PublicKey().Marshal(), builder.Pubkey)
	require.DeepSSZEqual(t, builderCreds[12:], builder.ExecutionAddress)
	require.Equal(t, primitives.Gwei(amount), builder.Balance)
	require.Equal(t, slots.ToEpoch(depSlot), builder.DepositEpoch)
}

func builderWithdrawalCredentials(addrByte byte) []byte {
	wc := make([]byte, fieldparams.RootLength)
	wc[0] = params.BeaconConfig().BuilderWithdrawalPrefixByte
	for i := 12; i < len(wc); i++ {
		wc[i] = addrByte
	}
	return wc
}

func newPendingDeposit(
	t *testing.T,
	sk bls.SecretKey,
	withdrawalCredentials []byte,
	amount uint64,
	slot primitives.Slot,
	valid bool,
) *ethpb.PendingDeposit {
	t.Helper()
	signature := make([]byte, fieldparams.BLSSignatureLength)
	if valid {
		signature = signDeposit(t, sk, withdrawalCredentials, amount)
	}
	return &ethpb.PendingDeposit{
		PublicKey:             sk.PublicKey().Marshal(),
		WithdrawalCredentials: withdrawalCredentials,
		Amount:                amount,
		Signature:             signature,
		Slot:                  slot,
	}
}

func signDeposit(t *testing.T, sk bls.SecretKey, withdrawalCredentials []byte, amount uint64) []byte {
	t.Helper()
	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
	require.NoError(t, err)
	msg := &ethpb.DepositMessage{
		PublicKey:             sk.PublicKey().Marshal(),
		WithdrawalCredentials: withdrawalCredentials,
		Amount:                amount,
	}
	signingRoot, err := signing.ComputeSigningRoot(msg, domain)
	require.NoError(t, err)
	sig := sk.Sign(signingRoot[:])
	return sig.Marshal()
}
