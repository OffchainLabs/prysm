//go:build !noMainnetGenesis
// +build !noMainnetGenesis

package embedded

import (
	_ "embed"

	"github.com/OffchainLabs/prysm/v6/config/params"
)

var (
	//go:embed mainnet.ssz.snappy
	mainnetRawSSZCompressed []byte // 1.8Mb
)

func init() {
	embeddedStates[params.MainnetName] = &mainnetRawSSZCompressed
}
