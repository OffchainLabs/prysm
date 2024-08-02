package verification

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/startup"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	payloadattestation "github.com/prysmaticlabs/prysm/v5/consensus-types/epbs/payload-attestation"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

func TestVerifyCurrentSlot(t *testing.T) {
	now := time.Now()
	// make genesis 1 slot in the past
	genesis := now.Add(-1 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second)
	clock := startup.NewClock(genesis, [32]byte{}, startup.WithNower(func() time.Time { return now }))

	init := Initializer{shared: &sharedResources{clock: clock}}

	t.Run("incorrect slot", func(t *testing.T) {
		pa := newPayloadAttestation(t, &ethpb.PayloadAttestationMessage{
			Data:      &ethpb.PayloadAttestationData{},
			Signature: make([]byte, 96),
		}, init)
		require.ErrorIs(t, pa.VerifyCurrentSlot(), ErrIncorrectPayloadAttSlot)
		require.Equal(t, true, pa.results.executed(RequireCurrentSlot))
		require.Equal(t, ErrIncorrectPayloadAttSlot, pa.results.result(RequireCurrentSlot))
	})

	t.Run("current slot", func(t *testing.T) {
		pa := newPayloadAttestation(t, &ethpb.PayloadAttestationMessage{
			Data: &ethpb.PayloadAttestationData{
				Slot: 1,
			},
			Signature: make([]byte, 96),
		}, init)
		require.NoError(t, pa.VerifyCurrentSlot())
		require.Equal(t, true, pa.results.executed(RequireCurrentSlot))
		require.NoError(t, pa.results.result(RequireCurrentSlot))
	})
}

func TestVerifyKnownPayloadStatus(t *testing.T) {
	init := Initializer{shared: &sharedResources{clock: &startup.Clock{}}}

	t.Run("unknown status", func(t *testing.T) {
		pa := newPayloadAttestation(t, &ethpb.PayloadAttestationMessage{
			Data: &ethpb.PayloadAttestationData{
				PayloadStatus: primitives.PAYLOAD_INVALID_STATUS,
			},
			Signature: make([]byte, 96),
		}, init)
		require.ErrorIs(t, pa.VerifyPayloadStatus(), ErrIncorrectPayloadAttStatus)
		require.Equal(t, true, pa.results.executed(RequireKnownPayloadStatus))
		require.Equal(t, ErrIncorrectPayloadAttStatus, pa.results.result(RequireKnownPayloadStatus))
	})

	t.Run("known status", func(t *testing.T) {
		pa := newPayloadAttestation(t, &ethpb.PayloadAttestationMessage{
			Data:      &ethpb.PayloadAttestationData{},
			Signature: make([]byte, 96),
		}, init)
		require.NoError(t, pa.VerifyPayloadStatus())
		require.Equal(t, true, pa.results.executed(RequireKnownPayloadStatus))
		require.NoError(t, pa.results.result(RequireKnownPayloadStatus))
	})
}

func TestVerifyBlockRootSeen(t *testing.T) {
	blockRoot := [32]byte{1}

	fc := &mockForkchoicer{
		HasNodeCB: func(parent [32]byte) bool {
			return parent == blockRoot
		},
	}

	t.Run("happy path", func(t *testing.T) {
		init := Initializer{shared: &sharedResources{fc: fc}}
		pa := newPayloadAttestation(t, &ethpb.PayloadAttestationMessage{
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot: blockRoot[:],
			},
			Signature: make([]byte, 96),
		}, init)
		require.NoError(t, pa.VerifyBlockRootSeen(nil))
		require.Equal(t, true, pa.results.executed(RequireBlockRootSeen))
		require.NoError(t, pa.results.result(RequireBlockRootSeen))
	})

	t.Run("unknown block", func(t *testing.T) {
		init := Initializer{shared: &sharedResources{fc: fc}}
		pa := newPayloadAttestation(t, &ethpb.PayloadAttestationMessage{
			Data:      &ethpb.PayloadAttestationData{},
			Signature: make([]byte, 96),
		}, init)
		require.ErrorIs(t, pa.VerifyBlockRootSeen(nil), ErrPayloadAttBlockRootNotSeen)
		require.Equal(t, true, pa.results.executed(RequireBlockRootSeen))
		require.Equal(t, ErrPayloadAttBlockRootNotSeen, pa.results.result(RequireBlockRootSeen))
	})

	t.Run("bad parent true", func(t *testing.T) {
		init := Initializer{shared: &sharedResources{}}
		pa := newPayloadAttestation(t, &ethpb.PayloadAttestationMessage{
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot: blockRoot[:],
			},
			Signature: make([]byte, 96),
		}, init)
		require.NoError(t, pa.VerifyBlockRootSeen(badParentCb(t, blockRoot, true)))
		require.Equal(t, true, pa.results.executed(RequireBlockRootSeen))
		require.NoError(t, pa.results.result(RequireBlockRootSeen))
	})

	t.Run("bad parent false, unknown block", func(t *testing.T) {
		init := Initializer{shared: &sharedResources{fc: fc}}
		pa := newPayloadAttestation(t, &ethpb.PayloadAttestationMessage{
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot: []byte{2},
			},
			Signature: make([]byte, 96),
		}, init)
		require.ErrorIs(t, pa.VerifyBlockRootSeen(badParentCb(t, [32]byte{2}, false)), ErrPayloadAttBlockRootNotSeen)
		require.Equal(t, true, pa.results.executed(RequireBlockRootSeen))
		require.Equal(t, ErrPayloadAttBlockRootNotSeen, pa.results.result(RequireBlockRootSeen))
	})
}

func TestVerifyBlockRootValid(t *testing.T) {
	blockRoot := [32]byte{1}

	t.Run("good block", func(t *testing.T) {
		init := Initializer{shared: &sharedResources{}}
		pa := newPayloadAttestation(t, &ethpb.PayloadAttestationMessage{
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot: blockRoot[:],
			},
			Signature: make([]byte, 96),
		}, init)
		require.NoError(t, pa.VerifyBlockRootValid(badParentCb(t, blockRoot, false)))
		require.Equal(t, true, pa.results.executed(RequireBlockRootValid))
		require.NoError(t, pa.results.result(RequireBlockRootValid))
	})

	t.Run("bad block", func(t *testing.T) {
		init := Initializer{shared: &sharedResources{}}
		pa := newPayloadAttestation(t, &ethpb.PayloadAttestationMessage{
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot: blockRoot[:],
			},
			Signature: make([]byte, 96),
		}, init)
		require.ErrorIs(t, pa.VerifyBlockRootValid(badParentCb(t, blockRoot, true)), ErrPayloadAttBlockRootInvalid)
		require.Equal(t, true, pa.results.executed(RequireBlockRootValid))
		require.Equal(t, ErrPayloadAttBlockRootInvalid, pa.results.result(RequireBlockRootValid))
	})
}

func TestGetPayloadTimelinessCommittee(t *testing.T) {
	validators := make([]*ethpb.Validator, 4*params.BeaconConfig().TargetCommitteeSize*uint64(params.BeaconConfig().SlotsPerEpoch))
	validatorIndices := make([]primitives.ValidatorIndex, len(validators))

	for i := 0; i < len(validators); i++ {
		k := make([]byte, 48)
		copy(k, strconv.Itoa(i))
		validators[i] = &ethpb.Validator{
			PublicKey:             k,
			WithdrawalCredentials: make([]byte, 32),
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
		}
		validatorIndices[i] = primitives.ValidatorIndex(i)
	}

	st, err := state_native.InitializeFromProtoEpbs(&ethpb.BeaconStateEPBS{
		Validators:  validators,
		RandaoMixes: make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),
	})
	require.NoError(t, err)

	slot := primitives.Slot(1)
	ctx := context.Background()
	ptc, err := helpers.GetPayloadTimelinessCommittee(ctx, st, slot)
	require.NoError(t, err)

	t.Run("in committee", func(t *testing.T) {
		init := Initializer{shared: &sharedResources{}}
		pa := newPayloadAttestation(t, &ethpb.PayloadAttestationMessage{
			ValidatorIndex: ptc[0],
			Data: &ethpb.PayloadAttestationData{
				Slot: slot,
			},
			Signature: make([]byte, 96),
		}, init)
		require.NoError(t, pa.VerifyValidatorInPTC(ctx, st))
		require.Equal(t, true, pa.results.executed(RequireValidatorInPTC))
		require.NoError(t, pa.results.result(RequireValidatorInPTC))
	})

	t.Run("not in committee", func(t *testing.T) {
		init := Initializer{shared: &sharedResources{}}
		pa := newPayloadAttestation(t, &ethpb.PayloadAttestationMessage{
			Data: &ethpb.PayloadAttestationData{
				Slot: slot,
			},
			Signature: make([]byte, 96),
		}, init)
		require.ErrorIs(t, pa.VerifyValidatorInPTC(ctx, st), ErrIncorrectPayloadAttValidator)
		require.Equal(t, true, pa.results.executed(RequireValidatorInPTC))
		require.Equal(t, ErrIncorrectPayloadAttValidator, pa.results.result(RequireValidatorInPTC))
	})
}

func TestPayloadAttestationVerifySignature(t *testing.T) {
	_, secretKeys, err := util.DeterministicDepositsAndKeys(2)
	require.NoError(t, err)

	st, err := state_native.InitializeFromProtoEpbs(&ethpb.BeaconStateEPBS{
		Validators: []*ethpb.Validator{{PublicKey: secretKeys[0].PublicKey().Marshal()},
			{PublicKey: secretKeys[1].PublicKey().Marshal()}},
		Fork: &ethpb.Fork{
			CurrentVersion:  params.BeaconConfig().EPBSForkVersion,
			PreviousVersion: params.BeaconConfig().ElectraForkVersion,
		},
	})
	require.NoError(t, err)

	t.Run("valid signature", func(t *testing.T) {
		init := Initializer{shared: &sharedResources{}}
		d := &ethpb.PayloadAttestationData{
			BeaconBlockRoot: bytesutil.PadTo([]byte{'a'}, 32),
			Slot:            1,
			PayloadStatus:   primitives.PAYLOAD_WITHHELD,
		}
		signedBytes, err := signing.ComputeDomainAndSign(
			st,
			slots.ToEpoch(d.Slot),
			d,
			params.BeaconConfig().DomainPTCAttester,
			secretKeys[0],
		)
		require.NoError(t, err)
		sig, err := bls.SignatureFromBytes(signedBytes)
		require.NoError(t, err)
		pa := newPayloadAttestation(t, &ethpb.PayloadAttestationMessage{
			Data:      d,
			Signature: sig.Marshal(),
		}, init)
		require.NoError(t, pa.VerifySignature(st))
		require.Equal(t, true, pa.results.executed(RequireSignatureValid))
		require.NoError(t, pa.results.result(RequireSignatureValid))
	})

	t.Run("invalid signature", func(t *testing.T) {
		init := Initializer{shared: &sharedResources{}}
		d := &ethpb.PayloadAttestationData{
			BeaconBlockRoot: bytesutil.PadTo([]byte{'a'}, 32),
			Slot:            1,
			PayloadStatus:   primitives.PAYLOAD_WITHHELD,
		}
		signedBytes, err := signing.ComputeDomainAndSign(
			st,
			slots.ToEpoch(d.Slot),
			d,
			params.BeaconConfig().DomainPTCAttester,
			secretKeys[0],
		)
		require.NoError(t, err)
		sig, err := bls.SignatureFromBytes(signedBytes)
		require.NoError(t, err)
		pa := newPayloadAttestation(t, &ethpb.PayloadAttestationMessage{
			ValidatorIndex: 1,
			Data:           d,
			Signature:      sig.Marshal(),
		}, init)
		require.ErrorIs(t, pa.VerifySignature(st), signing.ErrSigFailedToVerify)
		require.Equal(t, true, pa.results.executed(RequireSignatureValid))
		require.Equal(t, signing.ErrSigFailedToVerify, pa.results.result(RequireSignatureValid))
	})
}

func TestVerifiedPayloadAttestation(t *testing.T) {
	init := Initializer{shared: &sharedResources{}}
	pa := newPayloadAttestation(t, &ethpb.PayloadAttestationMessage{
		Data:      &ethpb.PayloadAttestationData{},
		Signature: make([]byte, 96),
	}, init)

	t.Run("missing last requirement", func(t *testing.T) {
		for _, requirement := range GossipPayloadAttestationMessageRequirements[:len(GossipPayloadAttestationMessageRequirements)-1] {
			pa.SatisfyRequirement(requirement)
		}
		_, err := pa.VerifiedPayloadAttestation()
		require.ErrorIs(t, err, ErrInvalidPayloadAttMessage)
	})

	t.Run("satisfy all the requirements", func(t *testing.T) {
		for _, requirement := range GossipPayloadAttestationMessageRequirements {
			pa.SatisfyRequirement(requirement)
		}
		_, err := pa.VerifiedPayloadAttestation()
		require.NoError(t, err)
	})
}

func newPayloadAttestation(t *testing.T, m *ethpb.PayloadAttestationMessage, init Initializer) *PayloadAttMsgVerifier {
	ro, err := payloadattestation.NewReadOnly(m)
	require.NoError(t, err)
	return init.NewPayloadAttestationMsgVerifier(ro, GossipPayloadAttestationMessageRequirements)
}
