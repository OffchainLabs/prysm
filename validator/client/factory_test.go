package client

import (
	"testing"

	grpcutil "github.com/OffchainLabs/prysm/v7/api/grpc"
	"github.com/OffchainLabs/prysm/v7/api/rest"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	validatorHelpers "github.com/OffchainLabs/prysm/v7/validator/helpers"
)

// The connection generation must come from whichever provider the factory
// actually selected, so host switches on the active transport are detected.
func TestNewValidatorClient_ConnectionGenerationFollowsTransport(t *testing.T) {
	var (
		grpcConnCounter uint64 = 2
		restConnCounter uint64 = 9
	)
	grpcProvider := &grpcutil.MockGrpcProvider{MockHosts: []string{"localhost:4000"}, ConnCounter: grpcConnCounter}
	restProvider := &rest.MockRestProvider{MockHosts: []string{"http://localhost:3500"}, ConnCounter: restConnCounter}

	t.Run("gRPC mode reads grpc counter even when rest provider present", func(t *testing.T) {
		conn, err := validatorHelpers.NewNodeConnection(
			validatorHelpers.WithGRPCProvider(grpcProvider), validatorHelpers.WithRestProvider(restProvider))
		require.NoError(t, err)
		assert.Equal(t, grpcConnCounter, NewValidatorClient(conn).ConnectionGeneration())
	})

	t.Run("REST mode reads rest counter even when grpc provider present", func(t *testing.T) {
		// Regression (r3511683129): gRPC provider is non-nil (its endpoint flag
		// defaults), but in REST mode the active provider is REST.
		reset := features.InitWithReset(&features.Flags{EnableBeaconRESTApi: true})
		defer reset()
		conn, err := validatorHelpers.NewNodeConnection(
			validatorHelpers.WithGRPCProvider(grpcProvider), validatorHelpers.WithRestProvider(restProvider))
		require.NoError(t, err)
		assert.Equal(t, restConnCounter, NewValidatorClient(conn).ConnectionGeneration())
	})

	t.Run("REST flag without rest provider falls back to grpc counter", func(t *testing.T) {
		// The factory falls back to the gRPC client here, so the generation must
		// track the gRPC counter, not sit at zero.
		reset := features.InitWithReset(&features.Flags{EnableBeaconRESTApi: true})
		defer reset()
		conn, err := validatorHelpers.NewNodeConnection(validatorHelpers.WithGRPCProvider(grpcProvider))
		require.NoError(t, err)
		assert.Equal(t, grpcConnCounter, NewValidatorClient(conn).ConnectionGeneration())
	})
}
