package grpc_api

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	eventClient "github.com/OffchainLabs/prysm/v6/api/client/event"
	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	mock2 "github.com/OffchainLabs/prysm/v6/testing/mock"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestToValidatorDutiesContainer_HappyPath(t *testing.T) {
	// Create a mock DutiesResponse with current and next duties.
	dutiesResp := &eth.DutiesResponse{
		CurrentEpochDuties: []*eth.DutiesResponse_Duty{
			{
				Committee:        []primitives.ValidatorIndex{100, 101},
				CommitteeIndex:   4,
				AttesterSlot:     200,
				ProposerSlots:    []primitives.Slot{400},
				PublicKey:        []byte{0xAA, 0xBB},
				Status:           eth.ValidatorStatus_ACTIVE,
				ValidatorIndex:   101,
				IsSyncCommittee:  false,
				CommitteesAtSlot: 2,
			},
		},
		NextEpochDuties: []*eth.DutiesResponse_Duty{
			{
				Committee:        []primitives.ValidatorIndex{300, 301},
				CommitteeIndex:   8,
				AttesterSlot:     600,
				ProposerSlots:    []primitives.Slot{700, 701},
				PublicKey:        []byte{0xCC, 0xDD},
				Status:           eth.ValidatorStatus_ACTIVE,
				ValidatorIndex:   301,
				IsSyncCommittee:  true,
				CommitteesAtSlot: 3,
			},
		},
	}

	gotContainer, err := toValidatorDutiesContainer(dutiesResp)
	require.NoError(t, err)

	// Validate we have the correct number of duties in current and next epochs.
	assert.Equal(t, len(gotContainer.CurrentEpochDuties), len(dutiesResp.CurrentEpochDuties))
	assert.Equal(t, len(gotContainer.NextEpochDuties), len(dutiesResp.NextEpochDuties))

	firstCurrentDuty := gotContainer.CurrentEpochDuties[0]
	expectedCurrentDuty := dutiesResp.CurrentEpochDuties[0]
	assert.DeepEqual(t, firstCurrentDuty.PublicKey, expectedCurrentDuty.PublicKey)
	assert.Equal(t, firstCurrentDuty.ValidatorIndex, expectedCurrentDuty.ValidatorIndex)
	assert.DeepEqual(t, firstCurrentDuty.ProposerSlots, expectedCurrentDuty.ProposerSlots)

	firstNextDuty := gotContainer.NextEpochDuties[0]
	expectedNextDuty := dutiesResp.NextEpochDuties[0]
	assert.DeepEqual(t, firstNextDuty.PublicKey, expectedNextDuty.PublicKey)
	assert.Equal(t, firstNextDuty.ValidatorIndex, expectedNextDuty.ValidatorIndex)
	assert.DeepEqual(t, firstNextDuty.ProposerSlots, expectedNextDuty.ProposerSlots)
}

func TestToValidatorDutiesContainerV2_HappyPath(t *testing.T) {
	// Create a mock DutiesResponse with current and next duties.
	dutiesResp := &eth.DutiesV2Response{
		CurrentEpochDuties: []*eth.DutiesV2Response_Duty{
			{
				CommitteeLength:         2,
				CommitteeIndex:          4,
				ValidatorCommitteeIndex: 1,
				AttesterSlot:            200,
				ProposerSlots:           []primitives.Slot{400},
				PublicKey:               []byte{0xAA, 0xBB},
				Status:                  eth.ValidatorStatus_ACTIVE,
				ValidatorIndex:          101,
				IsSyncCommittee:         false,
				CommitteesAtSlot:        2,
			},
		},
		NextEpochDuties: []*eth.DutiesV2Response_Duty{
			{
				CommitteeLength:         2,
				CommitteeIndex:          8,
				ValidatorCommitteeIndex: 1,
				AttesterSlot:            600,
				ProposerSlots:           []primitives.Slot{700, 701},
				PublicKey:               []byte{0xCC, 0xDD},
				Status:                  eth.ValidatorStatus_ACTIVE,
				ValidatorIndex:          301,
				IsSyncCommittee:         true,
				CommitteesAtSlot:        3,
			},
		},
	}

	gotContainer, err := toValidatorDutiesContainerV2(dutiesResp)
	require.NoError(t, err)

	// Validate we have the correct number of duties in current and next epochs.
	assert.Equal(t, len(gotContainer.CurrentEpochDuties), len(dutiesResp.CurrentEpochDuties))
	assert.Equal(t, len(gotContainer.NextEpochDuties), len(dutiesResp.NextEpochDuties))

	firstCurrentDuty := gotContainer.CurrentEpochDuties[0]
	expectedCurrentDuty := dutiesResp.CurrentEpochDuties[0]
	assert.DeepEqual(t, firstCurrentDuty.PublicKey, expectedCurrentDuty.PublicKey)
	assert.Equal(t, firstCurrentDuty.ValidatorIndex, expectedCurrentDuty.ValidatorIndex)
	assert.DeepEqual(t, firstCurrentDuty.ProposerSlots, expectedCurrentDuty.ProposerSlots)

	firstNextDuty := gotContainer.NextEpochDuties[0]
	expectedNextDuty := dutiesResp.NextEpochDuties[0]
	assert.DeepEqual(t, firstNextDuty.PublicKey, expectedNextDuty.PublicKey)
	assert.Equal(t, firstNextDuty.ValidatorIndex, expectedNextDuty.ValidatorIndex)
	assert.DeepEqual(t, firstNextDuty.ProposerSlots, expectedNextDuty.ProposerSlots)
}

func TestWaitForChainStart_StreamSetupFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	beaconNodeValidatorClient := mock2.NewMockBeaconNodeValidatorClient(ctrl)
	beaconNodeValidatorClient.EXPECT().WaitForChainStart(
		gomock.Any(),
		gomock.Any(),
	).Return(nil, errors.New("failed stream"))

	validatorClient := &grpcValidatorClient{beaconNodeValidatorClient, true}
	_, err := validatorClient.WaitForChainStart(t.Context(), &emptypb.Empty{})
	want := "could not setup beacon chain ChainStart streaming client"
	assert.ErrorContains(t, want, err)
}

func TestStartEventStream(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	beaconNodeValidatorClient := mock2.NewMockBeaconNodeValidatorClient(ctrl)
	grpcClient := &grpcValidatorClient{beaconNodeValidatorClient, true}
	tests := []struct {
		name    string
		topics  []string
		prepare func()
		verify  func(t *testing.T, event *eventClient.Event)
	}{
		{
			name:   "Happy path Head topic",
			topics: []string{"head"},
			prepare: func() {
				stream := mock2.NewMockBeaconNodeValidator_StreamSlotsClient(ctrl)
				beaconNodeValidatorClient.EXPECT().StreamSlots(gomock.Any(),
					&eth.StreamSlotsRequest{VerifiedOnly: true}).Return(stream, nil)
				stream.EXPECT().Context().Return(ctx).AnyTimes()
				stream.EXPECT().Recv().Return(
					&eth.StreamSlotsResponse{Slot: 123},
					nil,
				).AnyTimes()
			},
			verify: func(t *testing.T, event *eventClient.Event) {
				require.Equal(t, event.EventType, eventClient.EventHead)
				head := structs.HeadEvent{}
				require.NoError(t, json.Unmarshal(event.Data, &head))
				require.Equal(t, head.Slot, "123")
			},
		},
		{
			name:   "no head produces error",
			topics: []string{"unsupportedTopic"},
			prepare: func() {
				stream := mock2.NewMockBeaconNodeValidator_StreamSlotsClient(ctrl)
				beaconNodeValidatorClient.EXPECT().StreamSlots(gomock.Any(),
					&eth.StreamSlotsRequest{VerifiedOnly: true}).Return(stream, nil)
				stream.EXPECT().Context().Return(ctx).AnyTimes()
				stream.EXPECT().Recv().Return(
					&eth.StreamSlotsResponse{Slot: 123},
					nil,
				).AnyTimes()
			},
			verify: func(t *testing.T, event *eventClient.Event) {
				require.Equal(t, event.EventType, eventClient.EventConnectionError)
			},
		},
		{
			name:   "Unsupported topics warning",
			topics: []string{"head", "unsupportedTopic"},
			prepare: func() {
				stream := mock2.NewMockBeaconNodeValidator_StreamSlotsClient(ctrl)
				beaconNodeValidatorClient.EXPECT().StreamSlots(gomock.Any(),
					&eth.StreamSlotsRequest{VerifiedOnly: true}).Return(stream, nil)
				stream.EXPECT().Context().Return(ctx).AnyTimes()
				stream.EXPECT().Recv().Return(
					&eth.StreamSlotsResponse{Slot: 123},
					nil,
				).AnyTimes()
			},
			verify: func(t *testing.T, event *eventClient.Event) {
				require.Equal(t, event.EventType, eventClient.EventHead)
				head := structs.HeadEvent{}
				require.NoError(t, json.Unmarshal(event.Data, &head))
				require.Equal(t, head.Slot, "123")
				assert.LogsContain(t, hook, "gRPC only supports the head topic")
			},
		},
		{
			name:    "No topics error",
			topics:  []string{},
			prepare: func() {},
			verify: func(t *testing.T, event *eventClient.Event) {
				require.Equal(t, event.EventType, eventClient.EventError)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			eventsChannel := make(chan *eventClient.Event, 1) // Buffer to prevent blocking
			tc.prepare()                                      // Setup mock expectations

			go grpcClient.StartEventStream(ctx, tc.topics, eventsChannel)

			event := <-eventsChannel
			// Depending on what you're testing, you may need a timeout or a specific number of events to read
			time.AfterFunc(1*time.Second, cancel) // Prevents hanging forever
			tc.verify(t, event)
		})
	}
}
