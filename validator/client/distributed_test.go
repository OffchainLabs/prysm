package client

import (
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	validatormock "github.com/OffchainLabs/prysm/v7/testing/validator-mock"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	"go.uber.org/mock/gomock"
)

func TestAggregatedSelectionProofs_EmptyDuties(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	// Inactive validators are filtered at duty ingestion, so an empty store
	// (simulating no active duties) should produce zero selection requests.
	ds := &dutyStore{
		currentDuties: map[pubkey]*ethpb.AttesterDuty{},
		initialized:   true,
	}

	client.EXPECT().AggregatedSelections(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ any, req []iface.BeaconCommitteeSelection) ([]iface.BeaconCommitteeSelection, error) {
			assert.Equal(t, 0, len(req))
			return nil, nil
		},
	)

	v := &validator{
		validatorClient: client,
	}
	err := v.aggregatedSelectionProofs(t.Context(), ds)
	require.NoError(t, err)
}

func TestAggregatedSelectionProofs_StoresResults(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	kp := randKeypair(t)
	km := newMockKeymanager(t, kp)

	ds := &dutyStore{
		currentDuties: map[pubkey]*ethpb.AttesterDuty{
			kp.pub: {Pubkey: kp.pub[:], ValidatorIndex: 1, Slot: 5},
		},
		initialized: true,
	}

	client.EXPECT().DomainData(gomock.Any(), gomock.Any()).Return(
		&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil,
	)
	client.EXPECT().AggregatedSelections(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ any, req []iface.BeaconCommitteeSelection) ([]iface.BeaconCommitteeSelection, error) {
			require.Equal(t, 1, len(req))
			assert.Equal(t, primitives.Slot(5), req[0].Slot)
			assert.Equal(t, primitives.ValidatorIndex(1), req[0].ValidatorIndex)
			return req, nil
		},
	)

	v := &validator{
		validatorClient: client,
		km:              km,
		attSelections:   make(map[attSelectionKey]iface.BeaconCommitteeSelection),
		pubkeyToStatus: map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus{
			kp.pub: {status: &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE}},
		},
	}
	err := v.aggregatedSelectionProofs(t.Context(), ds)
	require.NoError(t, err)

	sel, ok := v.attSelections[attSelectionKey{slot: 5, index: 1}]
	assert.Equal(t, true, ok)
	assert.Equal(t, primitives.Slot(5), sel.Slot)
}
