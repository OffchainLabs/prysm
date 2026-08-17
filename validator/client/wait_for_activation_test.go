package client

import (
	"context"
	"fmt"
	"testing"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	validatormock "github.com/OffchainLabs/prysm/v7/testing/validator-mock"
	walletMock "github.com/OffchainLabs/prysm/v7/validator/accounts/testing"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager/derived"
	constant "github.com/OffchainLabs/prysm/v7/validator/testing"
	"github.com/pkg/errors"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/tyler-smith/go-bip39"
	util "github.com/wealdtech/go-eth2-util"
	"go.uber.org/mock/gomock"
)

func TestWaitActivation_Exiting_OK(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	validatorClient := validatormock.NewMockValidatorClient(ctrl)
	chainClient := validatormock.NewMockChainClient(ctrl)
	kp := randKeypair(t)
	v := validator{
		validatorClient:        validatorClient,
		km:                     newMockKeymanager(t, kp),
		chainClient:            chainClient,
		accountsChangedChannel: make(chan [][fieldparams.BLSPubkeyLength]byte, 1),
	}
	ctx := t.Context()
	resp := generateMultipleValidatorStatusResponse([][]byte{kp.pub[:]})
	resp.Statuses[0].Status = ethpb.ValidatorStatus_EXITING
	validatorClient.EXPECT().MultipleValidatorStatus(
		gomock.Any(),
		&ethpb.MultipleValidatorStatusRequest{
			PublicKeys: [][]byte{kp.pub[:]},
		},
	).Return(resp, nil)

	require.NoError(t, v.WaitForActivation(ctx))
	require.Equal(t, 1, len(v.pubkeyToStatus))
}

func TestWaitForActivation_RefetchKeys(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.MainnetConfig()
	cfg.ConfigName = "test"
	cfg.SlotDurationMilliseconds = 1000
	params.OverrideBeaconConfig(cfg)
	hook := logTest.NewGlobal()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	validatorClient := validatormock.NewMockValidatorClient(ctrl)
	chainClient := validatormock.NewMockChainClient(ctrl)

	kp := randKeypair(t)
	km := newMockKeymanager(t)

	v := validator{
		validatorClient: validatorClient,
		km:              km,
		chainClient:     chainClient,
		pubkeyToStatus:  make(map[[48]byte]*validatorStatus),
	}
	resp := generateMultipleValidatorStatusResponse([][]byte{kp.pub[:]})
	resp.Statuses[0].Status = ethpb.ValidatorStatus_ACTIVE

	validatorClient.EXPECT().MultipleValidatorStatus(
		gomock.Any(),
		&ethpb.MultipleValidatorStatusRequest{
			PublicKeys: [][]byte{kp.pub[:]},
		},
	).Return(resp, nil)

	accountChan := make(chan [][fieldparams.BLSPubkeyLength]byte, 1)
	sub := km.SubscribeAccountChanges(accountChan)
	defer func() {
		sub.Unsubscribe()
		close(accountChan)
	}()
	v.accountsChangedChannel = accountChan
	// update the accounts from 0 to 1 after a delay
	go func() {
		time.Sleep(1 * time.Second)
		require.NoError(t, km.add(kp))
		km.SimulateAccountChanges([][48]byte{kp.pub})
	}()
	assert.NoError(t, v.WaitForActivation(t.Context()), "Could not wait for activation")
	assert.LogsContain(t, hook, msgNoKeysFetched)
	assert.LogsContain(t, hook, "Validator activated")
}

// quarantineTestValidator wires a validator with the given keymanager and a
// subscribed accounts-changed channel, for doppelganger quarantine tests.
func quarantineTestValidator(t *testing.T, ctrl *gomock.Controller, km *mockKeymanager) (*validator, *validatormock.MockValidatorClient) {
	client := validatormock.NewMockValidatorClient(ctrl)
	v := &validator{
		validatorClient: client,
		km:              km,
		pubkeyToStatus:  make(map[[48]byte]*validatorStatus),
	}
	accountChan := make(chan [][fieldparams.BLSPubkeyLength]byte, 1)
	sub := km.SubscribeAccountChanges(accountChan)
	t.Cleanup(func() {
		sub.Unsubscribe()
		close(accountChan)
	})
	v.accountsChangedChannel = accountChan
	return v, client
}

// allActiveStatuses answers a status request marking every requested key ACTIVE.
func allActiveStatuses(_ context.Context, req *ethpb.MultipleValidatorStatusRequest) (*ethpb.MultipleValidatorStatusResponse, error) {
	resp := generateMultipleValidatorStatusResponse(req.PublicKeys)
	for i := range resp.Statuses {
		resp.Statuses[i].Status = ethpb.ValidatorStatus_ACTIVE
	}
	return resp, nil
}

func TestWaitForActivation_DoppelGangerQuarantine(t *testing.T) {
	t.Run("keys present at boot are not quarantined", func(t *testing.T) {
		enableDoppelGanger(t)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		kp := randKeypair(t)
		v, client := quarantineTestValidator(t, ctrl, newMockKeymanager(t, kp))
		client.EXPECT().MultipleValidatorStatus(gomock.Any(), gomock.Any()).DoAndReturn(allActiveStatuses)

		// The initial call leaves boot keys for the startup check to vet.
		require.NoError(t, v.WaitForActivation(t.Context()))
		assert.Equal(t, false, v.isDoppelGangerPending(kp.pub))
	})

	t.Run("a key imported while waiting with no validating keys is quarantined", func(t *testing.T) {
		enableDoppelGanger(t)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		kp := randKeypair(t)
		km := newMockKeymanager(t)
		v, client := quarantineTestValidator(t, ctrl, km)
		client.EXPECT().MultipleValidatorStatus(gomock.Any(), gomock.Any()).DoAndReturn(allActiveStatuses)

		// The import lands while WaitForActivation is blocked on the accounts
		// channel: this path bypasses HandleKeyReload entirely.
		go func() {
			time.Sleep(100 * time.Millisecond)
			require.NoError(t, km.add(kp))
			km.SimulateAccountChanges([][48]byte{kp.pub})
		}()
		require.NoError(t, v.WaitForActivation(t.Context()))
		assert.Equal(t, true, v.isDoppelGangerPending(kp.pub))
	})

	t.Run("a key imported during the connection-retry backoff is quarantined", func(t *testing.T) {
		enableDoppelGanger(t)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		kp := randKeypair(t)
		late := randKeypair(t)
		km := newMockKeymanager(t)
		v, client := quarantineTestValidator(t, ctrl, km)

		// A status failure sends the accounts-changed entry into its backoff
		// sleep; the retry's fetch is the first to ever see the late key, so it
		// must inherit the entry's accounts-changed origin to quarantine it.
		gomock.InOrder(
			client.EXPECT().MultipleValidatorStatus(gomock.Any(), gomock.Any()).Return(nil, errors.New("connection refused")),
			client.EXPECT().MultipleValidatorStatus(gomock.Any(), gomock.Any()).DoAndReturn(allActiveStatuses),
		)

		go func() {
			time.Sleep(100 * time.Millisecond)
			require.NoError(t, km.add(kp))
			km.SimulateAccountChanges([][48]byte{kp.pub})
			time.Sleep(400 * time.Millisecond) // mid-backoff
			require.NoError(t, km.add(late))
		}()
		require.NoError(t, v.WaitForActivation(t.Context()))
		assert.Equal(t, true, v.isDoppelGangerPending(kp.pub))
		assert.Equal(t, true, v.isDoppelGangerPending(late.pub))
	})
}

func TestWaitForActivation_AccountsChanged(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	hook := logTest.NewGlobal()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	t.Run("Imported keymanager", func(t *testing.T) {
		inactive := randKeypair(t)
		active := randKeypair(t)
		km := newMockKeymanager(t, inactive)
		validatorClient := validatormock.NewMockValidatorClient(ctrl)
		chainClient := validatormock.NewMockChainClient(ctrl)
		ch := make(chan [][fieldparams.BLSPubkeyLength]byte, 1)
		v := validator{
			validatorClient:        validatorClient,
			km:                     km,
			chainClient:            chainClient,
			pubkeyToStatus:         make(map[[48]byte]*validatorStatus),
			accountsChangedChannel: ch,
			accountChangedSub:      km.SubscribeAccountChanges(ch),
		}
		defer func() {
			close(v.accountsChangedChannel)
			v.accountChangedSub.Unsubscribe()
		}()

		inactiveResp := generateMultipleValidatorStatusResponse([][]byte{inactive.pub[:]})
		inactiveResp.Statuses[0].Status = ethpb.ValidatorStatus_UNKNOWN_STATUS

		activeResp := generateMultipleValidatorStatusResponse([][]byte{inactive.pub[:], active.pub[:]})
		activeResp.Statuses[0].Status = ethpb.ValidatorStatus_UNKNOWN_STATUS
		activeResp.Statuses[1].Status = ethpb.ValidatorStatus_ACTIVE
		gomock.InOrder(
			validatorClient.EXPECT().MultipleValidatorStatus(
				gomock.Any(),
				&ethpb.MultipleValidatorStatusRequest{
					PublicKeys: [][]byte{inactive.pub[:]},
				},
			).Return(inactiveResp, nil).Do(func(arg0, arg1 any) {
				require.NoError(t, km.add(active))
				km.SimulateAccountChanges([][fieldparams.BLSPubkeyLength]byte{inactive.pub, active.pub})
			}),
			validatorClient.EXPECT().MultipleValidatorStatus(
				gomock.Any(),
				&ethpb.MultipleValidatorStatusRequest{
					PublicKeys: [][]byte{inactive.pub[:], active.pub[:]},
				},
			).Return(activeResp, nil))

		chainClient.EXPECT().ChainHead(
			gomock.Any(),
			gomock.Any(),
		).Return(
			&ethpb.ChainHead{HeadEpoch: 0},
			nil,
		).AnyTimes()
		assert.NoError(t, v.WaitForActivation(t.Context()))
		assert.LogsContain(t, hook, "Waiting for deposit to be observed by beacon node")
		assert.LogsContain(t, hook, "Validator activated")
	})

	t.Run("Derived keymanager", func(t *testing.T) {
		seed := bip39.NewSeed(constant.TestMnemonic, "")
		inactivePrivKey, err :=
			util.PrivateKeyFromSeedAndPath(seed, fmt.Sprintf(derived.ValidatingKeyDerivationPathTemplate, 0))
		require.NoError(t, err)
		var inactivePubKey [fieldparams.BLSPubkeyLength]byte
		copy(inactivePubKey[:], inactivePrivKey.PublicKey().Marshal())
		activePrivKey, err :=
			util.PrivateKeyFromSeedAndPath(seed, fmt.Sprintf(derived.ValidatingKeyDerivationPathTemplate, 1))
		require.NoError(t, err)
		var activePubKey [fieldparams.BLSPubkeyLength]byte
		copy(activePubKey[:], activePrivKey.PublicKey().Marshal())
		wallet := &walletMock.Wallet{
			Files:            make(map[string]map[string][]byte),
			AccountPasswords: make(map[string]string),
			WalletPassword:   "secretPassw0rd$1999",
		}
		ctx := t.Context()
		km, err := derived.NewKeymanager(ctx, &derived.SetupConfig{
			Wallet:           wallet,
			ListenForChanges: true,
		})
		require.NoError(t, err)
		err = km.RecoverAccountsFromMnemonic(ctx, constant.TestMnemonic, derived.DefaultMnemonicLanguage, "", 1)
		require.NoError(t, err)
		validatorClient := validatormock.NewMockValidatorClient(ctrl)
		chainClient := validatormock.NewMockChainClient(ctrl)
		v := validator{
			validatorClient: validatorClient,
			km:              km,
			genesisTime:     time.Unix(1, 0),
			chainClient:     chainClient,
			pubkeyToStatus:  make(map[[48]byte]*validatorStatus),
		}

		inactiveResp := generateMultipleValidatorStatusResponse([][]byte{inactivePubKey[:]})
		inactiveResp.Statuses[0].Status = ethpb.ValidatorStatus_UNKNOWN_STATUS

		activeResp := generateMultipleValidatorStatusResponse([][]byte{inactivePubKey[:], activePubKey[:]})
		activeResp.Statuses[0].Status = ethpb.ValidatorStatus_UNKNOWN_STATUS
		activeResp.Statuses[1].Status = ethpb.ValidatorStatus_ACTIVE
		channel := make(chan [][fieldparams.BLSPubkeyLength]byte, 1)
		km.SubscribeAccountChanges(channel)
		v.accountsChangedChannel = channel
		gomock.InOrder(
			validatorClient.EXPECT().MultipleValidatorStatus(
				gomock.Any(),
				&ethpb.MultipleValidatorStatusRequest{
					PublicKeys: [][]byte{inactivePubKey[:]},
				},
			).Return(inactiveResp, nil).Do(func(arg0, arg1 any) {
				err = km.RecoverAccountsFromMnemonic(ctx, constant.TestMnemonic, derived.DefaultMnemonicLanguage, "", 2)
				require.NoError(t, err)
				pks, err := km.FetchValidatingPublicKeys(ctx)
				require.NoError(t, err)
				require.DeepEqual(t, pks, [][fieldparams.BLSPubkeyLength]byte{inactivePubKey, activePubKey})
				channel <- [][fieldparams.BLSPubkeyLength]byte{inactivePubKey, activePubKey}
			}),
			validatorClient.EXPECT().MultipleValidatorStatus(
				gomock.Any(),
				&ethpb.MultipleValidatorStatusRequest{
					PublicKeys: [][]byte{inactivePubKey[:], activePubKey[:]},
				},
			).Return(activeResp, nil))

		chainClient.EXPECT().ChainHead(
			gomock.Any(),
			gomock.Any(),
		).Return(
			&ethpb.ChainHead{HeadEpoch: 0},
			nil,
		).AnyTimes()
		assert.NoError(t, v.WaitForActivation(t.Context()))
		assert.LogsContain(t, hook, "Waiting for deposit to be observed by beacon node")
		assert.LogsContain(t, hook, "Validator activated")
	})
}

func TestWaitForActivation_AttemptsReconnectionOnFailure(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.MainnetConfig()
	cfg.ConfigName = "test"
	cfg.SlotDurationMilliseconds = 1000
	params.OverrideBeaconConfig(cfg)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	validatorClient := validatormock.NewMockValidatorClient(ctrl)
	chainClient := validatormock.NewMockChainClient(ctrl)
	kp := randKeypair(t)
	v := validator{
		validatorClient:        validatorClient,
		km:                     newMockKeymanager(t, kp),
		chainClient:            chainClient,
		pubkeyToStatus:         make(map[[48]byte]*validatorStatus),
		accountsChangedChannel: make(chan [][fieldparams.BLSPubkeyLength]byte, 1),
	}
	active := randKeypair(t)
	activeResp := generateMultipleValidatorStatusResponse([][]byte{active.pub[:]})
	activeResp.Statuses[0].Status = ethpb.ValidatorStatus_ACTIVE
	gomock.InOrder(
		validatorClient.EXPECT().MultipleValidatorStatus(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, errors.New("some random connection error")),
		validatorClient.EXPECT().MultipleValidatorStatus(
			gomock.Any(),
			gomock.Any(),
		).Return(activeResp, nil))
	chainClient.EXPECT().ChainHead(
		gomock.Any(),
		gomock.Any(),
	).Return(
		&ethpb.ChainHead{HeadEpoch: 0},
		nil,
	).AnyTimes()
	assert.NoError(t, v.WaitForActivation(t.Context()))
}
