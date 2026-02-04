package rpc

import (
	"testing"

	grpcutil "github.com/OffchainLabs/prysm/v7/api/grpc"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"google.golang.org/grpc/metadata"
)

func TestGrpcHeaders(t *testing.T) {
	ctx := t.Context()
	grpcHeaders := []string{"first=value1", "second=value2"}
	ctx = grpcutil.AppendHeaders(ctx, grpcHeaders)
	md, _ := metadata.FromOutgoingContext(ctx)
	require.Equal(t, 2, md.Len(), "MetadataV0 contains wrong number of values")
	assert.Equal(t, "value1", md.Get("first")[0])
	assert.Equal(t, "value2", md.Get("second")[0])
}
