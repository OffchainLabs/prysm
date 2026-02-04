package grpc_api

import (
	"errors"
	"testing"

	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/mock"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	validatorHelpers "github.com/OffchainLabs/prysm/v7/validator/helpers"
	"github.com/golang/protobuf/ptypes/empty"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
)

func TestGrpcNodeClient_IsReady(t *testing.T) {
	// The IsReady function now relies on GetHealth which returns:
	// - 200 OK (nil error) only if node is synced AND not optimistic
	// - 206 Partial Content (error) if syncing or optimistic
	// - 503 Unavailable (error) if unavailable
	testCases := []struct {
		name           string
		healthErr      error
		expectedResult bool
	}{
		{
			name:           "returns true when health check succeeds (synced and not optimistic)",
			healthErr:      nil,
			expectedResult: true,
		},
		{
			name:           "returns false when health check fails (syncing, optimistic, or unavailable)",
			healthErr:      errors.New("node not ready"),
			expectedResult: false,
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

			result := client.IsReady(ctx)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}
