package client

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/pkg/errors"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	validatormock "github.com/prysmaticlabs/prysm/v5/testing/validator-mock"
	walletMock "github.com/prysmaticlabs/prysm/v5/validator/accounts/testing"
	"github.com/prysmaticlabs/prysm/v5/validator/client/testutil"
	"github.com/prysmaticlabs/prysm/v5/validator/keymanager/derived"
	constant "github.com/prysmaticlabs/prysm/v5/validator/testing"
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
	prysmChainClient := validatormock.NewMockPrysmChainClient(ctrl)
	kp := randKeypair(t)
	v := validator{
		validatorClient:  validatorClient,
		km:               newMockKeymanager(t, kp),
		chainClient:      chainClient,
		prysmChainClient: prysmChainClient,
	}
	ctx := context.Background()
	resp := testutil.GenerateMultipleValidatorStatusResponse([][]byte{kp.pub[:]})
	resp.Statuses[0].Status = ethpb.ValidatorStatus_EXITING
	validatorClient.EXPECT().MultipleValidatorStatus(
		gomock.Any(),
		&ethpb.MultipleValidatorStatusRequest{
			PublicKeys: [][]byte{kp.pub[:]},
		},
	).Return(resp, nil)

	require.NoError(t, v.WaitForActivation(ctx, nil))
	require.Equal(t, 1, len(v.pubkeyToStatus))
}

func TestWaitForActivation_RefetchKeys(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.MainnetConfig().Copy()
	cfg.ConfigName = "test"
	cfg.SecondsPerSlot = 1
	params.OverrideBeaconConfig(cfg)
	hook := logTest.NewGlobal()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	validatorClient := validatormock.NewMockValidatorClient(ctrl)
	chainClient := validatormock.NewMockChainClient(ctrl)
	prysmChainClient := validatormock.NewMockPrysmChainClient(ctrl)

	kp := randKeypair(t)
	km := newMockKeymanager(t)

	v := validator{
		validatorClient:  validatorClient,
		km:               km,
		chainClient:      chainClient,
		prysmChainClient: prysmChainClient,
		pubkeyToStatus:   make(map[[48]byte]*validatorStatus),
	}
	resp := testutil.GenerateMultipleValidatorStatusResponse([][]byte{kp.pub[:]})
	resp.Statuses[0].Status = ethpb.ValidatorStatus_ACTIVE

	validatorClient.EXPECT().MultipleValidatorStatus(
		gomock.Any(),
		&ethpb.MultipleValidatorStatusRequest{
			PublicKeys: [][]byte{kp.pub[:]},
		},
	).Return(resp, nil)

	accountChan := make(chan [][fieldparams.BLSPubkeyLength]byte)
	sub := km.SubscribeAccountChanges(accountChan)
	defer func() {
		sub.Unsubscribe()
		close(accountChan)
	}()
	// update the accounts from 0 to 1 after a delay
	go func() {
		time.Sleep(1 * time.Second)
		require.NoError(t, km.add(kp))
		km.SimulateAccountChanges([][48]byte{kp.pub})
	}()
	assert.NoError(t, v.internalWaitForActivation(context.Background(), accountChan), "Could not wait for activation")
	assert.LogsContain(t, hook, msgNoKeysFetched)
	assert.LogsContain(t, hook, "Validator activated")
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
		prysmChainClient := validatormock.NewMockPrysmChainClient(ctrl)
		v := validator{
			validatorClient:  validatorClient,
			km:               km,
			chainClient:      chainClient,
			prysmChainClient: prysmChainClient,
			pubkeyToStatus:   make(map[[48]byte]*validatorStatus),
		}
		inactiveResp := testutil.GenerateMultipleValidatorStatusResponse([][]byte{inactive.pub[:]})
		inactiveResp.Statuses[0].Status = ethpb.ValidatorStatus_UNKNOWN_STATUS

		activeResp := testutil.GenerateMultipleValidatorStatusResponse([][]byte{inactive.pub[:], active.pub[:]})
		activeResp.Statuses[0].Status = ethpb.ValidatorStatus_UNKNOWN_STATUS
		activeResp.Statuses[1].Status = ethpb.ValidatorStatus_ACTIVE
		gomock.InOrder(
			validatorClient.EXPECT().MultipleValidatorStatus(
				gomock.Any(),
				&ethpb.MultipleValidatorStatusRequest{
					PublicKeys: [][]byte{inactive.pub[:]},
				},
			).Return(inactiveResp, nil).Do(func(arg0, arg1 interface{}) {
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
		assert.NoError(t, v.WaitForActivation(context.Background(), nil))
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
		ctx := context.Background()
		km, err := derived.NewKeymanager(ctx, &derived.SetupConfig{
			Wallet:           wallet,
			ListenForChanges: true,
		})
		require.NoError(t, err)
		err = km.RecoverAccountsFromMnemonic(ctx, constant.TestMnemonic, derived.DefaultMnemonicLanguage, "", 1)
		require.NoError(t, err)
		validatorClient := validatormock.NewMockValidatorClient(ctrl)
		chainClient := validatormock.NewMockChainClient(ctrl)
		prysmChainClient := validatormock.NewMockPrysmChainClient(ctrl)
		v := validator{
			validatorClient:  validatorClient,
			km:               km,
			genesisTime:      1,
			chainClient:      chainClient,
			prysmChainClient: prysmChainClient,
			pubkeyToStatus:   make(map[[48]byte]*validatorStatus),
		}

		inactiveResp := testutil.GenerateMultipleValidatorStatusResponse([][]byte{inactivePubKey[:]})
		inactiveResp.Statuses[0].Status = ethpb.ValidatorStatus_UNKNOWN_STATUS

		activeResp := testutil.GenerateMultipleValidatorStatusResponse([][]byte{inactivePubKey[:], activePubKey[:]})
		activeResp.Statuses[0].Status = ethpb.ValidatorStatus_UNKNOWN_STATUS
		activeResp.Statuses[1].Status = ethpb.ValidatorStatus_ACTIVE
		channel := make(chan [][fieldparams.BLSPubkeyLength]byte, 1)
		km.SubscribeAccountChanges(channel)
		gomock.InOrder(
			validatorClient.EXPECT().MultipleValidatorStatus(
				gomock.Any(),
				&ethpb.MultipleValidatorStatusRequest{
					PublicKeys: [][]byte{inactivePubKey[:]},
				},
			).Return(inactiveResp, nil).Do(func(arg0, arg1 interface{}) {
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
		assert.NoError(t, v.internalWaitForActivation(context.Background(), channel))
		assert.LogsContain(t, hook, "Waiting for deposit to be observed by beacon node")
		assert.LogsContain(t, hook, "Validator activated")
	})
}

func TestWaitForActivation_AttemptsReconnectionOnFailure(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.MainnetConfig().Copy()
	cfg.ConfigName = "test"
	cfg.SecondsPerSlot = 1
	params.OverrideBeaconConfig(cfg)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	validatorClient := validatormock.NewMockValidatorClient(ctrl)
	chainClient := validatormock.NewMockChainClient(ctrl)
	prysmChainClient := validatormock.NewMockPrysmChainClient(ctrl)
	kp := randKeypair(t)
	v := validator{
		validatorClient:  validatorClient,
		km:               newMockKeymanager(t, kp),
		chainClient:      chainClient,
		prysmChainClient: prysmChainClient,
		pubkeyToStatus:   make(map[[48]byte]*validatorStatus),
	}
	active := randKeypair(t)
	activeResp := testutil.GenerateMultipleValidatorStatusResponse([][]byte{active.pub[:]})
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
	assert.NoError(t, v.WaitForActivation(context.Background(), nil))
}
