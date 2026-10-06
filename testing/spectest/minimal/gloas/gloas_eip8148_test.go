package gloas

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/spectest/utils"
)

func TestMain(m *testing.M) {
	utils.SkipGloasSuiteForEip8148(m)
}
