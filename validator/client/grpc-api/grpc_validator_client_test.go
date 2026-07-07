package grpc_api

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	eventClient "github.com/OffchainLabs/prysm/v7/api/client/event"
	grpcutil "github.com/OffchainLabs/prysm/v7/api/grpc"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	mock2 "github.com/OffchainLabs/prysm/v7/testing/mock"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/validator/client/cache"
	validatorHelpers "github.com/OffchainLabs/prysm/v7/validator/helpers"
	validatorTesting "github.com/OffchainLabs/prysm/v7/validator/testing"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
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

	validatorClient := &grpcValidatorClient{
		grpcClientManager: newGrpcClientManager(
			validatorTesting.MockNodeConnection(),
			func(_ grpc.ClientConnInterface) eth.BeaconNodeValidatorClient {
				return beaconNodeValidatorClient
			},
		),
		isEventStreamRunning: true,
	}
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
	grpcClient := &grpcValidatorClient{
		grpcClientManager: newGrpcClientManager(
			validatorTesting.MockNodeConnection(),
			func(_ grpc.ClientConnInterface) eth.BeaconNodeValidatorClient {
				return beaconNodeValidatorClient
			},
		),
		isEventStreamRunning: true,
	}
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

func TestEnsureReady(t *testing.T) {
	tests := []struct {
		name           string
		hosts          []string
		healthResults  []error // one per GetHealth call in order
		expectedResult bool
		expectedIndex  int // expected provider index after EnsureReady
	}{
		{
			name:           "Single host ready",
			hosts:          []string{"host1:4000"},
			healthResults:  []error{nil},
			expectedResult: true,
			expectedIndex:  0,
		},
		{
			name:           "Single host not ready",
			hosts:          []string{"host1:4000"},
			healthResults:  []error{errors.New("not synced")},
			expectedResult: false,
			expectedIndex:  0,
		},
		{
			name:           "Multiple hosts first ready",
			hosts:          []string{"host1:4000", "host2:4000", "host3:4000"},
			healthResults:  []error{nil},
			expectedResult: true,
			expectedIndex:  0,
		},
		{
			name:           "Failover to second host",
			hosts:          []string{"host1:4000", "host2:4000", "host3:4000"},
			healthResults:  []error{errors.New("not synced"), nil},
			expectedResult: true,
			expectedIndex:  1,
		},
		{
			name:           "Failover to third host",
			hosts:          []string{"host1:4000", "host2:4000", "host3:4000"},
			healthResults:  []error{errors.New("not synced"), errors.New("not synced"), nil},
			expectedResult: true,
			expectedIndex:  2,
		},
		{
			name:           "All hosts down",
			hosts:          []string{"host1:4000", "host2:4000", "host3:4000"},
			healthResults:  []error{errors.New("not synced"), errors.New("not synced"), errors.New("not synced")},
			expectedResult: false,
			expectedIndex:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockProvider := &grpcutil.MockGrpcProvider{
				MockHosts: tt.hosts,
			}
			conn, err := validatorHelpers.NewNodeConnection(
				validatorHelpers.WithGRPCProvider(mockProvider),
			)
			require.NoError(t, err)

			mockNodeClient := mock2.NewMockNodeClient(ctrl)
			for _, healthErr := range tt.healthResults {
				if healthErr != nil {
					mockNodeClient.EXPECT().GetHealth(gomock.Any(), gomock.Any()).Return(nil, healthErr)
				} else {
					mockNodeClient.EXPECT().GetHealth(gomock.Any(), gomock.Any()).Return(&emptypb.Empty{}, nil)
				}
			}

			client := &grpcValidatorClient{
				grpcClientManager: newGrpcClientManager(conn, func(_ grpc.ClientConnInterface) eth.BeaconNodeValidatorClient {
					return mock2.NewMockBeaconNodeValidatorClient(ctrl)
				}),
				nodeClient: &grpcNodeClient{
					grpcClientManager: newGrpcClientManager(conn, func(_ grpc.ClientConnInterface) eth.NodeClient {
						return mockNodeClient
					}),
				},
			}

			result := client.EnsureReady(t.Context())
			assert.Equal(t, tt.expectedResult, result)
			assert.Equal(t, tt.expectedIndex, mockProvider.CurrentIndex)
		})
	}
}

func newTestGrpcValidatorClient(t *testing.T, client eth.BeaconNodeValidatorClient, stateless bool) *grpcValidatorClient {
	t.Helper()
	return &grpcValidatorClient{
		grpcClientManager: newGrpcClientManager(
			validatorTesting.MockNodeConnection(),
			func(_ grpc.ClientConnInterface) eth.BeaconNodeValidatorClient { return client },
		),
		stateless:     stateless,
		envelopeCache: cache.NewExecutionPayloadEnvelopeCache(),
	}
}

func TestBeaconBlock(t *testing.T) {
	slot := primitives.Slot(7)
	blockRoot := bytesutil.ToBytes32([]byte("beacon-block-root"))
	gloasBlock := util.NewBeaconBlockGloas().Block

	gloasContentsResp := &eth.GenericBeaconBlock{
		Block: &eth.GenericBeaconBlock_GloasContents{
			GloasContents: &eth.BeaconBlockContentsGloas{
				Block: gloasBlock,
				ExecutionPayloadEnvelope: &eth.ExecutionPayloadEnvelope{
					Payload:         &enginev1.ExecutionPayloadGloas{SlotNumber: slot},
					BeaconBlockRoot: blockRoot[:],
				},
				Blobs:     [][]byte{[]byte("blob")},
				KzgProofs: [][]byte{[]byte("proof")},
			},
		},
	}
	blockOnlyResp := &eth.GenericBeaconBlock{Block: &eth.GenericBeaconBlock_Gloas{Gloas: gloasBlock}}

	tests := []struct {
		name      string
		stateless bool
		resp      *eth.GenericBeaconBlock
		verify    func(t *testing.T, vc *grpcValidatorClient, got *eth.GenericBeaconBlock, err error)
	}{
		{
			name:      "stateless gloas contents are cached and unwrapped to a block-only response",
			stateless: true,
			resp:      gloasContentsResp,
			verify: func(t *testing.T, vc *grpcValidatorClient, got *eth.GenericBeaconBlock, err error) {
				require.NoError(t, err)
				// The proposer receives the block alone, matching the non-stateless response shape.
				gloas, ok := got.GetBlock().(*eth.GenericBeaconBlock_Gloas)
				require.Equal(t, true, ok)
				require.Equal(t, gloasBlock, gloas.Gloas)
				// The publisher's envelope + blobs were cached under the request slot.
				cachedEnv, blobs, proofs := vc.envelopeCache.Peek(slot)
				require.NotNil(t, cachedEnv)
				require.Equal(t, slot, cachedEnv.Payload.SlotNumber)
				require.Equal(t, 1, len(blobs))
				require.Equal(t, 1, len(proofs))
			},
		},
		{
			name:      "a non-contents response is returned unchanged and nothing is cached",
			stateless: false,
			resp:      blockOnlyResp,
			verify: func(t *testing.T, vc *grpcValidatorClient, got *eth.GenericBeaconBlock, err error) {
				require.NoError(t, err)
				require.Equal(t, blockOnlyResp, got)
				_, _, proofs := vc.envelopeCache.Peek(slot)
				require.IsNil(t, proofs)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := mock2.NewMockBeaconNodeValidatorClient(ctrl)
			client.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, in *eth.BlockRequest, _ ...grpc.CallOption) (*eth.GenericBeaconBlock, error) {
					// EagerPayloadStateRoot is set on the request iff the client is stateless.
					require.Equal(t, tt.stateless, in.EagerPayloadStateRoot)
					return tt.resp, nil
				})

			vc := newTestGrpcValidatorClient(t, client, tt.stateless)
			got, err := vc.BeaconBlock(t.Context(), &eth.BlockRequest{Slot: slot})
			tt.verify(t, vc, got, err)
		})
	}
}

func TestGetExecutionPayloadEnvelope(t *testing.T) {
	slot := primitives.Slot(9)
	matchingRoot := bytesutil.ToBytes32([]byte("matching-root"))
	requestedRoot := bytesutil.ToBytes32([]byte("requested-root"))
	cachedEnvelope := &eth.ExecutionPayloadEnvelope{
		Payload:         &enginev1.ExecutionPayloadGloas{SlotNumber: slot},
		BeaconBlockRoot: matchingRoot[:],
	}

	tests := []struct {
		name        string
		root        [32]byte
		prepare     func(vc *grpcValidatorClient, client *mock2.MockBeaconNodeValidatorClient)
		wantErr     string
		wantFull    bool
		wantBlinded bool
	}{
		{
			name: "cache hit returns full envelope",
			root: matchingRoot,
			// No gRPC EXPECT: a cache hit must short-circuit before any network call.
			prepare: func(vc *grpcValidatorClient, _ *mock2.MockBeaconNodeValidatorClient) {
				vc.envelopeCache.Add(slot, cachedEnvelope, nil, nil)
			},
			wantFull: true,
		},
		{
			name: "cache hit with mismatched root errors",
			root: requestedRoot,
			prepare: func(vc *grpcValidatorClient, _ *mock2.MockBeaconNodeValidatorClient) {
				vc.envelopeCache.Add(slot, cachedEnvelope, nil, nil)
			},
			wantErr: "cached execution payload envelope beacon_block_root does not match",
		},
		{
			name: "cache miss returns blinded envelope",
			root: matchingRoot,
			prepare: func(_ *grpcValidatorClient, client *mock2.MockBeaconNodeValidatorClient) {
				client.EXPECT().GetExecutionPayloadEnvelope(gomock.Any(), &eth.ExecutionPayloadEnvelopeRequest{Slot: slot}).Return(
					&eth.ExecutionPayloadEnvelopeResponse{Blinded: &eth.WireBlindedExecutionPayloadEnvelope{BeaconBlockRoot: matchingRoot[:]}}, nil)
			},
			wantBlinded: true,
		},
		{
			name: "cache miss with mismatched blinded root errors",
			root: requestedRoot,
			prepare: func(_ *grpcValidatorClient, client *mock2.MockBeaconNodeValidatorClient) {
				client.EXPECT().GetExecutionPayloadEnvelope(gomock.Any(), gomock.Any()).Return(
					&eth.ExecutionPayloadEnvelopeResponse{Blinded: &eth.WireBlindedExecutionPayloadEnvelope{BeaconBlockRoot: matchingRoot[:]}}, nil)
			},
			wantErr: "blinded execution payload envelope beacon_block_root does not match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := mock2.NewMockBeaconNodeValidatorClient(ctrl)
			vc := newTestGrpcValidatorClient(t, client, true)
			tt.prepare(vc, client)

			full, blinded, err := vc.GetExecutionPayloadEnvelope(t.Context(), slot, tt.root)
			if tt.wantErr != "" {
				require.ErrorContains(t, tt.wantErr, err)
				return
			}
			require.NoError(t, err)
			if tt.wantFull {
				require.Equal(t, cachedEnvelope, full)
			} else {
				require.IsNil(t, full)
			}
			if tt.wantBlinded {
				require.NotNil(t, blinded)
			} else {
				require.IsNil(t, blinded)
			}
		})
	}
}
