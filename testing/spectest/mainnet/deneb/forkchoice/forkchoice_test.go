package forkchoice

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v6/runtime/version"
	"github.com/prysmaticlabs/prysm/v6/testing/spectest/shared/common/forkchoice"
)

func TestMainnet_Deneb_Forkchoice(t *testing.T) {
	forkchoice.Run(t, "mainnet", version.Deneb)
}
