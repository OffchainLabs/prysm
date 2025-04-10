package beacon

import (
	"context"
	"fmt"
	"reflect"

	"github.com/prysmaticlabs/prysm/v5/config/params"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Deprecated: gRPC API will still be supported for some time, most likely until v8 in 2026, but will be eventually removed in favor of REST API.
//
// GetBeaconConfig retrieves the current configuration parameters of the beacon chain.
func (_ *Server) GetBeaconConfig(_ context.Context, _ *emptypb.Empty) (*ethpb.BeaconConfig, error) {
	conf := params.BeaconConfig()
	val := reflect.ValueOf(conf).Elem()
	numFields := val.Type().NumField()
	res := make(map[string]string, numFields)
	for i := 0; i < numFields; i++ {
		res[val.Type().Field(i).Name] = fmt.Sprintf("%v", val.Field(i).Interface())
	}
	return &ethpb.BeaconConfig{
		Config: res,
	}, nil
}
