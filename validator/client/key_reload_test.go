package client

import (
	"testing"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	validatormock "github.com/OffchainLabs/prysm/v6/testing/validator-mock"
	"github.com/OffchainLabs/prysm/v6/validator/client/testutil"
	"github.com/pkg/errors"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"go.uber.org/mock/gomock"
)

func TestValidator_HandleKeyReload(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("active", func(t *testing.T) {
		hook := logTest.NewGlobal()

		inactive := randKeypair(t)
		active := randKeypair(t)

		client := validatormock.NewMockValidatorClient(ctrl)
		chainClient := validatormock.NewMockChainClient(ctrl)
		prysmChainClient := validatormock.NewMockPrysmChainClient(ctrl)
		v := validator{
			validatorClient:  client,
			km:               newMockKeymanager(t, inactive),
			genesisTime:      time.Unix(1, 0),
			chainClient:      chainClient,
			prysmChainClient: prysmChainClient,
			pubkeyToStatus:   make(map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus),
		}

		resp := testutil.GenerateMultipleValidatorStatusResponse([][]byte{inactive.pub[:], active.pub[:]})
		resp.Statuses[0].Status = ethpb.ValidatorStatus_UNKNOWN_STATUS
		resp.Statuses[1].Status = ethpb.ValidatorStatus_ACTIVE
		client.EXPECT().MultipleValidatorStatus(
			gomock.Any(),
			&ethpb.MultipleValidatorStatusRequest{
				PublicKeys: [][]byte{inactive.pub[:], active.pub[:]},
			},
		).Return(resp, nil)

		anyActive, err := v.HandleKeyReload(t.Context(), [][fieldparams.BLSPubkeyLength]byte{inactive.pub, active.pub})
		require.NoError(t, err)
		assert.Equal(t, true, anyActive)
		assert.LogsContain(t, hook, "Waiting for deposit to be observed by beacon node")
		assert.LogsContain(t, hook, "Validator activated")
	})

	t.Run("no active", func(t *testing.T) {
		hook := logTest.NewGlobal()

		client := validatormock.NewMockValidatorClient(ctrl)
		chainClient := validatormock.NewMockChainClient(ctrl)
		prysmChainClient := validatormock.NewMockPrysmChainClient(ctrl)
		kp := randKeypair(t)
		v := validator{
			validatorClient:  client,
			km:               newMockKeymanager(t, kp),
			genesisTime:      time.Unix(1, 0),
			chainClient:      chainClient,
			prysmChainClient: prysmChainClient,
			pubkeyToStatus:   make(map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus),
		}

		resp := testutil.GenerateMultipleValidatorStatusResponse([][]byte{kp.pub[:]})
		resp.Statuses[0].Status = ethpb.ValidatorStatus_UNKNOWN_STATUS
		client.EXPECT().MultipleValidatorStatus(
			gomock.Any(),
			&ethpb.MultipleValidatorStatusRequest{
				PublicKeys: [][]byte{kp.pub[:]},
			},
		).Return(resp, nil)

		anyActive, err := v.HandleKeyReload(t.Context(), [][fieldparams.BLSPubkeyLength]byte{kp.pub})
		require.NoError(t, err)
		assert.Equal(t, false, anyActive)
		assert.LogsContain(t, hook, "Waiting for deposit to be observed by beacon node")
		assert.LogsDoNotContain(t, hook, "Validator activated")
	})

	t.Run("error when getting status", func(t *testing.T) {
		kp := randKeypair(t)
		client := validatormock.NewMockValidatorClient(ctrl)
		v := validator{
			validatorClient: client,
			km:              newMockKeymanager(t, kp),
			genesisTime:     time.Unix(1, 0),
			pubkeyToStatus:  make(map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus),
		}

		client.EXPECT().MultipleValidatorStatus(
			gomock.Any(),
			&ethpb.MultipleValidatorStatusRequest{
				PublicKeys: [][]byte{kp.pub[:]},
			},
		).Return(nil, errors.New("error"))

		_, err := v.HandleKeyReload(t.Context(), [][fieldparams.BLSPubkeyLength]byte{kp.pub})
		assert.ErrorContains(t, "error", err)
	})
}
