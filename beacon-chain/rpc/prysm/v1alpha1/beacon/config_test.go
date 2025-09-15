package beacon

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestServer_GetBeaconConfig(t *testing.T) {
	ctx := t.Context()
	bs := &Server{}
	res, err := bs.GetBeaconConfig(ctx, &emptypb.Empty{})
	require.NoError(t, err)
	conf := params.BeaconConfig()
	confType := reflect.TypeOf(conf).Elem()
	numFields := confType.NumField()

	// Count only exported fields, as unexported fields are not included in the config
	exportedFields := 0
	for i := 0; i < numFields; i++ {
		if confType.Field(i).IsExported() {
			exportedFields++
		}
	}

	// Check if the result has the same number of items as exported fields in our config struct.
	assert.Equal(t, exportedFields, len(res.Config), "Unexpected number of items in config")
	want := fmt.Sprintf("%d", conf.Eth1FollowDistance)

	// Check that an element is properly populated from the config.
	assert.Equal(t, want, res.Config["Eth1FollowDistance"], "Unexpected follow distance")
}
