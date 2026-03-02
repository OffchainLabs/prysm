package client

import (
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	validatormock "github.com/OffchainLabs/prysm/v7/testing/validator-mock"
	"go.uber.org/mock/gomock"
)

func TestSubscribeToSubnets_EmptyDuties(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	// Inactive validators are filtered at duty ingestion, so an empty store
	// (simulating that no active duties exist) should produce zero subscriptions.
	v := &validator{
		validatorClient: client,
		duties:          newDutyStoreFromLegacy(&ethpb.ValidatorDutiesContainer{}),
	}

	client.EXPECT().SubscribeCommitteeSubnets(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ any, req *ethpb.CommitteeSubnetsSubscribeRequest, _ []primitives.ValidatorIndex, _ []uint64) (*struct{}, error) {
			assert.Equal(t, 0, len(req.Slots))
			return nil, nil
		},
	)

	err := v.subscribeToSubnets(t.Context())
	require.NoError(t, err)
}

func TestSubscribeToSubnets_ActiveValidators(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	kp := randKeypair(t)
	km := newMockKeymanager(t, kp)

	v := &validator{
		validatorClient: client,
		km:              km,
		duties: newDutyStoreFromLegacy(&ethpb.ValidatorDutiesContainer{
			CurrentEpochDuties: []*ethpb.ValidatorDuty{
				{
					PublicKey:       kp.pub[:],
					ValidatorIndex:  1,
					AttesterSlot:    5,
					CommitteeIndex:  2,
					CommitteeLength: 128,
					Status:          ethpb.ValidatorStatus_ACTIVE,
				},
			},
		}),
		pubkeyToStatus: map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus{
			kp.pub: {status: &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE}},
		},
	}

	client.EXPECT().DomainData(gomock.Any(), gomock.Any()).Return(
		&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil,
	)
	client.EXPECT().SubscribeCommitteeSubnets(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ any, req *ethpb.CommitteeSubnetsSubscribeRequest, valIndices []primitives.ValidatorIndex, _ []uint64) (*struct{}, error) {
			require.Equal(t, 1, len(req.Slots))
			assert.Equal(t, primitives.Slot(5), req.Slots[0])
			assert.Equal(t, primitives.CommitteeIndex(2), req.CommitteeIds[0])
			require.Equal(t, 1, len(valIndices))
			assert.Equal(t, primitives.ValidatorIndex(1), valIndices[0])
			return nil, nil
		},
	)

	err := v.subscribeToSubnets(t.Context())
	require.NoError(t, err)
}

func TestSubscribeToSubnets_IncludesNextEpochDuties(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	kp := randKeypair(t)
	km := newMockKeymanager(t, kp)

	ds := &dutyStore{
		currentDuties: attesterMap(&ethpb.AttesterDutiesResponse{}),
		nextDuties: attesterMap(&ethpb.AttesterDutiesResponse{
			Duties: []*ethpb.AttesterDuty{
				{
					Pubkey:          kp.pub[:],
					ValidatorIndex:  1,
					Slot:            37,
					CommitteeIndex:  1,
					CommitteeLength: 64,
				},
			},
		}),
		initialized: true,
	}
	v := &validator{
		validatorClient: client,
		km:              km,
		duties:          ds,
		pubkeyToStatus: map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus{
			kp.pub: {status: &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE}},
		},
	}

	client.EXPECT().DomainData(gomock.Any(), gomock.Any()).Return(
		&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil,
	)
	client.EXPECT().SubscribeCommitteeSubnets(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ any, req *ethpb.CommitteeSubnetsSubscribeRequest, _ []primitives.ValidatorIndex, _ []uint64) (*struct{}, error) {
			require.Equal(t, 1, len(req.Slots))
			assert.Equal(t, primitives.Slot(37), req.Slots[0])
			return nil, nil
		},
	)

	err := v.subscribeToSubnets(t.Context())
	require.NoError(t, err)
}

func TestValidatorSubnetSubscriptionKey_Deterministic(t *testing.T) {
	key1 := validatorSubnetSubscriptionKey(10, 3)
	key2 := validatorSubnetSubscriptionKey(10, 3)
	assert.Equal(t, key1, key2)

	key3 := validatorSubnetSubscriptionKey(10, 4)
	assert.NotEqual(t, key1, key3)

	key4 := validatorSubnetSubscriptionKey(11, 3)
	assert.NotEqual(t, key1, key4)
}

func TestValidatorSubnetSubscriptionKey_NoCollision(t *testing.T) {
	a := validatorSubnetSubscriptionKey(1, 256)
	b := validatorSubnetSubscriptionKey(256, 1)
	assert.NotEqual(t, a, b)
}
