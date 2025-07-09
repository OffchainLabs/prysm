package mainnet

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/spectest/shared/bellatrix/ssz_static"
)

func TestMainnet_Bellatrix_SSZStatic(t *testing.T) {
	ssz_static.RunSSZStaticTests(t, "mainnet")
}
