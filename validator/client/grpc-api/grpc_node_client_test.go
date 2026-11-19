package grpc_api

import (
	"errors"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/fallback"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/mock"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	validatorHelpers "github.com/OffchainLabs/prysm/v7/validator/helpers"
	"github.com/golang/protobuf/ptypes/empty"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGrpcNodeClient_ReadyStatus(t *testing.T) {
	testCases := []struct {
		name           string
		healthErr      error
		expectedStatus fallback.NodeStatus
	}{
		{
			name:           "ready when health check succeeds",
			healthErr:      nil,
			expectedStatus: fallback.NodeReady,
		},
		{
			name:           "syncing",
			healthErr:      status.Error(codes.Unavailable, api.HealthSyncingReason),
			expectedStatus: fallback.NodeSyncing,
		},
		{
			name:           "optimistic",
			healthErr:      status.Error(codes.Unavailable, api.HealthOptimisticReason),
			expectedStatus: fallback.NodeOptimistic,
		},
		{
			name:           "unavailable on transport error",
			healthErr:      status.Error(codes.Unavailable, "connection error"),
			expectedStatus: fallback.NodeUnavailable,
		},
		{
			name:           "unavailable on other error",
			healthErr:      errors.New("node not ready"),
			expectedStatus: fallback.NodeUnavailable,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			ctx := t.Context()

			mockNodeClient := mock.NewMockNodeClient(ctrl)

			// Set up health check expectation
			mockNodeClient.EXPECT().GetHealth(
				gomock.Any(),
				gomock.Any(),
			).Return(&empty.Empty{}, tc.healthErr)

			// Create a mock provider
			provider := &mockProvider{hosts: []string{"host1:4000"}}
			conn, err := validatorHelpers.NewNodeConnection(validatorHelpers.WithGRPCProvider(provider))
			require.NoError(t, err)

			// Create client with injected mock
			client := &grpcNodeClient{
				grpcClientManager: &grpcClientManager[ethpb.NodeClient]{
					conn:            conn,
					client:          mockNodeClient,
					lastConnCounter: 0,
					newClient:       func(grpc.ClientConnInterface) ethpb.NodeClient { return mockNodeClient },
				},
			}

			assert.Equal(t, tc.expectedStatus, client.ReadyStatus(ctx))
		})
	}
}
