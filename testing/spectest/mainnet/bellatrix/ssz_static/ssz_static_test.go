package ssz_static

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/testing/spectest/shared/bellatrix/ssz_static"
)

func TestMainnet_Bellatrix_SSZStatic(t *testing.T) {
	ssz_static.RunSSZStaticTests(t, "mainnet")
}
