package beacon

import (
	"context"
	"fmt"
	"reflect"

	"github.com/OffchainLabs/prysm/v6/config/params"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Deprecated: The gRPC API will remain the default and fully supported through v8 (expected in 2026) but will be eventually removed in favor of REST API.
//
// GetBeaconConfig retrieves the current configuration parameters of the beacon chain.
func (_ *Server) GetBeaconConfig(_ context.Context, _ *emptypb.Empty) (*ethpb.BeaconConfig, error) {
	conf := params.BeaconConfig()
	val := reflect.ValueOf(conf).Elem()
	numFields := val.Type().NumField()
	res := make(map[string]string, numFields)
	for i := 0; i < numFields; i++ {
		field := val.Type().Field(i)
		if field.IsExported() {
			res[field.Name] = fmt.Sprintf("%v", val.Field(i).Interface())
		}
	}
	return &ethpb.BeaconConfig{
		Config: res,
	}, nil
}
