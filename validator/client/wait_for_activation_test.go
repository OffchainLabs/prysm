package client

import (
	"context"
	"fmt"
	"sync/atomic"
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

// allUnknownStatuses answers a status request marking every requested key UNKNOWN.
func allUnknownStatuses(_ context.Context, req *ethpb.MultipleValidatorStatusRequest) (*ethpb.MultipleValidatorStatusResponse, error) {
	return generateMultipleValidatorStatusResponse(req.PublicKeys), nil
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

	t.Run("a key imported during the connection-retry wait is quarantined", func(t *testing.T) {
		enableDoppelGanger(t)
		params.SetupTestConfigCleanup(t)
		cfg := params.MainnetConfig().Copy()
		cfg.SlotDurationMilliseconds = 50
		params.OverrideBeaconConfig(cfg)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		kp := randKeypair(t)
		late := randKeypair(t)
		km := newMockKeymanager(t)
		v, client := quarantineTestValidator(t, ctrl, km)
		monitor, healthy := testHealthMonitor(t)
		v.healthMonitor = monitor
		monitor.Start()

		// A status failure sends the accounts-changed entry into its retry wait;
		// the retry's fetch is the first to ever see the late key, so it must
		// inherit the entry's accounts-changed origin to quarantine it.
		gomock.InOrder(
			client.EXPECT().MultipleValidatorStatus(gomock.Any(), gomock.Any()).Return(nil, errors.New("connection refused")),
			client.EXPECT().MultipleValidatorStatus(gomock.Any(), gomock.Any()).DoAndReturn(allActiveStatuses),
		)

		go func() {
			time.Sleep(100 * time.Millisecond)
			require.NoError(t, km.add(kp))
			km.SimulateAccountChanges([][48]byte{kp.pub})
			time.Sleep(100 * time.Millisecond) // the retry is now blocked on the health monitor
			require.NoError(t, km.add(late))
			healthy.Store(true)
		}()
		require.NoError(t, v.WaitForActivation(t.Context()))
		assert.Equal(t, true, v.isDoppelGangerPending(kp.pub))
		assert.Equal(t, true, v.isDoppelGangerPending(late.pub))
	})

	t.Run("a key imported during the next-epoch wait is quarantined", func(t *testing.T) {
		enableDoppelGanger(t)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		kp := randKeypair(t)
		late := randKeypair(t)
		km := newMockKeymanager(t, kp)
		v, client := quarantineTestValidator(t, ctrl, km)
		v.genesisTime = time.Now() // a full epoch until the next one, so only the import ends the wait

		// An inactive key sends the loop into its next-epoch wait; the import
		// cuts that wait short and must count as an accounts change.
		gomock.InOrder(
			client.EXPECT().MultipleValidatorStatus(gomock.Any(), gomock.Any()).DoAndReturn(allUnknownStatuses),
			client.EXPECT().MultipleValidatorStatus(gomock.Any(), gomock.Any()).DoAndReturn(allActiveStatuses),
		)

		go func() {
			time.Sleep(100 * time.Millisecond)
			require.NoError(t, km.add(late))
			km.SimulateAccountChanges([][48]byte{kp.pub, late.pub})
		}()
		require.NoError(t, v.WaitForActivation(t.Context()))
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

func TestWaitForActivation_NextEpoch(t *testing.T) {
	t.Run("re-checks statuses once the next epoch starts", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.MainnetConfig().Copy()
		cfg.SlotDurationMilliseconds = 50
		params.OverrideBeaconConfig(cfg)
		hook := logTest.NewGlobal()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		validatorClient := validatormock.NewMockValidatorClient(ctrl)
		kp := randKeypair(t)
		// Start in the last slot of the epoch so the wait ends within a second.
		lastSlot := time.Duration(params.BeaconConfig().SlotsPerEpoch-1) * params.BeaconConfig().SlotDuration()
		v := &validator{
			validatorClient:        validatorClient,
			km:                     newMockKeymanager(t, kp),
			pubkeyToStatus:         make(map[[48]byte]*validatorStatus),
			accountsChangedChannel: make(chan [][fieldparams.BLSPubkeyLength]byte, 1),
			genesisTime:            time.Now().Add(-lastSlot),
		}
		gomock.InOrder(
			validatorClient.EXPECT().MultipleValidatorStatus(gomock.Any(), gomock.Any()).DoAndReturn(allUnknownStatuses),
			validatorClient.EXPECT().MultipleValidatorStatus(gomock.Any(), gomock.Any()).DoAndReturn(allActiveStatuses),
		)

		require.NoError(t, v.WaitForActivation(t.Context()))
		assert.LogsContain(t, hook, "Waiting until next epoch to check again")
		assert.LogsContain(t, hook, "Validator activated")
	})

	t.Run("returns the context error when cancelled mid-wait", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		validatorClient := validatormock.NewMockValidatorClient(ctrl)
		kp := randKeypair(t)
		v := &validator{
			validatorClient:        validatorClient,
			km:                     newMockKeymanager(t, kp),
			pubkeyToStatus:         make(map[[48]byte]*validatorStatus),
			accountsChangedChannel: make(chan [][fieldparams.BLSPubkeyLength]byte, 1),
			genesisTime:            time.Now(), // a full epoch until the next one, so only the cancel ends the wait
		}
		validatorClient.EXPECT().MultipleValidatorStatus(gomock.Any(), gomock.Any()).DoAndReturn(allUnknownStatuses)
		ctx, cancel := context.WithCancel(t.Context())
		time.AfterFunc(100*time.Millisecond, cancel)

		require.ErrorIs(t, v.WaitForActivation(ctx), context.Canceled)
	})
}

func TestWaitForActivation_AttemptsReconnectionOnFailure(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.MainnetConfig()
	cfg.ConfigName = "test"
	cfg.SlotDurationMilliseconds = 50
	params.OverrideBeaconConfig(cfg)

	// reconnectionTestValidator wires a validator whose first status call fails
	// and whose second reports an active key; its health monitor is not started.
	reconnectionTestValidator := func(t *testing.T) (*validator, *atomic.Bool) {
		ctrl := gomock.NewController(t)
		validatorClient := validatormock.NewMockValidatorClient(ctrl)
		chainClient := validatormock.NewMockChainClient(ctrl)
		kp := randKeypair(t)
		monitor, healthy := testHealthMonitor(t)
		v := &validator{
			validatorClient:        validatorClient,
			km:                     newMockKeymanager(t, kp),
			chainClient:            chainClient,
			pubkeyToStatus:         make(map[[48]byte]*validatorStatus),
			accountsChangedChannel: make(chan [][fieldparams.BLSPubkeyLength]byte, 1),
			healthMonitor:          monitor,
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
		return v, healthy
	}

	t.Run("retries once a probe reports the beacon node healthy", func(t *testing.T) {
		v, healthy := reconnectionTestValidator(t)
		healthy.Store(true)
		v.healthMonitor.Start()
		assert.NoError(t, v.WaitForActivation(t.Context()))
	})

	t.Run("retry blocks until a probe reports the beacon node healthy", func(t *testing.T) {
		hook := logTest.NewGlobal()
		v, healthy := reconnectionTestValidator(t)
		v.healthMonitor.Start()
		flipped := make(chan time.Time, 1)
		go func() {
			time.Sleep(200 * time.Millisecond)
			healthy.Store(true)
			flipped <- time.Now()
		}()
		assert.NoError(t, v.WaitForActivation(t.Context()))
		assert.Equal(t, false, time.Now().Before(<-flipped), "retry completed before the monitor became healthy")
		assert.LogsContain(t, hook, "Beacon node still unhealthy, waiting before retrying")
	})

	t.Run("returns the context error instead of waiting when the context is cancelled", func(t *testing.T) {
		hook := logTest.NewGlobal()
		ctrl := gomock.NewController(t)
		validatorClient := validatormock.NewMockValidatorClient(ctrl)
		kp := randKeypair(t)
		v := &validator{
			validatorClient:        validatorClient,
			km:                     newMockKeymanager(t, kp),
			pubkeyToStatus:         make(map[[48]byte]*validatorStatus),
			accountsChangedChannel: make(chan [][fieldparams.BLSPubkeyLength]byte, 1),
		}
		ctx, cancel := context.WithCancel(t.Context())
		validatorClient.EXPECT().MultipleValidatorStatus(gomock.Any(), gomock.Any()).DoAndReturn(
			func(context.Context, *ethpb.MultipleValidatorStatusRequest) (*ethpb.MultipleValidatorStatusResponse, error) {
				cancel()
				return nil, context.Canceled
			})

		require.ErrorIs(t, v.WaitForActivation(ctx), context.Canceled)
		assert.LogsDoNotContain(t, hook, "Connection broken while waiting for activation")
	})
}
