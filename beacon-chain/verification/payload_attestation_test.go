package verification

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	payloadattestation "github.com/OffchainLabs/prysm/v7/consensus-types/payload-attestation"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	testutil "github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

func TestPayloadAttestationVerifyCurrentSlot(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	now := time.Unix(1000, 0)
	genesis := now.Add(-time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second)
	clock := startup.NewClock(genesis, [32]byte{}, startup.WithNower(func() time.Time { return now }))
	ini := &Initializer{shared: &sharedResources{clock: clock}}

	msg := newPayloadAttestationMessage(primitives.Slot(1), 0, bytes.Repeat([]byte{0x11}, 32))
	pa, err := payloadattestation.NewReadOnly(msg)
	require.NoError(t, err)
	v := ini.NewPayloadAttestationMsgVerifier(pa, GossipPayloadAttestationMessageRequirements)
	require.NoError(t, v.VerifyCurrentSlot())

	msg = newPayloadAttestationMessage(primitives.Slot(2), 0, bytes.Repeat([]byte{0x11}, 32))
	pa, err = payloadattestation.NewReadOnly(msg)
	require.NoError(t, err)
	v = ini.NewPayloadAttestationMsgVerifier(pa, GossipPayloadAttestationMessageRequirements)
	require.ErrorIs(t, v.VerifyCurrentSlot(), ErrIncorrectPayloadAttSlot)
}

func TestPayloadAttestationVerifyBlockRootSeenAndValid(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	ini := &Initializer{shared: &sharedResources{}}
	root := bytes.Repeat([]byte{0x22}, 32)
	var root32 [32]byte
	copy(root32[:], root)

	msg := newPayloadAttestationMessage(primitives.Slot(1), 0, root)
	pa, err := payloadattestation.NewReadOnly(msg)
	require.NoError(t, err)
	v := ini.NewPayloadAttestationMsgVerifier(pa, GossipPayloadAttestationMessageRequirements)

	require.NoError(t, v.VerifyBlockRootSeen(func(r [32]byte) bool { return r == root32 }))
	require.ErrorIs(t, v.VerifyBlockRootSeen(func([32]byte) bool { return false }), ErrPayloadAttBlockRootNotSeen)

	require.NoError(t, v.VerifyBlockRootValid(func([32]byte) bool { return false }))
	require.ErrorIs(t, v.VerifyBlockRootValid(func([32]byte) bool { return true }), ErrPayloadAttBlockRootInvalid)
}

func TestPayloadAttestationVerifyValidatorInPTC(t *testing.T) {
	setupPayloadAttTestConfig(t)

	_, pk := newKey(t)
	st := newTestState(t, []*eth.Validator{activeValidator(pk)}, 1)
	msg := newPayloadAttestationMessage(primitives.Slot(1), 0, bytes.Repeat([]byte{0x33}, 32))
	pa, err := payloadattestation.NewReadOnly(msg)
	require.NoError(t, err)
	v := (&Initializer{shared: &sharedResources{}}).NewPayloadAttestationMsgVerifier(pa, GossipPayloadAttestationMessageRequirements)
	require.NoError(t, v.VerifyValidatorInPTC(context.Background(), st))

	msg = newPayloadAttestationMessage(primitives.Slot(1), 1, bytes.Repeat([]byte{0x33}, 32))
	pa, err = payloadattestation.NewReadOnly(msg)
	require.NoError(t, err)
	v = (&Initializer{shared: &sharedResources{}}).NewPayloadAttestationMsgVerifier(pa, GossipPayloadAttestationMessageRequirements)
	require.ErrorIs(t, v.VerifyValidatorInPTC(context.Background(), st), ErrIncorrectPayloadAttValidator)
}

func TestPayloadAttestationVerifySignature(t *testing.T) {
	setupPayloadAttTestConfig(t)

	sk, pk := newKey(t)
	st := newTestState(t, []*eth.Validator{activeValidator(pk)}, 1)
	root := bytes.Repeat([]byte{0x44}, 32)
	data := &eth.PayloadAttestationData{
		BeaconBlockRoot:   root,
		Slot:              1,
		PayloadPresent:    true,
		BlobDataAvailable: true,
	}
	msg := &eth.PayloadAttestationMessage{
		ValidatorIndex: 0,
		Data:           data,
		Signature:      signPayloadAttestationMessage(t, st, data, sk),
	}
	pa, err := payloadattestation.NewReadOnly(msg)
	require.NoError(t, err)
	v := (&Initializer{shared: &sharedResources{}}).NewPayloadAttestationMsgVerifier(pa, GossipPayloadAttestationMessageRequirements)
	require.NoError(t, v.VerifySignature(st))

	sk2, _ := newKey(t)
	msg.Signature = signPayloadAttestationMessage(t, st, data, sk2)
	pa, err = payloadattestation.NewReadOnly(msg)
	require.NoError(t, err)
	v = (&Initializer{shared: &sharedResources{}}).NewPayloadAttestationMsgVerifier(pa, GossipPayloadAttestationMessageRequirements)
	require.ErrorIs(t, v.VerifySignature(st), signing.ErrSigFailedToVerify)
}

func newPayloadAttestationMessage(slot primitives.Slot, idx primitives.ValidatorIndex, root []byte) *eth.PayloadAttestationMessage {
	return &eth.PayloadAttestationMessage{
		ValidatorIndex: idx,
		Data: &eth.PayloadAttestationData{
			BeaconBlockRoot:   root,
			Slot:              slot,
			PayloadPresent:    true,
			BlobDataAvailable: true,
		},
		Signature: []byte{0x01},
	}
}

func newTestState(t *testing.T, vals []*eth.Validator, slot primitives.Slot) state.BeaconState {
	st, err := testutil.NewBeaconStateGloas()
	require.NoError(t, err)
	for _, v := range vals {
		require.NoError(t, st.AppendValidator(v))
		require.NoError(t, st.AppendBalance(v.EffectiveBalance))
	}
	require.NoError(t, st.SetSlot(slot))
	require.NoError(t, helpers.UpdateCommitteeCache(t.Context(), st, slots.ToEpoch(slot)))
	return st
}

func setupPayloadAttTestConfig(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotsPerEpoch = 1
	cfg.MaxEffectiveBalanceElectra = cfg.MaxEffectiveBalance
	params.OverrideBeaconConfig(cfg)
}

func activeValidator(pub []byte) *eth.Validator {
	return &eth.Validator{
		PublicKey:             pub,
		EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
		WithdrawalCredentials: make([]byte, 32),
		ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
		WithdrawableEpoch:     params.BeaconConfig().FarFutureEpoch,
	}
}

func newKey(t *testing.T) (common.SecretKey, []byte) {
	sk, err := bls.RandKey()
	require.NoError(t, err)
	return sk, sk.PublicKey().Marshal()
}

func signPayloadAttestationMessage(t *testing.T, st state.ReadOnlyBeaconState, data *eth.PayloadAttestationData, sk common.SecretKey) []byte {
	domain, err := signing.Domain(st.Fork(), slots.ToEpoch(st.Slot()), params.BeaconConfig().DomainPTCAttester, st.GenesisValidatorsRoot())
	require.NoError(t, err)
	root, err := signing.ComputeSigningRoot(data, domain)
	require.NoError(t, err)
	sig := sk.Sign(root[:])
	return sig.Marshal()
}
