package apiutil

import (
	"net/url"
	"testing"

	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
)

func TestBeaconApiHelpers_TestUint64ToString(t *testing.T) {
	const expectedResult = "1234"
	const val = uint64(1234)

	assert.Equal(t, expectedResult, Uint64ToString(val))
	assert.Equal(t, expectedResult, Uint64ToString(primitives.Slot(val)))
	assert.Equal(t, expectedResult, Uint64ToString(primitives.ValidatorIndex(val)))
	assert.Equal(t, expectedResult, Uint64ToString(primitives.CommitteeIndex(val)))
	assert.Equal(t, expectedResult, Uint64ToString(primitives.Epoch(val)))
}

func TestBuildURL_NoParams(t *testing.T) {
	wanted := "/aaa/bbb/ccc"
	actual := BuildURL("/aaa/bbb/ccc")
	assert.Equal(t, wanted, actual)
}

func TestBuildURL_WithParams(t *testing.T) {
	params := url.Values{}
	params.Add("xxxx", "1")
	params.Add("yyyy", "2")
	params.Add("zzzz", "3")

	wanted := "/aaa/bbb/ccc?xxxx=1&yyyy=2&zzzz=3"
	actual := BuildURL("/aaa/bbb/ccc", params)
	assert.Equal(t, wanted, actual)
}
