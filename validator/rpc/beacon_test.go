package rpc

import (
	"context"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"google.golang.org/grpc/metadata"
)

func TestGrpcHeaders(t *testing.T) {
	s := &Server{
		ctx:         context.Background(),
		grpcHeaders: []string{"first=value1", "second=value2"},
	}
	err := s.registerBeaconClient()
	require.NoError(t, err)
	md, _ := metadata.FromOutgoingContext(s.ctx)
	require.Equal(t, 2, md.Len(), "MetadataV0 contains wrong number of values")
	assert.Equal(t, "value1", md.Get("first")[0])
	assert.Equal(t, "value2", md.Get("second")[0])
}
