package verification

import (
	"os"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
)

func TestMain(t *testing.M) {
	if err := kzg.Start(); err != nil {
		os.Exit(1)
	}
	t.Run()
}
