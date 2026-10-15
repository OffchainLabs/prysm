package client

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	validatorpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/validator-client"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/pkg/errors"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"go.uber.org/mock/gomock"
)

func TestSubmitInclusionList(t *testing.T) {
	const (
		slot = primitives.Slot(1)
		idx  = primitives.ValidatorIndex(7)
	)
	depRoot := bytesutil.PadTo([]byte{0xde}, fieldparams.RootLength)
	gvr := bytesutil.PadTo([]byte{0x9a}, fieldparams.RootLength)
	txs := [][]byte{{0x01, 0x02}, {0x03}}

	// writeILDuty gives key an inclusion list duty at slot.
	writeILDuty := func(v *validator, key [fieldparams.BLSPubkeyLength]byte) {
		var data dutyStoreData
		data.setFromContainer(&ethpb.ValidatorDutiesContainer{
			PrevDependentRoot: depRoot,
			CurrentEpochDuties: []*ethpb.ValidatorDuty{
				{PublicKey: key[:], ValidatorIndex: idx, InclusionListSlots: []primitives.Slot{slot}},
			},
		})
		v.duties.write(data)
	}
	// ilSetup gives dutyKey an inclusion list duty at slot, with slot started elapsed ago.
	ilSetup := func(t *testing.T, dutyKey [fieldparams.BLSPubkeyLength]byte, elapsed time.Duration) (*validator, *mocks) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 0
		cfg.HezeForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		v, m, _, finish := setup(t, false)
		t.Cleanup(finish)
		require.NoError(t, v.db.SaveGenesisValidatorsRoot(t.Context(), gvr))
		v.genesisTime = time.Now().Add(-params.SlotsDuration(slot, cfg) - elapsed)

		writeILDuty(v, dutyKey)
		return v, m
	}
	keyOf := func(v *validator) [fieldparams.BLSPubkeyLength]byte {
		return v.km.(*mockKeymanager).keys[0]
	}

	// newIL returns a validator whose own key holds the duty.
	newIL := func(t *testing.T, elapsed time.Duration) (*validator, *mocks, [fieldparams.BLSPubkeyLength]byte) {
		v, m := ilSetup(t, [fieldparams.BLSPubkeyLength]byte{}, elapsed)
		pubKey := keyOf(v)
		writeILDuty(v, pubKey)
		return v, m, pubKey
	}

	t.Run("happy path", func(t *testing.T) {
		hook := logTest.NewGlobal()
		v, m, pubKey := newIL(t, 0)

		var got *ethpb.SignedInclusionList
		m.validatorClient.EXPECT().ProduceInclusionList(gomock.Any(), slot).Return(txs, nil)
		m.validatorClient.EXPECT().SubmitInclusionList(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ any, in *ethpb.SignedInclusionList) error {
				got = in
				return nil
			})

		v.SubmitInclusionList(t.Context(), slot, pubKey)

		require.NotNil(t, got)
		assert.Equal(t, slot, got.Message.Slot)
		assert.Equal(t, idx, got.Message.ValidatorIndex)
		assert.DeepEqual(t, depRoot, got.Message.DependentRoot)
		assert.DeepEqual(t, txs, got.Message.Transactions.Slice())

		domain, err := signing.ComputeDomain(params.BeaconConfig().DomainInclusionListCommittee, params.BeaconConfig().HezeForkVersion, gvr)
		require.NoError(t, err)
		root, err := signing.ComputeSigningRoot(got.Message, domain)
		require.NoError(t, err)
		req := v.km.(*mockKeymanager).lastSignRequest()
		assert.DeepEqual(t, domain, req.SignatureDomain)
		assert.DeepEqual(t, root[:], req.SigningRoot)
		_, ok := req.Object.(*validatorpb.SignRequest_InclusionList)
		assert.Equal(t, true, ok)

		pub, err := bls.PublicKeyFromBytes(pubKey[:])
		require.NoError(t, err)
		sig, err := bls.SignatureFromBytes(got.Signature)
		require.NoError(t, err)
		assert.Equal(t, true, sig.Verify(pub, root[:]))
		require.LogsContain(t, hook, "Submitted inclusion list")
	})

	t.Run("empty list is skipped", func(t *testing.T) {
		hook := logTest.NewGlobal()
		v, m, pubKey := newIL(t, 0)
		m.validatorClient.EXPECT().ProduceInclusionList(gomock.Any(), slot).Return([][]byte{}, nil)
		v.SubmitInclusionList(t.Context(), slot, pubKey)
		require.LogsContain(t, hook, "Skipping inclusion list: no transactions")
	})

	t.Run("past the due point is skipped", func(t *testing.T) {
		hook := logTest.NewGlobal()
		v, m, pubKey := newIL(t, time.Duration(params.BeaconConfig().SlotDurationMilliseconds)*time.Millisecond)
		m.validatorClient.EXPECT().ProduceInclusionList(gomock.Any(), slot).Return(txs, nil)
		v.SubmitInclusionList(t.Context(), slot, pubKey)
		require.LogsContain(t, hook, "Skipping inclusion list: past the inclusion list due point")
	})

	t.Run("produce error", func(t *testing.T) {
		hook := logTest.NewGlobal()
		v, m, pubKey := newIL(t, 0)
		m.validatorClient.EXPECT().ProduceInclusionList(gomock.Any(), slot).Return(nil, errors.New("el down"))
		v.SubmitInclusionList(t.Context(), slot, pubKey)
		require.LogsContain(t, hook, "Could not produce inclusion list")
	})

	t.Run("submit error", func(t *testing.T) {
		hook := logTest.NewGlobal()
		v, m, pubKey := newIL(t, 0)
		m.validatorClient.EXPECT().ProduceInclusionList(gomock.Any(), slot).Return(txs, nil)
		m.validatorClient.EXPECT().SubmitInclusionList(gomock.Any(), gomock.Any()).Return(errors.New("bad request"))
		v.SubmitInclusionList(t.Context(), slot, pubKey)
		require.LogsContain(t, hook, "Could not submit inclusion list")
	})

	t.Run("keymanager error", func(t *testing.T) {
		hook := logTest.NewGlobal()
		// The duty key is not in the keymanager, so signing fails.
		unknown := [fieldparams.BLSPubkeyLength]byte{0x42}
		v, m := ilSetup(t, unknown, 0)
		m.validatorClient.EXPECT().ProduceInclusionList(gomock.Any(), slot).Return(txs, nil)
		v.SubmitInclusionList(t.Context(), slot, unknown)
		require.LogsContain(t, hook, "Could not sign inclusion list")
	})

	t.Run("no duty at slot", func(t *testing.T) {
		hook := logTest.NewGlobal()
		v, _, pubKey := newIL(t, 0)
		v.SubmitInclusionList(t.Context(), slot+1, pubKey)
		require.LogsContain(t, hook, "Could not find inclusion list duty")
	})

	t.Run("pre-Heze does nothing", func(t *testing.T) {
		v, _, pubKey := newIL(t, 0)
		cfg := params.BeaconConfig().Copy()
		cfg.HezeForkEpoch = 10
		params.OverrideBeaconConfig(cfg)
		// No client expectations: any call fails the test.
		v.SubmitInclusionList(t.Context(), slot, pubKey)
	})
}
